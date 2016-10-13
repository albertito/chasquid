package domaininfo

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func mustTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "greylisting_test")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("test directory: %q", dir)
	return dir
}

func TestBasic(t *testing.T) {
	dir := mustTempDir(t)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Load(); err != nil {
		t.Fatal(err)
	}

	if !db.IncomingSecLevel("d1", SecLevel_PLAIN) {
		t.Errorf("new domain as plain not allowed")
	}
	if !db.IncomingSecLevel("d1", SecLevel_TLS_SECURE) {
		t.Errorf("increment to tls-secure not allowed")
	}
	if db.IncomingSecLevel("d1", SecLevel_TLS_INSECURE) {
		t.Errorf("decrement to tls-insecure was allowed")
	}

	// Wait until it is written to disk.
	for dl := time.Now().Add(30 * time.Second); time.Now().Before(dl); {
		d := &Domain{}
		ok, _ := db.store.Get("d1", d)
		if ok {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Check that it was added to the store and a new db sees it.
	db2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db2.Load(); err != nil {
		t.Fatal(err)
	}
	if db2.IncomingSecLevel("d1", SecLevel_TLS_INSECURE) {
		t.Errorf("decrement to tls-insecure was allowed in new DB")
	}

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestNewDomain(t *testing.T) {
	dir := mustTempDir(t)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		domain string
		level  SecLevel
	}{
		{"plain", SecLevel_PLAIN},
		{"insecure", SecLevel_TLS_INSECURE},
		{"secure", SecLevel_TLS_SECURE},
	}
	for _, c := range cases {
		if !db.IncomingSecLevel(c.domain, c.level) {
			t.Errorf("domain %q not allowed (in) at %s", c.domain, c.level)
		}
		if !db.OutgoingSecLevel(c.domain, c.level) {
			t.Errorf("domain %q not allowed (out) at %s", c.domain, c.level)
		}
	}
	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestProgressions(t *testing.T) {
	dir := mustTempDir(t)
	db, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

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
		if ok := db.IncomingSecLevel(c.domain, c.lvl); ok != c.ok {
			t.Errorf("%2d %q in  attempt for %s failed: got %v, expected %v",
				i, c.domain, c.lvl, ok, c.ok)
		}
		if ok := db.OutgoingSecLevel(c.domain, c.lvl); ok != c.ok {
			t.Errorf("%2d %q out attempt for %s failed: got %v, expected %v",
				i, c.domain, c.lvl, ok, c.ok)
		}
	}

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}
