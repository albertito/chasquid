package nettrace

import (
	"testing"
	"time"
)

func TestHistogramBasic(t *testing.T) {
	h := histogram{}
	h.Add(1, 1*time.Millisecond)

	snap := h.Snapshot()
	if snap.Count != 1 ||
		snap.Min != 1*time.Millisecond ||
		snap.Max != 1*time.Millisecond ||
		snap.Avg != 1*time.Millisecond {
		t.Errorf("expected snapshot with only 1 sample, got %v", snap)
	}
}

func TestHistogramEmpty(t *testing.T) {
	h := histogram{}
	snap := h.Snapshot()

	if len(snap.Counts) != nBuckets || snap.Count != 0 ||
		snap.Avg != 0 || snap.Min != 0 || snap.Max != 0 {
		t.Errorf("expected zero snapshot, got %v", snap)
	}
}
