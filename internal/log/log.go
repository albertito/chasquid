// Package log implements a simple logger.
//
// It implements an API somewhat similar to "github.com/google/glog" with a
// focus towards logging to stderr, which is useful for systemd-based
// environments.
//
// There are command line flags (defined using the flag package) to control
// the behaviour of the default logger. By default, it will write to stderr
// without timestamps; this is suitable for systemd (or equivalent) logging.
package log

import (
	"flag"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Flags that control the default logging.
var (
	vLevel = flag.Int("v", 0, "Verbosity level (1 = debug)")

	logFile = flag.String("logfile", "",
		"file to log to (enables logtime)")

	logToSyslog = flag.String("logtosyslog", "",
		"log to syslog, with the given tag")

	logTime = flag.Bool("logtime", false,
		"include the time when writing the log to stderr")

	alsoLogToStderr = flag.Bool("alsologtostderr", false,
		"also log to stderr, in addition to the file")
)

// Logging levels.
type Level int

const (
	Fatal = Level(-2)
	Error = Level(-1)
	Info  = Level(0)
	Debug = Level(1)
)

var levelToLetter = map[Level]string{
	Fatal: "☠",
	Error: "E",
	Info:  "_",
	Debug: ".",
}

// A Logger represents a logging object that writes logs to the given writer.
type Logger struct {
	Level   Level
	LogTime bool

	CallerSkip int

	w io.WriteCloser
	sync.Mutex
}

func New(w io.WriteCloser) *Logger {
	return &Logger{
		w:          w,
		CallerSkip: 0,
		Level:      Info,
		LogTime:    true,
	}
}

func NewFile(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	l := New(f)
	l.LogTime = true
	return l, nil
}

func NewSyslog(priority syslog.Priority, tag string) (*Logger, error) {
	w, err := syslog.New(priority, tag)
	if err != nil {
		return nil, err
	}

	l := New(w)
	l.LogTime = false
	return l, nil
}

func (l *Logger) Close() {
	l.w.Close()
}

func (l *Logger) V(level Level) bool {
	return level <= l.Level
}

func (l *Logger) Log(level Level, skip int, format string, a ...interface{}) {
	if !l.V(level) {
		return
	}

	// Message.
	msg := fmt.Sprintf(format, a...)

	// Caller.
	_, file, line, ok := runtime.Caller(1 + l.CallerSkip + skip)
	if !ok {
		file = "unknown"
	}
	fl := fmt.Sprintf("%s:%-4d", filepath.Base(file), line)
	if len(fl) > 18 {
		fl = fl[len(fl)-18:]
	}
	msg = fmt.Sprintf("%-18s", fl) + " " + msg

	// Level.
	letter, ok := levelToLetter[level]
	if !ok {
		letter = strconv.Itoa(int(level))
	}
	msg = letter + " " + msg

	// Time.
	if l.LogTime {
		msg = time.Now().Format("20060102 15:04:05.000000 ") + msg
	}

	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	l.Lock()
	l.w.Write([]byte(msg))
	l.Unlock()
}

func (l *Logger) Debugf(format string, a ...interface{}) {
	l.Log(Debug, 1, format, a...)
}

func (l *Logger) Infof(format string, a ...interface{}) {
	l.Log(Info, 1, format, a...)
}

func (l *Logger) Errorf(format string, a ...interface{}) error {
	l.Log(Error, 1, format, a...)
	return fmt.Errorf(format, a...)
}

func (l *Logger) Fatalf(format string, a ...interface{}) {
	l.Log(-2, 1, format, a...)
	// TODO: Log traceback?
	os.Exit(1)
}

// The default logger, used by the top-level functions below.
var Default = &Logger{
	w:          os.Stderr,
	CallerSkip: 1,
	Level:      Info,
	LogTime:    false,
}

// Init the default logger, based on the command-line flags.
// Must be called after flag.Parse().
func Init() {
	var err error

	if *logToSyslog != "" {
		Default, err = NewSyslog(syslog.LOG_DAEMON|syslog.LOG_INFO, *logToSyslog)
		if err != nil {
			panic(err)
		}
	} else if *logFile != "" {
		Default, err = NewFile(*logFile)
		if err != nil {
			panic(err)
		}
		*logTime = true
	}

	if *alsoLogToStderr && Default.w != os.Stderr {
		Default.w = multiWriteCloser(Default.w, os.Stderr)
	}

	Default.CallerSkip = 1
	Default.Level = Level(*vLevel)
	Default.LogTime = *logTime
}

func V(level Level) bool {
	return Default.V(level)
}

func Log(level Level, skip int, format string, a ...interface{}) {
	Default.Log(level, skip, format, a...)
}

func Debugf(format string, a ...interface{}) {
	Default.Debugf(format, a...)
}

func Infof(format string, a ...interface{}) {
	Default.Infof(format, a...)
}

func Errorf(format string, a ...interface{}) error {
	return Default.Errorf(format, a...)
}

func Fatalf(format string, a ...interface{}) {
	Default.Fatalf(format, a...)
}

// multiWriteCloser creates a WriteCloser that duplicates its writes and
// closes to all the provided writers.
func multiWriteCloser(wc ...io.WriteCloser) io.WriteCloser {
	return mwc(wc)
}

type mwc []io.WriteCloser

func (m mwc) Write(p []byte) (n int, err error) {
	for _, w := range m {
		if n, err = w.Write(p); err != nil {
			return
		}
	}
	return
}
func (m mwc) Close() error {
	for _, w := range m {
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}
