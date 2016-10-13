package smtpsrv

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	"github.com/golang/glog"
)

// Flags.
var (
	externalSMTPAddr = flag.String("external_smtp_addr", "",
		"SMTP server address to test (defaults to use internal)")
	externalSubmissionAddr = flag.String("external_submission_addr", "",
		"submission server address to test (defaults to use internal)")
)

var (
	// Server addresses.
	// We default to internal ones, but may get overriden via flags.
	// TODO: Don't hard-code the default.
	smtpAddr       = "127.0.0.1:13444"
	submissionAddr = "127.0.0.1:13999"

	// TLS configuration to use in the clients.
	// Will contain the generated server certificate as root CA.
	tlsConfig *tls.Config
)

//
// === Tests ===
//

func mustDial(tb testing.TB, mode SocketMode, useTLS bool) *smtp.Client {
	addr := ""
	if mode == ModeSMTP {
		addr = smtpAddr
	} else {
		addr = submissionAddr
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		tb.Fatalf("smtp.Dial: %v", err)
	}

	if err = c.Hello("test"); err != nil {
		tb.Fatalf("c.Hello: %v", err)
	}

	if useTLS {
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

	if err = w.Close(); err != nil {
		tb.Errorf("Data close: %v", err)
	}
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

func TestAuthOnSMTP(t *testing.T) {
	c := mustDial(t, ModeSMTP, true)
	defer c.Close()

	auth := smtp.PlainAuth("", "testuser@localhost", "testpasswd", "127.0.0.1")

	// At least for now, we allow AUTH over the SMTP port to avoid unnecessary
	// complexity, so we expect it to work.
	sendEmailWithAuth(t, c, auth)
}

func TestWrongMailParsing(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()

	addrs := []string{"from", "a b c", "a @ b", "<x>", "<x y>", "><"}

	for _, addr := range addrs {
		if err := c.Mail(addr); err == nil {
			t.Errorf("Mail not failed as expected with %q", addr)
		}
	}

	if err := c.Mail("from@plain"); err != nil {
		t.Errorf("Mail: %v", err)
	}

	for _, addr := range addrs {
		if err := c.Rcpt(addr); err == nil {
			t.Errorf("Rcpt not failed as expected with %q", addr)
		}
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

func simpleCmd(t *testing.T, c *smtp.Client, cmd string, expected int) {
	if err := c.Text.PrintfLine(cmd); err != nil {
		t.Fatalf("Failed to write %s: %v", cmd, err)
	}

	if _, _, err := c.Text.ReadResponse(expected); err != nil {
		t.Errorf("Incorrect %s response: %v", cmd, err)
	}
}

func TestSimpleCommands(t *testing.T) {
	c := mustDial(t, ModeSMTP, false)
	defer c.Close()
	simpleCmd(t, c, "HELP", 214)
	simpleCmd(t, c, "NOOP", 250)
	simpleCmd(t, c, "VRFY", 252)
	simpleCmd(t, c, "EXPN", 252)
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

//
// === Benchmarks ===
//

func BenchmarkManyEmails(b *testing.B) {
	c := mustDial(b, ModeSMTP, false)
	defer c.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendEmail(b, c)

		// TODO: Make sendEmail() wait for delivery, and remove this.
		time.Sleep(10 * time.Millisecond)
	}
}

func BenchmarkManyEmailsParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		c := mustDial(b, ModeSMTP, false)
		defer c.Close()

		for pb.Next() {
			sendEmail(b, c)

			// TODO: Make sendEmail() wait for delivery, and remove this.
			time.Sleep(100 * time.Millisecond)
		}
	})
}

//
// === Test environment ===
//

// generateCert generates a new, INSECURE self-signed certificate and writes
// it to a pair of (cert.pem, key.pem) files to the given path.
// Note the certificate is only useful for testing purposes.
func generateCert(path string) error {
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1234),
		Subject: pkix.Name{
			Organization: []string{"chasquid_test.go"},
		},

		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},

		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(30 * time.Minute),

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA: true,
	}

	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return err
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// Create a global config for convenience.
	srvCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return err
	}
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(srvCert)
	tlsConfig = &tls.Config{
		ServerName: "localhost",
		RootCAs:    rootCAs,
	}

	certOut, err := os.Create(path + "/cert.pem")
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyOut, err := os.OpenFile(
		path+"/key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	pem.Encode(keyOut, block)
	return nil
}

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

// realMain is the real main function, which returns the value to pass to
// os.Exit(). We have to do this so we can use defer.
func realMain(m *testing.M) int {
	flag.Parse()
	defer glog.Flush()

	if *externalSMTPAddr != "" {
		smtpAddr = *externalSMTPAddr
		submissionAddr = *externalSubmissionAddr
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {
		// Generate certificates in a temporary directory.
		tmpDir, err := ioutil.TempDir("", "chasquid_test:")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", tmpDir)
			return 1
		}
		defer os.RemoveAll(tmpDir)

		err = generateCert(tmpDir)
		if err != nil {
			fmt.Printf("Failed to generate cert for testing: %v\n", err)
			return 1
		}

		s := NewServer()
		s.Hostname = "localhost"
		s.MaxDataSize = 50 * 1024 * 1025
		s.AddCerts(tmpDir+"/cert.pem", tmpDir+"/key.pem")
		s.AddAddr(smtpAddr, ModeSMTP)
		s.AddAddr(submissionAddr, ModeSubmission)

		localC := &courier.Procmail{}
		remoteC := &courier.SMTP{}
		s.InitQueue(tmpDir+"/queue", localC, remoteC)
		s.InitDomainInfo(tmpDir + "/domaininfo")

		udb := userdb.New("/dev/null")
		udb.AddUser("testuser", "testpasswd")
		s.aliasesR.AddAliasForTesting(
			"to@localhost", "testuser@localhost", aliases.EMAIL)
		s.AddDomain("localhost")
		s.AddUserDB("localhost", udb)

		go s.ListenAndServe()
	}

	waitForServer(smtpAddr)
	waitForServer(submissionAddr)
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(realMain(m))
}
