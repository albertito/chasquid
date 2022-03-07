package nettrace

import "context"

type ctxKeyT string

const ctxKey ctxKeyT = "blitiri.com.ar/go/srv/nettrace"

// NewContext returns a new context with the given trace attached.
func NewContext(ctx context.Context, tr Trace) context.Context {
	return context.WithValue(ctx, ctxKey, tr)
}

// FromContext returns the trace attached to the given context (if any).
func FromContext(ctx context.Context) (Trace, bool) {
	tr, ok := ctx.Value(ctxKey).(Trace)
	return tr, ok
}

// FromContextOrNew returns the trace attached to the given context, or a new
// trace if there is none.
func FromContextOrNew(ctx context.Context, family, title string) (Trace, context.Context) {
	tr, ok := FromContext(ctx)
	if ok {
		return tr, ctx
	}

	tr = New(family, title)
	return tr, NewContext(ctx, tr)
}

// ChildFromContext returns a new trace that is a child of the one attached to
// the context (if any).
func ChildFromContext(ctx context.Context, family, title string) Trace {
	parent, ok := FromContext(ctx)
	if ok {
		return parent.NewChild(family, title)
	}
	return New(family, title)
}
