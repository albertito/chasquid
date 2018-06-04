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
	// If we give both parameters to autodetect, it should return a new Auth
	// using them, even if they're not valid.
	a := Autodetect("uDoesNotExist", "cDoesNotExist")
	if a == nil {
		t.Errorf("Autodetection with two params failed")
	} else if *a != *NewAuth("uDoesNotExist", "cDoesNotExist") {
		t.Errorf("Autodetection with two params: got %v", a)
	}

	// We override the default paths, so we can point the "defaults" to our
	// test environment as needed.
	defaultUserdbPaths = []string{"/dev/null"}
	defaultClientPaths = []string{"/dev/null"}

	// Autodetect failure: no valid sockets on the list.
	a = Autodetect("", "")
	if a != nil {
		t.Errorf("Autodetection worked with only /dev/null, got %v", a)
	}

	// Create a temporary directory, and two sockets on it.
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	userdb := dir + "/userdb"
	client := dir + "/client"

	uL := mustListen(t, userdb)
	cL := mustListen(t, client)

	defaultUserdbPaths = append(defaultUserdbPaths, userdb)
	defaultClientPaths = append(defaultClientPaths, client)

	// Autodetect should work fine against open sockets.
	a = Autodetect("", "")
	if a == nil {
		t.Errorf("Autodetection failed (open sockets)")
	} else if a.userdbAddr != userdb || a.clientAddr != client {
		t.Errorf("Expected autodetect to pick {%q, %q}, but got {%q, %q}",
			userdb, client, a.userdbAddr, a.clientAddr)
	}

	// TODO: Close the two sockets, and re-do the test from above: Autodetect
	// should work fine against closed sockets.
	// To implement this test, we should call SetUnlinkOnClose, but
	// unfortunately that is only available in Go >= 1.8.
	// We want to support Go 1.7 for a while as it is in Debian stable; once
	// Debian stable moves on, we can implement this test easily.

	// Autodetect should pick the suggestions passed as parameters (if
	// possible).
	defaultUserdbPaths = []string{"/dev/null"}
	defaultClientPaths = []string{"/dev/null", client}
	a = Autodetect(userdb, "")
	if a == nil {
		t.Errorf("Autodetection failed (single parameter)")
	} else if a.userdbAddr != userdb || a.clientAddr != client {
		t.Errorf("Expected autodetect to pick {%q, %q}, but got {%q, %q}",
			userdb, client, a.userdbAddr, a.clientAddr)
	}

	uL.Close()
	cL.Close()
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

func TestNotASocket(t *testing.T) {
	if isUnixSocket("/doesnotexist") {
		t.Errorf("isUnixSocket(/doesnotexist) returned true")
	}
}
