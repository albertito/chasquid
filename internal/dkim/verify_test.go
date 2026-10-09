package dkim

import (
	"context"
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func toCRLF(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func makeLookupTXT(results map[string][]string) lookupTXTFunc {
	return func(ctx context.Context, domain string) ([]string, error) {
		return results[domain], nil
	}
}

func TestVerifyRF6376CExample(t *testing.T) {
	ctx := t.Context()
	ctx = WithTraceFunc(ctx, t.Logf)

	// Use the public key from the example in RFC 6376 appendix C.
	// https://datatracker.ietf.org/doc/html/rfc6376#appendix-C
	ctx = WithLookupTXTFunc(ctx, makeLookupTXT(map[string][]string{
		"brisbane._domainkey.example.com": {
			"v=DKIM1; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQ" +
				"KBgQDwIRP/UC3SBsEmGqZ9ZJW3/DkMoGeLnQg1fWn7/zYt" +
				"IxN2SnFCjxOCKG9v3b4jYfcTNh5ijSsq631uBItLa7od+v" +
				"/RtdC2UzJ1lWT947qR+Rcac2gbto/NMqJ0fzfVjH4OuKhi" +
				"tdY9tf6mcwGjaNBcWToIMmPSPDdQPNUYckcQ2QIDAQAB",
		},
	}))

	// Note that the examples in the RFC text have multiple issues:
	// - The double space in "game.  Are" should be a single
	//   space. Otherwise, the body hash does not match.
	//   https://www.rfc-editor.org/errata/eid3192
	// - The header indentation is incorrect. This causes
	//   signature validation failure (because the example uses simple
	//   canonicalization, which leaves the indentation untouched).
	//   https://www.rfc-editor.org/errata/eid4926
	message := toCRLF(
		`DKIM-Signature: v=1; a=rsa-sha256; s=brisbane; d=example.com;
      c=simple/simple; q=dns/txt; i=joe@football.example.com;
      h=Received : From : To : Subject : Date : Message-ID;
      bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
      b=AuUoFEfDxTDkHlLXSZEpZj79LICEps6eda7W3deTVFOk4yAUoqOB
      4nujc7YopdG5dWLSdNg6xNAZpOPr+kHxt1IrE+NahM6L/LbvaHut
      KVdkLLkpVaVVQPzeRDI009SO2Il5Lu7rDNH6mZckBdrIx0orEtZV
      4bmp/YzhwvcubU4=;
Received: from client1.football.example.com  [192.0.2.1]
      by submitserver.example.com with SUBMISSION;
      Fri, 11 Jul 2003 21:01:54 -0700 (PDT)
From: Joe SixPack <joe@football.example.com>
To: Suzie Q <suzie@shopping.example.net>
Subject: Is dinner ready?
Date: Fri, 11 Jul 2003 21:00:37 -0700 (PDT)
Message-ID: <20030712040037.46341.5F8J@football.example.com>

Hi.

We lost the game. Are you hungry yet?

Joe.
`)

	res, err := VerifyMessage(ctx, message)
	if res.Valid != 1 || err != nil {
		t.Errorf("VerifyMessage: wanted 1 valid / nil; got %v / %v", res, err)
	}

	// Extend the message, check it does not pass validation.
	res, err = VerifyMessage(ctx, message+"Extra line.\r\n")
	if res.Valid != 0 || err != nil {
		t.Errorf("VerifyMessage: wanted 0 valid / nil; got %v / %v", res, err)
	}

	// Alter a header, check it does not pass validation.
	res, err = VerifyMessage(ctx,
		strings.Replace(message, "Subject", "X-Subject", 1))
	if res.Valid != 0 || err != nil {
		t.Errorf("VerifyMessage: wanted 0 valid / nil; got %v / %v", res, err)
	}
}

func TestVerifyRFC8463Example(t *testing.T) {
	ctx := t.Context()
	ctx = WithTraceFunc(ctx, t.Logf)

	// Use the public keys from the example in RFC 8463 appendix A.2.
	// https://datatracker.ietf.org/doc/html/rfc6376#appendix-C
	ctx = WithLookupTXTFunc(ctx, makeLookupTXT(map[string][]string{
		"brisbane._domainkey.football.example.com": {
			"v=DKIM1; k=ed25519; " +
				"p=11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="},

		"test._domainkey.football.example.com": {
			"v=DKIM1; k=rsa; " +
				"p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDkHlOQoBTzWR" +
				"iGs5V6NpP3idY6Wk08a5qhdR6wy5bdOKb2jLQiY/J16JYi0Qvx/b" +
				"yYzCNb3W91y3FutACDfzwQ/BC/e/8uBsCR+yz1Lxj+PL6lHvqMKr" +
				"M3rG4hstT5QjvHO9PzoxZyVYLzBfO2EeC3Ip3G+2kryOTIKT+l/K" +
				"4w3QIDAQAB"},
	}))

	message := toCRLF(
		`DKIM-Signature: v=1; a=ed25519-sha256; c=relaxed/relaxed;
 d=football.example.com; i=@football.example.com;
 q=dns/txt; s=brisbane; t=1528637909; h=from : to :
 subject : date : message-id : from : subject : date;
 bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
 b=/gCrinpcQOoIfuHNQIbq4pgh9kyIK3AQUdt9OdqQehSwhEIug4D11Bus
 Fa3bT3FY5OsU7ZbnKELq+eXdp1Q1Dw==
DKIM-Signature: v=1; a=rsa-sha256; c=relaxed/relaxed;
 d=football.example.com; i=@football.example.com;
 q=dns/txt; s=test; t=1528637909; h=from : to : subject :
 date : message-id : from : subject : date;
 bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
 b=F45dVWDfMbQDGHJFlXUNB2HKfbCeLRyhDXgFpEL8GwpsRe0IeIixNTe3
 DhCVlUrSjV4BwcVcOF6+FF3Zo9Rpo1tFOeS9mPYQTnGdaSGsgeefOsk2Jz
 dA+L10TeYt9BgDfQNZtKdN1WO//KgIqXP7OdEFE4LjFYNcUxZQ4FADY+8=
From: Joe SixPack <joe@football.example.com>
To: Suzie Q <suzie@shopping.example.net>
Subject: Is dinner ready?
Date: Fri, 11 Jul 2003 21:00:37 -0700 (PDT)
Message-ID: <20030712040037.46341.5F8J@football.example.com>

Hi.

We lost the game.  Are you hungry yet?

Joe.
`)

	expected := &VerifyResult{
		Found: 2,
		Valid: 2,
		Results: []*OneResult{
			{
				SignatureHeader: toCRLF(
					` v=1; a=ed25519-sha256; c=relaxed/relaxed;
 d=football.example.com; i=@football.example.com;
 q=dns/txt; s=brisbane; t=1528637909; h=from : to :
 subject : date : message-id : from : subject : date;
 bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
 b=/gCrinpcQOoIfuHNQIbq4pgh9kyIK3AQUdt9OdqQehSwhEIug4D11Bus
 Fa3bT3FY5OsU7ZbnKELq+eXdp1Q1Dw==`),
				Domain:   "football.example.com",
				Selector: "brisbane",
				B: "/gCrinpcQOoIfuHNQIbq4pgh9kyIK3AQUdt9OdqQehSwhEIug4D11" +
					"BusFa3bT3FY5OsU7ZbnKELq+eXdp1Q1Dw==",
				State: SUCCESS,
				Error: nil,
			},
			{
				SignatureHeader: toCRLF(
					` v=1; a=rsa-sha256; c=relaxed/relaxed;
 d=football.example.com; i=@football.example.com;
 q=dns/txt; s=test; t=1528637909; h=from : to : subject :
 date : message-id : from : subject : date;
 bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
 b=F45dVWDfMbQDGHJFlXUNB2HKfbCeLRyhDXgFpEL8GwpsRe0IeIixNTe3
 DhCVlUrSjV4BwcVcOF6+FF3Zo9Rpo1tFOeS9mPYQTnGdaSGsgeefOsk2Jz
 dA+L10TeYt9BgDfQNZtKdN1WO//KgIqXP7OdEFE4LjFYNcUxZQ4FADY+8=`),
				Domain:   "football.example.com",
				Selector: "test",
				B: "F45dVWDfMbQDGHJFlXUNB2HKfbCeLRyhDXgFpEL8GwpsRe0IeIixNTe" +
					"3DhCVlUrSjV4BwcVcOF6+FF3Zo9Rpo1tFOeS9mPYQTnGdaSGsgeefO" +
					"sk2JzdA+L10TeYt9BgDfQNZtKdN1WO//KgIqXP7OdEFE4LjFYNcUxZ" +
					"Q4FADY+8=",
				State: SUCCESS,
				Error: nil,
			},
		},
	}

	res, err := VerifyMessage(ctx, message)
	if err != nil {
		t.Fatalf("VerifyMessage returned error: %v", err)
	}
	if diff := cmp.Diff(expected, res); diff != "" {
		t.Errorf("VerifyMessage diff (-want +got):\n%s", diff)
	}

	// Extend the message, check it does not pass validation.
	res, err = VerifyMessage(ctx, message+"Extra line.\r\n")
	if res.Found != 2 || res.Valid != 0 || err != nil {
		t.Errorf("VerifyMessage: wanted 2 found, 0 valid / nil; got %v / %v",
			res, err)
	}

	// Alter a header, check it does not pass validation.
	res, err = VerifyMessage(ctx,
		strings.Replace(message, "Subject", "X-Subject", 1))
	if res.Found != 2 || res.Valid != 0 || err != nil {
		t.Errorf("VerifyMessage: wanted 2 found, 0 valid / nil; got %v / %v",
			res, err)
	}
}

func TestVerifyBodyLength(t *testing.T) {
	ctx := t.Context()
	ctx = WithTraceFunc(ctx, t.Logf)

	// Any valid key works, since none of the signatures here are valid. We
	// use the one from the example in RFC 8463 appendix A.2.
	ctx = WithLookupTXTFunc(ctx, makeLookupTXT(map[string][]string{
		"brisbane._domainkey.example.com": {
			"v=DKIM1; k=ed25519; " +
				"p=11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="},
	}))

	// Use a valid body hash (of the "signed" part of the body), so we can
	// tell the l= cases apart: when l= covers exactly the signed part, the
	// body hash matches and we fail later on (because b= is bogus).
	body := "Hi.\r\n"

	cases := []struct {
		l      string
		signed string
		want   error
	}{
		{"0", "", ErrVerificationFailed},
		{"0", body, ErrBodyHashMismatch},
		{"4", body, ErrBodyHashMismatch},
		{"5", body, ErrVerificationFailed},
		{"6", body, errInvalidTag},
		{"18446744073709551615", body, errInvalidTag},
	}
	for _, c := range cases {
		bh := base64.StdEncoding.EncodeToString(
			hashWith(crypto.SHA256, []byte(c.signed)))
		message := toCRLF(
			"DKIM-Signature: v=1; a=ed25519-sha256; c=simple/simple;\n"+
				" d=example.com; s=brisbane; h=from; l="+c.l+";\n"+
				" bh="+bh+"; b=AAAA\n"+
				"From: Joe SixPack <joe@example.com>\n"+
				"\n") + body

		res, err := VerifyMessage(ctx, message)
		if err != nil {
			t.Fatalf("l=%s signed=%q: VerifyMessage returned error: %v",
				c.l, c.signed, err)
		}
		if res.Found != 1 || res.Valid != 0 {
			t.Fatalf("l=%s signed=%q: wanted 1 found, 0 valid; got %v",
				c.l, c.signed, res)
		}
		r := res.Results[0]
		if r.State != PERMFAIL || !errors.Is(r.Error, c.want) {
			t.Errorf("l=%s signed=%q: wanted PERMFAIL / %v; got %v / %v",
				c.l, c.signed, c.want, r.State, r.Error)
		}
	}
}

func TestHeadersToInclude(t *testing.T) {
	// Test that headersToInclude returns the expected headers.
	cases := []struct {
		sigH    header
		hTag    []string
		headers headers
		want    []header
	}{
		// Check that if a header appears more than once, we pick the latest
		// first.
		{
			sigH: header{
				Name:  "DKIM-Signature",
				Value: "v=1; a=rsa-sha256; s=brisbane; d=example.com;",
			},
			hTag: []string{"From", "To", "Subject"},
			headers: headers{
				{Name: "From", Value: "from1"},
				{Name: "To", Value: "to1"},
				{Name: "Subject", Value: "subject1"},
				{Name: "From", Value: "from2"},
			},
			want: []header{
				{Name: "From", Value: "from2"},
				{Name: "To", Value: "to1"},
				{Name: "Subject", Value: "subject1"},
			},
		},

		// Check that if a header is requested twice but only appears once, we
		// only return it once.
		// This is a common technique suggested by the RFC to make signatures
		// fail if a header is added.
		{
			sigH: header{
				Name:  "DKIM-Signature",
				Value: "v=1; a=rsa-sha256; s=brisbane; d=example.com;",
			},
			hTag: []string{"From", "From", "To", "Subject"},
			headers: headers{
				{Name: "From", Value: "from1"},
				{Name: "To", Value: "to1"},
				{Name: "Subject", Value: "subject1"},
			},
			want: []header{
				{Name: "From", Value: "from1"},
				{Name: "To", Value: "to1"},
				{Name: "Subject", Value: "subject1"},
			},
		},

		// Check that if DKIM-Signature is included, we do *not* include the
		// one we're currently verifying in the headers to include.
		// https://datatracker.ietf.org/doc/html/rfc6376#section-3.7
		{
			sigH: header{
				Name:  "DKIM-Signature",
				Value: "v=1; a=rsa-sha256; s=brisbane; d=example.com;",
			},
			hTag: []string{"From", "From", "DKIM-Signature", "DKIM-Signature"},
			headers: headers{
				{Name: "From", Value: "from1"},
				{Name: "To", Value: "to1"},
				{
					Name:  "DKIM-Signature",
					Value: "v=1; a=rsa-sha256; s=sidney; d=example.com;",
				},
				{
					Name:  "DKIM-Signature",
					Value: "v=1; a=rsa-sha256; s=brisbane; d=example.com;",
				},
			},
			want: []header{
				{Name: "From", Value: "from1"},
				{
					Name:  "DKIM-Signature",
					Value: "v=1; a=rsa-sha256; s=sidney; d=example.com;",
				},
			},
		},

		// Check that header names are compared case-insensitively, and that
		// headers that are not present are skipped.
		{
			sigH: header{
				Name:  "DKIM-Signature",
				Value: "v=1; a=rsa-sha256; s=brisbane; d=example.com;",
			},
			hTag: []string{"from", "X-Missing", "FROM", "subject"},
			headers: headers{
				{Name: "FROM", Value: "from1"},
				{Name: "Subject", Value: "subject1"},
				{Name: "From", Value: "from2"},
			},
			want: []header{
				{Name: "From", Value: "from2"},
				{Name: "FROM", Value: "from1"},
				{Name: "Subject", Value: "subject1"},
			},
		},
	}

	for _, c := range cases {
		got := headersToInclude(c.sigH, c.hTag, c.headers)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("headersToInclude(%q, %v, %v) diff (-want +got):\n%s",
				c.sigH, c.hTag, c.headers, diff)
		}
	}
}

func TestHeadersToIncludeLarge(t *testing.T) {
	// Regression test for a DoS where headersToInclude was O(len(h=) *
	// len(headers)), which with large (attacker-controlled) inputs could take
	// hours. This takes milliseconds, while the quadratic implementation
	// took minutes with this input. We don't check the timing, so a
	// regression will show up as this test being very slow.
	const n = 100_000

	sigH := header{Name: "DKIM-Signature", Value: "v=1;"}
	hTag := make([]string, n)
	hdrs := make(headers, n)
	for i := range n {
		hTag[i] = "a"
		hdrs[i] = header{Name: "a", Value: fmt.Sprint(i)}
	}

	got := headersToInclude(sigH, hTag, hdrs)
	if len(got) != n {
		t.Fatalf("wanted %d headers, got %d", n, len(got))
	}
	// Last instance first.
	if got[0].Value != fmt.Sprint(n-1) || got[n-1].Value != "0" {
		t.Errorf("unexpected order: first %q, last %q",
			got[0].Value, got[n-1].Value)
	}
}

func TestAuthenticationResults(t *testing.T) {
	resBrisbane := &OneResult{
		Domain:   "football.example.com",
		Selector: "brisbane",
		B: "/gCrinpcQOoIfuHNQIbq4pgh9kyIK3AQUdt9OdqQehSwhEIug4D11" +
			"BusFa3bT3FY5OsU7ZbnKELq+eXdp1Q1Dw==",
		State: SUCCESS,
		Error: nil,
	}
	resTest := &OneResult{
		Domain:   "football.example.com",
		Selector: "test",
		B: "F45dVWDfMbQDGHJFlXUNB2HKfbCeLRyhDXgFpEL8GwpsRe0IeIixNTe" +
			"3DhCVlUrSjV4BwcVcOF6+FF3Zo9Rpo1tFOeS9mPYQTnGdaSGsgeefO" +
			"sk2JzdA+L10TeYt9BgDfQNZtKdN1WO//KgIqXP7OdEFE4LjFYNcUxZ" +
			"Q4FADY+8=",
		State: SUCCESS,
		Error: nil,
	}
	resFail := &OneResult{
		Domain:   "football.example.com",
		Selector: "paris",
		B:        "slfkdMSDFeslif39seFfjl93sljisdsdlif923l",
		State:    PERMFAIL,
		Error:    ErrVerificationFailed,
	}
	resPermFail := &OneResult{
		Domain:   "football.example.com",
		Selector: "paris",
		// No B tag on purpose.
		State: PERMFAIL,
		Error: errMissingRequiredTag,
	}
	resTempFail := &OneResult{
		Domain:   "football.example.com",
		Selector: "paris",
		B:        "shorty", // Less than 12 characters to check we include it well.
		State:    TEMPFAIL,
		Error: &net.DNSError{
			Err:         "dns temp error (for testing)",
			IsTemporary: true,
		},
	}

	cases := []struct {
		results *VerifyResult
		want    string
	}{
		{
			results: &VerifyResult{},
			want:    ";dkim=none\r\n",
		},
		{
			results: &VerifyResult{
				Found:   1,
				Valid:   1,
				Results: []*OneResult{resBrisbane},
			},
			want: ";dkim=pass" +
				"  header.b=/gCrinpcQOoI  header.d=football.example.com\r\n",
		},
		{
			results: &VerifyResult{
				Found:   2,
				Valid:   2,
				Results: []*OneResult{resBrisbane, resTest},
			},
			want: ";dkim=pass" +
				"  header.b=/gCrinpcQOoI  header.d=football.example.com\r\n" +
				";dkim=pass" +
				"  header.b=F45dVWDfMbQD  header.d=football.example.com\r\n",
		},
		{
			results: &VerifyResult{
				Found:   2,
				Valid:   2,
				Results: []*OneResult{resBrisbane, resTest},
			},
			want: ";dkim=pass" +
				"  header.b=/gCrinpcQOoI  header.d=football.example.com\r\n" +
				";dkim=pass" +
				"  header.b=F45dVWDfMbQD  header.d=football.example.com\r\n",
		},
		{
			results: &VerifyResult{
				Found:   2,
				Valid:   1,
				Results: []*OneResult{resFail, resTest},
			},
			want: ";dkim=fail  reason=\"verification failed\"\r\n" +
				"  header.b=slfkdMSDFesl  header.d=football.example.com\r\n" +
				";dkim=pass" +
				"  header.b=F45dVWDfMbQD  header.d=football.example.com\r\n",
		},
		{
			results: &VerifyResult{
				Found:   1,
				Results: []*OneResult{resPermFail},
			},
			want: ";dkim=permerror  reason=\"missing required tag\"\r\n" +
				"  header.d=football.example.com\r\n",
		},
		{
			results: &VerifyResult{
				Found:   1,
				Results: []*OneResult{resTempFail},
			},
			want: ";dkim=temperror  reason=\"lookup : dns temp error (for testing)\"\r\n" +
				"  header.b=shorty  header.d=football.example.com\r\n",
		},
	}

	for i, c := range cases {
		got := c.results.AuthenticationResults()
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("case %d: AuthenticationResults() diff (-want +got):\n%s",
				i, diff)
		}
	}
}

func FuzzVerifyMessage(f *testing.F) {
	addTestdataCorpus(f)

	// Return valid keys for every domain, so we can get past the key lookup
	// and exercise the rest of the verification. The ed25519 key is the one
	// used by testdata/01-rfc8463.
	ctx := WithLookupTXTFunc(context.Background(),
		func(ctx context.Context, domain string) ([]string, error) {
			return []string{
				"v=DKIM1; k=ed25519; p=" + exampleEd25519KeyB64,
				"v=DKIM1; k=rsa; p=" + exampleRSAKeyB64,
			}, nil
		})

	f.Fuzz(func(t *testing.T, msg string) {
		res, err := VerifyMessage(ctx, msg)
		if err != nil {
			return
		}

		if res.Found != uint(len(res.Results)) {
			t.Errorf("found %d signatures, but got %d results",
				res.Found, len(res.Results))
		}
		if res.Valid > res.Found {
			t.Errorf("%d valid signatures, but only %d found",
				res.Valid, res.Found)
		}

		// This is always called on the result when receiving a message, so
		// exercise it too.
		res.AuthenticationResults()
	})
}
