// Package dovecot implements functions to interact with Dovecot's
// authentication service.
//
// In particular, it supports doing user authorization, and checking if a user
// exists. It is a very basic implementation, with only the minimum needed to
// cover chasquid's needs.
//
// https://wiki.dovecot.org/Design/AuthProtocol
// https://wiki.dovecot.org/Services#auth
package dovecot

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"strings"
	"time"
	"unicode"
)

// DefaultTimeout to use. We expect Dovecot to be quite fast, but don't want
// to hang forever if something gets stuck.
const DefaultTimeout = 5 * time.Second

var (
	errUsernameNotSafe = errors.New("username not safe (contains spaces)")
)

var defaultUserdbPaths = []string{
	"/var/run/dovecot/auth-chasquid-userdb",
	"/var/run/dovecot/auth-userdb",
}

var defaultClientPaths = []string{
	"/var/run/dovecot/auth-chasquid-client",
	"/var/run/dovecot/auth-client",
}

// Auth represents a particular Dovecot auth service to use.
type Auth struct {
	userdbAddr string
	clientAddr string

	// Timeout for connection and I/O operations (applies on each call).
	// Set to DefaultTimeout by NewAuth.
	Timeout time.Duration
}

// NewAuth returns a new connection against Dovecot authentication service. It
// takes the addresses of userdb and client sockets (usually paths as
// configured in dovecot).
func NewAuth(userdb, client string) *Auth {
	return &Auth{
		userdbAddr: userdb,
		clientAddr: client,
		Timeout:    DefaultTimeout,
	}
}

// String representation of this Auth, for human consumption.
func (a *Auth) String() string {
	return fmt.Sprintf("DovecotAuth(%q, %q)", a.userdbAddr, a.clientAddr)
}

// Check to see if this auth is valid (but may not be working).
func (a *Auth) Check() error {
	// We intentionally don't connect or complete any handshakes because
	// dovecot may not be up yet, even thought it may be configured properly.
	// Just check that the addresses are valid sockets.
	if !isUnixSocket(a.userdbAddr) {
		return fmt.Errorf("userdb is not an unix socket")
	}
	if !isUnixSocket(a.clientAddr) {
		return fmt.Errorf("client is not an unix socket")
	}

	return nil
}

// Exists returns true if the user exists, false otherwise.
func (a *Auth) Exists(user string) (bool, error) {
	if !isUsernameSafe(user) {
		return false, errUsernameNotSafe
	}

	conn, err := a.dial("unix", a.userdbAddr)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	// Dovecot greets us with version and server pid.
	// VERSION\t<major>\t<minor>
	// SPID\t<pid>
	err = expect(conn, "VERSION\t1")
	if err != nil {
		return false, fmt.Errorf("error receiving version: %v", err)
	}
	err = expect(conn, "SPID\t")
	if err != nil {
		return false, fmt.Errorf("error receiving SPID: %v", err)
	}

	// Send our version, and then the request.
	err = write(conn, "VERSION\t1\t1\n")
	if err != nil {
		return false, err
	}

	err = write(conn, fmt.Sprintf("USER\t1\t%s\tservice=smtp\n", user))
	if err != nil {
		return false, err
	}

	// Get the response, and we're done.
	resp, err := conn.ReadLine()
	if err != nil {
		return false, fmt.Errorf("error receiving response: %v", err)
	} else if strings.HasPrefix(resp, "USER\t1\t") {
		return true, nil
	} else if strings.HasPrefix(resp, "NOTFOUND\t") {
		return false, nil
	}
	return false, fmt.Errorf("invalid response: %q", resp)
}

// Authenticate returns true if the password is valid for the user, false
// otherwise.
func (a *Auth) Authenticate(user, passwd string) (bool, error) {
	if !isUsernameSafe(user) {
		return false, errUsernameNotSafe
	}

	conn, err := a.dial("unix", a.clientAddr)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	// Send our version, and then our PID.
	err = write(conn, fmt.Sprintf("VERSION\t1\t1\nCPID\t%d\n", os.Getpid()))
	if err != nil {
		return false, err
	}

	// Read the server-side handshake. We don't care about the contents
	// really, so just read all lines until we see the DONE.
	for {
		resp, err := conn.ReadLine()
		if err != nil {
			return false, fmt.Errorf("error receiving handshake: %v", err)
		}
		if resp == "DONE" {
			break
		}
	}

	// We only support PLAIN authentication, so construct the request.
	// Note we set the "secured" option, with the assumpition that we got the
	// password via a secure channel (like TLS). This is always true for
	// chasquid by design, and simplifies the API.
	// TODO: does dovecot handle utf8 domains well? do we need to encode them
	// in IDNA first?
	resp := base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s\x00%s\x00%s", user, user, passwd)))
	err = write(conn, fmt.Sprintf(
		"AUTH\t1\tPLAIN\tservice=smtp\tsecured\tno-penalty\tnologin\tresp=%s\n", resp))
	if err != nil {
		return false, err
	}

	// Get the response, and we're done.
	resp, err = conn.ReadLine()
	if err != nil {
		return false, fmt.Errorf("error receiving response: %v", err)
	} else if strings.HasPrefix(resp, "OK\t1") {
		return true, nil
	} else if strings.HasPrefix(resp, "FAIL\t1") {
		return false, nil
	}
	return false, fmt.Errorf("invalid response: %q", resp)
}

// Reload the authenticator. It's a no-op for dovecot, but it is needed to
// conform with the auth.Backend interface.
func (a *Auth) Reload() error {
	return nil
}

func (a *Auth) dial(network, addr string) (*textproto.Conn, error) {
	nc, err := net.DialTimeout(network, addr, a.Timeout)
	if err != nil {
		return nil, err
	}

	nc.SetDeadline(time.Now().Add(a.Timeout))

	return textproto.NewConn(nc), nil
}

func expect(conn *textproto.Conn, prefix string) error {
	resp, err := conn.ReadLine()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, prefix) {
		return fmt.Errorf("got %q", resp)
	}
	return nil
}

func write(conn *textproto.Conn, msg string) error {
	_, err := fmt.Fprintf(conn.W, msg)
	if err != nil {
		return err
	}

	return conn.W.Flush()
}

// isUsernameSafe to use in the dovecot protocol?
// Unfotunately dovecot's protocol is not very robust wrt. whitespace,
// so we need to be careful.
func isUsernameSafe(user string) bool {
	for _, r := range user {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Autodetect where the dovecot authentication paths are, and return an Auth
// instance for them. If any of userdb or client are != "", they will be used
// and not autodetected.
func Autodetect(userdb, client string) *Auth {
	// If both are given, no need to autodtect.
	if userdb != "" && client != "" {
		return NewAuth(userdb, client)
	}

	var userdbs, clients []string
	if userdb != "" {
		userdbs = append(userdbs, userdb)
	}
	if client != "" {
		clients = append(clients, client)
	}

	if len(userdbs) == 0 {
		userdbs = append(userdbs, defaultUserdbPaths...)
	}

	if len(clients) == 0 {
		clients = append(clients, defaultClientPaths...)
	}

	// Go through each possiblity, return the first auth that works.
	for _, u := range userdbs {
		for _, c := range clients {
			a := NewAuth(u, c)
			if a.Check() == nil {
				return a
			}
		}
	}

	return nil
}

func isUnixSocket(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSocket != 0
}
