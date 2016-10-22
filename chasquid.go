package main

import (
	"expvar"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/smtpsrv"
	"blitiri.com.ar/go/chasquid/internal/systemd"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
)

// Command-line flags.
var (
	configDir = flag.String("config_dir", "/etc/chasquid",
		"configuration directory")
	showVer = flag.Bool("version", false, "show version and exit")
)

// Build information, overridden at build time using
// -ldflags="-X main.version=blah".
var (
	version      = "undefined"
	sourceDateTs = "0"
)

var (
	versionVar = expvar.NewString("chasquid/version")

	sourceDate      time.Time
	sourceDateVar   = expvar.NewString("chasquid/sourceDateStr")
	sourceDateTsVar = expvar.NewInt("chasquid/sourceDateTimestamp")
)

func main() {
	flag.Parse()

	parseVersionInfo()
	if *showVer {
		fmt.Printf("chasquid %s (source date: %s)\n", version, sourceDate)
		return
	}

	glog.Infof("chasquid starting (version %s)", version)

	setupSignalHandling()

	defer glog.Flush()
	go periodicallyFlushLogs()

	// Seed the PRNG, just to prevent for it to be totally predictable.
	rand.Seed(time.Now().UnixNano())

	conf, err := config.Load(*configDir + "/chasquid.conf")
	if err != nil {
		glog.Fatalf("Error reading config: %v", err)
	}
	config.LogConfig(conf)

	// Change to the config dir.
	// This allow us to use relative paths for configuration directories.
	// It also can be useful in unusual environments and for testing purposes,
	// where paths inside the configuration itself could be relative, and this
	// fixes the point of reference.
	os.Chdir(*configDir)

	initMailLog(conf.MailLogPath)

	if conf.MonitoringAddress != "" {
		launchMonitoringServer(conf.MonitoringAddress)
	}

	s := smtpsrv.NewServer()
	s.Hostname = conf.Hostname
	s.MaxDataSize = conf.MaxDataSizeMb * 1024 * 1024
	s.PostDataHook = "hooks/post-data"

	s.SetAliasesConfig(conf.SuffixSeparators, conf.DropCharacters)

	// Load certificates from "certs/<directory>/{fullchain,privkey}.pem".
	// The structure matches letsencrypt's, to make it easier for that case.
	glog.Infof("Loading certificates")
	for _, info := range mustReadDir("certs/") {
		name := info.Name()
		glog.Infof("  %s", name)

		certPath := filepath.Join("certs/", name, "fullchain.pem")
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			continue
		}
		keyPath := filepath.Join("certs/", name, "privkey.pem")
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			continue
		}

		err := s.AddCerts(certPath, keyPath)
		if err != nil {
			glog.Fatalf("    %v", err)
		}
	}

	// Load domains from "domains/".
	glog.Infof("Domain config paths:")
	for _, info := range mustReadDir("domains/") {
		domain, err := normalize.Domain(info.Name())
		if err != nil {
			glog.Fatalf("Invalid name %+q: %v", info.Name(), err)
		}
		dir := filepath.Join("domains", info.Name())
		loadDomain(domain, dir, s)
	}

	// Always include localhost as local domain.
	// This can prevent potential trouble if we were to accidentally treat it
	// as a remote domain (for loops, alias resolutions, etc.).
	s.AddDomain("localhost")

	dinfo := s.InitDomainInfo(conf.DataDir + "/domaininfo")

	localC := &courier.Procmail{
		Binary:  conf.MailDeliveryAgentBin,
		Args:    conf.MailDeliveryAgentArgs,
		Timeout: 30 * time.Second,
	}
	remoteC := &courier.SMTP{Dinfo: dinfo}
	s.InitQueue(conf.DataDir+"/queue", localC, remoteC)

	// Load the addresses and listeners.
	systemdLs, err := systemd.Listeners()
	if err != nil {
		glog.Fatalf("Error getting systemd listeners: %v", err)
	}

	loadAddresses(s, conf.SmtpAddress,
		systemdLs["smtp"], smtpsrv.ModeSMTP)
	loadAddresses(s, conf.SubmissionAddress,
		systemdLs["submission"], smtpsrv.ModeSubmission)

	s.ListenAndServe()
}

func loadAddresses(srv *smtpsrv.Server, addrs []string, ls []net.Listener, mode smtpsrv.SocketMode) {
	// Load addresses.
	acount := 0
	for _, addr := range addrs {
		// The "systemd" address indicates we get listeners via systemd.
		if addr == "systemd" {
			srv.AddListeners(ls, mode)
			acount += len(ls)
		} else {
			srv.AddAddr(addr, mode)
			acount++
		}
	}

	if acount == 0 {
		glog.Errorf("No %v addresses/listeners", mode)
		glog.Errorf("If using systemd, check that you named the sockets")
		glog.Fatalf("Exiting")
	}
}

func initMailLog(path string) {
	var err error

	if path == "<syslog>" {
		maillog.Default, err = maillog.NewSyslog()
	} else {
		os.MkdirAll(filepath.Dir(path), 0775)
		var f *os.File
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		maillog.Default = maillog.New(f)
	}

	if err != nil {
		glog.Fatalf("Error opening mail log: %v", err)
	}
}

// Helper to load a single domain configuration into the server.
func loadDomain(name, dir string, s *smtpsrv.Server) {
	glog.Infof("  %s", name)
	s.AddDomain(name)

	if _, err := os.Stat(dir + "/users"); err == nil {
		glog.Infof("    adding users")
		udb, err := userdb.Load(dir + "/users")
		if err != nil {
			glog.Errorf("      error: %v", err)
		} else {
			s.AddUserDB(name, udb)
		}
	}

	glog.Infof("    adding aliases")
	err := s.AddAliasesFile(name, dir+"/aliases")
	if err != nil {
		glog.Errorf("      error: %v", err)
	}
}

// Flush logs periodically, to help troubleshooting if there isn't that much
// traffic.
func periodicallyFlushLogs() {
	for range time.Tick(5 * time.Second) {
		glog.Flush()
	}
}

// Set up signal handling, to flush logs when we get killed.
func setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		glog.Flush()
		os.Exit(1)
	}()
}

// Read a directory, which must have at least some entries.
func mustReadDir(path string) []os.FileInfo {
	dirs, err := ioutil.ReadDir(path)
	if err != nil {
		glog.Fatalf("Error reading %q directory: %v", path, err)
	}
	if len(dirs) == 0 {
		glog.Fatalf("No entries found in %q", path)
	}

	return dirs
}

func parseVersionInfo() {
	versionVar.Set(version)

	sdts, err := strconv.ParseInt(sourceDateTs, 10, 0)
	if err != nil {
		panic(err)
	}

	sourceDate = time.Unix(sdts, 0)
	sourceDateVar.Set(sourceDate.Format("2006-01-02 15:04:05 -0700"))
	sourceDateTsVar.Set(sdts)
}

func launchMonitoringServer(addr string) {
	glog.Infof("Monitoring HTTP server listening on %s", addr)

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
			glog.Infof("monitoring handler error: %v", err)
		}
	})

	flags := dumpFlags()
	http.HandleFunc("/debug/flags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flags))
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
