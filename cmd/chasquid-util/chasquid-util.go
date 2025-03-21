// chasquid-util is a command-line utility for chasquid-related operations.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"blitiri.com.ar/go/chasquid/internal/config"
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/localrpc"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/prototext"
)

// Usage to show users on --help or invocation errors.
const usage = `
Usage:
  chasquid-util [options] user-add <user@domain> [--password=<password>] [--receive_only]
    Add a user to the userdb.
  chasquid-util [options] user-remove <user@domain>
    Remove a user from the userdb.
  chasquid-util [options] authenticate <user@domain> [--password=<password>]
    Authenticate a user.
  chasquid-util [options] check-userdb <domain>
    Check if the userdb for the given domain is accessible.
  chasquid-util [options] aliases-resolve <address>
    Resolve an address. Talks to the running chasquid.
  chasquid-util [options] domaininfo-remove <domain>
    Remove domaininfo for the given domain. Talks to the running chasquid.
  chasquid-util [options] print-config
    Print the current chasquid configuration.

  chasquid-util [options] dkim-keygen <domain> [<selector> <private-key.pem>] [--algo=rsa3072|rsa4096|ed25519]
    Generate a new DKIM key pair for the domain.
  chasquid-util [options] dkim-dns <domain> [<selector> <private-key.pem>]
    Print the DNS TXT record to use for the domain, selector and
    private key.

Options:
  -C=<path>, --config_dir=<path>  Configuration directory
  -v                              Verbose mode
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
	if d, ok := args["--config_dir"]; ok {
		configDir = d
	}
	if d, ok := args["-C"]; ok {
		configDir = d
	}
	if d, ok := args["--configdir"]; ok {
		configDir = d
		Warnf("Option --configdir is deprecated, use --config_dir instead")
	}

	commands := map[string]func(){
		"user-add":          userAdd,
		"user-remove":       userRemove,
		"authenticate":      authenticate,
		"check-userdb":      checkUserDB,
		"aliases-resolve":   aliasesResolve,
		"print-config":      printConfig,
		"domaininfo-remove": domaininfoRemove,
		"dkim-keygen":       dkimKeygen,
		"dkim-dns":          dkimDNS,

		// These exist for testing purposes and may be removed in the future.
		// Do not rely on them.
		"dkim-verify": dkimVerify,
		"dkim-sign":   dkimSign,
	}

	cmd := args["$1"]
	if f, ok := commands[cmd]; ok {
		f()
	} else {
		fmt.Printf("Unknown argument %q\n", cmd)
		Fatalf(usage)
	}
}

// Fatalf prints the given message to stderr, then exits the program with an
// error code.
func Fatalf(s string, arg ...interface{}) {
	fmt.Fprintf(os.Stderr, s+"\n", arg...)
	os.Exit(1)
}

// Warnf prints the given message to stderr, but does not exit the program.
func Warnf(s string, arg ...interface{}) {
	fmt.Fprintf(os.Stderr, s+"\n", arg...)
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

	if create {
		dbDir := filepath.Dir(userDBForDomain(domain))
		if _, err := os.Stat(dbDir); errors.Is(err, fs.ErrNotExist) {
			fmt.Println("Creating database")
			err = os.MkdirAll(dbDir, 0755)
			if err != nil {
				Fatalf("Error creating database dir: %v", err)
			}
		}
	}

	db, err := userdb.Load(userDBForDomain(domain))
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	user, err = normalize.User(user)
	if err != nil {
		Fatalf("Error normalizing user: %v", err)
	}

	return user, domain, db
}

// chasquid-util check-userdb <domain>
func checkUserDB() {
	path := userDBForDomain("")
	// Check if the file exists. This is because userdb.Load does not consider
	// it an error.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		Fatalf("Error: file %q does not exist", path)
	}

	udb, err := userdb.Load(path)
	if err != nil {
		Fatalf("Error loading database: %v", err)
	}

	fmt.Printf("Database loaded (%d users)\n", udb.Len())
}

// chasquid-util user-add <user@domain> [--password=<password>] [--receive_only]
func userAdd() {
	user, _, db := userDBFromArgs(true)

	_, recvOnly := args["--receive_only"]
	_, hasPassword := args["--password"]

	if recvOnly && hasPassword {
		Fatalf("Cannot specify both --receive_only and --password")
	}

	var err error
	if recvOnly {
		err = db.AddDeniedUser(user)
	} else {
		password := getPassword()
		err = db.AddUser(user, password)
	}

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

	c := localrpc.NewClient(conf.DataDir + "/localrpc-v1")
	vs, err := c.Call("AliasResolve", "Address", args["$2"])
	if err != nil {
		Fatalf("Error resolving: %v", err)
	}

	// Result is a map of type -> []addresses.
	// Sort the types for deterministic output.
	ts := []string{}
	for t := range vs {
		ts = append(ts, t)
	}
	sort.Strings(ts)

	for _, t := range ts {
		for _, a := range vs[t] {
			fmt.Printf("%v  %s\n", t, a)
		}
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
	conf, err := config.Load(configDir+"/chasquid.conf", "")
	if err != nil {
		Fatalf("Error loading config: %v", err)
	}

	c := localrpc.NewClient(conf.DataDir + "/localrpc-v1")
	_, err = c.Call("DomaininfoClear", "Domain", args["$2"])
	if err != nil {
		Fatalf("Error removing domaininfo entry: %v", err)
	}
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
