// Package trace extends nettrace with logging.
package trace

import (
	"fmt"
	"strconv"
	"unicode/utf8"

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
func (t *Trace) Printf(format string, a ...any) {
	t.printf(log.Info, format, a...)
}

// Debugf adds this message to the trace's log, with a debugging level.
func (t *Trace) Debugf(format string, a ...any) {
	t.printf(log.Debug, format, a...)
}

func (t *Trace) printf(level log.Level, format string, a ...any) {
	// Format the message only once, as it can be expensive.
	msg := truncate(fmt.Sprintf(format, a...))
	t.t.Printf("%s", msg)

	// Only quote and log the message if it's going to be logged.
	if log.V(level) {
		log.Log(level, 2, "%s %s: %s", t.family, t.title, quote(msg))
	}
}

// Errorf adds this message to the trace's log, with an error level.
func (t *Trace) Errorf(format string, a ...any) error {
	// Note we can't just call t.Error here, as it breaks caller logging.
	err := fmt.Errorf(format, a...)
	msg := truncate(err.Error())
	t.t.SetError()
	t.t.Printf("error: %s", msg)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(msg))
	return err
}

// Error marks the trace as having seen an error, and also logs it to the
// trace's log.
func (t *Trace) Error(err error) error {
	msg := truncate(err.Error())
	t.t.SetError()
	t.t.Printf("error: %s", msg)

	log.Log(log.Info, 1, "%s %s: error: %s", t.family, t.title,
		quote(msg))

	return err
}

// Finish the trace. It should not be changed after this is called.
func (t *Trace) Finish() {
	t.t.Finish()
}

// Maximum length of a message in the trace or the log, in characters (so up
// to 4x that in bytes); longer ones get truncated. Messages can contain
// attacker-controlled data (e.g. headers, DNS records, remote server
// responses), and traces are kept in memory after they finish, so we need a
// cap. It is set well above the normal message sizes, so in practice it only
// affects pathological cases.
const maxMsgLen = 8 * 1024

// truncate s to maxMsgLen characters, noting its original length.
func truncate(s string) string {
	// Check the length in bytes first, as it's much cheaper, and it can't
	// have more characters than bytes.
	if len(s) <= maxMsgLen || utf8.RuneCountInString(s) <= maxMsgLen {
		return s
	}
	return fmt.Sprintf("%.*s… [truncated, %d bytes total]",
		maxMsgLen, s, len(s))
}

func quote(s string) string {
	qs := strconv.Quote(s)
	return qs[1 : len(qs)-1]
}
