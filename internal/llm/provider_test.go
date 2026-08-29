package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// TestSafeInvokeHandler_ContextCancelled verifies that a cancelled
// context propagates to the handler and the returned error wraps
// context.Canceled. Issue #711 #4 follow-up: context cancellation
// must be detectable from the handler's error so callers can
// distinguish cancellation from handler failure.
func TestSafeInvokeHandler_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invocation

	tool := Tool{
		Name: "ctx_cancelled_tool",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	_, err := SafeInvokeHandler(ctx, &tool, json.RawMessage(`{}`))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SafeInvokeHandler with cancelled ctx: got err=%v, want context.Canceled", err)
	}
}

// TestSafeInvokeHandler_ContextDeadlineExceeded verifies that a
// deadline-exceeded context is detectable from the returned error.
func TestSafeInvokeHandler_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tool := Tool{
		Name: "deadline_tool",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	_, err := SafeInvokeHandler(ctx, &tool, json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("SafeInvokeHandler with deadline-exceeded ctx: got err=%v, want context.DeadlineExceeded", err)
	}
}

// TestSafeInvokeHandler_ContextNotCancelled verifies the happy path:
// when the context is not cancelled, the handler runs to completion
// and the result is returned.
func TestSafeInvokeHandler_ContextNotCancelled(t *testing.T) {
	ctx := t.Context()

	tool := Tool{
		Name: "happy_path_tool",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"result":"ok"}`), nil
		},
	}

	got, err := SafeInvokeHandler(ctx, &tool, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("SafeInvokeHandler with non-cancelled ctx: unexpected error: %v", err)
	}
	if string(got) != `{"result":"ok"}` {
		t.Errorf("result = %q, want {\"result\":\"ok\"}", string(got))
	}
}
