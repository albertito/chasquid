package courier

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode"

	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

var (
	errTimeout = fmt.Errorf("operation timed out")
)

// MDA delivers local mail by executing a local binary, like procmail or
// maildrop.  It works with any binary that:
//   - Receives the email to deliver via stdin.
//   - Exits with code EX_TEMPFAIL (75) for transient issues.
type MDA struct {
	Binary  string        // Path to the binary.
	Args    []string      // Arguments to pass.
	Timeout time.Duration // Timeout for each invocation.
}

// Deliver an email. On failures, returns an error, and whether or not it is
// permanent.
func (p *MDA) Deliver(from string, to string, data []byte) (error, bool) {
	tr := trace.New("Courier.MDA", to)
	defer tr.Finish()

	// Sanitize, just in case.
	from = sanitizeForMDA(from)
	to = sanitizeForMDA(to)

	tr.Debugf("%s -> %s", from, to)

	// Prepare the command, replacing the necessary arguments.
	replacer := strings.NewReplacer(
		"%from%", from,
		"%from_user%", envelope.UserOf(from),
		"%from_domain%", envelope.DomainOf(from),

		"%to%", to,
		"%to_user%", envelope.UserOf(to),
		"%to_domain%", envelope.DomainOf(to),
	)

	args := []string{}
	for _, a := range p.Args {
		args = append(args, replacer.Replace(a))
	}
	tr.Debugf("%s %q", p.Binary, args)

	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Binary, args...)

	// Pass the email data via stdin. Normalize it to CRLF which is what the
	// RFC-compliant representation require. By doing this at this end, we can
	// keep a simpler internal representation and ensure there won't be any
	// inconsistencies in newlines within the message (e.g. added headers).
	cmd.Stdin = bytes.NewReader(normalize.ToCRLF(data))

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return tr.Error(errTimeout), false
	}

	if err != nil {
		// Determine if the error is permanent or not.
		// Default to permanent, but error code 75 is transient by general
		// convention (/usr/include/sysexits.h), and commonly relied upon.
		permanent := true
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				permanent = status.ExitStatus() != 75
			}
		}
		err = tr.Errorf("MDA delivery failed: %v - %q", err, string(output))
		return err, permanent
	}

	tr.Debugf("delivered")
	return nil, false
}

// sanitizeForMDA cleans the string, removing characters that could be
// problematic considering we will run an external command.
//
// The server does not rely on this to do substitution or proper filtering,
// that's done at a different layer; this is just for defense in depth.
func sanitizeForMDA(s string) string {
	valid := func(r rune) rune {
		switch {
		case unicode.IsSpace(r), unicode.IsControl(r),
			strings.ContainsRune("/;\"'\\|*&$%()[]{}`!", r):
			return rune(-1)
		default:
			return r
		}
	}
	return strings.Map(valid, s)
}
