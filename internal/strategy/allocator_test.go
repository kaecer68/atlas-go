package strategy

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAllocator_Bull(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)

	alloc := a.Compute(domain.RegimeRiskOn)

	if !alloc.Validate() {
		t.Error("bull allocation failed validation")
	}
	if alloc.Regime != domain.RegimeRiskOn {
		t.Errorf("expected RiskOn regime, got %s", alloc.Regime)
	}
	if alloc.Source != "config" {
		t.Errorf("expected source 'config', got %s", alloc.Source)
	}

	wGrowth := alloc.Allocations["growth"]
	if wGrowth < 0.35 || wGrowth > 0.45 {
		t.Errorf("bull growth weight = %.4f, expected ~0.40", wGrowth)
	}
	if alloc.Allocations["momentum"] < 0.25 {
		t.Errorf("bull momentum weight = %.4f, expected ~0.30", alloc.Allocations["momentum"])
	}
}

func TestAllocator_Bear(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)

	alloc := a.Compute(domain.RegimeRiskOff)

	if !alloc.Validate() {
		t.Error("bear allocation failed validation")
	}
	if alloc.Allocations["defensive"] < 0.35 {
		t.Errorf("bear defensive weight = %.4f, expected ~0.40", alloc.Allocations["defensive"])
	}
}

func TestAllocator_Neutral(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)
	alloc := a.Compute(domain.RegimeNeutral)
	if !alloc.Validate() {
		t.Error("neutral allocation failed validation")
	}
}

func TestAllocator_UnknownRegime(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)
	alloc := a.Compute("invalid_regime")

	if !alloc.Validate() {
		t.Error("unknown regime should fallback to equal weight")
	}
	if alloc.Source != "equal" {
		t.Errorf("unknown regime should use equal weight, got %s", alloc.Source)
	}
}

func TestAllocator_EmptyWeights(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)
	a.SetBaseWeights(map[domain.Regime]map[string]float64{
		domain.RegimeRiskOn: {},
	})
	alloc := a.Compute(domain.RegimeRiskOn)
	if !alloc.Validate() {
		t.Error("empty weights should fallback to equal")
	}
	if alloc.Source != "equal" {
		t.Errorf("empty weights should use equal, got %s", alloc.Source)
	}
}

func TestAllocator_FiltersUnregistered(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)
	a.SetBaseWeights(map[domain.Regime]map[string]float64{
		domain.RegimeRiskOff: {
			"defensive":   0.50,
			"nonexistent": 0.50,
		},
	})
	alloc := a.Compute(domain.RegimeRiskOff)
	if !alloc.Validate() {
		t.Error("filtered allocation failed validation")
	}
	if _, ok := alloc.Allocations["nonexistent"]; ok {
		t.Error("unregistered strategy should be filtered out")
	}
	if math.Abs(alloc.Allocations["defensive"]-1.0) > 0.001 {
		t.Errorf("defensive should be 1.0 after filtering, got %.4f", alloc.Allocations["defensive"])
	}
}

func TestAllocator_AllRegimesCovered(t *testing.T) {
	reg := NewRegistryWithDefaults()
	a := NewAllocator(reg)

	regimes := []domain.Regime{
		domain.RegimeRiskOn, domain.RegimeRiskOff, domain.RegimeNeutral,
	}
	for _, r := range regimes {
		alloc := a.Compute(r)
		if !alloc.Validate() {
			t.Errorf("%s allocation invalid", r)
		}
		if alloc.Regime != r {
			t.Errorf("%s: wrong regime %s", r, alloc.Regime)
		}
	}
}

func TestStrategyAllocation_Validate(t *testing.T) {
	valid := StrategyAllocation{
		Allocations: map[string]float64{"a": 0.6, "b": 0.4},
	}
	if !valid.Validate() {
		t.Error("valid allocation should pass")
	}

	tooMuch := StrategyAllocation{
		Allocations: map[string]float64{"a": 0.6, "b": 0.5},
	}
	if tooMuch.Validate() {
		t.Error("over-allocated should fail")
	}

	negWeight := StrategyAllocation{
		Allocations: map[string]float64{"a": -0.1, "b": 1.1},
	}
	if negWeight.Validate() {
		t.Error("negative weight should fail")
	}
}
