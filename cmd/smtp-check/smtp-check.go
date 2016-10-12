// smtp-check is a command-line too for checking SMTP setups.
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/smtp"

	"blitiri.com.ar/go/chasquid/internal/tlsconst"

	"golang.org/x/net/idna"
)

var (
	port = flag.String("port", "smtp",
		"port to use for connecting to the MX servers")
)

func main() {
	flag.Parse()

	domain := flag.Arg(0)
	if domain == "" {
		log.Fatal("Use: smtp-check <domain>")
	}

	domain, err := idna.ToASCII(domain)
	if err != nil {
		log.Fatalf("IDNA conversion failed: %v", err)
	}

	mxs, err := net.LookupMX(domain)
	if err != nil {
		log.Fatalf("MX lookup: %v", err)
	}

	if len(mxs) == 0 {
		log.Fatalf("MX lookup returned no results")
	}

	for _, mx := range mxs {
		log.Printf("=== Testing MX: %2d  %s", mx.Pref, mx.Host)

		c, err := smtp.Dial(mx.Host + ":" + *port)
		if err != nil {
			log.Fatal(err)
		}

		config := &tls.Config{
			// Expect the server to have a certificate valid for the MX
			// we're connecting to.
			ServerName: mx.Host,
		}
		err = c.StartTLS(config)
		if err != nil {
			log.Fatal(err)
		}

		cstate, _ := c.TLSConnectionState()
		log.Printf("%s - %s", tlsconst.VersionName(cstate.Version),
			tlsconst.CipherSuiteName(cstate.CipherSuite))

		log.Printf("")
		c.Close()
	}

	log.Printf("=== Success")
}
