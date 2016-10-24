package maillog

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
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
