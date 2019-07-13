package maillog

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/log"
)

var netAddr = &net.TCPAddr{
	IP:   net.ParseIP("1.2.3.4"),
	Port: 4321,
}

func expect(t *testing.T, buf *bytes.Buffer, s string) {
	if strings.Contains(buf.String(), s) {
		return
	}
	t.Errorf("buffer mismatch:")
	t.Errorf("  expected to contain: %q", s)
	t.Errorf("  got: %q", buf.String())
}

func TestLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(buf)

	l.Listening("1.2.3.4:4321")
	expect(t, buf, "daemon listening on 1.2.3.4:4321")
	buf.Reset()

	l.Auth(netAddr, "user@domain", false)
	expect(t, buf, "1.2.3.4:4321 auth failed for user@domain")
	buf.Reset()

	l.Auth(netAddr, "user@domain", true)
	expect(t, buf, "1.2.3.4:4321 auth succeeded for user@domain")
	buf.Reset()

	l.Rejected(netAddr, "from", []string{"to1", "to2"}, "error")
	expect(t, buf, "1.2.3.4:4321 rejected from=from to=[to1 to2] - error")
	buf.Reset()

	l.Queued(netAddr, "from", []string{"to1", "to2"}, "qid")
	expect(t, buf, "qid from=from queued ip=1.2.3.4:4321 to=[to1 to2]")
	buf.Reset()

	l.SendAttempt("qid", "from", "to", nil, false)
	expect(t, buf, "qid from=from to=to sent")
	buf.Reset()

	l.SendAttempt("qid", "from", "to", fmt.Errorf("error"), false)
	expect(t, buf, "qid from=from to=to failed (temporary): error")
	buf.Reset()

	l.SendAttempt("qid", "from", "to", fmt.Errorf("error"), true)
	expect(t, buf, "qid from=from to=to failed (permanent): error")
	buf.Reset()

	l.QueueLoop("qid", "from", 17*time.Second)
	expect(t, buf, "qid from=from completed loop, next in 17s")
	buf.Reset()

	l.QueueLoop("qid", "from", 0)
	expect(t, buf, "qid from=from all done")
	buf.Reset()
}

// Test that the default actions go reasonably to the default logger.
// Unfortunately this is almost the same as TestLogger.
func TestDefault(t *testing.T) {
	buf := &bytes.Buffer{}
	Default = New(buf)

	Listening("1.2.3.4:4321")
	expect(t, buf, "daemon listening on 1.2.3.4:4321")
	buf.Reset()

	Auth(netAddr, "user@domain", false)
	expect(t, buf, "1.2.3.4:4321 auth failed for user@domain")
	buf.Reset()

	Auth(netAddr, "user@domain", true)
	expect(t, buf, "1.2.3.4:4321 auth succeeded for user@domain")
	buf.Reset()

	Rejected(netAddr, "from", []string{"to1", "to2"}, "error")
	expect(t, buf, "1.2.3.4:4321 rejected from=from to=[to1 to2] - error")
	buf.Reset()

	Queued(netAddr, "from", []string{"to1", "to2"}, "qid")
	expect(t, buf, "qid from=from queued ip=1.2.3.4:4321 to=[to1 to2]")
	buf.Reset()

	SendAttempt("qid", "from", "to", nil, false)
	expect(t, buf, "qid from=from to=to sent")
	buf.Reset()

	SendAttempt("qid", "from", "to", fmt.Errorf("error"), false)
	expect(t, buf, "qid from=from to=to failed (temporary): error")
	buf.Reset()

	SendAttempt("qid", "from", "to", fmt.Errorf("error"), true)
	expect(t, buf, "qid from=from to=to failed (permanent): error")
	buf.Reset()

	QueueLoop("qid", "from", 17*time.Second)
	expect(t, buf, "qid from=from completed loop, next in 17s")
	buf.Reset()

	QueueLoop("qid", "from", 0)
	expect(t, buf, "qid from=from all done")
	buf.Reset()
}

// io.Writer that fails all write operations, for testing.
type failedWriter struct{}

func (w *failedWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("test error")
}

// nopCloser adds a Close method to an io.Writer, to turn it into a
// io.WriteCloser. This is the equivalent of ioutil.NopCloser but for
// io.Writer.
type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

// Test that we complain (only once) when we can't log.
func TestFailedLogger(t *testing.T) {
	// Set up a test logger, that will write to a buffer for us to check.
	buf := &bytes.Buffer{}
	log.Default = log.New(nopCloser{io.Writer(buf)})

	// Set up a maillog that will use a writer which always fail, to trigger
	// the condition.
	failedw := &failedWriter{}
	l := New(failedw)

	// Log something, which should fail. Then verify that the error message
	// appears in the log.
	l.printf("123 testing")
	s := buf.String()
	if !strings.Contains(s, "failed to write to maillog: test error") {
		t.Errorf("log did not contain expected message. Log: %#v", s)
	}

	// Further attempts should not generate any other errors.
	buf.Reset()
	l.printf("123 testing")
	s = buf.String()
	if s != "" {
		t.Errorf("expected second attempt to not log, but log had: %#v", s)
	}
}
