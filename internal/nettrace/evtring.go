package nettrace

import "time"

type evtRing struct {
	evts      []event
	max       int
	pos       int // Points to the latest element.
	firstDrop time.Time
}

func newEvtRing(n int) *evtRing {
	return &evtRing{
		max: n,
		pos: -1,
	}
}

func (r *evtRing) Add(e *event) {
	if len(r.evts) < r.max {
		r.evts = append(r.evts, *e)
		r.pos++
		return
	}

	r.pos = (r.pos + 1) % r.max

	// Record the first drop as the time of the first dropped message.
	if r.firstDrop.IsZero() {
		r.firstDrop = r.evts[r.pos].When
	}

	r.evts[r.pos] = *e
}

func (r *evtRing) Do(f func(e *event)) {
	for i := 0; i < len(r.evts); i++ {
		// Go from older to newer by starting at (r.pos+1).
		pos := (r.pos + 1 + i) % len(r.evts)
		f(&r.evts[pos])
	}
}
