package main

import (
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/log"

	// To enable live profiling in the monitoring server.
	_ "net/http/pprof"
)

func launchMonitoringServer(conf *config.Config) {
	log.Infof("Monitoring HTTP server listening on %s", conf.MonitoringAddress)

	osHostname, _ := os.Hostname()

	indexData := struct {
		Version    string
		SourceDate time.Time
		StartTime  time.Time
		Config     *config.Config
		Hostname   string
	}{
		Version:    version,
		SourceDate: sourceDate,
		StartTime:  time.Now(),
		Config:     conf,
		Hostname:   osHostname,
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

	http.HandleFunc("/debug/flags", debugFlagsHandler)

	go http.ListenAndServe(conf.MonitoringAddress, nil)
}

// Functions available inside the templates.
var tmplFuncs = template.FuncMap{
	"since":         time.Since,
	"roundDuration": roundDuration,
}

// Static index for the monitoring website.
var monitoringHTMLIndex = template.Must(
	template.New("index").Funcs(tmplFuncs).Parse(
		`<!DOCTYPE html>
<html>

<head>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Hostname}}: chasquid monitoring</title>

<style type="text/css">
  body {
    font-family: sans-serif;
  }
</style>
</head>

<body>
<h1>chasquid @{{.Config.Hostname}}</h1>

chasquid {{.Version}}<br>
source date {{.SourceDate.Format "2006-01-02 15:04:05 -0700"}}<p>

started {{.StartTime.Format "Mon, 2006-01-02 15:04:05 -0700"}}<br>
up for {{.StartTime | since | roundDuration}}<br>
os hostname <i>{{.Hostname}}</i><p>

<ul>
  <li><a href="/debug/queue">queue</a>
  <li>monitoring
    <ul>
      <li><a href="/debug/requests?exp=1">requests (short-lived)</a>
      <li><a href="/debug/events?exp=1">events (long-lived)</a>
      <li><a href="/debug/vars">exported variables</a>
        <small><a href="https://golang.org/pkg/expvar/">(ref)</a></small>
    </ul>
  <li>execution
    <ul>
      <li><a href="/debug/flags">flags</a>
      <li><a href="/debug/pprof/cmdline">command line</a>
    </ul>
  <li><a href="/debug/pprof">pprof</a>
      <small><a href="https://golang.org/pkg/net/http/pprof/">(ref)</a></small>
    <ul>
    </ul>
</ul>
</body>

</html>
`))

func debugFlagsHandler(w http.ResponseWriter, r *http.Request) {
	visited := make(map[string]bool)

	// Print set flags first, then the rest.
	flag.Visit(func(f *flag.Flag) {
		fmt.Fprintf(w, "-%s=%s\n", f.Name, f.Value.String())
		visited[f.Name] = true
	})

	fmt.Fprintf(w, "\n")

	flag.VisitAll(func(f *flag.Flag) {
		if !visited[f.Name] {
			fmt.Fprintf(w, "-%s=%s\n", f.Name, f.Value.String())
		}
	})
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Second)
}
