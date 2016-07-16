package auth

import (
	"encoding/base64"
	"testing"
	"time"

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
		"a\x00b", "a\x00b\x00c", "a@a\x00b@b\x00pass", "a\x00a\x00pass",
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

	// Test the correct case first
	ts := time.Now()
	if !Authenticate(db, "user", "password") {
		t.Errorf("failed valid authentication for user/password")
	}
	if time.Since(ts) < AuthenticateTime {
		t.Errorf("authentication was too fast")
	}

	// Incorrect cases.
	cases := []struct{ user, password string }{
		{"user", "incorrect"},
		{"invalid", "p"},
	}
	for _, c := range cases {
		ts = time.Now()
		if Authenticate(db, c.user, c.password) {
			t.Errorf("successful auth on %v", c)
		}
		if time.Since(ts) < AuthenticateTime {
			t.Errorf("authentication was too fast")
		}
	}

	// And the special case of a nil userdb.
	ts = time.Now()
	if Authenticate(nil, "user", "password") {
		t.Errorf("successful auth on a nil userdb")
	}
	if time.Since(ts) < AuthenticateTime {
		t.Errorf("authentication was too fast")
	}
}
