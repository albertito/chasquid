// chasquid is an SMTP (email) server, with a focus on simplicity, security,
// and ease of operation.
//
// See https://blitiri.com.ar/p/chasquid for more details.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/dovecot"
	"blitiri.com.ar/go/chasquid/internal/localrpc"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/smtpsrv"
	"blitiri.com.ar/go/chasquid/internal/sts"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"blitiri.com.ar/go/log"
	"blitiri.com.ar/go/systemd"
)

// Command-line flags.
var (
	configDir = flag.String("config_dir", "/etc/chasquid",
		"configuration directory")
	configOverrides = flag.String("config_overrides", "",
		"override configuration values (in text protobuf format)")
	showVer = flag.Bool("version", false, "show version and exit")
)

func main() {
	flag.Parse()
	log.Init()

	parseVersionInfo()
	if *showVer {
		fmt.Printf("chasquid %s (source date: %s)\n", version, sourceDate)
		return
	}

	log.Infof("chasquid starting (version %s)", version)

	// Seed the PRNG, just to prevent for it to be totally predictable.
	rand.Seed(time.Now().UnixNano())

	conf, err := config.Load(*configDir+"/chasquid.conf", *configOverrides)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	config.LogConfig(conf)

	// Change to the config dir.
	// This allow us to use relative paths for configuration directories.
	// It also can be useful in unusual environments and for testing purposes,
	// where paths inside the configuration itself could be relative, and this
	// fixes the point of reference.
	err = os.Chdir(*configDir)
	if err != nil {
		log.Fatalf("Error changing to config dir %q: %v", *configDir, err)
	}

	initMailLog(conf.MailLogPath)

	if conf.MonitoringAddress != "" {
		go launchMonitoringServer(conf)
	}

	s := smtpsrv.NewServer()
	s.Hostname = conf.Hostname
	s.MaxDataSize = conf.MaxDataSizeMb * 1024 * 1024
	s.HookPath = "hooks/"
	s.HAProxyEnabled = conf.HaproxyIncoming

	s.SetAliasesConfig(*conf.SuffixSeparators, *conf.DropCharacters)

	if conf.DovecotAuth {
		loadDovecot(s, conf.DovecotUserdbPath, conf.DovecotClientPath)
	}

	// Load certificates from "certs/<directory>/{fullchain,privkey}.pem".
	// The structure matches letsencrypt's, to make it easier for that case.
	log.Infof("Loading certificates:")
	for _, info := range mustReadDir("certs/") {
		if info.Type().IsRegular() {
			// Ignore regular files, we only care about directories.
			continue
		}

		name := info.Name()
		dir := filepath.Join("certs/", name)
		loadCert(name, dir, s)
	}

	// Load domains from "domains/".
	log.Infof("Loading domains:")
	for _, info := range mustReadDir("domains/") {
		domain, err := normalize.Domain(info.Name())
		if err != nil {
			log.Fatalf("Invalid name %+q: %v", info.Name(), err)
		}
		if info.Type().IsRegular() {
			// Ignore regular files, we only care about directories.
                        continue
                }
		dir := filepath.Join("domains", info.Name())
		loadDomain(domain, dir, s)
	}

	// Always include localhost as local domain.
	// This can prevent potential trouble if we were to accidentally treat it
	// as a remote domain (for loops, alias resolutions, etc.).
	s.AddDomain("localhost")

	dinfo, err := domaininfo.New(conf.DataDir + "/domaininfo")
	if err != nil {
		log.Fatalf("Error opening domain info database: %v", err)
	}
	s.SetDomainInfo(dinfo)

	stsCache, err := sts.NewCache(conf.DataDir + "/sts-cache")
	if err != nil {
		log.Fatalf("Failed to initialize STS cache: %v", err)
	}
	go stsCache.PeriodicallyRefresh(context.Background())

	localC := &courier.MDA{
		Binary:  conf.MailDeliveryAgentBin,
		Args:    conf.MailDeliveryAgentArgs,
		Timeout: 30 * time.Second,
	}
	remoteC := &courier.SMTP{
		HelloDomain: conf.Hostname,
		Dinfo:       dinfo,
		STSCache:    stsCache,
	}
	s.InitQueue(conf.DataDir+"/queue", localC, remoteC)

	// Load the addresses and listeners.
	systemdLs, err := systemd.Listeners()
	if err != nil {
		log.Fatalf("Error getting systemd listeners: %v", err)
	}

	naddr := loadAddresses(s, conf.SmtpAddress,
		systemdLs["smtp"], smtpsrv.ModeSMTP)
	naddr += loadAddresses(s, conf.SubmissionAddress,
		systemdLs["submission"], smtpsrv.ModeSubmission)
	naddr += loadAddresses(s, conf.SubmissionOverTlsAddress,
		systemdLs["submission_tls"], smtpsrv.ModeSubmissionTLS)

	if naddr == 0 {
		log.Fatalf("No address to listen on")
	}

	go localrpc.DefaultServer.ListenAndServe(conf.DataDir + "/localrpc-v1")

	go signalHandler(dinfo, s)

	s.ListenAndServe()
}

func loadAddresses(srv *smtpsrv.Server, addrs []string, ls []net.Listener, mode smtpsrv.SocketMode) int {
	naddr := 0
	for _, addr := range addrs {
		// The "systemd" address indicates we get listeners via systemd.
		if addr == "systemd" {
			srv.AddListeners(ls, mode)
			naddr += len(ls)
		} else {
			srv.AddAddr(addr, mode)
			naddr++
		}
	}

	if naddr == 0 {
		log.Errorf("Warning: No %v addresses/listeners", mode)
		log.Errorf("If using systemd, check that you named the sockets")
	}
	return naddr
}

func initMailLog(path string) {
	var err error

	switch path {
	case "<syslog>":
		maillog.Default, err = maillog.NewSyslog()
	case "<stdout>":
		maillog.Default = maillog.New(os.Stdout)
	case "<stderr>":
		maillog.Default = maillog.New(os.Stderr)
	default:
		_ = os.MkdirAll(filepath.Dir(path), 0775)
		maillog.Default, err = maillog.NewFile(path)
	}

	if err != nil {
		log.Fatalf("Error opening mail log: %v", err)
	}
}

func signalHandler(dinfo *domaininfo.DB, srv *smtpsrv.Server) {
	var err error

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

	for {
		switch sig := <-signals; sig {
		case syscall.SIGHUP:
			log.Infof("Received SIGHUP, reloading")

			// SIGHUP triggers a reopen of the log files. This is used for log
			// rotation.
			err = log.Default.Reopen()
			if err != nil {
				log.Fatalf("Error reopening log: %v", err)
			}

			err = maillog.Default.Reopen()
			if err != nil {
				log.Fatalf("Error reopening maillog: %v", err)
			}

			// We don't want to reload the domain info database periodically,
			// as it can be expensive, and it is not expected that the user
			// changes this behind chasquid's back.
			err = dinfo.Reload()
			if err != nil {
				log.Fatalf("Error reloading domain info: %v", err)
			}

			// Also trigger a server reload.
			srv.Reload()
		case syscall.SIGTERM, syscall.SIGINT:
			log.Fatalf("Got signal to exit: %v", sig)
		default:
			log.Errorf("Unexpected signal %v", sig)
		}
	}
}

// Helper to load a single certificate configuration into the server.
func loadCert(name, dir string, s *smtpsrv.Server) {
	log.Infof("  %s", name)

	// Ignore directories that don't have both keys.
	// We warn about this because it can be hard to debug otherwise.
	certPath := filepath.Join(dir, "fullchain.pem")
	if _, err := os.Stat(certPath); err != nil {
		log.Infof("    skipping: %v", err)
		return
	}
	keyPath := filepath.Join(dir, "privkey.pem")
	if _, err := os.Stat(keyPath); err != nil {
		log.Infof("    skipping: %v", err)
		return
	}

	err := s.AddCerts(certPath, keyPath)
	if err != nil {
		log.Fatalf("    %v", err)
	}
}

// Helper to load a single domain configuration into the server.
func loadDomain(name, dir string, s *smtpsrv.Server) {
	log.Infof("  %s", name)
	s.AddDomain(name)

	udb, err := userdb.Load(dir + "/users")
	if os.IsNotExist(err) {
		// No users file present, that's okay.
	} else if err != nil {
		log.Errorf("    users file error: %v", err)
	} else {
		s.AddUserDB(name, udb)
	}

	err = s.AddAliasesFile(name, dir+"/aliases")
	if err != nil {
		log.Errorf("    aliases file error: %v", err)
	}
}

func loadDovecot(s *smtpsrv.Server, userdb, client string) {
	a := dovecot.NewAuth(userdb, client)
	s.SetAuthFallback(a)
	log.Infof("Fallback authenticator: %v", a)

	if err := a.Check(); err != nil {
		log.Errorf("Warning: Dovecot auth is not responding: %v", err)
	}
}

// Read a directory, which must have at least some entries.
func mustReadDir(path string) []os.DirEntry {
	dirs, err := os.ReadDir(path)
	if err != nil {
		log.Fatalf("Error reading %q directory: %v", path, err)
	}
	if len(dirs) == 0 {
		log.Fatalf("No entries found in %q", path)
	}

	return dirs
}
