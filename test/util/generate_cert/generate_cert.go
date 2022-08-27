//go:build !coverage
// +build !coverage

// Utility to generate self-signed certificates.
// It generates a self-signed x509 certificate and key pair, and writes them
// to "fullchain.pem" and "privkey.pem".
//
// Intended for use in tests, not for production use.
package main

import (
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

var (
	host = flag.String("host", "",
		"Hostnames/IPs to generate the certificate for (comma separated)")
	validFor = flag.Duration("validfor", 4*time.Hour,
		"How long will the certificate be valid for")
	isCA = flag.Bool("ca", false,
		"Should this cert be its own CA?")
)

func fatalf(f string, a ...interface{}) {
	fmt.Printf(f, a...)
	os.Exit(1)
}

func main() {
	flag.Parse()
	if *host == "" {
		fatalf("Required flag: --host")
	}

	// Build the certificate template.
	serial, err := crand.Int(crand.Reader, big.NewInt(1<<62))
	if err != nil {
		fatalf("Error generating serial number: %v\n", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"Test Cert Org"}},

		// Valid from now until `--validfor` in the future.
		// Extended certs can be useful for manual troubleshooting.
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(*validFor),

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		BasicConstraintsValid: true,
	}

	if *isCA {
		tmpl.IsCA = true
	}

	hosts := strings.Split(*host, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			// We use IDNA-encoded DNS names, otherwise the TLS library won't
			// load the certificates.
			ih, err := idna.ToASCII(h)
			if err != nil {
				fatalf("Host %q cannot be IDNA-encoded: %v\n", h, err)
			}
			tmpl.DNSNames = append(tmpl.DNSNames, ih)
		}
	}

	// Generate a private key (RSA 2048).
	privK, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		fatalf("Error generating key: %v\n", err)
	}

	// Write the certificate.
	{
		derBytes, err := x509.CreateCertificate(
			crand.Reader, &tmpl, &tmpl, &privK.PublicKey, privK)
		if err != nil {
			fatalf("Failed to create certificate: %v\n", err)
		}

		fullchain, err := os.Create("fullchain.pem")
		if err != nil {
			fatalf("Failed to open fullchain.pem: %v\n", err)
		}
		err = pem.Encode(fullchain,
			&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
		if err != nil {
			fatalf("Error encoding certificate: %v\n", err)
		}
		fullchain.Close()
	}

	// Write the private key.
	{
		privkey, err := os.Create("privkey.pem")
		if err != nil {
			fatalf("failed to open privkey.pem: %v\n", err)
		}
		block := &pem.Block{Type: "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privK)}
		err = pem.Encode(privkey, block)
		if err != nil {
			fatalf("Error encoding private key: %v\n", err)
		}
		privkey.Close()
	}
}
