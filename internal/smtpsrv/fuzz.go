// Fuzz testing for package smtpsrv.  Based on server_test.

// +build gofuzz

package smtpsrv

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/textproto"
	"os"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"blitiri.com.ar/go/log"
)

var (
	// Server addresses. Will be filled in at init time.
	smtpAddr          = ""
	submissionAddr    = ""
	submissionTLSAddr = ""

	// TLS configuration to use in the clients.
	// Will contain the generated server certificate as root CA.
	tlsConfig *tls.Config
)

//
// === Fuzz test ===
//

func Fuzz(data []byte) int {
	// Byte 0: mode
	// The rest is what we will send the server, one line per command.
	if len(data) < 1 {
		return 0
	}

	var mode SocketMode
	addr := ""
	switch data[0] {
	case '0':
		mode = ModeSMTP
		addr = smtpAddr
	case '1':
		mode = ModeSubmission
		addr = submissionAddr
	case '2':
		mode = ModeSubmissionTLS
		addr = submissionTLSAddr
	default:
		return 0
	}
	data = data[1:]

	var err error
	var conn net.Conn
	if mode.TLS {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		panic(fmt.Errorf("failed to dial: %v", err))
	}
	defer conn.Close()

	tconn := textproto.NewConn(conn)
	defer tconn.Close()

	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := scanner.Text()
		cmd := strings.TrimSpace(strings.ToUpper(line))

		// Skip STARTTLS if it happens on a non-TLS connection - the jump is
		// not going to happen via fuzzer, it will just cause a timeout (which
		// is considered a crash).
		if cmd == "STARTTLS" && !mode.TLS {
			continue
		}

		if err = tconn.PrintfLine(line); err != nil {
			break
		}

		if _, _, err = tconn.ReadResponse(-1); err != nil {
			break
		}

		if cmd == "DATA" {
			// We just sent DATA and got a response; send the contents.
			err = exchangeData(scanner, tconn)
			if err != nil {
				break
			}
		}
	}
	if (err != nil && err != io.EOF) || scanner.Err() != nil {
		return 1
	}

	return 0
}

func exchangeData(scanner *bufio.Scanner, tconn *textproto.Conn) error {
	for scanner.Scan() {
		line := scanner.Text()
		if err := tconn.PrintfLine(line); err != nil {
			return err
		}
		if line == "." {
			break
		}
	}

	// Read the "." response.
	_, _, err := tconn.ReadResponse(-1)
	return err
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
		NotAfter:  time.Now().Add(24 * time.Hour),

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
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

// waitForServer waits 10 seconds for the server to start, and returns an error
// if it fails to do so.
// It does this by repeatedly connecting to the address until it either
// replies or times out. Note we do not do any validation of the reply.
func waitForServer(addr string) {
	start := time.Now()
	for time.Since(start) < 10*time.Second {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	panic(fmt.Errorf("%v not reachable", addr))
}

func init() {
	flag.Parse()

	log.Default.Level = log.Error

	// Generate certificates in a temporary directory.
	tmpDir, err := ioutil.TempDir("", "chasquid_smtpsrv_fuzz:")
	if err != nil {
		panic(fmt.Errorf("Failed to create temp dir: %v\n", tmpDir))
	}
	defer os.RemoveAll(tmpDir)

	err = generateCert(tmpDir)
	if err != nil {
		panic(fmt.Errorf("Failed to generate cert for testing: %v\n", err))
	}

	smtpAddr = testlib.GetFreePort()
	submissionAddr = testlib.GetFreePort()
	submissionTLSAddr = testlib.GetFreePort()

	s := NewServer()
	s.Hostname = "localhost"
	s.MaxDataSize = 50 * 1024 * 1025
	s.AddCerts(tmpDir+"/cert.pem", tmpDir+"/key.pem")
	s.AddAddr(smtpAddr, ModeSMTP)
	s.AddAddr(submissionAddr, ModeSubmission)
	s.AddAddr(submissionTLSAddr, ModeSubmissionTLS)

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

	// Disable SPF lookups, to avoid leaking DNS queries.
	disableSPFForTesting = true

	go s.ListenAndServe()

	waitForServer(smtpAddr)
	waitForServer(submissionAddr)
	waitForServer(submissionTLSAddr)
}
