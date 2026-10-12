package trace

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/nettrace"
	"blitiri.com.ar/go/log"
)

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// setupLog replaces the default logger with one that writes to the returned
// buffer, at the given level. It is restored when the test finishes.
func setupLog(t *testing.T, level log.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := log.Default
	log.Default = log.New(nopCloser{buf})
	log.Default.Level = level
	log.Default.LogTime = false
	log.Default.LogCaller = false
	t.Cleanup(func() { log.Default = orig })
	return buf
}

// traceEvents returns the rendered (HTML) active traces for the family.
func traceEvents(t *testing.T, family string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/debug/traces?fam="+family+"&b=-1", nil)
	w := httptest.NewRecorder()
	nettrace.RenderTraces(w, req)
	return w.Body.String()
}

// countingStringer counts how many times it was formatted.
type countingStringer struct{ n *int }

func (c countingStringer) String() string {
	*c.n++
	return "stringer"
}

func TestFormatOnce(t *testing.T) {
	for _, level := range []log.Level{log.Info, log.Debug} {
		setupLog(t, level)
		tr := New("TestFormatOnce", "title")

		n := 0
		tr.Debugf("%v", countingStringer{&n})
		if n != 1 {
			t.Errorf("level %d: Debugf formatted %d times, expected 1",
				level, n)
		}

		n = 0
		tr.Printf("%v", countingStringer{&n})
		if n != 1 {
			t.Errorf("level %d: Printf formatted %d times, expected 1",
				level, n)
		}

		tr.Finish()
	}
}

func TestLog(t *testing.T) {
	// At info level, debug messages are not logged.
	buf := setupLog(t, log.Info)
	tr := New("TestLog", "title")
	tr.Printf("info %d", 1)
	tr.Debugf("debug %d", 2)
	tr.Errorf("error %d", 3)
	tr.Error(errors.New("error 4"))
	tr.Finish()

	expected := "_ TestLog title: info 1\n" +
		"_ TestLog title: error: error 3\n" +
		"_ TestLog title: error: error 4\n"
	if got := buf.String(); got != expected {
		t.Errorf("unexpected log at info level:\n got: %q\nwant: %q",
			got, expected)
	}

	// At debug level, they are. Messages are quoted.
	buf = setupLog(t, log.Debug)
	tr = New("TestLog", "title")
	tr.Debugf("debug %q\n", "x")
	tr.Finish()

	expected = `. TestLog title: debug \"x\"\n` + "\n"
	if got := buf.String(); got != expected {
		t.Errorf("unexpected log at debug level:\n got: %q\nwant: %q",
			got, expected)
	}
}

func TestTruncate(t *testing.T) {
	rep := strings.Repeat
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "short"},
		{rep("a", maxMsgLen), rep("a", maxMsgLen)},
		{rep("a", maxMsgLen+1),
			rep("a", maxMsgLen) + "… [truncated, 8193 bytes total]"},

		// The limit is in characters, not bytes ("ñ" is 2 bytes).
		{rep("ñ", maxMsgLen), rep("ñ", maxMsgLen)},
		{rep("ñ", maxMsgLen+1),
			rep("ñ", maxMsgLen) + "… [truncated, 16386 bytes total]"},

		// Invalid UTF-8 bytes count as one character each, and are kept.
		{rep("\xff", maxMsgLen+1),
			rep("\xff", maxMsgLen) + "… [truncated, 8193 bytes total]"},
	}
	for i, c := range cases {
		if got := truncate(c.in); got != c.want {
			t.Errorf("%d: truncate(%d bytes) = %q (%d bytes), want %q",
				i, len(c.in), got[max(0, len(got)-40):], len(got),
				c.want[max(0, len(c.want)-40):])
		}
	}
}

func TestTruncateMessages(t *testing.T) {
	buf := setupLog(t, log.Debug)
	tr := New("TestTruncateMessages", "title")

	long := strings.Repeat("x", 3*maxMsgLen)
	tr.Printf("printf %s", long)
	tr.Debugf("debugf %s", long)
	err := tr.Errorf("errorf %s", long)
	tr.Error(errors.New("error " + long))

	// The error returned by Errorf must not be truncated.
	if len(err.Error()) != len("errorf ")+len(long) {
		t.Errorf("Errorf returned a truncated error (%d bytes)",
			len(err.Error()))
	}

	// All messages are truncated in the log...
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 log lines, got %d", len(lines))
	}
	for _, l := range lines {
		if len(l) > maxMsgLen+100 ||
			!strings.HasSuffix(l, " bytes total]") {
			t.Errorf("log line not truncated (%d bytes): ...%s",
				len(l), l[max(0, len(l)-60):])
		}
	}

	// ... and in the trace.
	body := traceEvents(t, "TestTruncateMessages")
	if n := strings.Count(body, " bytes total]"); n != 4 {
		t.Errorf("expected 4 truncated events in the trace, got %d", n)
	}
	if len(body) > 4*maxMsgLen+10*1024 {
		t.Errorf("trace too large (%d bytes), events not truncated?",
			len(body))
	}
	tr.Finish()
}

func TestTraceEvents(t *testing.T) {
	// Debug messages are added to the trace even if they are not logged.
	setupLog(t, log.Info)
	tr := New("TestTraceEvents", "title")
	tr.Printf("info %d", 1)
	tr.Debugf("debug %d", 2)
	tr.Errorf("error %d", 3)

	body := traceEvents(t, "TestTraceEvents")
	for _, s := range []string{"info 1", "debug 2", "error: error 3"} {
		if !strings.Contains(body, s) {
			t.Errorf("trace does not contain %q:\n%s", s, body)
		}
	}
	tr.Finish()
}
