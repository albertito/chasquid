// Support for overriding DNS lookups, for testing purposes.
// This is only used in tests, when the "dnsoverride" tag is active.
// It requires Go >= 1.8.
//
//go:build dnsoverride
// +build dnsoverride

package main

import (
	"context"
	"flag"
	"net"
	"time"
)

var (
	dnsAddr = flag.String("testing__dns_addr", "127.0.0.1:9053",
		"DNS server address to use, for testing purposes only")
)

var dialer = &net.Dialer{
	// We're going to talk to localhost, so have a short timeout so we fail
	// fast. Otherwise the callers might hang indefinitely when trying to
	// dial the DNS server.
	Timeout: 2 * time.Second,
}

func dial(ctx context.Context, network, address string) (net.Conn, error) {
	return dialer.DialContext(ctx, network, *dnsAddr)
}

func init() {
	// Override the resolver to talk with our local server for testing.
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = dial
}
