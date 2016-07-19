package courier

import (
	"crypto/tls"
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
	smtpPort = "25"

	// Fake MX records, used for testing only.
	fakeMX = map[string]string{}
)

// SMTP delivers remote mail via outgoing SMTP.
type SMTP struct {
}

func (s *SMTP) Deliver(from string, to string, data []byte) error {
	tr := trace.New("goingSMTP", "Deliver")
	defer tr.Finish()
	tr.LazyPrintf("%s  ->  %s", from, to)

	mx, err := lookupMX(envelope.DomainOf(to))
	if err != nil {
		return tr.Errorf("Could not find mail server: %v", err)
	}
	tr.LazyPrintf("MX: %s", mx)

	// Do we use insecure TLS?
	// Set as fallback when retrying.
	insecure := false

retry:
	conn, err := net.DialTimeout("tcp", mx+":"+smtpPort, smtpDialTimeout)
	if err != nil {
		return tr.Errorf("Could not dial: %v", err)
	}
	conn.SetDeadline(time.Now().Add(smtpTotalTimeout))

	c, err := smtp.NewClient(conn, mx)
	if err != nil {
		return tr.Errorf("Error creating client: %v", err)
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
				return tr.Errorf("TLS error: %v", err)
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

	if err = c.Mail(from); err != nil {
		return tr.Errorf("MAIL %v", err)
	}

	if err = c.Rcpt(to); err != nil {
		return tr.Errorf("RCPT TO %v", err)
	}

	w, err := c.Data()
	if err != nil {
		return tr.Errorf("DATA %v", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return tr.Errorf("DATA writing: %v", err)
	}

	err = w.Close()
	if err != nil {
		return tr.Errorf("DATA closing %v", err)
	}

	c.Quit()

	return nil
}

func lookupMX(domain string) (string, error) {
	if v, ok := fakeMX[domain]; ok {
		return v, nil
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
