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
	"path/filepath"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/courier"
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

	testCert = flag.String("test_cert", ".cert.pem",
		"Certificate file, for testing purposes")
	testKey = flag.String("test_key", ".key.pem",
		"Key file, for testing purposes")
)

func main() {
	flag.Parse()

	// Seed the PRNG, just to prevent for it to be totally predictable.
	rand.Seed(time.Now().UnixNano())

	conf, err := config.Load(*configDir + "/chasquid.conf")
	if err != nil {
		glog.Fatalf("Error reading config")
	}

	if conf.MonitoringAddress != "" {
		glog.Infof("Monitoring HTTP server listening on %s",
			conf.MonitoringAddress)
		go http.ListenAndServe(conf.MonitoringAddress, nil)
	}

	courier.MailDeliveryAgentBin = conf.MailDeliveryAgentBin
	courier.MailDeliveryAgentArgs = conf.MailDeliveryAgentArgs

	s := NewServer()
	s.Hostname = conf.Hostname
	s.MaxDataSize = conf.MaxDataSizeMb * 1024 * 1024

	// Load domains.
	domainDirs, err := ioutil.ReadDir(*configDir + "/domains/")
	if err != nil {
		glog.Fatalf("Error in glob: %v", err)
	}
	if len(domainDirs) == 0 {
		glog.Warningf("No domains found in config, using test certs")
		s.AddCerts(*testCert, *testKey)
	} else {
		glog.Infof("Domain config paths:")
		for _, info := range domainDirs {
			name := info.Name()
			dir := filepath.Join(*configDir, "domains", name)
			loadDomain(s, name, dir)
		}
	}

	// Always include localhost as local domain.
	// This can prevent potential trouble if we were to accidentally treat it
	// as a remote domain (for loops, alias resolutions, etc.).
	s.AddDomain("localhost")

	// Load addresses.
	acount := 0
	for _, addr := range conf.Address {
		// The "systemd" address indicates we get listeners via systemd.
		if addr == "systemd" {
			ls, err := systemd.Listeners()
			if err != nil {
				glog.Fatalf("Error getting listeners via systemd: %v", err)
			}
			s.AddListeners(ls)
			acount += len(ls)
		} else {
			s.AddAddr(addr)
			acount++
		}
	}

	if acount == 0 {
		glog.Errorf("No addresses/listeners configured")
		glog.Errorf("If using systemd, check that you started chasquid.socket")
		glog.Fatalf("Exiting")
	}

	s.ListenAndServe()
}

// Helper to load a single domain configuration into the server.
func loadDomain(s *Server, name, dir string) {
	glog.Infof("  %s", name)
	s.AddDomain(name)
	s.AddCerts(dir+"/cert.pem", dir+"/key.pem")

	if _, err := os.Stat(dir + "/users"); err == nil {
		glog.Infof("    adding users")
		udb, warnings, err := userdb.Load(dir + "/users")
		if err != nil {
			glog.Errorf("      error: %v", err)
		} else {
			for _, w := range warnings {
				glog.Warningf("     %v", w)
			}
			s.AddUserDB(name, udb)
			// TODO: periodically reload the database.
		}
	}
}

type Server struct {
	// Main hostname, used for display only.
	Hostname string

	// Maximum data size.
	MaxDataSize int64

	// Certificate and key pairs.
	certs, keys []string

	// Addresses.
	addrs []string

	// Listeners (that came via systemd).
	listeners []net.Listener

	// TLS config.
	tlsConfig *tls.Config

	// Local domains.
	localDomains *set.String

	// User databases (per domain).
	userDBs map[string]*userdb.DB

	// Time before we give up on a connection, even if it's sending data.
	connTimeout time.Duration

	// Time we wait for command round-trips (excluding DATA).
	commandTimeout time.Duration

	// Queue where we put incoming mail.
	queue *queue.Queue
}

func NewServer() *Server {
	return &Server{
		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		userDBs:        map[string]*userdb.DB{},
	}
}

func (s *Server) AddCerts(cert, key string) {
	s.certs = append(s.certs, cert)
	s.keys = append(s.keys, key)
}

func (s *Server) AddAddr(a string) {
	s.addrs = append(s.addrs, a)
}

func (s *Server) AddListeners(ls []net.Listener) {
	s.listeners = append(s.listeners, ls...)
}

func (s *Server) AddDomain(d string) {
	s.localDomains.Add(d)
}

func (s *Server) AddUserDB(domain string, db *userdb.DB) {
	s.userDBs[domain] = db
}

func (s *Server) getTLSConfig() (*tls.Config, error) {
	var err error
	conf := &tls.Config{}

	conf.Certificates = make([]tls.Certificate, len(s.certs))
	for i := 0; i < len(s.certs); i++ {
		conf.Certificates[i], err = tls.LoadX509KeyPair(s.certs[i], s.keys[i])
		if err != nil {
			return nil, fmt.Errorf("Error loading client certificate: %v", err)
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

	// TODO: Create the queue when creating the server?
	// Or even before, and just give it to the server?
	s.queue = queue.New(
		&courier.Procmail{}, &courier.SMTP{}, s.localDomains)

	for _, addr := range s.addrs {
		// Listen.
		l, err := net.Listen("tcp", addr)
		if err != nil {
			glog.Fatalf("Error listening: %v", err)
		}
		defer l.Close()

		glog.Infof("Server listening on %s", addr)

		// Serve.
		go s.serve(l)
	}

	for _, l := range s.listeners {
		defer l.Close()
		glog.Infof("Server listening on %s (via systemd)", l.Addr())

		// Serve.
		go s.serve(l)
	}

	// Never return. If the serve goroutines have problems, they will abort
	// execution.
	for {
		time.Sleep(24 * time.Hour)
	}
}

func (s *Server) serve(l net.Listener) {
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
			tlsConfig:      s.tlsConfig,
			userDBs:        s.userDBs,
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
	netconn net.Conn
	tc      *textproto.Conn

	// System configuration.
	config *config.Config

	// TLS configuration.
	tlsConfig *tls.Config

	// Envelope.
	mail_from string
	rcpt_to   []string
	data      []byte

	// Are we using TLS?
	onTLS bool

	// User databases - taken from the server at creation time.
	userDBs map[string]*userdb.DB

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
	// params should be: "FROM:<name@host>"
	// First, get rid of the "FROM:" part (but check it, it's mandatory).
	sp := strings.SplitN(strings.ToLower(params), ":", 2)
	if len(sp) != 2 || sp[0] != "from" {
		return 500, "unknown command"
	}

	// Special case a null reverse-path, which is explicitly allowed and used
	// for notification messages.
	// It should be written "<>", we check for that and remove spaces just to
	// be more flexible.
	e := &mail.Address{}
	if strings.Replace(sp[1], " ", "", -1) == "<>" {
		e.Address = "<>"
	} else {
		var err error
		e, err = mail.ParseAddress(sp[1])
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

	c.mail_from = e.Address
	return 250, "You feel like you are being watched"
}

func (c *Conn) RCPT(params string) (code int, msg string) {
	// params should be: "TO:<name@host>"
	// First, get rid of the "TO:" part (but check it, it's mandatory).
	sp := strings.SplitN(strings.ToLower(params), ":", 2)
	if len(sp) != 2 || sp[0] != "to" {
		return 500, "unknown command"
	}

	// TODO: Write our own parser (we have different needs, mail.ParseAddress
	// is useful for other things).
	// Allow utf8, but prevent "control" characters.

	e, err := mail.ParseAddress(sp[1])
	if err != nil || e.Address == "" {
		return 501, "malformed address"
	}

	if c.mail_from == "" {
		return 503, "sender not yet given"
	}

	// RFC says 100 is the minimum limit for this, but it seems excessive.
	if len(c.rcpt_to) > 100 {
		return
	}

	// TODO: do we allow receivers without a domain?
	// TODO: check the case:
	//  - local recipient, always ok
	//  - external recipient, only ok if mail_from is local (needs auth)

	c.rcpt_to = append(c.rcpt_to, e.Address)
	return 250, "You have an eerie feeling..."
}

func (c *Conn) DATA(params string, tr *trace.Trace) (code int, msg string) {
	if c.mail_from == "" {
		return 503, "sender not yet given"
	}

	if len(c.rcpt_to) == 0 {
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

	// There are no partial failures here: we put it in the queue, and then if
	// individual deliveries fail, we report via email.
	msgID, err := c.queue.Put(c.mail_from, c.rcpt_to, c.data)
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

func (c *Conn) STARTTLS(params string, tr *trace.Trace) (code int, msg string) {
	if c.onTLS {
		return 503, "You are already wearing that!"
	}

	err := c.writeResponse(220, "You experience a strange sense of peace")
	if err != nil {
		return 554, fmt.Sprintf("error writing STARTTLS response: %v", err)
	}

	tr.LazyPrintf("<- 220  You experience a strange sense of peace")

	client := tls.Server(c.netconn, c.tlsConfig)
	err = client.Handshake()
	if err != nil {
		return 554, fmt.Sprintf("error in client handshake: %v", err)
	}

	tr.LazyPrintf("<> ...  jump to TLS was successful")

	// Override the connections. We don't need the older ones anymore.
	c.netconn = client
	c.tc = textproto.NewConn(client)

	// Reset the envelope; clients must start over after switching to TLS.
	c.resetEnvelope()

	c.onTLS = true

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
	} else {
		return 535, "Incorrect user or password"
	}
}

func (c *Conn) resetEnvelope() {
	c.mail_from = ""
	c.rcpt_to = nil
	c.data = nil
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
