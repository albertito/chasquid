package localrpc

import (
	"bufio"
	"bytes"
	"net"
	"net/textproto"
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/trace"
)

func TestListenError(t *testing.T) {
	server := NewServer()
	err := server.ListenAndServe("/dev/null")
	if err == nil {
		t.Errorf("ListenAndServe(/dev/null) = nil, want error")
	}
}

// Test that the server can handle a broken client sending a bad request.
func TestServerBadRequest(t *testing.T) {
	server := NewServer()
	server.Register("Echo", Echo)

	srvConn, cliConn := net.Pipe()
	defer srvConn.Close()
	defer cliConn.Close()

	// Client sends an invalid request.
	go cliConn.Write([]byte("Echo this is an ; invalid ; query\n"))

	// Servers will handle the connection, and should return an error.
	tr := trace.New("test", "TestBadRequest")
	defer tr.Finish()
	go server.handleConn(tr, srvConn)

	// Read the error that the server should have sent.
	code, msg, err := textproto.NewConn(cliConn).ReadResponse(0)
	if err != nil {
		t.Errorf("ReadResponse error: %q", err)
	}
	if code != 500 {
		t.Errorf("ReadResponse code %d, expected 500", code)
	}
	if !strings.Contains(msg, "invalid semicolon separator") {
		t.Errorf("ReadResponse message %q, does not contain 'invalid semicolon separator'", msg)
	}
}

func TestShortReadRequest(t *testing.T) {
	// This request is too short, it does not have any arguments.
	// This does not happen with the real client, but just in case.
	buf := bufio.NewReader(bytes.NewReader([]byte("Method\n")))
	method, args, err := readRequest(textproto.NewReader(buf))
	if err != nil {
		t.Errorf("readRequest error: %v", err)
	}
	if method != "Method" {
		t.Errorf("readRequest method %q, expected 'Method'", method)
	}
	if args != "" {
		t.Errorf("readRequest args %q, expected ''", args)
	}
}
