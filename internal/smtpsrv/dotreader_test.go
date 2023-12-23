package smtpsrv

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestReadUntilDot(t *testing.T) {
	cases := []struct {
		input   string
		max     int64
		want    string
		wantErr error
	}{
		// EOF before any input -> unexpected EOF.
		{"", 0, "", io.ErrUnexpectedEOF},
		{"", 1, "", io.ErrUnexpectedEOF},

		// EOF after exceeding max -> unexpected EOF.
		{"abcdef", 2, "ab", io.ErrUnexpectedEOF},

		// \n at the beginning of the buffer are just as invalid, and the
		// error takes precedence over the unexpected EOF.
		{"\n", 0, "", errInvalidLineEnding},
		{"\n", 1, "", errInvalidLineEnding},
		{"\n", 2, "", errInvalidLineEnding},
		{"\n\r\n.\r\n", 10, "", errInvalidLineEnding},

		// \r and then EOF -> unexpected EOF, because we never had a chance to
		// assess if the line ending is valid or not.
		{"\r", 2, "", io.ErrUnexpectedEOF},

		// Lonely \r -> invalid line ending.
		{"abc\rdef", 10, "abc", errInvalidLineEnding},
		{"abc\r\rdef", 10, "abc", errInvalidLineEnding},

		// Lonely \n -> invalid line ending.
		{"abc\ndef", 10, "abc", errInvalidLineEnding},

		// Various valid cases.
		{"abc\r\n.\r\n", 10, "abc\n", nil},
		{"\r\n.\r\n", 10, "\n", nil},

		// Start with the final dot - the smallest "message" (empty).
		{".\r\n", 10, "", nil},

		// Max bytes reached -> message too large.
		{"abc\r\n.\r\n", 5, "abc\n", errMessageTooLarge},
		{"abcdefg\r\n.\r\n", 5, "abcde", errMessageTooLarge},
		{"ab\r\ncdefg\r\n.\r\n", 5, "ab\ncd", errMessageTooLarge},

		// Dot-stuffing.
		// https://www.rfc-editor.org/rfc/rfc5321#section-4.5.2
		{"abc\r\n.def\r\n.\r\n", 20, "abc\ndef\n", nil},
		{"abc\r\n..def\r\n.\r\n", 20, "abc\n.def\n", nil},
		{"abc\r\n..\r\n.\r\n", 20, "abc\n.\n", nil},
		{".x\r\n.\r\n", 20, "x\n", nil},
		{"..\r\n.\r\n", 20, ".\n", nil},
	}

	for i, c := range cases {
		r := bufio.NewReader(strings.NewReader(c.input))
		got, err := readUntilDot(r, c.max)
		if err != c.wantErr {
			t.Errorf("case %d %q: got error %v, want %v", i, c.input, err, c.wantErr)
		}
		if !bytes.Equal(got, []byte(c.want)) {
			t.Errorf("case %d %q: got %q, want %q", i, c.input, got, c.want)
		}
	}
}

type badBuffer bytes.Buffer

func (b *badBuffer) Read(p []byte) (int, error) {
	// Return an arbitrary non-EOF error for testing.
	return 0, io.ErrNoProgress
}

func TestReadUntilDotReadError(t *testing.T) {
	r := bufio.NewReader(&badBuffer{})
	_, err := readUntilDot(r, 10)
	if err != io.ErrNoProgress {
		t.Errorf("got error %v, want %v", err, io.ErrNoProgress)
	}
}
