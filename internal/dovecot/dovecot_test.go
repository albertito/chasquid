package dovecot

// The dovecot package is mainly tested via integration/external tests using
// the dovecot-auth-cli tool. See cmd/dovecot-auth-cli for more details.
// The tests here are more narrow and only test specific functionality that is
// easier to cover from Go.

import (
	"net"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
)

func TestUsernameNotSafe(t *testing.T) {
	a := NewAuth("/tmp/nothing", "/tmp/nothing")

	cases := []string{
		"a b", " ab", "ab ", "a\tb", "a\t", " ", "\t", "\t "}
	for _, c := range cases {
		ok, err := a.Authenticate(c, "passwd")
		if ok || err != errUsernameNotSafe {
			t.Errorf("Authenticate(%q, _): got %v, %v", c, ok, err)
		}

		ok, err = a.Exists(c)
		if ok || err != errUsernameNotSafe {
			t.Errorf("Exists(%q): got %v, %v", c, ok, err)
		}
	}
}

func TestAutodetect(t *testing.T) {
	// Check on a pair that does not exist.
	a := NewAuth("uDoesNotExist", "cDoesNotExist")
	err := a.Check()
	if err != errFailedToConnect {
		t.Errorf("Expected failure to connect, got %v", err)
	}

	// We override the default paths, so we can point the "defaults" to our
	// test environment as needed.
	defaultUserdbPaths = []string{"/dev/null"}
	defaultClientPaths = []string{"/dev/null"}

	// Autodetect failure: no valid sockets on the list.
	a = NewAuth("", "")
	err = a.Check()
	if err != errNoUserdbSocket {
		t.Errorf("Expected failure to find userdb socket, got %v", err)
	}
	ok, err := a.Exists("user")
	if ok != false || err != errNoUserdbSocket {
		t.Errorf("Expected {false, no userdb socket}, got {%v, %v}", ok, err)
	}
	ok, err = a.Authenticate("user", "password")
	if ok != false || err != errNoUserdbSocket {
		t.Errorf("Expected {false, no userdb socket}, got {%v, %v}", ok, err)
	}

	// Create a temporary directory, and two sockets on it.
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	userdb := dir + "/userdb"
	client := dir + "/client"

	uL := mustListen(t, userdb)
	cL := mustListen(t, client)

	// Autodetect finds the user, but fails to find the client.
	defaultUserdbPaths = []string{"/dev/null", userdb}
	defaultClientPaths = []string{"/dev/null"}
	a = NewAuth("", "")
	err = a.Check()
	if err != errNoClientSocket {
		t.Errorf("Expected failure to find userdb socket, got %v", err)
	}

	// Autodetect should pick the suggestions passed as parameters (if
	// possible).
	defaultUserdbPaths = []string{"/dev/null"}
	defaultClientPaths = []string{"/dev/null", client}
	a = NewAuth(userdb, "")
	err = a.Check()
	if err != nil {
		t.Errorf("Expected successful check, got %v", err)
	}
	if a.addr.userdb != userdb || a.addr.client != client {
		t.Errorf("Expected autodetect to pick {%q, %q}, but got {%q, %q}",
			userdb, client, a.addr.userdb, a.addr.client)
	}

	// Successful autodetection against open sockets.
	defaultUserdbPaths = append(defaultUserdbPaths, userdb)
	defaultClientPaths = append(defaultClientPaths, client)
	a = NewAuth("", "")
	err = a.Check()
	if err != nil {
		t.Errorf("Expected successful check, got %v", err)
	}

	// Close the two sockets, and re-do the check: now we have pinned the
	// paths, and check should fail to connect.
	// We need to tell Go to keep the socket files around explicitly, as the
	// default is to delete them since they were created by the net library.
	uL.SetUnlinkOnClose(false)
	uL.Close()
	err = a.Check()
	if err != errFailedToConnect {
		t.Errorf("Expected failed to connect, got %v", err)
	}

	cL.SetUnlinkOnClose(false)
	cL.Close()
	err = a.Check()
	if err != errFailedToConnect {
		t.Errorf("Expected failed to connect, got %v", err)
	}
}

func TestReload(t *testing.T) {
	// Make sure Reload does not fail.
	a := Auth{}
	if err := a.Reload(); err != nil {
		t.Errorf("Reload failed")
	}
}

func mustListen(t *testing.T, path string) *net.UnixListener {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		t.Fatalf("failed to resolve unix addr %q: %v", path, err)
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatalf("failed to listen on %q: %v", path, err)
	}

	return l
}
