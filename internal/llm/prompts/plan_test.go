package prompts

import (
	"strings"
	"testing"
)

// TestPlanTemplate_mentionsKeySchema verifies the LLM-facing JSON schema
// stays in sync with the parser in internal/llm/llm_driver_adapter.go.
// The DriverAdapter parses steps into orchestrator.PlanStep whose
// "kind" discriminator must be "tool" or "thought" — if these tokens
// disappear from the template, downstream parsing silently breaks.
func TestPlanTemplate_mentionsKeySchema(t *testing.T) {
	for _, want := range []string{
		`"steps"`,
		`"kind"`,
		`"tool"`,
		`"thought"`,
		`"tool_name"`,
		`"args"`,
		`"note"`,
	} {
		if !strings.Contains(PlanTemplate, want) {
			t.Errorf("PlanTemplate missing required schema token %q — orchestrator parser will reject LLM output", want)
		}
	}
}

// TestPlanTemplate_listsAvailableTools ensures the agent knows which
// tools it can call. Removing a tool without updating the template
// causes the LLM to invent tool calls the orchestrator cannot dispatch.
func TestPlanTemplate_listsAvailableTools(t *testing.T) {
	for _, tool := range []string{"get_factor_weight", "get_regime", "get_liquidity"} {
		if !strings.Contains(PlanTemplate, tool) {
			t.Errorf("PlanTemplate missing registered tool %q", tool)
		}
	}
}

func TestPlanPrompt_injectsContextAndTemplate(t *testing.T) {
	p := PlanPrompt("semiconductor", "2330")

	// Both context variables must appear in the rendered prompt.
	if !strings.Contains(p, "semiconductor") {
		t.Error("PlanPrompt missing skill context")
	}
	if !strings.Contains(p, "2330") {
		t.Error("PlanPrompt missing symbol context")
	}
	// The template must be embedded verbatim so the LLM sees the schema.
	if !strings.Contains(p, PlanTemplate) {
		t.Error("PlanPrompt must embed PlanTemplate so the LLM sees the schema")
	}
	// JSON-only contract: must explicitly forbid surrounding prose.
	if !strings.Contains(p, "Return ONLY the JSON object") {
		t.Error("PlanPrompt must enforce JSON-only output to keep parser strict")
	}
}

func TestReflectTemplate_mentionsKeySchema(t *testing.T) {
	for _, want := range []string{
		`"continue"`,
		`"final_conviction"`,
		`"reasoning"`,
	} {
		if !strings.Contains(ReflectTemplate, want) {
			t.Errorf("ReflectTemplate missing required schema token %q", want)
		}
	}
	// final_conviction must be bounded — orchestrator trusts the range.
	if !strings.Contains(ReflectTemplate, "[0, 100]") {
		t.Error("ReflectTemplate must declare final_conviction range [0, 100]")
	}
}

func TestReflectPrompt_injectsAllFields(t *testing.T) {
	p := ReflectPrompt("semiconductor", "2330", `{"close": 1234.5}`)

	for _, want := range []string{
		"semiconductor",     // skill
		"2330",              // symbol
		`{"close": 1234.5}`, // tool result must be embedded as-is
		ReflectTemplate,     // schema must be present
	} {
		if !strings.Contains(p, want) {
			t.Errorf("ReflectPrompt missing %q", want)
		}
	}
}
