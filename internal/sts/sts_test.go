package sts

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/testlib"
)

// Override the lookup function to control its results.
var txtResults = map[string][]string{
	"dom1": nil,
	"dom2": {},
	"dom3": {"abc", "def"},
	"dom4": {"abc", "v=STSv1; id=blah;"},

	// Matching policyForDomain below.
	"_mta-sts.domain.com": {"v=STSv1; id=blah;"},
	"_mta-sts.policy404":  {"v=STSv1; id=blah;"},
	"_mta-sts.version99":  {"v=STSv1; id=blah;"},
}
var errTest = fmt.Errorf("error for testing purposes")
var txtErrors = map[string]error{
	"_mta-sts.domErr": errTest,
}

func testLookupTXT(domain string) ([]string, error) {
	return txtResults[domain], txtErrors[domain]
}

// Test policy for each of the requested domains.  Will be served by the test
// HTTP server.
var policyForDomain = map[string]string{
	// domain.com -> valid, with reasonable policy.
	"domain.com": `
             version: STSv1
             mode: enforce
             mx: *.mail.domain.com
             max_age: 3600
        `,

	// version99 -> invalid policy (unknown version).
	"version99": `
             version: STSv99
             mode: enforce
             mx: *.mail.version99
             max_age: 999
        `,
}

func testHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// For testing, the domain in the path (see urlForDomain).
	policy, ok := policyForDomain[r.URL.Path[1:]]
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	fmt.Fprintln(w, policy)
	return
}

func TestMain(m *testing.M) {
	lookupTXT = testLookupTXT

	// Create a test HTTP server, used by the more end-to-end tests.
	httpServer := httptest.NewServer(http.HandlerFunc(testHTTPHandler))

	fakeURLForTesting = httpServer.URL
	os.Exit(m.Run())
}

func TestParsePolicy(t *testing.T) {
	const pol1 = `
  version: STSv1
  mode: enforce
  mx: *.mail.example.com
  max_age: 123456
`
	p, err := parsePolicy([]byte(pol1))
	if err != nil {
		t.Errorf("failed to parse policy: %v", err)
	}

	t.Logf("pol1: %+v", p)
}

func TestCheckPolicy(t *testing.T) {
	validPs := []Policy{
		{Version: "STSv1", Mode: "enforce", MaxAge: 1 * time.Hour,
			MXs: []string{"mx1", "mx2"}},
		{Version: "STSv1", Mode: "testing", MaxAge: 1 * time.Hour,
			MXs: []string{"mx1"}},
		{Version: "STSv1", Mode: "none", MaxAge: 1 * time.Hour,
			MXs: []string{"mx1"}},
	}
	for i, p := range validPs {
		if err := p.Check(); err != nil {
			t.Errorf("%d policy %v failed check: %v", i, p, err)
		}
	}

	invalid := []struct {
		p        Policy
		expected error
	}{
		{Policy{Version: "STSv2"}, ErrUnknownVersion},
		{Policy{Version: "STSv1"}, ErrInvalidMaxAge},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "blah"}, ErrInvalidMode},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "enforce"}, ErrInvalidMX},
		{Policy{Version: "STSv1", MaxAge: 1, Mode: "enforce", MXs: []string{}},
			ErrInvalidMX},
	}
	for i, c := range invalid {
		if err := c.p.Check(); err != c.expected {
			t.Errorf("%d policy %v check: expected %v, got %v", i, c.p,
				c.expected, err)
		}
	}
}

func TestMatchDomain(t *testing.T) {
	cases := []struct {
		domain, pattern string
		expected        bool
	}{
		{"lalala", "lalala", true},
		{"a.b.", "a.b", true},
		{"a.b", "a.b.", true},
		{"abc.com", "*.com", true},

		{"abc.com", "abc.*.com", false},
		{"abc.com", "x.abc.com", false},
		{"x.abc.com", "*.*.com", false},
		{"abc.def.com", "abc.*.com", false},

		{"ñaca.com", "ñaca.com", true},
		{"Ñaca.com", "ñaca.com", true},
		{"ñaca.com", "Ñaca.com", true},
		{"x.ñaca.com", "x.xn--aca-6ma.com", true},
		{"x.naca.com", "x.xn--aca-6ma.com", false},

		// Triggers errors in domainToASCII.
		{strings.Repeat("x", 65536) + "\uff00", "x.com", false},

		// Examples from the RFC.
		{"mail.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"foo.bar.example.com", "*.example.com", false},

		// Missing "*" (invalid, seen in the wild).
		{"aa.b.cc.com", ".aa.b.cc.com", false},
		{"zz.aa.b.cc.com", ".aa.b.cc.com", false},
		{"zz.aa.b.cc.com", "*.aa.b.cc.com", true},
	}

	for _, c := range cases {
		if r := matchDomain(c.domain, c.pattern); r != c.expected {
			t.Errorf("matchDomain(%q, %q) = %v, expected %v",
				c.domain, c.pattern, r, c.expected)
		}
	}
}

func TestMXIsAllowed(t *testing.T) {
	p := Policy{Version: "STSv1", Mode: "enforce", MaxAge: 1 * time.Hour,
		MXs: []string{"mx1", "mx2"}}
	if p.MXIsAllowed("notamx") {
		t.Errorf("notamx should not be allowed")
	}
	if !p.MXIsAllowed("mx1") {
		t.Errorf("mx1 should be allowed")
	}
	if !p.MXIsAllowed("mx2") {
		t.Errorf("mx2 should be allowed")
	}

	p = Policy{Version: "STSv1", Mode: "testing", MaxAge: 1 * time.Hour,
		MXs: []string{"mx1"}}
	if !p.MXIsAllowed("notamx") {
		t.Errorf("notamx should be allowed (policy not enforced)")
	}
}

func TestFetch(t *testing.T) {
	// Note the data "fetched" for each domain comes from policyForDomain,
	// defined in TestMain above. See httpGet for more details.

	// Normal fetch, all valid.
	p, err := Fetch(context.Background(), "domain.com")
	if err != nil {
		t.Errorf("failed to fetch policy: %v", err)
	}
	t.Logf("domain.com: %+v", p)

	// Domain without a policy (HTTP get fails).
	p, err = Fetch(context.Background(), "policy404")
	if err == nil {
		t.Errorf("fetched unknown policy: %v", p)
	}
	t.Logf("policy404: got error as expected: %v", err)

	// Domain with an invalid policy (unknown version).
	p, err = Fetch(context.Background(), "version99")
	if err != ErrUnknownVersion {
		t.Errorf("expected error %v, got %v (and policy: %v)",
			ErrUnknownVersion, err, p)
	}
	t.Logf("version99: got expected error: %v", err)

	// Error fetching TXT record for this domain.
	p, err = Fetch(context.Background(), "domErr")
	if err != errTest {
		t.Errorf("expected error %v, got %v (and policy: %v)",
			errTest, err, p)
	}
	t.Logf("domErr: got expected error: %v", err)
}

func TestPolicyTooBig(t *testing.T) {
	// Construct a valid but very large JSON as a policy.
	raw := `{"version": "STSv1", "mode": "enforce", "mx": [`
	for i := 0; i < 2000; i++ {
		raw += fmt.Sprintf("\"mx%d\", ", i)
	}
	raw += `"mxlast"], "max_age": 100}`
	policyForDomain["toobig"] = raw

	_, err := Fetch(context.Background(), "toobig")
	if err == nil {
		t.Errorf("fetch worked, but should have failed")
	}
	t.Logf("got error as expected: %v", err)
}

// Tests for the policy cache.

func expvarMustEq(t *testing.T, name string, v *expvar.Int, expected int) {
	// TODO: Use v.Value once we drop support of Go 1.7.
	value, _ := strconv.Atoi(v.String())
	if value != expected {
		t.Errorf("%s is %d, expected %d", name, value, expected)
	}
}

func TestCacheBasics(t *testing.T) {
	dir := testlib.MustTempDir(t)
	c, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Note the data "fetched" for each domain comes from policyForDomain,
	// defined in TestMain above. See httpGet for more details.

	// Reset the expvar counters that we use to validate hits, misses, etc.
	cacheFetches.Set(0)
	cacheHits.Set(0)

	ctx := context.Background()

	// Fetch domain.com, check we get a reasonable policy, and that it's a
	// cache miss.
	p, err := c.Fetch(ctx, "domain.com")
	if err != nil || p.Check() != nil || p.MXs[0] != "*.mail.domain.com" {
		t.Errorf("unexpected fetch result - policy = %v ; error = %v", p, err)
	}
	t.Logf("cache fetched domain.com: %v", p)
	expvarMustEq(t, "cacheFetches", cacheFetches, 1)
	expvarMustEq(t, "cacheHits", cacheHits, 0)

	// Fetch domain.com again, this time we should see a cache hit.
	p, err = c.Fetch(ctx, "domain.com")
	if err != nil || p.Check() != nil || p.MXs[0] != "*.mail.domain.com" {
		t.Errorf("unexpected fetch result - policy = %v ; error = %v", p, err)
	}
	t.Logf("cache fetched domain.com: %v", p)
	expvarMustEq(t, "cacheFetches", cacheFetches, 2)
	expvarMustEq(t, "cacheHits", cacheHits, 1)

	// Simulate an expired cache entry by changing the mtime of domain.com's
	// entry to the past.
	expires := time.Now().Add(-1 * time.Minute)
	os.Chtimes(c.domainPath("domain.com"), expires, expires)

	// Do a third fetch, check that we don't get a cache hit.
	p, err = c.Fetch(ctx, "domain.com")
	if err != nil || p.Check() != nil || p.MXs[0] != "*.mail.domain.com" {
		t.Errorf("unexpected fetch result - policy = %v ; error = %v", p, err)
	}
	t.Logf("cache fetched domain.com: %v", p)
	expvarMustEq(t, "cacheFetches", cacheFetches, 3)
	expvarMustEq(t, "cacheHits", cacheHits, 1)

	// Fetch for a domain without policy.
	p, err = c.Fetch(ctx, "domErr")
	if err == nil || p != nil {
		t.Errorf("expected failure, got: policy = %v ; error = %v", p, err)
	}
	t.Logf("cache fetched domErr: %v", p)
	expvarMustEq(t, "cacheFetches", cacheFetches, 4)
	expvarMustEq(t, "cacheHits", cacheHits, 1)
	expvarMustEq(t, "cacheFailedFetch", cacheFailedFetch, 1)

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

// Test how the cache behaves when the files are corrupt.
func TestCacheBadData(t *testing.T) {
	dir := testlib.MustTempDir(t)
	c, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	cacheUnmarshalErrors.Set(0)
	cacheInvalid.Set(0)

	cases := []string{
		// Case 1: A file with invalid json, which will fail unmarshalling.
		"this is not valid json",

		// Case 2: A file with a parseable but invalid policy.
		`{"version": "STSv1", "mode": "INVALID", "mx": ["mx"], "max_age": 1}`,
	}

	for _, badContent := range cases {
		// Reset the expvar counters that we use to validate hits, misses, etc.
		cacheFetches.Set(0)
		cacheHits.Set(0)

		// Fetch domain.com, should result in the file being added to the
		// cache.
		p, err := c.Fetch(ctx, "domain.com")
		if err != nil {
			t.Fatalf("Fetch failed: %v", err)
		}
		t.Logf("cache fetched domain.com: %v", p)
		expvarMustEq(t, "cacheFetches", cacheFetches, 1)
		expvarMustEq(t, "cacheHits", cacheHits, 0)

		// Edit the file, filling it with the bad content for this case.
		fname := c.domainPath("domain.com")
		mustRewriteAndChtime(t, fname, badContent)

		// We now expect Fetch to fall back to getting the policy from the
		// network (in our case, from policyForDomain).
		p, err = c.Fetch(ctx, "domain.com")
		if err != nil {
			t.Fatalf("Fetch failed: %v", err)
		}
		t.Logf("cache fetched domain.com: %v", p)
		expvarMustEq(t, "cacheFetches", cacheFetches, 2)
		expvarMustEq(t, "cacheHits", cacheHits, 0)

		// And now the file should be fine, resulting in a cache hit.
		p, err = c.Fetch(ctx, "domain.com")
		if err != nil {
			t.Fatalf("Fetch failed: %v", err)
		}
		t.Logf("cache fetched domain.com: %v", p)
		expvarMustEq(t, "cacheFetches", cacheFetches, 3)
		expvarMustEq(t, "cacheHits", cacheHits, 1)

		// Remove the file, to start with a clean slate for the next case.
		os.Remove(fname)
	}

	expvarMustEq(t, "cacheUnmarshalErrors", cacheUnmarshalErrors, 1)
	expvarMustEq(t, "cacheInvalid", cacheInvalid, 1)

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func mustFetch(t *testing.T, c *PolicyCache, ctx context.Context, d string) *Policy {
	p, err := c.Fetch(ctx, d)
	if err != nil {
		t.Fatalf("Fetch %q failed: %v", d, err)
	}
	t.Logf("Fetch %q: %v", d, p)
	return p
}

func mustRewriteAndChtime(t *testing.T, fname, content string) {
	testlib.Rewrite(t, fname, content)

	// Advance the expiration time to the future, so the rewritten policy is
	// not considered expired.
	expires := time.Now().Add(10 * time.Second)
	err := os.Chtimes(fname, expires, expires)
	if err != nil {
		t.Fatalf("failed to chtime %q to the past: %v", fname, err)
	}
}

func TestCacheRefresh(t *testing.T) {
	dir := testlib.MustTempDir(t)
	c, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	txtResults["_mta-sts.refresh-test"] = []string{"v=STSv1; id=blah;"}
	policyForDomain["refresh-test"] = `
		version: STSv1
		mode: enforce
		mx: mx
		max_age: 100`
	p := mustFetch(t, c, ctx, "refresh-test")
	if p.MaxAge != 100*time.Second {
		t.Fatalf("policy.MaxAge is %v, expected 100s", p.MaxAge)
	}

	// Change the "published" policy, check that we see the old version at
	// fetch (should be cached), and a new version after a refresh.
	policyForDomain["refresh-test"] = `
		version: STSv1
		mode: enforce
		mx: mx
		max_age: 200`

	p = mustFetch(t, c, ctx, "refresh-test")
	if p.MaxAge != 100*time.Second {
		t.Fatalf("policy.MaxAge is %v, expected 100s", p.MaxAge)
	}

	// Launch background refreshes, and wait for one to complete.
	// TODO: change to cacheRefreshCycles.Value once we drop support for Go
	// 1.7.
	cacheRefreshCycles.Set(0)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	go c.PeriodicallyRefresh(ctx)
	for cacheRefreshCycles.String() == "0" {
		time.Sleep(5 * time.Millisecond)
	}

	p = mustFetch(t, c, ctx, "refresh-test")
	if p.MaxAge != 200*time.Second {
		t.Fatalf("policy.MaxAge is %v, expected 200s", p.MaxAge)
	}

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestCacheSlashSafe(t *testing.T) {
	dir := testlib.MustTempDir(t)
	c, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered: %v", r)
		} else {
			t.Fatalf("check did not panic as expected")
		}
	}()

	c.domainPath("a/b")
}

func TestURLForDomain(t *testing.T) {
	// This function will behave differently if fakeURLForTesting is set, so
	// temporarily unset it.
	oldURL := fakeURLForTesting
	fakeURLForTesting = ""
	defer func() { fakeURLForTesting = oldURL }()

	got := urlForDomain("a-test-domain")
	expected := "https://mta-sts.a-test-domain/.well-known/mta-sts.txt"
	if got != expected {
		t.Errorf("got %q, expected %q", got, expected)
	}
}

func TestHasSTSRecord(t *testing.T) {
	txtResults["_mta-sts.dom1"] = nil
	txtResults["_mta-sts.dom2"] = []string{}
	txtResults["_mta-sts.dom3"] = []string{"abc", "def"}
	txtResults["_mta-sts.dom4"] = []string{"abc", "v=STSv1; id=blah;"}

	cases := []struct {
		domain string
		ok     bool
		err    error
	}{
		{"", false, nil},
		{"dom1", false, nil},
		{"dom2", false, nil},
		{"dom3", false, nil},
		{"dom4", true, nil},
		{"domErr", false, errTest},
	}
	for _, c := range cases {
		ok, err := hasSTSRecord(c.domain)
		if ok != c.ok || err != c.err {
			t.Errorf("%s: expected {%v, %v}, got {%v, %v}", c.domain,
				c.ok, c.err, ok, err)
		}
	}
}

func TestHTTPGet(t *testing.T) {
	// Basic test, it should work.
	srv1 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(policyForDomain["domain.com"]))
		}))
	defer srv1.Close()

	ctx := context.Background()
	raw, err := httpGet(ctx, srv1.URL)
	if err != nil {
		t.Errorf("GET failed: got %q, %v", raw, err)
	}

	// Test that redirects are rejected.
	srv2 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fakeURLForTesting, http.StatusMovedPermanently)
		}))
	defer srv2.Close()

	raw, err = httpGet(ctx, srv2.URL)
	if err == nil {
		t.Errorf("redirect allowed, should have failed: got %q, %v", raw, err)
	}

	// Content type != text/plain should be rejected.
	srv3 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/json")
			w.Write([]byte(policyForDomain["domain.com"]))
		}))
	defer srv3.Close()

	raw, err = httpGet(ctx, srv3.URL)
	if err != ErrInvalidMediaType {
		t.Errorf("content type != text/plain was allowed: got %q, %v", raw, err)
	}

	// Invalid (unparseable) media type.
	srv4 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "invalid/content/type")
			w.Write([]byte(policyForDomain["domain.com"]))
		}))
	defer srv4.Close()

	raw, err = httpGet(ctx, srv4.URL)
	if err == nil || err == ErrInvalidMediaType {
		t.Errorf("invalid content type was allowed: got %q, %v", raw, err)
	}
}
