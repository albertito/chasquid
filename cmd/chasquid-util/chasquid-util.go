// chasquid-util is a command-line utility for chasquid-related operations.
//
// Don't include it in the coverage build.
// +build !coverage

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/prototext"
)

// Usage to show users on --help or invocation errors.
const usage = `
Usage:
  chasquid-util [options] user-add <user@domain> [--password=<password>]
  chasquid-util [options] user-remove <user@domain>
  chasquid-util [options] authenticate <user@domain> [--password=<password>]
  chasquid-util [options] check-userdb <domain>
  chasquid-util [options] aliases-resolve <address>
  chasquid-util [options] domaininfo-remove <domain>
  chasquid-util [options] print-config
  chasquid-util [options] aliases-add <source> <target>

Options:
  -C=<path>, --configdir=<path>  Configuration directory
`

// Command-line arguments.
// Arguments starting with "-" will be parsed as key-value pairs, and
// positional arguments will appear as "$POS" -> value.
//
// For example, "--abc=def x y -p=q -r" will result in:
// {"--abc": "def", "$1": "x", "$2": "y", "-p": "q", "-r": ""}
var args map[string]string

// Globals, loaded from top-level options.
var (
	configDir = "/etc/chasquid"
)

func main() {
	args = parseArgs(usage)

	if _, ok := args["--help"]; ok {
		fmt.Print(usage)
		return
	}

	// Load globals.
	if d, ok := args["--configdir"]; ok {
		configDir = d
	}
	if d, ok := args["-C"]; ok {
		configDir = d
	}

	commands := map[string]func(){
		"user-add":          userAdd,
		"user-remove":       userRemove,
		"authenticate":      authenticate,
		"check-userdb":      checkUserDB,
		"aliases-resolve":   aliasesResolve,
		"print-config":      printConfig,
		"domaininfo-remove": domaininfoRemove,
		"aliases-add":       aliasesAdd,
	}

	cmd := args["$1"]
	if f, ok := commands[cmd]; ok {
		f()
	} else {
		fmt.Printf("Unknown argument %q\n", cmd)
		Fatalf(usage)
	}
}

// Fatalf prints the given message, then exits the program with an error code.
func Fatalf(s string, arg ...interface{}) {
	fmt.Printf(s+"\n", arg...)
	os.Exit(1)
}

func userDBForDomain(domain string) string {
	if domain == "" {
		domain = args["$2"]
	}
	return configDir + "/domains/" + domain + "/users"
}

func userDBFromArgs(create bool) (string, string, *userdb.DB) {
	username := args["$2"]
	user, domain := envelope.Split(username)
	if domain == "" {
		Fatalf("Domain missing, username should be of the form 'user@domain'")
	}

	db, err := userdb.Load(userDBForDomain(domain))
	if err != nil {
		if create && os.IsNotExist(err) {
			fmt.Println("Creating database")
			err = os.MkdirAll(filepath.Dir(userDBForDomain(domain)), 0755)
			if err != nil {
				Fatalf("Error creating database dir: %v", err)
			}
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
func checkUserDB() {
	_, err := userdb.Load(userDBForDomain(""))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	fmt.Println("Database loaded")
}

// chasquid-util user-add <user@domain> [--password=<password>]
func userAdd() {
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

// chasquid-util authenticate <user@domain> [--password=<password>]
func authenticate() {
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
	password, ok := args["--password"]
	if ok {
		return password
	}

	fmt.Printf("Password: ")
	p1, err := term.ReadPassword(syscall.Stdin)
	fmt.Printf("\n")
	if err != nil {
		Fatalf("Error reading password: %v\n", err)
	}

	fmt.Printf("Confirm password: ")
	p2, err := term.ReadPassword(syscall.Stdin)
	fmt.Printf("\n")
	if err != nil {
		Fatalf("Error reading password: %v", err)
	}

	if !bytes.Equal(p1, p2) {
		Fatalf("Passwords don't match")
	}

	return string(p1)
}

// chasquid-util user-remove <user@domain>
func userRemove() {
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
func aliasesResolve() {
	conf, err := config.Load(configDir+"/chasquid.conf", "")
	if err != nil {
		Fatalf("Error loading config: %v", err)
	}
	_ = os.Chdir(configDir)

	r := aliases.NewResolver(allUsersExist)
	r.SuffixSep = *conf.SuffixSeparators
	r.DropChars = *conf.DropCharacters

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

	rcpts, err := r.Resolve(args["$2"])
	if err != nil {
		Fatalf("Error resolving: %v", err)
	}
	for _, rcpt := range rcpts {
		fmt.Printf("%v  %s\n", rcpt.Type, rcpt.Addr)
	}

}

// chasquid-util print-config
func printConfig() {
	conf, err := config.Load(configDir+"/chasquid.conf", "")
	if err != nil {
		Fatalf("Error loading config: %v", err)
	}

	fmt.Println(prototext.Format(conf))
}

// chasquid-util domaininfo-remove <domain>
func domaininfoRemove() {
	domain := args["$2"]

	conf, err := config.Load(configDir+"/chasquid.conf", "")
	if err != nil {
		Fatalf("Error loading config: %v", err)
	}

	// File for the corresponding domain.
	// Note this is making some assumptions about the data layout and
	// protoio's storage structure, so it will need adjustment if they change.
	file := conf.DataDir + "/domaininfo/s:" + url.QueryEscape(domain)
	err = os.Remove(file)
	if err != nil {
		Fatalf("Error removing file: %v", err)
	}
}

func allUsersExist(user, domain string) (bool, error) { return true, nil }

// chasquid-util aliases-add <source> <target>
func aliasesAdd() {
	source := args["$2"]
	target := args["$3"]

	user, domain := envelope.Split(source)
	if domain == "" {
		Fatalf("Domain required in source address")
	}

	if target == "" {
		Fatalf("Target must be present")
	}

	// Ensure the domain exists.
	if _, err := os.Stat(filepath.Join(configDir, "domains", domain)); os.IsNotExist(err) {
		Fatalf("Domain doesn't exist")
	}

	conf, err := config.Load(configDir+"/chasquid.conf", "")
	if err != nil {
		Fatalf("Error loading config: %v", err)
	}
	_ = os.Chdir(configDir)

	// Setup alias resolver.
	r := aliases.NewResolver(allUsersExist)
	r.SuffixSep = *conf.SuffixSeparators
	r.DropChars = *conf.DropCharacters

	r.AddDomain(domain)
	aliasesFilePath := filepath.Join("domains", domain, "aliases")
	if err := r.AddAliasesFile(domain, aliasesFilePath); err != nil {
		Fatalf("%s: error loading %q: %v", domain, aliasesFilePath, err)
	}

	// Check for existing entry.
	if _, ok := r.Exists(source); ok {
		Fatalf("There's already an entry for %v", source)
	}

	// Append the new entry.
	aliasesFile, err := os.OpenFile(aliasesFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		Fatalf("Couldn't open %s: %v", aliasesFilePath, err)
	}
	_, err = fmt.Fprintf(aliasesFile, "%s: %s\n", user, target)
	if err != nil {
		Fatalf("Couldn't write to %s: %v", aliasesFilePath, err)
	}
	aliasesFile.Close()
	fmt.Println("Added alias")
}

// parseArgs parses the command line arguments, and returns a map.
//
// Arguments starting with "-" will be parsed as key-value pairs, and
// positional arguments will appear as "$POS" -> value.
//
// For example, "--abc=def x y -p=q -r" will result in:
// {"--abc": "def", "$1": "x", "$2": "y", "-p": "q", "-r": ""}
func parseArgs(usage string) map[string]string {
	args := map[string]string{}

	pos := 1
	for _, a := range os.Args[1:] {
		// Note: Consider handling end of args marker "--" explicitly in
		// the future if needed.
		if strings.HasPrefix(a, "-") {
			sp := strings.SplitN(a, "=", 2)
			if len(sp) < 2 {
				args[a] = ""
			} else {
				args[sp[0]] = sp[1]
			}
		} else {
			args["$"+strconv.Itoa(pos)] = a
			pos++
		}
	}

	return args
}
