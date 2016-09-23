// chasquid-util is a command-line utility for chasquid-related operations.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"

	"bytes"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	"github.com/docopt/docopt-go"
	"golang.org/x/crypto/ssh/terminal"
)

// Usage, which doubles as parameter definitions thanks to docopt.
const usage = `
Usage:
  chasquid-util adduser <db> <username> [--password=<password>]
  chasquid-util removeuser <db> <username>
  chasquid-util authenticate <db> <username> [--password=<password>]
  chasquid-util check-userdb <db>
  chasquid-util aliases-resolve <configdir> <address>
`

// Command-line arguments.
var args map[string]interface{}

func main() {
	args, _ = docopt.Parse(usage, nil, true, "", false)

	commands := map[string]func(){
		"adduser":         AddUser,
		"removeuser":      RemoveUser,
		"authenticate":    Authenticate,
		"check-userdb":    CheckUserDB,
		"aliases-resolve": AliasesResolve,
	}

	for cmd, f := range commands {
		if args[cmd].(bool) {
			f()
		}
	}
}

func Fatalf(s string, arg ...interface{}) {
	fmt.Printf(s+"\n", arg...)
	os.Exit(1)
}

// chasquid-util check-userdb <db>
func CheckUserDB() {
	_, err := userdb.Load(args["<db>"].(string))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	fmt.Println("Database loaded")
}

// chasquid-util adduser <db> <username> [--password=<password>]
func AddUser() {
	db, err := userdb.Load(args["<db>"].(string))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Creating database")
		} else {
			Fatalf("Error loading database: %v", err)
		}
	}

	password := getPassword()

	err = db.AddUser(args["<username>"].(string), password)
	if err != nil {
		Fatalf("Error adding user: %v", err)
	}

	err = db.Write()
	if err != nil {
		Fatalf("Error writing database: %v", err)
	}

	fmt.Println("Added user")
}

// chasquid-util authenticate <db> <username> [--password=<password>]
func Authenticate() {
	db, err := userdb.Load(args["<db>"].(string))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	password := getPassword()
	ok := db.Authenticate(args["<username>"].(string), password)
	if ok {
		fmt.Println("Authentication succeeded")
	} else {
		Fatalf("Authentication failed")
	}
}

func getPassword() string {
	password, ok := args["--password"].(string)
	if ok {
		return password
	}

	fmt.Printf("Password: ")
	p1, err := terminal.ReadPassword(syscall.Stdin)
	fmt.Printf("\n")
	if err != nil {
		Fatalf("Error reading password: %v\n", err)
	}

	fmt.Printf("Confirm password: ")
	p2, err := terminal.ReadPassword(syscall.Stdin)
	fmt.Printf("\n")
	if err != nil {
		Fatalf("Error reading password: %v", err)
	}

	if !bytes.Equal(p1, p2) {
		Fatalf("Passwords don't match")
	}

	return string(p1)
}

// chasquid-util removeuser <db> <username>
func RemoveUser() {
	db, err := userdb.Load(args["<db>"].(string))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	present := db.RemoveUser(args["<username>"].(string))
	if !present {
		Fatalf("Unknown user")
	}

	err = db.Write()
	if err != nil {
		Fatalf("Error writing database: %v", err)
	}

	fmt.Println("Removed user")
}

// chasquid-util aliases-resolve <configdir> <address>
func AliasesResolve() {
	configDir := args["<configdir>"].(string)
	conf, err := config.Load(configDir + "/chasquid.conf")
	if err != nil {
		Fatalf("Error reading config")
	}
	os.Chdir(configDir)

	r := aliases.NewResolver()
	r.SuffixSep = conf.SuffixSeparators
	r.DropChars = conf.DropCharacters

	domainDirs, err := ioutil.ReadDir("domains/")
	if err != nil {
		Fatalf("Error reading domains/ directory: %v", err)
	}
	if len(domainDirs) == 0 {
		Fatalf("No domains found in config")
	}

	for _, info := range domainDirs {
		name := info.Name()
		aliasfile := "domains/" + name + "/aliases"
		r.AddDomain(name)
		err := r.AddAliasesFile(name, aliasfile)
		if err == nil {
			fmt.Printf("%s: loaded %q\n", name, aliasfile)
		} else if err != nil && os.IsNotExist(err) {
			fmt.Printf("%s: no aliases file\n", name)
		} else {
			fmt.Printf("%s: error loading %q: %v\n", name, aliasfile, err)
		}
	}

	rcpts, err := r.Resolve(args["<address>"].(string))
	if err != nil {
		Fatalf("Error resolving: %v", err)
	}
	for _, rcpt := range rcpts {
		fmt.Printf("%v  %s\n", rcpt.Type, rcpt.Addr)
	}

}
