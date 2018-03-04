// Package userdb implements a simple user database.
//
//
// Format
//
// The user database is a file containing a list of users and their passwords,
// encrypted with some scheme.
// We use a text-encoded protobuf, the structure can be found in userdb.proto.
//
// We write text instead of binary to make it easier for administrators to
// troubleshoot, and since performance is not an issue for our expected usage.
//
// Users must be UTF-8 and NOT contain whitespace; the library will enforce
// this.
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

//go:generate protoc --go_out=. userdb.proto

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/scrypt"

	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/protoio"
)

// DB represents a single user database.
type DB struct {
	fname string
	db    *ProtoDB

	// Lock protecting db.
	mu sync.RWMutex
}

// New returns a new user database, on the given file name.
func New(fname string) *DB {
	return &DB{
		fname: fname,
		db:    &ProtoDB{Users: map[string]*Password{}},
	}
}

// Load the database from the given file.
// Return the database, and a fatal error if the database could not be
// loaded.
func Load(fname string) (*DB, error) {
	db := New(fname)
	err := protoio.ReadTextMessage(fname, db.db)

	// Reading may result in an empty protobuf or dictionary; make sure we
	// return an empty but usable structure.
	// This simplifies many of our uses, as we can assume the map is not nil.
	if db.db == nil || db.db.Users == nil {
		db.db = &ProtoDB{Users: map[string]*Password{}}
	}

	return db, err
}

// Reload the database, refreshing its contents from the current file on disk.
// If there are errors reading from the file, they are returned and the
// database is not changed.
func (db *DB) Reload() error {
	newdb, err := Load(db.fname)
	if err != nil {
		return err
	}

	db.mu.Lock()
	db.db = newdb.db
	db.mu.Unlock()

	return nil
}

// Write the database to disk. It will do a complete rewrite each time, and is
// not safe to call it from different processes in parallel.
func (db *DB) Write() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return protoio.WriteTextMessage(db.fname, db.db, 0660)
}

// Authenticate returns true if the password is valid for the user, false
// otherwise.
func (db *DB) Authenticate(name, plainPassword string) bool {
	db.mu.RLock()
	passwd, ok := db.db.Users[name]
	db.mu.RUnlock()

	if !ok {
		return false
	}

	return passwd.PasswordMatches(plainPassword)
}

// PasswordMatches returns true if the given password is a match.
func (p *Password) PasswordMatches(plain string) bool {
	switch s := p.Scheme.(type) {
	case nil:
		return false
	case *Password_Scrypt:
		return s.Scrypt.PasswordMatches(plain)
	case *Password_Plain:
		return s.Plain.PasswordMatches(plain)
	default:
		return false
	}
}

// AddUser to the database. If the user is already present, override it.
// Note we enforce that the name has been normalized previously.
func (db *DB) AddUser(name, plainPassword string) error {
	if norm, err := normalize.User(name); err != nil || name != norm {
		return errors.New("invalid username")
	}

	s := &Scrypt{
		// Use hard-coded standard parameters for now.
		// Follow the recommendations from the scrypt paper.
		LogN: 14, R: 8, P: 1, KeyLen: 32,

		// 16 bytes of salt (will be filled later).
		Salt: make([]byte, 16),
	}

	n, err := rand.Read(s.Salt)
	if n != 16 || err != nil {
		return fmt.Errorf("failed to get salt - %d - %v", n, err)
	}

	s.Encrypted, err = scrypt.Key([]byte(plainPassword), s.Salt,
		1<<s.LogN, int(s.R), int(s.P), int(s.KeyLen))
	if err != nil {
		return fmt.Errorf("scrypt failed: %v", err)
	}

	db.mu.Lock()
	db.db.Users[name] = &Password{
		Scheme: &Password_Scrypt{s},
	}
	db.mu.Unlock()

	return nil
}

// RemoveUser from the database. Returns True if the user was there, False
// otherwise.
func (db *DB) RemoveUser(name string) bool {
	db.mu.Lock()
	_, present := db.db.Users[name]
	delete(db.db.Users, name)
	db.mu.Unlock()
	return present
}

// Exists returns true if the user is present, false otherwise.
func (db *DB) Exists(name string) bool {
	db.mu.Lock()
	_, present := db.db.Users[name]
	db.mu.Unlock()
	return present
}

///////////////////////////////////////////////////////////
// Encryption schemes
//

// PasswordMatches implementation for the plain text scheme.
// Useful mostly for testing and debugging.
// TODO: Do we really need this? Removing it would make accidents less likely
// to happen. Consider doing so when we add another scheme, so we a least have
// two and multi-scheme support does not bit-rot.
func (p *Plain) PasswordMatches(plain string) bool {
	return plain == string(p.Password)
}

// PasswordMatches implementation for the scrypt scheme, which we use by
// default.
func (s *Scrypt) PasswordMatches(plain string) bool {
	dk, err := scrypt.Key([]byte(plain), s.Salt,
		1<<s.LogN, int(s.R), int(s.P), int(s.KeyLen))

	if err != nil {
		// The encryption failed, this is due to the parameters being invalid.
		// We validated them before, so something went really wrong.
		// TODO: do we want to return false instead?
		panic(fmt.Sprintf("scrypt failed: %v", err))
	}

	// This comparison should be high enough up the stack that it doesn't
	// matter, but do it in constant time just in case.
	return subtle.ConstantTimeCompare(dk, []byte(s.Encrypted)) == 1
}
