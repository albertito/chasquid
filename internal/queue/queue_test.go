package queue

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/set"
)

// Test courier. Delivery is done by sending on a channel, so users have fine
// grain control over the results.
type ChanCourier struct {
	requests chan deliverRequest
	results  chan error
}

type deliverRequest struct {
	from string
	to   string
	data []byte
}

func (cc *ChanCourier) Deliver(from string, to string, data []byte) (error, bool) {
	cc.requests <- deliverRequest{from, to, data}
	return <-cc.results, false
}
func newChanCourier() *ChanCourier {
	return &ChanCourier{
		requests: make(chan deliverRequest),
		results:  make(chan error),
	}
}

// Courier for test purposes. Never fails, and always remembers everything.
type TestCourier struct {
	wg       sync.WaitGroup
	requests []*deliverRequest
	reqFor   map[string]*deliverRequest
	sync.Mutex
}

func (tc *TestCourier) Deliver(from string, to string, data []byte) (error, bool) {
	defer tc.wg.Done()
	dr := &deliverRequest{from, to, data}
	tc.Lock()
	tc.requests = append(tc.requests, dr)
	tc.reqFor[to] = dr
	tc.Unlock()
	return nil, false
}

func newTestCourier() *TestCourier {
	return &TestCourier{
		reqFor: map[string]*deliverRequest{},
	}
}

func TestBasic(t *testing.T) {
	localC := newTestCourier()
	remoteC := newTestCourier()
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		localC, remoteC)

	localC.wg.Add(2)
	remoteC.wg.Add(1)
	id, err := q.Put("host", "from", []string{"am@loco", "x@remote", "nodomain"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if len(id) < 6 {
		t.Errorf("short ID: %v", id)
	}

	localC.wg.Wait()
	remoteC.wg.Wait()

	cases := []struct {
		courier    *TestCourier
		expectedTo string
	}{
		{localC, "nodomain"},
		{localC, "am@loco"},
		{remoteC, "x@remote"},
	}
	for _, c := range cases {
		req := c.courier.reqFor[c.expectedTo]
		if req == nil {
			t.Errorf("missing request for %q", c.expectedTo)
			continue
		}

		if req.from != "from" || req.to != c.expectedTo ||
			!bytes.Equal(req.data, []byte("data")) {
			t.Errorf("wrong request for %q: %v", c.expectedTo, req)
		}
	}
}

func TestFullQueue(t *testing.T) {
	q := New("/tmp/queue_test", set.NewString(), aliases.NewResolver(),
		dumbCourier, dumbCourier)

	// Force-insert maxQueueSize items in the queue.
	oneID := ""
	for i := 0; i < maxQueueSize; i++ {
		item := &Item{
			Message: Message{
				ID:   <-newID,
				From: fmt.Sprintf("from-%d", i),
				Rcpt: []*Recipient{
					{"to", Recipient_EMAIL, Recipient_PENDING, "", ""}},
				Data: []byte("data"),
			},
			CreatedAt: time.Now(),
		}
		q.q[item.ID] = item
		oneID = item.ID
	}

	// This one should fail due to the queue being too big.
	id, err := q.Put("host", "from", []string{"to"}, []byte("data-qf"))
	if err != errQueueFull {
		t.Errorf("Not failed as expected: %v - %v", id, err)
	}

	// Remove one, and try again: it should succeed.
	// Write it first so we don't get complaints about the file not existing
	// (as we did not all the items properly).
	q.q[oneID].WriteTo(q.path)
	q.Remove(oneID)

	id, err = q.Put("host", "from", []string{"to"}, []byte("data"))
	if err != nil {
		t.Errorf("Put: %v", err)
	}
	q.Remove(id)
}

// Dumb courier, for when we just want to return directly.
type DumbCourier struct{}

func (c DumbCourier) Deliver(from string, to string, data []byte) (error, bool) {
	return nil, false
}

var dumbCourier = DumbCourier{}

func TestAliases(t *testing.T) {
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		dumbCourier, dumbCourier)

	q.aliases.AddDomain("loco")
	q.aliases.AddAliasForTesting("ab@loco", "pq@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("ab@loco", "rs@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("ab@loco", "command", aliases.PIPE)
	q.aliases.AddAliasForTesting("cd@loco", "ata@hualpa", aliases.EMAIL)

	cases := []struct {
		to       []string
		expected []*Recipient
	}{
		{[]string{"ab@loco"}, []*Recipient{
			{"pq@loco", Recipient_EMAIL, Recipient_PENDING, "", "ab@loco"},
			{"rs@loco", Recipient_EMAIL, Recipient_PENDING, "", "ab@loco"},
			{"command", Recipient_PIPE, Recipient_PENDING, "", "ab@loco"}}},
		{[]string{"ab@loco", "cd@loco"}, []*Recipient{
			{"pq@loco", Recipient_EMAIL, Recipient_PENDING, "", "ab@loco"},
			{"rs@loco", Recipient_EMAIL, Recipient_PENDING, "", "ab@loco"},
			{"command", Recipient_PIPE, Recipient_PENDING, "", "ab@loco"},
			{"ata@hualpa", Recipient_EMAIL, Recipient_PENDING, "", "cd@loco"}}},
	}
	for _, c := range cases {
		id, err := q.Put("host", "from", c.to, []byte("data"))
		if err != nil {
			t.Errorf("Put: %v", err)
		}
		item := q.q[id]
		if !reflect.DeepEqual(item.Rcpt, c.expected) {
			t.Errorf("case %q, expected %v, got %v", c.to, c.expected, item.Rcpt)
		}
		q.Remove(id)
	}
}

func TestPipes(t *testing.T) {
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		dumbCourier, dumbCourier)
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from",
			Rcpt: []*Recipient{
				{"true", Recipient_PIPE, Recipient_PENDING, "", ""}},
			Data: []byte("data"),
		},
		CreatedAt: time.Now(),
	}

	if err, _ := item.deliver(q, item.Rcpt[0]); err != nil {
		t.Errorf("pipe delivery failed: %v", err)
	}
}
