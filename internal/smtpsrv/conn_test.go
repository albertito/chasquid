package smtpsrv

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/chasquid/internal/trace"
	"blitiri.com.ar/go/spf"
)

func TestSecLevel(t *testing.T) {
	// We can't simulate this externally because of the SPF record
	// requirement, so do a narrow test on Conn.secLevelCheck.
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	dinfo, err := domaininfo.New(dir)
	if err != nil {
		t.Fatalf("Failed to create domain info: %v", err)
	}

	c := &Conn{
		tr:    trace.New("testconn", "testconn"),
		dinfo: dinfo,
	}

	// No SPF, skip security checks.
	c.spfResult = spf.None
	c.onTLS = true
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("TLS seclevel failed")
	}

	c.onTLS = false
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel failed, even though SPF does not exist")
	}

	// Now the real checks, once SPF passes.
	c.spfResult = spf.Pass

	if !c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel failed")
	}

	c.onTLS = true
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("TLS seclevel failed")
	}

	c.onTLS = false
	if c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel worked, downgrade was allowed")
	}
}

func TestIsHeader(t *testing.T) {
	no := []string{
		"a", "\n", "\n\n", " \n", " ",
		"a:b", "a:  b\nx: y",
		"\na:b\n", " a\nb:c\n",
	}
	for _, s := range no {
		if isHeader([]byte(s)) {
			t.Errorf("%q accepted as header, should be rejected", s)
		}
	}

	yes := []string{
		"", "a:b\n",
		"X-Post-Data: success\n",
	}
	for _, s := range yes {
		if !isHeader([]byte(s)) {
			t.Errorf("%q rejected as header, should be accepted", s)
		}
	}
}

func TestReadUntilDot(t *testing.T) {
	// This must be > than the minimum buffer size for bufio.Reader, which
	// unfortunately is not available to us. The current value is 16, these
	// tests will break if it gets increased, and the nonfinal cases will need
	// to be adjusted.
	size := 20
	xs := "12345678901234567890"

	final := []string{
		"", ".", "..",
		".\r\n", "\r\n.", "\r\n.\r\n",
		".\n", "\n.", "\n.\n",
		".\r", "\r.", "\r.\r",
		xs + "\r\n.\r\n",
		xs + "1234\r\n.\r\n",
		xs + xs + "\r\n.\r\n",
		xs + xs + xs + "\r\n.\r\n",
		xs + "." + xs + "\n.",
		xs + ".\n" + xs + "\n.",
	}
	for _, s := range final {
		t.Logf("testing %q", s)
		buf := bufio.NewReaderSize(strings.NewReader(s), size)
		readUntilDot(buf)
		if r := buf.Buffered(); r != 0 {
			t.Errorf("%q: there are %d remaining bytes", s, r)
		}
	}

	nonfinal := []struct {
		s string
		r int
	}{
		{".\na", 1},
		{"\n.\na", 1},
		{"\n.\nabc", 3},
		{"\n.\n12345678", 8},
		{"\n.\n" + xs, size - 3},
		{"\n.\n" + xs + xs, size - 3},
		{"\n.\n.\n", 2},
	}
	for _, c := range nonfinal {
		t.Logf("testing %q", c.s)
		buf := bufio.NewReaderSize(strings.NewReader(c.s), size)
		readUntilDot(buf)
		if r := buf.Buffered(); r != c.r {
			t.Errorf("%q: expected %d remaining bytes, got %d", c.s, c.r, r)
		}
	}
}

func TestAddrLiteral(t *testing.T) {
	// TCP addresses.
	casesTCP := []struct {
		addr     net.IP
		expected string
	}{
		{net.IPv4(1, 2, 3, 4), "1.2.3.4"},
		{net.IPv4(0, 0, 0, 0), "0.0.0.0"},
		{net.ParseIP("1.2.3.4"), "1.2.3.4"},
		{net.ParseIP("2001:db8::68"), "IPv6:2001:db8::68"},
		{net.ParseIP("::1"), "IPv6:::1"},
	}
	for _, c := range casesTCP {
		tcp := &net.TCPAddr{
			IP:   c.addr,
			Port: 12345,
		}
		s := addrLiteral(tcp)
		if s != c.expected {
			t.Errorf("%v: expected %q, got %q", tcp, c.expected, s)
		}
	}

	// Non-TCP addresses. We expect these to match addr.String().
	casesOther := []net.Addr{
		&net.UDPAddr{
			IP:   net.ParseIP("1.2.3.4"),
			Port: 12345,
		},
	}
	for _, addr := range casesOther {
		s := addrLiteral(addr)
		if s != addr.String() {
			t.Errorf("%v: expected %q, got %q", addr, addr.String(), s)
		}
	}
}
