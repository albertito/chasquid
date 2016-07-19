// Package courier implements various couriers for delivering messages.
package courier

// Courier delivers mail to a single recipient.
// It is implemented by different couriers, for both local and remote
// recipients.
type Courier interface {
	Deliver(from string, to string, data []byte) error
}
