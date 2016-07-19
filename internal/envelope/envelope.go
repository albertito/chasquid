// Package envelope implements functions related to handling email envelopes
// (basically tuples of (from, to, data).
package envelope

import (
	"strings"

	"blitiri.com.ar/go/chasquid/internal/set"
)

// Split an user@domain address into user and domain.
func split(addr string) (string, string) {
	ps := strings.SplitN(addr, "@", 2)
	if len(ps) != 2 {
		return addr, ""
	}

	return ps[0], ps[1]
}

func UserOf(addr string) string {
	user, _ := split(addr)
	return user
}

func DomainOf(addr string) string {
	_, domain := split(addr)
	return domain
}

func DomainIn(addr string, locals *set.String) bool {
	domain := DomainOf(addr)
	if domain == "" {
		return true
	}

	return locals.Has(domain)
}
