package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/systemd"
	"blitiri.com.ar/go/chasquid/internal/trace"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	_ "net/http/pprof"

	"github.com/golang/glog"
)

var (
	configDir = flag.String("config_dir", "/etc/chasquid",
		"configuration directory")
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
		glog.Infof("Monitoring HTTP server listening on %s",
			conf.MonitoringAddress)
		go http.ListenAndServe(conf.MonitoringAddress, nil)
	}

	s := NewServer()
	s.Hostname = conf.Hostname
	s.MaxDataSize = conf.MaxDataSizeMb * 1024 * 1024

	s.aliasesR.SuffixSep = conf.SuffixSeparators
	s.aliasesR.DropChars = conf.DropCharacters

	// Load domains.
	// They live inside the config directory, so the relative path works.
	domainDirs, err := ioutil.ReadDir("domains/")
	if err != nil {
		glog.Fatalf("Error reading domains/ directory: %v", err)
	}
	if len(domainDirs) == 0 {
		glog.Fatalf("No domains found in config")
	}

	glog.Infof("Domain config paths:")
	for _, info := range domainDirs {
		name := info.Name()
		dir := filepath.Join("domains", name)
		loadDomain(name, dir, s)
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
	s.AddCerts(dir+"/cert.pem", dir+"/key.pem")

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

	// Certificate and key pairs.
	certs, keys []string

	// Addresses.
	addrs map[SocketMode][]string

	// Listeners (that came via systemd).
	listeners map[SocketMode][]net.Listener

	// TLS config.
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
		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		userDBs:        map[string]*userdb.DB{},
		aliasesR:       aliases.NewResolver(),
	}
}

func (s *Server) AddCerts(cert, key string) {
	s.certs = append(s.certs, cert)
	s.keys = append(s.keys, key)
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

func (s *Server) getTLSConfig() (*tls.Config, error) {
	var err error
	conf := &tls.Config{}

	conf.Certificates = make([]tls.Certificate, len(s.certs))
	for i := 0; i < len(s.certs); i++ {
		conf.Certificates[i], err = tls.LoadX509KeyPair(s.certs[i], s.keys[i])
		if err != nil {
			return nil, err
		}
	}

	conf.BuildNameToCertificate()

	return conf, nil
}

func (s *Server) ListenAndServe() {
	var err error

	// Configure TLS.
	s.tlsConfig, err = s.getTLSConfig()
	if err != nil {
		glog.Fatalf("Error loading TLS config: %v", err)
	}

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

	l.Close()
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

	// System configuration.
	config *config.Config

	// TLS configuration.
	tlsConfig *tls.Config

	// Envelope.
	mailFrom string
	rcptTo   []string
	data     []byte

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

func (c *Conn) Handle() {
	defer c.netconn.Close()

	tr := trace.New("SMTP", "Connection")
	defer tr.Finish()
	tr.LazyPrintf("RemoteAddr: %s", c.netconn.RemoteAddr())

	c.tc.PrintfLine("220 %s ESMTP chasquid", c.hostname)

	var cmd, params string
	var err error

loop:
	for {
		if time.Since(c.deadline) > 0 {
			tr.LazyPrintf("connection deadline exceeded")
			err = fmt.Errorf("connection deadline exceeded")
			break
		}

		c.netconn.SetDeadline(time.Now().Add(c.commandTimeout))

		cmd, params, err = c.readCommand()
		if err != nil {
			c.tc.PrintfLine("554 error reading command: %v", err)
			break
		}

		tr.LazyPrintf("-> %s %s", cmd, params)

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
			code, msg = c.DATA(params, tr)
		case "STARTTLS":
			code, msg = c.STARTTLS(params, tr)
		case "AUTH":
			code, msg = c.AUTH(params, tr)
		case "QUIT":
			c.writeResponse(221, "Be seeing you...")
			break loop
		default:
			code = 500
			msg = "unknown command"
		}

		if code > 0 {
			tr.LazyPrintf("<- %d  %s", code, msg)

			err = c.writeResponse(code, msg)
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		tr.LazyPrintf("exiting with error: %v", err)
		tr.SetError()
	}
}

func (c *Conn) HELO(params string) (code int, msg string) {
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
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, c.hostname+" - Your hour of destiny has come.\n")
	fmt.Fprintf(buf, "8BITMIME\n")
	fmt.Fprintf(buf, "PIPELINING\n")
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
	// "BODY=8BITMIME" (which we ignore).
	// Check that it begins with "FROM:" first, otherwise it's pointless.
	if !strings.HasPrefix(strings.ToLower(params), "from:") {
		return 500, "unknown command"
	}

	addr := ""
	_, err := fmt.Sscanf(params[5:], "%s ", &addr)
	if err != nil {
		return 500, "malformed command - " + err.Error()
	}

	// Special case a null reverse-path, which is explicitly allowed and used
	// for notification messages.
	// It should be written "<>", we check for that and remove spaces just to
	// be more flexible.
	e := &mail.Address{}
	if strings.Replace(addr, " ", "", -1) == "<>" {
		e.Address = "<>"
	} else {
		var err error
		e, err = mail.ParseAddress(addr)
		if err != nil || e.Address == "" {
			return 501, "malformed address"
		}

		if !strings.Contains(e.Address, "@") {
			return 501, "sender address must contain a domain"
		}
	}

	// Note some servers check (and fail) if we had a previous MAIL command,
	// but that's not according to the RFC. We reset the envelope instead.
	c.resetEnvelope()

	// If the source is local, check that it completed auth for that user.
	if e.Address != "<>" && envelope.DomainIn(e.Address, c.localDomains) {
		user, domain := envelope.Split(e.Address)
		if user != c.authUser || domain != c.authDomain {
			return 503, "user not authorized"
		}
	}

	c.mailFrom = e.Address
	return 250, "You feel like you are being watched"
}

func (c *Conn) RCPT(params string) (code int, msg string) {
	// params should be: "TO:<name@host>"
	// First, get rid of the "TO:" part (but check it, it's mandatory).
	sp := strings.SplitN(strings.ToLower(params), ":", 2)
	if len(sp) != 2 || sp[0] != "to" {
		return 500, "unknown command"
	}

	// RFC says 100 is the minimum limit for this, but it seems excessive.
	if len(c.rcptTo) > 100 {
		return 503, "too many recipients"
	}

	// TODO: Write our own parser (we have different needs, mail.ParseAddress
	// is useful for other things).
	// Allow utf8, but prevent "control" characters.

	e, err := mail.ParseAddress(sp[1])
	if err != nil || e.Address == "" {
		return 501, "malformed address"
	}

	if c.mailFrom == "" {
		return 503, "sender not yet given"
	}

	localDst := envelope.DomainIn(e.Address, c.localDomains)
	if !localDst && !c.completedAuth {
		return 503, "relay not allowed"
	}

	if localDst && !c.userExists(e.Address) {
		return 550, "recipient unknown, please check the address for typos"
	}

	c.rcptTo = append(c.rcptTo, e.Address)
	return 250, "You have an eerie feeling..."
}

func (c *Conn) DATA(params string, tr *trace.Trace) (code int, msg string) {
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

	tr.LazyPrintf("<- 354  You experience a strange sense of peace")

	// Increase the deadline for the data transfer to the connection-level
	// one, we don't want the command timeout to interfere.
	c.netconn.SetDeadline(c.deadline)

	dotr := io.LimitReader(c.tc.DotReader(), c.maxDataSize)
	c.data, err = ioutil.ReadAll(dotr)
	if err != nil {
		return 554, fmt.Sprintf("error reading DATA: %v", err)
	}

	tr.LazyPrintf("-> ... %d bytes of data", len(c.data))

	c.addReceivedHeader()

	// There are no partial failures here: we put it in the queue, and then if
	// individual deliveries fail, we report via email.
	msgID, err := c.queue.Put(c.hostname, c.mailFrom, c.rcptTo, c.data)
	if err != nil {
		tr.LazyPrintf("   error queueing: %v", err)
		tr.SetError()
		return 554, fmt.Sprintf("Failed to enqueue message: %v", err)
	}

	tr.LazyPrintf("   ... queued: %q", msgID)

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

	if c.completedAuth {
		v += fmt.Sprintf("from user %s@%s\n", c.authUser, c.authDomain)
	} else {
		v += fmt.Sprintf("from %s\n", c.netconn.RemoteAddr().String())
	}

	v += fmt.Sprintf("by %s (chasquid SMTP) over ", c.hostname)
	if c.tlsConnState != nil {
		v += fmt.Sprintf("TLS (%#x-%#x)\n",
			c.tlsConnState.Version, c.tlsConnState.CipherSuite)
	} else {
		v += "plain text!\n"
	}

	// Note we must NOT include c.rcptTo, that would leak BCCs.
	v += fmt.Sprintf("(envelope from %q)\n", c.mailFrom)

	// This should be the last part in the Received header, by RFC.
	// The ";" is a mandatory separator. The date format is not standard but
	// this one seems to be widely used.
	// https://tools.ietf.org/html/rfc5322#section-3.6.7
	v += fmt.Sprintf("on ; %s\n", time.Now().Format(time.RFC1123Z))
	c.data = envelope.AddHeader(c.data, "Received", v)
}

func (c *Conn) STARTTLS(params string, tr *trace.Trace) (code int, msg string) {
	if c.onTLS {
		return 503, "You are already wearing that!"
	}

	err := c.writeResponse(220, "You experience a strange sense of peace")
	if err != nil {
		return 554, fmt.Sprintf("error writing STARTTLS response: %v", err)
	}

	tr.LazyPrintf("<- 220  You experience a strange sense of peace")

	server := tls.Server(c.netconn, c.tlsConfig)
	err = server.Handshake()
	if err != nil {
		return 554, fmt.Sprintf("error in TLS handshake: %v", err)
	}

	tr.LazyPrintf("<> ...  jump to TLS was successful")

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

func (c *Conn) AUTH(params string, tr *trace.Trace) (code int, msg string) {
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
		return 235, ""
	}

	return 535, "Incorrect user or password"
}

func (c *Conn) resetEnvelope() {
	c.mailFrom = ""
	c.rcptTo = nil
	c.data = nil
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
