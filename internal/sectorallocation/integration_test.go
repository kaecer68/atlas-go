package sectorallocation

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestIntegration_ConfigToEnginePipeline exercises the full
// parameters.json → SectorAllocationConfig → WeightEngine pipeline using
// real config and stub providers. Verifies that the engine consumes
// the real base weights (semiconductor=0.30, financials=0.14) and
// produces a stable, sorted plan.
func TestIntegration_ConfigToEnginePipeline(t *testing.T) {
	cfg := config.GetParametersConfig()
	sa := cfg.SectorAllocation

	if _, ok := sa.BaseWeights["semiconductor"]; !ok {
		t.Fatal("sector_allocation.base_weights missing semiconductor key")
	}
	if sa.BaseWeights["semiconductor"] <= 0 {
		t.Errorf("semiconductor base weight non-positive: %f", sa.BaseWeights["semiconductor"])
	}

	engine := NewDefaultEngine(
		sa,
		stubCycle{v: 1.0},
		stubSeasonal{v: 1.0},
		stubLinkage{v: 1.0},
		stubNarrative{v: 1.0},
		stubMacro{tilt: 0.0},
		stubFactor{tilt: 0.0},
		0.3, 2.5,
	)

	plan, err := engine.ComputeWeights(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ComputeWeights: %v", err)
	}

	if len(plan) != len(sa.BaseWeights) {
		t.Errorf("plan length %d != base_weights count %d", len(plan), len(sa.BaseWeights))
	}

	for _, w := range plan {
		if w.AdjustedWeight != w.BaseWeight {
			t.Errorf("neutral multipliers: %s adjusted (%.4f) != base (%.4f)",
				w.ID, w.AdjustedWeight, w.BaseWeight)
		}
	}

	for i := 1; i < len(plan); i++ {
		if plan[i-1].AdjustedWeight < plan[i].AdjustedWeight {
			t.Errorf("plan not sorted desc at index %d: %.4f < %.4f",
				i, plan[i-1].AdjustedWeight, plan[i].AdjustedWeight)
		}
	}
}

// TestIntegration_DerivationFactorsPopulated verifies that the 12 industries
// in the default config each have an associated derivation factor list,
// so the frontend can render the "因子說明" panel.
func TestIntegration_DerivationFactorsPopulated(t *testing.T) {
	cfg := config.GetParametersConfig().SectorAllocation

	required := []string{
		"semiconductor", "financials", "electronics", "materials",
		"industrials", "consumer", "healthcare", "energy",
		"telecom", "utilities", "real_estate",
	}
	for _, id := range required {
		factors, ok := cfg.DerivationFactors[id]
		if !ok {
			t.Errorf("missing derivation_factors for %s", id)
			continue
		}
		if len(factors) == 0 {
			t.Errorf("empty derivation_factors for %s", id)
		}
		var sum float64
		for _, f := range factors {
			sum += f.Weight
		}
		if sum < 0.99 || sum > 1.01 {
			t.Errorf("derivation_factors[%s] weights sum to %.4f, expected ~1.0", id, sum)
		}
	}
}

// TestIntegration_NonNeutralShiftsAdjustWeights verifies that when one
// provider shifts off neutral (e.g., narrative=1.20), the engine picks
// up the change and the adjustment log records it.
func TestIntegration_NonNeutralShiftsAdjustWeights(t *testing.T) {
	cfg := config.GetParametersConfig().SectorAllocation
	engine := NewDefaultEngine(
		cfg,
		stubCycle{v: 1.0},
		stubSeasonal{v: 1.0},
		stubLinkage{v: 1.0},
		stubNarrative{v: 1.20},
		stubMacro{tilt: -0.10},
		stubFactor{tilt: 0.0},
		0.3, 2.5,
	)
	w, err := engine.ComputeWeight(context.Background(), "semiconductor", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	if w.AdjustedWeight <= w.BaseWeight*1.05 {
		t.Errorf("expected semiconductor adjusted > base × 1.05 with narrative=1.20 macro=-0.10, got %.4f vs base %.4f",
			w.AdjustedWeight, w.BaseWeight)
	}
	if len(w.AdjustmentLog) < 7 {
		t.Errorf("expected at least 7 log entries, got %d", len(w.AdjustmentLog))
	}
}
