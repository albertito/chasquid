// CLI used for testing the dovecot authentication package.
//
// NOT for production use.
// +build !coverage

package main

import (
	"flag"
	"fmt"
	"os"

	"blitiri.com.ar/go/chasquid/internal/dovecot"
)

const help = `
Usage:
	dovecot-auth-cli <path prefix> exists user@domain
	dovecot-auth-cli <path prefix> auth user@domain password

Example:
	dovecot-auth-cli /var/run/dovecot/auth-chasquid exists user@domain
	dovecot-auth-cli /var/run/dovecot/auth-chasquid auth user@domain password

`

func main() {
	flag.Parse()

	if len(flag.Args()) < 3 {
		fmt.Fprint(os.Stderr, help)
		fmt.Print("no: invalid arguments\n")
		return
	}

	a := dovecot.NewAuth(flag.Arg(0)+"-userdb", flag.Arg(0)+"-client")

	var ok bool
	var err error

	switch flag.Arg(1) {
	case "exists":
		ok, err = a.Exists(flag.Arg(2))
	case "auth":
		ok, err = a.Authenticate(flag.Arg(2), flag.Arg(3))
	default:
		err = fmt.Errorf("unknown subcommand %q", flag.Arg(1))
	}

	if ok {
		fmt.Print("yes\n")
		return
	}

	fmt.Printf("no: %v\n", err)
}
