package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// FuzzHandlerArgs fuzz-tests the SafeInvokeHandler + BindTypedArgs
// pipeline with arbitrary JSON inputs. The fuzzer should NEVER
// trigger an unhandled panic — SafeInvokeHandler's recover() must
// catch all panics. Issue #711 test bar: ≥1 fuzz test (T2 fix
// from plan v2).
//
// Seed corpus covers the common cases plus known-malicious patterns
// (prototype pollution attempts, deeply nested objects, unicode
// tricks, oversized strings) to give the fuzzer a good starting point.
//
// Run with: go test -fuzz=FuzzHandlerArgs -fuzztime=10s ./internal/llm/
func FuzzHandlerArgs(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"city":"Taipei"}`,
		`{"__proto__":{"polluted":true}}`,
		`{"constructor":{"prototype":{"polluted":true}}}`,
		`null`,
		`42`,
		`"string"`,
		`[1,2,3]`,
		`{"unicode":"` + "\x00\x01\x02" + `"}`,
		`{"nested":{"deep":{"deeper":{"deepest":null}}}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, args string) {
		factory := func() map[string]any { return map[string]any{} }
		handler := func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		}
		bound := BindTypedArgs("fuzz_target", factory, handler)
		tool := Tool{
			Name:    "fuzz_target",
			Handler: bound,
		}
		// Must never panic — SafeInvokeHandler's recover() guarantees this.
		result, err := SafeInvokeHandler(context.Background(), &tool, json.RawMessage(args))
		if err != nil {
			// Errors are expected for malformed JSON; verify the error
			// is wrapped with the tool name for traceability.
			if !strings.Contains(err.Error(), "fuzz_target") {
				t.Errorf("error should mention tool name 'fuzz_target', got: %v", err)
			}
		}
		// result is either valid JSON or nil on error — both acceptable.
		_ = result
	})
}
