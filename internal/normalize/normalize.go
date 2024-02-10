// Package normalize contains functions to normalize usernames, domains and
// addresses.
package normalize

import (
	"bytes"
	"strings"

	"blitiri.com.ar/go/chasquid/internal/envelope"
	"golang.org/x/net/idna"
	"golang.org/x/text/secure/precis"
	"golang.org/x/text/unicode/norm"
)

// User normalizes an username using PRECIS.
// On error, it will also return the original username to simplify callers.
func User(user string) (string, error) {
	norm, err := precis.UsernameCaseMapped.String(user)
	if err != nil {
		return user, err
	}

	return norm, nil
}

// Domain normalizes a DNS domain into a cleaned UTF-8 form.
// On error, it will also return the original domain to simplify callers.
func Domain(domain string) (string, error) {
	// For now, we just convert them to lower case and make sure it's in NFC
	// form for consistency.
	// There are other possible transformations (like nameprep) but for our
	// purposes these should be enough.
	// https://tools.ietf.org/html/rfc5891#section-5.2
	// https://blog.golang.org/normalization
	d, err := idna.ToUnicode(domain)
	if err != nil {
		return domain, err
	}

	d = norm.NFC.String(d)
	d = strings.ToLower(d)
	return d, nil
}

// Addr normalizes an email address, applying User and Domain to its
// respective components.
// On error, it will also return the original address to simplify callers.
func Addr(addr string) (string, error) {
	user, domain := envelope.Split(addr)

	user, err := User(user)
	if err != nil {
		return addr, err
	}

	domain, err = Domain(domain)
	if err != nil {
		return addr, err
	}

	return user + "@" + domain, nil
}

// DomainToUnicode takes an address with an ASCII domain, and convert it to
// Unicode as per IDNA, including basic normalization.
// The user part is unchanged.
func DomainToUnicode(addr string) (string, error) {
	if addr == "<>" {
		return addr, nil
	}
	user, domain := envelope.Split(addr)

	domain, err := Domain(domain)
	return user + "@" + domain, err
}

// ToCRLF converts the given buffer to CRLF line endings. If a line has a
// preexisting CRLF, it leaves it be. It assumes that CR is never used on its
// own.
func ToCRLF(in []byte) []byte {
	b := bytes.NewBuffer(nil)
	b.Grow(len(in))
	for _, c := range in {
		switch c {
		case '\r':
			// Ignore CR, we'll add it back later. It should never appear
			// alone in the contexts where this function is used.
		case '\n':
			b.Write([]byte("\r\n"))
		default:
			b.WriteByte(c)
		}
	}
	return b.Bytes()
}

// StringToCRLF is like ToCRLF, but operates on strings.
func StringToCRLF(in string) string {
	return string(ToCRLF([]byte(in)))
}
