package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"runtime"
	"time"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/expvarom"
	"blitiri.com.ar/go/log"
	"google.golang.org/protobuf/encoding/prototext"

	// To enable live profiling in the monitoring server.
	_ "net/http/pprof"
)

func launchMonitoringServer(conf *config.Config) {
	log.Infof("Monitoring HTTP server listening on %s", conf.MonitoringAddress)

	osHostname, _ := os.Hostname()

	indexData := struct {
		Version    string
		GoVersion  string
		SourceDate time.Time
		StartTime  time.Time
		Config     *config.Config
		Hostname   string
	}{
		Version:    version,
		GoVersion:  runtime.Version(),
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

	srv := &http.Server{Addr: conf.MonitoringAddress}

	http.HandleFunc("/exit", exitHandler(srv))
	http.HandleFunc("/metrics", expvarom.MetricsHandler)
	http.HandleFunc("/debug/flags", debugFlagsHandler)
	http.HandleFunc("/debug/config", debugConfigHandler(conf))

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Monitoring server failed: %v", err)
	}
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
  @media (prefers-color-scheme: dark) {
    body {
      background: #121212;
      color: #c9d1d9;
    }
    a { color: #44b4ec; }
  }
</style>
</head>

<body>
<h1>chasquid @{{.Config.Hostname}}</h1>

<p>
chasquid {{.Version}}<br>
source date {{.SourceDate.Format "2006-01-02 15:04:05 -0700"}}<br>
built with {{.GoVersion}}<br>
</p>

<p>
started {{.StartTime.Format "Mon, 2006-01-02 15:04:05 -0700"}}<br>
up for {{.StartTime | since | roundDuration}}<br>
os hostname <i>{{.Hostname}}</i><br>
</p>

<ul>
  <li><a href="/debug/queue">queue</a>
  <li>monitoring
    <ul>
      <li><a href="/debug/requests?exp=1">requests (short-lived)</a>
      <li><a href="/debug/events?exp=1">events (long-lived)</a>
	  <li><a href="https://blitiri.com.ar/p/chasquid/monitoring/#variables">
	        exported variables</a>:
          <a href="/debug/vars">expvar</a>
          <small><a href="https://golang.org/pkg/expvar/">(ref)</a></small>,
		  <a href="/metrics">openmetrics</a>
		  <small><a href="https://openmetrics.io/">(ref)</a></small>
    </ul>
  <li>execution
    <ul>
      <li><a href="/debug/flags">flags</a>
      <li><a href="/debug/config">config</a>
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

func exitHandler(srv *http.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Use POST method for exiting", http.StatusMethodNotAllowed)
			return
		}

		log.Infof("Received /exit")
		http.Error(w, "OK exiting", http.StatusOK)

		// Launch srv.Shutdown asynchronously, and then exit.
		// The http documentation says to wait for Shutdown to return before
		// exiting, to gracefully close all ongoing requests.
		go func() {
			if err := srv.Shutdown(context.Background()); err != nil {
				log.Fatalf("Monitoring server shutdown failed: %v", err)
			}
			os.Exit(0)
		}()
	}
}

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

func debugConfigHandler(conf *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(prototext.Format(conf)))
	}
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Second)
}
