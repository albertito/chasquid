package dkim

import (
	"errors"
	"fmt"
	"strings"
)

type header struct {
	Name   string
	Value  string
	Source string
}

type headers []header

// FindAll the headers with the given name, in order of appearance.
func (h headers) FindAll(name string) headers {
	hs := make(headers, 0)
	for _, header := range h {
		// Compare using strings.ToLower and not strings.EqualFold, to be
		// consistent with the verifier (see headersToInclude). They differ
		// for some non-ASCII characters, which are invalid in header names
		// but can still appear in messages.
		if strings.ToLower(header.Name) == strings.ToLower(name) {
			hs = append(hs, header)
		}
	}
	return hs
}

var errInvalidHeader = errors.New("invalid header")

// Parse a RFC822 message, return the headers, body, and error if any.
// We expect it to only contain CRLF line endings.
// Does NOT touch whitespace, this is important to preserve the original
// message and headers, which is required for the signature.
func parseMessage(message string) (headers, string, error) {
	lines := strings.Split(message, "\r\n")

	// Group the lines of each header: the first line, followed by its
	// continuation lines (if any).
	groups := [][]string{}

	// End of headers. If there's no blank line, all the lines are headers.
	eoh := len(lines)
	for i, line := range lines {
		if line == "" {
			eoh = i
			break
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			// Continuation of the previous header.
			if len(groups) == 0 {
				return nil, "", fmt.Errorf(
					"%w: bad continuation", errInvalidHeader)
			}
			last := len(groups) - 1
			groups[last] = append(groups[last], line)
		} else {
			// New header.
			groups = append(groups, []string{line})
		}
	}

	headers := make(headers, 0, len(groups))
	for _, g := range groups {
		h, err := parseHeader(g)
		if err != nil {
			return nil, "", err
		}
		headers = append(headers, h)
	}

	// The body is everything after the blank line (if there is one).
	body := ""
	if eoh < len(lines) {
		body = strings.Join(lines[eoh+1:], "\r\n")
	}

	return headers, body, nil
}

// Parse a header, given its lines (the first one, followed by the
// continuation lines, if any).
func parseHeader(lines []string) (header, error) {
	// The colon must be in the first line.
	if !strings.Contains(lines[0], ":") {
		return header{}, fmt.Errorf("%w: no colon", errInvalidHeader)
	}

	source := strings.Join(lines, "\r\n")
	name, value, _ := strings.Cut(source, ":")
	return header{
		Name:   name,
		Value:  value,
		Source: source,
	}, nil
}
