// Package trace extends nettrace with logging.
package trace

import (
	"fmt"
	"strconv"

	"blitiri.com.ar/go/chasquid/internal/nettrace"
	"blitiri.com.ar/go/log"
)

func init() {
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
	// SMTP exchange. Expand it to 100 which should be large enough to keep
	// most of the traces.
	t.t.SetMaxEvents(100)
	return t
}

// NewChild creates a new child trace.
func (t *Trace) NewChild(family, title string) *Trace {
	n := &Trace{family, title, t.t.NewChild(family, title)}
	n.t.SetMaxEvents(100)
	return n
}

// Printf adds this message to the trace's log.
func (t *Trace) Printf(format string, a ...interface{}) {
	t.t.Printf(format, a...)

	log.Log(log.Info, 1, "%s %s: %s", t.family, t.title,
		quote(fmt.Sprintf(format, a...)))
}

// Debugf adds this message to the trace's log, with a debugging level.
func (t *Trace) Debugf(format string, a ...interface{}) {
	t.t.Printf(format, a...)

	log.Log(log.Debug, 1, "%s %s: %s",
		t.family, t.title, quote(fmt.Sprintf(format, a...)))
}

// Errorf adds this message to the trace's log, with an error level.
func (t *Trace) Errorf(format string, a ...interface{}) error {
	// Note we can't just call t.Error here, as it breaks caller logging.
	err := fmt.Errorf(format, a...)
	t.t.SetError()
	t.t.Printf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))
	return err
}

// Error marks the trace as having seen an error, and also logs it to the
// trace's log.
func (t *Trace) Error(err error) error {
	t.t.SetError()
	t.t.Printf("error: %v", err)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(err.Error()))

	return err
}

// Finish the trace. It should not be changed after this is called.
func (t *Trace) Finish() {
	t.t.Finish()
}

func quote(s string) string {
	qs := strconv.Quote(s)
	return qs[1 : len(qs)-1]
}
