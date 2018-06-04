// CLI used for testing the dovecot authentication package.
//
// NOT for production use.
// +build !coverage

package main

import (
	"flag"
	"fmt"

	"blitiri.com.ar/go/chasquid/internal/dovecot"
)

func main() {
	flag.Parse()
	a := dovecot.NewAuth(flag.Arg(0)+"-userdb", flag.Arg(0)+"-client")

	var ok bool
	var err error

	switch flag.Arg(1) {
	case "exists":
		ok, err = a.Exists(flag.Arg(2))
	case "auth":
		ok, err = a.Authenticate(flag.Arg(2), flag.Arg(3))
	default:
		fmt.Printf("unknown subcommand\n")
	}

	if ok {
		fmt.Printf("yes\n")
		return
	}

	fmt.Printf("no: %v\n", err)
}
