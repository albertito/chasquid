// Package testlib provides common test utilities.
package testlib

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// Rewrite a file with the given contents, or fail the test.
func Rewrite(t *testing.T, path, contents string) {
	t.Helper()
	err := os.WriteFile(path, []byte(contents), 0600)
	if err != nil {
		t.Fatalf("failed to rewrite file: %v", err)
	}
}

type deliverRequest struct {
	From string
	To   string
	Data []byte
	Via  []string
}

// TestCourier never fails, and always remembers everything.
type TestCourier struct {
	wg       sync.WaitGroup
	Requests []*deliverRequest
	ReqFor   map[string]*deliverRequest
	mu       sync.Mutex
}

// Deliver the given mail (saving it in tc.Requests).
func (tc *TestCourier) Deliver(from string, to string, data []byte) (error, bool) {
	defer tc.wg.Done()
	dr := &deliverRequest{from, to, data, nil}
	tc.mu.Lock()
	tc.Requests = append(tc.Requests, dr)
	tc.ReqFor[to] = dr
	tc.mu.Unlock()
	return nil, false
}

func (tc *TestCourier) Forward(from string, to string, data []byte, servers []string) (error, bool) {
	defer tc.wg.Done()
	dr := &deliverRequest{from, to, data, servers}
	tc.mu.Lock()
	tc.Requests = append(tc.Requests, dr)
	tc.ReqFor[to] = dr
	tc.mu.Unlock()
	return nil, false
}

// Expect i mails to be delivered.
func (tc *TestCourier) Expect(i int) {
	tc.wg.Add(i)
}

// Wait until all mails have been delivered.
func (tc *TestCourier) Wait() {
	tc.wg.Wait()
}

// NewTestCourier returns a new, empty TestCourier instance.
func NewTestCourier() *TestCourier {
	return &TestCourier{
		ReqFor: map[string]*deliverRequest{},
	}
}

type dumbCourier struct{}

func (c dumbCourier) Deliver(from string, to string, data []byte) (error, bool) {
	return nil, false
}

func (c dumbCourier) Forward(from string, to string, data []byte, servers []string) (error, bool) {
	return nil, false
}

// DumbCourier always succeeds delivery, and ignores everything.
var DumbCourier = dumbCourier{}

// GenerateCert generates a new, INSECURE self-signed certificate and writes
// it to a pair of (cert.pem, key.pem) files to the given path.
// Note the certificate is only useful for testing purposes.
func GenerateCert(path string) (*tls.Config, error) {
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
		IsCA:                  true,
	}

	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	// Create a global config for convenience.
	srvCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, err
	}
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(srvCert)
	tlsConfig := &tls.Config{
		ServerName: "localhost",
		RootCAs:    rootCAs,
	}

	certOut, err := os.Create(path + "/cert.pem")
	if err != nil {
		return nil, err
	}
	defer certOut.Close()
	err = pem.Encode(certOut,
		&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return nil, err
	}

	keyOut, err := os.OpenFile(
		path+"/key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	defer keyOut.Close()

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	err = pem.Encode(keyOut, block)
	return tlsConfig, err
}
