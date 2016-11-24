package log

import (
	"io/ioutil"
	"os"
	"regexp"
	"testing"
)

func mustNewFile(t *testing.T) (string, *Logger) {
	f, err := ioutil.TempFile("", "log_test-")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	l, err := NewFile(f.Name())
	if err != nil {
		t.Fatalf("failed to open new log file: %v", err)
	}

	return f.Name(), l
}

func checkContentsMatch(t *testing.T, name, path, expected string) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	got := string(content)
	if !regexp.MustCompile(expected).Match(content) {
		t.Errorf("%s: regexp %q did not match %q",
			name, expected, got)
	}
}

func testLogger(t *testing.T, fname string, l *Logger) {
	l.LogTime = false
	l.Infof("message %d", 1)
	checkContentsMatch(t, "info-no-time", fname,
		"^_ log_test.go:....   message 1\n")

	os.Truncate(fname, 0)
	l.Infof("message %d\n", 1)
	checkContentsMatch(t, "info-with-newline", fname,
		"^_ log_test.go:....   message 1\n")

	os.Truncate(fname, 0)
	l.LogTime = true
	l.Infof("message %d", 1)
	checkContentsMatch(t, "info-with-time", fname,
		`^\d{8} ..:..:..\.\d{6} _ log_test.go:....   message 1\n`)

	os.Truncate(fname, 0)
	l.LogTime = false
	l.Errorf("error %d", 1)
	checkContentsMatch(t, "error", fname, `^E log_test.go:....   error 1\n`)

	if l.V(Debug) {
		t.Fatalf("Debug level enabled by default (level: %v)", l.Level)
	}

	os.Truncate(fname, 0)
	l.LogTime = false
	l.Debugf("debug %d", 1)
	checkContentsMatch(t, "debug-no-log", fname, `^$`)

	os.Truncate(fname, 0)
	l.Level = Debug
	l.Debugf("debug %d", 1)
	checkContentsMatch(t, "debug", fname, `^\. log_test.go:....   debug 1\n`)

	if !l.V(Debug) {
		t.Errorf("l.Level = Debug, but V(Debug) = false")
	}

	os.Truncate(fname, 0)
	l.Level = Info
	l.Log(Debug, 0, "log debug %d", 1)
	l.Log(Info, 0, "log info %d", 1)
	checkContentsMatch(t, "log", fname,
		`^_ log_test.go:....   log info 1\n`)

	os.Truncate(fname, 0)
	l.Level = Info
	l.Log(Fatal, 0, "log fatal %d", 1)
	checkContentsMatch(t, "log", fname,
		`^☠ log_test.go:....   log fatal 1\n`)
}

func TestBasic(t *testing.T) {
	fname, l := mustNewFile(t)
	defer l.Close()
	defer os.Remove(fname)

	testLogger(t, fname, l)
}

func TestDefaultFile(t *testing.T) {
	fname, l := mustNewFile(t)
	l.Close()
	defer os.Remove(fname)

	*logFile = fname

	Init()

	testLogger(t, fname, Default)
}
