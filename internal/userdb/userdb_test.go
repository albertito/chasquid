package userdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Remove the file if the test was successful. Used in defer statements, to
// leave files around for inspection when the tests failed.
func removeIfSuccessful(t *testing.T, fname string) {
	// Safeguard, to make sure we only remove test files.
	// This should help prevent accidental deletions.
	if !strings.Contains(fname, "userdb_test") {
		panic("invalid/dangerous directory")
	}

	if !t.Failed() {
		os.Remove(fname)
	}
}

// Create a database with the given content on a temporary filename. Return
// the filename, or an error if there were errors creating it.
func mustCreateDB(t *testing.T, content string) string {
	f, err := ioutil.TempFile("", "userdb_test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}

	t.Logf("file: %q", f.Name())
	return f.Name()
}

func dbEquals(a, b *DB) bool {
	if a.db == nil || b.db == nil {
		return a.db == nil && b.db == nil
	}

	if len(a.db.Users) != len(b.db.Users) {
		return false
	}

	for k, av := range a.db.Users {
		bv, ok := b.db.Users[k]
		if !ok || !reflect.DeepEqual(av, bv) {
			return false
		}
	}

	return true
}

var emptyDB = &DB{
	db: &ProtoDB{Users: map[string]*Password{}},
}

// Test various cases of loading an empty/broken database.
func TestEmptyLoad(t *testing.T) {
	cases := []struct {
		desc     string
		content  string
		fatal    bool
		fatalErr error
	}{
		{"empty file", "", false, nil},
		{"invalid ", "users: < invalid >", true, nil},
	}

	for _, c := range cases {
		testOneLoad(t, c.desc, c.content, c.fatal, c.fatalErr)
	}
}

func testOneLoad(t *testing.T, desc, content string, fatal bool, fatalErr error) {
	fname := mustCreateDB(t, content)
	defer removeIfSuccessful(t, fname)
	db, err := Load(fname)
	if fatal {
		if err == nil {
			t.Errorf("case %q: expected error loading, got nil", desc)
		}
		if fatalErr != nil && fatalErr != err {
			t.Errorf("case %q: expected error %v, got %v", desc, fatalErr, err)
		}
	} else if !fatal && err != nil {
		t.Fatalf("case %q: error loading database: %v", desc, err)
	}

	if db != nil && !dbEquals(db, emptyDB) {
		t.Errorf("case %q: DB not empty: %#v", desc, db.db.Users)
	}
}

func mustLoad(t *testing.T, fname string) *DB {
	db, err := Load(fname)
	if err != nil {
		t.Fatalf("error loading database: %v", err)
	}

	return db
}

func TestWrite(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	if err := db.Write(); err != nil {
		t.Fatalf("error writing database: %v", err)
	}

	// Load again, check it works and it's still empty.
	db = mustLoad(t, fname)
	if !dbEquals(emptyDB, db) {
		t.Fatalf("expected %v, got %v", emptyDB, db)
	}

	// Add two users, write, and load again.
	if err := db.AddUser("user1", "passwd1"); err != nil {
		t.Fatalf("failed to add user1: %v", err)
	}
	if err := db.AddUser("ñoño", "añicos"); err != nil {
		t.Fatalf("failed to add ñoño: %v", err)
	}
	if err := db.Write(); err != nil {
		t.Fatalf("error writing database: %v", err)
	}

	db = mustLoad(t, fname)
	for _, name := range []string{"user1", "ñoño"} {
		if !db.Exists(name) {
			t.Errorf("user %q not in database", name)
		}
		if db.db.Users[name].GetScheme() == nil {
			t.Errorf("user %q not using scrypt: %#v", name, db.db.Users[name])
		}
	}

	// Check various user and password combinations, not all valid.
	combinations := []struct {
		user, passwd string
		expected     bool
	}{
		{"user1", "passwd1", true},
		{"user1", "passwd", false},
		{"user1", "passwd12", false},
		{"ñoño", "añicos", true},
		{"ñoño", "anicos", false},
		{"notindb", "something", false},
		{"", "", false},
		{" ", "  ", false},
	}
	for _, c := range combinations {
		if db.Authenticate(c.user, c.passwd) != c.expected {
			t.Errorf("auth(%q, %q) != %v", c.user, c.passwd, c.expected)
		}
	}
}

func TestNew(t *testing.T) {
	fname := fmt.Sprintf("%s/userdb_test-%d", os.TempDir(), os.Getpid())
	defer os.Remove(fname)
	db1 := New(fname)
	db1.AddUser("user", "passwd")
	db1.Write()

	db2, err := Load(fname)
	if err != nil {
		t.Fatalf("error loading: %v", err)
	}

	if !dbEquals(db1, db2) {
		t.Errorf("databases differ. db1:%v  !=  db2:%v", db1, db2)
	}
}

func TestInvalidUsername(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	// Names that are invalid.
	names := []string{
		// Contain various types of spaces.
		" ", "  ", "a b", "ñ ñ", "a\xa0b", "a\x85b", "a\nb", "a\tb", "a\xffb",

		// Contain characters not allowed by PRECIS.
		"\u00b9", "\u2163",

		// Names that are not normalized, but would otherwise be valid.
		"A", "Ñ",
	}
	for _, name := range names {
		err := db.AddUser(name, "passwd")
		if err == nil {
			t.Errorf("AddUser(%q) worked, expected it to fail", name)
		}
	}
}

func plainPassword(p string) *Password {
	return &Password{
		Scheme: &Password_Plain{
			Plain: &Plain{Password: []byte(p)},
		},
	}
}

// Test the plain scheme. Note we don't expect to use it in cases other than
// debugging, but it should be functional for that purpose.
func TestPlainScheme(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	db.db.Users["user"] = plainPassword("pass word")
	err := db.Write()
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	db = mustLoad(t, fname)
	if !db.Authenticate("user", "pass word") {
		t.Errorf("failed plain authentication")
	}
	if db.Authenticate("user", "wrong") {
		t.Errorf("plain authentication worked but it shouldn't")
	}
}

func TestReload(t *testing.T) {
	content := "users:< key: 'u1' value:< plain:< password: 'pass' >>>"
	fname := mustCreateDB(t, content)
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	// Add a valid line to the file.
	content += "users:< key: 'u2' value:< plain:< password: 'pass' >>>"
	ioutil.WriteFile(fname, []byte(content), 0660)

	err := db.Reload()
	if err != nil {
		t.Errorf("Reload failed: %v", err)
	}
	if len(db.db.Users) != 2 {
		t.Errorf("expected 2 users, got %d", len(db.db.Users))
	}

	// And now a broken one.
	content += "users:< invalid >"
	ioutil.WriteFile(fname, []byte(content), 0660)

	err = db.Reload()
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(db.db.Users) != 2 {
		t.Errorf("expected 2 users, got %d", len(db.db.Users))
	}

	// Cause an even bigger error loading, check the database is not changed.
	db.fname = "/does/not/exist"
	err = db.Reload()
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(db.db.Users) != 2 {
		t.Errorf("expected 2 users, got %d", len(db.db.Users))
	}
}

func TestRemoveUser(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	if ok := db.RemoveUser("unknown"); ok {
		t.Errorf("removal of unknown user succeeded")
	}

	if err := db.AddUser("user", "passwd"); err != nil {
		t.Fatalf("error adding user: %v", err)
	}

	if ok := db.RemoveUser("unknown"); ok {
		t.Errorf("removal of unknown user succeeded")
	}

	if ok := db.RemoveUser("user"); !ok {
		t.Errorf("removal of existing user failed")
	}

	if ok := db.RemoveUser("user"); ok {
		t.Errorf("removal of unknown user succeeded")
	}
}

func TestExists(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	if db.Exists("unknown") {
		t.Errorf("unknown user exists")
	}

	if err := db.AddUser("user", "passwd"); err != nil {
		t.Fatalf("error adding user: %v", err)
	}

	if db.Exists("unknown") {
		t.Errorf("unknown user exists")
	}

	if !db.Exists("user") {
		t.Errorf("known user does not exist")
	}

	if !db.Exists("user") {
		t.Errorf("known user does not exist")
	}
}
