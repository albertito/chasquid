package courier

import (
	"crypto/tls"
	"expvar"
	"flag"
	"net"
	"os"
	"time"

	"golang.org/x/net/idna"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/smtp"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

var (
	// Timeouts for SMTP delivery.
	smtpDialTimeout  = 1 * time.Minute
	smtpTotalTimeout = 10 * time.Minute

	// Port for outgoing SMTP.
	// Tests can override this.
	smtpPort = flag.String("testing__outgoing_smtp_port", "25",
		"port to use for outgoing SMTP connections, ONLY FOR TESTING")

	// Fake MX records, used for testing only.
	fakeMX = map[string]string{}
)

// Exported variables.
var (
	tlsCount   = expvar.NewMap("chasquid/smtpOut/tlsCount")
	slcResults = expvar.NewMap("chasquid/smtpOut/securityLevelChecks")
)

// SMTP delivers remote mail via outgoing SMTP.
type SMTP struct {
	Dinfo *domaininfo.DB
}

func (s *SMTP) Deliver(from string, to string, data []byte) (error, bool) {
	tr := trace.New("Courier.SMTP", to)
	defer tr.Finish()
	tr.Debugf("%s  ->  %s", from, to)

	toDomain := envelope.DomainOf(to)
	mx, err := lookupMX(tr, toDomain)
	if err != nil {
		// Note this is considered a permanent error.
		// This is in line with what other servers (Exim) do. However, the
		// downside is that temporary DNS issues can affect delivery, so we
		// have to make sure we try hard enough on the lookup above.
		return tr.Errorf("Could not find mail server: %v", err), true
	}

	// Issue an EHLO with a valid domain; otherwise, some servers like postfix
	// will complain.
	helloDomain, err := idna.ToASCII(envelope.DomainOf(from))
	if err != nil {
		return tr.Errorf("Sender domain not IDNA compliant: %v", err), true
	}
	if helloDomain == "" {
		// This can happen when sending bounces. Last resort.
		helloDomain, _ = os.Hostname()
	}

	// Do we use insecure TLS?
	// Set as fallback when retrying.
	insecure := false
	secLevel := domaininfo.SecLevel_PLAIN

retry:
	conn, err := net.DialTimeout("tcp", mx+":"+*smtpPort, smtpDialTimeout)
	if err != nil {
		return tr.Errorf("Could not dial: %v", err), false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(smtpTotalTimeout))

	c, err := smtp.NewClient(conn, mx)
	if err != nil {
		return tr.Errorf("Error creating client: %v", err), false
	}

	if err = c.Hello(helloDomain); err != nil {
		return tr.Errorf("Error saying hello: %v", err), false
	}

	if ok, _ := c.Extension("STARTTLS"); ok {
		config := &tls.Config{
			ServerName:         mx,
			InsecureSkipVerify: insecure,
		}
		err = c.StartTLS(config)
		if err != nil {
			// Unfortunately, many servers use self-signed certs, so if we
			// fail verification we just try again without validating.
			if insecure {
				tlsCount.Add("tls:failed", 1)
				return tr.Errorf("TLS error: %v", err), false
			}

			insecure = true
			tr.Debugf("TLS error, retrying insecurely")
			goto retry
		}

		if config.InsecureSkipVerify {
			tr.Debugf("Insecure - using TLS, but cert does not match %s", mx)
			tlsCount.Add("tls:insecure", 1)
			secLevel = domaininfo.SecLevel_TLS_INSECURE
		} else {
			tlsCount.Add("tls:secure", 1)
			tr.Debugf("Secure - using TLS")
			secLevel = domaininfo.SecLevel_TLS_SECURE
		}
	} else {
		tlsCount.Add("plain", 1)
		tr.Debugf("Insecure - NOT using TLS")
	}

	if toDomain != "" && !s.Dinfo.OutgoingSecLevel(toDomain, secLevel) {
		// We consider the failure transient, so transient misconfigurations
		// do not affect deliveries.
		slcResults.Add("fail", 1)
		return tr.Errorf("Security level check failed (level:%s)", secLevel), false
	}
	slcResults.Add("pass", 1)

	// c.Mail will add the <> for us when the address is empty.
	if from == "<>" {
		from = ""
	}
	if err = c.MailAndRcpt(from, to); err != nil {
		return tr.Errorf("MAIL+RCPT %v", err), smtp.IsPermanent(err)
	}

	w, err := c.Data()
	if err != nil {
		return tr.Errorf("DATA %v", err), smtp.IsPermanent(err)
	}
	_, err = w.Write(data)
	if err != nil {
		return tr.Errorf("DATA writing: %v", err), smtp.IsPermanent(err)
	}

	err = w.Close()
	if err != nil {
		return tr.Errorf("DATA closing %v", err), smtp.IsPermanent(err)
	}

	c.Quit()
	tr.Debugf("done")

	return nil, false
}

func lookupMX(tr *trace.Trace, domain string) (string, error) {
	if v, ok := fakeMX[domain]; ok {
		return v, nil
	}

	domain, err := idna.ToASCII(domain)
	if err != nil {
		return "", err
	}

	mxs, err := net.LookupMX(domain)
	if err == nil {
		if len(mxs) == 0 {
			tr.Debugf("domain %q has no MX, falling back to A", domain)
			return domain, nil
		}

		tr.Debugf("MX %s", mxs[0].Host)
		return mxs[0].Host, nil
	}

	// There was an error. It could be that the domain has no MX, in which
	// case we have to fall back to A, or a bigger problem.
	// Unfortunately, go's API doesn't let us easily distinguish between them.
	// For now, if the error is permanent, we assume it's because there was no
	// MX and fall back, otherwise we return.
	// TODO: Find a better way to do this.
	dnsErr, ok := err.(*net.DNSError)
	if !ok {
		tr.Debugf("MX lookup error: %v", err)
		return "", err
	} else if dnsErr.Temporary() {
		tr.Debugf("temporary DNS error: %v", dnsErr)
		return "", err
	}

	// Permanent error, we assume MX does not exist and fall back to A.
	tr.Debugf("failed to resolve MX for %s, falling back to A", domain)
	return domain, nil
}
