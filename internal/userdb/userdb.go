// Package userdb implements a simple user database.
//
//
// Format
//
// The user database is a single text file, with one line per user.
// All contents must be UTF-8.
//
// For extensibility, the first line MUST be:
//
//   #chasquid-userdb-v1
//
// Then, each line is structured as follows:
//
//   user SP scheme SP password
//
// Where user is the username in question (usually without the domain,
// although this package is agnostic to it); scheme is the encryption scheme
// used for the password; and finally the password, encrypted with the
// referenced scheme and base64-encoded.
//
// Lines with parsing errors, including unknown schemes, will be ignored.
// Users must be UTF-8 and NOT contain whitespace; the library will enforce
// this as well.
//
//
// Schemes
//
// The default scheme is SCRYPT, with hard-coded parameters. The API does not
// allow the user to change this, at least for now.
// A PLAIN scheme is also supported for debugging purposes.
//
//
// Writing
//
// The functions that write a database file will not preserve ordering,
// invalid lines, empty lines, or any formatting.
//
// It is also not safe for concurrent use from different processes.
//
package userdb

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/scrypt"

	"blitiri.com.ar/go/chasquid/internal/safeio"
)

type user struct {
	name     string
	scheme   scheme
	password string
}

type DB struct {
	fname string
	finfo os.FileInfo

	// Map of username -> user structure
	users map[string]user

	// Lock protecting the users map.
	mu sync.RWMutex
}

var (
	MissingHeaderErr   = errors.New("missing '#chasquid-userdb-v1' header")
	InvalidUsernameErr = errors.New("username contains invalid characters")
)

func New(fname string) *DB {
	return &DB{
		fname: fname,
		users: map[string]user{},
	}
}

// Load the database from the given file.
// Return the database, a list of warnings (if any), and a fatal error if the
// database could not be loaded.
func Load(fname string) (*DB, []error, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, nil, err
	}

	db := &DB{
		fname: fname,
		users: map[string]user{},
	}

	db.finfo, err = f.Stat()
	if err != nil {
		return nil, nil, err
	}

	// Special case: an empty file is a valid, empty database.
	// This simplifies clients.
	if db.finfo.Size() == 0 {
		return db, nil, nil
	}

	scanner := bufio.NewScanner(f)
	scanner.Scan()
	if scanner.Text() != "#chasquid-userdb-v1" {
		return nil, nil, MissingHeaderErr
	}

	var warnings []error

	// Now the users, one per line. Skip invalid ones.
	for i := 2; scanner.Scan(); i++ {
		var name, schemeStr, b64passwd string
		n, err := fmt.Sscanf(scanner.Text(), "%s %s %s",
			&name, &schemeStr, &b64passwd)
		if err != nil || n != 3 {
			warnings = append(warnings, fmt.Errorf(
				"line %d: error parsing - %d elements - %v", i, n, err))
			break
		}

		if !ValidUsername(name) {
			warnings = append(warnings, fmt.Errorf(
				"line %d: invalid username", i))
			continue
		}

		password, err := base64.StdEncoding.DecodeString(b64passwd)
		if err != nil {
			warnings = append(warnings, fmt.Errorf(
				"line %d: error decoding password: %v", i, err))
			continue
		}

		sc, err := schemeFromString(schemeStr)
		if err != nil {
			warnings = append(warnings, fmt.Errorf(
				"line %d: error in scheme: %v", i, err))
			continue
		}

		u := user{
			name:     name,
			scheme:   sc,
			password: string(password),
		}
		db.users[name] = u
	}

	if err := scanner.Err(); err != nil {
		return nil, warnings, err
	}

	return db, warnings, nil
}

// Reload the database, refreshing its contents from the current file on disk.
// If there are errors reading from the file, they are returned and the
// database is not changed. Warnings are returned regardless.
func (db *DB) Reload() ([]error, error) {
	newdb, warnings, err := Load(db.fname)
	if err != nil {
		return warnings, err
	}

	db.mu.Lock()
	db.users = newdb.users
	db.finfo = newdb.finfo
	db.mu.Unlock()

	return warnings, nil
}

// Write the database to disk. It will do a complete rewrite each time, and is
// not safe to call it from different processes in parallel.
func (db *DB) Write() error {
	buf := new(bytes.Buffer)
	buf.WriteString("#chasquid-userdb-v1\n")

	db.mu.RLock()
	defer db.mu.RUnlock()

	// TODO: Sort the usernames, just to be friendlier.
	for _, user := range db.users {
		if strings.ContainsAny(user.name, illegalUsernameChars) {
			return InvalidUsernameErr
		}
		fmt.Fprintf(buf, "%s %s %s\n",
			user.name, user.scheme.String(),
			base64.StdEncoding.EncodeToString([]byte(user.password)))
	}

	mode := os.FileMode(0660)
	if db.finfo != nil {
		mode = db.finfo.Mode()
	}
	return safeio.WriteFile(db.fname, buf.Bytes(), mode)
}

// Does this user exist in the database?
func (db *DB) Exists(user string) bool {
	db.mu.RLock()
	_, ok := db.users[user]
	db.mu.RUnlock()

	return ok
}

// Is this password valid for the user?
func (db *DB) Authenticate(name, plainPassword string) bool {
	db.mu.RLock()
	u, ok := db.users[name]
	db.mu.RUnlock()

	if !ok {
		return false
	}

	return u.scheme.PasswordMatches(plainPassword, u.password)
}

// Check if the given user name is valid.
// User names have to be UTF-8, and must not have some particular characters,
// including whitespace.
func ValidUsername(name string) bool {
	return utf8.ValidString(name) &&
		!strings.ContainsAny(name, illegalUsernameChars)
}

// Illegal characters. Only whitespace for now, to prevent/minimize the
// chances of parsing issues.
// TODO: do we want to stop other characters, specifically about email? Or
// keep this generic and handle the mail-specific filtering in chasquid?
const illegalUsernameChars = "\t\n\v\f\r \xa0\x85"

// Add a user to the database. If the user is already present, override it.
func (db *DB) AddUser(name, plainPassword string) error {
	if !ValidUsername(name) {
		return InvalidUsernameErr
	}

	s := scryptScheme{
		// Use hard-coded standard parameters for now.
		// Follow the recommendations from the scrypt paper.
		logN: 14, r: 8, p: 1, keyLen: 32,

		// 16 bytes of salt (will be filled later).
		salt: make([]byte, 16),
	}

	n, err := rand.Read(s.salt)
	if n != 16 || err != nil {
		return fmt.Errorf("failed to get salt - %d - %v", n, err)
	}

	encrypted, err := scrypt.Key([]byte(plainPassword), s.salt,
		1<<s.logN, s.r, s.p, s.keyLen)
	if err != nil {
		return fmt.Errorf("scrypt failed: %v", err)
	}

	db.mu.Lock()
	db.users[name] = user{
		name:     name,
		scheme:   s,
		password: string(encrypted),
	}
	db.mu.Unlock()

	return nil
}

///////////////////////////////////////////////////////////
// Encryption schemes
//

type scheme interface {
	String() string
	PasswordMatches(plain, encrypted string) bool
}

// Plain text scheme. Useful mostly for testing and debugging.
// TODO: Do we really need this? Removing it would make accidents less likely
// to happen. Consider doing so when we add another scheme, so we a least have
// two and multi-scheme support does not bit-rot.
type plainScheme struct{}

func (s plainScheme) String() string {
	return "PLAIN"
}

func (s plainScheme) PasswordMatches(plain, encrypted string) bool {
	return plain == encrypted
}

// scrypt scheme, which we use by default.
type scryptScheme struct {
	logN   uint // 1<<logN requires this to be uint
	r, p   int
	keyLen int
	salt   []byte
}

func (s scryptScheme) String() string {
	// We're encoding the salt in base64, which uses "/+=", and the URL
	// variant uses "-_=". We use standard encoding, but shouldn't use any of
	// those as separators, just to be safe.
	// It's important that the salt be last, as we can only scan
	// space-delimited strings.
	return fmt.Sprintf("SCRYPT@n:%d,r:%d,p:%d,l:%d,%s",
		s.logN, s.r, s.p, s.keyLen,
		base64.StdEncoding.EncodeToString(s.salt))
}

func (s scryptScheme) PasswordMatches(plain, encrypted string) bool {
	dk, err := scrypt.Key([]byte(plain), s.salt, 1<<s.logN, s.r, s.p, s.keyLen)

	if err != nil {
		// The encryption failed, this is due to the parameters being invalid.
		// We validated them before, so something went really wrong.
		// TODO: do we want to return false instead?
		panic(fmt.Sprintf("scrypt failed: %v", err))
	}

	return bytes.Equal(dk, []byte(encrypted))
}

func schemeFromString(s string) (scheme, error) {
	if s == "PLAIN" {
		return plainScheme{}, nil
	} else if strings.HasPrefix(s, "SCRYPT@") {
		sc := scryptScheme{}
		var b64salt string
		n, err := fmt.Sscanf(s, "SCRYPT@n:%d,r:%d,p:%d,l:%d,%s",
			&sc.logN, &sc.r, &sc.p, &sc.keyLen, &b64salt)
		if n != 5 || err != nil {
			return nil, fmt.Errorf("error scanning scrypt: %d %v", n, err)
		}
		sc.salt, err = base64.StdEncoding.DecodeString(b64salt)
		if err != nil {
			return nil, fmt.Errorf("error decoding salt: %v", err)
		}

		// Perform some sanity checks on the parameters, just in case.
		if (sc.logN >= 32) || (sc.r*sc.p >= 1<<30) || (sc.keyLen < 24) {
			return nil, fmt.Errorf("invalid scrypt parameters")
		}

		return sc, nil
	}

	return nil, fmt.Errorf("unknown scheme")
}
