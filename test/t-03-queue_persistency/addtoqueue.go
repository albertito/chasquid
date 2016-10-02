// addtoqueue is a test helper which adds a queue item directly to the queue
// directory, behind chasquid's back.
//
// Note that chasquid does NOT support this, we do it before starting up the
// daemon for testing purposes only.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"blitiri.com.ar/go/chasquid/internal/queue"
)

var (
	queueDir = flag.String("queue_dir", ".queue", "queue directory")
	id       = flag.String("id", "mid1234", "Message ID")
	from     = flag.String("from", "from", "Mail from")
	rcpt     = flag.String("rcpt", "rcpt", "Rcpt to")
)

func main() {
	flag.Parse()

	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Printf("error reading data: %v\n", err)
		os.Exit(1)
	}

	item := &queue.Item{
		Message: queue.Message{
			ID:   *id,
			From: *from,
			To:   []string{*rcpt},
			Rcpt: []*queue.Recipient{
				{*rcpt, queue.Recipient_EMAIL, queue.Recipient_PENDING, "", ""},
			},
			Data: data,
		},
		CreatedAt: time.Now(),
	}

	os.MkdirAll(*queueDir, 0700)
	err = item.WriteTo(*queueDir)
	if err != nil {
		fmt.Printf("error writing item: %v\n", err)
		os.Exit(1)
	}
}
