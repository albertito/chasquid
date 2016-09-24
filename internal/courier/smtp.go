package courier

import (
	"crypto/tls"
	"flag"
	"net"
	"net/smtp"
	"time"

	"github.com/golang/glog"

	"blitiri.com.ar/go/chasquid/internal/envelope"
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

	// Bypass the MX lookup, for testing purposes.
	bypassMX = flag.Bool("testing__bypass_mx_lookup", false,
		"bypass MX lookup, ONLY FOR TESTING")

	// Fake MX records, used for testing only.
	fakeMX = map[string]string{}
)

// SMTP delivers remote mail via outgoing SMTP.
type SMTP struct {
}

func (s *SMTP) Deliver(from string, to string, data []byte) (error, bool) {
	tr := trace.New("goingSMTP", "Deliver")
	defer tr.Finish()
	tr.LazyPrintf("%s  ->  %s", from, to)

	// TODO: Fall back to A if MX is not available.
	mx, err := lookupMX(envelope.DomainOf(to))
	if err != nil {
		// Note this is considered a permanent error.
		// This is in line with what other servers (Exim) do. However, the
		// downside is that temporary DNS issues can affect delivery, so we
		// have to make sure we try hard enough on the lookup above.
		return tr.Errorf("Could not find mail server: %v", err), true
	}
	tr.LazyPrintf("MX: %s", mx)

	// Do we use insecure TLS?
	// Set as fallback when retrying.
	insecure := false

retry:
	conn, err := net.DialTimeout("tcp", mx+":"+*smtpPort, smtpDialTimeout)
	if err != nil {
		return tr.Errorf("Could not dial: %v", err), false
	}
	conn.SetDeadline(time.Now().Add(smtpTotalTimeout))

	c, err := smtp.NewClient(conn, mx)
	if err != nil {
		return tr.Errorf("Error creating client: %v", err), false
	}

	// TODO: Keep track of hosts and MXs that we've successfully done TLS
	// against, and enforce it.
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
				return tr.Errorf("TLS error: %v", err), false
			}

			insecure = true
			tr.LazyPrintf("TLS error, retrying insecurely")
			goto retry
		}

		if config.InsecureSkipVerify {
			tr.LazyPrintf("Insecure - self-signed certificate")
		} else {
			tr.LazyPrintf("Secure - using TLS")
		}
	} else {
		tr.LazyPrintf("Insecure - not using TLS")
	}

	// TODO: check if the errors we get back are transient or not.
	// Go's smtp does not allow us to do this, so leave for when we do it
	// ourselves.

	if err = c.Mail(from); err != nil {
		return tr.Errorf("MAIL %v", err), false
	}

	if err = c.Rcpt(to); err != nil {
		return tr.Errorf("RCPT TO %v", err), false
	}

	w, err := c.Data()
	if err != nil {
		return tr.Errorf("DATA %v", err), false
	}
	_, err = w.Write(data)
	if err != nil {
		return tr.Errorf("DATA writing: %v", err), false
	}

	err = w.Close()
	if err != nil {
		return tr.Errorf("DATA closing %v", err), false
	}

	c.Quit()

	return nil, false
}

func lookupMX(domain string) (string, error) {
	if v, ok := fakeMX[domain]; ok {
		return v, nil
	}

	if *bypassMX {
		return domain, nil
	}

	mxs, err := net.LookupMX(domain)
	if err != nil {
		return "", err
	} else if len(mxs) == 0 {
		glog.Infof("domain %q has no MX, falling back to A", domain)
		return domain, nil
	}

	return mxs[0].Host, nil
}
