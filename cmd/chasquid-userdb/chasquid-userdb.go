package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"

	"blitiri.com.ar/go/chasquid/internal/userdb"
)

var (
	dbFname  = flag.String("database", "", "database file")
	adduser  = flag.String("add_user", "", "user to add")
	password = flag.String("password", "",
		"password for the user to add (will prompt if missing)")
	disableChecks = flag.Bool("dangerously_disable_checks", false,
		"disable security checks - DANGEROUS, use for testing only")
)

func main() {
	flag.Parse()

	if *dbFname == "" {
		fmt.Printf("database name missing, forgot --database?\n")
		os.Exit(1)
	}

	db, err := userdb.Load(*dbFname)
	if err != nil {
		if *adduser != "" && os.IsNotExist(err) {
			fmt.Printf("creating database\n")
		} else {
			fmt.Printf("error loading database: %v\n", err)
			os.Exit(1)
		}
	}

	if *adduser == "" {
		fmt.Printf("database loaded\n")
		return
	}

	if *password == "" {
		fmt.Printf("Password: ")
		p1, err := terminal.ReadPassword(syscall.Stdin)
		fmt.Printf("\n")
		if err != nil {
			fmt.Printf("error reading password: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Confirm password: ")
		p2, err := terminal.ReadPassword(syscall.Stdin)
		fmt.Printf("\n")
		if err != nil {
			fmt.Printf("error reading password: %v\n", err)
			os.Exit(1)
		}

		if !bytes.Equal(p1, p2) {
			fmt.Printf("passwords don't match\n")
			os.Exit(1)
		}

		*password = string(p1)
	}

	if !*disableChecks {
		if len(*password) < 8 {
			fmt.Printf("password is too short\n")
			os.Exit(1)
		}
	}

	err = db.AddUser(*adduser, *password)
	if err != nil {
		fmt.Printf("error adding user: %v\n", err)
		os.Exit(1)
	}

	err = db.Write()
	if err != nil {
		fmt.Printf("error writing database: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("added user\n")
}
