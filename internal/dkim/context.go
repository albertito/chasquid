package dkim

import (
	"context"
	"net"
)

type contextKey string

const traceKey contextKey = "trace"

func trace(ctx context.Context, f string, args ...interface{}) {
	traceFunc, ok := ctx.Value(traceKey).(TraceFunc)
	if !ok {
		return
	}
	traceFunc(f, args...)
}

type TraceFunc func(f string, a ...interface{})

func WithTraceFunc(ctx context.Context, trace TraceFunc) context.Context {
	return context.WithValue(ctx, traceKey, trace)
}

const lookupTXTKey contextKey = "lookupTXT"

func lookupTXT(ctx context.Context, domain string) ([]string, error) {
	lookupTXTFunc, ok := ctx.Value(lookupTXTKey).(lookupTXTFunc)
	if !ok {
		return net.LookupTXT(domain)
	}
	return lookupTXTFunc(ctx, domain)
}

type lookupTXTFunc func(ctx context.Context, domain string) ([]string, error)

func WithLookupTXTFunc(ctx context.Context, lookupTXT lookupTXTFunc) context.Context {
	return context.WithValue(ctx, lookupTXTKey, lookupTXT)
}

const maxHeadersKey contextKey = "maxHeaders"

func WithMaxHeaders(ctx context.Context, maxHeaders int) context.Context {
	return context.WithValue(ctx, maxHeadersKey, maxHeaders)
}

func maxHeaders(ctx context.Context) int {
	maxHeaders, ok := ctx.Value(maxHeadersKey).(int)
	if !ok {
		// By default, cap the number of headers to 5 (arbitrarily chosen, may
		// be adjusted in the future).
		return 5
	}
	return maxHeaders
}
