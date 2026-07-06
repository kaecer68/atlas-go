package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// mockProvider is a minimal llm.ProviderImpl for adapter tests.
// Returns the canned response and error configured at construction.
type mockProvider struct {
	resp llm.Response
	err  error
}

func (m *mockProvider) Supports(_ llm.Capability) bool { return true }
func (m *mockProvider) Call(_ context.Context, _ llm.Request) (llm.Response, error) {
	return m.resp, m.err
}

func (m *mockProvider) Health() llm.HealthStatus {
	return llm.HealthStatus{
		Provider:    llm.ProviderMock,
		Healthy:     true,
		LastSuccess: time.Now(),
	}
}

// TestParsePlanResponse_Valid covers the happy path: a clean JSON
// response with one tool step and one thought step.
func TestParsePlanResponse_Valid(t *testing.T) {
	input := `{
		"steps": [
			{"kind": "tool", "tool_name": "get_factor_weight", "args": {"symbol": "2330"}},
			{"kind": "thought", "note": "Need more data before committing."}
		]
	}`
	steps, err := ParsePlanResponse(input)
	if err != nil {
		t.Fatalf("ParsePlanResponse: unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if steps[0].Kind != "tool" || steps[0].ToolName != "get_factor_weight" {
		t.Errorf("steps[0] = %+v, want tool/get_factor_weight", steps[0])
	}
	if steps[0].Args["symbol"] != "2330" {
		t.Errorf("steps[0].Args[symbol] = %v, want 2330", steps[0].Args["symbol"])
	}
	if steps[1].Kind != "thought" || steps[1].Note == "" {
		t.Errorf("steps[1] = %+v, want thought with note", steps[1])
	}
}

// TestParsePlanResponse_MarkdownFenced covers the common case
// where the LLM wraps the JSON in ```json ... ``` fences.
func TestParsePlanResponse_MarkdownFenced(t *testing.T) {
	input := "```json\n" + `{"steps":[{"kind":"tool","tool_name":"get_regime","args":{}}]}` + "\n```"
	steps, err := ParsePlanResponse(input)
	if err != nil {
		t.Fatalf("ParsePlanResponse: unexpected error: %v", err)
	}
	if len(steps) != 1 || steps[0].ToolName != "get_regime" {
		t.Errorf("steps = %+v, want 1 step with tool_name=get_regime", steps)
	}
}

// TestParsePlanResponse_PlainFence covers the case where the
// fence has no language tag (just ```).
func TestParsePlanResponse_PlainFence(t *testing.T) {
	input := "```\n" + `{"steps":[{"kind":"thought","note":"x"}]}` + "\n```"
	steps, err := ParsePlanResponse(input)
	if err != nil {
		t.Fatalf("ParsePlanResponse: unexpected error: %v", err)
	}
	if len(steps) != 1 || steps[0].Note != "x" {
		t.Errorf("steps = %+v, want 1 thought step with note=x", steps)
	}
}

// TestParsePlanResponse_Malformed verifies that invalid JSON
// produces a wrapped error.
func TestParsePlanResponse_Malformed(t *testing.T) {
	_, err := ParsePlanResponse("not json at all")
	if err == nil {
		t.Fatal("ParsePlanResponse: expected error for malformed input, got nil")
	}
	if !strings.Contains(err.Error(), "parse plan response") {
		t.Errorf("error should mention 'parse plan response', got: %v", err)
	}
}

// TestParsePlanResponse_EmptySteps verifies that a valid JSON
// with an empty steps array is rejected.
func TestParsePlanResponse_EmptySteps(t *testing.T) {
	_, err := ParsePlanResponse(`{"steps":[]}`)
	if err == nil {
		t.Fatal("ParsePlanResponse: expected error for empty steps, got nil")
	}
	if !strings.Contains(err.Error(), "empty steps") {
		t.Errorf("error should mention 'empty steps', got: %v", err)
	}
}

// TestParsePlanResponse_InvalidKind verifies that a step with
// an unknown kind is rejected.
func TestParsePlanResponse_InvalidKind(t *testing.T) {
	_, err := ParsePlanResponse(`{"steps":[{"kind":"unknown","tool_name":"x"}]}`)
	if err == nil {
		t.Fatal("ParsePlanResponse: expected error for invalid kind, got nil")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error should mention 'invalid kind', got: %v", err)
	}
}

// TestParsePlanResponse_ToolWithoutName verifies that a tool step
// without a tool_name is rejected.
func TestParsePlanResponse_ToolWithoutName(t *testing.T) {
	_, err := ParsePlanResponse(`{"steps":[{"kind":"tool","args":{}}]}`)
	if err == nil {
		t.Fatal("ParsePlanResponse: expected error for tool without tool_name, got nil")
	}
	if !strings.Contains(err.Error(), "tool_name is empty") {
		t.Errorf("error should mention 'tool_name is empty', got: %v", err)
	}
}

// TestParseReflectResponse_Valid covers the happy path.
func TestParseReflectResponse_Valid(t *testing.T) {
	input := `{"continue": true, "final_conviction": 75, "reasoning": "Need more data"}`
	ref, err := ParseReflectResponse(input)
	if err != nil {
		t.Fatalf("ParseReflectResponse: unexpected error: %v", err)
	}
	if !ref.Continue {
		t.Error("ref.Continue = false, want true")
	}
	if ref.FinalConviction != 75 {
		t.Errorf("ref.FinalConviction = %d, want 75", ref.FinalConviction)
	}
	if ref.Reasoning != "Need more data" {
		t.Errorf("ref.Reasoning = %q, want 'Need more data'", ref.Reasoning)
	}
}

// TestParseReflectResponse_ContinueFalse covers the
// commit-final-recommendation case.
func TestParseReflectResponse_ContinueFalse(t *testing.T) {
	input := `{"continue": false, "final_conviction": 90, "reasoning": "Done"}`
	ref, err := ParseReflectResponse(input)
	if err != nil {
		t.Fatalf("ParseReflectResponse: unexpected error: %v", err)
	}
	if ref.Continue {
		t.Error("ref.Continue = true, want false")
	}
	if ref.FinalConviction != 90 {
		t.Errorf("ref.FinalConviction = %d, want 90", ref.FinalConviction)
	}
}

// TestParseReflectResponse_MarkdownFenced covers the fenced case.
func TestParseReflectResponse_MarkdownFenced(t *testing.T) {
	input := "```json\n" + `{"continue":true,"final_conviction":50,"reasoning":"x"}` + "\n```"
	ref, err := ParseReflectResponse(input)
	if err != nil {
		t.Fatalf("ParseReflectResponse: unexpected error: %v", err)
	}
	if ref.FinalConviction != 50 {
		t.Errorf("ref.FinalConviction = %d, want 50", ref.FinalConviction)
	}
}

// TestParseReflectResponse_Malformed verifies error wrapping.
func TestParseReflectResponse_Malformed(t *testing.T) {
	_, err := ParseReflectResponse("garbage")
	if err == nil {
		t.Fatal("ParseReflectResponse: expected error for malformed input, got nil")
	}
	if !strings.Contains(err.Error(), "parse reflect response") {
		t.Errorf("error should mention 'parse reflect response', got: %v", err)
	}
}

// TestParseReflectResponse_OutOfRange verifies that
// final_conviction outside [0,100] is rejected.
func TestParseReflectResponse_OutOfRange(t *testing.T) {
	_, err := ParseReflectResponse(`{"continue":false,"final_conviction":150,"reasoning":"x"}`)
	if err == nil {
		t.Fatal("ParseReflectResponse: expected error for out-of-range conviction, got nil")
	}
	if !strings.Contains(err.Error(), "out of [0,100]") {
		t.Errorf("error should mention 'out of [0,100]', got: %v", err)
	}
}

// TestDriverAdapter_PlanComplete_HappyPath verifies the full
// pipeline: adapter calls provider, gets response, parses steps.
func TestDriverAdapter_PlanComplete_HappyPath(t *testing.T) {
	provider := &mockProvider{
		resp: llm.Response{
			Output: `{"steps":[{"kind":"tool","tool_name":"get_factor_weight","args":{"symbol":"2330"}}]}`,
		},
	}
	adapter := NewDriverAdapter(provider)
	steps, err := adapter.PlanComplete(context.Background(), "semiconductor", "2330")
	if err != nil {
		t.Fatalf("PlanComplete: unexpected error: %v", err)
	}
	if len(steps) != 1 || steps[0].ToolName != "get_factor_weight" {
		t.Errorf("steps = %+v, want 1 tool step", steps)
	}
}

// TestDriverAdapter_PlanComplete_ProviderError verifies that
// provider errors are wrapped.
func TestDriverAdapter_PlanComplete_ProviderError(t *testing.T) {
	provider := &mockProvider{err: errors.New("provider down")}
	adapter := NewDriverAdapter(provider)
	_, err := adapter.PlanComplete(context.Background(), "semiconductor", "2330")
	if err == nil {
		t.Fatal("PlanComplete: expected error from provider, got nil")
	}
	if !strings.Contains(err.Error(), "provider call") {
		t.Errorf("error should mention 'provider call', got: %v", err)
	}
}

// TestDriverAdapter_PlanComplete_ParseError verifies that
// malformed provider responses surface as parse errors.
func TestDriverAdapter_PlanComplete_ParseError(t *testing.T) {
	provider := &mockProvider{resp: llm.Response{Output: "not json"}}
	adapter := NewDriverAdapter(provider)
	_, err := adapter.PlanComplete(context.Background(), "semiconductor", "2330")
	if err == nil {
		t.Fatal("PlanComplete: expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "PlanComplete") {
		t.Errorf("error should mention PlanComplete, got: %v", err)
	}
}

// TestDriverAdapter_ReflectComplete_HappyPath verifies the
// reflect pipeline.
func TestDriverAdapter_ReflectComplete_HappyPath(t *testing.T) {
	provider := &mockProvider{
		resp: llm.Response{
			Output: `{"continue":false,"final_conviction":80,"reasoning":"Done"}`,
		},
	}
	adapter := NewDriverAdapter(provider)
	ref, err := adapter.ReflectComplete(context.Background(), "semiconductor", "2330", "tool output")
	if err != nil {
		t.Fatalf("ReflectComplete: unexpected error: %v", err)
	}
	if ref.Continue {
		t.Error("ref.Continue = true, want false")
	}
	if ref.FinalConviction != 80 {
		t.Errorf("ref.FinalConviction = %d, want 80", ref.FinalConviction)
	}
}

// TestDriverAdapter_ReflectComplete_ProviderError verifies error
// wrapping for reflect.
func TestDriverAdapter_ReflectComplete_ProviderError(t *testing.T) {
	provider := &mockProvider{err: errors.New("rate limited")}
	adapter := NewDriverAdapter(provider)
	_, err := adapter.ReflectComplete(context.Background(), "semiconductor", "2330", "x")
	if err == nil {
		t.Fatal("ReflectComplete: expected error from provider, got nil")
	}
	if !strings.Contains(err.Error(), "provider call") {
		t.Errorf("error should mention 'provider call', got: %v", err)
	}
}

// TestStripMarkdownFences covers the helper directly.
func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no fence", `{"a":1}`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"plain fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"leading whitespace", "  \n{\"a\":1}\n  ", `{"a":1}`},
		{"single-line fence", "```{\"a\":1}```", `{"a":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripMarkdownFences(tc.in)
			if got != tc.want {
				t.Errorf("stripMarkdownFences(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
