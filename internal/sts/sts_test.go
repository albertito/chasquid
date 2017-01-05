package sts

import (
	"context"
	"testing"
	"time"
)

func TestParsePolicy(t *testing.T) {
	const pol1 = `{
  "version": "STSv1",
  "mode": "enforce",
  "mx": ["*.mail.example.com"],
  "max_age": 123456
}
`
	p, err := parsePolicy([]byte(pol1))
	if err != nil {
		t.Errorf("failed to parse policy: %v", err)
	}

	t.Logf("pol1: %+v", p)
}

func TestCheckPolicy(t *testing.T) {
	validPs := []Policy{
		{Version: "STSv1", Mode: "enforce", MaxAge: 1 * time.Hour,
			MXs: []string{"mx1", "mx2"}},
		{Version: "STSv1", Mode: "report", MaxAge: 1 * time.Hour,
			MXs: []string{"mx1"}},
	}
	for i, p := range validPs {
		if err := p.Check(); err != nil {
			t.Errorf("%d policy %v failed check: %v", i, p, err)
		}
	}

	invalid := []struct {
		p        Policy
		expected error
	}{
		{Policy{Version: "STSv2"}, ErrUnknownVersion},
		{Policy{Version: "STSv1"}, ErrInvalidMaxAge},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "blah"}, ErrInvalidMode},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "enforce"}, ErrInvalidMX},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "enforce", MXs: []string{}},
			ErrInvalidMX},
	}
	for i, c := range invalid {
		if err := c.p.Check(); err != c.expected {
			t.Errorf("%d policy %v check: expected %v, got %v", i, c.p,
				c.expected, err)
		}
	}
}

func TestMatchDomain(t *testing.T) {
	cases := []struct {
		domain, pattern string
		expected        bool
	}{
		{"lalala", "lalala", true},
		{"a.b.", "a.b", true},
		{"a.b", "a.b.", true},
		{"abc.com", "*.com", true},

		{"abc.com", "abc.*.com", false},
		{"abc.com", "x.abc.com", false},
		{"x.abc.com", "*.*.com", false},

		{"ñaca.com", "ñaca.com", true},
		{"Ñaca.com", "ñaca.com", true},
		{"ñaca.com", "Ñaca.com", true},
		{"x.ñaca.com", "x.xn--aca-6ma.com", true},
		{"x.naca.com", "x.xn--aca-6ma.com", false},
	}

	for _, c := range cases {
		if r := matchDomain(c.domain, c.pattern); r != c.expected {
			t.Errorf("matchDomain(%q, %q) = %v, expected %v",
				c.domain, c.pattern, r, c.expected)
		}
	}
}

func TestFetch(t *testing.T) {
	// Normal fetch, all valid.
	fakeContent["https://mta-sts.domain.com/.well-known/mta-sts.json"] = `
		{
             "version": "STSv1",
             "mode": "enforce",
             "mx": ["*.mail.example.com"],
             "max_age": 123456
        }`
	p, err := Fetch(context.Background(), "domain.com")
	if err != nil {
		t.Errorf("failed to fetch policy: %v", err)
	}
	t.Logf("domain.com: %+v", p)

	// Domain without a policy (HTTP get fails).
	p, err = Fetch(context.Background(), "unknown")
	if err == nil {
		t.Errorf("fetched unknown policy: %v", p)
	}
	t.Logf("unknown: got error as expected: %v", err)

	// Domain with an invalid policy (unknown version).
	fakeContent["https://mta-sts.version99/.well-known/mta-sts.json"] = `
		{
             "version": "STSv99",
             "mode": "enforce",
             "mx": ["*.mail.example.com"],
             "max_age": 123456
        }`
	p, err = Fetch(context.Background(), "version99")
	if err != ErrUnknownVersion {
		t.Errorf("expected error %v, got %v (and policy: %v)",
			ErrUnknownVersion, err, p)
	}
	t.Logf("version99: got expected error: %v", err)
}
