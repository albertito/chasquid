package smtp

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestIsPermanent(t *testing.T) {
	cases := []struct {
		err       error
		permanent bool
	}{
		{&textproto.Error{499, ""}, false},
		{&textproto.Error{500, ""}, true},
		{&textproto.Error{599, ""}, true},
		{&textproto.Error{600, ""}, false},
		{fmt.Errorf("something"), false},
	}
	for _, c := range cases {
		if p := IsPermanent(c.err); p != c.permanent {
			t.Errorf("%v: expected %v, got %v", c.err, c.permanent, p)
		}
	}
}

func TestIsASCII(t *testing.T) {
	cases := []struct {
		str   string
		ascii bool
	}{
		{"", true},
		{"<>", true},
		{"lalala", true},
		{"ñaca", false},
		{"año", false},
	}
	for _, c := range cases {
		if ascii := isASCII(c.str); ascii != c.ascii {
			t.Errorf("%q: expected %v, got %v", c.str, c.ascii, ascii)
		}
	}
}

func TestBasic(t *testing.T) {
	fake, client := fakeDialog(`> EHLO a_test
< 250-server replies your hello
< 250-SIZE 35651584
< 250-SMTPUTF8
< 250-8BITMIME
< 250 HELP
> MAIL FROM:<from@from> BODY=8BITMIME
< 250 MAIL FROM is fine
> RCPT TO:<to@to>
< 250 RCPT TO is fine
`)

	c := &Client{
		Client: &smtp.Client{Text: textproto.NewConn(fake)}}
	if err := c.Hello("a_test"); err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if err := c.MailAndRcpt("from@from", "to@to"); err != nil {
		t.Fatalf("MailAndRcpt failed: %v", err)
	}

	cmds := fake.Client()
	if client != cmds {
		t.Fatalf("Got:\n%s\nExpected:\n%s", cmds, client)
	}
}

func TestSMTPUTF8(t *testing.T) {
	fake, client := fakeDialog(`> EHLO araña
< 250-chasquid replies your hello
< 250-SIZE 35651584
< 250-SMTPUTF8
< 250-8BITMIME
< 250 HELP
> MAIL FROM:<año@ñudo> BODY=8BITMIME SMTPUTF8
< 250 MAIL FROM is fine
> RCPT TO:<ñaca@ñoño>
< 250 RCPT TO is fine
`)

	c := &Client{
		Client: &smtp.Client{Text: textproto.NewConn(fake)}}
	if err := c.Hello("araña"); err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if err := c.MailAndRcpt("año@ñudo", "ñaca@ñoño"); err != nil {
		t.Fatalf("MailAndRcpt failed: %v\nDialog: %s", err, fake.Client())
	}

	cmds := fake.Client()
	if client != cmds {
		t.Fatalf("Got:\n%s\nExpected:\n%s", cmds, client)
	}
}

func TestSMTPUTF8NotSupported(t *testing.T) {
	fake, client := fakeDialog(`> EHLO araña
< 250-chasquid replies your hello
< 250-SIZE 35651584
< 250-8BITMIME
< 250 HELP
`)

	c := &Client{
		Client: &smtp.Client{Text: textproto.NewConn(fake)}}
	if err := c.Hello("araña"); err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if err := c.MailAndRcpt("año@ñudo", "ñaca@ñoño"); err != nil {
		terr, ok := err.(*textproto.Error)
		if !ok || terr.Code != 599 {
			t.Fatalf("MailAndRcpt failed with unexpected error: %v\nDialog: %s",
				err, fake.Client())
		}
	}

	cmds := fake.Client()
	if client != cmds {
		t.Fatalf("Got:\n%s\nExpected:\n%s", cmds, client)
	}
}

func TestFallbackToIDNA(t *testing.T) {
	fake, client := fakeDialog(`> EHLO araña
< 250-chasquid replies your hello
< 250-SIZE 35651584
< 250-8BITMIME
< 250 HELP
> MAIL FROM:<gran@xn--udo-6ma> BODY=8BITMIME
< 250 MAIL FROM is fine
> RCPT TO:<alto@xn--oo-yjab>
< 250 RCPT TO is fine
`)

	c := &Client{
		Client: &smtp.Client{Text: textproto.NewConn(fake)}}
	if err := c.Hello("araña"); err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if err := c.MailAndRcpt("gran@ñudo", "alto@ñoño"); err != nil {
		terr, ok := err.(*textproto.Error)
		if !ok || terr.Code != 599 {
			t.Fatalf("MailAndRcpt failed with unexpected error: %v\nDialog: %s",
				err, fake.Client())
		}
	}

	cmds := fake.Client()
	if client != cmds {
		t.Fatalf("Got:\n%s\nExpected:\n%s", cmds, client)
	}
}

type faker struct {
	buf *bytes.Buffer
	*bufio.ReadWriter
}

func (f faker) Close() error                     { return nil }
func (f faker) LocalAddr() net.Addr              { return nil }
func (f faker) RemoteAddr() net.Addr             { return nil }
func (f faker) SetDeadline(time.Time) error      { return nil }
func (f faker) SetReadDeadline(time.Time) error  { return nil }
func (f faker) SetWriteDeadline(time.Time) error { return nil }
func (f faker) Client() string {
	f.ReadWriter.Writer.Flush()
	return f.buf.String()
}

// Takes a dialog, returns the corresponding faker and expected client
// messages.  Ideally we would check this interactively, and it's not that
// difficult, but this is good enough for now.
func fakeDialog(dialog string) (faker, string) {
	var client, server string

	for _, l := range strings.Split(dialog, "\n") {
		if strings.HasPrefix(l, "< ") {
			server += l[2:] + "\r\n"
		} else if strings.HasPrefix(l, "> ") {
			client += l[2:] + "\r\n"
		}
	}

	fake := faker{}
	fake.buf = &bytes.Buffer{}
	fake.ReadWriter = bufio.NewReadWriter(
		bufio.NewReader(strings.NewReader(server)), bufio.NewWriter(fake.buf))

	return fake, client
}
