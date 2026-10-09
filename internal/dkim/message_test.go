package dkim

import (
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/normalize"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseMessage(t *testing.T) {
	cases := []struct {
		message string
		headers headers
		body    string
	}{
		{
			message: normalize.StringToCRLF(`From: a@b
To: c@d
Subject: test
Continues: This
  continues.

body`),
			headers: headers{
				header{Name: "From", Value: " a@b",
					Source: "From: a@b"},
				header{Name: "To", Value: " c@d",
					Source: "To: c@d"},
				header{Name: "Subject", Value: " test",
					Source: "Subject: test"},
				header{Name: "Continues", Value: " This\r\n  continues.",
					Source: "Continues: This\r\n  continues."},
			},
			body: "body",
		},
		{
			// Multiple continuation lines, with spaces and tabs, in a header
			// that is not the last one. Colons in continuation lines are
			// part of the value.
			message: normalize.StringToCRLF(`From: a@b
Multi: one
 two:three
	four
To: c@d

body`),
			headers: headers{
				header{Name: "From", Value: " a@b",
					Source: "From: a@b"},
				header{Name: "Multi",
					Value:  " one\r\n two:three\r\n\tfour",
					Source: "Multi: one\r\n two:three\r\n\tfour"},
				header{Name: "To", Value: " c@d",
					Source: "To: c@d"},
			},
			body: "body",
		},
		{
			// Empty message.
			message: "",
			headers: headers{},
			body:    "",
		},
	}

	for i, c := range cases {
		headers, body, err := parseMessage(c.message)
		if diff := cmp.Diff(c.headers, headers); diff != "" {
			t.Errorf("parseMessage([%d]) headers mismatch (-want +got):\n%s",
				i, diff)
		}
		if diff := cmp.Diff(c.body, body); diff != "" {
			t.Errorf("parseMessage([%d]) body mismatch (-want +got):\n%s",
				i, diff)
		}
		if err != nil {
			t.Errorf("parseMessage([%d]) error: %v", i, err)
		}

	}
}

func TestParseMessageWithErrors(t *testing.T) {
	cases := []struct {
		message string
		err     error
	}{
		{
			// Continuation without previous header.
			message: " continuation.",
			err:     errInvalidHeader,
		},
		{
			// Header without ':'.
			message: "No colon",
			err:     errInvalidHeader,
		},
		{
			// Header without ':' in its first line, even if there's one in
			// a continuation line.
			message: "No colon\r\n but: here",
			err:     errInvalidHeader,
		},
	}

	for i, c := range cases {
		_, _, err := parseMessage(c.message)
		if diff := cmp.Diff(c.err, err, cmpopts.EquateErrors()); diff != "" {
			t.Errorf("parseMessage([%d]) err mismatch (-want +got):\n%s",
				i, diff)
		}
	}
}

func TestParseMessageWithoutBlankLine(t *testing.T) {
	// If there's no blank line, all the lines are headers, including the
	// continuation lines of the last one, and the body is empty.
	hs, body, err := parseMessage("From: a@b\r\nTo: c@d\r\n e@f")
	if err != nil {
		t.Fatalf("parseMessage error: %v", err)
	}
	want := headers{
		{Name: "From", Value: " a@b", Source: "From: a@b"},
		{Name: "To", Value: " c@d\r\n e@f", Source: "To: c@d\r\n e@f"},
	}
	if diff := cmp.Diff(want, hs); diff != "" {
		t.Errorf("parseMessage headers mismatch (-want +got):\n%s", diff)
	}
	if body != "" {
		t.Errorf("parseMessage body: wanted empty, got %q", body)
	}
}

func TestParseMessageManyContinuations(t *testing.T) {
	// Regression test for parsing being quadratic on the number of
	// continuation lines of a header. This takes milliseconds, while the
	// quadratic implementation took minutes with this input. We don't check
	// the timing, so a regression will show up as this test being very slow.
	const n = 1_000_000
	value := strings.Repeat("\r\n a", n)
	message := "From: a@b\r\nLong:" + value + "\r\n\r\nbody"

	headers, body, err := parseMessage(message)
	if err != nil {
		t.Fatalf("parseMessage error: %v", err)
	}
	if len(headers) != 2 {
		t.Fatalf("wanted 2 headers, got %d", len(headers))
	}
	if headers[1].Name != "Long" {
		t.Fatalf("unexpected header name: %q", headers[1].Name)
	}
	if headers[1].Value != value {
		t.Errorf("unexpected value (len %d, want len %d)",
			len(headers[1].Value), len(value))
	}
	if headers[1].Source != "Long:"+value {
		t.Errorf("unexpected source (len %d, want len %d)",
			len(headers[1].Source), len("Long:"+value))
	}
	if body != "body" {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestHeadersFindAll(t *testing.T) {
	hs := headers{
		{Name: "From", Value: "a@b", Source: "From: a@b"},
		{Name: "To", Value: "c@d", Source: "To: c@d"},
		{Name: "Subject", Value: "test", Source: "Subject: test"},
		{Name: "fROm", Value: "z@y", Source: "fROm:  z@y"},
	}

	fromHs := hs.FindAll("froM")
	expected := headers{
		{Name: "From", Value: "a@b", Source: "From: a@b"},
		{Name: "fROm", Value: "z@y", Source: "fROm:  z@y"},
	}
	if diff := cmp.Diff(expected, fromHs); diff != "" {
		t.Errorf("headers.Find() mismatch (-want +got):\n%s", diff)
	}

}
