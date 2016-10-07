// Command line tool for playing with the SPF library.
//
// Not for use in production, just development and experimentation.
package main

import (
	"flag"
	"fmt"
	"net"

	"blitiri.com.ar/go/chasquid/internal/spf"
)

func main() {
	flag.Parse()

	r, err := spf.CheckHost(net.ParseIP(flag.Arg(0)), flag.Arg(1))
	fmt.Println(r)
	fmt.Println(err)
}
