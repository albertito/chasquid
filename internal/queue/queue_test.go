package queue

import (
	"bytes"
	"fmt"
	"strings"
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
		localC, remoteC, "dsndomain")

	localC.wg.Add(2)
	remoteC.wg.Add(1)
	id, err := q.Put("from", []string{"am@loco", "x@remote", "nodomain"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if len(id) < 6 {
		t.Errorf("short ID: %v", id)
	}

	localC.wg.Wait()
	remoteC.wg.Wait()

	// Make sure the delivered items leave the queue.
	for d := time.Now().Add(2 * time.Second); time.Now().Before(d); {
		if q.Len() == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if q.Len() != 0 {
		t.Fatalf("%d items not removed from the queue after delivery", q.Len())
	}

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

func TestDSNOnTimeout(t *testing.T) {
	localC := newTestCourier()
	remoteC := newTestCourier()
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		localC, remoteC, "dsndomain")

	// Insert an expired item in the queue.
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: fmt.Sprintf("from@loco"),
			Rcpt: []*Recipient{
				{"to@to", Recipient_EMAIL, Recipient_PENDING, "err", "to@to"}},
			Data: []byte("data"),
		},
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	q.q[item.ID] = item
	err := item.WriteTo(q.path)
	if err != nil {
		t.Errorf("failed to write item: %v", err)
	}

	// Launch the sending loop, expect 1 local delivery (the DSN).
	localC.wg.Add(1)
	go item.SendLoop(q)
	localC.wg.Wait()

	req := localC.reqFor["from@loco"]
	if req == nil {
		t.Fatal("missing DSN")
	}

	if req.from != "<>" || req.to != "from@loco" ||
		!strings.Contains(string(req.data), "X-Failed-Recipients: to@to,") {
		t.Errorf("wrong DSN: %q", string(req.data))
	}
}

func TestAliases(t *testing.T) {
	localC := newTestCourier()
	remoteC := newTestCourier()
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		localC, remoteC, "dsndomain")

	q.aliases.AddDomain("loco")
	q.aliases.AddAliasForTesting("ab@loco", "pq@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("ab@loco", "rs@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("cd@loco", "ata@hualpa", aliases.EMAIL)
	// Note the pipe aliases are tested below, as they don't use the couriers
	// and it can be quite inconvenient to test them in this way.

	localC.wg.Add(2)
	remoteC.wg.Add(1)
	_, err := q.Put("from", []string{"ab@loco", "cd@loco"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	localC.wg.Wait()
	remoteC.wg.Wait()

	cases := []struct {
		courier    *TestCourier
		expectedTo string
	}{
		{localC, "pq@loco"},
		{localC, "rs@loco"},
		{remoteC, "ata@hualpa"},
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

// Dumb courier, for when we just want to return directly.
type DumbCourier struct{}

func (c DumbCourier) Deliver(from string, to string, data []byte) (error, bool) {
	return nil, false
}

var dumbCourier = DumbCourier{}

func TestFullQueue(t *testing.T) {
	q := New("/tmp/queue_test", set.NewString(), aliases.NewResolver(),
		dumbCourier, dumbCourier, "dsndomain")

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
	id, err := q.Put("from", []string{"to"}, []byte("data-qf"))
	if err != errQueueFull {
		t.Errorf("Not failed as expected: %v - %v", id, err)
	}

	// Remove one, and try again: it should succeed.
	// Write it first so we don't get complaints about the file not existing
	// (as we did not all the items properly).
	q.q[oneID].WriteTo(q.path)
	q.Remove(oneID)

	id, err = q.Put("from", []string{"to"}, []byte("data"))
	if err != nil {
		t.Errorf("Put: %v", err)
	}
	q.Remove(id)
}

func TestPipes(t *testing.T) {
	q := New("/tmp/queue_test", set.NewString("loco"), aliases.NewResolver(),
		dumbCourier, dumbCourier, "dsndomain")
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

func TestNextDelay(t *testing.T) {
	cases := []struct{ since, min time.Duration }{
		{10 * time.Second, 1 * time.Minute},
		{3 * time.Minute, 5 * time.Minute},
		{7 * time.Minute, 10 * time.Minute},
		{15 * time.Minute, 20 * time.Minute},
		{30 * time.Minute, 20 * time.Minute},
	}
	for _, c := range cases {
		// Repeat each case a few times to exercise the perturbation a bit.
		for i := 0; i < 10; i++ {
			delay := nextDelay(time.Now().Add(-c.since))

			max := c.min + 1*time.Minute
			if delay < c.min || delay > max {
				t.Errorf("since:%v  expected [%v, %v], got %v",
					c.since, c.min, max, delay)
			}
		}
	}
}
