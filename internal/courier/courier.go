// Package courier implements various couriers for delivering messages.
package courier

import "blitiri.com.ar/go/chasquid/internal/envelope"

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
	d := envelope.DomainOf(to)
	if r.LocalDomains[d] {
		return r.Local.Deliver(from, to, data)
	} else {
		return r.Remote.Deliver(from, to, data)
	}
}
