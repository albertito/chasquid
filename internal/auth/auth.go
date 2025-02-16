// Package auth implements authentication services for chasquid.
package auth

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

// Backend is the common interface for all authentication backends.
type Backend interface {
	Authenticate(user, password string) (bool, error)
	Exists(user string) (bool, error)
	Reload() error
}

// NoErrorBackend is the interface for authentication backends that don't need
// to emit errors.  This allows backends to avoid unnecessary complexity, in
// exchange for a bit more here.
// They can be converted to normal Backend using WrapNoErrorBackend (defined
// below).
type NoErrorBackend interface {
	Authenticate(user, password string) bool
	Exists(user string) bool
	Reload() error
}

// Authenticator tracks the backends for each domain, and allows callers to
// query them with a more practical API.
type Authenticator struct {
	// Registered backends, map of domain (string) -> Backend.
	// Backend operations will _not_ include the domain in the username.
	backends map[string]Backend

	// Fallback backend, to use when backends[domain] (which may not exist)
	// did not yield a positive result.
	// Note that this backend gets the user with the domain included, of the
	// form "user@domain" (if available).
	Fallback Backend

	// How long Authenticate calls should last, approximately.
	// This will be applied both for successful and unsuccessful attempts.
	// We will increase this number by 0-20%.
	AuthDuration time.Duration
}

// NewAuthenticator returns a new Authenticator with no backends.
func NewAuthenticator() *Authenticator {
	return &Authenticator{
		backends:     map[string]Backend{},
		AuthDuration: 100 * time.Millisecond,
	}
}

// Register a backend to use for the given domain.
func (a *Authenticator) Register(domain string, be Backend) {
	a.backends[domain] = be
}

// Authenticate the user@domain with the given password.
func (a *Authenticator) Authenticate(tr *trace.Trace, user, domain, password string) (bool, error) {
	tr = tr.NewChild("Auth.Authenticate", user+"@"+domain)
	defer tr.Finish()

	// Make sure the call takes a.AuthDuration + 0-20% regardless of the
	// outcome, to prevent basic timing attacks.
	defer func(start time.Time) {
		elapsed := time.Since(start)
		delay := a.AuthDuration - elapsed
		if delay > 0 {
			maxDelta := int64(float64(delay) * 0.2)
			delay += time.Duration(rand.Int64N(maxDelta))
			time.Sleep(delay)
		}
	}(time.Now())

	if be, ok := a.backends[domain]; ok {
		ok, err := be.Authenticate(user, password)
		tr.Debugf("Backend: %v %v", ok, err)
		if ok || err != nil {
			return ok, err
		}
	}

	if a.Fallback != nil {
		id := user
		if domain != "" {
			id = user + "@" + domain
		}
		ok, err := a.Fallback.Authenticate(id, password)
		tr.Debugf("Fallback: %v %v", ok, err)
		return ok, err
	}

	tr.Debugf("Rejected by default")
	return false, nil
}

// Exists checks that user@domain exists.
func (a *Authenticator) Exists(tr *trace.Trace, user, domain string) (bool, error) {
	tr = tr.NewChild("Auth.Exists", user+"@"+domain)
	defer tr.Finish()

	if be, ok := a.backends[domain]; ok {
		ok, err := be.Exists(user)
		tr.Debugf("Backend: %v %v", ok, err)
		if ok || err != nil {
			return ok, err
		}
	}

	if a.Fallback != nil {
		id := user
		if domain != "" {
			id = user + "@" + domain
		}
		ok, err := a.Fallback.Exists(id)
		tr.Debugf("Fallback: %v %v", ok, err)
		return ok, err
	}

	tr.Debugf("Rejected by default")
	return false, nil
}

// Reload the registered backends.
func (a *Authenticator) Reload() error {
	msgs := []string{}

	for domain, be := range a.backends {
		tr := trace.New("Auth.Reload", domain)
		err := be.Reload()
		if err != nil {
			tr.Error(err)
			msgs = append(msgs, fmt.Sprintf("%q: %v", domain, err))
		}
		tr.Finish()
	}
	if a.Fallback != nil {
		tr := trace.New("Auth.Reload", "<fallback>")
		err := a.Fallback.Reload()
		if err != nil {
			tr.Error(err)
			msgs = append(msgs, fmt.Sprintf("<fallback>: %v", err))
		}
		tr.Finish()
	}

	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, " ; "))
	}
	return nil
}

// DecodeResponse decodes a plain auth response.
//
// It must be a a base64-encoded string of the form:
//
//	<authorization id> NUL <authentication id> NUL <password>
//
// https://tools.ietf.org/html/rfc4954#section-4.1.
//
// Either both IDs match, or one of them is empty.
//
// We split the id into user@domain, since in most cases we expect that to be
// the used form, and normalize them. If there is no domain, we just return
// "" for it. The rest of the stack will know how to handle it.
func DecodeResponse(response string) (user, domain, passwd string, err error) {
	buf, err := base64.StdEncoding.DecodeString(response)
	if err != nil {
		return
	}

	bufsp := bytes.SplitN(buf, []byte{0}, 3)
	if len(bufsp) != 3 {
		err = fmt.Errorf("response pieces != 3, as per RFC")
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
			err = fmt.Errorf("auth IDs do not match")
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
		err = fmt.Errorf("empty identity, must be in the form user@domain")
		return
	}

	// Split identity into "user@domain", if possible.
	user = identity
	idsp := strings.SplitN(identity, "@", 2)
	if len(idsp) >= 2 {
		user = idsp[0]
		domain = idsp[1]
	}

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
