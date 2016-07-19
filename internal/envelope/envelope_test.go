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
