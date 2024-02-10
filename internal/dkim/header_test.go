package dkim

import (
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSignatureFromHeader(t *testing.T) {
	cases := []struct {
		in   string
		want *dkimSignature
		err  error
	}{
		{
			in:   "v=1; a=rsa-sha256",
			want: nil,
			err:  errMissingRequiredTag,
		},
		{
			in: "v=1; a=rsa-sha256 ; c = simple/relaxed ;" +
				" d=example.com; h= from : to: subject ; " +
				"i=agent@example.com; l=77; q=dns/txt; " +
				"s=selector; t=1600700888; x=1600700999; " +
				"z=From:lala@lele | to:lili@lolo;" +
				"b=aG9sY\r\n SBxdWUgdGFs;" +
				"bh = Y29\ttby Bhbm Rhcw==",
			want: &dkimSignature{
				v:  "1",
				a:  "rsa-sha256",
				cH: simpleCanonicalization,
				cB: relaxedCanonicalization,
				d:  "example.com",
				h:  []string{"from", "to", "subject"},
				i:  "agent@example.com",
				l:  77,
				q:  []string{"dns/txt"},
				s:  "selector",
				t:  time.Unix(1600700888, 0),
				x:  time.Unix(1600700999, 0),
				z:  "From:lala@lele|to:lili@lolo",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),

				KeyType: keyTypeRSA,
				Hash:    crypto.SHA256,
			},
		},
		{
			// Example from RFC.
			// https://datatracker.ietf.org/doc/html/rfc6376#section-3.5
			in: "v=1; a=rsa-sha256; d=example.net; s=brisbane;\r\n" +
				" c=simple; q=dns/txt; i=@eng.example.net;\r\n" +
				" t=1117574938; x=1118006938;\r\n" +
				" h=from:to:subject:date;\r\n" +
				" z=From:foo@eng.example.net|To:joe@example.com|\r\n" +
				"  Subject:demo=20run|Date:July=205,=202005=203:44:08=20PM=20-0700;\r\n" +
				"bh=MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=;\r\n" +
				"b=dzdVyOfAKCdLXdJOc9G2q8LoXSlEniS" +
				"bav+yuU4zGeeruD00lszZVoG4ZHRNiYzR",
			want: &dkimSignature{
				v:  "1",
				a:  "rsa-sha256",
				d:  "example.net",
				s:  "brisbane",
				cH: simpleCanonicalization,
				cB: simpleCanonicalization,
				q:  []string{"dns/txt"},
				i:  "@eng.example.net",
				t:  time.Unix(1117574938, 0),
				x:  time.Unix(1118006938, 0),
				h:  []string{"from", "to", "subject", "date"},
				z: "From:foo@eng.example.net|To:joe@example.com|" +
					"Subject:demo=20run|" +
					"Date:July=205,=202005=203:44:08=20PM=20-0700",
				bh: []byte("12345678901234567890123456789012"),
				b: []byte("w7U\xc8\xe7\xc0('K]\xd2Ns\xd1\xb6" +
					"\xab\xc2\xe8])D\x9e$\x9bj\xff\xb2\xb9N3" +
					"\x19\xe7\xab\xb8=4\x96\xcc\xd9V\x81\xb8" +
					"dtM\x89\x8c\xd1"),
				KeyType: keyTypeRSA,
				Hash:    crypto.SHA256,
			},
		},
		{
			in:   "",
			want: nil,
			err:  errInvalidTag,
		},
		{
			in:   "v=666",
			want: nil,
			err:  errInvalidVersion,
		},
		{
			in:   "v=1; a=something;",
			want: nil,
			err:  errBadATag,
		},
		{
			// Invalid b= tag.
			in:   "v=1; a=rsa-sha256; b=invalid",
			want: nil,
			err:  base64.CorruptInputError(4),
		},
		{
			// Invalid bh= tag.
			in:   "v=1; a=rsa-sha256; bh=invalid",
			want: nil,
			err:  base64.CorruptInputError(4),
		},
		{
			// Invalid c= tag.
			in:   "v=1; a=rsa-sha256; c=caca",
			want: nil,
			err:  errUnknownCanonicalization,
		},
		{
			// Invalid l= tag.
			in:   "v=1; a=rsa-sha256; l=a1234b",
			want: nil,
			err:  strconv.ErrSyntax,
		},
		{
			// q= tag without dns/txt.
			in:   "v=1; a=rsa-sha256; q=other/method",
			want: nil,
			err:  errInvalidSignature,
		},
		{
			// Invalid t= tag.
			in:   "v=1; a=rsa-sha256; t=a1234b",
			want: nil,
			err:  strconv.ErrSyntax,
		},
		{
			// Invalid x= tag.
			in:   "v=1; a=rsa-sha256; x=a1234b",
			want: nil,
			err:  strconv.ErrSyntax,
		},
		{
			// Unknown hash algorithm.
			in:   "v=1; a=rsa-sxa666",
			want: nil,
			err:  errUnsupportedHash,
		},
		{
			// Unknown key type.
			in:   "v=1; a=rxa-sha256",
			want: nil,
			err:  errUnsupportedKeyType,
		},
	}

	for _, c := range cases {
		sig, err := dkimSignatureFromHeader(c.in)
		diff := cmp.Diff(c.want, sig,
			cmp.AllowUnexported(dkimSignature{}),
			cmpopts.EquateEmpty(),
		)
		if diff != "" {
			t.Errorf("dkimSignatureFromHeader(%q) mismatch (-want +got):\n%s",
				c.in, diff)
		}
		if !errors.Is(err, c.err) {
			t.Errorf("dkimSignatureFromHeader(%q) error: got %v, want %v",
				c.in, err, c.err)
		}
	}
}

func TestCanonicalizationFromString(t *testing.T) {
	cases := []struct {
		in     string
		cH, cB canonicalization
		err    error
	}{
		{
			in: "",
			cH: simpleCanonicalization,
			cB: simpleCanonicalization,
		},
		{
			in: "simple",
			cH: simpleCanonicalization,
			cB: simpleCanonicalization,
		},
		{
			in: "relaxed",
			cH: relaxedCanonicalization,
			cB: simpleCanonicalization,
		},
		{
			in: "simple/simple",
			cH: simpleCanonicalization,
			cB: simpleCanonicalization,
		},
		{
			in: "relaxed/relaxed",
			cH: relaxedCanonicalization,
			cB: relaxedCanonicalization,
		},
		{
			in: "simple/relaxed",
			cH: simpleCanonicalization,
			cB: relaxedCanonicalization,
		},
		{
			in:  "relaxed/bad",
			cH:  relaxedCanonicalization,
			err: errUnknownCanonicalization,
		},
		{
			in:  "bad/relaxed",
			err: errUnknownCanonicalization,
		},
		{
			in:  "bad",
			err: errUnknownCanonicalization,
		},
	}

	for _, c := range cases {
		sig := &dkimSignature{}
		err := sig.canonicalizationFromString(c.in)
		if sig.cH != c.cH || sig.cB != c.cB || !errors.Is(err, c.err) {
			t.Errorf("canonicalizationFromString(%q) "+
				"got (%v, %v, %v), want (%v, %v, %v)",
				c.in, sig.cH, sig.cB, err, c.cH, c.cB, c.err)
		}
	}
}

func TestCheckRequiredTags(t *testing.T) {
	cases := []struct {
		sig *dkimSignature
		err string
	}{
		{
			sig: &dkimSignature{},
			err: "missing required tag: a=",
		},
		{
			sig: &dkimSignature{a: "rsa-sha256"},
			err: "missing required tag: b=",
		},
		{
			sig: &dkimSignature{a: "rsa-sha256", b: []byte("hola que tal")},
			err: "missing required tag: bh=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
			},
			err: "missing required tag: d=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
			},
			err: "missing required tag: h=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"from"},
			},
			err: "missing required tag: s=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"subject"},
				s:  "selector",
			},
			err: "invalid tag: h= does not contain 'from'",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"from"},
				s:  "selector",
				i:  "@example.net",
			},
			err: "invalid tag: i= is not a subdomain of d=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"from"},
				s:  "selector",
				i:  "@anexample.com", // i= is a substring but not subdomain.
			},
			err: "invalid tag: i= is not a subdomain of d=",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"From"}, // Capitalize to check case fold.
				s:  "selector",
				i:  "@example.com", // i= is the same as d=
			},
			err: "<nil>",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"From"},
				s:  "selector",
				i:  "@sub.example.com", // i= is a subdomain of d=
			},
			err: "<nil>",
		},
		{
			sig: &dkimSignature{
				a:  "rsa-sha256",
				b:  []byte("hola que tal"),
				bh: []byte("como andas"),
				d:  "example.com",
				h:  []string{"from"},
				s:  "selector",
			},
			err: "<nil>",
		},
	}

	for i, c := range cases {
		err := c.sig.checkRequiredTags()
		got := fmt.Sprintf("%v", err)
		if c.err != got {
			t.Errorf("%d: checkRequiredTags() got %v, want %v",
				i, err, c.err)
		}
	}
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want tags
		err  error
	}{
		{
			in: "v=1; a=lalala; b = 123  ; c= 456;\t d \t= \t789\t ",
			want: tags{
				"v": "1",
				"a": "lalala",
				"b": "123",
				"c": "456",
				"d": "789",
			},
			err: nil,
		},
		{
			// Trailing semicolon.
			in: "v=1; a=lalala ; ",
			want: tags{
				"v": "1",
				"a": "lalala",
			},
			err: nil,
		},
		{
			// Missing tag value; this is okay.
			in: "v=1; b = ; c = d;",
			want: tags{
				"v": "1",
				"b": "",
				"c": "d",
			},
			err: nil,
		},
		{
			// Missing '='.
			in:   "v=1;   ; c = d;",
			want: nil,
			err:  errInvalidTag,
		},
		{
			// Missing tag name.
			in:   "v=1; = b ; c = d;",
			want: nil,
			err:  errInvalidTag,
		},
		{
			// Duplicate tag.
			in:   "v=1; a=b; a=c;",
			want: nil,
			err:  errInvalidTag,
		},
	}

	for _, c := range cases {
		got, err := parseTags(c.in)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("parseTags(%q) mismatch (-want +got):\n%s", c.in, diff)
		}
		if !errors.Is(err, c.err) {
			t.Errorf("parseTags(%q) error: got %v, want %v", c.in, err, c.err)
		}
	}
}
