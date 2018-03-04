package smtpsrv

import (
	"bytes"
	"context"
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
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/tlsconst"
	"blitiri.com.ar/go/chasquid/internal/trace"
	"blitiri.com.ar/go/spf"
)

// Exported variables.
var (
	commandCount      = expvar.NewMap("chasquid/smtpIn/commandCount")
	responseCodeCount = expvar.NewMap("chasquid/smtpIn/responseCodeCount")
	spfResultCount    = expvar.NewMap("chasquid/smtpIn/spfResultCount")
	loopsDetected     = expvar.NewInt("chasquid/smtpIn/loopsDetected")
	tlsCount          = expvar.NewMap("chasquid/smtpIn/tlsCount")
	slcResults        = expvar.NewMap("chasquid/smtpIn/securityLevelChecks")
	hookResults       = expvar.NewMap("chasquid/smtpIn/hookResults")
)

var (
	maxReceivedHeaders = flag.Int("testing__max_received_headers", 50,
		"max Received headers, for loop detection; ONLY FOR TESTING")

	// Some go tests disable SPF, to avoid leaking DNS lookups.
	disableSPFForTesting = false
)

// SocketMode represents the mode for a socket (listening or connection).
// We keep them distinct, as policies can differ between them.
type SocketMode struct {
	// Is this mode submission?
	IsSubmission bool

	// Is this mode TLS-wrapped? That means that we don't use STARTTLS, the
	// connection is directly established over TLS (like HTTPS).
	TLS bool
}

func (mode SocketMode) String() string {
	s := "SMTP"
	if mode.IsSubmission {
		s = "submission"
	}
	if mode.TLS {
		s += "+TLS"
	}
	return s
}

// Valid socket modes.
var (
	ModeSMTP          = SocketMode{IsSubmission: false, TLS: false}
	ModeSubmission    = SocketMode{IsSubmission: true, TLS: false}
	ModeSubmissionTLS = SocketMode{IsSubmission: true, TLS: true}
)

// Conn represents an incoming SMTP connection.
type Conn struct {
	// Main hostname, used for display only.
	hostname string

	// Maximum data size.
	maxDataSize int64

	// Post-DATA hook location.
	postDataHook string

	// Connection information.
	conn         net.Conn
	tc           *textproto.Conn
	mode         SocketMode
	tlsConnState *tls.ConnectionState

	// Tracer to use.
	tr *trace.Trace

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

	// Authenticator, aliases and local domains, taken from the server at
	// creation time.
	authr        *auth.Authenticator
	localDomains *set.String
	aliasesR     *aliases.Resolver
	dinfo        *domaininfo.DB

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

// Close the connection.
func (c *Conn) Close() {
	c.conn.Close()
}

// Handle implements the main protocol loop (reading commands, sending
// replies).
func (c *Conn) Handle() {
	defer c.Close()

	c.tr = trace.New("SMTP.Conn", c.conn.RemoteAddr().String())
	defer c.tr.Finish()
	c.tr.Debugf("Connected, mode: %s", c.mode)

	if tc, ok := c.conn.(*tls.Conn); ok {
		// For TLS connections, complete the handshake and get the state, so
		// it can be used when we say hello below.
		tc.Handshake()
		cstate := tc.ConnectionState()
		c.tlsConnState = &cstate
		if name := c.tlsConnState.ServerName; name != "" {
			c.hostname = name
		}
	}

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

		c.conn.SetDeadline(time.Now().Add(c.commandTimeout))

		cmd, params, err = c.readCommand()
		if err != nil {
			c.tc.PrintfLine("554 error reading command: %v", err)
			break
		}

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
			// Sanitize it a bit to avoid filling the logs and events with
			// noisy data. Keep the first 6 bytes for debugging.
			cmd = fmt.Sprintf("unknown<%.6s>", cmd)
			code = 500
			msg = "unknown command"
		}

		commandCount.Add(cmd, 1)
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

// HELO SMTP command handler.
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

// EHLO SMTP command handler.
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

// HELP SMTP command handler.
func (c *Conn) HELP(params string) (code int, msg string) {
	return 214, "hoy por ti, mañana por mi"
}

// RSET SMTP command handler.
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

// VRFY SMTP command handler.
func (c *Conn) VRFY(params string) (code int, msg string) {
	// 252 can be used for cases like ours, when we don't really want to
	// confirm or deny anything.
	// See https://tools.ietf.org/html/rfc2821#section-3.5.3.
	return 252, "You have a strange feeling for a moment, then it passes."
}

// EXPN SMTP command handler.
func (c *Conn) EXPN(params string) (code int, msg string) {
	// 252 can be used for cases like ours, when we don't really want to
	// confirm or deny anything.
	// See https://tools.ietf.org/html/rfc2821#section-3.5.3.
	return 252, "You feel disoriented for a moment."
}

// NOOP SMTP command handler.
func (c *Conn) NOOP(params string) (code int, msg string) {
	return 250, "You hear a faint typing noise."
}

// MAIL SMTP command handler.
func (c *Conn) MAIL(params string) (code int, msg string) {
	// params should be: "FROM:<name@host>", and possibly followed by
	// options such as "BODY=8BITMIME" (which we ignore).
	// Check that it begins with "FROM:" first, it's mandatory.
	if !strings.HasPrefix(strings.ToLower(params), "from:") {
		return 500, "unknown command"
	}
	if c.mode.IsSubmission && !c.completedAuth {
		return 550, "mail to submission port must be authenticated"
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
			maillog.Rejected(c.conn.RemoteAddr(), addr, nil,
				fmt.Sprintf("failed SPF: %v", c.spfError))
			return 550, fmt.Sprintf(
				"SPF check failed: %v", c.spfError)
		}

		if !c.secLevelCheck(addr) {
			maillog.Rejected(c.conn.RemoteAddr(), addr, nil,
				"security level check failed")
			return 550, "security level check failed"
		}

		addr, err = normalize.DomainToUnicode(addr)
		if err != nil {
			maillog.Rejected(c.conn.RemoteAddr(), addr, nil,
				fmt.Sprintf("malformed address: %v", err))
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

	if disableSPFForTesting {
		return "", nil
	}

	if tcp, ok := c.conn.RemoteAddr().(*net.TCPAddr); ok {
		res, err := spf.CheckHost(
			tcp.IP, envelope.DomainOf(addr))

		c.tr.Debugf("SPF %v (%v)", res, err)
		spfResultCount.Add(string(res), 1)

		return res, err
	}

	return "", nil
}

// secLevelCheck checks if the security level is acceptable for the given
// address.
func (c *Conn) secLevelCheck(addr string) bool {
	// Only check if SPF passes. This serves two purposes:
	//  - Skip for authenticated connections (we trust them implicitly).
	//  - Don't apply this if we can't be sure the sender is authorized.
	//    Otherwise anyone could raise the level of any domain.
	if c.spfResult != spf.Pass {
		slcResults.Add("skip", 1)
		c.tr.Debugf("SPF did not pass, skipping security level check")
		return true
	}

	domain := envelope.DomainOf(addr)
	level := domaininfo.SecLevel_PLAIN
	if c.onTLS {
		level = domaininfo.SecLevel_TLS_CLIENT
	}

	ok := c.dinfo.IncomingSecLevel(domain, level)
	if ok {
		slcResults.Add("pass", 1)
		c.tr.Debugf("security level check for %s passed (%s)", domain, level)
	} else {
		slcResults.Add("fail", 1)
		c.tr.Errorf("security level check for %s failed (%s)", domain, level)
	}

	return ok
}

// RCPT SMTP command handler.
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
		maillog.Rejected(c.conn.RemoteAddr(), c.mailFrom, []string{addr},
			"relay not allowed")
		return 503, "relay not allowed"
	}

	if localDst {
		addr, err = normalize.Addr(addr)
		if err != nil {
			maillog.Rejected(c.conn.RemoteAddr(), c.mailFrom, []string{addr},
				fmt.Sprintf("invalid address: %v", err))
			return 550, "recipient invalid, please check the address for typos"
		}

		if !c.userExists(addr) {
			maillog.Rejected(c.conn.RemoteAddr(), c.mailFrom, []string{addr},
				"local user does not exist")
			return 550, "recipient unknown, please check the address for typos"
		}
	}

	c.rcptTo = append(c.rcptTo, addr)
	return 250, "You have an eerie feeling..."
}

// DATA SMTP command handler.
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
	c.conn.SetDeadline(c.deadline)

	dotr := io.LimitReader(c.tc.DotReader(), c.maxDataSize)
	c.data, err = ioutil.ReadAll(dotr)
	if err != nil {
		return 554, fmt.Sprintf("error reading DATA: %v", err)
	}

	c.tr.Debugf("-> ... %d bytes of data", len(c.data))

	if err := checkData(c.data); err != nil {
		maillog.Rejected(c.conn.RemoteAddr(), c.mailFrom, c.rcptTo, err.Error())
		return 554, err.Error()
	}

	c.addReceivedHeader()

	hookOut, permanent, err := c.runPostDataHook(c.data)
	if err != nil {
		maillog.Rejected(c.conn.RemoteAddr(), c.mailFrom, c.rcptTo, err.Error())
		if permanent {
			return 554, err.Error()
		}
		return 451, err.Error()
	}
	c.data = append(hookOut, c.data...)

	// There are no partial failures here: we put it in the queue, and then if
	// individual deliveries fail, we report via email.
	msgID, err := c.queue.Put(c.mailFrom, c.rcptTo, c.data)
	if err != nil {
		return 554, fmt.Sprintf("Failed to enqueue message: %v", err)
	}

	c.tr.Printf("Queued from %s to %s - %s", c.mailFrom, c.rcptTo, msgID)
	maillog.Queued(c.conn.RemoteAddr(), c.mailFrom, c.rcptTo, msgID)

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
			c.ehloAddress, c.conn.RemoteAddr().String())
	}

	v += fmt.Sprintf("by %s (chasquid)\n", c.hostname)

	v += fmt.Sprintf("(over %s ", c.mode)
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
	if len(msg.Header["Received"]) > *maxReceivedHeaders {
		loopsDetected.Add(1)
		return fmt.Errorf("email passed through more than %d MTAs, looping?",
			*maxReceivedHeaders)
	}

	return nil
}

// runPostDataHook and return the new headers to add, and on error a boolean
// indicating if it's permanent, and the error itself.
func (c *Conn) runPostDataHook(data []byte) ([]byte, bool, error) {
	// TODO: check if the file is executable.
	if _, err := os.Stat(c.postDataHook); os.IsNotExist(err) {
		hookResults.Add("post-data:skip", 1)
		return nil, false, nil
	}
	tr := trace.New("Hook.Post-DATA", c.conn.RemoteAddr().String())
	defer tr.Finish()
	tr.Debugf("running")

	ctx, cancel := context.WithDeadline(context.Background(),
		time.Now().Add(1*time.Minute))
	defer cancel()
	cmd := exec.CommandContext(ctx, c.postDataHook)
	cmd.Stdin = bytes.NewReader(data)

	// Prepare the environment, copying some common variables so the hook has
	// someting reasonable, and then setting the specific ones for this case.
	for _, v := range strings.Fields("USER PWD SHELL PATH") {
		cmd.Env = append(cmd.Env, v+"="+os.Getenv(v))
	}
	cmd.Env = append(cmd.Env, "REMOTE_ADDR="+c.conn.RemoteAddr().String())
	cmd.Env = append(cmd.Env, "MAIL_FROM="+c.mailFrom)
	cmd.Env = append(cmd.Env, "RCPT_TO="+strings.Join(c.rcptTo, " "))

	if c.completedAuth {
		cmd.Env = append(cmd.Env, "AUTH_AS="+c.authUser+"@"+c.authDomain)
	} else {
		cmd.Env = append(cmd.Env, "AUTH_AS=")
	}

	cmd.Env = append(cmd.Env, "ON_TLS="+boolToStr(c.onTLS))
	cmd.Env = append(cmd.Env, "FROM_LOCAL_DOMAIN="+boolToStr(
		envelope.DomainIn(c.mailFrom, c.localDomains)))
	cmd.Env = append(cmd.Env, "SPF_PASS="+boolToStr(c.spfResult == spf.Pass))

	out, err := cmd.Output()
	if err != nil {
		hookResults.Add("post-data:fail", 1)
		tr.Error(err)
		tr.Debugf("stdout: %s", out)

		permanent := false
		if ee, ok := err.(*exec.ExitError); ok {
			tr.Printf("stderr: %s", string(ee.Stderr))
			if status, ok := ee.Sys().(syscall.WaitStatus); ok {
				permanent = status.ExitStatus() == 20
			}
		}

		// The error contains the last line of stdout, so filters can pass
		// some rejection information back to the sender.
		err = fmt.Errorf(lastLine(string(out)))
		return nil, permanent, err
	}

	// Check that output looks like headers, to avoid breaking the email
	// contents. If it does not, just skip it.
	if !isHeader(out) {
		hookResults.Add("post-data:badoutput", 1)
		tr.Errorf("error parsing post-data output: '%s'", out)
		return nil, false, nil
	}

	tr.Debugf("success")
	tr.Debugf("stdout: %s", out)
	hookResults.Add("post-data:success", 1)
	return out, false, nil
}

// isHeader checks if the given buffer is a valid MIME header.
func isHeader(b []byte) bool {
	s := string(b)
	if len(s) == 0 {
		return true
	}

	// If it is just a \n, or contains two \n, then it's not a header.
	if s == "\n" || strings.Contains(s, "\n\n") {
		return false
	}

	// If it does not end in \n, not a header.
	if s[len(s)-1] != '\n' {
		return false
	}

	// Each line must either start with a space or have a ':'.
	seen := false
	for _, line := range strings.SplitAfter(s, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if !seen {
				// Continuation without a header first (invalid).
				return false
			}
			continue
		}
		if !strings.Contains(line, ":") {
			return false
		}
		seen = true
	}
	return true
}

func lastLine(s string) string {
	l := strings.Split(s, "\n")
	if len(l) < 2 {
		return ""
	}
	return l[len(l)-2]
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// STARTTLS SMTP command handler.
func (c *Conn) STARTTLS(params string) (code int, msg string) {
	if c.onTLS {
		return 503, "You are already wearing that!"
	}

	err := c.writeResponse(220, "You experience a strange sense of peace")
	if err != nil {
		return 554, fmt.Sprintf("error writing STARTTLS response: %v", err)
	}

	c.tr.Debugf("<- 220  You experience a strange sense of peace")

	server := tls.Server(c.conn, c.tlsConfig)
	err = server.Handshake()
	if err != nil {
		return 554, fmt.Sprintf("error in TLS handshake: %v", err)
	}

	c.tr.Debugf("<> ...  jump to TLS was successful")

	// Override the connections. We don't need the older ones anymore.
	c.conn = server
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

// AUTH SMTP command handler.
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

	authOk, err := c.authr.Authenticate(user, domain, passwd)
	if err != nil {
		c.tr.Errorf("error authenticating %q@%q: %v", user, domain, err)
	}
	if authOk {
		c.authUser = user
		c.authDomain = domain
		c.completedAuth = true
		maillog.Auth(c.conn.RemoteAddr(), user+"@"+domain, true)
		return 235, ""
	}

	maillog.Auth(c.conn.RemoteAddr(), user+"@"+domain, false)
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
	ok, err := c.authr.Exists(user, domain)
	if err != nil {
		c.tr.Errorf("error checking if user %q exists: %v", addr, err)
	}
	return ok
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
