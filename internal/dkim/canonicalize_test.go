package dkim

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestStringToCanonicalization(t *testing.T) {
	cases := []struct {
		in   string
		want canonicalization
		err  error
	}{
		{"simple", simpleCanonicalization, nil},
		{"relaxed", relaxedCanonicalization, nil},
		{"", "", errUnknownCanonicalization},
		{" ", "", errUnknownCanonicalization},
		{" simple", "", errUnknownCanonicalization},
		{"simple ", "", errUnknownCanonicalization},
		{"si mple ", "", errUnknownCanonicalization},
	}

	for _, c := range cases {
		got, err := stringToCanonicalization(c.in)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("stringToCanonicalization(%q) diff (-want +got): %s",
				c.in, diff)
		}
		diff := cmp.Diff(c.err, err, cmpopts.EquateErrors())
		if diff != "" {
			t.Errorf("stringToCanonicalization(%q) err diff (-want +got): %s",
				c.in, diff)
		}
	}
}

func TestSimpleBody(t *testing.T) {
	cases := []struct {
		in, want string
	}{

		// Bodies end with \r\n, including the empty one.
		{"", "\r\n"},
		{"a", "a\r\n"},
		{"a\r\n", "a\r\n"},

		// Repeated CRLF at the end of the body is replaced with a single CRLF.
		{"Body \r\n\r\n\r\n", "Body \r\n"},

		// Example from RFC.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.5
		{
			" C \r\nD \t E\r\n\r\n\r\n",
			" C \r\nD \t E\r\n",
		},
	}

	for _, c := range cases {
		got := simpleCanonicalization.body(c.in)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("simpleCanonicalization.body(%q) diff (-want +got): %s",
				c.in, diff)
		}
	}
}

func TestRelaxBody(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a\r\n", "a\r\n"},

		// Repeated WSP before CRLF.
		{"a \r\n", "a\r\n"},
		{"a  \r\n", "a\r\n"},
		{"a \t \r\n", "a\r\n"},
		{"a\t\t\t\r\n", "a\r\n"},

		// Repeated WSP within a line.
		{"a   b\r\n", "a b\r\n"},
		{"a\t\t\tb\r\n", "a b\r\n"},
		{"a \t \t b\r\n", "a b\r\n"},

		// Ignore empty lines at the end.
		{"a\r\n\r\n", "a\r\n"},
		{"a\r\n\r\n\r\n", "a\r\n"},

		// Body must end with \r\n, unless it's empty.
		{"", ""},
		{"\r\n", "\r\n"},
		{"a", "a\r\n"},

		// Example from RFC.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.5
		{" C \r\nD \t E\r\n\r\n\r\n", " C\r\nD E\r\n"},
	}

	for _, c := range cases {
		got := relaxedCanonicalization.body(c.in)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("relaxedCanonicalization.body(%q) diff (-want +got): %s",
				c.in, diff)
		}
	}
}

func mkHs(hs ...string) headers {
	var headers headers
	for i := 0; i < len(hs); i += 2 {
		h := header{
			Name:   hs[i],
			Value:  hs[i+1],
			Source: hs[i] + ":" + hs[i+1],
		}
		headers = append(headers, h)
	}
	return headers
}

func TestHeaders(t *testing.T) {
	cases := []struct {
		in    string
		wantS headers
		wantR headers
	}{
		// Unfold headers.
		{"A: B\r\n C\r\n", mkHs("A", " B\r\n C"), mkHs("a", "B C")},
		{"A: B\r\n\tC\r\n", mkHs("A", " B\r\n\tC"), mkHs("a", "B C")},
		{"A: B\r\n  \t  C\r\n", mkHs("A", " B\r\n  \t  C"), mkHs("a", "B C")},

		// Reduce all sequences of WSP within a line to a single SP.
		{"A: B  C\r\n", mkHs("A", " B  C"), mkHs("a", "B C")},
		{"A: B\t\tC\r\n", mkHs("A", " B\t\tC"), mkHs("a", "B C")},
		{"A: B \t \t C\r\n", mkHs("A", " B \t \t C"), mkHs("a", "B C")},

		// Delete all WSP at the end of each unfolded header field.
		{"A: B \r\n", mkHs("A", " B "), mkHs("a", "B")},
		{"A: B  \r\n", mkHs("A", " B  "), mkHs("a", "B")},
		{"A: B\t \r\n", mkHs("A", " B\t "), mkHs("a", "B")},
		{"A: B\t\t\t\r\n", mkHs("A", " B\t\t\t"), mkHs("a", "B")},
		{"A: B\r\n  \t  C   \t\r\n",
			mkHs("A", " B\r\n  \t  C   \t"), mkHs("a", "B C")},

		// Whitespace before and after the colon.
		{"A : B\r\n", mkHs("A ", " B"), mkHs("a", "B")},
		{"A  :  B\r\n", mkHs("A  ", "  B"), mkHs("a", "B")},
		{"A\t:\tB\r\n", mkHs("A\t", "\tB"), mkHs("a", "B")},
		{"A\t \t : \t \tB\r\n", mkHs("A\t \t ", " \t \tB"), mkHs("a", "B")},

		// Example from RFC.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-3.4.5
		{"A: X\r\nB : Y\t\r\n\tZ  \r\n",
			mkHs("A", " X", "B ", " Y\t\r\n\tZ  "),
			mkHs("a", "X", "b", "Y Z")},
	}

	for i, c := range cases {
		hs, _, err := parseMessage(c.in)
		if err != nil {
			t.Fatalf("parseMessage(%q) = %v, want nil", c.in, err)
		}

		gotS := simpleCanonicalization.headers(hs)
		if diff := cmp.Diff(c.wantS, gotS); diff != "" {
			t.Errorf("%d: simpleCanonicalization.headers(%q) diff (-want +got): %s",
				i, c.in, diff)
		}

		gotR := relaxedCanonicalization.headers(hs)
		if diff := cmp.Diff(c.wantR, gotR); diff != "" {
			t.Errorf("%d: relaxedCanonicalization.headers(%q) diff (-want +got): %s",
				i, c.in, diff)
		}

		// Test the single-header variant if possible.
		if len(hs) == 1 {
			gotS := simpleCanonicalization.header(hs[0])
			if diff := cmp.Diff(c.wantS[0], gotS); diff != "" {
				t.Errorf("%d: simpleCanonicalization.header(%q) diff (-want +got): %s",
					i, c.in, diff)
			}

			gotR := relaxedCanonicalization.header(hs[0])
			if diff := cmp.Diff(c.wantR[0], gotR); diff != "" {
				t.Errorf("%d: relaxedCanonicalization.header(%q) diff (-want +got): %s",
					i, c.in, diff)
			}
		}
	}
}

func TestBadCanonicalization(t *testing.T) {
	bad := canonicalization("bad")
	if !panics(func() { bad.body("") }) {
		t.Errorf("bad.body() did not panic")
	}
	if !panics(func() { bad.header(header{}) }) {
		t.Errorf("bad.header() did not panic")
	}
	if !panics(func() { bad.headers(nil) }) {
		t.Errorf("bad.headers() did not panic")
	}
}

func panics(f func()) (panicked bool) {
	defer func() {
		r := recover()
		panicked = r != nil
	}()
	f()
	return
}
