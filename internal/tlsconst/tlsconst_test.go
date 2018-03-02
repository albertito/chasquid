package tlsconst

import "testing"

func TestVersionName(t *testing.T) {
	cases := []struct {
		ver      uint16
		expected string
	}{
		{0x0302, "TLS-1.1"},
		{0x1234, "TLS-0x1234"},
	}
	for _, c := range cases {
		got := VersionName(c.ver)
		if got != c.expected {
			t.Errorf("VersionName(%x) = %q, expected %q",
				c.ver, got, c.expected)
		}
	}
}

func TestCipherSuiteName(t *testing.T) {
	cases := []struct {
		suite    uint16
		expected string
	}{
		{0xc073, "TLS_ECDHE_ECDSA_WITH_CAMELLIA_256_CBC_SHA384"},
		{0x1234, "TLS_UNKNOWN_CIPHER_SUITE-0x1234"},
	}
	for _, c := range cases {
		got := CipherSuiteName(c.suite)
		if got != c.expected {
			t.Errorf("CipherSuiteName(%x) = %q, expected %q",
				c.suite, got, c.expected)
		}
	}
}
