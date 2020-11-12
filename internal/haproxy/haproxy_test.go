package haproxy

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
)

func TestNoNewline(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("PROXY "))
	_, _, err := Handshake(r)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestBasic(t *testing.T) {
	var (
		src4, _ = net.ResolveTCPAddr("tcp", "1.1.1.1:3333")
		dst4, _ = net.ResolveTCPAddr("tcp", "2.2.2.2:4444")
		src6, _ = net.ResolveTCPAddr("tcp", "[5::5]:7777")
		dst6, _ = net.ResolveTCPAddr("tcp", "[6::6]:8888")
	)

	cases := []struct {
		str      string
		src, dst net.Addr
		err      error
	}{
		// Early line errors.
		{"", nil, nil, errInvalidProtoID},
		{"lalala", nil, nil, errInvalidProtoID},
		{"PROXY", nil, nil, errInvalidProtoID},
		{"PROXY lalala", nil, nil, errUnkProtocol},
		{"PROXY UNKNOWN", nil, nil, errUnkProtocol},

		// Number of field errors.
		{"PROXY TCP4", nil, nil, errInvalidFields},
		{"PROXY TCP4 a", nil, nil, errInvalidFields},
		{"PROXY TCP4 a b", nil, nil, errInvalidFields},
		{"PROXY TCP4 a b c", nil, nil, errInvalidFields},

		// Parsing of ipv4 addresses.
		{"PROXY TCP4 a b c d", nil, nil, errInvalidSrcIP},
		{"PROXY TCP4 1.1.1.1 b c d",
			nil, nil, errInvalidDstIP},
		{"PROXY TCP4 1.1.1.1 2.2.2.2 c d",
			nil, nil, errInvalidSrcPort},
		{"PROXY TCP4 1.1.1.1 2.2.2.2 3333 d",
			nil, nil, errInvalidDstPort},
		{"PROXY TCP4 1.1.1.1 2.2.2.2 3333 4444",
			src4, dst4, nil},

		// Parsing of ipv6 addresses.
		{"PROXY TCP6 a b c d", nil, nil, errInvalidSrcIP},
		{"PROXY TCP6 5::5 b c d",
			nil, nil, errInvalidDstIP},
		{"PROXY TCP6 5::5 6::6 c d",
			nil, nil, errInvalidSrcPort},
		{"PROXY TCP6 5::5 6::6 7777 d",
			nil, nil, errInvalidDstPort},
		{"PROXY TCP6 5::5 6::6 7777 8888",
			src6, dst6, nil},
	}

	for i, c := range cases {
		t.Logf("testing %d: %v", i, c.str)

		src, dst, err := Handshake(newR(c.str))

		if !addrEq(src, c.src) {
			t.Errorf("%d: got src %v, expected %v", i, src, c.src)
		}
		if !addrEq(dst, c.dst) {
			t.Errorf("%d: got dst %v, expected %v", i, dst, c.dst)
		}
		if err != c.err {
			t.Errorf("%d: got error %v, expected %v", i, err, c.err)
		}
	}
}

func newR(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s + "\r\n"))
}

func addrEq(a, b net.Addr) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	ta := a.(*net.TCPAddr)
	tb := b.(*net.TCPAddr)
	return ta.IP.Equal(tb.IP) && ta.Port == tb.Port
}
