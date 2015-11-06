package queue

import (
	"bytes"
	"testing"
	"time"
)

// Our own courier, for testing purposes.
// Delivery is done by sending on a channel.
type ChanCourier struct {
	requests chan deliverRequest
	results  chan error
}

type deliverRequest struct {
	from string
	to   string
	data []byte
}

func (cc *ChanCourier) Deliver(from string, to string, data []byte) error {
	cc.requests <- deliverRequest{from, to, data}
	return <-cc.results
}

func newCourier() *ChanCourier {
	return &ChanCourier{
		requests: make(chan deliverRequest),
		results:  make(chan error),
	}
}

func TestBasic(t *testing.T) {
	courier := newCourier()
	q := New(courier)

	id, err := q.Put("from", []string{"to"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if len(id) < 6 {
		t.Errorf("short ID: %v", id)
	}

	q.mu.RLock()
	item := q.q[id]
	q.mu.RUnlock()

	if item == nil {
		t.Fatalf("item not in queue, racy test?")
	}

	if item.From != "from" || item.To[0] != "to" ||
		!bytes.Equal(item.Data, []byte("data")) {
		t.Errorf("different item: %#v", item)
	}

	// Test that we delivered the item.
	req := <-courier.requests
	courier.results <- nil

	if req.from != "from" || req.to != "to" ||
		!bytes.Equal(req.data, []byte("data")) {
		t.Errorf("different courier request: %#v", req)
	}
}

func TestFullQueue(t *testing.T) {
	q := New(newCourier())

	// Force-insert maxQueueSize items in the queue.
	oneID := ""
	for i := 0; i < maxQueueSize; i++ {
		item := &Item{
			ID:      <-newID,
			From:    "from",
			To:      []string{"to"},
			Data:    []byte("data"),
			Created: time.Now(),
			Results: map[string]error{},
		}
		q.q[item.ID] = item
		oneID = item.ID
	}

	// This one should fail due to the queue being too big.
	id, err := q.Put("from", []string{"to"}, []byte("data"))
	if err != queueFullError {
		t.Errorf("Not failed as expected: %v - %v", id, err)
	}

	// Remove one, and try again: it should succeed.
	q.Remove(oneID)
	_, err = q.Put("from", []string{"to"}, []byte("data"))
	if err != nil {
		t.Errorf("Put: %v", err)
	}
}
