package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/dkim"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
)

func dkimSign() {
	domain := args["$2"]
	selector := args["$3"]
	keyPath := args["$4"]

	msg, err := io.ReadAll(os.Stdin)
	if err != nil {
		Fatalf("%v", err)
	}
	msg = normalize.ToCRLF(msg)

	if domain == "" {
		domain = getDomainFromMsg(msg)
	}
	if selector == "" {
		selector = findSelectorForDomain(domain)
	}
	if keyPath == "" {
		keyPath = keyPathFor(domain, selector)
	}

	signer := &dkim.Signer{
		Domain:   domain,
		Selector: selector,
		Signer:   loadPrivateKey(keyPath),
	}

	ctx := context.Background()
	if _, verbose := args["-v"]; verbose {
		ctx = dkim.WithTraceFunc(ctx,
			func(format string, args ...interface{}) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			})
	}

	header, err := signer.Sign(ctx, string(msg))
	if err != nil {
		Fatalf("Error signing message: %v", err)
	}
	fmt.Printf("DKIM-Signature: %s\r\n",
		strings.ReplaceAll(header, "\r\n", "\r\n\t"))
}

func dkimVerify() {
	msg, err := io.ReadAll(os.Stdin)
	if err != nil {
		Fatalf("%v", err)
	}
	msg = normalize.ToCRLF(msg)

	ctx := context.Background()
	if _, verbose := args["-v"]; verbose {
		ctx = dkim.WithTraceFunc(ctx,
			func(format string, args ...interface{}) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			})
	}

	results, err := dkim.VerifyMessage(ctx, string(msg))
	if err != nil {
		Fatalf("Error verifying message: %v", err)
	}

	hostname, _ := os.Hostname()
	ar := "Authentication-Results: " + hostname + "\r\n\t"
	ar += strings.ReplaceAll(
		results.AuthenticationResults(), "\r\n", "\r\n\t")

	fmt.Println(ar)
}

func dkimDNS() {
	domain := args["$2"]
	selector := args["$3"]
	keyPath := args["$4"]

	if domain == "" {
		Fatalf("Error: missing domain parameter")
	}
	if selector == "" {
		selector = findSelectorForDomain(domain)
	}
	if keyPath == "" {
		keyPath = keyPathFor(domain, selector)
	}

	fmt.Println(dnsRecordFor(domain, selector, loadPrivateKey(keyPath)))
}

func dnsRecordFor(domain, selector string, private crypto.Signer) string {
	public := private.Public()

	var err error
	algoStr := ""
	pubBytes := []byte{}
	switch private.(type) {
	case *rsa.PrivateKey:
		algoStr = "rsa"
		pubBytes, err = x509.MarshalPKIXPublicKey(public)
	case ed25519.PrivateKey:
		algoStr = "ed25519"
		pubBytes = public.(ed25519.PublicKey)
	}

	if err != nil {
		Fatalf("Error marshaling public key: %v", err)
	}

	return fmt.Sprintf(
		"%s._domainkey.%s\tTXT\t\"v=DKIM1; k=%s; p=%s\"",
		selector, domain,
		algoStr, base64.StdEncoding.EncodeToString(pubBytes))
}

func dkimKeygen() {
	domain := args["$2"]
	selector := args["$3"]
	keyPath := args["$4"]
	algo := args["--algo"]

	if domain == "" {
		Fatalf("Error: missing domain parameter")
	}
	if selector == "" {
		selector = time.Now().UTC().Format("20060102")
	}
	if keyPath == "" {
		keyPath = keyPathFor(domain, selector)
	}

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		Fatalf("Error: key already exists at %q", keyPath)
	}

	var private crypto.Signer
	var err error
	switch algo {
	case "", "rsa3072":
		private, err = rsa.GenerateKey(rand.Reader, 3072)
	case "rsa4096":
		private, err = rsa.GenerateKey(rand.Reader, 4096)
	case "ed25519":
		_, private, err = ed25519.GenerateKey(rand.Reader)
	default:
		Fatalf("Error: unsupported algorithm %q", algo)
	}

	if err != nil {
		Fatalf("Error generating key: %v", err)
	}

	privB, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		Fatalf("Error marshaling private key: %v", err)
	}

	f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
	if err != nil {
		Fatalf("Error creating key file %q: %v", keyPath, err)
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privB,
	}
	if err := pem.Encode(f, block); err != nil {
		Fatalf("Error PEM-encoding key: %v", err)
	}
	f.Close()

	fmt.Printf("Key written to %q\n\n", keyPath)

	fmt.Println(dnsRecordFor(domain, selector, private))
}

func keyPathFor(domain, selector string) string {
	return path.Clean(fmt.Sprintf("%s/domains/%s/dkim:%s.pem",
		configDir, domain, selector))
}

func getDomainFromMsg(msg []byte) string {
	m, err := mail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		Fatalf("Error parsing message: %v", err)
	}

	addr, err := mail.ParseAddress(m.Header.Get("From"))
	if err != nil {
		Fatalf("Error parsing From: header: %v", err)
	}

	return envelope.DomainOf(addr.Address)
}

func findSelectorForDomain(domain string) string {
	glob := path.Clean(configDir + "/domains/" + domain + "/dkim:*.pem")
	ms, err := filepath.Glob(glob)
	if err != nil {
		Fatalf("Error finding DKIM keys: %v", err)
	}
	for _, m := range ms {
		base := filepath.Base(m)
		selector := strings.TrimPrefix(base, "dkim:")
		selector = strings.TrimSuffix(selector, ".pem")
		return selector
	}

	Fatalf("No DKIM keys found in %q", glob)
	return ""
}

func loadPrivateKey(path string) crypto.Signer {
	key, err := os.ReadFile(path)
	if err != nil {
		Fatalf("Error reading private key from %q: %v", path, err)
	}

	block, _ := pem.Decode(key)
	if block == nil {
		Fatalf("Error decoding PEM block")
	}

	switch strings.ToUpper(block.Type) {
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			Fatalf("Error parsing private key: %v", err)
		}
		return k.(crypto.Signer)
	default:
		Fatalf("Unsupported key type: %s", block.Type)
		return nil
	}
}
