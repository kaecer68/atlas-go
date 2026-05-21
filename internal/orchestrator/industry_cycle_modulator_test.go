package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// TestIndustryCycleModulator_NoProvenance validates that the industry cycle
// modulator produces correct ConvictionSteps with provenance fields populated.
func TestIndustryCycleModulator_NoProvenance(t *testing.T) {
	tracker := industry.NewCycleTracker()
	tracker.UpdatePosition("semiconductor", industry.IndustryMetrics{
		IndustryID:          "semiconductor",
		RevenueGrowthYoY:    0.25,
		ProfitGrowthYoY:     0.30,
		InventoryTurnover:   5.5,
		CapacityUtilization: 0.85,
	})

	mod := NewIndustryCycleModulator(tracker)

	recs := []domain.Recommendation{
		{Agent: "agent1", Conviction: 50, ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 50, Floor: 40, Final: 50, Steps: []domain.ConvictionStep{}}},
	}
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{{ID: "agent1", Skill: "semiconductor_desk"}},
	}

	steps := mod.CollectModulationSteps(recs, registry)
	if len(steps) == 0 {
		t.Fatal("expected at least one modulation step")
	}
	for _, ms := range steps {
		if ms.RecIndex >= len(recs) {
			continue
		}
		for _, step := range ms.Steps {
			recs[ms.RecIndex].Conviction += step.Delta
			if recs[ms.RecIndex].ConvictionBreakdown != nil {
				recs[ms.RecIndex].ConvictionBreakdown.Steps = append(recs[ms.RecIndex].ConvictionBreakdown.Steps, step)
				recs[ms.RecIndex].ConvictionBreakdown.Final = recs[ms.RecIndex].Conviction
			}
		}
	}

	if len(recs[0].ConvictionBreakdown.Steps) == 0 {
		t.Fatal("expected at least one conviction step after modulation")
	}

	step := recs[0].ConvictionBreakdown.Steps[len(recs[0].ConvictionBreakdown.Steps)-1]
	if step.Rule != "modulator:industry_cycle:cycle_phase" {
		t.Fatalf("expected rule 'modulator:industry_cycle:cycle_phase', got %q", step.Rule)
	}

	if step.Source == "" {
		t.Error("expected Source to be populated")
	}
	if step.ParamRef == "" {
		t.Error("expected ParamRef to be populated")
	}
	if step.ParamValue == "" {
		t.Error("expected ParamValue to be populated")
	}
}
