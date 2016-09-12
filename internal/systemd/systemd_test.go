package systemd

import (
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

func setenv(pid, fds string, names ...string) {
	os.Setenv("LISTEN_PID", pid)
	os.Setenv("LISTEN_FDS", fds)
	os.Setenv("LISTEN_FDNAMES", strings.Join(names, ":"))
}

func TestEmptyEnvironment(t *testing.T) {
	cases := []struct{ pid, fds string }{
		{"", ""},
		{"123", ""},
		{"", "4"},
	}
	for _, c := range cases {
		setenv(c.pid, c.fds)

		if ls, err := Listeners(); ls != nil || err != nil {
			t.Logf("Case: LISTEN_PID=%q  LISTEN_FDS=%q", c.pid, c.fds)
			t.Errorf("Unexpected result: %v // %v", ls, err)
		}
	}
}

func TestBadEnvironment(t *testing.T) {
	// Create a listener so we have something to reference.
	l := newListener(t)
	firstFD = listenerFd(t, l)

	ourPID := strconv.Itoa(os.Getpid())
	cases := []struct {
		pid, fds string
		names    []string
	}{
		{"a", "1", []string{"name"}},              // Invalid PID.
		{ourPID, "a", []string{"name"}},           // Invalid number of fds.
		{"1", "1", []string{"name"}},              // PID != ourselves.
		{ourPID, "1", []string{"name1", "name2"}}, // Too many names.
		{ourPID, "1", []string{}},                 // Not enough names.
	}
	for _, c := range cases {
		setenv(c.pid, c.fds, c.names...)

		if ls, err := Listeners(); err == nil {
			t.Logf("Case: LISTEN_PID=%q  LISTEN_FDS=%q LISTEN_FDNAMES=%q", c.pid, c.fds, c.names)
			t.Errorf("Unexpected result: %v // %v", ls, err)
		}
	}
}

func TestWrongPID(t *testing.T) {
	// Find a pid != us. 1 should always work in practice.
	pid := 1
	for pid == os.Getpid() {
		pid = rand.Int()
	}

	setenv(strconv.Itoa(pid), "4")
	if _, err := Listeners(); err != ErrPIDMismatch {
		t.Errorf("Did not fail with PID mismatch: %v", err)
	}
}

func TestNoFDs(t *testing.T) {
	setenv(strconv.Itoa(os.Getpid()), "0")
	if ls, err := Listeners(); len(ls) != 0 || err != nil {
		t.Errorf("Got a non-empty result: %v // %v", ls, err)
	}
}

// newListener creates a TCP listener.
func newListener(t *testing.T) *net.TCPListener {
	addr := &net.TCPAddr{
		Port: 0,
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("Could not create TCP listener: %v", err)
	}

	return l
}

// listenerFd returns a file descriptor for the listener.
// Note it is a NEW file descriptor, not the original one.
func listenerFd(t *testing.T, l *net.TCPListener) int {
	f, err := l.File()
	if err != nil {
		t.Fatalf("Could not get TCP listener file: %v", err)
	}

	return int(f.Fd())
}

func sameAddr(a, b net.Addr) bool {
	return a.Network() == b.Network() && a.String() == b.String()
}

func TestOneSocket(t *testing.T) {
	l := newListener(t)
	firstFD = listenerFd(t, l)

	setenv(strconv.Itoa(os.Getpid()), "1", "name")

	lsMap, err := Listeners()
	if err != nil || len(lsMap) != 1 {
		t.Fatalf("Got an invalid result: %v // %v", lsMap, err)
	}

	ls := lsMap["name"]

	if !sameAddr(ls[0].Addr(), l.Addr()) {
		t.Errorf("Listener 0 address mismatch, expected %#v, got %#v",
			l.Addr(), ls[0].Addr())
	}

	if os.Getenv("LISTEN_PID") != "" || os.Getenv("LISTEN_FDS") != "" {
		t.Errorf("Failed to reset the environment")
	}
}

func TestManySockets(t *testing.T) {
	// Create two contiguous listeners.
	// The test environment does not guarantee us that they are contiguous, so
	// keep going until they are.
	var l0, l1 *net.TCPListener
	var f0, f1 int = -1, -3

	for f0+1 != f1 {
		// We have to be careful with the order of these operations, because
		// listenerFd will create *new* file descriptors.
		l0 = newListener(t)
		l1 = newListener(t)
		f0 = listenerFd(t, l0)
		f1 = listenerFd(t, l1)
		t.Logf("Looping for FDs: %d %d", f0, f1)
	}

	firstFD = f0

	setenv(strconv.Itoa(os.Getpid()), "2", "name1", "name2")

	lsMap, err := Listeners()
	if err != nil || len(lsMap) != 2 {
		t.Fatalf("Got an invalid result: %v // %v", lsMap, err)
	}

	ls := []net.Listener{
		lsMap["name1"][0],
		lsMap["name2"][0],
	}

	if !sameAddr(ls[0].Addr(), l0.Addr()) {
		t.Errorf("Listener 0 address mismatch, expected %#v, got %#v",
			l0.Addr(), ls[0].Addr())
	}

	if !sameAddr(ls[1].Addr(), l1.Addr()) {
		t.Errorf("Listener 1 address mismatch, expected %#v, got %#v",
			l1.Addr(), ls[1].Addr())
	}

	if os.Getenv("LISTEN_PID") != "" ||
		os.Getenv("LISTEN_FDS") != "" ||
		os.Getenv("LISTEN_FDNAMES") != "" {
		t.Errorf("Failed to reset the environment")
	}
}
