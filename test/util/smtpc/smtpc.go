package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"io"
	"net"
	"net/smtp"
	"os"
	"strings"
)

var (
	addr = flag.String("addr", "", "Address of the SMTP server")

	user     = flag.String("user", "", "Username to use in SMTP AUTH")
	password = flag.String("password", "", "Password to use in SMTP AUTH")

	from = flag.String("from", "", "From address to use in the message")

	serverCert = flag.String("server_cert", "",
		"Path to the server certificate to expect")

	confPath = flag.String("c", "smtpc.conf",
		"Path to the configuration file")
)

func main() {
	flag.Parse()
	loadConfig()

	// Read message from stdin.
	rawMsg, err := io.ReadAll(os.Stdin)
	notnil(err)

	// RCPT TO from the command line.
	tos := make([]string, len(flag.Args()))
	for i, to := range flag.Args() {
		tos[i] = to
	}

	// Connect to the server.
	var conn net.Conn
	if *serverCert != "" {
		cert := loadCert(*serverCert)
		rootCAs := x509.NewCertPool()

		rootCAs.AddCert(cert)
		tlsConfig := &tls.Config{
			ServerName: cert.DNSNames[0],
			RootCAs:    rootCAs,
		}

		conn, err = tls.Dial("tcp", *addr, tlsConfig)
		defer conn.Close()
	} else {
		conn, err = net.Dial("tcp", *addr)
	}
	notnil(err)

	// Send the message.
	client, err := smtp.NewClient(conn, *addr)
	notnil(err)

	if *user != "" {
		auth := smtp.PlainAuth("", *user, *password, *addr)
		err = client.Auth(auth)
		notnil(err)
	}

	if *from == "" {
		*from = *user
	}
	err = client.Mail(*from)
	notnil(err)

	for _, to := range tos {
		err = client.Rcpt(to)
		notnil(err)
	}

	w, err := client.Data()
	notnil(err)
	_, err = io.Copy(w, bytes.NewReader(rawMsg))
	notnil(err)
	err = w.Close()
	notnil(err)

	err = client.Quit()
	notnil(err)
}

func loadConfig() {
	data, err := os.ReadFile(*confPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	notnil(err)

	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}

		k = strings.TrimSpace(k)

		// Set the flag but only if it wasn't already set.
		// Command-line flags take precedence.
		isSet := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == k {
				isSet = true
			}
		})
		if !isSet {
			flag.Lookup(k).Value.Set(strings.TrimSpace(v))
		}
	}
}

func loadCert(path string) *x509.Certificate {
	data, err := os.ReadFile(path)
	notnil(err)

	block, _ := pem.Decode(data)

	cert, err := x509.ParseCertificate(block.Bytes)
	notnil(err)

	return cert
}

func notnil(err error) {
	if err != nil {
		panic(err)
	}
}
