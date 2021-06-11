package auth

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/dovecot"
	"blitiri.com.ar/go/chasquid/internal/userdb"
)

func TestDecodeResponse(t *testing.T) {
	// Successful cases. Note we hard-code the response for extra assurance.
	cases := []struct {
		response, user, domain, passwd string
	}{
		{"dUBkAHVAZABwYXNz", "u", "d", "pass"},     // u@d\0u@d\0pass
		{"dUBkAABwYXNz", "u", "d", "pass"},         // u@d\0\0pass
		{"AHVAZABwYXNz", "u", "d", "pass"},         // \0u@d\0pass
		{"dUBkAABwYXNz/w==", "u", "d", "pass\xff"}, // u@d\0\0pass\xff
		{"dQB1AHBhc3M=", "u", "", "pass"},          // u\0u\0pass

		// "ñaca@ñeque\0\0clavaré"
		{"w7FhY2FAw7FlcXVlAABjbGF2YXLDqQ==", "ñaca", "ñeque", "clavaré"},
	}
	for _, c := range cases {
		u, d, p, err := DecodeResponse(c.response)
		if err != nil {
			t.Errorf("Error in case %v: %v", c, err)
		}

		if u != c.user || d != c.domain || p != c.passwd {
			t.Errorf("Expected %q %q %q ; got %q %q %q",
				c.user, c.domain, c.passwd, u, d, p)
		}
	}

	_, _, _, err := DecodeResponse("this is not base64 encoded")
	if err == nil {
		t.Errorf("invalid base64 did not fail as expected")
	}

	failedCases := []string{
		"", "\x00", "\x00\x00", "\x00\x00\x00", "\x00\x00\x00\x00",
		"a\x00b", "a\x00b\x00c", "a@a\x00b@b\x00pass",
		"\xffa@b\x00\xffa@b\x00pass",
	}
	for _, c := range failedCases {
		r := base64.StdEncoding.EncodeToString([]byte(c))
		_, _, _, err := DecodeResponse(r)
		if err == nil {
			t.Errorf("Expected case %q to fail, but succeeded", c)
		} else {
			t.Logf("OK: %q failed with %v", c, err)
		}
	}
}

func TestAuthenticate(t *testing.T) {
	db := userdb.New("/dev/null")
	db.AddUser("user", "password")

	a := NewAuthenticator()
	a.Register("domain", WrapNoErrorBackend(db))

	// Shorten the duration to speed up the test. This should still be long
	// enough for it to fail if we don't sleep intentionally.
	a.AuthDuration = 20 * time.Millisecond

	// Test the correct case first
	check(t, a, "user", "domain", "password", true)

	// Wrong password, but valid user@domain.
	ts := time.Now()
	if ok, _ := a.Authenticate("user", "domain", "invalid"); ok {
		t.Errorf("invalid password, but authentication succeeded")
	}
	if time.Since(ts) < a.AuthDuration {
		t.Errorf("authentication was too fast (invalid case)")
	}

	// Incorrect cases, where the user@domain do not exist.
	cases := []struct{ user, domain, password string }{
		{"user", "unknown", "password"},
		{"invalid", "domain", "p"},
		{"invalid", "unknown", "p"},
		{"user", "", "password"},
		{"invalid", "", "p"},
		{"", "domain", "password"},
		{"", "", ""},
	}
	for _, c := range cases {
		check(t, a, c.user, c.domain, c.password, false)
	}
}

func check(t *testing.T, a *Authenticator, user, domain, passwd string, expect bool) {
	c := fmt.Sprintf("{%s@%s %s}", user, domain, passwd)
	ts := time.Now()

	ok, err := a.Authenticate(user, domain, passwd)
	if time.Since(ts) < a.AuthDuration {
		t.Errorf("auth on %v was too fast", c)
	}
	if ok != expect {
		t.Errorf("auth on %v: got %v, expected %v", c, ok, expect)
	}
	if err != nil {
		t.Errorf("auth on %v: got error %v", c, err)
	}

	ok, err = a.Exists(user, domain)
	if ok != expect {
		t.Errorf("exists on %v: got %v, expected %v", c, ok, expect)
	}
	if err != nil {
		t.Errorf("exists on %v: error %v", c, err)
	}
}

func TestInterfaces(t *testing.T) {
	var _ NoErrorBackend = userdb.New("/dev/null")
	var _ Backend = dovecot.NewAuth("/dev/null", "/dev/null")
}

// Backend implementation for testing.
type TestBE struct {
	users       map[string]string
	reloadCount int
	nextError   error
}

func NewTestBE() *TestBE {
	return &TestBE{
		users: map[string]string{},
	}
}
func (d *TestBE) add(user, password string) {
	d.users[user] = password
}

func (d *TestBE) Authenticate(user, password string) (bool, error) {
	if d.nextError != nil {
		return false, d.nextError
	}

	if validP, ok := d.users[user]; ok {
		return validP == password, nil
	}
	return false, nil
}

func (d *TestBE) Exists(user string) (bool, error) {
	if d.nextError != nil {
		return false, d.nextError
	}
	_, ok := d.users[user]
	return ok, nil
}

func (d *TestBE) Reload() error {
	d.reloadCount++
	if d.nextError != nil {
		return d.nextError
	}
	return nil
}

func TestMultipleBackends(t *testing.T) {
	domain1 := NewTestBE()
	domain2 := NewTestBE()
	fallback := NewTestBE()

	a := NewAuthenticator()
	a.Register("domain1", domain1)
	a.Register("domain2", domain2)
	a.Fallback = fallback

	// Shorten the duration to speed up the test. This should still be long
	// enough for it to fail if we don't sleep intentionally.
	a.AuthDuration = 20 * time.Millisecond

	domain1.add("user1", "passwd1")
	domain2.add("user2", "passwd2")
	fallback.add("user3@fallback", "passwd3")
	fallback.add("user4@domain1", "passwd4")

	// Successful tests.
	cases := []struct{ user, domain, password string }{
		{"user1", "domain1", "passwd1"},
		{"user2", "domain2", "passwd2"},
		{"user3", "fallback", "passwd3"},
		{"user4", "domain1", "passwd4"},
	}
	for _, c := range cases {
		check(t, a, c.user, c.domain, c.password, true)
	}

	// Unsuccessful tests (users don't exist).
	cases = []struct{ user, domain, password string }{
		{"nobody", "domain1", "p"},
		{"nobody", "domain2", "p"},
		{"nobody", "fallback", "p"},
		{"user3", "", "p"},
	}
	for _, c := range cases {
		check(t, a, c.user, c.domain, c.password, false)
	}
}

func TestErrors(t *testing.T) {
	be := NewTestBE()
	be.add("user", "passwd")

	a := NewAuthenticator()
	a.Register("domain", be)
	a.AuthDuration = 0

	ok, err := a.Authenticate("user", "domain", "passwd")
	if err != nil || !ok {
		t.Fatalf("failed auth")
	}

	expectedErr := fmt.Errorf("test error")
	be.nextError = expectedErr

	ok, err = a.Authenticate("user", "domain", "passwd")
	if ok {
		t.Errorf("authentication succeeded, expected error")
	}
	if err != expectedErr {
		t.Errorf("expected error, got %v", err)
	}

	ok, err = a.Exists("user", "domain")
	if ok {
		t.Errorf("exists succeeded, expected error")
	}
	if err != expectedErr {
		t.Errorf("expected error, got %v", err)
	}
}

func TestReload(t *testing.T) {
	be1 := NewTestBE()
	be2 := NewTestBE()
	fallback := NewTestBE()

	a := NewAuthenticator()
	a.Register("domain1", be1)
	a.Register("domain2", be2)
	a.Fallback = fallback

	err := a.Reload()
	if err != nil {
		t.Errorf("unexpected error reloading: %v", err)
	}
	if be1.reloadCount != 1 || be2.reloadCount != 1 || fallback.reloadCount != 1 {
		t.Errorf("unexpected reload counts: %d %d %d != 1 1 1",
			be1.reloadCount, be2.reloadCount, fallback.reloadCount)
	}

	be2.nextError = fmt.Errorf("test error")
	err = a.Reload()
	if err == nil {
		t.Errorf("expected error reloading, got nil")
	}
	if be1.reloadCount != 2 || be2.reloadCount != 2 || fallback.reloadCount != 2 {
		t.Errorf("unexpected reload counts: %d %d %d != 2 2 2",
			be1.reloadCount, be2.reloadCount, fallback.reloadCount)
	}

	a2 := NewAuthenticator()
	a2.Register("domain", WrapNoErrorBackend(userdb.New("/dev/null")))
	if err = a2.Reload(); err != nil {
		t.Errorf("unexpected error reloading wrapped backend: %v", err)
	}
}
