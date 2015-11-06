// Package courier implements various couriers for delivering messages.
package courier

import "strings"

// Courier delivers mail to a single recipient.
// It is implemented by different couriers, for both local and remote
// recipients.
type Courier interface {
	Deliver(from string, to string, data []byte) error
}

// Router decides if the destination is local or remote, and delivers
// accordingly.
type Router struct {
	Local        Courier
	Remote       Courier
	LocalDomains map[string]bool
}

func (r *Router) Deliver(from string, to string, data []byte) error {
	d := domainOf(to)
	if r.LocalDomains[d] {
		return r.Local.Deliver(from, to, data)
	} else {
		return r.Remote.Deliver(from, to, data)
	}
}

// Split an user@domain address into user and domain.
func split(addr string) (string, string) {
	ps := strings.SplitN(addr, "@", 2)
	if len(ps) != 2 {
		return addr, ""
	}

	return ps[0], ps[1]
}

func userOf(addr string) string {
	user, _ := split(addr)
	return user
}

func domainOf(addr string) string {
	_, domain := split(addr)
	return domain
}
