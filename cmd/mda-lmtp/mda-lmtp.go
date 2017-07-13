// mda-lmtp is a very basic MDA that uses LMTP to do the delivery.
//
// See the usage below for details.
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"strings"
)

// Command-line flags
var (
	fromwhom  = flag.String("f", "", "Whom the message is from")
	recipient = flag.String("d", "", "Recipient")

	addrNetwork = flag.String("addr_network", "",
		"Network of the LMTP address (e.g. unix or tcp)")
	addr = flag.String("addr", "", "LMTP server address")
)

func usage() {
	fmt.Fprintf(os.Stderr, `
mda-lmtp is a very basic MDA that uses LMTP to do the mail delivery.

It takes command line arguments similar to maildrop or procmail, reads an
email via standard input, and sends it over the given LMTP server.
Supports connecting to LMTP servers over UNIX sockets and TCP.

It can be used when your mail server does not support LMTP directly.

Example of use:
$ mda-lmtp --addr localhost:1234 -f juan@casa -d jose < email

Flags:
`)
	flag.PrintDefaults()
}

// Exit with EX_TEMPFAIL.
func tempExit(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
	// 75 = EX_TEMPFAIL "temporary failure" exit code (sysexits.h).
	os.Exit(75)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if *addr == "" {
		fmt.Printf("No LMTP server address given (use --addr)\n")
		os.Exit(2)
	}

	// Try to autodetect the network if it's missing.
	if *addrNetwork == "" {
		*addrNetwork = "tcp"
		if strings.HasPrefix(*addr, "/") {
			*addrNetwork = "unix"
		}
	}

	conn, err := net.Dial(*addrNetwork, *addr)
	if err != nil {
		tempExit("Error connecting to (%s, %s): %v",
			*addrNetwork, *addr, err)
	}

	tc := textproto.NewConn(conn)

	// Expect the hello from the server.
	_, _, err = tc.ReadResponse(220)
	if err != nil {
		tempExit("Server greeting error: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		tempExit("Could not get hostname: %v", err)
	}

	cmd(tc, 250, "LHLO %s", hostname)
	cmd(tc, 250, "MAIL FROM:<%s>", *fromwhom)
	cmd(tc, 250, "RCPT TO:<%s>", *recipient)
	cmd(tc, 354, "DATA")

	w := tc.DotWriter()
	_, err = io.Copy(w, os.Stdin)
	w.Close()
	if err != nil {
		tempExit("Error writing DATA: %v", err)
	}

	// This differs from SMTP: here we get one reply per recipient, with the
	// result of the delivery. Since we deliver to only one recipient, read
	// one code.
	_, _, err = tc.ReadResponse(250)
	if err != nil {
		tempExit("Delivery failed remotely: %v", err)
	}

	cmd(tc, 221, "QUIT")

	tc.Close()
}

// cmd sends a command and checks it matched the expected code.
func cmd(conn *textproto.Conn, expectCode int, format string, args ...interface{}) {
	id, err := conn.Cmd(format, args...)
	if err != nil {
		tempExit("Sent %q, got %v", fmt.Sprintf(format, args...), err)
	}
	conn.StartResponse(id)
	defer conn.EndResponse(id)

	_, _, err = conn.ReadResponse(expectCode)
	if err != nil {
		tempExit("Sent %q, got %v", fmt.Sprintf(format, args...), err)
	}
}
