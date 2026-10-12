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
