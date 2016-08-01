package userdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

// Remove the file if the test was successful. Used in defer statements, to
// leave files around for inspection when the tests failed.
func removeIfSuccessful(t *testing.T, fname string) {
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
	if a.users == nil || b.users == nil {
		return a.users == nil && b.users == nil
	}

	if len(a.users) != len(b.users) {
		return false
	}

	for k, av := range a.users {
		bv, ok := b.users[k]
		if !ok || av.name != bv.name || av.password != bv.password {
			return false
		}
	}

	return true
}

var emptyDB = &DB{
	users: map[string]user{},
}

const (
	scryptNoSalt = ("#chasquid-userdb-v1\n" +
		"user1 SCRYPT@n:14,r:8,p:1,l:32, " +
		"WyZPRd08NPAkWgBuqB5kwK4fEuB6FHu/X1pA1SxnXhc=")
	scryptInvalidSalt = ("#chasquid-userdb-v1\n" +
		"user1 SCRYPT@n:99,r:8,p:1,l:16,not-valid$base64!nono== " +
		"WyZPRd08NPAkWgBuqB5kwK4fEuB6FHu/X1pA1SxnXhc=")
	scryptMissingR = ("#chasquid-userdb-v1\n" +
		"user1 SCRYPT@n:14,r:,p:1,l:32,gY3a3PIzehu7xu6KM9PeOQ== " +
		"WyZPRd08NPAkWgBuqB5kwK4fEuB6FHu/X1pA1SxnXhc=")
	scryptBadN = ("#chasquid-userdb-v1\n" +
		"user1 SCRYPT@n:99,r:8,p:1,l:32,gY3a3PIzehu7xu6KM9PeOQ== " +
		"WyZPRd08NPAkWgBuqB5kwK4fEuB6FHu/X1pA1SxnXhc=")
	scryptShortKeyLen = ("#chasquid-userdb-v1\n" +
		"user1 SCRYPT@n:99,r:8,p:1,l:16,gY3a3PIzehu7xu6KM9PeOQ== " +
		"WyZPRd08NPAkWgBuqB5kwK4fEuB6FHu/X1pA1SxnXhc=")
)

// Test various cases of loading an empty/broken database.
func TestLoad(t *testing.T) {
	cases := []struct {
		desc     string
		content  string
		fatal    bool
		fatalErr error
		warns    bool
	}{
		{"empty file", "", false, nil, false},
		{"header \\n", "#chasquid-userdb-v1\n", false, nil, false},
		{"header \\r\\n", "#chasquid-userdb-v1\r\n", false, nil, false},
		{"header EOF", "#chasquid-userdb-v1", false, nil, false},
		{"missing header", "this is not the header",
			true, ErrMissingHeader, false},
		{"invalid user", "#chasquid-userdb-v1\nnam\xa0e PLAIN pass\n",
			false, nil, true},
		{"too few fields", "#chasquid-userdb-v1\nfield1 field2\n",
			false, nil, true},
		{"too many fields", "#chasquid-userdb-v1\nf1 f2 f3 f4\n",
			false, nil, true},
		{"unknown scheme", "#chasquid-userdb-v1\nuser SCHEME pass\n",
			false, nil, true},
		{"scrypt no salt", scryptNoSalt, false, nil, true},
		{"scrypt invalid salt", scryptInvalidSalt, false, nil, true},
		{"scrypt missing R", scryptMissingR, false, nil, true},
		{"scrypt bad N", scryptBadN, false, nil, true},
		{"scrypt short key len", scryptShortKeyLen, false, nil, true},
	}

	for _, c := range cases {
		testOneLoad(t, c.desc, c.content, c.fatal, c.fatalErr, c.warns)
	}
}

func testOneLoad(t *testing.T, desc, content string, fatal bool, fatalErr error, warns bool) {
	fname := mustCreateDB(t, content)
	defer removeIfSuccessful(t, fname)
	db, warnings, err := Load(fname)
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

	if warns && warnings == nil {
		t.Errorf("case %q: expected warnings, got nil", desc)
	} else if !warns {
		for _, w := range warnings {
			t.Errorf("case %q: warning loading database: %v", desc, w)
		}
	}

	if db != nil && !dbEquals(db, emptyDB) {
		t.Errorf("case %q: DB not empty: %#v", desc, db)
	}
}

func mustLoad(t *testing.T, fname string) *DB {
	db, warnings, err := Load(fname)
	for _, w := range warnings {
		t.Errorf("warning loading database: %v", w)
	}
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
		if _, ok := db.users[name].scheme.(scryptScheme); !ok {
			t.Errorf("user %q not using scrypt: %#v", name, db.users[name])
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
	db1 := New(fname)
	db1.AddUser("user", "passwd")
	db1.Write()

	db2, ws, err := Load(fname)
	if err != nil {
		t.Fatalf("error loading: %v", err)
	}
	if len(ws) != 0 {
		t.Errorf("warnings loading: %v", ws)
	}

	if !dbEquals(db1, db2) {
		t.Errorf("databases differ. db1:%v  !=  db2:%v", db1, db2)
	}
}

func TestInvalidUsername(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	names := []string{
		" ", "  ", "a b", "ñ ñ", "a\xa0b", "a\x85b", "a\nb", "a\tb", "a\xffb"}
	for _, name := range names {
		err := db.AddUser(name, "passwd")
		if err == nil {
			t.Errorf("AddUser(%q) worked, expected it to fail", name)
		}
	}

	// Add an invalid user from behind, and check that Write fails.
	db.users["in valid"] = user{"in valid", plainScheme{}, "password"}
	err := db.Write()
	if err == nil {
		t.Errorf("Write worked, expected it to fail")
	}
}

// Test the plain scheme. Note we don't expect to use it in cases other than
// debugging, but it should be functional for that purpose.
func TestPlainScheme(t *testing.T) {
	fname := mustCreateDB(t, "")
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	db.users["user"] = user{"user", plainScheme{}, "pass word"}
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
	content := "#chasquid-userdb-v1\nu1 PLAIN pass\n"
	fname := mustCreateDB(t, content)
	defer removeIfSuccessful(t, fname)
	db := mustLoad(t, fname)

	// Add some things to the file, including a broken line.
	content += "u2 UNKNOWN pass\n"
	content += "u3 PLAIN pass\n"
	ioutil.WriteFile(fname, []byte(content), db.finfo.Mode())

	warnings, err := db.Reload()
	if err != nil {
		t.Errorf("Reload failed: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %v", warnings)
	}
	if len(db.users) != 2 {
		t.Errorf("expected 2 users, got %d", len(db.users))
	}

	// Cause an error loading, check the database is not changed.
	db.fname = "/does/not/exist"
	warnings, err = db.Reload()
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(db.users) != 2 {
		t.Errorf("expected 2 users, got %d", len(db.users))
	}

}
