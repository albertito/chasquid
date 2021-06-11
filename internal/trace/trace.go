// Package trace extends golang.org/x/net/trace.
package trace

import (
	"fmt"
	"net/http"
	"strconv"

	"blitiri.com.ar/go/log"

	nettrace "golang.org/x/net/trace"
)

func init() {
	// golang.org/x/net/trace has its own authorization which by default only
	// allows localhost. This can be confusing and limiting in environments
	// which access the monitoring server remotely.
	nettrace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
		return true, true
	}
}

// A Trace represents an active request.
type Trace struct {
	family string
	title  string
	t      nettrace.Trace
}

// New trace.
func New(family, title string) *Trace {
	t := &Trace{family, title, nettrace.New(family, title)}

	// The default for max events is 10, which is a bit short for a normal
	// SMTP exchange. Expand it to 30 which should be large enough to keep
	// most of the traces.
	t.t.SetMaxEvents(30)
	return t
}

// Printf adds this message to the trace's log.
func (t *Trace) Printf(format string, a ...interface{}) {
	t.t.LazyPrintf(format, a...)

	log.Log(log.Info, 1, "%s %s: %s", t.family, t.title,
		quote(fmt.Sprintf(format, a...)))
}

// Debugf adds this message to the trace's log, with a debugging level.
func (t *Trace) Debugf(format string, a ...interface{}) {
	t.t.LazyPrintf(format, a...)

	log.Log(log.Debug, 1, "%s %s: %s",
		t.family, t.title, quote(fmt.Sprintf(format, a...)))
}

// Errorf adds this message to the trace's log, with an error level.
func (t *Trace) Errorf(format string, a ...interface{}) error {
	// Note we can't just call t.Error here, as it breaks caller logging.
	err := fmt.Errorf(format, a...)
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))
	return err
}

// Error marks the trace as having seen an error, and also logs it to the
// trace's log.
func (t *Trace) Error(err error) error {
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))

	return err
}

// Finish the trace. It should not be changed after this is called.
func (t *Trace) Finish() {
	t.t.Finish()
}

// EventLog is used for tracing long-lived objects.
type EventLog struct {
	family string
	title  string
	e      nettrace.EventLog
}

// NewEventLog returns a new EventLog.
func NewEventLog(family, title string) *EventLog {
	return &EventLog{family, title, nettrace.NewEventLog(family, title)}
}

// Printf adds the message to the EventLog.
func (e *EventLog) Printf(format string, a ...interface{}) {
	e.e.Printf(format, a...)

	log.Log(log.Info, 1, "%s %s: %s", e.family, e.title,
		quote(fmt.Sprintf(format, a...)))
}

// Debugf adds the message to the EventLog, with a debugging level.
func (e *EventLog) Debugf(format string, a ...interface{}) {
	e.e.Printf(format, a...)

	log.Log(log.Debug, 1, "%s %s: %s", e.family, e.title,
		quote(fmt.Sprintf(format, a...)))
}

// Errorf adds the message to the EventLog, with an error level.
func (e *EventLog) Errorf(format string, a ...interface{}) error {
	err := fmt.Errorf(format, a...)
	e.e.Errorf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s",
		e.family, e.title, quote(err.Error()))

	return err
}

func quote(s string) string {
	qs := strconv.Quote(s)
	return qs[1 : len(qs)-1]
}
