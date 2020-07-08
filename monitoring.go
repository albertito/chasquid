package main

import (
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"blitiri.com.ar/go/log"

	// To enable live profiling in the monitoring server.
	_ "net/http/pprof"
)

func launchMonitoringServer(addr string) {
	log.Infof("Monitoring HTTP server listening on %s", addr)

	indexData := struct {
		Version    string
		SourceDate time.Time
		StartTime  time.Time
	}{
		Version:    version,
		SourceDate: sourceDate,
		StartTime:  time.Now(),
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := monitoringHTMLIndex.Execute(w, indexData); err != nil {
			log.Infof("monitoring handler error: %v", err)
		}
	})

	flags := dumpFlags()
	http.HandleFunc("/debug/flags", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(flags))
	})

	go http.ListenAndServe(addr, nil)
}

// Static index for the monitoring website.
var monitoringHTMLIndex = template.Must(template.New("index").Funcs(
	template.FuncMap{"since": time.Since}).Parse(
	`<!DOCTYPE html>
<html>
  <head>
    <title>chasquid monitoring</title>
  </head>
  <body>
    <h1>chasquid monitoring</h1>

	chasquid {{.Version}}<br>
	source date {{.SourceDate.Format "2006-01-02 15:04:05 -0700"}}<p>

	started {{.StartTime.Format "Mon, 2006-01-02 15:04:05 -0700"}}<br>
	up since {{.StartTime | since}}<p>

    <ul>
      <li><a href="/debug/queue">queue</a>
      <li><a href="/debug/vars">exported variables</a>
	       <small><a href="https://golang.org/pkg/expvar/">(ref)</a></small>
	  <li>traces <small><a href="https://godoc.org/golang.org/x/net/trace">
            (ref)</a></small>
        <ul>
          <li><a href="/debug/requests?exp=1">requests (short-lived)</a>
          <li><a href="/debug/events?exp=1">events (long-lived)</a>
        </ul>
      <li><a href="/debug/flags">flags</a>
      <li><a href="/debug/pprof">pprof</a>
          <small><a href="https://golang.org/pkg/net/http/pprof/">
            (ref)</a></small>
        <ul>
          <li><a href="/debug/pprof/goroutine?debug=1">goroutines</a>
        </ul>
    </ul>
  </body>
</html>
`))

// dumpFlags to a string, for troubleshooting purposes.
func dumpFlags() string {
	s := ""
	visited := make(map[string]bool)

	// Print set flags first, then the rest.
	flag.Visit(func(f *flag.Flag) {
		s += fmt.Sprintf("-%s=%s\n", f.Name, f.Value.String())
		visited[f.Name] = true
	})

	s += "\n"
	flag.VisitAll(func(f *flag.Flag) {
		if !visited[f.Name] {
			s += fmt.Sprintf("-%s=%s\n", f.Name, f.Value.String())
		}
	})

	return s
}
