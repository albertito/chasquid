package dkim

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type Signer struct {
	// Domain to sign for.
	Domain string

	// Selector to use.
	Selector string

	// Signer containing the private key.
	// This can be an *rsa.PrivateKey or a ed25519.PrivateKey.
	Signer crypto.Signer
}

var headersToSign = []string{
	// https://datatracker.ietf.org/doc/html/rfc6376#section-5.4.1
	"From", // Required.
	"Reply-To",
	"Subject",
	"Date",
	"To", "Cc",
	"Resent-Date", "Resent-From", "Resent-To", "Resent-Cc",
	"In-Reply-To", "References",
	"List-Id", "List-Help", "List-Unsubscribe", "List-Subscribe", "List-Post",
	"List-Owner", "List-Archive",

	// Our additions.
	"Message-ID",
}

var extraHeadersToSign = []string{
	// Headers to add an extra of, to prevent additions after signing.
	// If they're included here, they must be in headersToSign too.
	"From",
	"Subject", "Date",
	"To", "Cc",
	"Message-ID",
}

// Sign the given message. Returns the *value* of the DKIM-Signature header to
// be added to the message. It will usually be multi-line, but without
// indenting.
func (s *Signer) Sign(ctx context.Context, message string) (string, error) {
	headers, body, err := parseMessage(message)
	if err != nil {
		return "", err
	}

	algoStr, err := s.algoStr()
	if err != nil {
		return "", err
	}

	trace(ctx, "Signing for %s / %s with %s", s.Domain, s.Selector, algoStr)

	dkimSignature := fmt.Sprintf(
		"v=1; a=%s; c=relaxed/relaxed;\r\n", algoStr)
	dkimSignature += fmt.Sprintf(
		"d=%s; s=%s; t=%d;\r\n", s.Domain, s.Selector, time.Now().Unix())

	// Add the headers to sign.
	hsForHeader := []string{}
	for _, h := range headersToSign {
		// Append the header as many times as it appears in the message.
		for i := 0; i < len(headers.FindAll(h)); i++ {
			hsForHeader = append(hsForHeader, h)
		}
	}
	hsForHeader = append(hsForHeader, extraHeadersToSign...)

	dkimSignature += fmt.Sprintf(
		"h=%s;\r\n", formatHeaders(hsForHeader))

	// Compute and add bh= (body hash).
	bodyH := sha256.Sum256([]byte(
		relaxedCanonicalization.body(body)))
	dkimSignature += fmt.Sprintf(
		"bh=%s;\r\n", base64.StdEncoding.EncodeToString(bodyH[:]))

	// Compute b= (signature).
	// First, the canonicalized headers.
	b := sha256.New()
	for _, h := range headersToSign {
		for _, header := range headers.FindAll(h) {
			hsrc := relaxedCanonicalization.header(header).Source + "\r\n"
			trace(ctx, "Hashing header: %q", hsrc)
			b.Write([]byte(hsrc))
		}
	}

	// Now, the (canonicalized) DKIM-Signature header itself, but with an
	// empty b= tag, without a trailing \r\n, and ending with ";".
	// We include the ";" because we will add it at the end (see below). It is
	// legal not to include that final ";", we just choose to do so.
	// We replace \r\n with \r\n\t so the canonicalization considers them
	// proper continuations, and works correctly.
	dkimSignature += "b="
	dkimSignatureForSigning := strings.ReplaceAll(
		dkimSignature, "\r\n", "\r\n\t") + ";"
	relaxedDH := relaxedCanonicalization.header(header{
		Name:   "DKIM-Signature",
		Value:  dkimSignatureForSigning,
		Source: dkimSignatureForSigning,
	})
	b.Write([]byte(relaxedDH.Source))
	trace(ctx, "Hashing header: %q", relaxedDH.Source)
	bSum := b.Sum(nil)
	trace(ctx, "Resulting hash: %q", base64.StdEncoding.EncodeToString(bSum))

	// Finally, sign the hash.
	sig, err := s.sign(bSum)
	if err != nil {
		return "", err
	}
	sigb64 := base64.StdEncoding.EncodeToString(sig)

	dkimSignature += breakLongLines(sigb64) + ";"

	return dkimSignature, nil
}

func (s *Signer) algoStr() (string, error) {
	switch k := s.Signer.(type) {
	case *rsa.PrivateKey:
		return "rsa-sha256", nil
	case ed25519.PrivateKey:
		return "ed25519-sha256", nil
	default:
		return "", fmt.Errorf("%w: %T", errUnsupportedKeyType, k)
	}
}

func (s *Signer) sign(bSum []byte) ([]byte, error) {
	var h crypto.Hash
	switch s.Signer.(type) {
	case *rsa.PrivateKey:
		h = crypto.SHA256
	case ed25519.PrivateKey:
		h = crypto.Hash(0)
	}

	return s.Signer.Sign(rand.Reader, bSum, h)
}

func breakLongLines(s string) string {
	// Break long lines, indenting with 2 spaces for continuation (to make
	// it clear it's under the same tag).
	const limit = 70
	var sb strings.Builder
	for len(s) > 0 {
		if len(s) > limit {
			sb.WriteString(s[:limit])
			sb.WriteString("\r\n  ")
			s = s[limit:]
		} else {
			sb.WriteString(s)
			s = ""
		}
	}
	return sb.String()
}

func formatHeaders(hs []string) string {
	// Format the list of headers for inclusion in the DKIM-Signature header.
	// This includes converting them to lowercase, and line-wrapping.
	// Extra lines will be indented with 2 spaces, to make it clear they're
	// under the same tag.
	const limit = 70
	var sb strings.Builder
	line := ""
	for i, h := range hs {
		if len(line)+1+len(h) > limit {
			sb.WriteString(line + "\r\n  ")
			line = ""
		}

		if i > 0 {
			line += ":"
		}
		line += h
	}
	sb.WriteString(line)

	return strings.TrimSpace(strings.ToLower(sb.String()))
}
