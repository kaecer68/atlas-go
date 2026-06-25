package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestBindTypedArgs_MalformedJSON_EdgeCases covers malformed JSON
// inputs that BindTypedArgs should reject gracefully. The basic
// unmarshal-error case is in invocation_test.go; this file extends
// coverage to edge cases (empty input, wrong root type, nested
// truncation, oversized payloads, etc.).
func TestBindTypedArgs_MalformedJSON_EdgeCases(t *testing.T) {
	type weatherArgs struct {
		City string `json:"city"`
	}
	factory := func() weatherArgs { return weatherArgs{} }
	handler := func(_ context.Context, _ weatherArgs) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	}

	tests := []struct {
		name string
		args string
	}{
		{"empty input", ""},
		{"plain text", "not json at all"},
		{"truncated object", `{"city":`},
		{"wrong root type (array)", `["a","b"]`},
		{"wrong root type (number)", `42`},
		{"nested truncation", `{"outer":{"inner":`},
		{"invalid escape", `{"city":"\x"}`},
		// Genuinely malformed: huge key followed by truncation. Exercises
		// the unmarshal path with a 50KB payload to catch memory/buffer
		// issues in the JSON decoder, not just shape mismatches.
		{"huge truncated key", `{"` + strings.Repeat("x", 50000) + `":`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bound := BindTypedArgs("fuzz_target", factory, handler)
			_, err := bound(context.Background(), json.RawMessage(tc.args))
			if err == nil {
				t.Fatalf("expected unmarshal error for %q, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), "typed args unmarshal") {
				t.Errorf("error should mention 'typed args unmarshal', got: %v", err)
			}
			if !strings.Contains(err.Error(), "fuzz_target") {
				t.Errorf("error should mention tool name 'fuzz_target', got: %v", err)
			}
		})
	}
}

// TestBindTypedArgs_HandlerError_Wrapped verifies that errors from
// the typed handler are wrapped with the tool name for traceability
// AND the original error remains unwrappable via errors.Is.
func TestBindTypedArgs_HandlerError_Wrapped(t *testing.T) {
	factory := func() map[string]any { return map[string]any{} }
	sentinel := errors.New("handler-specific failure")
	handler := func(_ context.Context, _ map[string]any) (map[string]any, error) {
		return nil, sentinel
	}

	bound := BindTypedArgs("my_tool", factory, handler)
	_, err := bound(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected handler error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap the original handler error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "my_tool") {
		t.Errorf("error should mention tool name 'my_tool', got: %v", err)
	}
}

// TestBindTypedArgs_MarshalError_TriggeredIndirectly documents the
// typed-result marshal path: a typed Out type that fails json.Marshal
// (e.g. channel, function) will produce a wrapped marshal error.
// This is a defensive test — production code should not use such
// Out types, but the binding must fail gracefully if it happens.
func TestBindTypedArgs_MarshalError_TriggeredIndirectly(t *testing.T) {
	factory := func() map[string]any { return map[string]any{} }
	// Channels are not JSON-marshalable
	handler := func(_ context.Context, _ map[string]any) (chan int, error) {
		ch := make(chan int, 1)
		ch <- 42
		return ch, nil
	}

	bound := BindTypedArgs("bad_out", factory, handler)
	_, err := bound(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected marshal error for chan Out type, got nil")
	}
	if !strings.Contains(err.Error(), "typed result marshal") {
		t.Errorf("error should mention 'typed result marshal', got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad_out") {
		t.Errorf("error should mention tool name 'bad_out', got: %v", err)
	}
}
