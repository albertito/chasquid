// Package envelope implements functions related to handling email envelopes
// (basically tuples of (from, to, data).
package envelope

import (
	"fmt"
	"strings"

	"blitiri.com.ar/go/chasquid/internal/set"
)

// Split an user@domain address into user and domain.
func Split(addr string) (string, string) {
	ps := strings.SplitN(addr, "@", 2)
	if len(ps) != 2 {
		return addr, ""
	}

	return ps[0], ps[1]
}

// UserOf user@domain returns user.
func UserOf(addr string) string {
	user, _ := Split(addr)
	return user
}

// DomainOf user@domain returns domain.
func DomainOf(addr string) string {
	_, domain := Split(addr)
	return domain
}

// DomainIn checks that the domain of the address is on the given set.
func DomainIn(addr string, locals *set.String) bool {
	domain := DomainOf(addr)
	if domain == "" {
		return true
	}

	return locals.Has(domain)
}

// AddHeader adds (prepends) a MIME header to the message.
func AddHeader(data []byte, k, v string) []byte {
	if len(v) > 0 {
		// If the value contains newlines, indent them properly.
		if v[len(v)-1] == '\n' {
			v = v[:len(v)-1]
		}
		v = strings.Replace(v, "\n", "\n\t", -1)
	}

	header := []byte(fmt.Sprintf("%s: %s\n", k, v))
	return append(header, data...)
}
