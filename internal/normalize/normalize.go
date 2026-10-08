// Package normalize contains functions to normalize usernames, domains and
// addresses.
package normalize

import (
	"bytes"
	"errors"
	"fmt"
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

// Domain normalizes a DNS domain into a cleaned UTF-8 form, and checks that
// it is valid (see checkDomain for details).
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

	if err := checkDomain(d); err != nil {
		return domain, err
	}
	return d, nil
}

var errInvalidDomain = errors.New("invalid domain")

// checkDomain checks that the domain, in its ASCII form, only contains
// characters that can appear in DNS names: letters, digits, '-', '.', and
// '_' (not valid in hostnames, but found in the real world).
//
// This is intentionally lenient: we don't check label structure or lengths,
// as those are not a problem for us and DNS lookups will fail anyway. The
// goal is to reject characters that could be problematic when the domain is
// used in other contexts (e.g. '/' in file paths or URLs).
// Note this also rejects domain literals (e.g. "[1.2.3.4]").
func checkDomain(domain string) error {
	// Check the ASCII form, as that is what is used for DNS lookups and
	// other external interactions. Punycode encoding preserves ASCII
	// characters, so they will show up here even in mixed labels.
	a, err := idna.ToASCII(domain)
	if err != nil {
		return err
	}

	for _, c := range []byte(a) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '.', c == '_':
		default:
			return fmt.Errorf("%w: invalid character %q", errInvalidDomain, c)
		}
	}
	return nil
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
	b := bytes.Buffer{}
	b.Grow(len(in))

	// We go line by line, but beware:
	//   Split("a\nb", "\n") -> ["a", "b"]
	//   Split("a\nb\n", "\n") -> ["a", "b", ""]
	// So we handle the last line separately.
	lines := bytes.Split(in, []byte("\n"))
	for i, line := range lines {
		b.Write(line)
		if i == len(lines)-1 {
			// Do not add newline to the last line:
			//  - If the string ends with a newline, we already added it in
			//    the previous-to-last line, and this line is "".
			//  - If the string does NOT end with a newline, this preserves
			//    that property.
			break
		}
		if !bytes.HasSuffix(line, []byte("\r")) {
			// Missing the CR.
			b.WriteByte('\r')
		}
		b.WriteByte('\n')
	}

	return b.Bytes()
}

// StringToCRLF is like ToCRLF, but operates on strings.
func StringToCRLF(in string) string {
	// We implement it the same way as ToCRLF, but with string versions.
	// This is significantly faster than converting the string to a byte
	// slice, calling ToCRLF, and converting it back.
	b := strings.Builder{}
	b.Grow(len(in))

	// We go line by line, but beware:
	//   Split("a\nb", "\n") -> ["a", "b"]
	//   Split("a\nb\n", "\n") -> ["a", "b", ""]
	// So we handle the last line separately.
	lines := strings.Split(in, "\n")
	for i, line := range lines {
		b.WriteString(line)
		if i == len(lines)-1 {
			// Do not add newline to the last line:
			//  - If the string ends with a newline, we already added it in
			//    the previous-to-last line, and this line is "".
			//  - If the string does NOT end with a newline, this preserves
			//    that property.
			break
		}
		if !strings.HasSuffix(line, "\r") {
			// Missing the CR.
			b.WriteByte('\r')
		}
		b.WriteByte('\n')
	}

	return b.String()
}
