package courier

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/textproto"
	"os"
	"sync"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
)

// Fake server, to test SMTP out.
type FakeServer struct {
	t         *testing.T
	tmpDir    string
	responses map[string]string
	wg        *sync.WaitGroup
	addr      string
	tlsConfig *tls.Config
}

func newFakeServer(t *testing.T, responses map[string]string) *FakeServer {
	s := &FakeServer{
		t:         t,
		tmpDir:    testlib.MustTempDir(t),
		responses: responses,
		wg:        &sync.WaitGroup{},
	}
	s.start()
	return s
}

func (s *FakeServer) Cleanup() {
	// Remove our temporary data. Be extra paranoid and make sure the
	// directory isn't too shallow.
	if len(s.tmpDir) > 8 {
		os.RemoveAll(s.tmpDir)
	}
}

func (s *FakeServer) initTLS() {
	var err error
	s.tlsConfig, err = testlib.GenerateCert(s.tmpDir)
	if err != nil {
		s.t.Fatalf("error generating cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(s.tmpDir+"/cert.pem", s.tmpDir+"/key.pem")
	if err != nil {
		s.t.Fatalf("error loading temp cert: %v", err)
	}

	s.tlsConfig.Certificates = []tls.Certificate{cert}
}

func (s *FakeServer) rootCA() *x509.CertPool {
	s.t.Helper()
	pool := x509.NewCertPool()
	path := s.tmpDir + "/cert.pem"
	data, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatalf("error reading cert %q: %v", path, err)
	}
	ok := pool.AppendCertsFromPEM(data)
	if !ok {
		s.t.Fatalf("failed to load cert %q", path)
	}
	return pool
}

func (s *FakeServer) start() string {
	s.t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		s.t.Fatalf("fake server listen: %v", err)
	}
	s.addr = l.Addr().String()

	s.initTLS()

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer l.Close()

		c, err := l.Accept()
		if err != nil {
			panic(err)
		}
		defer c.Close()

		s.t.Logf("fakeServer got connection")

		r := textproto.NewReader(bufio.NewReader(c))
		c.Write([]byte(s.responses["_welcome"]))
		for {
			line, err := r.ReadLine()
			if err != nil {
				s.t.Logf("fakeServer exiting: %v\n", err)
				return
			}

			s.t.Logf("fakeServer read: %q\n", line)
			if line == "STARTTLS" && s.responses["_STARTTLS"] == "ok" {
				c.Write([]byte(s.responses["STARTTLS"]))

				tlssrv := tls.Server(c, s.tlsConfig)
				err = tlssrv.Handshake()
				if err != nil {
					s.t.Logf("starttls handshake error: %v", err)
					return
				}

				// Replace the connection with the wrapped one.
				// Don't send a reply, as per the protocol.
				c = tlssrv
				defer c.Close()
				r = textproto.NewReader(bufio.NewReader(c))
				continue
			}

			c.Write([]byte(s.responses[line]))
			if line == "DATA" {
				_, err = r.ReadDotBytes()
				if err != nil {
					s.t.Logf("fakeServer exiting: %v\n", err)
					return
				}
				c.Write([]byte(s.responses["_DATA"]))
			}
		}
	}()

	return s.addr
}

func (s *FakeServer) HostPort() (string, string) {
	host, port, _ := net.SplitHostPort(s.addr)
	return host, port
}

func (s *FakeServer) Wait() {
	s.wg.Wait()
}
