// Package sts implements the MTA-STS (Strict Transport Security), based on
// the current draft, https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02.
//
// This is an EXPERIMENTAL implementation for now.
//
// It lacks (at least) the following:
// - Caching.
// - DNS TXT checking.
// - Facilities for reporting.
//
package sts

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/idna"
)

// Policy represents a parsed policy.
// https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-3.2
type Policy struct {
	Version string        `json:"version"`
	Mode    Mode          `json:"mode"`
	MXs     []string      `json:"mx"`
	MaxAge  time.Duration `json:"max_age"`
}

type Mode string

// Valid modes.
const (
	Enforce = Mode("enforce")
	Report  = Mode("report")
)

// parsePolicy parses a JSON representation of the policy, and returns the
// corresponding Policy structure.
func parsePolicy(raw []byte) (*Policy, error) {
	p := &Policy{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}

	// MaxAge is in seconds.
	p.MaxAge = p.MaxAge * time.Second

	return p, nil
}

var (
	ErrUnknownVersion = errors.New("unknown policy version")
	ErrInvalidMaxAge  = errors.New("invalid max_age")
	ErrInvalidMode    = errors.New("invalid mode")
	ErrInvalidMX      = errors.New("invalid mx")
)

// Check that the policy contents are valid.
func (p *Policy) Check() error {
	if p.Version != "STSv1" {
		return ErrUnknownVersion
	}
	if p.MaxAge <= 0 {
		return ErrInvalidMaxAge
	}

	if p.Mode != Enforce && p.Mode != Report {
		return ErrInvalidMode
	}

	// "mx" field is required, and the policy is invalid if it's not present.
	// https://mailarchive.ietf.org/arch/msg/uta/Omqo1Bw6rJbrTMl2Zo69IJr35Qo
	if len(p.MXs) == 0 {
		return ErrInvalidMX
	}

	return nil
}

// MXMatches checks if the given MX is allowed, according to the policy.
// https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-4.1
func (p *Policy) MXIsAllowed(mx string) bool {
	for _, pattern := range p.MXs {
		if matchDomain(mx, pattern) {
			return true
		}
	}

	return false
}

// UncheckedFetch fetches and parses the policy, but does NOT check it.
// This can be useful for debugging and troubleshooting, but you should always
// call Check on the policy before using it.
func UncheckedFetch(ctx context.Context, domain string) (*Policy, error) {
	// Convert the domain to ascii form, as httpGet does not support IDNs in
	// any other way.
	domain, err := idna.ToASCII(domain)
	if err != nil {
		return nil, err
	}

	// URL composed from the domain, as explained in:
	// https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-3.3
	// https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-3.2
	url := "https://mta-sts." + domain + "/.well-known/mta-sts.json"

	rawPolicy, err := httpGet(ctx, url)
	if err != nil {
		return nil, err
	}

	return parsePolicy(rawPolicy)
}

// Fetch a policy for the given domain. Note this results in various network
// lookups and HTTPS GETs, so it can be slow.
// The returned policy is parsed and sanity-checked (using Policy.Check), so
// it should be safe to use.
func Fetch(ctx context.Context, domain string) (*Policy, error) {
	p, err := UncheckedFetch(ctx, domain)
	if err != nil {
		return nil, err
	}

	err = p.Check()
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Fake HTTP content for testing purposes only.
var fakeContent = map[string]string{}

// httpGet performs an HTTP GET of the given URL, using the context and
// rejecting redirects, as per the standard.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{
		// We MUST NOT follow redirects, see
		// https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-3.3
		CheckRedirect: rejectRedirect,
	}

	// Note that http does not care for the context deadline, so we need to
	// construct it here.
	if deadline, ok := ctx.Deadline(); ok {
		client.Timeout = deadline.Sub(time.Now())
	}

	if len(fakeContent) > 0 {
		// If we have fake content for testing, then return the content for
		// the URL, or an error if it's missing.
		// This makes sure we don't make actual requests for testing.
		if d, ok := fakeContent[url]; ok {
			return []byte(d), nil
		}
		return nil, errors.New("error for testing")
	}

	resp, err := ctxhttp.Get(ctx, client, url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

var errRejectRedirect = errors.New("redirects not allowed in MTA-STS")

func rejectRedirect(req *http.Request, via []*http.Request) error {
	return errRejectRedirect
}

// matchDomain checks if the domain matches the given pattern, according to
// https://tools.ietf.org/html/rfc6125#section-6.4
// (from https://tools.ietf.org/html/draft-ietf-uta-mta-sts-02#section-4.1).
func matchDomain(domain, pattern string) bool {
	domain, dErr := domainToASCII(domain)
	pattern, pErr := domainToASCII(pattern)
	if dErr != nil || pErr != nil {
		// Domains should already have been checked and normalized by the
		// caller, exposing this is not worth the API complexity in this case.
		return false
	}

	domainLabels := strings.Split(domain, ".")
	patternLabels := strings.Split(pattern, ".")

	if len(domainLabels) != len(patternLabels) {
		return false
	}

	for i, p := range patternLabels {
		// Wildcards only apply to the first part, see
		// https://tools.ietf.org/html/rfc6125#section-6.4.3 #1 and #2.
		// This also allows us to do the lenght comparison above.
		if p == "*" && i == 0 {
			continue
		}

		if p != domainLabels[i] {
			return false
		}
	}

	return true
}

// domainToASCII converts the domain to ASCII form, similar to idna.ToASCII
// but with some preprocessing convenient for our use cases.
func domainToASCII(domain string) (string, error) {
	domain = strings.TrimSuffix(domain, ".")
	domain = strings.ToLower(domain)
	return idna.ToASCII(domain)
}
