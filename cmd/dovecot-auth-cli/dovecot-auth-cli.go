// CLI used for testing the dovecot authentication package.
//
// NOT for production use.
package main

import (
	"fmt"
	"os"

	"blitiri.com.ar/go/chasquid/internal/dovecot"
)

func main() {
	a := dovecot.NewAuth(os.Args[1]+"-userdb", os.Args[1]+"-client")

	var ok bool
	var err error

	switch os.Args[2] {
	case "exists":
		ok, err = a.Exists(os.Args[3])
	case "auth":
		ok, err = a.Authenticate(os.Args[3], os.Args[4])
	default:
		fmt.Printf("unknown subcommand\n")
		os.Exit(1)
	}

	if ok {
		fmt.Printf("yes\n")
		return
	}

	fmt.Printf("no: %v\n", err)
	os.Exit(1)
}
