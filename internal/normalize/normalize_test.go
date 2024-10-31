package normalize

import (
	"bytes"
	"strings"
	"testing"
)

func TestUser(t *testing.T) {
	valid := []struct{ user, norm string }{
		{"ÑAndÚ", "ñandú"},
		{"Pingüino", "pingüino"},
	}
	for _, c := range valid {
		nu, err := User(c.user)
		if nu != c.norm {
			t.Errorf("%q normalized to %q, expected %q", c.user, nu, c.norm)
		}
		if err != nil {
			t.Errorf("%q error: %v", c.user, err)
		}

	}

	invalid := []string{
		"á é", "a\te", "x ", "x\xa0y", "x\x85y", "x\vy", "x\fy", "x\ry",
		"henry\u2163", "\u265a", "\u00b9",
	}
	for _, u := range invalid {
		nu, err := User(u)
		if err == nil {
			t.Errorf("expected User(%+q) to fail, but did not", u)
		}
		if nu != u {
			t.Errorf("%+q failed norm, but returned %+q", u, nu)
		}
	}
}

func TestDomain(t *testing.T) {
	valid := []struct{ user, norm string }{
		{"ÑAndÚ", "ñandú"},
		{"Pingüino", "pingüino"},
		{"xn--aca-6ma", "ñaca"},
		{"xn--lca", "ñ"}, // Punycode is for 'Ñ'.
		{"e\u0301", "é"}, // Transform to NFC form.
	}
	for _, c := range valid {
		nu, err := Domain(c.user)
		if nu != c.norm {
			t.Errorf("%q normalized to %q, expected %q", c.user, nu, c.norm)
		}
		if err != nil {
			t.Errorf("%q error: %v", c.user, err)
		}

	}

	invalid := []string{"xn---", "xn--xyz-ñ"}
	for _, u := range invalid {
		nu, err := Domain(u)
		if err == nil {
			t.Errorf("expected Domain(%+q) to fail, but did not", u)
		}
		if nu != u {
			t.Errorf("%+q failed norm, but returned %+q", u, nu)
		}
	}
}

func TestAddr(t *testing.T) {
	valid := []struct{ user, norm string }{
		{"ÑAndÚ@pampa", "ñandú@pampa"},
		{"Pingüino@patagonia", "pingüino@patagonia"},
		{"pe\u0301@le\u0301a", "pé@léa"}, // Transform to NFC form.
	}
	for _, c := range valid {
		nu, err := Addr(c.user)
		if nu != c.norm {
			t.Errorf("%q normalized to %q, expected %q", c.user, nu, c.norm)
		}
		if err != nil {
			t.Errorf("%q error: %v", c.user, err)
		}
	}

	invalid := []string{
		"á é@i", "henry\u2163@throne", "a@xn---",
	}
	for _, u := range invalid {
		nu, err := Addr(u)
		if err == nil {
			t.Errorf("expected Addr(%+q) to fail, but did not", u)
		}
		if nu != u {
			t.Errorf("%+q failed norm, but returned %+q", u, nu)
		}
	}
}

func TestDomainToUnicode(t *testing.T) {
	valid := []struct{ domain, expected string }{
		{"<>", "<>"},
		{"a@b", "a@b"},
		{"a@Ñ", "a@ñ"},
		{"xn--lca@xn--lca", "xn--lca@ñ"}, // Punycode is for 'Ñ'.
		{"a@e\u0301", "a@é"},             // Transform to NFC form.

		// Degenerate case, we don't expect to ever produce this; at least
		// check it does not crash.
		{"", "@"},
	}
	for _, c := range valid {
		got, err := DomainToUnicode(c.domain)
		if got != c.expected {
			t.Errorf("DomainToUnicode(%q) = %q, expected %q",
				c.domain, got, c.expected)
		}
		if err != nil {
			t.Errorf("DomainToUnicode(%q) error: %v", c.domain, err)
		}
	}

	invalid := []string{"a@xn---", "a@xn--xyz-ñ"}
	for _, u := range invalid {
		got, err := DomainToUnicode(u)
		if err == nil {
			t.Errorf("expected DomainToUnicode(%+q) to fail, but did not", u)
		}
		if got != u {
			t.Errorf("%+q failed norm, but returned %+q", u, got)
		}
	}
}

func TestToCRLF(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"a", "a"},

		// Does not end in newline.
		{"a\n", "a\r\n"},
		{"a\nb", "a\r\nb"},
		{"a\r\nb", "a\r\nb"},

		// Ends in newline.
		{"a\nb\n", "a\r\nb\r\n"},
		{"a\r\nb\n", "a\r\nb\r\n"},
		{"a\r\nb\r\n", "a\r\nb\r\n"},
		{"a\r\nb\n\n", "a\r\nb\r\n\r\n"},
		{"a\r\nb\r\n\r\n", "a\r\nb\r\n\r\n"},
	}
	for _, c := range cases {
		got := string(ToCRLF([]byte(c.in)))
		if got != c.out {
			t.Errorf("ToCRLF(%q) = %q, expected %q", c.in, got, c.out)
		}

		got = StringToCRLF(c.in)
		if got != c.out {
			t.Errorf("StringToCRLF(%q) = %q, expected %q", c.in, got, c.out)
		}
	}
}

func FuzzUser(f *testing.F) {
	f.Fuzz(func(t *testing.T, user string) {
		User(user)
	})
}

func FuzzDomain(f *testing.F) {
	f.Fuzz(func(t *testing.T, domain string) {
		Domain(domain)
	})
}

func FuzzAddr(f *testing.F) {
	f.Fuzz(func(t *testing.T, addr string) {
		Addr(addr)
	})
}

func FuzzDomainToUnicode(f *testing.F) {
	f.Fuzz(func(t *testing.T, addr string) {
		DomainToUnicode(addr)
	})
}

func BenchmarkToCRLF(b *testing.B) {
	// Generate a 1000-line message.
	bb := bytes.Buffer{}
	for i := 0; i < 1000; i++ {
		bb.WriteString("this is a very pretty line 🐅\n")
	}
	buf := bb.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ToCRLF(buf)
	}
}

func BenchmarkStringToCRLF(b *testing.B) {
	// Generate a 1000-line message.
	sb := strings.Builder{}
	for i := 0; i < 1000; i++ {
		sb.WriteString("this is a very pretty line 🐅\n")
	}
	s := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StringToCRLF(s)
	}
}
