//go:build !coverage
// +build !coverage

// SMTP load generator, for testing purposes.
package main

import (
	"flag"
	"net"
	"net/http"
	"net/textproto"
	"runtime"
	"sync"
	"time"

	_ "net/http/pprof"

	"golang.org/x/net/trace"

	"blitiri.com.ar/go/chasquid/internal/smtp"
	"blitiri.com.ar/go/log"
)

var (
	addr = flag.String("addr", "",
		"server address")
	httpAddr = flag.String("http_addr", "localhost:8011",
		"monitoring HTTP server listening address")
	parallel = flag.Int("parallel", 0,
		"how many sending loops to run in parallel")
	runFor = flag.Duration("run_for", 0,
		"how long to run for (0 = forever)")
	wait = flag.Bool("wait", false,
		"don't exit after --run_for has lapsed")
	noop = flag.Bool("noop", false,
		"don't send an email, just connect and run a NOOP")
)

var (
	host string
	exit bool

	globalCount   int64 = 0
	globalRuntime time.Duration
	globalMu      = &sync.Mutex{}
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

	if *parallel == 0 {
		*parallel = runtime.GOMAXPROCS(0)
	}

	lt := "full"
	if *noop {
		lt = "noop"
	}

	log.Infof("launching %d %s sending loops in parallel", *parallel, lt)
	for i := 0; i < *parallel; i++ {
		go serial(i)
	}

	var totalCount int64
	var totalRuntime time.Duration
	start := time.Now()
	for range time.Tick(1 * time.Second) {
		globalMu.Lock()
		totalCount += globalCount
		totalRuntime += globalRuntime
		count := globalCount
		runtime := globalRuntime
		globalCount = 0
		globalRuntime = 0
		globalMu.Unlock()

		if count == 0 {
			log.Infof("0 ops")
		} else {
			log.Infof("%d ops, %v /op", count,
				time.Duration(runtime.Nanoseconds()/count).Truncate(time.Microsecond))
		}

		if *runFor > 0 && time.Since(start) > *runFor {
			exit = true
			break
		}
	}

	end := time.Now()
	window := end.Sub(start)
	log.Infof("total: %d ops, %v wall, %v run",
		totalCount,
		window.Truncate(time.Millisecond),
		totalRuntime.Truncate(time.Millisecond))

	avgLat := time.Duration(totalRuntime.Nanoseconds() / totalCount)
	log.Infof("avg: %v /op, %.0f ops/s",
		avgLat.Truncate(time.Microsecond),
		float64(totalCount)/window.Seconds(),
	)

	if *wait {
		for {
			time.Sleep(24 * time.Hour)
		}
	}
}

func serial(id int) {
	var count int64
	start := time.Now()
	for {
		count += 1
		err := one()
		if err != nil {
			log.Fatalf("%v", err)
		}

		if count == 5 {
			globalMu.Lock()
			globalCount += count
			globalRuntime += time.Since(start)
			globalMu.Unlock()
			count = 0
			start = time.Now()

			if exit {
				return
			}
		}
	}
}

func one() error {
	tr := trace.New("one", *addr)
	defer tr.Finish()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if *noop {
		err = client.Noop()
		if err != nil {
			return err
		}
	} else {
		err = client.MailAndRcpt("test@test", "null@testserver")
		if err != nil {
			return err
		}

	retry:
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(body)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			// If we are sending too fast we might hit chasquid's queue size
			// limit. In that case, wait and try again.
			// We detect it with error code 451 which is used for this
			// situation.
			if terr, ok := err.(*textproto.Error); ok {
				if terr.Code == 451 {
					time.Sleep(10 * time.Millisecond)
					goto retry
				}
			}
			return err
		}
	}

	return nil
}

var body = []byte(`Subject: Load test

This is the body of the load test email.
`)
