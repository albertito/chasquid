package nettrace

import (
	"time"
)

type histogram struct {
	count [nBuckets]uint64

	totalQ uint64
	totalT time.Duration
	min    time.Duration
	max    time.Duration
}

func (h *histogram) Add(bucket int, latency time.Duration) {
	if h.totalQ == 0 || h.min > latency {
		h.min = latency
	}
	if h.max < latency {
		h.max = latency
	}

	h.count[bucket]++
	h.totalQ++
	h.totalT += latency
}

type histSnapshot struct {
	Counts        map[time.Duration]line
	Count         uint64
	Avg, Min, Max time.Duration
}

type line struct {
	Start     time.Duration
	BucketIdx int
	Count     uint64
	Percent   float32
	CumPct    float32
}

func (h *histogram) Snapshot() *histSnapshot {
	s := &histSnapshot{
		Counts: map[time.Duration]line{},
		Count:  h.totalQ,
		Min:    h.min,
		Max:    h.max,
	}

	if h.totalQ > 0 {
		s.Avg = time.Duration(uint64(h.totalT) / h.totalQ)
	}

	var cumCount uint64
	for i := 0; i < nBuckets; i++ {
		cumCount += h.count[i]
		l := line{
			Start:     buckets[i],
			BucketIdx: i,
			Count:     h.count[i],
		}
		if h.totalQ > 0 {
			l.Percent = float32(h.count[i]) / float32(h.totalQ) * 100
			l.CumPct = float32(cumCount) / float32(h.totalQ) * 100
		}
		s.Counts[buckets[i]] = l
	}

	return s
}
