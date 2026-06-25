package orchestrator

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestBaseConvictionDriver_PlanComplete_DefaultArgs asserts the deterministic
// LLMDriver stub returns a single-step plan carrying the input skill and
// symbol so the loop driver can detect sector context.
func TestBaseConvictionDriver_PlanComplete_DefaultArgs(t *testing.T) {
	drv := NewBaseConvictionDriver()
	steps, err := drv.PlanComplete(context.Background(), "semiconductor", "2330.TW")
	if err != nil {
		t.Fatalf("PlanComplete error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 plan step, got %d", len(steps))
	}
	if steps[0].Kind != "thought" {
		t.Errorf("expected Kind=thought, got %q", steps[0].Kind)
	}
	if steps[0].Args["skill"] != "semiconductor" {
		t.Errorf("expected skill arg, got %v", steps[0].Args["skill"])
	}
	if steps[0].Args["symbol"] != "2330.TW" {
		t.Errorf("expected symbol arg, got %v", steps[0].Args["symbol"])
	}
}

// TestBaseConvictionDriver_ReflectComplete_Converges asserts the reflection
// always signals Continue=false so the loop converges in one iteration.
func TestBaseConvictionDriver_ReflectComplete_Converges(t *testing.T) {
	drv := NewBaseConvictionDriver()
	refl, err := drv.ReflectComplete(context.Background(), "etf", "0050.TW", "stub result")
	if err != nil {
		t.Fatalf("ReflectComplete error = %v", err)
	}
	if refl.Continue {
		t.Errorf("expected Continue=false (deterministic convergence), got true")
	}
}

// TestRunSectorAgentLoop_NilDriver_ReturnsError asserts that the loop
// refuses to run without a wired driver (Issue #719 acceptance: nil driver
// preserves deterministic path).
func TestRunSectorAgentLoop_NilDriver_ReturnsError(t *testing.T) {
	agent := &SectorAgentLLM{AgentLoop: NewAgentLoop(3), Skill: "semi"}
	_, err := RunSectorAgentLoop(context.Background(), agent, "2330.TW", domain.Recommendation{Agent: "semi", Conviction: 50})
	if err == nil {
		t.Fatalf("expected ErrNotImplemented when driver is nil")
	}
}

// TestRunSectorAgentLoop_BaseDriver_RunsOnce asserts that with the
// deterministic BaseConvictionDriver the loop runs once and returns the
// base conviction (since reflection does not modify it).
func TestRunSectorAgentLoop_BaseDriver_RunsOnce(t *testing.T) {
	agent := &SectorAgentLLM{
		AgentLoop:       NewAgentLoop(3),
		Skill:           "semi",
		LLM:             NewBaseConvictionDriver(),
		ConvictionFloor: 60,
	}
	got, err := RunSectorAgentLoop(context.Background(), agent, "2330.TW", domain.Recommendation{Agent: "semi", Conviction: 50})
	if err != nil {
		t.Fatalf("RunSectorAgentLoop error = %v", err)
	}
	if got != 60 {
		t.Errorf("expected ConvictionFloor=60, got %d", got)
	}
}

// TestLLMSectorAgentsPlugin_NilDriver_PassThrough asserts that when no
// LLMDriver is wired, the plugin returns the recommendations unchanged so
// the deterministic sector path stays active (Issue #719 acceptance).
func TestLLMSectorAgentsPlugin_NilDriver_PassThrough(t *testing.T) {
	p := &llmSectorAgentsPlugin{}
	in := []domain.Recommendation{
		{Agent: "semi", Symbol: "2330.TW", Conviction: 50},
	}
	out := p.ProcessRecommendations(domain.RegimeRiskOn, in)
	if len(out) != 1 || out[0].Conviction != 50 {
		t.Errorf("expected pass-through with conviction 50, got %+v", out)
	}
}

// TestLLMSectorAgentsPlugin_BaseDriver_AdjustsConviction asserts that when
// the BaseConvictionDriver is wired AND a recommendation is sector-layer,
// the loop's reflection preserves the ConvictionFloor (deterministic).
func TestLLMSectorAgentsPlugin_BaseDriver_AdjustsConviction(t *testing.T) {
	p := &llmSectorAgentsPlugin{
		driver: NewBaseConvictionDriver(),
	}
	in := []domain.Recommendation{
		{Agent: "semi", Symbol: "2330.TW", Conviction: 40},
	}
	out := p.ProcessRecommendations(domain.RegimeRiskOn, in)
	if len(out) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(out))
	}
	// BaseConvictionDriver sets ConvictionFloor via the SectorAgentLLM
	// struct when RunSectorAgentLoop is called. ConvictionFloor defaults
	// to 0 in our test, so the reflection result (FinalConviction=0)
	// keeps the original conviction unchanged.
	if out[0].Conviction != 40 {
		t.Errorf("expected unchanged conviction 40 (BaseConvictionDriver no-op), got %d", out[0].Conviction)
	}
}
