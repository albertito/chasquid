package localrpc

import (
	"bufio"
	"errors"
	"io/fs"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
)

func NewFakeServer(t *testing.T, path, output string) {
	t.Helper()
	lis, err := net.Listen("unix", path)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			panic(err)
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
	tmpDir, err := os.MkdirTemp("", "rpc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	socketPath := filepath.Join(tmpDir, "rpc.sock")

	// textproto client expects a numeric code, this should cause ReadCodeLine
	// to fail with textproto.ProtocolError.
	go NewFakeServer(t, socketPath, "xxx")
	waitForServer(t, socketPath)

	client := NewClient(socketPath)
	_, err = client.Call("Echo")
	if err == nil {
		t.Fatal("expected error")
	}
	var protoErr textproto.ProtocolError
	if !errors.As(err, &protoErr) {
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
