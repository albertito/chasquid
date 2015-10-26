package main

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

	"github.com/golang/glog"
)

// Flags.
var (
	externalServerAddr = flag.String("external_server_addr", "",
		"address of the external server to test (defaults to use internal)")
)

var (
	// Server address.
	// We default to an internal one, but may get overriden via
	// --external_server_addr.
	// TODO: Don't hard-code the default.
	srvAddr = "127.0.0.1:13453"

	// TLS configuration to use in the clients.
	// Will contain the generated server certificate as root CA.
	tlsConfig *tls.Config
)

//
// === Tests ===
//

func mustDial(tb testing.TB, useTLS bool) *smtp.Client {
	c, err := smtp.Dial(srvAddr)
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
	var err error

	if err = c.Mail("from@from"); err != nil {
		tb.Errorf("Mail: %v", err)
	}

	if err = c.Rcpt("to@to"); err != nil {
		tb.Errorf("Rcpt: %v", err)
	}

	w, err := c.Data()
	if err != nil {
		tb.Fatalf("Data: %v", err)
	}

	msg := []byte("Hi! This is an email\n")
	if _, err = w.Write(msg); err != nil {
		tb.Errorf("Data write: %v", err)
	}

	if err = w.Close(); err != nil {
		tb.Errorf("Data close: %v", err)
	}
}

func TestSimple(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()
	sendEmail(t, c)
}

func TestSimpleTLS(t *testing.T) {
	c := mustDial(t, true)
	defer c.Close()
	sendEmail(t, c)
}

func TestManyEmails(t *testing.T) {
	c := mustDial(t, true)
	defer c.Close()
	sendEmail(t, c)
	sendEmail(t, c)
	sendEmail(t, c)
}

func TestWrongMailParsing(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	addrs := []string{"", "from", "a b c", "a @ b", "<x>", "<x y>", "><"}

	for _, addr := range addrs {
		if err := c.Mail(addr); err == nil {
			t.Errorf("Mail not failed as expected with %q", addr)
		}
	}

	if err := c.Mail("from@from"); err != nil {
		t.Errorf("Mail:", err)
	}

	for _, addr := range addrs {
		if err := c.Rcpt(addr); err == nil {
			t.Errorf("Rcpt not failed as expected with %q", addr)
		}
	}
}

func TestNullMailFrom(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	addrs := []string{"<>", "  <>", " <  > "}
	for _, addr := range addrs {
		if err := c.Text.PrintfLine(addr); err != nil {
			t.Fatalf("MAIL FROM failed with addr %q: %v", addr, err)
		}
	}
}

func TestRcptBeforeMail(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	if err := c.Rcpt("to@to"); err == nil {
		t.Errorf("Rcpt not failed as expected")
	}
}

func TestHelp(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	if err := c.Text.PrintfLine("HELP"); err != nil {
		t.Fatalf("Failed to write HELP: %v", err)
	}

	if _, _, err := c.Text.ReadResponse(214); err != nil {
		t.Errorf("Incorrect HELP response: %v", err)
	}
}

func TestNoop(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	if err := c.Text.PrintfLine("NOOP"); err != nil {
		t.Fatalf("Failed to write NOOP: %v", err)
	}

	if _, _, err := c.Text.ReadResponse(250); err != nil {
		t.Errorf("Incorrect NOOP response: %v", err)
	}
}

func TestReset(t *testing.T) {
	c := mustDial(t, false)
	defer c.Close()

	if err := c.Mail("from@from"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}

	if err := c.Reset(); err != nil {
		t.Errorf("RSET: %v", err)
	}

	if err := c.Mail("from@from"); err != nil {
		t.Errorf("MAIL after RSET: %v", err)
	}
}

func TestRepeatedStartTLS(t *testing.T) {
	c, err := smtp.Dial(srvAddr)
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
	c := mustDial(b, false)
	defer c.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendEmail(b, c)
	}
}

func BenchmarkManyEmailsParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		c := mustDial(b, false)
		defer c.Close()

		for pb.Next() {
			sendEmail(b, c)
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

	if *externalServerAddr != "" {
		srvAddr = *externalServerAddr
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

		s := NewServer("localhost")
		s.AddCerts(tmpDir+"/cert.pem", tmpDir+"/key.pem")
		s.AddAddr(srvAddr)
		go s.ListenAndServe()
	}

	waitForServer(srvAddr)
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(realMain(m))
}
