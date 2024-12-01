package dkim

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	errUnknownCanonicalization = errors.New("unknown canonicalization")
)

type canonicalization string

var (
	simpleCanonicalization  canonicalization = "simple"
	relaxedCanonicalization canonicalization = "relaxed"
)

func (c canonicalization) body(b string) string {
	switch c {
	case simpleCanonicalization:
		return simpleBody(b)
	case relaxedCanonicalization:
		return relaxBody(b)
	default:
		panic("unknown canonicalization")
	}
}

func (c canonicalization) headers(hs headers) headers {
	switch c {
	case simpleCanonicalization:
		return hs
	case relaxedCanonicalization:
		return relaxHeaders(hs)
	default:
		panic("unknown canonicalization")
	}
}

func (c canonicalization) header(h header) header {
	switch c {
	case simpleCanonicalization:
		return h
	case relaxedCanonicalization:
		return relaxHeader(h)
	default:
		panic("unknown canonicalization")
	}
}

func stringToCanonicalization(s string) (canonicalization, error) {
	switch s {
	case "simple":
		return simpleCanonicalization, nil
	case "relaxed":
		return relaxedCanonicalization, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnknownCanonicalization, s)
	}
}

// Notes on whitespace reduction:
// https://datatracker.ietf.org/doc/html/rfc6376#section-2.8
// There are only 3 forms of whitespace:
//  - WSP  =  SP / HTAB
//    Simple whitespace: space or tab.
//  - LWSP =  *(WSP / CRLF WSP)
//    Linear whitespace: any number of { simple whitespace OR CRLF followed by
//    simple whitespace }.
//  - FWS  =  [*WSP CRLF] 1*WSP
//    Folding whitespace: optional { simple whitespace OR CRLF } followed by
//    one or more simple whitespace.

func simpleBody(body string) string {
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.3
	// Replace repeated CRLF at the end of the body with a single CRLF.
	body = repeatedCRLFAtTheEnd.ReplaceAllString(body, "\r\n")

	// Ensure a non-empty body ends with a single CRLF.
	// All bodies (including an empty one) must end with a CRLF.
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}

	return body
}

var (
	// Continued header: WSP after CRLF.
	continuedHeader = regexp.MustCompile(`\r\n[ \t]+`)

	// WSP before CRLF.
	wspBeforeCRLF = regexp.MustCompile(`[ \t]+\r\n`)

	// Repeated WSP.
	repeatedWSP = regexp.MustCompile(`[ \t]+`)

	// Empty lines at the end of the body.
	repeatedCRLFAtTheEnd = regexp.MustCompile(`(\r\n)+$`)
)

func relaxBody(body string) string {
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.4
	body = wspBeforeCRLF.ReplaceAllLiteralString(body, "\r\n")
	body = repeatedWSP.ReplaceAllLiteralString(body, " ")
	body = repeatedCRLFAtTheEnd.ReplaceAllLiteralString(body, "\r\n")

	// Ensure a non-empty body ends with a single CRLF.
	if len(body) >= 1 && !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}

	return body
}

func relaxHeader(h header) header {
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.2
	// Convert all header field names to lowercase.
	name := strings.ToLower(h.Name)

	// Remove WSP before the ":" separating the name and value.
	name = strings.TrimRight(name, " \t")

	// Unfold continuation lines in values.
	value := continuedHeader.ReplaceAllString(h.Value, " ")

	// Reduce all sequences of WSP to a single SP.
	value = repeatedWSP.ReplaceAllLiteralString(value, " ")

	// Delete all WSP at the end of each unfolded header field value.
	value = strings.TrimRight(value, " \t")

	// Remove WSP after the ":" separating the name and value.
	value = strings.TrimLeft(value, " \t")

	return header{
		Name:  name,
		Value: value,

		// The "source" is the relaxed field: name, colon, and value (with
		// no space around the colon).
		Source: name + ":" + value,
	}
}

func relaxHeaders(hs headers) headers {
	rh := make(headers, 0, len(hs))
	for _, h := range hs {
		rh = append(rh, relaxHeader(h))
	}

	return rh
}
