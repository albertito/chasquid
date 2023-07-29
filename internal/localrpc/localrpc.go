// Local RPC package.
//
// This is a simple RPC package that uses a line-oriented protocol for
// encoding and decoding, and Unix sockets for transport. It is meant to be
// used for lightweight occassional communication between processes on the
// same machine.
package localrpc

import (
	"errors"
	"net"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/trace"
)

// Handler is the type of RPC request handlers.
type Handler func(tr *trace.Trace, input url.Values) (url.Values, error)

//
// Server
//

// Server represents the RPC server.
type Server struct {
	handlers map[string]Handler
	lis      net.Listener
}

// NewServer creates a new local RPC server.
func NewServer() *Server {
	return &Server{
		handlers: make(map[string]Handler),
	}
}

var errUnknownMethod = errors.New("unknown method")

// Register a handler for the given name.
func (s *Server) Register(name string, handler Handler) {
	s.handlers[name] = handler
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe(path string) error {
	tr := trace.New("LocalRPC.Server", path)
	defer tr.Finish()

	// Previous instances of the server may have shut down uncleanly, leaving
	// behind the socket file. Remove it just in case.
	os.Remove(path)

	var err error
	s.lis, err = net.Listen("unix", path)
	if err != nil {
		return err
	}

	tr.Printf("Listening")
	for {
		conn, err := s.lis.Accept()
		if err != nil {
			tr.Errorf("Accept error: %v", err)
			return err
		}
		go s.handleConn(tr, conn)
	}
}

// Close stops the server.
func (s *Server) Close() error {
	return s.lis.Close()
}

func (s *Server) handleConn(tr *trace.Trace, conn net.Conn) {
	tr = tr.NewChild("LocalRPC.Handle", conn.RemoteAddr().String())
	defer tr.Finish()

	// Set a generous deadline to prevent client issues from tying up a server
	// goroutine indefinitely.
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	tconn := textproto.NewConn(conn)
	defer tconn.Close()

	// Read the request.
	name, inS, err := readRequest(&tconn.Reader)
	if err != nil {
		tr.Debugf("error reading request: %v", err)
		return
	}
	tr.Debugf("<- %s %s", name, inS)

	// Find the handler.
	handler, ok := s.handlers[name]
	if !ok {
		writeError(tr, tconn, errUnknownMethod)
		return
	}

	// Unmarshal the input.
	inV, err := url.ParseQuery(inS)
	if err != nil {
		writeError(tr, tconn, err)
		return
	}

	// Call the handler.
	outV, err := handler(tr, inV)
	if err != nil {
		writeError(tr, tconn, err)
		return
	}

	// Send the response.
	outS := outV.Encode()
	tr.Debugf("-> 200 %s", outS)
	tconn.PrintfLine("200 %s", outS)
}

func readRequest(r *textproto.Reader) (string, string, error) {
	line, err := r.ReadLine()
	if err != nil {
		return "", "", err
	}

	sp := strings.SplitN(line, " ", 2)
	if len(sp) == 1 {
		return sp[0], "", nil
	}
	return sp[0], sp[1], nil
}

func writeError(tr *trace.Trace, tconn *textproto.Conn, err error) {
	tr.Errorf("-> 500 %s", err.Error())
	tconn.PrintfLine("500 %s", err.Error())
}

// Default server. This is a singleton server that can be used for
// convenience.
var DefaultServer = NewServer()

//
// Client
//

// Client for the localrpc server.
type Client struct {
	path string
}

// NewClient creates a new client for the given path.
func NewClient(path string) *Client {
	return &Client{path: path}
}

// CallWithValues calls the given method.
func (c *Client) CallWithValues(name string, input url.Values) (url.Values, error) {
	conn, err := textproto.Dial("unix", c.path)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	err = conn.PrintfLine("%s %s", name, input.Encode())
	if err != nil {
		return nil, err
	}

	code, msg, err := conn.ReadCodeLine(0)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, errors.New(msg)
	}

	return url.ParseQuery(msg)
}

// Call the given method. The arguments are key-value strings, and must be
// provided in pairs.
func (c *Client) Call(name string, args ...string) (url.Values, error) {
	v := url.Values{}
	for i := 0; i < len(args); i += 2 {
		v.Set(args[i], args[i+1])
	}
	return c.CallWithValues(name, v)
}
