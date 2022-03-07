// Package nettrace implements tracing of requests. Traces are created by
// nettrace.New, and can then be viewed over HTTP on /debug/traces.
package nettrace

import (
	"container/ring"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IDs are of the form "family!timestamp!unique identifier", which allows for
// sorting them by time much easily, and also some convenient optimizations
// when looking up an id across all the known ones.
// Family is not escaped. It should not contain the separator.
// It is not expected to be stable, for internal use only.
type id string

func newID(family string, ts int64) id {
	return id(
		family + "!" +
			strconv.FormatInt(ts, 10) + "!" +
			strconv.FormatUint(rand.Uint64(), 10))
}

func (id id) Family() string {
	sp := strings.SplitN(string(id), "!", 2)
	if len(sp) != 2 {
		return string(id)
	}
	return sp[0]
}

// Trace represents a single request trace.
type Trace interface {
	// NewChild creates a new trace, that is a child of this one.
	NewChild(family, title string) Trace

	// Link to another trace with the given message.
	Link(other Trace, msg string)

	// SetMaxEvents sets the maximum number of events that will be stored in
	// the trace. It must be called right after initialization.
	SetMaxEvents(n int)

	// SetError marks that the trace was for an error event.
	SetError()

	// Printf adds a message to the trace.
	Printf(format string, a ...interface{})

	// Errorf adds a message to the trace, marks it as an error, and returns
	// an error for it.
	Errorf(format string, a ...interface{}) error

	// Finish marks the trace as complete.
	// The trace should not be used after calling this method.
	Finish()
}

// A single trace. Can be active or inactive.
// Exported fields are allowed to be accessed directly, e.g. by the HTTP
// handler. Private ones are mutex protected.
type trace struct {
	ID id

	Family string
	Title  string

	Parent *trace

	Start time.Time

	// Fields below are mu-protected.
	// We keep them unexported so they're not accidentally accessed in html
	// templates.
	mu sync.Mutex

	end time.Time

	isError   bool
	maxEvents int

	// We keep two separate groups: the first ~1/3rd events in a simple slice,
	// and the last 2/3rd in a ring so we can drop events without losing the
	// first ones.
	cutoff      int
	firstEvents []event
	lastEvents  *evtRing
}

type evtType uint8

const (
	evtLOG = evtType(1 + iota)
	evtCHILD
	evtLINK
	evtDROP
)

func (t evtType) IsLog() bool   { return t == evtLOG }
func (t evtType) IsChild() bool { return t == evtCHILD }
func (t evtType) IsLink() bool  { return t == evtLINK }
func (t evtType) IsDrop() bool  { return t == evtDROP }

type event struct {
	When time.Time
	Type evtType

	Ref *trace
	Msg string
}

const defaultMaxEvents = 30

func newTrace(family, title string) *trace {
	start := time.Now()
	tr := &trace{
		ID:     newID(family, start.UnixNano()),
		Family: family,
		Title:  title,
		Start:  start,

		maxEvents: defaultMaxEvents,
		cutoff:    defaultMaxEvents / 3,
	}

	// Pre-allocate a couple of events to speed things up.
	// Don't allocate lastEvents, that can be expensive and it is not always
	// needed. No need to slow down trace creation just for it.
	tr.firstEvents = make([]event, 0, 4)

	familiesMu.Lock()
	ft, ok := families[family]
	if !ok {
		ft = newFamilyTraces()
		families[family] = ft
	}
	familiesMu.Unlock()

	ft.mu.Lock()
	ft.active[tr.ID] = tr
	ft.mu.Unlock()

	return tr
}

// New creates a new trace with the given family and title.
func New(family, title string) Trace {
	return newTrace(family, title)
}

func (tr *trace) append(evt *event) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if len(tr.firstEvents) < tr.cutoff {
		tr.firstEvents = append(tr.firstEvents, *evt)
		return
	}

	if tr.lastEvents == nil {
		// The ring holds the last 2/3rds of the events.
		tr.lastEvents = newEvtRing(tr.maxEvents - tr.cutoff)
	}

	tr.lastEvents.Add(evt)
}

// String is for debugging only.
func (tr *trace) String() string {
	return fmt.Sprintf("trace{%s, %s, %q, %d}",
		tr.Family, tr.Title, tr.ID, len(tr.Events()))
}

func (tr *trace) NewChild(family, title string) Trace {
	c := newTrace(family, title)
	c.Parent = tr

	// Add the event to the parent.
	evt := &event{
		When: time.Now(),
		Type: evtCHILD,
		Ref:  c,
	}
	tr.append(evt)

	return c
}

func (tr *trace) Link(other Trace, msg string) {
	evt := &event{
		When: time.Now(),
		Type: evtLINK,
		Ref:  other.(*trace),
		Msg:  msg,
	}
	tr.append(evt)
}

func (tr *trace) SetMaxEvents(n int) {
	// Set a minimum of 6, so the truncation works without running into
	// issues.
	if n < 6 {
		n = 6
	}
	tr.mu.Lock()
	tr.maxEvents = n
	tr.cutoff = n / 3
	tr.mu.Unlock()
}

func (tr *trace) SetError() {
	tr.mu.Lock()
	tr.isError = true
	tr.mu.Unlock()
}

func (tr *trace) Printf(format string, a ...interface{}) {
	evt := &event{
		When: time.Now(),
		Type: evtLOG,
		Msg:  fmt.Sprintf(format, a...),
	}

	tr.append(evt)
}

func (tr *trace) Errorf(format string, a ...interface{}) error {
	tr.SetError()
	err := fmt.Errorf(format, a...)
	tr.Printf(err.Error())
	return err
}

func (tr *trace) Finish() {
	tr.mu.Lock()
	tr.end = time.Now()
	tr.mu.Unlock()

	familiesMu.Lock()
	ft := families[tr.Family]
	familiesMu.Unlock()
	ft.finalize(tr)
}

// Duration of this trace.
func (tr *trace) Duration() time.Duration {
	tr.mu.Lock()
	start, end := tr.Start, tr.end
	tr.mu.Unlock()

	if end.IsZero() {
		return time.Since(start)
	}
	return end.Sub(start)
}

// Events returns a copy of the trace events.
func (tr *trace) Events() []event {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	evts := make([]event, len(tr.firstEvents))
	copy(evts, tr.firstEvents)

	if tr.lastEvents == nil {
		return evts
	}

	if !tr.lastEvents.firstDrop.IsZero() {
		evts = append(evts, event{
			When: tr.lastEvents.firstDrop,
			Type: evtDROP,
		})
	}

	tr.lastEvents.Do(func(e *event) {
		evts = append(evts, *e)
	})

	return evts
}

func (tr *trace) IsError() bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.isError
}

//
// Trace hierarchy
//
// Each trace belongs to a family. For each family, we have all active traces,
// and then N traces that finished <1s, N that finished <2s, etc.

// We keep this many buckets of finished traces.
const nBuckets = 8

// Buckets to use. Lenght must match nBuckets.
// "Traces with a latency >= $duration".
var buckets = []time.Duration{
	time.Duration(0),
	5 * time.Millisecond,
	10 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	300 * time.Millisecond,
	1 * time.Second,
	10 * time.Second,
}

func findBucket(latency time.Duration) int {
	for i, d := range buckets {
		if latency >= d {
			continue
		}
		return i - 1
	}

	return nBuckets - 1
}

// How many traces we keep per bucket.
const tracesInBucket = 10

type traceRing struct {
	ring *ring.Ring
	max  int
	l    int
}

func newTraceRing(n int) *traceRing {
	return &traceRing{
		ring: ring.New(n),
		max:  n,
	}
}

func (r *traceRing) Add(tr *trace) {
	r.ring.Value = tr
	r.ring = r.ring.Next()
	if r.l < r.max {
		r.l++
	}
}

func (r *traceRing) Len() int {
	return r.l
}

func (r *traceRing) Do(f func(tr *trace)) {
	r.ring.Do(func(x interface{}) {
		if x == nil {
			return
		}
		f(x.(*trace))
	})
}

type familyTraces struct {
	mu sync.Mutex

	// All active ones.
	active map[id]*trace

	// The ones we decided to keep.
	// Each bucket is a ring-buffer, finishedHead keeps the head pointer.
	finished [nBuckets]*traceRing

	// The ones that errored have their own bucket.
	errors *traceRing

	// Histogram of latencies.
	latencies histogram
}

func newFamilyTraces() *familyTraces {
	ft := &familyTraces{}
	ft.active = map[id]*trace{}
	for i := 0; i < nBuckets; i++ {
		ft.finished[i] = newTraceRing(tracesInBucket)
	}
	ft.errors = newTraceRing(tracesInBucket)
	return ft
}

func (ft *familyTraces) LenActive() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return len(ft.active)
}

func (ft *familyTraces) LenErrors() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.errors.Len()
}

func (ft *familyTraces) LenBucket(b int) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.finished[b].Len()
}

func (ft *familyTraces) TracesFor(b int, allgt bool) []*trace {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	trs := []*trace{}
	appendTrace := func(tr *trace) {
		trs = append(trs, tr)
	}
	if b == -2 {
		ft.errors.Do(appendTrace)
	} else if b == -1 {
		for _, tr := range ft.active {
			appendTrace(tr)
		}
	} else if b < nBuckets {
		ft.finished[b].Do(appendTrace)
		if allgt {
			for i := b + 1; i < nBuckets; i++ {
				ft.finished[i].Do(appendTrace)
			}
		}
	}

	// Sort them by start, newer first. This is the order that will be used
	// when displaying them.
	sort.Slice(trs, func(i, j int) bool {
		return trs[i].Start.After(trs[j].Start)
	})
	return trs
}

func (ft *familyTraces) find(id id) *trace {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if tr, ok := ft.active[id]; ok {
		return tr
	}

	var found *trace
	for _, bs := range ft.finished {
		bs.Do(func(tr *trace) {
			if tr.ID == id {
				found = tr
			}
		})
		if found != nil {
			return found
		}
	}

	ft.errors.Do(func(tr *trace) {
		if tr.ID == id {
			found = tr
		}
	})
	if found != nil {
		return found
	}

	return nil
}

func (ft *familyTraces) finalize(tr *trace) {
	latency := tr.end.Sub(tr.Start)
	b := findBucket(latency)

	ft.mu.Lock()

	// Delete from the active list.
	delete(ft.active, tr.ID)

	// Add it to the corresponding finished bucket, based on the trace
	// latency.
	ft.finished[b].Add(tr)

	// Errors go on their own list, in addition to the above.
	if tr.isError {
		ft.errors.Add(tr)
	}

	ft.latencies.Add(b, latency)

	ft.mu.Unlock()
}

func (ft *familyTraces) Latencies() *histSnapshot {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.latencies.Snapshot()
}

//
// Global state
//

var (
	familiesMu sync.Mutex
	families   = map[string]*familyTraces{}
)

func copyFamilies() map[string]*familyTraces {
	n := map[string]*familyTraces{}

	familiesMu.Lock()
	for f, trs := range families {
		n[f] = trs
	}
	familiesMu.Unlock()

	return n
}

func findInFamilies(traceID id, refID id) *trace {
	// First, try to find it via the family.
	family := traceID.Family()
	familiesMu.Lock()
	fts, ok := families[family]
	familiesMu.Unlock()

	if ok {
		tr := fts.find(traceID)
		if tr != nil {
			return tr
		}
	}

	// If that fail and we have a reference, try finding via it.
	// The reference can be a parent or a child.
	if refID != id("") {
		ref := findInFamilies(refID, "")
		if ref == nil {
			return nil
		}

		// Is the reference's parent the one we're looking for?
		if ref.Parent != nil && ref.Parent.ID == traceID {
			return ref.Parent
		}

		// Try to find it in the ref's events.
		for _, e := range ref.Events() {
			if e.Ref != nil && e.Ref.ID == traceID {
				return e.Ref
			}
		}
	}
	return nil
}
