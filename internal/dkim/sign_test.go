package dkim

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"
)

var basicMessage = toCRLF(
	`Received: from client1.football.example.com  [192.0.2.1]
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

func TestSignRSA(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceFunc(ctx, t.Logf)

	// Generate a new key pair.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pub, err := x509.MarshalPKIXPublicKey(priv.Public())
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}

	ctx = WithLookupTXTFunc(ctx, makeLookupTXT(map[string][]string{
		"test._domainkey.example.com": []string{
			"v=DKIM1; p=" + base64.StdEncoding.EncodeToString(pub),
		},
	}))

	s := &Signer{
		Domain:   "example.com",
		Selector: "test",
		Signer:   priv,
	}

	sig, err := s.Sign(ctx, basicMessage)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify the signature.
	res, err := VerifyMessage(ctx, addSig(sig, basicMessage))
	if err != nil || res.Valid != 1 {
		t.Errorf("VerifyMessage: wanted 1 valid / nil; got %v / %v", res, err)
	}

	// Compare the reproducible parts against a known-good header.
	want := regexp.MustCompile(
		"v=1; a=rsa-sha256; c=relaxed/relaxed;\r\n" +
			"d=example.com; s=test; t=\\d+;\r\n" +
			"h=from:subject:date:to:message-id:from:subject:date:to:cc:message-id;\r\n" +
			"bh=[A-Za-z0-9+/]+=*;\r\n" +
			"b=[A-Za-z0-9+/ \r\n]+=*;")
	if !want.MatchString(sig) {
		t.Errorf("Unexpected signature:")
		t.Errorf("  Want: %q (regexp)", want)
		t.Errorf("  Got:  %q", sig)
	}
}

func TestSignEd25519(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceFunc(ctx, t.Logf)

	// Generate a new key pair.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	ctx = WithLookupTXTFunc(ctx, makeLookupTXT(map[string][]string{
		"test._domainkey.example.com": []string{
			"v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub),
		},
	}))

	s := &Signer{
		Domain:   "example.com",
		Selector: "test",
		Signer:   priv,
	}

	sig, err := s.Sign(ctx, basicMessage)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify the signature.
	res, err := VerifyMessage(ctx, addSig(sig, basicMessage))
	if err != nil || res.Valid != 1 {
		t.Errorf("VerifyMessage: wanted 1 valid / nil; got %v / %v", res, err)
	}

	// Compare the reproducible parts against a known-good header.
	want := regexp.MustCompile(
		"v=1; a=ed25519-sha256; c=relaxed/relaxed;\r\n" +
			"d=example.com; s=test; t=\\d+;\r\n" +
			"h=from:subject:date:to:message-id:from:subject:date:to:cc:message-id;\r\n" +
			"bh=[A-Za-z0-9+/]+=*;\r\n" +
			"b=[A-Za-z0-9+/ \r\n]+=*;")
	if !want.MatchString(sig) {
		t.Errorf("Unexpected signature:")
		t.Errorf("  Want: %q (regexp)", want)
		t.Errorf("  Got:  %q", sig)
	}
}

func addSig(sig, message string) string {
	return "DKIM-Signature: " +
		strings.ReplaceAll(sig, "\r\n", "\r\n\t") +
		"\r\n" + message
}

func TestSignBadMessage(t *testing.T) {
	s := &Signer{
		Domain:   "example.com",
		Selector: "test",
	}
	_, err := s.Sign(context.Background(), "Bad message")
	if err == nil {
		t.Errorf("Sign: wanted error; got nil")
	}
}

func TestSignBadAlgorithm(t *testing.T) {
	s := &Signer{
		Domain:   "example.com",
		Selector: "test",
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	s.Signer = priv

	_, err = s.Sign(context.Background(), basicMessage)
	if !errors.Is(err, errUnsupportedKeyType) {
		t.Errorf("Sign: wanted unsupported key type; got %v", err)
	}
}

func TestBreakLongLines(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1234567890", "1234567890"},
		{
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70",
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70",
		},
		{
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70123",
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70\r\n  123",
		},
		{
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70xxxxxxxx80" +
				"xxxxxxxx90xxxxxxx100xxxxxxx110xxxxxxx120" +
				"xxxxxxx130xxxxxxx140xxxxxxx150xxxxxxx160",
			"xxxxxxxx10xxxxxxxx20xxxxxxxx30xxxxxxxx40" +
				"xxxxxxxx50xxxxxxxx60xxxxxxxx70\r\n  " +
				"xxxxxxxx80xxxxxxxx90xxxxxxx100xxxxxxx110" +
				"xxxxxxx120xxxxxxx130xxxxxxx140\r\n  " +
				"xxxxxxx150xxxxxxx160",
		},
	}

	for i, c := range cases {
		got := breakLongLines(c.in)
		if got != c.want {
			t.Errorf("%d: breakLongLines(%q):", i, c.in)
			t.Errorf("      want %q", c.want)
			t.Errorf("      got  %q", got)
		}
	}
}

func TestFormatHeaders(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"From"}, "from"},
		{
			[]string{"From", "Subject", "Date"},
			"from:subject:date",
		},
		{
			[]string{"from", "subject", "date", "to", "message-id",
				"from", "subject", "date", "to", "cc", "in-reply-to",
				"message-id"},
			"from:subject:date:to:message-id:" +
				"from:subject:date:to:cc:in-reply-to\r\n" +
				"  :message-id",
		},
		{
			[]string{"from", "subject", "date", "to", "message-id",
				"from", "subject", "date", "to", "cc", "xxxxxxxxxxxx70"},
			"from:subject:date:to:message-id:" +
				"from:subject:date:to:cc:xxxxxxxxxxxx70",
		},
		{
			[]string{"from", "subject", "date", "to", "message-id",
				"from", "subject", "date", "to", "cc", "xxxxxxxxxxxx701"},
			"from:subject:date:to:message-id:from:subject:date:to:cc\r\n" +
				"  :xxxxxxxxxxxx701",
		},
		{
			[]string{"from", "subject", "date", "to", "message-id",
				"from", "subject", "date", "to", "cc", "xxxxxxxxxxxx70",
				"1"},
			"from:subject:date:to:message-id:" +
				"from:subject:date:to:cc:xxxxxxxxxxxx70\r\n" +
				"  :1",
		},
	}

	for i, c := range cases {
		got := formatHeaders(c.in)
		if got != c.want {
			t.Errorf("%d: formatHeaders(%q):", i, c.in)
			t.Errorf("      want %q", c.want)
			t.Errorf("      got  %q", got)
		}
	}
}
