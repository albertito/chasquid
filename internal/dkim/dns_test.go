package dkim

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestLookupError(t *testing.T) {
	testErr := errors.New("lookup error")
	errLookupF := func(ctx context.Context, name string) ([]string, error) {
		return nil, testErr
	}
	ctx := WithLookupTXTFunc(context.Background(), errLookupF)

	pks, err := findPublicKeys(ctx, "example.com", "selector")
	if pks != nil || err != testErr {
		t.Errorf("findPublicKeys expected nil / lookup error, got %v / %v",
			pks, err)
	}
}

// RSA key from the RFC example.
// https://datatracker.ietf.org/doc/html/rfc6376#appendix-C
const exampleRSAKeyB64 = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQ" +
	"KBgQDwIRP/UC3SBsEmGqZ9ZJW3/DkMoGeLnQg1fWn7/zYt" +
	"IxN2SnFCjxOCKG9v3b4jYfcTNh5ijSsq631uBItLa7od+v" +
	"/RtdC2UzJ1lWT947qR+Rcac2gbto/NMqJ0fzfVjH4OuKhi" +
	"tdY9tf6mcwGjaNBcWToIMmPSPDdQPNUYckcQ2QIDAQAB"

var exampleRSAKeyBuf, _ = base64.StdEncoding.DecodeString(exampleRSAKeyB64)
var exampleRSAKey, _ = x509.ParsePKCS1PublicKey(exampleRSAKeyBuf)

// Ed25519 key from the RFC example.
// https://datatracker.ietf.org/doc/html/rfc8463#appendix-A.2
const exampleEd25519KeyB64 = "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="

var exampleEd25519KeyBuf, _ = base64.StdEncoding.DecodeString(
	exampleEd25519KeyB64)
var exampleEd25519Key = ed25519.PublicKey(exampleEd25519KeyBuf)

var results = map[string][]string{}
var resultErr = map[string]error{}

func testLookupTXT(ctx context.Context, name string) ([]string, error) {
	return results[name], resultErr[name]
}

func TestSkipBadRecords(t *testing.T) {
	ctx := WithLookupTXTFunc(context.Background(), testLookupTXT)
	results["selector._domainkey.example.com"] = []string{
		"not a tag",
		"v=DKIM1; p=" + exampleRSAKeyB64,
	}
	defer clear(results)

	pks, err := findPublicKeys(ctx, "example.com", "selector")
	if err != nil {
		t.Errorf("findPublicKeys expected nil, got %v", err)
	}
	if len(pks) != 1 {
		t.Errorf("findPublicKeys expected 1 key, got %v", len(pks))
	}
}

func TestParsePublicKey(t *testing.T) {
	cases := []struct {
		in  string
		pk  *publicKey
		err error
	}{
		// Invalid records.
		{"not a tag", nil, errInvalidTag},
		{"v=DKIM666;", nil, errInvalidVersion},
		{"p=abc~*#def", nil, base64.CorruptInputError(3)},
		{"k=blah; p=" + exampleRSAKeyB64, nil, errUnsupportedKeyType},

		// Error parsing the keys.
		{"p=", nil, errInvalidRSAPublicKey},

		// RSA key but the contents are a (valid) ECDSA key.
		{"p=" +
			"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIT0qsh+0jdY" +
			"DhK5+rSedhT7W/5rTRiulhphqtuplGFAyNiSh9I5t6MsrIu" +
			"xFQV7A/cWAt8qcbVscT3Q2l6iu3w==",
			nil, errNotRSAPublicKey},

		// Valid RSA key, that is too short.
		{"p=" +
			"MEgCQQCo9+BpMRYQ/dL3DS2CyJxRF+j6ctbT3/Qp84+KeFh" +
			"nii7NT7fELilKUSnxS30WAvQCCo2yU1orfgqr41mM70MBAg" +
			"MBAAE=", nil, errRSAKeyTooSmall},

		// Invalid ed25519 key.
		{"k=ed25519; p=MFkwEwYH", nil, errInvalidEd25519Key},

		// Valid.
		{"p=" + exampleRSAKeyB64,
			&publicKey{K: keyTypeRSA, P: exampleRSAKeyBuf}, nil},
		{"k=rsa ; p=" + exampleRSAKeyB64,
			&publicKey{K: keyTypeRSA, P: exampleRSAKeyBuf}, nil},
		{
			"k=rsa; h=sha256; p=" + exampleRSAKeyB64,
			&publicKey{
				K: keyTypeRSA,
				H: []crypto.Hash{crypto.SHA256},
				P: exampleRSAKeyBuf},
			nil,
		},
		{"t=s; p=" + exampleRSAKeyB64,
			&publicKey{
				K: keyTypeRSA,
				P: exampleRSAKeyBuf,
				T: []string{"s"},
			},
			nil,
		},
		{"t = s : y; p=" + exampleRSAKeyB64,
			&publicKey{
				K: keyTypeRSA,
				P: exampleRSAKeyBuf,
				T: []string{"s", "y"},
			},
			nil,
		},
		{
			// We should ignore unrecognized hash algorithms.
			"k=rsa; h=sha1:xxx123:sha256; p=" + exampleRSAKeyB64,
			&publicKey{
				K: keyTypeRSA,
				H: []crypto.Hash{crypto.SHA256},
				P: exampleRSAKeyBuf},
			nil,
		},
		{"k=ed25519; p=" + exampleEd25519KeyB64,
			&publicKey{K: keyTypeEd25519, P: exampleEd25519KeyBuf}, nil},
	}

	for i, c := range cases {
		pk, err := parsePublicKey(c.in)
		diff := cmp.Diff(c.pk, pk,
			cmpopts.IgnoreUnexported(publicKey{}),
			cmpopts.EquateEmpty(),
		)
		if diff != "" {
			t.Errorf("%d: parsePublicKey(%q) key: (-want +got)\n%s",
				i, c.in, diff)
		}
		if !errors.Is(err, c.err) {
			t.Errorf("%d: parsePublicKey(%q) error: want %v, got %v",
				i, c.in, c.err, err)
		}
	}
}

func TestPublicKeyMatches(t *testing.T) {
	cases := []struct {
		pk *publicKey
		kt keyType
		h  crypto.Hash
		ok bool
	}{
		{
			&publicKey{K: keyTypeRSA},
			keyTypeRSA, crypto.SHA256,
			true,
		},
		{
			&publicKey{K: keyTypeRSA, H: []crypto.Hash{crypto.SHA1}},
			keyTypeRSA, crypto.SHA1,
			true,
		},
		{
			&publicKey{K: keyTypeRSA, H: []crypto.Hash{crypto.SHA1}},
			keyTypeRSA, crypto.SHA256,
			false,
		},
		{
			&publicKey{K: keyTypeRSA, H: []crypto.Hash{crypto.SHA1}},
			keyTypeEd25519, crypto.SHA1,
			false,
		},
	}

	for i, c := range cases {
		if ok := c.pk.Matches(c.kt, c.h); ok != c.ok {
			t.Errorf("%d: matches(%v, %v) = %v, want %v",
				i, c.kt, c.h, ok, c.ok)
		}
	}
}

func TestStrictDomainCheck(t *testing.T) {
	cases := []struct {
		t  string
		ok bool
	}{
		{"", false},
		{"y", false},
		{"x:y", false},
		{":x::y", false},
		{"s", true},
		{"y:s", true},
		{" y: s", true},
		{"y:s:x", true},
	}

	for i, c := range cases {
		pkS := "k=ed25519; p=" + exampleEd25519KeyB64 + "; t=" + c.t
		pk, err := parsePublicKey(pkS)
		if err != nil {
			t.Fatalf("%d: parsePublicKey(%q) = %v", i, pkS, err)
		}
		if ok := pk.StrictDomainCheck(); ok != c.ok {
			t.Errorf("%d: strictDomainCheck(t=%q) = %v, want %v",
				i, c.t, ok, c.ok)
		}
	}
}

func FuzzParsePublicKey(f *testing.F) {
	// Add some initial corpus from the tests above.
	f.Add("not a tag")
	f.Add("v=DKIM666;")
	f.Add("p=abc~*#def")
	f.Add("k=blah; p=" + exampleRSAKeyB64)
	f.Add("p=")
	f.Add("k=ed25519; p=")
	f.Add("k=ed25519; p=MFkwEwYH")
	f.Add("p=" + exampleEd25519KeyB64)
	f.Add("k=rsa ; p=" + exampleRSAKeyB64)
	f.Add("v=DKIM1; p=" + exampleRSAKeyB64)
	f.Add("t=s; p=" + exampleRSAKeyB64)
	f.Add("t = s : y; p=" + exampleRSAKeyB64)
	f.Add("k=rsa; h=sha256; p=" + exampleRSAKeyB64)
	f.Add("k=rsa; h=sha1:xxx123:sha256; p=" + exampleRSAKeyB64)

	f.Fuzz(func(t *testing.T, in string) {
		parsePublicKey(in)
	})
}
