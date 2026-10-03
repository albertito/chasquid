package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/sts"
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

func newSMTP(t *testing.T) *SMTP {
	dinfo, err := domaininfo.New(t.ArtifactDir())
	if err != nil {
		t.Fatal(err)
	}

	return &SMTP{"hello", dinfo, nil}
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
	srv := newFakeServer(t, responses, 1)
	host, port := srv.HostPort()

	// Put a non-existing host first, so we check that if the first host
	// doesn't work, we try with the rest.
	// The host we use is invalid, to avoid having to do an actual network
	// lookup which makes the test more hermetic. This is a hack, ideally we
	// would be able to override the default resolver, but Go does not
	// implement that yet.
	testMX["to"] = []*net.MX{
		{Host: ":::", Pref: 10},
		{Host: host, Pref: 20},
	}
	*smtpPort = port

	s := newSMTP(t)
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

	cases := []struct {
		responses map[string]string
		wantErr   string
	}{
		// First test: hang response, should fail due to timeout.
		// Which step times out depends on timing, so just check that the
		// error mentions the server.
		{map[string]string{
			"_welcome": "220 no newline",
		}, ""},

		// Server rejects us in the greeting.
		{map[string]string{
			"_welcome": "554 go away\n",
		}, "greeting failed: 554 "},

		// MAIL FROM not allowed.
		{map[string]string{
			"_welcome":          "220 mail from not allowed\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "501 mail error\n",
		}, "MAIL/RCPT failed: 501 "},

		// RCPT TO not allowed.
		{map[string]string{
			"_welcome":          "220 rcpt to not allowed\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "501 rcpt error\n",
		}, "MAIL/RCPT failed: 501 "},

		// DATA error.
		{map[string]string{
			"_welcome":          "220 data error\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "554 data error\n",
		}, "DATA failed: 554 "},

		// DATA response error.
		{map[string]string{
			"_welcome":          "220 data response error\n",
			"EHLO hello":        "250 ehlo ok\n",
			"MAIL FROM:<me@me>": "250 mail ok\n",
			"RCPT TO:<to@to>":   "250 rcpt ok\n",
			"DATA":              "354 send data\n",
			"_DATA":             "551 data response error\n",
		}, "message not accepted: 551 "},
	}

	for _, c := range cases {
		srv := newFakeServer(t, c.responses, 1)
		host, port := srv.HostPort()

		testMX["to"] = []*net.MX{{Host: host, Pref: 10}}
		*smtpPort = port

		s := newSMTP(t)
		err, _ := s.Deliver("me@me", "to@to", []byte("data"))
		want := host + ": " + c.wantErr
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("case %q: expected error containing %q, got %v",
				c.responses["_welcome"], want, err)
		}
		t.Logf("failed as expected: %v", err)

		srv.Wait()
	}
}

func TestSMTPForward(t *testing.T) {
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
	srv := newFakeServer(t, responses, 1)
	host, port := srv.HostPort()
	*smtpPort = port

	s := newSMTP(t)

	// Successful forward. The first server is invalid (see TestSMTP), so we
	// also check that we try the next one.
	err, _ := s.Forward("me@me", "to@to", []byte("data"),
		[]string{":::", host})
	if err != nil {
		t.Errorf("forward failed: %v", err)
	}
	srv.Wait()

	// All servers fail with transient errors.
	err, permanent := s.Forward("me@me", "to@to", []byte("data"),
		[]string{":::"})
	want := "all servers failed temporarily, last error: :::: "
	if err == nil || !strings.HasPrefix(err.Error(), want) || permanent {
		t.Errorf("expected transient error starting with %q, got %v (%v)",
			want, err, permanent)
	}

	// Permanent failure: we return right away.
	responses["RCPT TO:<to@to>"] = "550 rcpt error\n"
	srv = newFakeServer(t, responses, 1)
	host, port = srv.HostPort()
	*smtpPort = port
	err, permanent = s.Forward("me@me", "to@to", []byte("data"),
		[]string{host})
	want = host + ": MAIL/RCPT failed: 550 "
	if err == nil || !strings.HasPrefix(err.Error(), want) || !permanent {
		t.Errorf("expected permanent error starting with %q, got %v (%v)",
			want, err, permanent)
	}
	srv.Wait()

	// No servers to forward to.
	err, permanent = s.Forward("me@me", "to@to", []byte("data"), nil)
	if err == nil || err.Error() != "no servers to forward to" || permanent {
		t.Errorf("expected transient 'no servers' error, got %v (%v)",
			err, permanent)
	}
}

// Test delivery to a domain whose MTA-STS policy doesn't allow any of its
// MXs.
func TestSTSNoMXAllowed(t *testing.T) {
	testMX["sts-none"] = []*net.MX{
		{Host: "mx1", Pref: 10},
		{Host: "mx2", Pref: 20},
	}
	t.Cleanup(func() { delete(testMX, "sts-none") })

	// Put the policy in the cache directly, so we don't need to fetch it.
	// This mimics the cache's on-disk format: the policy as JSON in a
	// "pol:<domain>" file, with the expiration time as its modification time.
	dir := t.ArtifactDir()
	policy := &sts.Policy{
		Version: "STSv1",
		Mode:    sts.Enforce,
		MXs:     []string{"other-mx"},
		MaxAge:  1 * time.Hour,
	}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	fname := dir + "/pol:sts-none"
	if err := os.WriteFile(fname, data, 0640); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(policy.MaxAge)
	if err := os.Chtimes(fname, expires, expires); err != nil {
		t.Fatal(err)
	}

	stsCache, err := sts.NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	s := newSMTP(t)
	s.STSCache = stsCache
	err, permanent := s.Deliver("me@me", "to@sts-none", []byte("data"))
	want := `none of the mail servers for "sts-none" are allowed by its ` +
		`MTA-STS policy`
	if err == nil || err.Error() != want {
		t.Errorf("expected error %q, got %v", want, err)
	}
	if permanent {
		t.Errorf("expected transient failure, got permanent")
	}
}

func TestNoMXServer(t *testing.T) {
	testMX["to"] = []*net.MX{}

	s := newSMTP(t)
	err, permanent := s.Deliver("me@me", "to@to", []byte("data"))
	if err == nil {
		t.Errorf("delivery worked, expected failure")
	}
	if !permanent {
		t.Errorf("expected permanent failure, got transient (%v)", err)
	}
	if want := `no mail servers found for "to"`; err.Error() != want {
		t.Errorf("expected error %q, got %q", want, err)
	}
}

func TestDeliverMXLookupError(t *testing.T) {
	dnsErr := &net.DNSError{
		Err:         "temp error (test)",
		Name:        "lookuperr",
		IsTemporary: true,
	}
	testMXErr["lookuperr"] = dnsErr
	t.Cleanup(func() { delete(testMXErr, "lookuperr") })

	s := newSMTP(t)
	err, permanent := s.Deliver("me@me", "to@lookuperr", []byte("data"))
	want := `error looking up mail servers for "lookuperr": ` +
		`lookup lookuperr: temp error (test)`
	if err == nil || err.Error() != want {
		t.Errorf("expected error %q, got %v", want, err)
	}
	if !errors.Is(err, dnsErr) {
		t.Errorf("expected error to wrap the DNS error, got %v", err)
	}
	if permanent {
		t.Errorf("expected transient failure, got permanent")
	}
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
	srv := newFakeServer(t, tlsResponses, 1)
	_, *smtpPort = srv.HostPort()

	testMX["to"] = []*net.MX{
		{Host: "localhost", Pref: 20},
	}

	s := newSMTP(t)
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
	srv = newFakeServer(t, responses, 1)
	_, *smtpPort = srv.HostPort()

	err, permanent := s.Deliver("me@me", "to@to", []byte("data"))
	if !strings.Contains(err.Error(),
		"security level check failed: connection is PLAIN") {
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
		"_welcome": "220 welcome\n",

		// STARTTLS should be advertised so we try to initiate it.
		"EHLO hello": "250-ehlo ok\n250 STARTTLS\n",

		// Error in STARTTLS request. Note that a TLS-layer error also falls
		// under this code path, so both situations are covered by this test.
		"STARTTLS":  "500 starttls err\n",
		"_STARTTLS": "no",

		// Rest of the transaction is normal and straightforward.
		"MAIL FROM:<me@me>": "250 mail ok\n",
		"RCPT TO:<to@to>":   "250 rcpt ok\n",
		"DATA":              "354 send data\n",
		"_DATA":             "250 data ok\n",
		"QUIT":              "250 quit ok\n",
	}
	// Note we expect 2 connections to the fake server (because of the retry
	// after the failed STARTTLS). Note this also checks that we correctly
	// close the errored connection, instead of leaving it lingering.
	srv := newFakeServer(t, responses, 2)
	_, *smtpPort = srv.HostPort()

	testMX["to"] = []*net.MX{
		{Host: "localhost", Pref: 20},
	}

	s := newSMTP(t)
	err, _ := s.Deliver("me@me", "to@to", []byte("data"))
	if err != nil {
		t.Errorf("deliver failed: %v", err)
	}

	// Double check that we delivered over a plaintext connection.
	tr := trace.New("test", "test")
	defer tr.Finish()
	if !s.Dinfo.OutgoingSecLevel(tr, "to", domaininfo.SecLevel_PLAIN) {
		t.Errorf("delivery did not took place over plaintext as expected")
	}

	srv.Wait()
}

func TestSTSPolicyEnforcement(t *testing.T) {
	smtpTotalTimeout = 5 * time.Second
	srv := newFakeServer(t, tlsResponses, 1)
	_, *smtpPort = srv.HostPort()

	s := newSMTP(t)

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
		"connection is TLS_INSECURE, but the MTA-STS policy") {
		t.Errorf("expected invalid sec level error, got %v", err)
	}
	if permanent != false {
		t.Errorf("expected transient error, got permanent")
	}

	srv.Wait()

	// Do another delivery attempt, but this time we trust the server cert.
	// This time it should be successful, because the connection level should
	// be TLS_SECURE which is required by the STS policy.
	srv = newFakeServer(t, tlsResponses, 1)
	_, *smtpPort = srv.HostPort()

	certRoots = srv.rootCA()
	defer func() {
		certRoots = nil
	}()

	err, permanent = a.deliver("localhost")
	if err != nil {
		t.Errorf("expected success, got %v (permanent=%v)", err, permanent)
	}

	srv.Wait()
}
