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
		if strings.EqualFold(header.Name, name) {
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
	headers := make(headers, 0)
	lines := strings.Split(message, "\r\n")
	eoh := 0
	for i, line := range lines {
		if line == "" {
			eoh = i
			break
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			// Continuation of the previous header.
			if len(headers) == 0 {
				return nil, "", fmt.Errorf(
					"%w: bad continuation", errInvalidHeader)
			}
			headers[len(headers)-1].Value += "\r\n" + line
			headers[len(headers)-1].Source += "\r\n" + line
		} else {
			// New header.
			h, err := parseHeader(line)
			if err != nil {
				return nil, "", err
			}

			headers = append(headers, h)
		}
	}

	return headers, strings.Join(lines[eoh+1:], "\r\n"), nil
}

func parseHeader(line string) (header, error) {
	name, value, found := strings.Cut(line, ":")
	if !found {
		return header{}, fmt.Errorf("%w: no colon", errInvalidHeader)
	}

	return header{
		Name:   name,
		Value:  value,
		Source: line,
	}, nil
}
