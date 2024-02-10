package dkim

import (
	"context"
	"fmt"
	"net"
	"testing"
)

func TestTraceNoCtx(t *testing.T) {
	// Call trace() on a context without a trace function, to check it doesn't
	// panic.
	ctx := context.Background()
	trace(ctx, "test")
}

func TestTrace(t *testing.T) {
	s := ""
	traceF := func(f string, a ...interface{}) {
		s = fmt.Sprintf(f, a...)
	}
	ctx := WithTraceFunc(context.Background(), traceF)
	trace(ctx, "test %d", 1)
	if s != "test 1" {
		t.Errorf("trace function not called")
	}
}

func TestLookupTXTNoCtx(t *testing.T) {
	// Call lookupTXT() on a context without an override, to check it calls
	// the real function.
	// We just check there is a reasonable error.
	// We don't specifically check that it's NXDOMAIN because if we don't have
	// internet access, the error may be different.
	ctx := context.Background()
	_, err := lookupTXT(ctx, "does.not.exist.example.com")
	if _, ok := err.(*net.DNSError); !ok {
		t.Fatalf("expected *net.DNSError, got %T", err)
	}
}

func TestLookupTXT(t *testing.T) {
	called := false
	lookupTXTF := func(ctx context.Context, name string) ([]string, error) {
		called = true
		return nil, nil
	}
	ctx := WithLookupTXTFunc(context.Background(), lookupTXTF)
	lookupTXT(ctx, "example.com")
	if !called {
		t.Errorf("lookupTXT function not called")
	}
}

func TestMaxHeaders(t *testing.T) {
	// First without an override, check we return the default.
	ctx := context.Background()
	if m := maxHeaders(ctx); m != 5 {
		t.Errorf("expected 5, got %d", m)
	}

	// Now with an override.
	ctx = WithMaxHeaders(ctx, 10)
	if m := maxHeaders(ctx); m != 10 {
		t.Errorf("expected 10, got %d", m)
	}
}
