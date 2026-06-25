// Package prompts contains the prompt templates used by the L2.3
// sector-agent DriverAdapter (internal/llm/llm_driver_adapter.go).
//
// The templates instruct the LLM to return a specific JSON structure
// that the adapter parses into orchestrator.PlanStep. The format
// must stay in sync with the parser in llm_driver_adapter.go.
package prompts

import "fmt"

// PlanTemplate is the JSON structure the LLM must return for the
// plan phase. The adapter parses this into []orchestrator.PlanStep.
const PlanTemplate = `Return your plan as a JSON object with this exact structure:
{
  "steps": [
    {"kind": "tool", "tool_name": "<name>", "args": {...}},
    {"kind": "thought", "note": "<reasoning>"}
  ]
}

Each step is either:
- A "tool" invocation: kind="tool", tool_name=<registered tool name>, args=<JSON object>
- A "thought" note: kind="thought", note=<free-form reasoning>

Available tools:
- get_factor_weight: Get current factor weights
- get_regime: Get current market regime
- get_liquidity: Get current liquidity metrics

Return ONLY the JSON object, no other text.`

// PlanPrompt returns the full prompt for the plan phase, including
// the skill and symbol context plus the expected response format.
func PlanPrompt(skill, symbol string) string {
	return fmt.Sprintf(
		`You are a sector agent for %q. Generate a plan for analyzing %q.

%s`,
		skill, symbol, PlanTemplate,
	)
}
