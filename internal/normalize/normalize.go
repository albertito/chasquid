// Package normalize contains functions to normalize usernames and addresses.
package normalize

import (
	"blitiri.com.ar/go/chasquid/internal/envelope"
	"golang.org/x/text/secure/precis"
)

// User normalices an username using PRECIS.
// On error, it will also return the original username to simplify callers.
func User(user string) (string, error) {
	norm, err := precis.UsernameCaseMapped.String(user)
	if err != nil {
		return user, err
	}

	return norm, nil
}

// Name normalices an email address using PRECIS.
// On error, it will also return the original address to simplify callers.
func Addr(addr string) (string, error) {
	user, domain := envelope.Split(addr)

	user, err := User(user)
	if err != nil {
		return addr, err
	}

	return user + "@" + domain, nil
}
