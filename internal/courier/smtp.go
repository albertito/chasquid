package courier

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"net"
	"time"

	"golang.org/x/net/idna"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/expvarom"
	"blitiri.com.ar/go/chasquid/internal/smtp"
	"blitiri.com.ar/go/chasquid/internal/sts"
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

	// Allow overriding of net.LookupMX for testing purposes.
	// TODO: replace this with proper lookup interception once it is supported
	// by Go.
	netLookupMX = net.LookupMX
)

// Exported variables.
var (
	tlsCount = expvarom.NewMap("chasquid/smtpOut/tlsCount",
		"result", "count of TLS status on outgoing connections")
	slcResults = expvarom.NewMap("chasquid/smtpOut/securityLevelChecks",
		"result", "count of security level checks on outgoing connections")

	stsSecurityModes = expvarom.NewMap("chasquid/smtpOut/sts/mode",
		"mode", "count of STS checks on outgoing connections")
	stsSecurityResults = expvarom.NewMap("chasquid/smtpOut/sts/security",
		"result", "count of STS security checks on outgoing connections")
)

// SMTP delivers remote mail via outgoing SMTP.
type SMTP struct {
	HelloDomain string
	Dinfo       *domaininfo.DB
	STSCache    *sts.PolicyCache
}

// Deliver an email. On failures, returns an error, and whether or not it is
// permanent.
func (s *SMTP) Deliver(from string, to string, data []byte) (error, bool) {
	a := &attempt{
		courier:  s,
		from:     from,
		to:       to,
		toDomain: envelope.DomainOf(to),
		data:     data,
		tr:       trace.New("Courier.SMTP", to),
	}
	defer a.tr.Finish()
	a.tr.Debugf("%s  ->  %s", from, to)

	// smtp.Client.Mail will add the <> for us when the address is empty.
	if a.from == "<>" {
		a.from = ""
	}

	mxs, err, perm := lookupMXs(a.tr, a.toDomain)
	if err != nil || len(mxs) == 0 {
		// Note this is considered a permanent error.
		// This is in line with what other servers (Exim) do. However, the
		// downside is that temporary DNS issues can affect delivery, so we
		// have to make sure we try hard enough on the lookup above.
		return a.tr.Errorf("Could not find mail server: %v", err), perm
	}

	a.stsPolicy = s.fetchSTSPolicy(a.tr, a.toDomain)

	for _, mx := range mxs {
		if a.stsPolicy != nil && !a.stsPolicy.MXIsAllowed(mx) {
			a.tr.Printf("%q skipped as per MTA-STA policy", mx)
			continue
		}

		var permanent bool
		err, permanent = a.deliver(mx)
		if err == nil {
			return nil, false
		}
		if permanent {
			return err, true
		}
		a.tr.Errorf("%q returned transient error: %v", mx, err)
	}

	// We exhausted all MXs failed to deliver, try again later.
	return a.tr.Errorf("all MXs returned transient failures (last: %v)", err), false
}

type attempt struct {
	courier *SMTP

	from string
	to   string
	data []byte

	toDomain string

	stsPolicy *sts.Policy

	tr *trace.Trace
}

func (a *attempt) deliver(mx string) (error, bool) {
	skipTLS := false
retry:
	conn, err := net.DialTimeout("tcp", mx+":"+*smtpPort, smtpDialTimeout)
	if err != nil {
		return a.tr.Errorf("Could not dial: %v", err), false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(smtpTotalTimeout))

	c, err := smtp.NewClient(conn, mx)
	if err != nil {
		return a.tr.Errorf("Error creating client: %v", err), false
	}

	if err = c.Hello(a.courier.HelloDomain); err != nil {
		return a.tr.Errorf("Error saying hello: %v", err), false
	}

	secLevel := domaininfo.SecLevel_PLAIN
	if ok, _ := c.Extension("STARTTLS"); ok && !skipTLS {
		config := &tls.Config{
			ServerName: mx,

			// Unfortunately, many servers use self-signed and invalid
			// certificates. So we use a custom verification (identical to
			// Go's) to distinguish between invalid and valid certificates.
			// That information is used to track the security level, to
			// prevent downgrade attacks.
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				secLevel = a.verifyConnection(cs)
				return nil
			},
		}

		err = c.StartTLS(config)
		if err != nil {
			// If we could not complete a jump to TLS (either because the
			// STARTTLS command itself failed server-side, or because we got a
			// TLS negotiation error), retry but without trying to use TLS.
			// This should be quite rare, but it can happen if the server
			// certificate is not parseable by the Go library, or if it has a
			// broken TLS stack.
			// Note that invalid and self-signed certs do NOT fall in this
			// category, those are handled by the VerifyConnection function
			// above, and don't need a retry. This is only needed for lower
			// level errors.
			tlsCount.Add("tls:failed", 1)
			a.tr.Errorf("TLS error, retrying without TLS: %v", err)
			skipTLS = true
			conn.Close()
			goto retry
		}
	} else {
		tlsCount.Add("plain", 1)
		a.tr.Debugf("Insecure - NOT using TLS")
	}

	if !a.courier.Dinfo.OutgoingSecLevel(a.tr, a.toDomain, secLevel) {
		// We consider the failure transient, so transient misconfigurations
		// do not affect deliveries.
		slcResults.Add("fail", 1)
		return a.tr.Errorf("Security level check failed (level:%s)", secLevel), false
	}
	slcResults.Add("pass", 1)

	if a.stsPolicy != nil && a.stsPolicy.Mode == sts.Enforce {
		// The connection MUST be validated by TLS.
		// https://tools.ietf.org/html/rfc8461#section-4.2
		if secLevel != domaininfo.SecLevel_TLS_SECURE {
			stsSecurityResults.Add("fail", 1)
			return a.tr.Errorf("invalid security level (%v) for STS policy",
				secLevel), false
		}
		stsSecurityResults.Add("pass", 1)
		a.tr.Debugf("STS policy: connection is using valid TLS")
	}

	if err = c.MailAndRcpt(a.from, a.to); err != nil {
		return a.tr.Errorf("MAIL+RCPT %v", err), smtp.IsPermanent(err)
	}

	w, err := c.Data()
	if err != nil {
		return a.tr.Errorf("DATA %v", err), smtp.IsPermanent(err)
	}
	_, err = w.Write(a.data)
	if err != nil {
		return a.tr.Errorf("DATA writing: %v", err), smtp.IsPermanent(err)
	}

	err = w.Close()
	if err != nil {
		return a.tr.Errorf("DATA closing %v", err), smtp.IsPermanent(err)
	}

	_ = c.Quit()
	a.tr.Debugf("done")

	return nil, false
}

// CA roots to validate against, so we can override it for testing.
var certRoots *x509.CertPool = nil

func (a *attempt) verifyConnection(cs tls.ConnectionState) domaininfo.SecLevel {
	// Validate certificates, using the same logic Go does, and following the
	// official example at
	// https://pkg.go.dev/crypto/tls#example-Config-VerifyConnection.
	opts := x509.VerifyOptions{
		DNSName:       cs.ServerName,
		Intermediates: x509.NewCertPool(),
		Roots:         certRoots,
	}
	for _, cert := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(cert)
	}
	_, err := cs.PeerCertificates[0].Verify(opts)

	if err != nil {
		// Invalid TLS cert, since it could not be verified.
		a.tr.Debugf("Insecure - using TLS, but with an invalid cert")
		tlsCount.Add("tls:insecure", 1)
		return domaininfo.SecLevel_TLS_INSECURE
	} else {
		tlsCount.Add("tls:secure", 1)
		a.tr.Debugf("Secure - using TLS")
		return domaininfo.SecLevel_TLS_SECURE
	}
}

func (s *SMTP) fetchSTSPolicy(tr *trace.Trace, domain string) *sts.Policy {
	if s.STSCache == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	policy, err := s.STSCache.Fetch(ctx, domain)
	if err != nil {
		return nil
	}

	tr.Debugf("got STS policy")
	stsSecurityModes.Add(string(policy.Mode), 1)

	return policy
}

func lookupMXs(tr *trace.Trace, domain string) ([]string, error, bool) {
	domain, err := idna.ToASCII(domain)
	if err != nil {
		return nil, err, true
	}

	mxs := []string{}

	mxRecords, err := netLookupMX(domain)
	if err != nil {
		// There was an error. It could be that the domain has no MX, in which
		// case we have to fall back to A, or a bigger problem.
		dnsErr, ok := err.(*net.DNSError)
		if !ok {
			tr.Debugf("Error resolving MX on %q: %v", domain, err)
			return nil, err, false
		} else if dnsErr.IsNotFound {
			// MX not found, fall back to A.
			tr.Debugf("MX for %s not found, falling back to A", domain)
			mxs = []string{domain}
		} else {
			tr.Debugf("MX lookup error on %q: %v", domain, dnsErr)
			return nil, err, !dnsErr.Temporary()
		}

	} else {
		// Convert the DNS records to a plain string slice. They're already
		// sorted by priority.
		for _, r := range mxRecords {
			mxs = append(mxs, r.Host)
		}
	}

	// Note that mxs could be empty; in that case we do NOT fall back to A.
	// This case is explicitly covered by the SMTP RFC.
	// https://tools.ietf.org/html/rfc5321#section-5.1

	// Cap the list of MXs to 5 hosts, to keep delivery attempt times
	// sane and prevent abuse.
	if len(mxs) > 5 {
		mxs = mxs[:5]
	}

	tr.Debugf("MXs: %v", mxs)
	return mxs, nil, true
}
