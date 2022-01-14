package queue

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/testlib"
)

func allUsersExist(user, domain string) (bool, error) { return true, nil }

func TestBasic(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	localC := testlib.NewTestCourier()
	remoteC := testlib.NewTestCourier()
	q, _ := New(dir, set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		localC, remoteC)

	localC.Expect(2)
	remoteC.Expect(1)
	id, err := q.Put("from", []string{"am@loco", "x@remote", "nodomain"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if len(id) < 6 {
		t.Errorf("short ID: %v", id)
	}

	localC.Wait()
	remoteC.Wait()

	// Make sure the delivered items leave the queue.
	testlib.WaitFor(func() bool { return q.Len() == 0 }, 2*time.Second)
	if q.Len() != 0 {
		t.Fatalf("%d items not removed from the queue after delivery", q.Len())
	}

	cases := []struct {
		courier    *testlib.TestCourier
		expectedTo string
	}{
		{localC, "nodomain"},
		{localC, "am@loco"},
		{remoteC, "x@remote"},
	}
	for _, c := range cases {
		req := c.courier.ReqFor[c.expectedTo]
		if req == nil {
			t.Errorf("missing request for %q", c.expectedTo)
			continue
		}

		if req.From != "from" || req.To != c.expectedTo ||
			!bytes.Equal(req.Data, []byte("data")) {
			t.Errorf("wrong request for %q: %v", c.expectedTo, req)
		}
	}
}

func TestDSNOnTimeout(t *testing.T) {
	localC := testlib.NewTestCourier()
	remoteC := testlib.NewTestCourier()
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	q, _ := New(dir, set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		localC, remoteC)

	// Insert an expired item in the queue.
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from@loco",
			Rcpt: []*Recipient{
				mkR("to@to", Recipient_EMAIL, Recipient_PENDING, "err", "to@to")},
			Data: []byte("data"),
		},
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	q.q[item.ID] = item
	err := item.WriteTo(q.path)
	if err != nil {
		t.Errorf("failed to write item: %v", err)
	}

	// Exercise DumpString while at it.
	q.DumpString()

	// Launch the sending loop, expect 1 local delivery (the DSN).
	localC.Expect(1)
	go item.SendLoop(q)
	localC.Wait()

	req := localC.ReqFor["from@loco"]
	if req == nil {
		t.Fatal("missing DSN")
	}

	if req.From != "<>" || req.To != "from@loco" ||
		!strings.Contains(string(req.Data), "X-Failed-Recipients: to@to,") {
		t.Errorf("wrong DSN: %q", string(req.Data))
	}
}

func TestAliases(t *testing.T) {
	localC := testlib.NewTestCourier()
	remoteC := testlib.NewTestCourier()
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	q, _ := New(dir, set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		localC, remoteC)

	q.aliases.AddDomain("loco")
	q.aliases.AddAliasForTesting("ab@loco", "pq@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("ab@loco", "rs@loco", aliases.EMAIL)
	q.aliases.AddAliasForTesting("cd@loco", "ata@hualpa", aliases.EMAIL)
	// Note the pipe aliases are tested below, as they don't use the couriers
	// and it can be quite inconvenient to test them in this way.

	localC.Expect(2)
	remoteC.Expect(1)
	_, err := q.Put("from", []string{"ab@loco", "cd@loco"}, []byte("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	localC.Wait()
	remoteC.Wait()

	cases := []struct {
		courier    *testlib.TestCourier
		expectedTo string
	}{
		{localC, "pq@loco"},
		{localC, "rs@loco"},
		{remoteC, "ata@hualpa"},
	}
	for _, c := range cases {
		req := c.courier.ReqFor[c.expectedTo]
		if req == nil {
			t.Errorf("missing request for %q", c.expectedTo)
			continue
		}

		if req.From != "from" || req.To != c.expectedTo ||
			!bytes.Equal(req.Data, []byte("data")) {
			t.Errorf("wrong request for %q: %v", c.expectedTo, req)
		}
	}
}

func TestFullQueue(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	q, _ := New(dir, set.NewString(),
		aliases.NewResolver(allUsersExist),
		testlib.DumbCourier, testlib.DumbCourier)

	// Force-insert maxQueueSize items in the queue.
	oneID := ""
	for i := 0; i < maxQueueSize; i++ {
		item := &Item{
			Message: Message{
				ID:   <-newID,
				From: fmt.Sprintf("from-%d", i),
				Rcpt: []*Recipient{
					mkR("to", Recipient_EMAIL, Recipient_PENDING, "", "")},
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
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	q, _ := New(dir, set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		testlib.DumbCourier, testlib.DumbCourier)
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from",
			Rcpt: []*Recipient{
				mkR("true", Recipient_PIPE, Recipient_PENDING, "", "")},
			Data: []byte("data"),
		},
		CreatedAt: time.Now(),
	}

	if err, _ := item.deliver(q, item.Rcpt[0]); err != nil {
		t.Errorf("pipe delivery failed: %v", err)
	}
}

func TestBadPath(t *testing.T) {
	// A new queue will attempt to os.MkdirAll the path.
	// We expect this path to fail.
	_, err := New("/proc/doesnotexist", set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		testlib.DumbCourier, testlib.DumbCourier)
	if err == nil {
		t.Errorf("could create queue, expected permission denied")
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

func TestSerialization(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	// Save an item in the queue directory.
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from@loco",
			Rcpt: []*Recipient{
				mkR("to@to", Recipient_EMAIL, Recipient_PENDING, "err", "to@to")},
			Data: []byte("data"),
		},
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	err := item.WriteTo(dir)
	if err != nil {
		t.Errorf("failed to write item: %v", err)
	}

	// Create the queue; should load the
	remoteC := testlib.NewTestCourier()
	remoteC.Expect(1)
	q, _ := New(dir, set.NewString("loco"),
		aliases.NewResolver(allUsersExist),
		testlib.DumbCourier, remoteC)
	q.Load()

	// Launch the sending loop, expect 1 remote delivery for the item we saved.
	remoteC.Wait()

	req := remoteC.ReqFor["to@to"]
	if req == nil {
		t.Fatal("email not delivered")
	}

	if req.From != "from@loco" || req.To != "to@to" {
		t.Errorf("wrong email: %v", req)
	}
}

func mkR(a string, t Recipient_Type, s Recipient_Status, m, o string) *Recipient {
	return &Recipient{
		Address:            a,
		Type:               t,
		Status:             s,
		LastFailureMessage: m,
		OriginalAddress:    o,
	}
}
