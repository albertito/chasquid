package dkim

import (
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
	}

	for i, c := range cases {
		_, _, err := parseMessage(c.message)
		if diff := cmp.Diff(c.err, err, cmpopts.EquateErrors()); diff != "" {
			t.Errorf("parseMessage([%d]) err mismatch (-want +got):\n%s",
				i, diff)
		}
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
