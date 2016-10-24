// Package trace extends golang.org/x/net/trace.
package trace

import (
	"fmt"
	"strconv"

	"blitiri.com.ar/go/chasquid/internal/log"

	nettrace "golang.org/x/net/trace"
)

type Trace struct {
	family string
	title  string
	t      nettrace.Trace
}

func New(family, title string) *Trace {
	t := &Trace{family, title, nettrace.New(family, title)}

	// The default for max events is 10, which is a bit short for a normal
	// SMTP exchange. Expand it to 30 which should be large enough to keep
	// most of the traces.
	t.t.SetMaxEvents(30)
	return t
}

func (t *Trace) Printf(format string, a ...interface{}) {
	t.t.LazyPrintf(format, a...)

	log.Log(log.Info, 1, "%s %s: %s", t.family, t.title,
		quote(fmt.Sprintf(format, a...)))
}

func (t *Trace) Debugf(format string, a ...interface{}) {
	t.t.LazyPrintf(format, a...)

	log.Log(log.Debug, 1, "%s %s: %s",
		t.family, t.title, quote(fmt.Sprintf(format, a...)))
}

func (t *Trace) SetError() {
	t.t.SetError()
}

func (t *Trace) Errorf(format string, a ...interface{}) error {
	// Note we can't just call t.Error here, as it breaks caller logging.
	err := fmt.Errorf(format, a...)
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))
	return err
}

func (t *Trace) Error(err error) error {
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))

	return err
}

func (t *Trace) Finish() {
	t.t.Finish()
}

type EventLog struct {
	family string
	title  string
	e      nettrace.EventLog
}

func NewEventLog(family, title string) *EventLog {
	return &EventLog{family, title, nettrace.NewEventLog(family, title)}
}

func (e *EventLog) Printf(format string, a ...interface{}) {
	e.e.Printf(format, a...)

	log.Log(log.Info, 1, "%s %s: %s", e.family, e.title,
		quote(fmt.Sprintf(format, a...)))
}

func (e *EventLog) Debugf(format string, a ...interface{}) {
	e.e.Printf(format, a...)

	log.Log(log.Debug, 1, "%s %s: %s", e.family, e.title,
		quote(fmt.Sprintf(format, a...)))
}

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
