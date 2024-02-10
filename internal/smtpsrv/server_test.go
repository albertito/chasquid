package smtpsrv

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/chasquid/internal/userdb"
)

// Flags.
var (
	externalSMTPAddr = flag.String("external_smtp_addr", "",
		"SMTP server address to test (defaults to use internal)")
	externalSubmissionAddr = flag.String("external_submission_addr", "",
		"submission server address to test (defaults to use internal)")
	externalSubmissionTLSAddr = flag.String("external_submission_tls_addr", "",
		"submission+TLS server address to test (defaults to use internal)")
)

var (
	// Server addresses. Will be filled in at init time.
	// We default to internal ones, but may get overridden via flags.
	smtpAddr          = ""
	submissionAddr    = ""
	submissionTLSAddr = ""

	// TLS configuration to use in the clients.
	// Will contain the generated server certificate as root CA.
	tlsConfig *tls.Config

	// Test couriers, so we can validate that emails got sent.
	localC  = testlib.NewTestCourier()
	remoteC = testlib.NewTestCourier()

	// Max data size, in MiB.
	maxDataSizeMiB = 5
)

//
// === Tests ===
//

func mustDial(tb testing.TB, mode SocketMode, startTLS bool) *smtp.Client {
	addr := ""
	switch mode {
	case ModeSMTP:
		addr = smtpAddr
	case ModeSubmission:
		addr = submissionAddr
	case ModeSubmissionTLS:
		addr = submissionTLSAddr
	}

	var err error
	var conn net.Conn
	if mode.TLS {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		tb.Fatalf("(net||tls).Dial: %v", err)
	}
	c, err := smtp.NewClient(conn, "127.0.0.1")
	if err != nil {
		tb.Fatalf("smtp.Dial: %v", err)
	}

	if err = c.Hello("test"); err != nil {
		tb.Fatalf("c.Hello: %v", err)
	}

	if startTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			tb.Fatalf("STARTTLS not advertised in EHLO")
		}

		if err = c.StartTLS(tlsConfig); err != nil {
			tb.Fatalf("StartTLS: %v", err)
		}
	}

	return c
}

func sendEmail(tb testing.TB, c *smtp.Client) {
	sendEmailWithAuth(tb, c, nil)
}

func sendEmailWithAuth(tb testing.TB, c *smtp.Client, auth smtp.Auth) {
	var err error
	from := "from@from"

	if auth != nil {
		if err = c.Auth(auth); err != nil {
			tb.Errorf("Auth: %v", err)
		}

		// If we authenticated, we must use the user as from, as the server
		// checks otherwise.
		from = "testuser@localhost"
	}

	if err = c.Mail(from); err != nil {
		tb.Errorf("Mail: %v", err)
	}

	if err = c.Rcpt("to@localhost"); err != nil {
		tb.Errorf("Rcpt: %v", err)
	}

	w, err := c.Data()
	if err != nil {
		tb.Fatalf("Data: %v", err)
	}

	msg := []byte("Subject: Hi!\n\n This is an email\n")
	if _, err = w.Write(msg); err != nil {
		tb.Errorf("Data write: %v", err)
	}

	localC.Expect(1)

	if err = w.Close(); err != nil {
		tb.Errorf("Data close: %v", err)
	}

	localC.Wait()
}

func TestSimple(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()
	sendEmail(t, c)
}

func TestSimpleTLS(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()
	sendEmail(t, c)
}

func TestManyEmails(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()
	sendEmail(t, c)
	sendEmail(t, c)
	sendEmail(t, c)
}

func TestAuth(t *testing.T) {
	c := mustDial(t, ModeSubmission, true)
	defer c.Close()

	auth := smtp.PlainAuth("", "testuser@localhost", "testpasswd", "127.0.0.1")
	sendEmailWithAuth(t, c, auth)
}

func TestSubmissionWithoutAuth(t *testing.T) {
	c := mustDial(t, ModeSubmission, true)
	defer c.Close()

	if err := c.Mail("from@from"); err == nil {
		t.Errorf("Mail not failed as expected")
	}
}

func TestAuthOnTLS(t *testing.T) {
	c := mustDial(t, ModeSubmissionTLS, false)
	defer c.Close()

	auth := smtp.PlainAuth("", "testuser@localhost", "testpasswd", "127.0.0.1")
	sendEmailWithAuth(t, c, auth)
}

func TestAuthOnSMTP(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	auth := smtp.PlainAuth("", "testuser@localhost", "testpasswd", "127.0.0.1")

	// At least for now, we allow AUTH over the SMTP port to avoid unnecessary
	// complexity, so we expect it to work.
	sendEmailWithAuth(t, c, auth)
}

func TestBrokenAuth(t *testing.T) {
	c := mustDial(t, ModeSubmission, true)
	defer c.Close()

	auth := smtp.PlainAuth("", "user@broken", "passwd", "127.0.0.1")
	err := c.Auth(auth)
	if err == nil {
		t.Errorf("Broken auth succeeded")
	} else if err.Error() != "454 4.7.0 Temporary authentication failure" {
		t.Errorf("Broken auth returned unexpected error %q", err.Error())
	}
}

func TestWrongMailParsing(t *testing.T) {
	addrs := []string{"from", "a b c", "a @ b", "<x>", "<x y>", "><"}
	for _, addr := range addrs {
		c := mustDial(t, ModeSMTP, false)

		if err := c.Mail(addr); err == nil {
			t.Errorf("Mail not failed as expected with %q", addr)
		}

		if err := c.Mail("from@plain"); err != nil {
			t.Errorf("Mail: %v", err)
		}

		for _, addr := range addrs {
			if err := c.Rcpt(addr); err == nil {
				t.Errorf("Rcpt not failed as expected with %q", addr)
			}
		}

		c.Close()
	}
}

func TestNullMailFrom(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	addrs := []string{"<>", "  <>", "<> OPTION"}
	for _, addr := range addrs {
		simpleCmd(t, c, fmt.Sprintf("MAIL FROM:%s", addr), 250)
	}
}

func TestRcptBeforeMail(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	if err := c.Rcpt("to@to"); err == nil {
		t.Errorf("Rcpt not failed as expected")
	}
}

func TestRcptOption(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	if err := c.Mail("from@localhost"); err != nil {
		t.Fatalf("Mail: %v", err)
	}

	params := []string{
		"<to@localhost>", "  <to@localhost>", "<to@localhost> OPTION"}
	for _, p := range params {
		simpleCmd(t, c, fmt.Sprintf("RCPT TO:%s", p), 250)
	}
}

func TestRelayForbidden(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	if err := c.Mail("from@somewhere"); err != nil {
		t.Errorf("Mail: %v", err)
	}

	if err := c.Rcpt("to@somewhere"); err == nil {
		t.Errorf("Accepted relay email")
	}
}

func TestTooManyRecipients(t *testing.T) {
	c := mustDial(t, ModeSubmission, true)
	defer c.Close()

	auth := smtp.PlainAuth("", "testuser@localhost", "testpasswd", "127.0.0.1")
	if err := c.Auth(auth); err != nil {
		t.Fatalf("Auth: %v", err)
	}

	if err := c.Mail("testuser@localhost"); err != nil {
		t.Fatalf("Mail: %v", err)
	}

	for i := 0; i < 101; i++ {
		if err := c.Rcpt(fmt.Sprintf("to%d@somewhere", i)); err != nil {
			t.Fatalf("Rcpt: %v", err)
		}
	}

	err := c.Rcpt("to102@somewhere")
	if err == nil || err.Error() != "452 4.5.3 Too many recipients" {
		t.Errorf("Expected too many recipients, got: %v", err)
	}
}

func TestRcptBrokenExists(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	if err := c.Mail("from@localhost"); err != nil {
		t.Fatalf("Mail: %v", err)
	}

	err := c.Rcpt("to@broken")
	if err == nil {
		t.Errorf("Accepted RCPT with broken Exists")
	}
	expect := "451 4.4.3 Temporary error checking address"
	if err.Error() != expect {
		t.Errorf("RCPT returned unexpected error %q", err.Error())
	}
}

func TestRcptUserDoesNotExist(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	if err := c.Mail("from@localhost"); err != nil {
		t.Fatalf("Mail: %v", err)
	}

	err := c.Rcpt("doesnotexist@localhost")
	if err == nil {
		t.Errorf("Accepted RCPT for non-existent user")
	}
	expect := "550 5.1.1 Destination address is unknown (user does not exist)"
	if err.Error() != expect {
		t.Errorf("RCPT returned unexpected error %q", err.Error())
	}
}

var str1MiB string

func sendLargeEmail(tb testing.TB, c *smtp.Client, sizeMiB int) error {
	tb.Helper()
	if err := c.Mail("from@from"); err != nil {
		tb.Fatalf("Mail: %v", err)
	}
	if err := c.Rcpt("to@localhost"); err != nil {
		tb.Fatalf("Rcpt: %v", err)
	}

	w, err := c.Data()
	if err != nil {
		tb.Fatalf("Data: %v", err)
	}

	if _, err := w.Write([]byte("Subject: I ate too much\n\n")); err != nil {
		tb.Fatalf("Data write: %v", err)
	}

	// Write the 1 MiB string sizeMiB times.
	for i := 0; i < sizeMiB; i++ {
		if _, err := w.Write([]byte(str1MiB)); err != nil {
			tb.Fatalf("Data write: %v", err)
		}
	}

	return w.Close()
}

func TestTooMuchData(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	localC.Expect(1)
	err := sendLargeEmail(t, c, maxDataSizeMiB-1)
	if err != nil {
		t.Errorf("Error sending large but ok email: %v", err)
	}
	localC.Wait()

	// Repeat the test - we want to check that the limit applies to each
	// message, not the entire connection.
	localC.Expect(1)
	err = sendLargeEmail(t, c, maxDataSizeMiB-1)
	if err != nil {
		t.Errorf("Error sending large but ok email: %v", err)
	}
	localC.Wait()

	err = sendLargeEmail(t, c, maxDataSizeMiB+1)
	if err == nil || err.Error() != "552 5.3.4 Message too big" {
		t.Fatalf("Expected message too big, got: %v", err)
	}

	// Repeat the test once again, the limit should not prevent connection
	// from continuing.
	localC.Expect(1)
	err = sendLargeEmail(t, c, maxDataSizeMiB-1)
	if err != nil {
		t.Errorf("Error sending large but ok email: %v", err)
	}
	localC.Wait()
}

func simpleCmd(t *testing.T, c *smtp.Client, cmd string, expected int) string {
	t.Helper()
	if err := c.Text.PrintfLine(cmd); err != nil {
		t.Fatalf("Failed to write %s: %v", cmd, err)
	}

	_, msg, err := c.Text.ReadResponse(expected)
	if err != nil {
		t.Errorf("Incorrect %s response: %v", cmd, err)
	}
	return msg
}

func TestSimpleCommands(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()
	simpleCmd(t, c, "HELP", 214)
	simpleCmd(t, c, "NOOP", 250)
	simpleCmd(t, c, "VRFY", 502)
	simpleCmd(t, c, "EXPN", 502)
}

func TestLongLines(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	// Send a not-too-long line.
	simpleCmd(t, c, fmt.Sprintf("%1000s", "x"), 500)

	// Send a very long line, expect an error.
	msg := simpleCmd(t, c, fmt.Sprintf("%1001s", "x"), 554)
	if msg != "error reading command: line too long" {
		t.Errorf("Expected 'line too long', got %v", msg)
	}
}

func TestReset(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	if err := c.Mail("from@plain"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}

	if err := c.Reset(); err != nil {
		t.Errorf("RSET: %v", err)
	}

	if err := c.Mail("from@plain"); err != nil {
		t.Errorf("MAIL after RSET: %v", err)
	}
}

func TestRepeatedStartTLS(t *testing.T) {
	c, err := smtp.Dial(smtpAddr)
	if err != nil {
		t.Fatalf("smtp.Dial: %v", err)
	}

	if err = c.StartTLS(tlsConfig); err != nil {
		t.Fatalf("StartTLS: %v", err)
	}

	if err = c.StartTLS(tlsConfig); err == nil {
		t.Errorf("Second STARTTLS did not fail as expected")
	}
}

// Test that STARTTLS fails on a TLS connection.
func TestStartTLSOnTLS(t *testing.T) {
	c := mustDial(t, ModeSubmissionTLS, false)
	defer c.Close()

	if err := c.StartTLS(tlsConfig); err == nil {
		t.Errorf("STARTTLS did not fail as expected")
	}
}

func TestAddDKIMSigner(t *testing.T) {
	s := NewServer()
	err := s.AddDKIMSigner("example.com", "selector", "keyfile-does-not-exist")
	if !os.IsNotExist(err) {
		t.Errorf("AddDKIMSigner: expected not exist, got %v", err)
	}

	tmpDir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, tmpDir)

	// Invalid PEM file.
	kf1 := tmpDir + "/key1-bad_pem.pem"
	testlib.Rewrite(t, kf1, "not a valid PEM file")
	err = s.AddDKIMSigner("example.com", "selector", kf1)
	if !errors.Is(err, errDecodingPEMBlock) {
		t.Errorf("AddDKIMSigner: expected %v, got %v",
			errDecodingPEMBlock, err)
	}

	// Unsupported block type.
	kf2 := tmpDir + "/key2.pem"
	testlib.Rewrite(t, kf2,
		"-----BEGIN TEST KEY-----\n-----END TEST KEY-----")
	err = s.AddDKIMSigner("example.com", "selector", kf2)
	if !errors.Is(err, errUnsupportedBlockType) {
		t.Errorf("AddDKIMSigner: expected %v, got %v",
			errUnsupportedBlockType, err)
	}

	// x509 error: this is an ed448 key, which is not supported.
	kf3 := tmpDir + "/key3.pem"
	testlib.Rewrite(t, kf3, `-----BEGIN PRIVATE KEY-----
MEcCAQAwBQYDK2VxBDsEOSBHT9DNG6/FNBnRGrLay+jIrK8WrViiVMz9AoXqYSb6
ghwTZSd3E0X8oIFTgs9ch3pxJM1KDrs4NA==
-----END PRIVATE KEY-----`)
	err = s.AddDKIMSigner("example.com", "selector", kf3)
	if !strings.Contains(err.Error(),
		"x509: PKCS#8 wrapping contained private key with unknown algorithm") {
		t.Errorf("AddDKIMSigner: expected x509 error, got %q", err.Error())
	}

	// Unsupported key type: X25519.
	kf4 := tmpDir + "/key4.pem"
	testlib.Rewrite(t, kf4, `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VuBCIEIKBUDwEDc5cCv/yEvnA93yk0gXyiTZe7Qip8QU3rJuZC
-----END PRIVATE KEY-----`)
	err = s.AddDKIMSigner("example.com", "selector", kf4)
	if !errors.Is(err, errUnsupportedKeyType) {
		t.Errorf("AddDKIMSigner: expected %v, got %v",
			errUnsupportedKeyType, err)
	}

	// Successful.
	kf5 := tmpDir + "/key5.pem"
	testlib.Rewrite(t, kf5, `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEID6bjSoiW6g6NJA67RNl0SZ7zpylVOq9w/VGAXF5whnS
-----END PRIVATE KEY-----`)
	err = s.AddDKIMSigner("example.com", "selector", kf5)
	if err != nil {
		t.Errorf("AddDKIMSigner: %v", err)
	}
}

//
// === Benchmarks ===
//

func BenchmarkManyEmails(b *testing.B) {
	c := mustDial(b, ModeSMTP, false)
	defer c.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendEmail(b, c)
	}
}

func BenchmarkManyEmailsParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		c := mustDial(b, ModeSMTP, false)
		defer c.Close()

		for pb.Next() {
			sendEmail(b, c)
		}
	})
}

//
// === Test environment ===
//

// waitForServer waits 5 seconds for the server to start, and returns an error
// if it fails to do so.
// It does this by repeatedly connecting to the address until it either
// replies or times out. Note we do not do any validation of the reply.
func waitForServer(addr string) error {
	start := time.Now()
	for time.Since(start) < 10*time.Second {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("not reachable")
}

type brokenAuthBE struct{}

func (b brokenAuthBE) Authenticate(user, password string) (bool, error) {
	return false, fmt.Errorf("failed to auth")
}

func (b brokenAuthBE) Exists(user string) (bool, error) {
	return false, fmt.Errorf("failed to check if user exists")
}

func (b brokenAuthBE) Reload() error {
	return fmt.Errorf("failed to reload")
}

// realMain is the real main function, which returns the value to pass to
// os.Exit(). We have to do this so we can use defer.
func realMain(m *testing.M) int {
	flag.Parse()

	// Create a 1MiB string, which the large message tests use.
	buf := make([]byte, 1024*1024)
	for i := 0; i < len(buf); i++ {
		buf[i] = 'a'
	}
	str1MiB = string(buf)

	// Set up the mail log to stdout, which is captured by the test runner,
	// so we have better debugging information on failures.
	maillog.Default = maillog.New(os.Stdout)

	if *externalSMTPAddr != "" {
		smtpAddr = *externalSMTPAddr
		submissionAddr = *externalSubmissionAddr
		submissionTLSAddr = *externalSubmissionTLSAddr
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {
		// Generate certificates in a temporary directory.
		tmpDir, err := os.MkdirTemp("", "chasquid_test:")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", tmpDir)
			return 1
		}
		defer os.RemoveAll(tmpDir)

		tlsConfig, err = testlib.GenerateCert(tmpDir)
		if err != nil {
			fmt.Printf("Failed to generate cert for testing: %v\n", err)
			return 1
		}

		smtpAddr = testlib.GetFreePort()
		submissionAddr = testlib.GetFreePort()
		submissionTLSAddr = testlib.GetFreePort()

		s := NewServer()
		s.Hostname = "localhost"
		s.MaxDataSize = int64(maxDataSizeMiB) * 1024 * 1024
		s.AddCerts(tmpDir+"/cert.pem", tmpDir+"/key.pem")
		s.AddAddr(smtpAddr, ModeSMTP)
		s.AddAddr(submissionAddr, ModeSubmission)
		s.AddAddr(submissionTLSAddr, ModeSubmissionTLS)

		s.InitQueue(tmpDir+"/queue", localC, remoteC)

		dinfo, err := domaininfo.New(tmpDir + "/domaininfo")
		if err != nil {
			fmt.Printf("Error initializing domaininfo: %v", err)
			return 1
		}
		s.SetDomainInfo(dinfo)

		udb := userdb.New("/dev/null")
		udb.AddUser("testuser", "testpasswd")
		s.aliasesR.AddAliasForTesting(
			"to@localhost", "testuser@localhost", aliases.EMAIL)
		s.AddDomain("localhost")
		s.AddUserDB("localhost", udb)

		s.AddDomain("broken")
		s.authr.Register("broken", &brokenAuthBE{})

		// Disable SPF lookups, to avoid leaking DNS queries.
		disableSPFForTesting = true

		// Disable reloading.
		reloadEvery = nil

		go s.ListenAndServe()
	}

	waitForServer(smtpAddr)
	waitForServer(submissionAddr)
	waitForServer(submissionTLSAddr)
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(realMain(m))
}
