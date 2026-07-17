package sectorallocation

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

type stubCycle struct{ v float64 }

func (s stubCycle) GetCycleMultiplier(_ context.Context, _ string) (float64, error) {
	return s.v, nil
}

type stubSeasonal struct{ v float64 }

func (s stubSeasonal) GetSeasonalMultiplier(_ context.Context, _ string, _ time.Time) (float64, error) {
	return s.v, nil
}

func (s stubSeasonal) GetActivePatternNames(_ context.Context, _ string, _ time.Time) []string {
	return nil
}

type stubLinkage struct{ v float64 }

func (s stubLinkage) GetLinkageMultiplier(_ context.Context, _ string) (float64, error) {
	return s.v, nil
}

type stubNarrative struct{ v float64 }

func (s stubNarrative) GetNarrativeMultiplier(_ context.Context, _ string) (float64, error) {
	return s.v, nil
}

type stubMacro struct{ tilt float64 }

func (s stubMacro) GetMacroTilt(_ context.Context, _, _, _ string) (float64, error) {
	return s.tilt, nil
}
func (s stubMacro) GetMacroFlowDescription(_, _ string) string { return "" }

type stubFactor struct{ tilt float64 }

func (s stubFactor) GetFactorTilt(_ context.Context, _ string) (float64, error) {
	return s.tilt, nil
}

func newTestEngine(cycle, seasonal, linkage, narrative float64, macro, factor float64) WeightEngine {
	cfg := config.SectorAllocationConfig{
		BaseWeights: map[string]float64{
			"semiconductor": 0.30,
			"financials":    0.14,
			"_cash_reserve": 0.02,
		},
		DerivationFactors: map[string][]config.WeightFactorConfig{
			"semiconductor": {{Factor: "出口比重", Weight: 0.35, Source: "X", Evidence: "Y"}},
		},
		CycleWeight:     1.0,
		SeasonalWeight:  1.0,
		LinkageWeight:   1.0,
		NarrativeWeight: 1.0,
		MacroWeight:     1.0,
		FactorWeight:    1.0,
		WeightFloor:     0.01,
	}
	return NewDefaultEngine(
		cfg,
		stubCycle{v: cycle},
		stubSeasonal{v: seasonal},
		stubLinkage{v: linkage},
		stubNarrative{v: narrative},
		stubMacro{tilt: macro},
		stubFactor{tilt: factor},
		0.3, 2.5,
	)
}

func TestComputeWeights_NeutralMultipliers(t *testing.T) {
	engine := newTestEngine(1.0, 1.0, 1.0, 1.0, 0.0, 0.0)
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)

	weights, err := engine.ComputeWeights(ctx, now)
	if err != nil {
		t.Fatalf("ComputeWeights: %v", err)
	}
	if len(weights) != 3 {
		t.Fatalf("expected 3 weights, got %d", len(weights))
	}
	for _, w := range weights {
		if w.AdjustedWeight != w.BaseWeight {
			t.Errorf("industry %s: adjusted (%.4f) should equal base (%.4f) when all multipliers are neutral",
				w.ID, w.AdjustedWeight, w.BaseWeight)
		}
	}
}

func TestComputeWeights_AllMultipliersActive(t *testing.T) {
	engine := newTestEngine(1.1, 1.05, 1.15, 1.1, -0.15, 0.05)
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)

	weights, err := engine.ComputeWeights(ctx, now)
	if err != nil {
		t.Fatalf("ComputeWeights: %v", err)
	}
	for _, w := range weights {
		expected := w.BaseWeight * 1.1 * 1.05 * 1.15 * 1.1 * 0.85 * 1.05
		if diff := abs(w.AdjustedWeight - expected); diff > 0.0001 {
			t.Errorf("industry %s: adjusted (%.6f) != expected (%.6f)", w.ID, w.AdjustedWeight, expected)
		}
	}
}

func TestComputeWeights_ZeroBaseWeights_Rejected(t *testing.T) {
	cfg := config.SectorAllocationConfig{BaseWeights: map[string]float64{}}
	engine := NewDefaultEngine(cfg, nil, nil, nil, nil, nil, nil, 0.3, 2.5)
	_, err := engine.ComputeWeights(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected error for empty base_weights")
	}
}

func TestComputeWeight_UnknownIndustry_Rejected(t *testing.T) {
	engine := newTestEngine(1.0, 1.0, 1.0, 1.0, 0.0, 0.0)
	_, err := engine.ComputeWeight(context.Background(), "unknown", time.Now())
	if err == nil {
		t.Fatal("expected error for unknown industry")
	}
}

func TestComputeWeight_FloorClamp(t *testing.T) {
	cfg := config.SectorAllocationConfig{
		BaseWeights:     map[string]float64{"x": 0.05},
		CycleWeight:     1.0,
		SeasonalWeight:  1.0,
		LinkageWeight:   1.0,
		NarrativeWeight: 1.0,
		MacroWeight:     1.0,
		FactorWeight:    1.0,
		WeightFloor:     0.50,
	}
	engine := NewDefaultEngine(cfg,
		stubCycle{v: 1.0}, stubSeasonal{v: 1.0}, stubLinkage{v: 1.0},
		stubNarrative{v: 1.0}, stubMacro{}, stubFactor{},
		0.3, 2.5)
	w, err := engine.ComputeWeight(context.Background(), "x", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	if w.AdjustedWeight < 0.025 {
		t.Errorf("expected floor-adjusted weight >= 0.025 (0.05*0.50), got %.4f", w.AdjustedWeight)
	}
}

func TestComputeWeights_SortDesc(t *testing.T) {
	engine := newTestEngine(1.5, 1.0, 1.0, 1.0, 0.0, 0.0)
	weights, err := engine.ComputeWeights(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ComputeWeights: %v", err)
	}
	for i := 1; i < len(weights); i++ {
		if weights[i-1].AdjustedWeight < weights[i].AdjustedWeight {
			t.Errorf("weights not sorted desc: %v", []float64{
				weights[i-1].AdjustedWeight, weights[i].AdjustedWeight,
			})
		}
	}
}

func TestComputeWeight_AdjustmentLogPopulated(t *testing.T) {
	engine := newTestEngine(1.1, 1.0, 1.0, 1.0, 0.0, 0.0)
	w, err := engine.ComputeWeight(context.Background(), "semiconductor", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	if len(w.AdjustmentLog) < 7 {
		t.Errorf("expected at least 7 log entries, got %d", len(w.AdjustmentLog))
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
