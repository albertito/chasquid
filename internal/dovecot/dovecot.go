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
	"sync"
	"time"
	"unicode"
)

// DefaultTimeout to use. We expect Dovecot to be quite fast, but don't want
// to hang forever if something gets stuck.
const DefaultTimeout = 5 * time.Second

var (
	errUsernameNotSafe = errors.New("username not safe (contains spaces)")
	errFailedToConnect = errors.New("failed to connect to dovecot")
	errNoUserdbSocket  = errors.New("unable to find userdb socket")
	errNoClientSocket  = errors.New("unable to find client socket")
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
	addr struct {
		mu     *sync.Mutex
		userdb string
		client string
	}

	// Timeout for connection and I/O operations (applies on each call).
	// Set to DefaultTimeout by NewAuth.
	Timeout time.Duration
}

// NewAuth returns a new connection against Dovecot authentication service. It
// takes the addresses of userdb and client sockets (usually paths as
// configured in dovecot).
func NewAuth(userdb, client string) *Auth {
	a := &Auth{}
	a.addr.mu = &sync.Mutex{}
	a.addr.userdb = userdb
	a.addr.client = client
	a.Timeout = DefaultTimeout
	return a
}

// String representation of this Auth, for human consumption.
func (a *Auth) String() string {
	a.addr.mu.Lock()
	defer a.addr.mu.Unlock()
	return fmt.Sprintf("DovecotAuth(%q, %q)", a.addr.userdb, a.addr.client)
}

// Check to see if this auth is functional.
func (a *Auth) Check() error {
	u, c, err := a.getAddrs()
	if err != nil {
		return err
	}
	if !(a.canDial(u) && a.canDial(c)) {
		return errFailedToConnect
	}
	return nil
}

// Exists returns true if the user exists, false otherwise.
func (a *Auth) Exists(user string) (bool, error) {
	if !isUsernameSafe(user) {
		return false, errUsernameNotSafe
	}

	userdbAddr, _, err := a.getAddrs()
	if err != nil {
		return false, err
	}

	conn, err := a.dial("unix", userdbAddr)
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

	_, clientAddr, err := a.getAddrs()
	if err != nil {
		return false, err
	}

	conn, err := a.dial("unix", clientAddr)
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
	_, err := conn.W.Write([]byte(msg))
	if err != nil {
		return err
	}

	return conn.W.Flush()
}

// isUsernameSafe to use in the dovecot protocol?
// Unfortunately dovecot's protocol is not very robust wrt. whitespace,
// so we need to be careful.
func isUsernameSafe(user string) bool {
	for _, r := range user {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// getAddrs returns the addresses to the userdb and client sockets.
func (a *Auth) getAddrs() (string, string, error) {
	a.addr.mu.Lock()
	defer a.addr.mu.Unlock()

	if a.addr.userdb == "" {
		for _, u := range defaultUserdbPaths {
			if a.canDial(u) {
				a.addr.userdb = u
				break
			}
		}
		if a.addr.userdb == "" {
			return "", "", errNoUserdbSocket
		}
	}

	if a.addr.client == "" {
		for _, c := range defaultClientPaths {
			if a.canDial(c) {
				a.addr.client = c
				break
			}
		}
		if a.addr.client == "" {
			return "", "", errNoClientSocket
		}
	}

	return a.addr.userdb, a.addr.client, nil
}

func (a *Auth) canDial(path string) bool {
	conn, err := a.dial("unix", path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
