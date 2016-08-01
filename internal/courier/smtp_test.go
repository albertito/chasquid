package courier

import (
	"bufio"
	"net"
	"net/textproto"
	"testing"
	"time"
)

// Fake server, to test SMTP out.
func fakeServer(t *testing.T, responses map[string]string) string {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("fake server listen: %v", err)
	}

	go func() {
		defer l.Close()

		c, err := l.Accept()
		if err != nil {
			t.Fatalf("fake server accept: %v", err)
		}
		defer c.Close()

		t.Logf("fakeServer got connection")

		r := textproto.NewReader(bufio.NewReader(c))
		c.Write([]byte(responses["_welcome"]))
		for {
			line, err := r.ReadLine()
			if err != nil {
				t.Logf("fakeServer exiting: %v\n", err)
				return
			}

			t.Logf("fakeServer read: %q\n", line)
			c.Write([]byte(responses[line]))

			if line == "DATA" {
				_, err = r.ReadDotBytes()
				if err != nil {
					t.Logf("fakeServer exiting: %v\n", err)
					return
				}
				c.Write([]byte(responses["_DATA"]))
			}
		}
	}()

	return l.Addr().String()
}

func TestSMTP(t *testing.T) {
	// Shorten the total timeout, so the test fails quickly if the protocol
	// gets stuck.
	smtpTotalTimeout = 5 * time.Second

	responses := map[string]string{
		"_welcome":          "220 welcome\n",
		"EHLO localhost":    "250 ehlo ok\n",
		"MAIL FROM:<me@me>": "250 mail ok\n",
		"RCPT TO:<to@to>":   "250 rcpt ok\n",
		"DATA":              "354 send data\n",
		"_DATA":             "250 data ok\n",
		"QUIT":              "250 quit ok\n",
	}
	addr := fakeServer(t, responses)
	host, port, _ := net.SplitHostPort(addr)

	fakeMX["to"] = host
	smtpPort = port

	s := &SMTP{}
	err := s.Deliver("me@me", "to@to", []byte("data"))
	if err != nil {
		t.Errorf("deliver failed: %v", err)
	}
}

func TestSMTPErrors(t *testing.T) {
	// Shorten the total timeout, so the test fails quickly if the protocol
	// gets stuck.
	smtpTotalTimeout = 1 * time.Second

	responses := []map[string]string{
		// First test: hang response, should fail due to timeout.
		{
			"_welcome": "220 no newline",
		},

		// MAIL FROM not allowed.
		{
			"_welcome":          "220 mail from not allowed\n",
			"EHLO localhost":    "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "501 mail error\n",
		},

		// RCPT TO not allowed.
		{
			"_welcome":          "220 rcpt to not allowed\n",
			"EHLO localhost":    "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "501 rcpt error\n",
		},

		// DATA error.
		{
			"_welcome":          "220 data error\n",
			"EHLO localhost":    "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "554 data error\n",
		},

		// DATA response error.
		{
			"_welcome":          "220 data response error\n",
			"EHLO localhost":    "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "354 send data\n",
			"_DATA":             "551 data response error\n",
		},
	}

	for _, rs := range responses {
		addr := fakeServer(t, rs)
		host, port, _ := net.SplitHostPort(addr)

		fakeMX["to"] = host
		smtpPort = port

		s := &SMTP{}
		err := s.Deliver("me@me", "to@to", []byte("data"))
		if err == nil {
			t.Errorf("deliver not failed in case %q: %v", rs["_welcome"], err)
		}
		t.Logf("failed as expected: %v", err)
	}
}

// TODO: Test STARTTLS negotiation.
