// Package auth implements authentication services for chasquid.
package auth

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/normalize"
)

// Interface for authentication backends.
type Backend interface {
	Authenticate(user, password string) (bool, error)
	Exists(user string) (bool, error)
	Reload() error
}

// Interface for authentication backends that don't need to emit errors.
// This allows backends to avoid unnecessary complexity, in exchange for a bit
// more here.
// They can be converted to normal Backend using WrapNoErrorBackend (defined
// below).
type NoErrorBackend interface {
	Authenticate(user, password string) bool
	Exists(user string) bool
	Reload() error
}

type Authenticator struct {
	// Registered backends, map of domain (string) -> Backend.
	// Backend operations will _not_ include the domain in the username.
	backends map[string]Backend

	// Fallback backend, to use when backends[domain] (which may not exist)
	// did not yield a positive result.
	// Note that this backend gets the user with the domain included, of the
	// form "user@domain".
	Fallback Backend

	// How long Authenticate calls should last, approximately.
	// This will be applied both for successful and unsuccessful attempts.
	// We will increase this number by 0-20%.
	AuthDuration time.Duration
}

func NewAuthenticator() *Authenticator {
	return &Authenticator{
		backends:     map[string]Backend{},
		AuthDuration: 100 * time.Millisecond,
	}
}

func (a *Authenticator) Register(domain string, be Backend) {
	a.backends[domain] = be
}

// Authenticate the user@domain with the given password.
func (a *Authenticator) Authenticate(user, domain, password string) (bool, error) {
	// Make sure the call takes a.AuthDuration + 0-20% regardless of the
	// outcome, to prevent basic timing attacks.
	defer func(start time.Time) {
		elapsed := time.Since(start)
		delay := a.AuthDuration - elapsed
		if delay > 0 {
			maxDelta := int64(float64(delay) * 0.2)
			delay += time.Duration(rand.Int63n(maxDelta))
			time.Sleep(delay)
		}
	}(time.Now())

	if be, ok := a.backends[domain]; ok {
		ok, err := be.Authenticate(user, password)
		if ok || err != nil {
			return ok, err
		}
	}

	if a.Fallback != nil {
		return a.Fallback.Authenticate(user+"@"+domain, password)
	}

	return false, nil
}

func (a *Authenticator) Exists(user, domain string) (bool, error) {
	if be, ok := a.backends[domain]; ok {
		ok, err := be.Exists(user)
		if ok || err != nil {
			return ok, err
		}
	}

	if a.Fallback != nil {
		return a.Fallback.Exists(user + "@" + domain)
	}

	return false, nil
}

// Reload the registered backends.
func (a *Authenticator) Reload() error {
	msgs := []string{}

	for domain, be := range a.backends {
		err := be.Reload()
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("%q: %v", domain, err))
		}
	}
	if a.Fallback != nil {
		err := a.Fallback.Reload()
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("<fallback>: %v", err))
		}
	}

	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, " ; "))
	}
	return nil
}

// DecodeResponse decodes a plain auth response.
//
// It must be a a base64-encoded string of the form:
//   <authorization id> NUL <authentication id> NUL <password>
//
// https://tools.ietf.org/html/rfc4954#section-4.1.
//
// Either both ID match, or one of them is empty.
// We expect the ID to be "user@domain", which is NOT an RFC requirement but
// our own.
func DecodeResponse(response string) (user, domain, passwd string, err error) {
	buf, err := base64.StdEncoding.DecodeString(response)
	if err != nil {
		return
	}

	bufsp := bytes.SplitN(buf, []byte{0}, 3)
	if len(bufsp) != 3 {
		err = fmt.Errorf("Response pieces != 3, as per RFC")
		return
	}

	identity := ""
	passwd = string(bufsp[2])

	{
		// We don't make the distinction between the two IDs, as long as one is
		// empty, or they're the same.
		z := string(bufsp[0])
		c := string(bufsp[1])

		// If neither is empty, then they must be the same.
		if (z != "" && c != "") && (z != c) {
			err = fmt.Errorf("Auth IDs do not match")
			return
		}

		if z != "" {
			identity = z
		}
		if c != "" {
			identity = c
		}
	}

	if identity == "" {
		err = fmt.Errorf("Empty identity, must be in the form user@domain")
		return
	}

	// Identity must be in the form "user@domain".
	// This is NOT an RFC requirement, it's our own.
	idsp := strings.SplitN(identity, "@", 2)
	if len(idsp) != 2 {
		err = fmt.Errorf("Identity must be in the form user@domain")
		return
	}

	user = idsp[0]
	domain = idsp[1]

	// Normalize the user and domain. This is so users can write the username
	// in their own style and still can log in.  For the domain, we use IDNA
	// and relevant transformations to turn it to utf8 which is what we use
	// internally.
	user, err = normalize.User(user)
	if err != nil {
		return
	}
	domain, err = normalize.Domain(domain)
	if err != nil {
		return
	}

	return
}

// WrapNoErrorBackend wraps a NoErrorBackend, converting it into a valid
// Backend. This is normally used in Auth.Register calls, to register no-error
// backends.
func WrapNoErrorBackend(be NoErrorBackend) Backend {
	return &wrapNoErrorBackend{be}
}

type wrapNoErrorBackend struct {
	be NoErrorBackend
}

func (w *wrapNoErrorBackend) Authenticate(user, password string) (bool, error) {
	return w.be.Authenticate(user, password), nil
}

func (w *wrapNoErrorBackend) Exists(user string) (bool, error) {
	return w.be.Exists(user), nil
}

func (w *wrapNoErrorBackend) Reload() error {
	return w.be.Reload()
}
