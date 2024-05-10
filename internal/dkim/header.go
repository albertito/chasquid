package dkim

import (
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// https://datatracker.ietf.org/doc/html/rfc6376#section-6

type dkimSignature struct {
	// Version. Must be "1".
	v string

	// Algorithm. Like "rsa-sha256".
	a string

	// Key type, extracted from a=.
	KeyType keyType

	// Hash, extracted from a=.
	Hash crypto.Hash

	// Signature data.
	// Decoded from base64, ignoring whitespace.
	b []byte

	// Hash of canonicalized body.
	// Decoded from base64, ignoring whitespace.
	bh []byte

	// Canonicalization modes.
	cH canonicalization
	cB canonicalization

	// Domain ("SDID"), in plain text.
	// IDNs MUST be encoded as A-labels.
	d string

	// Signed header fields.
	// Colon-separated list of header fields.
	h []string

	// AUID, in plain text.
	i string

	// Body octet count of the canonicalized body.
	l uint64

	// Query methods used for DNS lookup.
	// Colon-separated list of methods. Only "dns/txt" is valid.
	q []string

	// Selector.
	s string

	// Timestamp. In Seconds since the UNIX epoch.
	t time.Time

	// Signature expiration. In Seconds since the UNIX epoch.
	x time.Time

	// Copied header fields.
	// Has a specific encoding but whitespace is ignored.
	z string
}

func (sig *dkimSignature) canonicalizationFromString(s string) error {
	if s == "" {
		sig.cH = simpleCanonicalization
		sig.cB = simpleCanonicalization
		return nil
	}

	// Either "header/body" or "header". In the latter case, "simple" is used
	// for the body canonicalization.
	// No whitespace around the '/' is allowed.
	hs, bs, _ := strings.Cut(s, "/")
	if bs == "" {
		bs = "simple"
	}

	var err error
	sig.cH, err = stringToCanonicalization(hs)
	if err != nil {
		return fmt.Errorf("header: %w", err)
	}
	sig.cB, err = stringToCanonicalization(bs)
	if err != nil {
		return fmt.Errorf("body: %w", err)
	}

	return nil
}

func (sig *dkimSignature) checkRequiredTags() error {
	// https://datatracker.ietf.org/doc/html/rfc6376#section-6.1.1
	if sig.a == "" {
		return fmt.Errorf("%w: a=", errMissingRequiredTag)
	}
	if len(sig.b) == 0 {
		return fmt.Errorf("%w: b=", errMissingRequiredTag)
	}
	if len(sig.bh) == 0 {
		return fmt.Errorf("%w: bh=", errMissingRequiredTag)
	}
	if sig.d == "" {
		return fmt.Errorf("%w: d=", errMissingRequiredTag)
	}
	if len(sig.h) == 0 {
		return fmt.Errorf("%w: h=", errMissingRequiredTag)
	}
	if sig.s == "" {
		return fmt.Errorf("%w: s=", errMissingRequiredTag)
	}

	// h= must contain From.
	var isFrom = func(s string) bool { return strings.EqualFold(s, "from") }
	if !slices.ContainsFunc(sig.h, isFrom) {
		return fmt.Errorf("%w: h= does not contain 'from'", errInvalidTag)
	}

	// If i= is present, its domain must be equal to, or a subdomain of, d=.
	if sig.i != "" {
		_, domain, _ := strings.Cut(sig.i, "@")
		if domain != sig.d && !strings.HasSuffix(domain, "."+sig.d) {
			return fmt.Errorf("%w: i= is not a subdomain of d=",
				errInvalidTag)
		}
	}

	return nil
}

var (
	errInvalidSignature   = errors.New("invalid signature")
	errInvalidVersion     = errors.New("invalid version")
	errBadATag            = errors.New("invalid a= tag")
	errUnsupportedHash    = errors.New("unsupported hash")
	errUnsupportedKeyType = errors.New("unsupported key type")
	errMissingRequiredTag = errors.New("missing required tag")
	errNegativeTimestamp  = errors.New("negative timestamp")
)

// String replacer that removes whitespace.
var eatWhitespace = strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "")

func dkimSignatureFromHeader(header string) (*dkimSignature, error) {
	tags, err := parseTags(header)
	if err != nil {
		return nil, err
	}

	sig := &dkimSignature{
		v: tags["v"],
		a: tags["a"],
	}

	// v= tag is mandatory and must be 1.
	if sig.v != "1" {
		return nil, errInvalidVersion
	}

	// a= tag is mandatory; check that we can parse it and that we support the
	// algorithms.
	ktS, hS, found := strings.Cut(sig.a, "-")
	if !found {
		return nil, errBadATag
	}
	sig.KeyType, err = keyTypeFromString(ktS)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, sig.a)
	}
	sig.Hash, err = hashFromString(hS)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, sig.a)
	}

	// b is base64-encoded, and whitespace in it must be ignored.
	sig.b, err = base64.StdEncoding.DecodeString(
		eatWhitespace.Replace(tags["b"]))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode b: %w",
			errInvalidSignature, err)
	}

	// bh - same as b.
	sig.bh, err = base64.StdEncoding.DecodeString(
		eatWhitespace.Replace(tags["bh"]))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode bh: %w",
			errInvalidSignature, err)
	}

	err = sig.canonicalizationFromString(tags["c"])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse c: %w",
			errInvalidSignature, err)
	}

	sig.d = tags["d"]

	// h is a colon-separated list of header fields.
	if tags["h"] != "" {
		sig.h = strings.Split(eatWhitespace.Replace(tags["h"]), ":")
	}

	sig.i = tags["i"]

	if tags["l"] != "" {
		sig.l, err = strconv.ParseUint(tags["l"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse l: %w",
				errInvalidSignature, err)
		}
	}

	// q is a colon-separated list of query methods.
	if tags["q"] != "" {
		sig.q = strings.Split(eatWhitespace.Replace(tags["q"]), ":")
	}
	if len(sig.q) > 0 && !slices.Contains(sig.q, "dns/txt") {
		return nil, fmt.Errorf("%w: no dns/txt query method in q",
			errInvalidSignature)
	}

	sig.s = tags["s"]

	if tags["t"] != "" {
		sig.t, err = unixStrToTime(tags["t"])
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse t: %w",
				errInvalidSignature, err)
		}
	}

	if tags["x"] != "" {
		sig.x, err = unixStrToTime(tags["x"])
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse x: %w",
				errInvalidSignature, err)
		}
	}

	sig.z = eatWhitespace.Replace(tags["z"])

	// Check required tags are present.
	if err := sig.checkRequiredTags(); err != nil {
		return nil, err
	}

	return sig, nil
}

func unixStrToTime(s string) (time.Time, error) {
	// Technically the timestamp is an "unsigned decimal integer", but since
	// time.Unix takes an int64, we use that and check it's positive.
	ti, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if ti < 0 {
		return time.Time{}, errNegativeTimestamp
	}
	return time.Unix(ti, 0), nil
}

type keyType string

const (
	keyTypeRSA     keyType = "rsa"
	keyTypeEd25519 keyType = "ed25519"
)

func keyTypeFromString(s string) (keyType, error) {
	switch s {
	case "rsa":
		return keyTypeRSA, nil
	case "ed25519":
		return keyTypeEd25519, nil
	default:
		return "", errUnsupportedKeyType
	}
}

func hashFromString(s string) (crypto.Hash, error) {
	switch s {
	// Note SHA1 is not supported: as per RFC 8301, it must not be used
	// for signing or verifying.
	// https://datatracker.ietf.org/doc/html/rfc8301#section-3.1
	case "sha256":
		return crypto.SHA256, nil
	default:
		return 0, errUnsupportedHash
	}
}

// DKIM Tag=Value lists, as defined in RFC 6376, Section 3.2.
// https://datatracker.ietf.org/doc/html/rfc6376#section-3.2
type tags map[string]string

var errInvalidTag = errors.New("invalid tag")

func parseTags(s string) (tags, error) {
	// First trim space, and trailing semicolon, to simplify parsing below.
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ";")

	tags := make(tags)
	for _, tv := range strings.Split(s, ";") {
		t, v, found := strings.Cut(tv, "=")
		if !found {
			return nil, fmt.Errorf("%w: missing '='", errInvalidTag)
		}

		// Trim leading and trailing whitespace from tag and value, as per
		// RFC.
		t = strings.TrimSpace(t)
		v = strings.TrimSpace(v)

		if t == "" {
			return nil, fmt.Errorf("%w: missing tag name", errInvalidTag)
		}

		// RFC 6376, Section 3.2: Tags with duplicate names MUST NOT occur
		// within a single tag-list; if a tag name does occur more than once,
		// the entire tag-list is invalid.
		if _, exists := tags[t]; exists {
			return nil, fmt.Errorf("%w: duplicate tag", errInvalidTag)
		}

		tags[t] = v
	}

	return tags, nil
}
