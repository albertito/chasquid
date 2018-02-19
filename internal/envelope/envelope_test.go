package envelope

import (
	"testing"

	"blitiri.com.ar/go/chasquid/internal/set"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		addr, user, domain string
	}{
		{"lalala@lelele", "lalala", "lelele"},
	}

	for _, c := range cases {
		if user := UserOf(c.addr); user != c.user {
			t.Errorf("%q: expected user %q, got %q", c.addr, c.user, user)
		}
		if domain := DomainOf(c.addr); domain != c.domain {
			t.Errorf("%q: expected domain %q, got %q",
				c.addr, c.domain, domain)
		}
	}
}

func TestDomainIn(t *testing.T) {
	ls := set.NewString("domain1", "domain2")
	cases := []struct {
		addr string
		in   bool
	}{
		{"u@domain1", true},
		{"u@domain2", true},
		{"u@domain3", false},
		{"u", true},
	}
	for _, c := range cases {
		if in := DomainIn(c.addr, ls); in != c.in {
			t.Errorf("%q: expected %v, got %v", c.addr, c.in, in)
		}
	}
}

func TestAddHeader(t *testing.T) {
	cases := []struct {
		data, k, v, expected string
	}{
		{"", "Key", "Value", "Key: Value\n"},
		{"data", "Key", "Value", "Key: Value\ndata"},
		{"data", "Key", "Value\n", "Key: Value\ndata"},
		{"data", "Key", "L1\nL2", "Key: L1\n\tL2\ndata"},
		{"data", "Key", "L1\nL2\n", "Key: L1\n\tL2\ndata"},

		// Degenerate cases: we don't expect to ever produce these, and the
		// output is admittedly not nice, but they should at least not cause
		// chasquid to crash.
		{"data", "Key", "", "Key: \ndata"},
		{"data", "", "", ": \ndata"},
		{"", "", "", ": \n"},
	}
	for i, c := range cases {
		got := string(AddHeader([]byte(c.data), c.k, c.v))
		if got != c.expected {
			t.Errorf("%d (%q -> %q): expected %q, got %q",
				i, c.k, c.v, c.expected, got)
		}
	}
}
