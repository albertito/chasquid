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
	"blitiri.com.ar/go/chasquid/internal/trace"
)

var (
	errTimeout = fmt.Errorf("Operation timed out")
)

// Procmail delivers local mail via procmail.
type Procmail struct {
	Binary  string        // Path to the binary.
	Args    []string      // Arguments to pass.
	Timeout time.Duration // Timeout for each invocation.
}

func (p *Procmail) Deliver(from string, to string, data []byte) (error, bool) {
	tr := trace.New("Courier.Procmail", to)
	defer tr.Finish()

	// Sanitize, just in case.
	from = sanitizeForProcmail(from)
	to = sanitizeForProcmail(to)

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

	ctx, cancel := context.WithDeadline(context.Background(),
		time.Now().Add(p.Timeout))
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Binary, args...)
	cmd.Stdin = bytes.NewReader(data)

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
		err = tr.Errorf("Procmail failed: %v - %q", err, string(output))
		return err, permanent
	}

	tr.Debugf("delivered")
	return nil, false
}

// sanitizeForProcmail cleans the string, removing characters that could be
// problematic considering we will run an external command.
//
// The server does not rely on this to do substitution or proper filtering,
// that's done at a different layer; this is just for defense in depth.
func sanitizeForProcmail(s string) string {
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
