package queue

import (
	"bytes"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	q := New()

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

	// TODO: There's a race because the item may finish the loop before we
	// poll it from the queue, and we would get a nil item in that case.
	// We have to live with this for now, and will close it later once we
	// implement deliveries.
	if item == nil {
		t.Logf("hit item race, nothing else to do")
		return
	}

	if item.From != "from" || item.To[0] != "to" ||
		!bytes.Equal(item.Data, []byte("data")) {
		t.Errorf("different item: %#v", item)
	}
}

func TestFullQueue(t *testing.T) {
	q := New()

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
