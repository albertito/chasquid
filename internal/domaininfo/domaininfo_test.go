package domaininfo

import (
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

func TestBasic(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tr := trace.New("test", "basic")
	defer tr.Finish()

	if !db.IncomingSecLevel(tr, "d1", SecLevel_PLAIN) {
		t.Errorf("new domain as plain not allowed")
	}
	if !db.IncomingSecLevel(tr, "d1", SecLevel_TLS_SECURE) {
		t.Errorf("increment to tls-secure not allowed")
	}
	if db.IncomingSecLevel(tr, "d1", SecLevel_TLS_INSECURE) {
		t.Errorf("decrement to tls-insecure was allowed")
	}

	// Check that it was added to the store and a new db sees it.
	db2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if db2.IncomingSecLevel(tr, "d1", SecLevel_TLS_INSECURE) {
		t.Errorf("decrement to tls-insecure was allowed in new DB")
	}
}

func TestNewDomain(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tr := trace.New("test", "basic")
	defer tr.Finish()

	cases := []struct {
		domain string
		level  SecLevel
	}{
		{"plain", SecLevel_PLAIN},
		{"insecure", SecLevel_TLS_INSECURE},
		{"secure", SecLevel_TLS_SECURE},
	}
	for _, c := range cases {
		if !db.IncomingSecLevel(tr, c.domain, c.level) {
			t.Errorf("domain %q not allowed (in) at %s", c.domain, c.level)
		}
		if !db.OutgoingSecLevel(tr, c.domain, c.level) {
			t.Errorf("domain %q not allowed (out) at %s", c.domain, c.level)
		}
	}
}

func TestProgressions(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tr := trace.New("test", "basic")
	defer tr.Finish()

	cases := []struct {
		domain string
		lvl    SecLevel
		ok     bool
	}{
		{"pisis", SecLevel_PLAIN, true},
		{"pisis", SecLevel_TLS_INSECURE, true},
		{"pisis", SecLevel_TLS_SECURE, true},
		{"pisis", SecLevel_TLS_INSECURE, false},
		{"pisis", SecLevel_TLS_SECURE, true},

		{"ssip", SecLevel_TLS_SECURE, true},
		{"ssip", SecLevel_TLS_SECURE, true},
		{"ssip", SecLevel_TLS_INSECURE, false},
		{"ssip", SecLevel_PLAIN, false},
	}
	for i, c := range cases {
		if ok := db.IncomingSecLevel(tr, c.domain, c.lvl); ok != c.ok {
			t.Errorf("%2d %q in  attempt for %s failed: got %v, expected %v",
				i, c.domain, c.lvl, ok, c.ok)
		}
		if ok := db.OutgoingSecLevel(tr, c.domain, c.lvl); ok != c.ok {
			t.Errorf("%2d %q out attempt for %s failed: got %v, expected %v",
				i, c.domain, c.lvl, ok, c.ok)
		}
	}
}

func TestErrors(t *testing.T) {
	// Non-existent directory.
	_, err := New("/doesnotexists")
	if err == nil {
		t.Error("could create a DB on a non-existent directory")
	}

	// Corrupt/invalid file.
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	tr := trace.New("test", "basic")
	defer tr.Finish()

	if !db.IncomingSecLevel(tr, "d1", SecLevel_TLS_SECURE) {
		t.Errorf("increment to tls-secure not allowed")
	}

	testlib.Rewrite(t, dir+"/s:d1", "invalid-text-protobuf-contents")

	err = db.Reload()
	if err == nil {
		t.Errorf("no error when reloading db with invalid file")
	}
}
