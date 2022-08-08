// Command line tool for playing with the SPF library.
//
// Not for use in production, just development and experimentation.
//go:build !coverage
// +build !coverage

package main

import (
	"flag"
	"fmt"
	"net"

	"blitiri.com.ar/go/spf"
)

func main() {
	flag.Parse()

	r, err := spf.CheckHostWithSender(
		net.ParseIP(flag.Arg(0)), "", flag.Arg(1))
	fmt.Println(r)
	fmt.Println(err)
}
