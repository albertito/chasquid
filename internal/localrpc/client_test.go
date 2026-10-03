package localrpc

import (
	"bufio"
	"errors"
	"io/fs"
	"net"
	"net/textproto"
	"path/filepath"
	"testing"
)

// NewFakeServer starts a fake server listening on the given path, which
// replies with the given output to every request.
func NewFakeServer(t *testing.T, path, output string) {
	t.Helper()
	lis, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lis.Close() })

	go fakeServe(t, lis, output)
}

func fakeServe(t *testing.T, lis net.Listener, output string) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			// Listener closed at the end of the test.
			return
		}
		t.Logf("FakeServer %v: accepted ", conn)

		name, inS, err := readRequest(
			textproto.NewReader(bufio.NewReader(conn)))
		t.Logf("FakeServer %v: readRequest: %q %q / %v", conn, name, inS, err)

		n, err := conn.Write([]byte(output))
		t.Logf("FakeServer %v: writeMessage(%q): %d %v",
			conn, output, n, err)

		t.Logf("FakeServer %v: closing", conn)
		conn.Close()
	}
}

func TestBadServer(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "rpc.sock")

	// textproto client expects a numeric code, this should cause ReadCodeLine
	// to fail with textproto.ProtocolError.
	NewFakeServer(t, socketPath, "xxx")

	client := NewClient(socketPath)
	_, err := client.Call("Echo")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := errors.AsType[textproto.ProtocolError](err); !ok {
		t.Errorf("wanted textproto.ProtocolError, got: %v (%T)", err, err)
	}
}

func TestBadSocket(t *testing.T) {
	c := NewClient("/does/not/exist")
	_, err := c.Call("Echo")

	opErr, ok := err.(*net.OpError)
	if !ok {
		t.Fatalf("expected net.OpError, got %q (%T)", err, err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("wanted ErrNotExist, got: %q (%T)", opErr.Err, opErr.Err)
	}
}
