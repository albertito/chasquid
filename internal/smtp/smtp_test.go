package smtp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
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
		{&textproto.Error{Code: 499, Msg: ""}, false},
		{&textproto.Error{Code: 500, Msg: ""}, true},
		{&textproto.Error{Code: 599, Msg: ""}, true},
		{&textproto.Error{Code: 600, Msg: ""}, false},
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

func mustNewClient(t *testing.T, nc net.Conn) *Client {
	t.Helper()

	c, err := NewClient(nc, "")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return c
}

func TestBasic(t *testing.T) {
	fake, client := fakeDialog(`< 220 welcome
> EHLO a_test
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

	c := mustNewClient(t, fake)
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
	fake, client := fakeDialog(`< 220 welcome
> EHLO araña
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

	c := mustNewClient(t, fake)
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
	fake, client := fakeDialog(`< 220 welcome
> EHLO araña
< 250-chasquid replies your hello
< 250-SIZE 35651584
< 250-8BITMIME
< 250 HELP
`)

	c := mustNewClient(t, fake)
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
	fake, client := fakeDialog(`< 220 welcome
> EHLO araña
< 250-chasquid replies your hello
< 250-SIZE 35651584
< 250-8BITMIME
< 250 HELP
> MAIL FROM:<gran@xn--udo-6ma> BODY=8BITMIME
< 250 MAIL FROM is fine
> RCPT TO:<alto@xn--oo-yjab>
< 250 RCPT TO is fine
`)

	c := mustNewClient(t, fake)
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

func TestLineTooLong(t *testing.T) {
	// Fake the server sending a >2MiB reply.
	dialog := `< 220 welcome
> EHLO araña
< 250 HELP
> NOOP
< 250 longreply:` + fmt.Sprintf("%2097152s", "x") + `:
> NOOP
< 250 ok
`

	fake, client := fakeDialog(dialog)

	c := mustNewClient(t, fake)
	if err := c.Hello("araña"); err != nil {
		t.Fatalf("Hello failed: %v", err)
	}

	if err := c.Noop(); err != nil {
		t.Errorf("Noop failed: %v", err)
	}

	if err := c.Noop(); err != io.EOF {
		t.Errorf("Expected EOF, got: %v", err)
	}

	cmds := fake.Client()
	if client != cmds {
		t.Errorf("Got:\n%s\nExpected:\n%s", cmds, client)
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

var _ net.Conn = faker{}

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
