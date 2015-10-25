package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/trace"
)

func main() {
	flag.Parse()

	monAddr := ":1099"
	glog.Infof("Monitoring HTTP server listening on %s", monAddr)
	go http.ListenAndServe(monAddr, nil)

	ListenAndServe()
}

const (
	hostname = "charqui.com.ar"
)

func ListenAndServe() {
	addr := ":1025"
	l, err := net.Listen("tcp", addr)
	if err != nil {
		glog.Fatalf("Error listening: %v", err)
	}
	defer l.Close()

	glog.Infof("Server listening on %s", addr)
	for {
		conn, err := l.Accept()
		if err != nil {
			glog.Fatalf("Error accepting: %v", err)
		}

		sc := &Conn{
			netconn: conn,
			tc:      textproto.NewConn(conn),
		}
		go sc.Handle()
	}
}

type Conn struct {
	// Connection information.
	netconn net.Conn
	tc      *textproto.Conn

	// Message data.
	mail_from string
	rcpt_to   []string
	data      string
}

func (c *Conn) Handle() {
	defer c.netconn.Close()

	tr := trace.New("SMTP", "connection")
	defer tr.Finish()

	c.tc.PrintfLine("220 %s ESMTP charquid", hostname)

	var cmd, params string
	var err error

loop:
	for {
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
		case "MAIL":
			code, msg = c.MAIL(params)
		case "RCPT":
			code, msg = c.RCPT(params)
		case "DATA":
			code, msg = c.DATA(params)
			if code == 354 {
				// TODO: write response, read until dot, store in data, send
				// reply.
			}
		case "QUIT":
			c.writeResponse(221, "Be seeing you...")
			break loop
		default:
			code = 500
			msg = "unknown command"
		}

		tr.LazyPrintf("<- %d", code)

		err = c.writeResponse(code, msg)
		if err != nil {
			break
		}
	}

	if err != nil {
		tr.LazyPrintf("exiting with error: %v", err)
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
	fmt.Fprintf(buf, hostname+" - Your hour of destiny has come.\n")
	fmt.Fprintf(buf, "8BITMIME\n")
	fmt.Fprintf(buf, "PIPELINING\n")
	fmt.Fprintf(buf, "SIZE 52428800\n")
	fmt.Fprintf(buf, "STARTTLS\n")
	fmt.Fprintf(buf, "HELP\n")
	return 250, buf.String()
}

func (c *Conn) HELP(params string) (code int, msg string) {
	return 214, "hoy por ti, mañana por mi"
}

func (c *Conn) RSET(params string) (code int, msg string) {
	c.resetMessageData()

	msgs := []string{
		"Who was that Maud person anyway?",
		"Thinking of Maud you forget everything else.",
		"Your mind releases itself from mundane concerns.",
		"As your mind turns inward on itself, you forget everything else.",
	}
	return 250, msgs[rand.Int()%len(msgs)]
}

func (c *Conn) NOOP(params string) (code int, msg string) {
	return 250, "noooooooooooooooooooop"
}

func (c *Conn) MAIL(params string) (code int, msg string) {
	// params should be: "FROM:<name@host>"
	// First, get rid of the "FROM:" part (but check it, it's mandatory).
	sp := strings.SplitN(params, ":", 2)
	if len(sp) != 2 || sp[0] != "FROM" {
		return 500, "unknown command"
	}

	e, err := mail.ParseAddress(sp[1])
	if err != nil || e.Address == "" {
		return 501, "malformed address"
	}

	if !strings.Contains(e.Address, "@") {
		return 501, "sender address must contain a domain"
	}

	c.resetMessageData()
	c.mail_from = e.Address
	return 250, "You feel like you are being watched"
}

func (c *Conn) RCPT(params string) (code int, msg string) {
	// params should be: "TO:<name@host>"
	// First, get rid of the "TO:" part (but check it, it's mandatory).
	sp := strings.SplitN(params, ":", 2)
	if len(sp) != 2 || sp[0] != "TO" {
		return 500, "unknown command"
	}

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

func (c *Conn) DATA(params string) (code int, msg string) {
	if c.mail_from == "" {
		return 503, "sender not yet given"
	}

	if len(c.rcpt_to) == 0 {
		return 503, "need an address to send to"
	}

	return 354, "You experience a strange sense of peace"
}

func (c *Conn) resetMessageData() {
	c.mail_from = ""
	c.rcpt_to = nil
	c.data = ""
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
