// Package maillog implements a log specifically for email.
package maillog

import (
	"fmt"
	"io"
	"io/ioutil"
	"log/syslog"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"

	"blitiri.com.ar/go/chasquid/internal/trace"
)

// Global event logs.
var (
	authLog = trace.NewEventLog("Authentication", "Incoming SMTP")
)

// A writer that prepends timing information.
type timedWriter struct {
	w io.Writer
}

func (t timedWriter) Write(b []byte) (int, error) {
	fmt.Fprintf(t.w, "%s  ", time.Now().Format("2006-01-02 15:04:05.000000"))
	return t.w.Write(b)
}

type Logger struct {
	w    io.Writer
	once sync.Once
}

func New(w io.Writer) *Logger {
	return &Logger{w: timedWriter{w}}
}

func NewSyslog() (*Logger, error) {
	w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_MAIL, "chasquid")
	if err != nil {
		return nil, err
	}

	l := &Logger{w: w}
	return l, nil
}

func (l *Logger) printf(format string, args ...interface{}) {
	_, err := fmt.Fprintf(l.w, format, args...)
	if err != nil {
		l.once.Do(func() {
			glog.Errorf("failed to write to maillog: %v", err)
			glog.Error("(will not report this again)")
		})
	}
}

func (l *Logger) Listening(a string) {
	l.printf("daemon listening on %s\n", a)
}

func (l *Logger) Auth(netAddr net.Addr, user string, successful bool) {
	res := "successful"
	if !successful {
		res = "failed"
	}
	msg := fmt.Sprintf("%s authentication %s for %s\n", netAddr, res, user)
	l.printf(msg)
	authLog.Debugf(msg)
}

func (l *Logger) Rejected(netAddr net.Addr, from string, to []string, err string) {
	if from != "" {
		from = fmt.Sprintf(" from=%s", from)
	}
	toStr := ""
	if len(to) > 0 {
		toStr = fmt.Sprintf(" to=%v", to)
	}
	l.printf("%s rejected%s%s - %v\n", netAddr, from, toStr, err)
}

func (l *Logger) Queued(netAddr net.Addr, from string, to []string, id string) {
	l.printf("%s from=%s queued ip=%s to=%v\n", id, from, netAddr, to)
}

func (l *Logger) SendAttempt(id, from, to string, err error, permanent bool) {
	if err == nil {
		l.printf("%s from=%s to=%s sent successfully\n", id, from, to)
	} else {
		t := "(temporary)"
		if permanent {
			t = "(permanent)"
		}
		l.printf("%s from=%s to=%s sent failed %s: %v\n", id, from, to, t, err)
	}
}

func (l *Logger) QueueLoop(id string, nextDelay time.Duration) {
	if nextDelay > 0 {
		l.printf("%s completed loop, next in %v\n", id, nextDelay)
	} else {
		l.printf("%s all done\n", id)
	}
}

// The default logger used in the following top-level functions.
var Default *Logger = New(ioutil.Discard)

func Listening(a string) {
	Default.Listening(a)
}

func Auth(netAddr net.Addr, user string, successful bool) {
	Default.Auth(netAddr, user, successful)
}

func Rejected(netAddr net.Addr, from string, to []string, err string) {
	Default.Rejected(netAddr, from, to, err)
}

func Queued(netAddr net.Addr, from string, to []string, id string) {
	Default.Queued(netAddr, from, to, id)
}

func SendAttempt(id, from, to string, err error, permanent bool) {
	Default.SendAttempt(id, from, to, err, permanent)
}

func QueueLoop(id string, nextDelay time.Duration) {
	Default.QueueLoop(id, nextDelay)
}
