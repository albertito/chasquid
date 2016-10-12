package main

import (
	"bytes"
	"crypto/tls"
	"expvar"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/spf"
	"blitiri.com.ar/go/chasquid/internal/systemd"
	"blitiri.com.ar/go/chasquid/internal/tlsconst"
	"blitiri.com.ar/go/chasquid/internal/trace"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
)

// Command-line flags.
var (
	configDir = flag.String("config_dir", "/etc/chasquid",
		"configuration directory")
)

// Exported variables.
var (
	commandCount      = expvar.NewMap("chasquid/smtpIn/commandCount")
	responseCodeCount = expvar.NewMap("chasquid/smtpIn/responseCodeCount")
	spfResultCount    = expvar.NewMap("chasquid/smtpIn/spfResultCount")
	loopsDetected     = expvar.NewInt("chasquid/smtpIn/loopsDetected")
	tlsCount          = expvar.NewMap("chasquid/smtpIn/tlsCount")
)

// Global event logs.
var (
	authLog = trace.NewEventLog("Authentication", "Incoming SMTP")
)

func main() {
	flag.Parse()

	setupSignalHandling()

	defer glog.Flush()
	go periodicallyFlushLogs()

	// Seed the PRNG, just to prevent for it to be totally predictable.
	rand.Seed(time.Now().UnixNano())

	conf, err := config.Load(*configDir + "/chasquid.conf")
	if err != nil {
		glog.Fatalf("Error reading config")
	}
	config.LogConfig(conf)

	// Change to the config dir.
	// This allow us to use relative paths for configuration directories.
	// It also can be useful in unusual environments and for testing purposes,
	// where paths inside the configuration itself could be relative, and this
	// fixes the point of reference.
	os.Chdir(*configDir)

	if conf.MonitoringAddress != "" {
		launchMonitoringServer(conf.MonitoringAddress)
	}

	s := NewServer()
	s.Hostname = conf.Hostname
	s.MaxDataSize = conf.MaxDataSizeMb * 1024 * 1024

	s.aliasesR.SuffixSep = conf.SuffixSeparators
	s.aliasesR.DropChars = conf.DropCharacters

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

	localC := &courier.Procmail{
		Binary:  conf.MailDeliveryAgentBin,
		Args:    conf.MailDeliveryAgentArgs,
		Timeout: 30 * time.Second,
	}
	remoteC := &courier.SMTP{}
	s.InitQueue(conf.DataDir+"/queue", localC, remoteC)

	go s.periodicallyReload()

	// Load the addresses and listeners.
	systemdLs, err := systemd.Listeners()
	if err != nil {
		glog.Fatalf("Error getting systemd listeners: %v", err)
	}

	loadAddresses(s, conf.SmtpAddress, systemdLs["smtp"], ModeSMTP)
	loadAddresses(s, conf.SubmissionAddress, systemdLs["submission"], ModeSubmission)

	s.ListenAndServe()
}

func loadAddresses(srv *Server, addrs []string, ls []net.Listener, mode SocketMode) {
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

// Helper to load a single domain configuration into the server.
func loadDomain(name, dir string, s *Server) {
	glog.Infof("  %s", name)
	s.AddDomain(name)
	s.aliasesR.AddDomain(name)

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
	err := s.aliasesR.AddAliasesFile(name, dir+"/aliases")
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

// Mode for a socket (listening or connection).
// We keep them distinct, as policies can differ between them.
type SocketMode string

const (
	ModeSMTP       SocketMode = "SMTP"
	ModeSubmission SocketMode = "Submission"
)

type Server struct {
	// Main hostname, used for display only.
	Hostname string

	// Maximum data size.
	MaxDataSize int64

	// Addresses.
	addrs map[SocketMode][]string

	// Listeners (that came via systemd).
	listeners map[SocketMode][]net.Listener

	// TLS config (including loaded certificates).
	tlsConfig *tls.Config

	// Local domains.
	localDomains *set.String

	// User databases (per domain).
	userDBs map[string]*userdb.DB

	// Aliases resolver.
	aliasesR *aliases.Resolver

	// Time before we give up on a connection, even if it's sending data.
	connTimeout time.Duration

	// Time we wait for command round-trips (excluding DATA).
	commandTimeout time.Duration

	// Queue where we put incoming mail.
	queue *queue.Queue
}

func NewServer() *Server {
	return &Server{
		addrs:          map[SocketMode][]string{},
		listeners:      map[SocketMode][]net.Listener{},
		tlsConfig:      &tls.Config{},
		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		userDBs:        map[string]*userdb.DB{},
		aliasesR:       aliases.NewResolver(),
	}
}

func (s *Server) AddCerts(certPath, keyPath string) error {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return err
	}
	s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, cert)
	return nil
}

func (s *Server) AddAddr(a string, m SocketMode) {
	s.addrs[m] = append(s.addrs[m], a)
}

func (s *Server) AddListeners(ls []net.Listener, m SocketMode) {
	s.listeners[m] = append(s.listeners[m], ls...)
}

func (s *Server) AddDomain(d string) {
	s.localDomains.Add(d)
}

func (s *Server) AddUserDB(domain string, db *userdb.DB) {
	s.userDBs[domain] = db
}

func (s *Server) InitQueue(path string, localC, remoteC courier.Courier) {
	q := queue.New(path, s.localDomains, s.aliasesR, localC, remoteC)
	err := q.Load()
	if err != nil {
		glog.Fatalf("Error loading queue: %v", err)
	}
	s.queue = q

	http.HandleFunc("/debug/queue",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(q.DumpString()))
		})
}

// periodicallyReload some of the server's information, such as aliases and
// the user databases.
func (s *Server) periodicallyReload() {
	for range time.Tick(30 * time.Second) {
		err := s.aliasesR.Reload()
		if err != nil {
			glog.Errorf("Error reloading aliases: %v", err)
		}

		for domain, udb := range s.userDBs {
			err = udb.Reload()
			if err != nil {
				glog.Errorf("Error reloading %q user db: %v", domain, err)
			}
		}
	}
}

func (s *Server) ListenAndServe() {
	// At this point the TLS config should be done, build the
	// name->certificate map (used by the TLS library for SNI).
	s.tlsConfig.BuildNameToCertificate()

	for m, addrs := range s.addrs {
		for _, addr := range addrs {
			// Listen.
			l, err := net.Listen("tcp", addr)
			if err != nil {
				glog.Fatalf("Error listening: %v", err)
			}

			glog.Infof("Server listening on %s (%v)", addr, m)

			// Serve.
			go s.serve(l, m)
		}
	}

	for m, ls := range s.listeners {
		for _, l := range ls {
			glog.Infof("Server listening on %s (%v, via systemd)", l.Addr(), m)

			// Serve.
			go s.serve(l, m)
		}
	}

	// Never return. If the serve goroutines have problems, they will abort
	// execution.
	for {
		time.Sleep(24 * time.Hour)
	}
}

func (s *Server) serve(l net.Listener, mode SocketMode) {
	for {
		conn, err := l.Accept()
		if err != nil {
			glog.Fatalf("Error accepting: %v", err)
		}

		sc := &Conn{
			hostname:       s.Hostname,
			maxDataSize:    s.MaxDataSize,
			netconn:        conn,
			tc:             textproto.NewConn(conn),
			mode:           mode,
			tlsConfig:      s.tlsConfig,
			userDBs:        s.userDBs,
			aliasesR:       s.aliasesR,
			localDomains:   s.localDomains,
			deadline:       time.Now().Add(s.connTimeout),
			commandTimeout: s.commandTimeout,
			queue:          s.queue,
		}
		go sc.Handle()
	}
}

type Conn struct {
	// Main hostname, used for display only.
	hostname string

	// Maximum data size.
	maxDataSize int64

	// Connection information.
	netconn      net.Conn
	tc           *textproto.Conn
	mode         SocketMode
	tlsConnState *tls.ConnectionState

	// Tracer to use.
	tr *trace.Trace

	// System configuration.
	config *config.Config

	// TLS configuration.
	tlsConfig *tls.Config

	// Address given at HELO/EHLO, used for tracing purposes.
	ehloAddress string

	// Envelope.
	mailFrom string
	rcptTo   []string
	data     []byte

	// SPF results.
	spfResult spf.Result
	spfError  error

	// Are we using TLS?
	onTLS bool

	// User databases, aliases and local domains, taken from the server at
	// creation time.
	userDBs      map[string]*userdb.DB
	localDomains *set.String
	aliasesR     *aliases.Resolver

	// Have we successfully completed AUTH?
	completedAuth bool

	// How many times have we attempted AUTH?
	authAttempts int

	// Authenticated user and domain, empty if !completedAuth.
	authUser   string
	authDomain string

	// When we should close this connection, no matter what.
	deadline time.Time

	// Queue where we put incoming mails.
	queue *queue.Queue

	// Time we wait for network operations.
	commandTimeout time.Duration
}

func (c *Conn) Close() {
	c.netconn.Close()
}

func (c *Conn) Handle() {
	defer c.Close()

	c.tr = trace.New("SMTP.Conn", c.netconn.RemoteAddr().String())
	defer c.tr.Finish()

	c.tc.PrintfLine("220 %s ESMTP chasquid", c.hostname)

	var cmd, params string
	var err error
	var errCount int

loop:
	for {
		if time.Since(c.deadline) > 0 {
			err = fmt.Errorf("connection deadline exceeded")
			c.tr.Error(err)
			break
		}

		c.netconn.SetDeadline(time.Now().Add(c.commandTimeout))

		cmd, params, err = c.readCommand()
		if err != nil {
			c.tc.PrintfLine("554 error reading command: %v", err)
			break
		}

		commandCount.Add(cmd, 1)
		if cmd == "AUTH" {
			c.tr.Debugf("-> AUTH <redacted>")
		} else {
			c.tr.Debugf("-> %s %s", cmd, params)
		}

		var code int
		var msg string

		switch cmd {
		case "HELO":
			code, msg = c.HELO(params)
		case "EHLO":
			code, msg = c.EHLO(params)
		case "HELP":
			code, msg = c.HELP(params)
		case "NOOP":
			code, msg = c.NOOP(params)
		case "RSET":
			code, msg = c.RSET(params)
		case "VRFY":
			code, msg = c.VRFY(params)
		case "EXPN":
			code, msg = c.EXPN(params)
		case "MAIL":
			code, msg = c.MAIL(params)
		case "RCPT":
			code, msg = c.RCPT(params)
		case "DATA":
			// DATA handles the whole sequence.
			code, msg = c.DATA(params)
		case "STARTTLS":
			code, msg = c.STARTTLS(params)
		case "AUTH":
			code, msg = c.AUTH(params)
		case "QUIT":
			c.writeResponse(221, "Be seeing you...")
			break loop
		default:
			code = 500
			msg = "unknown command"
		}

		if code > 0 {
			c.tr.Debugf("<- %d  %s", code, msg)

			if code >= 400 {
				// Be verbose about errors, to help troubleshooting.
				c.tr.Errorf("%s failed: %d  %s", cmd, code, msg)

				errCount++
				if errCount > 10 {
					// https://tools.ietf.org/html/rfc5321#section-4.3.2
					c.tr.Errorf("too many errors, breaking connection")
					c.writeResponse(421, "too many errors, bye")
					break
				}
			}

			err = c.writeResponse(code, msg)
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		c.tr.Errorf("exiting with error: %v", err)
	}
}

func (c *Conn) HELO(params string) (code int, msg string) {
	if len(strings.TrimSpace(params)) == 0 {
		return 501, "Invisible customers are not welcome!"
	}
	c.ehloAddress = strings.Fields(params)[0]

	types := []string{
		"general store", "used armor dealership", "second-hand bookstore",
		"liquor emporium", "antique weapons outlet", "delicatessen",
		"jewelers", "quality apparel and accessories", "hardware",
		"rare books", "lighting store"}
	t := types[rand.Int()%len(types)]
	msg = fmt.Sprintf("Hello my friend, welcome to chasqui's %s!", t)

	return 250, msg
}

func (c *Conn) EHLO(params string) (code int, msg string) {
	if len(strings.TrimSpace(params)) == 0 {
		return 501, "Invisible customers are not welcome!"
	}
	c.ehloAddress = strings.Fields(params)[0]

	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, c.hostname+" - Your hour of destiny has come.\n")
	fmt.Fprintf(buf, "8BITMIME\n")
	fmt.Fprintf(buf, "PIPELINING\n")
	fmt.Fprintf(buf, "SMTPUTF8\n")
	fmt.Fprintf(buf, "SIZE %d\n", c.maxDataSize)
	if c.onTLS {
		fmt.Fprintf(buf, "AUTH PLAIN\n")
	} else {
		fmt.Fprintf(buf, "STARTTLS\n")
	}
	fmt.Fprintf(buf, "HELP\n")
	return 250, buf.String()
}

func (c *Conn) HELP(params string) (code int, msg string) {
	return 214, "hoy por ti, mañana por mi"
}

func (c *Conn) RSET(params string) (code int, msg string) {
	c.resetEnvelope()

	msgs := []string{
		"Who was that Maud person anyway?",
		"Thinking of Maud you forget everything else.",
		"Your mind releases itself from mundane concerns.",
		"As your mind turns inward on itself, you forget everything else.",
	}
	return 250, msgs[rand.Int()%len(msgs)]
}

func (c *Conn) VRFY(params string) (code int, msg string) {
	// 252 can be used for cases like ours, when we don't really want to
	// confirm or deny anything.
	// See https://tools.ietf.org/html/rfc2821#section-3.5.3.
	return 252, "You have a strange feeling for a moment, then it passes."
}

func (c *Conn) EXPN(params string) (code int, msg string) {
	// 252 can be used for cases like ours, when we don't really want to
	// confirm or deny anything.
	// See https://tools.ietf.org/html/rfc2821#section-3.5.3.
	return 252, "You feel disoriented for a moment."
}

func (c *Conn) NOOP(params string) (code int, msg string) {
	return 250, "You hear a faint typing noise."
}

func (c *Conn) MAIL(params string) (code int, msg string) {
	// params should be: "FROM:<name@host>", and possibly followed by
	// options such as "BODY=8BITMIME" (which we ignore).
	// Check that it begins with "FROM:" first, it's mandatory.
	if !strings.HasPrefix(strings.ToLower(params), "from:") {
		return 500, "unknown command"
	}

	rawAddr := ""
	_, err := fmt.Sscanf(params[5:], "%s ", &rawAddr)
	if err != nil {
		return 500, "malformed command - " + err.Error()
	}

	// Note some servers check (and fail) if we had a previous MAIL command,
	// but that's not according to the RFC. We reset the envelope instead.
	c.resetEnvelope()

	// Special case a null reverse-path, which is explicitly allowed and used
	// for notification messages.
	// It should be written "<>", we check for that and remove spaces just to
	// be more flexible.
	addr := ""
	if strings.Replace(rawAddr, " ", "", -1) == "<>" {
		addr = "<>"
	} else {
		e, err := mail.ParseAddress(rawAddr)
		if err != nil || e.Address == "" {
			return 501, "malformed address"
		}
		addr = e.Address

		if !strings.Contains(addr, "@") {
			return 501, "sender address must contain a domain"
		}

		// https://tools.ietf.org/html/rfc5321#section-4.5.3.1.3
		if len(addr) > 256 {
			return 501, "address too long"
		}

		// SPF check - https://tools.ietf.org/html/rfc7208#section-2.4
		// We opt not to fail on errors, to avoid accidents from preventing
		// delivery.
		c.spfResult, c.spfError = c.checkSPF(addr)
		if c.spfResult == spf.Fail {
			// https://tools.ietf.org/html/rfc7208#section-8.4
			return 550, fmt.Sprintf(
				"SPF check failed: %v", c.spfError)
		}

		addr, err = normalize.DomainToUnicode(addr)
		if err != nil {
			return 501, "malformed address (IDNA conversion failed)"
		}
	}

	c.mailFrom = addr
	return 250, "You feel like you are being watched"
}

// checkSPF for the given address, based on the current connection.
func (c *Conn) checkSPF(addr string) (spf.Result, error) {
	// Does not apply to authenticated connections, they're allowed regardless.
	if c.completedAuth {
		return "", nil
	}

	if tcp, ok := c.netconn.RemoteAddr().(*net.TCPAddr); ok {
		res, err := spf.CheckHost(
			tcp.IP, envelope.DomainOf(addr))

		c.tr.Debugf("SPF %v (%v)", res, err)
		spfResultCount.Add(string(res), 1)

		return res, err
	}

	return "", nil
}

func (c *Conn) RCPT(params string) (code int, msg string) {
	// params should be: "TO:<name@host>", and possibly followed by options
	// such as "NOTIFY=SUCCESS,DELAY" (which we ignore).
	// Check that it begins with "TO:" first, it's mandatory.
	if !strings.HasPrefix(strings.ToLower(params), "to:") {
		return 500, "unknown command"
	}

	if c.mailFrom == "" {
		return 503, "sender not yet given"
	}

	rawAddr := ""
	_, err := fmt.Sscanf(params[3:], "%s ", &rawAddr)
	if err != nil {
		return 500, "malformed command - " + err.Error()
	}

	// RFC says 100 is the minimum limit for this, but it seems excessive.
	// https://tools.ietf.org/html/rfc5321#section-4.5.3.1.8
	if len(c.rcptTo) > 100 {
		return 452, "too many recipients"
	}

	e, err := mail.ParseAddress(rawAddr)
	if err != nil || e.Address == "" {
		return 501, "malformed address"
	}

	addr, err := normalize.DomainToUnicode(e.Address)
	if err != nil {
		return 501, "malformed address (IDNA conversion failed)"
	}

	// https://tools.ietf.org/html/rfc5321#section-4.5.3.1.3
	if len(addr) > 256 {
		return 501, "address too long"
	}

	localDst := envelope.DomainIn(addr, c.localDomains)
	if !localDst && !c.completedAuth {
		return 503, "relay not allowed"
	}

	if localDst {
		addr, err = normalize.Addr(addr)
		if err != nil {
			return 550, "recipient invalid, please check the address for typos"
		}

		if !c.userExists(addr) {
			return 550, "recipient unknown, please check the address for typos"
		}
	}

	c.rcptTo = append(c.rcptTo, addr)
	return 250, "You have an eerie feeling..."
}

func (c *Conn) DATA(params string) (code int, msg string) {
	if c.ehloAddress == "" {
		return 503, "Invisible customers are not welcome!"
	}
	if c.mailFrom == "" {
		return 503, "sender not yet given"
	}

	if len(c.rcptTo) == 0 {
		return 503, "need an address to send to"
	}

	// We're going ahead.
	err := c.writeResponse(354, "You suddenly realize it is unnaturally quiet")
	if err != nil {
		return 554, fmt.Sprintf("error writing DATA response: %v", err)
	}

	c.tr.Debugf("<- 354  You experience a strange sense of peace")
	if c.onTLS {
		tlsCount.Add("tls", 1)
	} else {
		tlsCount.Add("plain", 1)
	}

	// Increase the deadline for the data transfer to the connection-level
	// one, we don't want the command timeout to interfere.
	c.netconn.SetDeadline(c.deadline)

	dotr := io.LimitReader(c.tc.DotReader(), c.maxDataSize)
	c.data, err = ioutil.ReadAll(dotr)
	if err != nil {
		return 554, fmt.Sprintf("error reading DATA: %v", err)
	}

	c.tr.Debugf("-> ... %d bytes of data", len(c.data))

	if err := checkData(c.data); err != nil {
		return 554, err.Error()
	}

	c.addReceivedHeader()

	// There are no partial failures here: we put it in the queue, and then if
	// individual deliveries fail, we report via email.
	msgID, err := c.queue.Put(c.hostname, c.mailFrom, c.rcptTo, c.data)
	if err != nil {
		return 554, fmt.Sprintf("Failed to enqueue message: %v", err)
	}

	c.tr.Printf("Queued from %s to %s - %s", c.mailFrom, c.rcptTo, msgID)

	// It is very important that we reset the envelope before returning,
	// so clients can send other emails right away without needing to RSET.
	c.resetEnvelope()

	msgs := []string{
		"You offer the Amulet of Yendor to Anhur...",
		"An invisible choir sings, and you are bathed in radiance...",
		"The voice of Anhur booms out: Congratulations, mortal!",
		"In return to thy service, I grant thee the gift of Immortality!",
		"You ascend to the status of Demigod(dess)...",
	}
	return 250, msgs[rand.Int()%len(msgs)]
}

func (c *Conn) addReceivedHeader() {
	var v string

	// Format is semi-structured, defined by
	// https://tools.ietf.org/html/rfc5321#section-4.4

	if c.completedAuth {
		v += fmt.Sprintf("from %s (authenticated as %s@%s)\n",
			c.ehloAddress, c.authUser, c.authDomain)
	} else {
		v += fmt.Sprintf("from %s (%s)\n",
			c.ehloAddress, c.netconn.RemoteAddr().String())
	}

	v += fmt.Sprintf("by %s (chasquid)\n", c.hostname)

	v += "(over "
	if c.tlsConnState != nil {
		v += fmt.Sprintf("%s-%s)\n",
			tlsconst.VersionName(c.tlsConnState.Version),
			tlsconst.CipherSuiteName(c.tlsConnState.CipherSuite))
	} else {
		v += "plain text!)\n"
	}

	// Note we must NOT include c.rcptTo, that would leak BCCs.
	v += fmt.Sprintf("(envelope from %q)\n", c.mailFrom)

	// This should be the last part in the Received header, by RFC.
	// The ";" is a mandatory separator. The date format is not standard but
	// this one seems to be widely used.
	// https://tools.ietf.org/html/rfc5322#section-3.6.7
	v += fmt.Sprintf("; %s\n", time.Now().Format(time.RFC1123Z))
	c.data = envelope.AddHeader(c.data, "Received", v)

	if c.spfResult != "" {
		// https://tools.ietf.org/html/rfc7208#section-9.1
		v = fmt.Sprintf("%s (%v)", c.spfResult, c.spfError)
		c.data = envelope.AddHeader(c.data, "Received-SPF", v)
	}
}

// checkData performs very basic checks on the body of the email, to help
// detect very broad problems like email loops. It does not fully check the
// sanity of the headers or the structure of the payload.
func checkData(data []byte) error {
	msg, err := mail.ReadMessage(bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error parsing message: %v", err)
	}

	// This serves as a basic form of loop prevention. It's not infallible but
	// should catch most instances of accidental looping.
	// https://tools.ietf.org/html/rfc5321#section-6.3
	if len(msg.Header["Received"]) > 50 {
		loopsDetected.Add(1)
		return fmt.Errorf("email passed through more than 50 MTAs, looping?")
	}

	return nil
}

func (c *Conn) STARTTLS(params string) (code int, msg string) {
	if c.onTLS {
		return 503, "You are already wearing that!"
	}

	err := c.writeResponse(220, "You experience a strange sense of peace")
	if err != nil {
		return 554, fmt.Sprintf("error writing STARTTLS response: %v", err)
	}

	c.tr.Debugf("<- 220  You experience a strange sense of peace")

	server := tls.Server(c.netconn, c.tlsConfig)
	err = server.Handshake()
	if err != nil {
		return 554, fmt.Sprintf("error in TLS handshake: %v", err)
	}

	c.tr.Debugf("<> ...  jump to TLS was successful")

	// Override the connections. We don't need the older ones anymore.
	c.netconn = server
	c.tc = textproto.NewConn(server)

	// Take the connection state, so we can use it later for logging and
	// tracing purposes.
	cstate := server.ConnectionState()
	c.tlsConnState = &cstate

	// Reset the envelope; clients must start over after switching to TLS.
	c.resetEnvelope()

	c.onTLS = true

	// If the client requested a specific server and we complied, that's our
	// identity from now on.
	if name := c.tlsConnState.ServerName; name != "" {
		c.hostname = name
	}

	// 0 indicates not to send back a reply.
	return 0, ""
}

func (c *Conn) AUTH(params string) (code int, msg string) {
	if !c.onTLS {
		return 503, "You feel vulnerable"
	}

	if c.completedAuth {
		// After a successful AUTH command completes, a server MUST reject
		// any further AUTH commands with a 503 reply.
		// https://tools.ietf.org/html/rfc4954#section-4
		return 503, "You are already wearing that!"
	}

	if c.authAttempts > 3 {
		// TODO: close the connection?
		return 503, "Too many attempts - go away"
	}
	c.authAttempts++

	// We only support PLAIN for now, so no need to make this too complicated.
	// Params should be either "PLAIN" or "PLAIN <response>".
	// If the response is not there, we reply with 334, and expect the
	// response back from the client in the next message.

	sp := strings.SplitN(params, " ", 2)
	if len(sp) < 1 || sp[0] != "PLAIN" {
		// As we only offer plain, this should not really happen.
		return 534, "Asmodeus demands 534 zorkmids for safe passage"
	}

	// Note we use more "serious" error messages from now own, as these may
	// find their way to the users in some circumstances.

	// Get the response, either from the message or interactively.
	response := ""
	if len(sp) == 2 {
		response = sp[1]
	} else {
		// Reply 334 and expect the user to provide it.
		// In this case, the text IS relevant, as it is taken as the
		// server-side SASL challenge (empty for PLAIN).
		// https://tools.ietf.org/html/rfc4954#section-4
		err := c.writeResponse(334, "")
		if err != nil {
			return 554, fmt.Sprintf("error writing AUTH 334: %v", err)
		}

		response, err = c.readLine()
		if err != nil {
			return 554, fmt.Sprintf("error reading AUTH response: %v", err)
		}
	}

	user, domain, passwd, err := auth.DecodeResponse(response)
	if err != nil {
		return 535, fmt.Sprintf("error decoding AUTH response: %v", err)
	}

	if auth.Authenticate(c.userDBs[domain], user, passwd) {
		c.authUser = user
		c.authDomain = domain
		c.completedAuth = true
		authLog.Debugf("%s successful for %s@%s",
			c.netconn.RemoteAddr().String(), user, domain)
		return 235, ""
	}

	authLog.Debugf("%s failed for %s@%s",
		c.netconn.RemoteAddr().String(), user, domain)
	return 535, "Incorrect user or password"
}

func (c *Conn) resetEnvelope() {
	c.mailFrom = ""
	c.rcptTo = nil
	c.data = nil
	c.spfResult = ""
	c.spfError = nil
}

func (c *Conn) userExists(addr string) bool {
	var ok bool
	addr, ok = c.aliasesR.Exists(addr)
	if ok {
		return true
	}

	// Note we used the address returned by the aliases resolver, which has
	// cleaned it up. This means that a check for "us.er@domain" will have us
	// look up "user" in our databases if the domain is local, which is what
	// we want.
	user, domain := envelope.Split(addr)
	udb := c.userDBs[domain]
	if udb == nil {
		return false
	}
	return udb.HasUser(user)
}

func (c *Conn) readCommand() (cmd, params string, err error) {
	var msg string

	msg, err = c.tc.ReadLine()
	if err != nil {
		return "", "", err
	}

	sp := strings.SplitN(msg, " ", 2)
	cmd = strings.ToUpper(sp[0])
	if len(sp) > 1 {
		params = sp[1]
	}

	return cmd, params, err
}

func (c *Conn) readLine() (line string, err error) {
	return c.tc.ReadLine()
}

func (c *Conn) writeResponse(code int, msg string) error {
	defer c.tc.W.Flush()

	responseCodeCount.Add(strconv.Itoa(code), 1)
	return writeResponse(c.tc.W, code, msg)
}

// writeResponse writes a multi-line response to the given writer.
// This is the writing version of textproto.Reader.ReadResponse().
func writeResponse(w io.Writer, code int, msg string) error {
	var i int
	lines := strings.Split(msg, "\n")

	// The first N-1 lines use "<code>-<text>".
	for i = 0; i < len(lines)-2; i++ {
		_, err := w.Write([]byte(fmt.Sprintf("%d-%s\r\n", code, lines[i])))
		if err != nil {
			return err
		}
	}

	// The last line uses "<code> <text>".
	_, err := w.Write([]byte(fmt.Sprintf("%d %s\r\n", code, lines[i])))
	if err != nil {
		return err
	}

	return nil
}

func launchMonitoringServer(addr string) {
	glog.Infof("Monitoring HTTP server listening on %s", addr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(monitoringHTMLIndex))
	})

	flags := dumpFlags()
	http.HandleFunc("/debug/flags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(flags))
	})

	go http.ListenAndServe(addr, nil)
}

// Static index for the monitoring website.
const monitoringHTMLIndex = `<!DOCTYPE html>
<html>
  <head>
    <title>chasquid monitoring</title>
  </head>
  <body>
    <h1>chasquid monitoring</h1>
    <ul>
      <li><a href="/debug/queue">queue</a>
      <li><a href="/debug/requests">requests</a>
          <small><a href="https://godoc.org/golang.org/x/net/trace">
            (ref)</a></small>
      <li><a href="/debug/flags">flags</a>
      <li><a href="/debug/vars">public variables</a>
      <li><a href="/debug/pprof">pprof</a>
          <small><a href="https://golang.org/pkg/net/http/pprof/">
            (ref)</a></small>
        <ul>
          <li><a href="/debug/pprof/goroutine?debug=1">goroutines</a>
        </ul>
    </ul>
  </body>
</html>
`

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
