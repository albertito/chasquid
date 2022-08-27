//go:build !coverage
// +build !coverage

// SMTP connection generator, for testing purposes.
package main

import (
	"flag"
	"net"
	"net/http"
	"net/smtp"
	"time"

	"golang.org/x/net/trace"

	_ "net/http/pprof"

	"blitiri.com.ar/go/log"
)

var (
	addr = flag.String("addr", "",
		"server address")
	httpAddr = flag.String("http_addr", "localhost:8011",
		"monitoring HTTP server listening address")
	wait = flag.Bool("wait", false,
		"don't exit after --run_for has lapsed")
	count = flag.Int("count", 1000,
		"how many connections to open")
)

var (
	host string
	exit bool
)

func main() {
	var err error

	flag.Parse()
	log.Init()

	host, _, err = net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("failed to split --addr=%q: %v", *addr, err)
	}

	if *wait {
		go http.ListenAndServe(*httpAddr, nil)
		log.Infof("monitoring address: http://%v/debug/requests?fam=one&b=11",
			*httpAddr)
	}

	log.Infof("creating %d simultaneous connections", *count)
	conns := []*C{}
	for i := 0; i < *count; i++ {
		c, err := newC()
		if err != nil {
			log.Fatalf("failed to connect #%d: %v", i, err)
		}

		conns = append(conns, c)

		if i%200 == 0 {
			log.Infof("  ... %d connections", i)

		}
	}
	log.Infof("done, created %d simultaneous connections", *count)

	if *wait {
		for {
			time.Sleep(24 * time.Hour)
		}
	}
}

type C struct {
	tr trace.Trace
	n  net.Conn
	s  *smtp.Client
}

func newC() (*C, error) {
	tr := trace.New("conn", *addr)

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		return nil, err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, err
	}

	err = client.Hello(host)
	if err != nil {
		return nil, err
	}

	return &C{tr: tr, n: conn, s: client}, nil
}

func (c *C) close() {
	c.tr.Finish()
	c.s.Close()
	c.n.Close()
}
