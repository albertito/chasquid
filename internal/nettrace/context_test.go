package nettrace

import (
	"context"
	"testing"
)

func TestContext(t *testing.T) {
	tr := New("TestContext", "trace")
	defer tr.Finish()

	// Attach the trace to a new context.
	ctx := NewContext(context.Background(), tr)

	// Get the trace back from the context.
	{
		tr2, ok := FromContext(ctx)
		if !ok {
			t.Errorf("Context with trace returned not found")
		}
		if tr != tr2 {
			t.Errorf("Trace from context is different: %v != %v", tr, tr2)
		}
	}

	// Create a child trace from the context.
	{
		tr3 := ChildFromContext(ctx, "TestContext", "child")
		if p := tr3.(*trace).Parent; p != tr {
			t.Errorf("Child doesn't have the right parent: %v != %v", p, tr)
		}
		tr3.Finish()
	}

	// FromContextOrNew returns the one from the context.
	{
		tr4, ctx4 := FromContextOrNew(ctx, "TestContext", "from-ctx")
		if ctx4 != ctx {
			t.Errorf("Got new context: %v != %v", ctx4, ctx)
		}
		if tr4 != tr {
			t.Errorf("Context with trace returned new trace: %v != %v", tr4, tr)
		}
	}

	// FromContextOrNew needs to create a new one.
	{
		tr5, ctx5 := FromContextOrNew(
			context.Background(), "TestContext", "tr5")
		if tr, _ := FromContext(ctx5); tr != tr5 {
			t.Errorf("Context with trace returned the wrong trace: %v != %v",
				tr, tr5)
		}
		tr5.Finish()
	}

	// Child from a context that has no trace attached.
	{
		tr6 := ChildFromContext(
			context.Background(), "TestContext", "child")
		tr6.Finish()
		if p := tr6.(*trace).Parent; p != nil {
			t.Errorf("Expected orphan trace, it has a parent: %v", p)
		}
	}
}
