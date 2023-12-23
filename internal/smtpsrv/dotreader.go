package smtpsrv

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

var (
	// TODO: Include the line number and specific error, and have the
	// caller add them to the trace.
	errMessageTooLarge   = errors.New("message too large")
	errInvalidLineEnding = errors.New("invalid line ending")
)

// readUntilDot reads from r until it encounters a dot-terminated line, or we
// read max bytes. It enforces that input lines are terminated by "\r\n", and
// that there are not "lonely" "\r" or "\n"s in the input.
// It returns \n-terminated lines, which is what we use for our internal
// representation for convenience (same as textproto DotReader does).
func readUntilDot(r *bufio.Reader, max int64) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	n := int64(0)

	// Little state machine.
	const (
		prevOther = iota
		prevCR
		prevCRLF
	)
	// Start as if we just came from a '\r\n'; that way we avoid the need
	// for special-casing the dot-stuffing at the very beginning.
	prev := prevCRLF
	last4 := make([]byte, 4)
	skip := false

loop:
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			return buf, io.ErrUnexpectedEOF
		} else if err != nil {
			return buf, err
		}
		n++

		switch b {
		case '\r':
			if prev == prevCR {
				return buf, errInvalidLineEnding
			}
			prev = prevCR
			// We return a LF-terminated line, so skip the CR. This simplifies
			// internal representation and makes it easier/less error prone to
			// work with. It is converted back to CRLF on endpoints (e.g. in
			// the couriers).
			skip = true
		case '\n':
			if prev != prevCR {
				return buf, errInvalidLineEnding
			}
			// If we come from a '\r\n.\r', we're done.
			if bytes.Equal(last4, []byte("\r\n.\r")) {
				break loop
			}

			// If we are only starting and see ".\r\n", we're also done; in
			// that case the message is empty.
			if n == 3 && bytes.Equal(last4, []byte("\x00\x00.\r")) {
				return []byte{}, nil
			}
			prev = prevCRLF
		default:
			if prev == prevCR {
				return buf, errInvalidLineEnding
			}
			if b == '.' && prev == prevCRLF {
				// We come from "\r\n" and got a "."; as per dot-stuffing
				// rules, we should skip that '.' in the output.
				// https://www.rfc-editor.org/rfc/rfc5321#section-4.5.2
				skip = true
			}
			prev = prevOther
		}

		// Keep the last 4 bytes separately, because they may not be in buf on
		// messages that are too large.
		copy(last4, last4[1:])
		last4[3] = b

		if int64(len(buf)) < max && !skip {
			buf = append(buf, b)
		}
		skip = false
	}

	// Return an error if the message is too large. It is important to do this
	// _outside_ the loop, because we need to keep reading until we get to the
	// final "." before we return an error, so the SMTP dialog can continue
	// properly after that.
	// If we return too early, the remainder of the email is interpreted as
	// part of the SMTP dialog (and exposing ourselves to smuggling attacks).
	if n > max {
		return buf, errMessageTooLarge
	}

	// If we made it this far, buf naturally ends in "\n" because we skipped
	// the '.' due to dot-stuffing, and skip "\r"s.
	return buf, nil
}
