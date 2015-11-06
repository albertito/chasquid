// Package trace extends golang.org/x/net/trace.
package trace

import (
	"fmt"

	"github.com/golang/glog"
	nettrace "golang.org/x/net/trace"
)

type Trace struct {
	family string
	title  string
	t      nettrace.Trace
}

func New(family, title string) *Trace {
	return &Trace{family, title, nettrace.New(family, title)}
}

func (t *Trace) LazyPrintf(format string, a ...interface{}) {
	t.t.LazyPrintf(format, a...)

	if glog.V(2) {
		msg := fmt.Sprintf("%p %s %s: %+q", t, t.family, t.title,
			fmt.Sprintf(format, a...))
		glog.InfoDepth(1, msg)
	}
}

func (t *Trace) SetError() {
	t.t.SetError()
}

func (t *Trace) Errorf(format string, a ...interface{}) error {
	err := fmt.Errorf(format, a...)
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	if glog.V(2) {
		msg := fmt.Sprintf("%p %s %s: error: %v", t, t.family, t.title, err)
		glog.InfoDepth(1, msg)
	}
	return err
}

func (t *Trace) Error(err error) error {
	t.t.SetError()
	t.t.LazyPrintf("error: %v", err)

	if glog.V(2) {
		msg := fmt.Sprintf("%p %s %s: error: %v", t, t.family, t.title, err)
		glog.InfoDepth(1, msg)
	}
	return err
}

func (t *Trace) Finish() {
	t.t.Finish()
}
