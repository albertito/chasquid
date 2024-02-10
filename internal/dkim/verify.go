package dkim

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
)

// These two errors are returned when the verification fails, but the header
// is considered valid.
var (
	ErrBodyHashMismatch   = errors.New("body hash mismatch")
	ErrVerificationFailed = errors.New("verification failed")
)

// Evaluation states, as per
// https://datatracker.ietf.org/doc/html/rfc6376#section-3.9.
type EvaluationState string

const (
	SUCCESS  EvaluationState = "SUCCESS"
	PERMFAIL EvaluationState = "PERMFAIL"
	TEMPFAIL EvaluationState = "TEMPFAIL"
)

type VerifyResult struct {
	// How many signatures were found.
	Found uint

	// How many signatures were verified successfully.
	Valid uint

	// The details for each signature that was found.
	Results []*OneResult
}

type OneResult struct {
	// The raw signature header.
	SignatureHeader string

	// Domain and selector from the signature header.
	Domain   string
	Selector string

	// Base64-encoded signature. May be missing if it is not present in the
	// header.
	B string

	// The result of the evaluation.
	State EvaluationState
	Error error
}

// Returns the DKIM-specific contents for an Authentication-Results header.
// It is just the contents, the header needs to still be constructed.
// Note that the output will need to be indented by the caller.
// https://datatracker.ietf.org/doc/html/rfc8601#section-2.7.1
func (r *VerifyResult) AuthenticationResults() string {
	// The weird placement of the ";" is due to the specification saying they
	// have to be before each method, not at the end.
	// By doing it this way, we can concate the output of this function with
	// other results.
	ar := &strings.Builder{}
	if r.Found == 0 {
		// https://datatracker.ietf.org/doc/html/rfc8601#section-2.7.1
		ar.WriteString(";dkim=none\r\n")
		return ar.String()
	}

	for _, res := range r.Results {
		// Map state to the corresponding result.
		// https://datatracker.ietf.org/doc/html/rfc8601#section-2.7.1
		switch res.State {
		case SUCCESS:
			ar.WriteString(";dkim=pass")
		case TEMPFAIL:
			// The reason must come before the properties, include it here.
			fmt.Fprintf(ar, ";dkim=temperror  reason=%q\r\n", res.Error)
		case PERMFAIL:
			// The reason must come before the properties, include it here.
			if errors.Is(res.Error, ErrVerificationFailed) ||
				errors.Is(res.Error, ErrBodyHashMismatch) {
				fmt.Fprintf(ar, ";dkim=fail  reason=%q\r\n", res.Error)
			} else {
				fmt.Fprintf(ar, ";dkim=permerror  reason=%q\r\n", res.Error)
			}
		}

		if res.B != "" {
			// Include a partial b= tag to help identify which signature
			// is being referred to.
			// https://datatracker.ietf.org/doc/html/rfc6008#section-4
			fmt.Fprintf(ar, "  header.b=%.12s", res.B)
		}

		ar.WriteString("  header.d=" + res.Domain + "\r\n")
	}

	return ar.String()
}

func VerifyMessage(ctx context.Context, message string) (*VerifyResult, error) {
	// https://datatracker.ietf.org/doc/html/rfc6376#section-6
	headers, body, err := parseMessage(message)
	if err != nil {
		trace(ctx, "Error parsing message: %v", err)
		return nil, err
	}

	results := &VerifyResult{
		Results: []*OneResult{},
	}

	for i, sig := range headers.FindAll("DKIM-Signature") {
		trace(ctx, "Found DKIM-Signature header: %s", sig.Value)

		if i >= maxHeaders(ctx) {
			// Protect from potential DoS by capping the number of signatures.
			// https://datatracker.ietf.org/doc/html/rfc6376#section-4.2
			// https://datatracker.ietf.org/doc/html/rfc6376#section-8.4
			trace(ctx, "Too many DKIM-Signature headers found")
			break
		}

		results.Found++
		res := verifySignature(ctx, sig, headers, body)
		results.Results = append(results.Results, res)
		if res.State == SUCCESS {
			results.Valid++
		}
	}

	trace(ctx, "Found %d signatures, %d valid", results.Found, results.Valid)
	return results, nil
}

// Regular expression that matches the "b=" tag.
// First capture group is the "b=" part (including any whitespace up to the
// '=').
var bTag = regexp.MustCompile(`(b[ \t\r\n]*=)[^;]+`)

func verifySignature(ctx context.Context, sigH header,
	headers headers, body string) *OneResult {
	result := &OneResult{
		SignatureHeader: sigH.Value,
	}

	sig, err := dkimSignatureFromHeader(sigH.Value)
	if err != nil {
		// Header validation errors are a PERMFAIL.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-6.1.1
		result.Error = err
		result.State = PERMFAIL
		return result
	}

	result.Domain = sig.d
	result.Selector = sig.s
	result.B = base64.StdEncoding.EncodeToString(sig.b)

	// Get the public key.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-6.1.2
	pubKeys, err := findPublicKeys(ctx, sig.d, sig.s)
	if err != nil {
		result.Error = err

		// DNS errors when looking up the public key are a TEMPFAIL; all
		// others are PERMFAIL.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-6.1.2
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.Temporary() {
			result.State = TEMPFAIL
		} else {
			result.State = PERMFAIL
		}
		return result
	}

	// Compute the verification.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-6.1.3

	// Step 1: Prepare a canonicalized version of the body, truncate it to l=
	// (if present).
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.7
	bodyC := sig.cB.body(body)
	if sig.l > 0 {
		bodyC = bodyC[:sig.l]
	}

	// Step 2: Compute the hash of the canonicalized body.
	bodyH := hashWith(sig.Hash, []byte(bodyC))

	// Step 3: Verify the hash of the body by comparing it with bh=.
	if !bytes.Equal(bodyH, sig.bh) {
		bodyHStr := base64.StdEncoding.EncodeToString(bodyH)
		trace(ctx, "Body hash mismatch: %q", bodyHStr)

		result.Error = fmt.Errorf("%w (got %s)",
			ErrBodyHashMismatch, bodyHStr)
		result.State = PERMFAIL
		return result
	}
	trace(ctx, "Body hash matches: %q",
		base64.StdEncoding.EncodeToString(bodyH))

	// Step 4 A: Hash the (canonicalized) headers that appear in the h= tag.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.7
	b := sig.Hash.New()
	for _, header := range headersToInclude(sigH, sig.h, headers) {
		hsrc := sig.cH.header(header).Source + "\r\n"
		trace(ctx, "Hashing header: %q", hsrc)
		b.Write([]byte(hsrc))
	}

	// Step 4 B: Hash the (canonicalized) DKIM-Signature header itself, but
	// with an empty b= tag, and without a trailing \r\n.
	// https://datatracker.ietf.org/doc/html/rfc6376#section-3.7
	sigC := sig.cH.header(sigH)
	sigCStr := bTag.ReplaceAllString(sigC.Source, "$1")
	trace(ctx, "Hashing header: %q", sigCStr)
	b.Write([]byte(sigCStr))
	bSum := b.Sum(nil)
	trace(ctx, "Resulting hash: %q", base64.StdEncoding.EncodeToString(bSum))

	// Step 4 C: Validate the signature.
	for _, pubKey := range pubKeys {
		if !pubKey.Matches(sig.KeyType, sig.Hash) {
			trace(ctx, "PK %v: key type or hash mismatch, skipping", pubKey)
			continue
		}

		if sig.i != "" && pubKey.StrictDomainCheck() {
			_, domain, _ := strings.Cut(sig.i, "@")
			if domain != sig.d {
				trace(ctx, "PK %v: Strict domain check failed: %q != %q (%q)",
					pubKey, sig.d, domain, sig.i)
				continue
			}

			trace(ctx, "PK %v: Strict domain check passed", pubKey)
		}

		err := pubKey.verify(sig.Hash, bSum, sig.b)
		if err != nil {
			trace(ctx, "PK %v: Verification failed: %v", pubKey, err)
			continue
		}
		trace(ctx, "PK %v: Verification succeeded", pubKey)
		result.State = SUCCESS
		return result
	}

	result.State = PERMFAIL
	result.Error = ErrVerificationFailed
	return result
}

func headersToInclude(sigH header, hTag []string, headers headers) []header {
	// Return the actual headers to include in the hash, based on the list
	// given in the h= tag.
	// This is complicated because:
	//  - Headers can be included multiple times. In that case, we must pick
	//    the last instance (which hasn't been already included).
	//    https://datatracker.ietf.org/doc/html/rfc6376#section-5.4.2
	//  - Headers may appear fewer times than they are requested.
	//  - DKIM-Signature header may be included, but we must not include the
	//    one being verified.
	//    https://datatracker.ietf.org/doc/html/rfc6376#section-3.7
	//  - Headers may be missing, and that's allowed.
	//    https://datatracker.ietf.org/doc/html/rfc6376#section-5.4
	seen := map[string]int{}
	include := []header{}
	for _, h := range hTag {
		all := headers.FindAll(h)
		slices.Reverse(all)

		// We keep track of the last instance of each header that we
		// included, and find the next one every time it appears in h=.
		// We have to be careful because the header itself may not be present,
		// or we may be asked to include it more times than it appears.
		lh := strings.ToLower(h)
		i := seen[lh]
		if i >= len(all) {
			continue
		}
		seen[lh]++

		selected := all[i]

		if selected == sigH {
			continue
		}

		include = append(include, selected)
	}

	return include
}

func hashWith(a crypto.Hash, data []byte) []byte {
	h := a.New()
	h.Write(data)
	return h.Sum(nil)
}
