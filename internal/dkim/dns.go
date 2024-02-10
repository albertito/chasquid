package dkim

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func findPublicKeys(ctx context.Context, domain, selector string) ([]*publicKey, error) {
	// Subdomain where the key lives.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.6.2
	d := selector + "._domainkey." + domain
	values, err := lookupTXT(ctx, d)
	if err != nil {
		trace(ctx, "TXT lookup of %q failed: %v", d, err)
		return nil, err
	}

	// There should be only a single record; RFC 6376 says the results are
	// undefined if there are multiple TXT records.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.6.2.2
	//
	// What other implementations do:
	//  - dkimpy: Use the first TXT record (whatever it is).
	//  - OpenDKIM: Use the first TXT record (whatever it is).
	//  - driusan/dkim: Use the first TXT record that can be parsed as a key.
	//  - go-msgauth: Reject if there are multiple records.
	//
	// What we do: use _all_ TXT records that can be parsed as keys. This is
	// possibly too much, and we could reconsider this in the future.

	pks := []*publicKey{}
	for _, v := range values {
		trace(ctx, "TXT record for %q: %q", d, v)
		pk, err := parsePublicKey(v)
		if err != nil {
			trace(ctx, "Skipping: %v", err)
			continue
		}
		trace(ctx, "Parsed public key: %s", pk)
		pks = append(pks, pk)
	}

	return pks, nil
}

// Function to verify a signature with this public key.
type verifyFunc func(h crypto.Hash, hashed, signature []byte) error

type publicKey struct {
	H []crypto.Hash
	K keyType
	P []byte

	T []string // t= tag, representing flags.

	verify verifyFunc
}

func (pk *publicKey) String() string {
	return fmt.Sprintf("[%s:%.8x]", pk.K, pk.P)
}

func (pk *publicKey) Matches(kt keyType, h crypto.Hash) bool {
	if pk.K != kt {
		return false
	}
	if len(pk.H) > 0 {
		return slices.Contains(pk.H, h)
	}
	return true
}

func (pk *publicKey) StrictDomainCheck() bool {
	// t=s is set.
	return slices.Contains(pk.T, "s")
}

func parsePublicKey(v string) (*publicKey, error) {
	// Public key is a tag-value list.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.6.1
	tags, err := parseTags(v)
	if err != nil {
		return nil, err
	}

	// "v" is optional, but if present it must be "DKIM1".
	ver, ok := tags["v"]
	if ok && ver != "DKIM1" {
		return nil, fmt.Errorf("%w: %q", errInvalidVersion, ver)
	}

	pk := &publicKey{
		// The default key type is rsa.
		K: keyTypeRSA,
	}

	// h is a colon-separated list of hashing algorithm names.
	if tags["h"] != "" {
		hs := strings.Split(eatWhitespace.Replace(tags["h"]), ":")
		for _, h := range hs {
			x, err := hashFromString(h)
			if err != nil {
				// Unrecognized algorithms must be ignored.
				// https://datatracker.ietf.org/doc/html/rfc6376#section-3.6.1
				continue
			}
			pk.H = append(pk.H, x)
		}
	}

	// k is key type (may not be present, rsa is used in that case).
	if tags["k"] != "" {
		pk.K, err = keyTypeFromString(tags["k"])
		if err != nil {
			return nil, err
		}
	}

	// p is public-key data, base64-encoded, and whitespace in it must be
	// ignored. Required.
	p, err := base64.StdEncoding.DecodeString(
		eatWhitespace.Replace(tags["p"]))
	if err != nil {
		return nil, fmt.Errorf("error decoding p=: %w", err)
	}
	pk.P = p

	switch pk.K {
	case keyTypeRSA:
		pk.verify, err = parseRSAPublicKey(p)
	case keyTypeEd25519:
		pk.verify, err = parseEd25519PublicKey(p)
	}

	// t is a colon-separated list of flags.
	if t := eatWhitespace.Replace(tags["t"]); t != "" {
		pk.T = strings.Split(t, ":")
	}

	if err != nil {
		return nil, err
	}
	return pk, nil
}

var (
	errInvalidRSAPublicKey = errors.New("invalid RSA public key")
	errNotRSAPublicKey     = errors.New("not an RSA public key")
	errRSAKeyTooSmall      = errors.New("RSA public key too small")
	errInvalidEd25519Key   = errors.New("invalid Ed25519 public key")
)

func parseRSAPublicKey(p []byte) (verifyFunc, error) {
	// Either PKCS#1 or SubjectPublicKeyInfo.
	// See https://www.rfc-editor.org/errata/eid3017.
	pub, err := x509.ParsePKIXPublicKey(p)
	if err != nil {
		pub, err = x509.ParsePKCS1PublicKey(p)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRSAPublicKey, err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errNotRSAPublicKey
	}

	// Enforce 1024-bit minimum.
	// https://datatracker.ietf.org/doc/html/rfc8301#section-3.2
	if rsaPub.Size()*8 < 1024 {
		return nil, errRSAKeyTooSmall
	}

	return func(h crypto.Hash, hashed, signature []byte) error {
		return rsa.VerifyPKCS1v15(rsaPub, h, hashed, signature)
	}, nil
}

func parseEd25519PublicKey(p []byte) (verifyFunc, error) {
	// https: //datatracker.ietf.org/doc/html/rfc8463
	if len(p) != ed25519.PublicKeySize {
		return nil, errInvalidEd25519Key
	}

	pub := ed25519.PublicKey(p)
	return func(h crypto.Hash, hashed, signature []byte) error {
		if ed25519.Verify(pub, hashed, signature) {
			return nil
		}
		return errors.New("signature verification failed")
	}, nil
}
