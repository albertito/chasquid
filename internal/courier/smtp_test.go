package courier

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/sts"
	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

// This domain will cause idna.ToASCII to fail.
var invalidDomain = "test " + strings.Repeat("x", 65536) + "\uff00"

// Override the netLookupMX function, to return controlled results for
// testing.
var testMX = map[string][]*net.MX{}
var testMXErr = map[string]error{}

func init() {
	netLookupMX = func(name string) ([]*net.MX, error) {
		return testMX[name], testMXErr[name]
	}
}

func newSMTP(t *testing.T) (*SMTP, string) {
	dir := testlib.MustTempDir(t)
	dinfo, err := domaininfo.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	return &SMTP{"hello", dinfo, nil}, dir
}

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

func TestSMTP(t *testing.T) {
	// Shorten the total timeout, so the test fails quickly if the protocol
	// gets stuck.
	smtpTotalTimeout = 5 * time.Second

	responses := map[string]string{
		"_welcome":          "220 welcome\n",
		"EHLO hello":        "250 ehlo ok\n",
		"MAIL FROM:<me@me>": "250 mail ok\n",
		"RCPT TO:<to@to>":   "250 rcpt ok\n",
		"DATA":              "354 send data\n",
		"_DATA":             "250 data ok\n",
		"QUIT":              "250 quit ok\n",
	}
	srv := newFakeServer(t, responses)
	defer srv.Cleanup()
	host, port := srv.HostPort()

	// Put a non-existing host first, so we check that if the first host
	// doesn't work, we try with the rest.
	// The host we use is invalid, to avoid having to do an actual network
	// lookup whick makes the test more hermetic. This is a hack, ideally we
	// would be able to override the default resolver, but Go does not
	// implement that yet.
	testMX["to"] = []*net.MX{
		{Host: ":::", Pref: 10},
		{Host: host, Pref: 20},
	}
	*smtpPort = port

	s, tmpDir := newSMTP(t)
	defer testlib.RemoveIfOk(t, tmpDir)
	err, _ := s.Deliver("me@me", "to@to", []byte("data"))
	if err != nil {
		t.Errorf("deliver failed: %v", err)
	}

	srv.Wait()
}

func TestSMTPErrors(t *testing.T) {
	// Shorten the total timeout, so the test fails quickly if the protocol
	// gets stuck.
	smtpTotalTimeout = 1 * time.Second

	responses := []map[string]string{
		// First test: hang response, should fail due to timeout.
		{
			"_welcome": "220 no newline",
		},

		// MAIL FROM not allowed.
		{
			"_welcome":          "220 mail from not allowed\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "501 mail error\n",
		},

		// RCPT TO not allowed.
		{
			"_welcome":          "220 rcpt to not allowed\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "501 rcpt error\n",
		},

		// DATA error.
		{
			"_welcome":          "220 data error\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "554 data error\n",
		},

		// DATA response error.
		{
			"_welcome":          "220 data response error\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "354 send data\n",
			"_DATA":             "551 data response error\n",
		},
	}

	for _, rs := range responses {
		srv := newFakeServer(t, rs)
		defer srv.Cleanup()
		host, port := srv.HostPort()

		testMX["to"] = []*net.MX{{Host: host, Pref: 10}}
		*smtpPort = port

		s, tmpDir := newSMTP(t)
		defer testlib.RemoveIfOk(t, tmpDir)
		err, _ := s.Deliver("me@me", "to@to", []byte("data"))
		if err == nil {
			t.Errorf("deliver not failed in case %q: %v", rs["_welcome"], err)
		}
		t.Logf("failed as expected: %v", err)

		srv.Wait()
	}
}

func TestNoMXServer(t *testing.T) {
	testMX["to"] = []*net.MX{}

	s, tmpDir := newSMTP(t)
	defer testlib.RemoveIfOk(t, tmpDir)
	err, permanent := s.Deliver("me@me", "to@to", []byte("data"))
	if err == nil {
		t.Errorf("delivery worked, expected failure")
	}
	if !permanent {
		t.Errorf("expected permanent failure, got transient (%v)", err)
	}
	t.Logf("got permanent failure, as expected: %v", err)
}

func TestTooManyMX(t *testing.T) {
	tr := trace.New("test", "test")
	testMX["domain"] = []*net.MX{
		{Host: "h1", Pref: 10}, {Host: "h2", Pref: 20},
		{Host: "h3", Pref: 30}, {Host: "h4", Pref: 40},
		{Host: "h5", Pref: 50}, {Host: "h5", Pref: 60},
	}
	mxs, err, perm := lookupMXs(tr, "domain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm != true {
		t.Fatalf("expected perm == true")
	}
	if len(mxs) != 5 {
		t.Errorf("expected len(mxs) == 5, got: %v", mxs)
	}
}

func TestFallbackToA(t *testing.T) {
	tr := trace.New("test", "test")
	testMX["domain"] = nil
	testMXErr["domain"] = &net.DNSError{
		Err:         "no such host (test)",
		IsTemporary: false,
		IsNotFound:  true,
	}

	mxs, err, perm := lookupMXs(tr, "domain")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if perm != true {
		t.Errorf("expected perm == true")
	}
	if !(len(mxs) == 1 && mxs[0] == "domain") {
		t.Errorf("expected mxs == [domain], got: %v", mxs)
	}
}

func TestTemporaryDNSerror(t *testing.T) {
	tr := trace.New("test", "test")
	testMX["domain"] = nil
	testMXErr["domain"] = &net.DNSError{
		Err:         "temp error (test)",
		IsTemporary: true,
	}

	mxs, err, perm := lookupMXs(tr, "domain")
	if !(mxs == nil && err == testMXErr["domain"]) {
		t.Errorf("expected mxs == nil, err == test error, got: %v, %v", mxs, err)
	}
	if perm != false {
		t.Errorf("expected perm == false")
	}
}

func TestMXLookupError(t *testing.T) {
	tr := trace.New("test", "test")
	testMX["domain"] = nil
	testMXErr["domain"] = fmt.Errorf("test error")

	mxs, err, perm := lookupMXs(tr, "domain")
	if !(mxs == nil && err == testMXErr["domain"]) {
		t.Errorf("expected mxs == nil, err == test error, got: %v, %v", mxs, err)
	}
	if perm != false {
		t.Errorf("expected perm == false")
	}
}

func TestLookupInvalidDomain(t *testing.T) {
	tr := trace.New("test", "test")

	mxs, err, perm := lookupMXs(tr, invalidDomain)
	if !(mxs == nil && err != nil) {
		t.Errorf("expected err != nil, got: %v, %v", mxs, err)
	}
	if perm != true {
		t.Fatalf("expected perm == true")
	}
}

// Server fake responses for a complete TLS delivery.
// We use this in a few tests, so make it common.
var tlsResponses = map[string]string{
	"_welcome":          "220 welcome\n",
	"EHLO hello":        "250-ehlo ok\n250 STARTTLS\n",
	"STARTTLS":          "220 starttls go\n",
	"_STARTTLS":         "ok",
	"MAIL FROM:<me@me>": "250 mail ok\n",
	"RCPT TO:<to@to>":   "250 rcpt ok\n",
	"DATA":              "354 send data\n",
	"_DATA":             "250 data ok\n",
	"QUIT":              "250 quit ok\n",
}

func TestTLS(t *testing.T) {
	smtpTotalTimeout = 5 * time.Second
	srv := newFakeServer(t, tlsResponses)
	defer srv.Cleanup()
	_, *smtpPort = srv.HostPort()

	testMX["to"] = []*net.MX{
		{Host: "localhost", Pref: 20},
	}

	s, tmpDir := newSMTP(t)
	defer testlib.RemoveIfOk(t, tmpDir)
	err, _ := s.Deliver("me@me", "to@to", []byte("data"))
	if err != nil {
		t.Errorf("deliver failed: %v", err)
	}

	srv.Wait()

	// Now do another delivery, but without TLS, to check that the detection
	// of connection downgrade is working.
	responses := map[string]string{
		"_welcome":          "220 welcome\n",
		"EHLO hello":        "250 ehlo ok\n",
		"MAIL FROM:<me@me>": "250 mail ok\n",
		"RCPT TO:<to@to>":   "250 rcpt ok\n",
		"DATA":              "354 send data\n",
		"_DATA":             "250 data ok\n",
		"QUIT":              "250 quit ok\n",
	}
	srv = newFakeServer(t, responses)
	defer srv.Cleanup()
	_, *smtpPort = srv.HostPort()

	err, permanent := s.Deliver("me@me", "to@to", []byte("data"))
	if !strings.Contains(err.Error(),
		"Security level check failed (level:PLAIN)") {
		t.Errorf("expected sec level check failed, got: %v", err)
	}
	if permanent != false {
		t.Errorf("expected transient failure, got permanent")
	}

	srv.Wait()
}

func TestTLSError(t *testing.T) {
	smtpTotalTimeout = 5 * time.Second

	responses := map[string]string{
		"_welcome":   "220 welcome\n",
		"EHLO hello": "250-ehlo ok\n250 STARTTLS\n",
		"STARTTLS":   "500 starttls err\n",
		"_STARTTLS":  "no",
	}
	srv := newFakeServer(t, responses)
	defer srv.Cleanup()
	_, *smtpPort = srv.HostPort()

	testMX["to"] = []*net.MX{
		{Host: "localhost", Pref: 20},
	}

	s, tmpDir := newSMTP(t)
	defer testlib.RemoveIfOk(t, tmpDir)
	err, permanent := s.Deliver("me@me", "to@to", []byte("data"))
	if !strings.Contains(err.Error(), "TLS error:") {
		t.Errorf("expected TLS error, got: %v", err)
	}
	if permanent != false {
		t.Errorf("expected transient failure, got permanent")
	}

	srv.Wait()
}

func TestSTSPolicyEnforcement(t *testing.T) {
	smtpTotalTimeout = 5 * time.Second
	srv := newFakeServer(t, tlsResponses)
	defer srv.Cleanup()
	_, *smtpPort = srv.HostPort()

	s, tmpDir := newSMTP(t)
	defer testlib.RemoveIfOk(t, tmpDir)

	a := &attempt{
		courier:  s,
		from:     "me@me",
		to:       "to@to",
		toDomain: "to",
		data:     []byte("data"),
		tr:       trace.New("test", "test"),
	}

	a.stsPolicy = &sts.Policy{
		Version: "STSv1",
		Mode:    sts.Enforce,
		MXs:     []string{"mx"},
		MaxAge:  1 * time.Minute,
	}

	// At this point the cert is not valid, which is incompatible with STS
	// policy, so we expect it to fail.
	err, permanent := a.deliver("localhost")
	if !strings.Contains(err.Error(),
		"invalid security level (TLS_INSECURE) for STS policy") {
		t.Errorf("expected invalid sec level error, got %v", err)
	}
	if permanent != false {
		t.Errorf("expected transient error, got permanent")
	}

	srv.Wait()

	// Do another delivery attempt, but this time we trust the server cert.
	// This time it should be successful, because the connection level should
	// be TLS_SECURE which is required by the STS policy.
	srv = newFakeServer(t, tlsResponses)
	_, *smtpPort = srv.HostPort()
	defer srv.Cleanup()

	certRoots = loadCert(t, srv.tmpDir+"/cert.pem")
	defer func() {
		certRoots = nil
	}()

	err, permanent = a.deliver("localhost")
	if err != nil {
		t.Errorf("expected success, got %v (permanent=%v)", err, permanent)
	}

	srv.Wait()
}

func loadCert(t *testing.T, path string) *x509.CertPool {
	t.Helper()

	pool := x509.NewCertPool()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("error reading cert %q: %v", path, err)
	}
	ok := pool.AppendCertsFromPEM(data)
	if !ok {
		t.Fatalf("failed to load cert %q", path)
	}
	return pool
}
