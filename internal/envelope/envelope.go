// Package envelope implements functions related to handling email envelopes
// (basically tuples of (from, to, data).
package envelope

import (
	"fmt"
	"strings"

	"golang.org/x/net/idna"

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

func UserOf(addr string) string {
	user, _ := Split(addr)
	return user
}

func DomainOf(addr string) string {
	_, domain := Split(addr)
	return domain
}

func DomainIn(addr string, locals *set.String) bool {
	domain := DomainOf(addr)
	if domain == "" {
		return true
	}

	return locals.Has(domain)
}

func AddHeader(data []byte, k, v string) []byte {
	// If the value contains newlines, indent them properly.
	if v[len(v)-1] == '\n' {
		v = v[:len(v)-1]
	}
	v = strings.Replace(v, "\n", "\n\t", -1)

	header := []byte(fmt.Sprintf("%s: %s\n", k, v))
	return append(header, data...)
}

// Take an address with a potentially unicode domain, and convert it to ASCII
// as per IDNA.
// The user part is unchanged.
func IDNAToASCII(addr string) (string, error) {
	if addr == "<>" {
		return addr, nil
	}
	user, domain := Split(addr)
	domain, err := idna.ToASCII(domain)
	return user + "@" + domain, err
}

// Take an address with an ASCII domain, and convert it to Unicode as per
// IDNA.
// The user part is unchanged.
func IDNAToUnicode(addr string) (string, error) {
	if addr == "<>" {
		return addr, nil
	}
	user, domain := Split(addr)
	domain, err := idna.ToUnicode(domain)
	return user + "@" + domain, err
}
