package nettrace

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func expectEvents(t *testing.T, tr Trace, n int) {
	t.Helper()
	if evts := tr.(*trace).Events(); len(evts) != n {
		t.Errorf("expected %d events, got %d", n, len(evts))
		t.Logf("%v", evts)
	}
}

func TestBasic(t *testing.T) {
	var tr Trace = New("TestBasic", "basic")
	defer tr.Finish()
	tr.Printf("hola marola")
	tr.Printf("hola marola 2")

	c1 := tr.NewChild("TestBasic", "basic child")
	c1.Printf("hijo de la noche")

	expectEvents(t, tr, 3)

	if s := tr.(*trace).String(); !strings.Contains(s, "TestBasic, basic") {
		t.Errorf("tr.String does not contain family and title: %q", s)
	}
}

func TestLong(t *testing.T) {
	tr := New("TestLong", "long")
	defer tr.Finish()
	tr.SetMaxEvents(100)

	// First 90 events, no drop.
	for i := 0; i < 90; i++ {
		tr.Printf("evt %d", i)
	}
	expectEvents(t, tr, 90)

	// Up to 99, still no drop.
	for i := 0; i < 9; i++ {
		tr.Printf("evt %d", i)
	}
	expectEvents(t, tr, 99)

	// Note that we go up to 101 due to rounding errors, we're ok with it.
	tr.Printf("evt 100")
	expectEvents(t, tr, 100)
	tr.Printf("evt 101")
	expectEvents(t, tr, 101)
	tr.Printf("evt 102")
	expectEvents(t, tr, 101)

	// Add more events, expect none of them to exceed 101.
	for i := 0; i < 9; i++ {
		tr.Printf("evt %d", i)
		expectEvents(t, tr, 101)
	}
}

func TestIsError(t *testing.T) {
	tr := New("TestIsError", "long")
	defer tr.Finish()
	if tr.(*trace).IsError() != false {
		tr.Errorf("new trace is error")
	}

	tr.Errorf("this is an error")
	if tr.(*trace).IsError() != true {
		tr.Errorf("error not recorded properly")
	}
}

func TestFindViaRef(t *testing.T) {
	parent := New("TestFindViaRef", "parent")
	parentID := parent.(*trace).ID
	defer parent.Finish()
	child := parent.NewChild("TestFindViaRef", "child")
	childID := child.(*trace).ID
	defer child.Finish()

	// Basic check that both can be directly found.
	if f := findInFamilies(parentID, id("")); f != parent {
		t.Errorf("didn't find parent directly, found: %v", f)
	}
	if f := findInFamilies(childID, id("")); f != child {
		t.Errorf("didn't find child directly, found: %v", f)
	}

	// Hackily remove child from the active traces, to force a reference
	// lookup when needed.
	familiesMu.Lock()
	delete(families["TestFindViaRef"].active, child.(*trace).ID)
	familiesMu.Unlock()

	// Now the child should not be findable directly anymore.
	if f := findInFamilies(childID, id("")); f != nil {
		t.Errorf("child should not be findable directly, found: %v", f)
	}

	// But we still should be able to get to it via the parent.
	if f := findInFamilies(childID, parentID); f != child {
		t.Errorf("didn't find child via parent, found: %v", f)
	}
}

func TestMaxEvents(t *testing.T) {
	tr := trace{}

	// Test that we keep a minimum, and that the cutoff behaves as expected.
	cases := []struct{ me, exp, cutoffExp int }{
		{0, 6, 2},
		{5, 6, 2},
		{6, 6, 2},
		{7, 7, 2},
		{8, 8, 2},
		{9, 9, 3},
		{12, 12, 4},
	}
	for _, c := range cases {
		tr.SetMaxEvents(c.me)
		if got := tr.maxEvents; got != c.exp {
			t.Errorf("set max events to %d, expected %d, got %d",
				c.me, c.exp, got)
		}
		if got := tr.cutoff; got != c.cutoffExp {
			t.Errorf("set max events to %d, expected cutoff %d, got %d",
				c.me, c.cutoffExp, got)
		}
	}
}

func TestFind(t *testing.T) {
	// Make sure we find the right bucket, including for latencies above the
	// last one.
	for i, d := range buckets {
		found := findBucket(d + 1*time.Millisecond)
		if found != i {
			t.Errorf("find bucket [%s + 1ms] got %d, expected %d",
				d, found, i)
		}
	}

	// Create a family, populate it with traces in all buckets.
	finished := [nBuckets]*trace{}
	for i, d := range buckets {
		lat := d + 1*time.Millisecond
		tr := newTrace("TestFind", fmt.Sprintf("evt-%s", lat))
		tr.end = tr.Start.Add(lat)
		families[tr.Family].finalize(tr)
		finished[i] = tr
	}

	// Also have an active trace.
	activeTr := newTrace("TestFind", "active")

	// And add an error trace, which isn't on any of the other buckets (to
	// simulate that they've been rotated out of the latency buckets, but are
	// still around in errors)
	errTr := newTrace("TestFind", "evt-err")
	errTr.end = errTr.Start.Add(666 * time.Millisecond)
	errTr.SetError()
	delete(families[errTr.Family].active, errTr.ID)
	families[errTr.Family].errors.Add(errTr)

	// Find all of them.
	for i := range buckets {
		found := findInFamilies(finished[i].ID, "")
		if found != finished[i] {
			t.Errorf("finding trace %d on bucket %s, expected %v, got %v",
				i, buckets[i], finished[i], found)
		}
	}
	if found := findInFamilies(activeTr.ID, ""); found != activeTr {
		t.Errorf("finding active trace, expected %v, got %v",
			activeTr, found)
	}
	if found := findInFamilies(errTr.ID, ""); found != errTr {
		t.Errorf("finding error trace, expected %v, got %v",
			errTr, found)
	}

	// Non-existent traces.
	if found := findInFamilies("does!notexist", ""); found != nil {
		t.Errorf("finding non-existent trace, expected nil, got %v", found)
	}
	if found := findInFamilies("does!notexist", "does!notexist"); found != nil {
		t.Errorf("finding non-existent trace w/ref, expected nil, got %v", found)
	}
}

func TestFindParent(t *testing.T) {
	// Direct parent finding.
	// If the ref is the parent, we should find it even if the target trace
	// isn't known to the family (e.g. the child is there, but the parent has
	// been rotated and is no longer referenced).

	parent := newTrace("TestFindParent", "parent")
	child := parent.NewChild("TestFindParent", "child").(*trace)

	// Remove the parent from the active list.
	delete(families[parent.Family].active, parent.ID)

	if found := findInFamilies(parent.ID, ""); found != nil {
		t.Errorf("finding parent without ref, expected nil, got %v", found)
	}
	if found := findInFamilies(parent.ID, child.ID); found != parent {
		t.Errorf("finding parent with ref, expected %v, got %v", parent, found)
	}
}
