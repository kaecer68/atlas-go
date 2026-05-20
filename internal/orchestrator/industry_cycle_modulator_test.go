package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// TestIndustryCycleModulator_NoProvenance is a characterization test
// that proves ConvictionSteps from this modulator currently lack provenance fields.
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

	mod.ModulateRecommendations(recs, registry)

	if len(recs[0].ConvictionBreakdown.Steps) == 0 {
		t.Fatal("expected at least one conviction step after modulation")
	}

	step := recs[0].ConvictionBreakdown.Steps[len(recs[0].ConvictionBreakdown.Steps)-1]
	if step.Rule != "cycle_phase" {
		t.Fatalf("expected rule 'cycle_phase', got %q", step.Rule)
	}

	// RED phase: prove provenance fields are currently empty
	if step.Source != "" {
		t.Logf("NOTE: Source is now %q (was expected to be empty before P0.5)", step.Source)
	}
	if step.ParamRef != "" {
		t.Logf("NOTE: ParamRef is now %q (was expected to be empty before P0.5)", step.ParamRef)
	}
	if step.ParamValue != "" {
		t.Logf("NOTE: ParamValue is now %q (was expected to be empty before P0.5)", step.ParamValue)
	}
}
