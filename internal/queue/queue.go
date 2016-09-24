// Package queue implements our email queue.
// Accepted envelopes get put in the queue, and processed asynchronously.
package queue

// Command to generate queue.pb.go from queue.proto.
//go:generate protoc --go_out=. -I=${GOPATH}/src -I. queue.proto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bytes"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/protoio"
	"blitiri.com.ar/go/chasquid/internal/set"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/trace"
)

const (
	// Maximum size of the queue; we reject emails when we hit this.
	maxQueueSize = 200

	// Give up sending attempts after this duration.
	giveUpAfter = 12 * time.Hour

	// Prefix for item file names.
	// This is for convenience, versioning, and to be able to tell them apart
	// temporary files and other cruft.
	// It's important that it's outside the base64 space so it doesn't get
	// generated accidentally.
	itemFilePrefix = "m:"
)

var (
	errQueueFull = fmt.Errorf("Queue size too big, try again later")
)

// Channel used to get random IDs for items in the queue.
var newID chan string

func generateNewIDs() {
	// IDs are base64(8 random bytes), but the code doesn't care.
	var err error
	buf := make([]byte, 8)
	id := ""
	for {
		_, err = rand.Read(buf)
		if err != nil {
			panic(err)
		}

		id = base64.RawURLEncoding.EncodeToString(buf)
		newID <- id
	}

}

func init() {
	newID = make(chan string, 4)
	go generateNewIDs()
}

// Queue that keeps mail waiting for delivery.
type Queue struct {
	// Items in the queue. Map of id -> Item.
	q map[string]*Item

	// Mutex protecting q.
	mu sync.RWMutex

	// Couriers to use to deliver mail.
	localC  courier.Courier
	remoteC courier.Courier

	// Domains we consider local.
	localDomains *set.String

	// Path where we store the queue.
	path string

	// Aliases resolver.
	aliases *aliases.Resolver
}

// Load the queue and launch the sending loops on startup.
func New(path string, localDomains *set.String, aliases *aliases.Resolver,
	localC, remoteC courier.Courier) *Queue {

	os.MkdirAll(path, 0700)

	return &Queue{
		q:            map[string]*Item{},
		localC:       localC,
		remoteC:      remoteC,
		localDomains: localDomains,
		path:         path,
		aliases:      aliases,
	}
}

func (q *Queue) Load() error {
	files, err := filepath.Glob(q.path + "/" + itemFilePrefix + "*")
	if err != nil {
		return err
	}

	for _, fname := range files {
		item, err := ItemFromFile(fname)
		if err != nil {
			glog.Errorf("error loading queue item from %q: %v", fname, err)
			continue
		}

		q.mu.Lock()
		q.q[item.ID] = item
		q.mu.Unlock()

		go item.SendLoop(q)
	}

	return nil
}

func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.q)
}

// Put an envelope in the queue.
func (q *Queue) Put(from string, to []string, data []byte) (string, error) {
	if q.Len() >= maxQueueSize {
		return "", errQueueFull
	}

	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: from,
			Data: data,
		},
		CreatedAt: time.Now(),
	}

	for _, t := range to {
		rcpts, err := q.aliases.Resolve(t)
		if err != nil {
			return "", fmt.Errorf("error resolving aliases for %q: %v", t, err)
		}

		// Add the recipients (after resolving aliases); this conversion is
		// not very pretty but at least it's self contained.
		for _, aliasRcpt := range rcpts {
			r := &Recipient{
				Address: aliasRcpt.Addr,
				Status:  Recipient_PENDING,
			}
			switch aliasRcpt.Type {
			case aliases.EMAIL:
				r.Type = Recipient_EMAIL
			case aliases.PIPE:
				r.Type = Recipient_PIPE
			default:
				glog.Errorf("unknown alias type %v when resolving %q",
					aliasRcpt.Type, t)
				return "", fmt.Errorf("internal error - unknown alias type")
			}
			item.Rcpt = append(item.Rcpt, r)
		}
	}

	err := item.WriteTo(q.path)
	if err != nil {
		return "", fmt.Errorf("failed to write item: %v", err)
	}

	q.mu.Lock()
	q.q[item.ID] = item
	q.mu.Unlock()

	glog.Infof("%s accepted from %q", item.ID, from)

	// Begin to send it right away.
	go item.SendLoop(q)

	return item.ID, nil
}

// Remove an item from the queue.
func (q *Queue) Remove(id string) {
	path := fmt.Sprintf("%s/%s%s", q.path, itemFilePrefix, id)
	err := os.Remove(path)
	if err != nil {
		glog.Errorf("failed to remove queue file %q: %v", path, err)
	}

	q.mu.Lock()
	delete(q.q, id)
	q.mu.Unlock()
}

// TODO: http handler for dumping the queue.
// Register it in main().

// An item in the queue.
type Item struct {
	// Base the item on the protobuf message.
	// We will use this for serialization, so any fields below are NOT
	// serialized.
	Message

	// Protect the entire item.
	sync.Mutex

	// Go-friendly version of Message.CreatedAtTs.
	CreatedAt time.Time
}

func ItemFromFile(fname string) (*Item, error) {
	item := &Item{}
	err := protoio.ReadTextMessage(fname, &item.Message)
	if err != nil {
		return nil, err
	}

	item.CreatedAt, err = ptypes.Timestamp(item.CreatedAtTs)
	return item, err
}

func (item *Item) WriteTo(dir string) error {
	item.Lock()
	defer item.Unlock()

	var err error
	item.CreatedAtTs, err = ptypes.TimestampProto(item.CreatedAt)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/%s%s", dir, itemFilePrefix, item.ID)

	return protoio.WriteTextMessage(path, &item.Message, 0600)
}

func (item *Item) SendLoop(q *Queue) {

	tr := trace.New("Queue", item.ID)
	defer tr.Finish()
	tr.LazyPrintf("from: %s", item.From)

	var delay time.Duration
	for time.Since(item.CreatedAt) < giveUpAfter {
		// Send to all recipients that are still pending.
		var wg sync.WaitGroup
		for _, rcpt := range item.Rcpt {
			item.Lock()
			status := rcpt.Status
			item.Unlock()

			if status != Recipient_PENDING {
				continue
			}

			wg.Add(1)
			go func(rcpt *Recipient, oldStatus Recipient_Status) {
				defer wg.Done()
				to := rcpt.Address
				tr.LazyPrintf("%s sending", to)

				err, permanent := item.deliver(q, rcpt)

				if err != nil {
					if permanent {
						tr.LazyPrintf("permanent error: %v", err)
						glog.Infof("%s  -> %q permanent fail: %v", item.ID, to, err)
						status = Recipient_FAILED
					} else {
						tr.LazyPrintf("error: %v", err)
						glog.Infof("%s  -> %q fail: %v", item.ID, to, err)
					}
				} else {
					tr.LazyPrintf("%s successful", to)
					glog.Infof("%s  -> %q sent", item.ID, to)

					status = Recipient_SENT
				}

				// Update + write on status change.
				if oldStatus != status {
					item.Lock()
					rcpt.Status = status
					item.Unlock()

					err = item.WriteTo(q.path)
					if err != nil {
						tr.LazyPrintf("failed to write: %v", err)
						glog.Errorf("%s failed to write: %v", item.ID, err)
					}
				}
			}(rcpt, status)
		}
		wg.Wait()

		pending := 0
		for _, rcpt := range item.Rcpt {
			if rcpt.Status == Recipient_PENDING {
				pending++
			}
		}

		if pending == 0 {
			// Completed to all recipients (some may not have succeeded).
			tr.LazyPrintf("all done")
			glog.Infof("%s all done", item.ID)

			q.Remove(item.ID)
			return
		}

		// TODO: Consider sending a non-final notification after 30m or so,
		// that some of the messages have been delayed.

		delay = nextDelay(delay)
		tr.LazyPrintf("waiting for %v", delay)
		glog.Infof("%s waiting for %v", item.ID, delay)
		time.Sleep(delay)
	}

	// TODO: Send a notification message for the recipients we failed to send,
	// remove item from the queue, and remove from disk.
}

// deliver the item to the given recipient, using the couriers from the queue.
// Return an error (if any), and whether it is permanent or not.
func (item *Item) deliver(q *Queue, rcpt *Recipient) (err error, permanent bool) {
	if rcpt.Type == Recipient_PIPE {
		c := strings.Fields(rcpt.Address)
		if len(c) == 0 {
			return fmt.Errorf("empty pipe"), true
		}
		ctx, _ := context.WithDeadline(context.Background(),
			time.Now().Add(30*time.Second))
		cmd := exec.CommandContext(ctx, c[0], c[1:]...)
		cmd.Stdin = bytes.NewReader(item.Data)
		return cmd.Run(), true

	} else {
		if envelope.DomainIn(rcpt.Address, q.localDomains) {
			return q.localC.Deliver(item.From, rcpt.Address, item.Data)
		} else {
			return q.remoteC.Deliver(item.From, rcpt.Address, item.Data)
		}
	}
}

func nextDelay(last time.Duration) time.Duration {
	switch {
	case last < 1*time.Minute:
		return 1 * time.Minute
	case last < 5*time.Minute:
		return 5 * time.Minute
	case last < 10*time.Minute:
		return 10 * time.Minute
	default:
		return 20 * time.Minute
	}
}

func timestampNow() *timestamp.Timestamp {
	now := time.Now()
	ts, _ := ptypes.TimestampProto(now)
	return ts
}
