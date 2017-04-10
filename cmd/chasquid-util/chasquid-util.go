// chasquid-util is a command-line utility for chasquid-related operations.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"bytes"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/userdb"

	"github.com/docopt/docopt-go"
	"github.com/golang/protobuf/proto"
	"golang.org/x/crypto/ssh/terminal"
)

// Usage, which doubles as parameter definitions thanks to docopt.
const usage = `
Usage:
  chasquid-util [options] user-add <username> [--password=<password>]
  chasquid-util [options] user-remove <username>
  chasquid-util [options] authenticate <username> [--password=<password>]
  chasquid-util [options] check-userdb <domain>
  chasquid-util [options] aliases-resolve <address>
  chasquid-util [options] print-config

Options:
  -C --configdir=<path>  Configuration directory
`

// Command-line arguments.
var args map[string]interface{}

// Globals, loaded from top-level options.
var (
	configDir = "/etc/chasquid"
)

func main() {
	args, _ = docopt.Parse(usage, nil, true, "", false)

	// Load globals.
	if d, ok := args["--configdir"].(string); ok {
		configDir = d
	}

	commands := map[string]func(){
		"user-add":        UserAdd,
		"user-remove":     UserRemove,
		"authenticate":    Authenticate,
		"check-userdb":    CheckUserDB,
		"aliases-resolve": AliasesResolve,
		"print-config":    PrintConfig,
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

func userDBForDomain(domain string) string {
	if domain == "" {
		domain = args["<domain>"].(string)
	}
	return configDir + "/domains/" + domain + "/users"
}

func userDBFromArgs(create bool) (string, string, *userdb.DB) {
	username := args["<username>"].(string)
	user, domain := envelope.Split(username)
	if domain == "" {
		Fatalf("Domain missing, username should be of the form 'user@domain'")
	}

	db, err := userdb.Load(userDBForDomain(domain))
	if err != nil {
		if create && os.IsNotExist(err) {
			fmt.Println("Creating database")
			os.MkdirAll(filepath.Dir(userDBForDomain(domain)), 0755)
		} else {
			Fatalf("Error loading database: %v", err)
		}
	}

	user, err = normalize.User(user)
	if err != nil {
		Fatalf("Error normalizing user: %v", err)
	}

	return user, domain, db
}

// chasquid-util check-userdb <domain>
func CheckUserDB() {
	_, err := userdb.Load(userDBForDomain(""))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	fmt.Println("Database loaded")
}

// chasquid-util user-add <username> [--password=<password>]
func UserAdd() {
	user, _, db := userDBFromArgs(true)
	password := getPassword()

	err := db.AddUser(user, password)
	if err != nil {
		Fatalf("Error adding user: %v", err)
	}

	err = db.Write()
	if err != nil {
		Fatalf("Error writing database: %v", err)
	}

	fmt.Println("Added user")
}

// chasquid-util authenticate <username> [--password=<password>]
func Authenticate() {
	user, _, db := userDBFromArgs(false)

	password := getPassword()
	ok := db.Authenticate(user, password)
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

// chasquid-util user-remove <username>
func UserRemove() {
	user, _, db := userDBFromArgs(false)

	present := db.RemoveUser(user)
	if !present {
		Fatalf("Unknown user")
	}

	err := db.Write()
	if err != nil {
		Fatalf("Error writing database: %v", err)
	}

	fmt.Println("Removed user")
}

// chasquid-util aliases-resolve <address>
func AliasesResolve() {
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

// chasquid-util print-config
func PrintConfig() {
	conf, err := config.Load(configDir + "/chasquid.conf")
	if err != nil {
		Fatalf("Error reading config")
	}

	fmt.Println(proto.MarshalTextString(conf))
}
