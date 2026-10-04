package main

import (
	"context"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/expvarom"
	"blitiri.com.ar/go/chasquid/internal/nettrace"
	"blitiri.com.ar/go/log"
	"google.golang.org/protobuf/encoding/prototext"

	// To enable live profiling in the monitoring server.
	_ "net/http/pprof"
)

// Build information, overridden at build time using
// -ldflags="-X main.version=blah".
var (
	version      = ""
	sourceDateTs = ""
)

var (
	versionVar = expvar.NewString("chasquid/version")

	sourceDate      time.Time
	sourceDateVar   = expvar.NewString("chasquid/sourceDateStr")
	sourceDateTsVar = expvarom.NewInt("chasquid/sourceDateTimestamp",
		"timestamp when the binary was built, in seconds since epoch")
)

func parseVersionInfo() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		panic("unable to read build info")
	}

	dirty := false
	gitRev := ""
	gitTime := ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.modified":
			if s.Value == "true" {
				dirty = true
			}
		case "vcs.time":
			gitTime = s.Value
		case "vcs.revision":
			gitRev = s.Value
		}
	}

	if sourceDateTs != "" {
		sdts, err := strconv.ParseInt(sourceDateTs, 10, 0)
		if err != nil {
			panic(err)
		}

		sourceDate = time.Unix(sdts, 0)
	} else {
		sourceDate, _ = time.Parse(time.RFC3339, gitTime)
	}
	sourceDateVar.Set(sourceDate.Format("2006-01-02 15:04:05 -0700"))
	sourceDateTsVar.Set(sourceDate.Unix())

	if version == "" {
		version = sourceDate.Format("20060102")

		if gitRev != "" {
			version += fmt.Sprintf("-%.9s", gitRev)
		}
		if dirty {
			version += "-dirty"
		}
	}
	versionVar.Set(version)
}

func launchMonitoringServer(conf *config.Config) {
	log.Infof("Monitoring HTTP server listening on %s", conf.MonitoringAddress)

	srv := &http.Server{
		Addr:              conf.MonitoringAddress,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Use the default mux, as that's where the expvar and pprof handlers
	// register themselves.
	srv.Handler = monitoringHandler(http.DefaultServeMux, srv, conf)

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Monitoring server failed: %v", err)
	}
}

// monitoringHandler registers the monitoring handlers in the given mux, and
// returns the handler to use for the monitoring server.
func monitoringHandler(mux *http.ServeMux, srv *http.Server,
	conf *config.Config) http.Handler {
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

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := monitoringHTMLIndex.Execute(w, indexData); err != nil {
			log.Infof("monitoring handler error: %v", err)
		}
	})

	mux.HandleFunc("POST /exit", exitHandler(srv))
	mux.HandleFunc("GET /metrics", expvarom.MetricsHandler)
	mux.HandleFunc("GET /debug/flags", debugFlagsHandler)
	mux.HandleFunc("GET /debug/config", debugConfigHandler(conf))
	mux.HandleFunc("GET /debug/traces", nettrace.RenderTraces)

	// Reject cross-origin requests from browsers, except for safe methods
	// (like GET). This prevents cross-site request forgery: otherwise, any
	// web page could make the browser of someone who can reach the monitoring
	// server send it a POST /exit.
	return http.NewCrossOriginProtection().Handler(mux)
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
      <li><a href="/debug/traces">traces</a>
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

func debugFlagsHandler(w http.ResponseWriter, _ *http.Request) {
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
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(prototext.Format(conf)))
	}
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Second)
}
