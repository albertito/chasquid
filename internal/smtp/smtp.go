// Package smtp implements the Simple Mail Transfer Protocol as defined in RFC
// 5321.  It extends net/smtp as follows:
//
//  - Supports SMTPUTF8, via MailAndRcpt.
//  - Adds IsPermanent.
//
package smtp

import (
	"bufio"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"unicode"

	"blitiri.com.ar/go/chasquid/internal/envelope"

	"golang.org/x/net/idna"
)

// A Client represents a client connection to an SMTP server.
type Client struct {
	*smtp.Client
}

// NewClient uses the given connection to create a new Client.
func NewClient(conn net.Conn, host string) (*Client, error) {
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return nil, err
	}

	// Wrap the textproto.Conn reader so we are not exposed to a memory
	// exhaustion DoS on very long replies from the server.
	// Limit to 2 MiB total (all replies through the lifetime of the client),
	// which should be plenty for our uses of SMTP.
	lr := &io.LimitedReader{R: c.Text.Reader.R, N: 2 * 1024 * 1024}
	c.Text.Reader.R = bufio.NewReader(lr)

	return &Client{c}, nil
}

// cmd sends a command and returns the response over the text connection.
// Based on Go's method of the same name.
func (c *Client) cmd(expectCode int, format string, args ...interface{}) (int, string, error) {
	id, err := c.Text.Cmd(format, args...)
	if err != nil {
		return 0, "", err
	}
	c.Text.StartResponse(id)
	defer c.Text.EndResponse(id)

	return c.Text.ReadResponse(expectCode)
}

// MailAndRcpt issues MAIL FROM and RCPT TO commands, in sequence.
// It will check the addresses, decide if SMTPUTF8 is needed, and apply the
// necessary transformations.
func (c *Client) MailAndRcpt(from string, to string) error {
	from, fromNeeds, err := c.prepareForSMTPUTF8(from)
	if err != nil {
		return err
	}

	to, toNeeds, err := c.prepareForSMTPUTF8(to)
	if err != nil {
		return err
	}
	smtputf8Needed := fromNeeds || toNeeds

	cmdStr := "MAIL FROM:<%s>"
	if ok, _ := c.Extension("8BITMIME"); ok {
		cmdStr += " BODY=8BITMIME"
	}
	if smtputf8Needed {
		cmdStr += " SMTPUTF8"
	}
	_, _, err = c.cmd(250, cmdStr, from)
	if err != nil {
		return err
	}

	_, _, err = c.cmd(25, "RCPT TO:<%s>", to)
	return err
}

// prepareForSMTPUTF8 prepares the address for SMTPUTF8.
// It returns:
//  - The address to use. It is based on addr, and possibly modified to make
//    it not need the extension, if the server does not support it.
//  - Whether the address needs the extension or not.
//  - An error if the address needs the extension, but the client does not
//    support it.
func (c *Client) prepareForSMTPUTF8(addr string) (string, bool, error) {
	// ASCII address pass through.
	if isASCII(addr) {
		return addr, false, nil
	}

	// Non-ASCII address also pass through if the server supports the
	// extension.
	// Note there's a chance the server wants the domain in IDNA anyway, but
	// it could also require it to be UTF8. We assume that if it supports
	// SMTPUTF8 then it knows what its doing.
	if ok, _ := c.Extension("SMTPUTF8"); ok {
		return addr, true, nil
	}

	// Something is not ASCII, and the server does not support SMTPUTF8:
	//  - If it's the local part, there's no way out and is required.
	//  - If it's the domain, use IDNA.
	user, domain := envelope.Split(addr)

	if !isASCII(user) {
		return addr, true, &textproto.Error{Code: 599,
			Msg: "local part is not ASCII but server does not support SMTPUTF8"}
	}

	// If it's only the domain, convert to IDNA and move on.
	domain, err := idna.ToASCII(domain)
	if err != nil {
		// The domain is not IDNA compliant, which is odd.
		// Fail with a permanent error, not ideal but this should not
		// happen.
		return addr, true, &textproto.Error{
			Code: 599, Msg: "non-ASCII domain is not IDNA safe"}
	}

	return user + "@" + domain, false, nil
}

// isASCII returns true if all the characters in s are ASCII, false otherwise.
func isASCII(s string) bool {
	for _, c := range s {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// IsPermanent returns true if the error is permanent, and false otherwise.
// If it can't tell, it returns false.
func IsPermanent(err error) bool {
	terr, ok := err.(*textproto.Error)
	if !ok {
		return false
	}

	// Error codes 5yz are permanent.
	// https://tools.ietf.org/html/rfc5321#section-4.2.1
	if terr.Code >= 500 && terr.Code < 600 {
		return true
	}

	return false
}
