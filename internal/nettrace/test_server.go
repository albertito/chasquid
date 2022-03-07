//go:build ignore

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	_ "net/http/pprof"

	"blitiri.com.ar/go/srv/nettrace"
)

func main() {
	addr := flag.String("addr", ":8080", "listening address")
	flag.Parse()

	go RandomEvents("random-one")
	go RandomEvents("random-two")
	go RandomEvents("random-three")
	go RandomLongEvent("random-long", "long-one")
	go RandomLongEvent("random-long", "long-two")
	go RandomLongEvent("random-long", "long-three")
	go RandomBunny("random-bunny", "first 🐇")
	go RandomNested("random-nested")
	go RandomLazy("random-lazy")

	http.DefaultServeMux.Handle("/",
		WithLogging(http.HandlerFunc(HandleRoot)))

	http.DefaultServeMux.Handle("/debug/traces",
		WithLogging(http.HandlerFunc(nettrace.RenderTraces)))

	fmt.Printf("listening on %s\n", *addr)
	http.ListenAndServe(*addr, nil)
}

func RandomEvents(family string) {
	for i := 0; ; i++ {
		tr := nettrace.New(family, fmt.Sprintf("evt-%d", i))
		randomTrace(family, tr)
	}
}

func randomTrace(family string, tr nettrace.Trace) {
	tr.Printf("this is a random event")
	tr.Printf("and it has a random delay")
	delay := time.Duration(rand.Intn(1000)) * time.Millisecond
	tr.Printf("sleeping %v", delay)
	time.Sleep(delay)

	if rand.Intn(100) < 1 {
		tr.Printf("this unlucky one is an error")
		tr.SetError()
	}

	if rand.Intn(100) < 10 {
		tr.Printf("this one got super verbose!")
		for j := 0; j < 100; j++ {
			tr.Printf("message %d", j)
		}
	}

	if rand.Intn(100) < 30 {
		tr.Printf("this one had a child")
		c := tr.NewChild(family, "achild")
		go randomTrace(family, c)
	}

	tr.Printf("all done")
	tr.Finish()
}

func RandomLongEvent(family, title string) {
	tr := nettrace.New(family, title)
	tr.Printf("this is a random long event")
	for i := 0; ; i++ {
		delay := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay)
		tr.Printf("message %d, slept %v", i, delay)
	}
	tr.Finish()
}

func RandomBunny(family, title string) {
	tr := nettrace.New(family, title)
	tr.SetMaxEvents(100)
	tr.Printf("this is the top 🐇")
	for i := 0; ; i++ {
		delay := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(delay)
		tr.Printf("message %d, slept %v", i, delay)

		if rand.Intn(100) < 40 {
			c := tr.NewChild(family, fmt.Sprintf("child-%d", i))
			go randomTrace(family, c)
		}

		if rand.Intn(100) < 40 {
			n := nettrace.New(family, fmt.Sprintf("linked-%d", i))
			go randomTrace(family, n)
			tr.Link(n, "linking with this guy")
		}
	}
	tr.Finish()
}

func randomNested(family string, depth int, parent nettrace.Trace) {
	tr := parent.NewChild(family, fmt.Sprintf("nested-%d", depth))
	defer tr.Finish()

	tr.Printf("I am a spoiled child")

	delay := time.Duration(rand.Intn(100)) * time.Millisecond
	time.Sleep(delay)
	tr.Printf("slept %v", delay)

	if depth > 10 {
		tr.Printf("I went too far")
		return
	}

	// If we make this < 50, then it grows forever.
	if rand.Intn(100) < 75 {
		tr.Printf("I sang really well")
		return
	}

	max := rand.Intn(5)
	for i := 0; i < max; i++ {
		tr.Printf("spawning %d", i)
		go randomNested(family, depth+1, tr)
	}

}
func RandomNested(family string) {
	tr := nettrace.New(family, "nested-0")
	for i := 0; ; i++ {
		randomNested(family, 1, tr)
	}
}

func RandomLazy(family string) {
	for i := 0; ; i++ {
		tr := nettrace.New(family, fmt.Sprintf("evt-%d", i))
		tr.Printf("I am very lazy and do little work")
		tr.Finish()
		time.Sleep(500 * time.Millisecond)
	}
}

func HandleRoot(w http.ResponseWriter, r *http.Request) {
	if delayS := r.FormValue("delay"); delayS != "" {
		delay, err := time.ParseDuration(delayS)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		time.Sleep(delay)
	}

	if withError := r.FormValue("error"); withError != "" {
		tr, _ := nettrace.FromContext(r.Context())
		tr.SetError()
	}

	w.Write([]byte(rootHTML))
}

func WithLogging(parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr := nettrace.New("http", r.URL.String())
		defer tr.Finish()

		// Associate the trace with this request.
		r = r.WithContext(nettrace.NewContext(r.Context(), tr))

		tr.Printf("%s %s %s %s",
			r.RemoteAddr, r.Proto, r.Method, r.URL.String())
		start := time.Now()
		parent.ServeHTTP(w, r)
		latency := time.Since(start)
		tr.Printf("handler took %s", latency)

		// 1.2.3.4:34575 HTTP/2.0 GET /path = 1.2ms
		fmt.Printf("%s %s %s %s = %s\n",
			r.RemoteAddr, r.Proto, r.Method, r.URL.String(),
			latency)
	})
}

const rootHTML = `
<html>
<head>
<style>
  td {
    min-width: 2em;
	text-align: center;
  }
  input#delay {
    width: 5em;
  }
</style>
</head>
<body>

<a href="/debug/traces">Traces</a><p>

<table>
  <tr>
  <td>Delay:</td>
  <td><a href="/?delay=0s">0s</a></td>
  <td><a href="/?delay=0.1s">0.1s</a></td>
  <td><a href="/?delay=0.2s">0.2s</a></td>
  <td><a href="/?delay=0.5s">0.5s</a></td>
  <td><a href="/?delay=1s">1s</a></td>
  </tr>

  <tr>
  <td>+ error:</td>
  <td><a href="/?delay=0&error=on">0s</a></td>
  <td><a href="/?delay=0.1s&error=on">0.1s</a></td>
  <td><a href="/?delay=0.2s&error=on">0.2s</a></td>
  <td><a href="/?delay=0.5s&error=on">0.5s</a></td>
  <td><a href="/?delay=1s&error=on">1s</a></td>
  </tr>
</table>

<form action="/" method="get">
  <label for="delay">Custom delay:</label>
  <input type="text" id="delay" name="delay" placeholder="250ms"
    autofocus required>
  <input type="checkbox" id="error" name="error">
  <label for="error">is error</label>
  <input type="submit">
</form>

</body>
</html>
`
