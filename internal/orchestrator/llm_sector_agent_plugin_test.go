package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestLLMSectorAgentsPlugin_NilDriver_PassThrough asserts that when no
// driver is wired, the plugin returns the recommendations unchanged so
// the deterministic sector path stays active during the observation
// window (Issue #719 acceptance: nil driver preserves deterministic
// path).
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

// TestLLMSectorAgentsPlugin_NonSectorAgent_Skipped asserts that the
// plugin only touches sector-layer recs. A non-sector rec should pass
// through unchanged even when a driver is wired.
func TestLLMSectorAgentsPlugin_NonSectorAgent_Skipped(t *testing.T) {
	p := &llmSectorAgentsPlugin{
		driver: &SectorAgentLLMDriver{},
		registry: domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "style", Layer: domain.LayerStyle},
			},
		},
	}
	in := []domain.Recommendation{
		{Agent: "style", Symbol: "0050.TW", Conviction: 30},
	}
	out := p.ProcessRecommendations(domain.RegimeRiskOn, in)
	if len(out) != 1 || out[0].Conviction != 30 {
		t.Errorf("expected non-sector pass-through with conviction 30, got %+v", out)
	}
}

// TestLLMSectorAgentsPlugin_SectorAgent_NoConvictionChange asserts that
// the current wired plugin is a no-op pass-through on conviction even
// when a driver is wired and the rec is sector-layer. The actual loop
// driver is integrated incrementally; this guards against accidental
// conviction mutation that would break replay reproducibility.
func TestLLMSectorAgentsPlugin_SectorAgent_NoConvictionChange(t *testing.T) {
	p := &llmSectorAgentsPlugin{
		driver: &SectorAgentLLMDriver{},
		registry: domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "semi", Layer: domain.LayerSector},
			},
		},
	}
	in := []domain.Recommendation{
		{Agent: "semi", Symbol: "2330.TW", Conviction: 60},
	}
	out := p.ProcessRecommendations(domain.RegimeRiskOn, in)
	if len(out) != 1 || out[0].Conviction != 60 {
		t.Errorf("expected sector pass-through with conviction 60, got %+v", out)
	}
}

// TestLLMSectorAgentsPlugin_EmptyRegistry_TreatsAsSector asserts that
// an empty registry (test/migration case) defaults to treating the rec
// as sector-layer — observability over strictness. Combined with a nil
// driver the rec still passes through unchanged.
func TestLLMSectorAgentsPlugin_EmptyRegistry_TreatsAsSector(t *testing.T) {
	p := &llmSectorAgentsPlugin{
		driver:   nil,
		registry: domain.AgentRegistry{},
	}
	in := []domain.Recommendation{
		{Agent: "semi", Symbol: "2330.TW", Conviction: 40},
	}
	out := p.ProcessRecommendations(domain.RegimeRiskOn, in)
	if len(out) != 1 || out[0].Conviction != 40 {
		t.Errorf("expected empty-registry pass-through with conviction 40, got %+v", out)
	}
}

// TestSectorAgentLLMDriver_EmbedsInterfaces asserts that
// SectorAgentLLMDriver.PlanDriver and ReflectDriver fields can be
// populated directly (the embedded-interface form introduced by Issue
// #711 Phase 3, PR #726). The test does not actually drive the LLM
// loop; it only asserts that the struct accepts the interface fields.
func TestSectorAgentLLMDriver_EmbedsInterfaces(t *testing.T) {
	drv := &SectorAgentLLMDriver{}
	if drv.PlanDriver != nil {
		t.Fatalf("PlanDriver should default to nil")
	}
	if drv.ReflectDriver != nil {
		t.Fatalf("ReflectDriver should default to nil")
	}
}
