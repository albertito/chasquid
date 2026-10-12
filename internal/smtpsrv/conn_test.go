package smtpsrv

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/trace"
	"blitiri.com.ar/go/spf"
)

func TestSecLevel(t *testing.T) {
	// We can't simulate this externally because of the SPF record
	// requirement, so do a narrow test on Conn.secLevelCheck.
	dinfo, err := domaininfo.New(t.ArtifactDir())
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

func TestReceivedHeaderEnvelopeFrom(t *testing.T) {
	c := &Conn{
		hostname:   "mx.example.com",
		ehloDomain: "client.example.com",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234},
		tr:         trace.New("test", "TestReceivedHeaderEnvelopeFrom"),
		// The local part could have been quoted, see isSafeAddress.
		mailFrom: `x)(y; z\@example.org`,
		data:     []byte("Subject: test\n\nbody\n"),
	}
	defer c.tr.Finish()
	c.addReceivedHeader()

	hdr, _, _ := strings.Cut(string(c.data), "Subject:")
	want := `envelope from "x\)\(y; z\\@example.org")`
	if !strings.Contains(hdr, want) {
		t.Errorf("Received header does not contain %q:\n%s", want, hdr)
	}

	// Parentheses must be balanced, considering the escaped ones.
	depth := 0
	for i := 0; i < len(hdr); i++ {
		switch hdr[i] {
		case '\\':
			i++ // Skip the escaped character.
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				t.Fatalf("unbalanced ')' at %d:\n%s", i, hdr)
			}
		}
	}
	if depth != 0 {
		t.Errorf("unbalanced comments (depth %d):\n%s", depth, hdr)
	}
}

func TestValidServerName(t *testing.T) {
	// Self-signed certificate valid for mx.example.com and *.example.net.
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"mx.example.com", "*.example.net"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(nil, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
		Leaf:        leaf,
	}

	cases := []struct {
		sni  string
		want string
	}{
		{"", ""},
		{"mx.example.com", "mx.example.com"},
		{"MX.Example.COM", "MX.Example.COM"},
		{"a.example.net", "a.example.net"},
		{"a.b.example.net", ""},
		{"other.example.com", ""},
		{"mx.example.com; dkim=pass header.d=example.org", ""},
		{"mx.example.com\r\nX-Injected: yes", ""},
		{"a;b.example.net", ""},
	}
	for _, c := range cases {
		conn := &Conn{
			tlsConfig:    &tls.Config{Certificates: []tls.Certificate{cert}},
			tlsConnState: &tls.ConnectionState{ServerName: c.sni},
			tr:           trace.New("test", "TestValidServerName"),
		}
		if got := conn.validServerName(); got != c.want {
			t.Errorf("SNI %q: got %q, want %q", c.sni, got, c.want)
		}
		conn.tr.Finish()
	}

	// Without the parsed leaf, we can't check, so the SNI is not used.
	cert.Leaf = nil
	conn := &Conn{
		tlsConfig:    &tls.Config{Certificates: []tls.Certificate{cert}},
		tlsConnState: &tls.ConnectionState{ServerName: "mx.example.com"},
		tr:           trace.New("test", "TestValidServerName"),
	}
	if got := conn.validServerName(); got != "" {
		t.Errorf("without leaf: got %q, want \"\"", got)
	}
	conn.tr.Finish()
}

func TestSanitizeEHLODomain(t *testing.T) {
	equal := []string{
		"domain", "do.main", "do-main",
		"1.2.3.4", "a:b:c", "[a:b:c]",
		"abz", "AbZ",
	}
	for _, str := range equal {
		if got := sanitizeEHLODomain(str); got != str {
			t.Errorf("sanitizeEHLODomain(%q) returned %q, expected %q",
				str, got, str)
		}
	}

	invalid := []struct {
		str      string
		expected string
	}{
		{"ñaca", "aca"}, {"a\nb", "ab"}, {"a\x00b", "ab"}, {"a\x7fb", "ab"},
		{"a/z", "az"}, {"a;b", "ab"}, {"a$b", "ab"}, {"a^b", "ab"},
		{"a b", "ab"}, {"a+b", "ab"}, {"a@b", "ab"}, {`a"b`, "ab"},
		{`a\b`, "ab"},
	}
	for _, c := range invalid {
		if got := sanitizeEHLODomain(c.str); got != c.expected {
			t.Errorf("sanitizeEHLODomain(%q) returned %q, expected %q",
				c.str, got, c.expected)
		}
	}
}

func TestEHLODomainIsSanitized(t *testing.T) {
	cases := []struct {
		params   string
		expected string
	}{
		{"mx.example.com", "mx.example.com"},
		{"[1.2.3.4] extra params", "[1.2.3.4]"},
		{"evil;(x", "evilx"},
		{"!!!", invalidEHLODomain},
	}
	for _, c := range cases {
		for _, cmd := range []string{"HELO", "EHLO"} {
			conn := &Conn{}
			if cmd == "HELO" {
				conn.HELO(c.params)
			} else {
				conn.EHLO(c.params)
			}
			if conn.ehloDomain != c.expected {
				t.Errorf("%s %q: got EHLO domain %q, expected %q",
					cmd, c.params, conn.ehloDomain, c.expected)
			}
		}
	}
}

func TestIsSafeAddress(t *testing.T) {
	safe := []string{
		"john@example.com", "ñandú@example.com", "用户@例子.广告",
		// Characters that need quoting are fine, we're lenient with them.
		"a;b@example.com", "a(b)@example.com", `a"b@example.com`,
	}
	for _, addr := range safe {
		if !isSafeAddress(addr) {
			t.Errorf("%q: expected safe, got unsafe", addr)
		}
	}

	unsafe := []string{
		// More than one "@" (or none).
		"a@b@example.com", "@a@example.com", "example.com",
		// Control characters (C1, U+009B is CSI).
		"a\u0081b@example.com", "a\u009b31mb@example.com",
		"a@exam\u0085ple.com",
	}
	for _, addr := range unsafe {
		if isSafeAddress(addr) {
			t.Errorf("%q: expected unsafe, got safe", addr)
		}
	}
}
