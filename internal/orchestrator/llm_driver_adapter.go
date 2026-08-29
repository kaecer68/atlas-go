package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/prompts"
)

// DriverAdapter implements PlanDriver and ReflectDriver by
// delegating to a concrete llm.ProviderImpl and parsing the
// textual response into structured types. It is the production
// wiring for the L2.3 sector-agent plan/reflect loop (plan v2
// PR5a).
//
// File location: this adapter lives in the orchestrator package
// (not internal/llm) because it returns orchestrator.PlanStep and
// orchestrator.Reflection. Placing it in internal/llm would
// create an import cycle: llm → orchestrator (for the types) →
// llm (via sector_agent_llm.go for llm.Tool). This is a known
// deviation from plan v2's nominal location.
//
// Protocol contract:
//   - PlanComplete: provider returns text matching prompts.PlanTemplate
//     (a JSON object with "steps" array). Parsed into
//     []PlanStep.
//   - ReflectComplete: provider returns text matching
//     prompts.ReflectTemplate (a JSON object with "continue",
//     "final_conviction", "reasoning"). Parsed into Reflection.
//
// The adapter does not enforce llm.Request.Validate() — callers
// should do so before dispatch. The adapter trusts the provider's
// response shape (per Issue #711 #11).
type DriverAdapter struct {
	provider llm.ProviderImpl
}

// NewDriverAdapter wraps a concrete LLM ProviderImpl as a
// plan/reflect driver for SectorAgentLLM.
func NewDriverAdapter(provider llm.ProviderImpl) *DriverAdapter {
	return &DriverAdapter{provider: provider}
}

// PlanComplete satisfies PlanDriver. Sends a plan prompt to the
// underlying provider and parses the textual response into
// []PlanStep. The prompt is built via prompts.PlanPrompt so the
// JSON format specification and skill/symbol context live in one
// place.
func (a *DriverAdapter) PlanComplete(ctx context.Context, skill, symbol string) ([]PlanStep, error) {
	prompt := prompts.PlanPrompt(skill, symbol)
	req := llm.Request{
		Capability: llm.CapabilityRationaleGeneration,
		Payload:    prompt,
		DataClass:  llm.DataClassNonRegulated,
		Options:    llm.Options{Timeout: 30_000_000_000}, // 30s
	}
	resp, err := a.provider.Call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("DriverAdapter.PlanComplete: provider call: %w", err)
	}
	steps, err := ParsePlanResponse(resp.Output)
	if err != nil {
		return nil, fmt.Errorf("DriverAdapter.PlanComplete: %w", err)
	}
	return steps, nil
}

// ReflectComplete satisfies ReflectDriver. Sends a reflect prompt
// to the underlying provider and parses the textual response
// into Reflection. The prompt is built via prompts.ReflectPrompt.
func (a *DriverAdapter) ReflectComplete(ctx context.Context, skill, symbol, toolResult string) (Reflection, error) {
	prompt := prompts.ReflectPrompt(skill, symbol, toolResult)
	req := llm.Request{
		Capability: llm.CapabilityConfidenceCommentary,
		Payload:    prompt,
		DataClass:  llm.DataClassNonRegulated,
		Options:    llm.Options{Timeout: 30_000_000_000}, // 30s
	}
	resp, err := a.provider.Call(ctx, req)
	if err != nil {
		return Reflection{}, fmt.Errorf("DriverAdapter.ReflectComplete: provider call: %w", err)
	}
	ref, err := ParseReflectResponse(resp.Output)
	if err != nil {
		return Reflection{}, fmt.Errorf("DriverAdapter.ReflectComplete: %w", err)
	}
	return ref, nil
}

// planResponseJSON is the intermediate struct for parsing LLM
// plan responses. Mirrors prompts.PlanTemplate.
type planResponseJSON struct {
	Steps []planStepJSON `json:"steps"`
}

type planStepJSON struct {
	Kind     string         `json:"kind"`
	ToolName string         `json:"tool_name,omitempty"`
	Args     map[string]any `json:"args,omitempty"`
	Note     string         `json:"note,omitempty"`
}

// reflectResponseJSON is the intermediate struct for parsing LLM
// reflect responses. Mirrors prompts.ReflectTemplate.
type reflectResponseJSON struct {
	Continue        bool   `json:"continue"`
	FinalConviction int    `json:"final_conviction"`
	Reasoning       string `json:"reasoning"`
}

// ParsePlanResponse parses an LLM textual response into
// []PlanStep. Exported for direct unit testing.
//
// The LLM is instructed to return ONLY the JSON object, but
// defensive parsing strips surrounding markdown fences or
// leading/trailing whitespace.
func ParsePlanResponse(output string) ([]PlanStep, error) {
	cleaned := stripMarkdownFences(output)
	var resp planResponseJSON
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse plan response: %w", err)
	}
	if len(resp.Steps) == 0 {
		return nil, fmt.Errorf("parse plan response: empty steps array")
	}
	steps := make([]PlanStep, 0, len(resp.Steps))
	for i, s := range resp.Steps {
		if s.Kind != "tool" && s.Kind != "thought" {
			return nil, fmt.Errorf("parse plan response: step[%d] has invalid kind %q (want \"tool\" or \"thought\")", i, s.Kind)
		}
		if s.Kind == "tool" && s.ToolName == "" {
			return nil, fmt.Errorf("parse plan response: step[%d] is a tool but tool_name is empty", i)
		}
		steps = append(steps, PlanStep(s))
	}
	return steps, nil
}

// ParseReflectResponse parses an LLM textual response into
// Reflection. Exported for direct unit testing.
func ParseReflectResponse(output string) (Reflection, error) {
	cleaned := stripMarkdownFences(output)
	var resp reflectResponseJSON
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return Reflection{}, fmt.Errorf("parse reflect response: %w", err)
	}
	if resp.FinalConviction < 0 || resp.FinalConviction > 100 {
		return Reflection{}, fmt.Errorf("parse reflect response: final_conviction %d out of [0,100]", resp.FinalConviction)
	}
	return Reflection(resp), nil
}

// stripMarkdownFences removes leading/trailing whitespace and
// optional ```json / ``` fences that some LLM providers wrap
// around JSON responses.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```"); ok {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		} else {
			s = after
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
