package strategy

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Helper: build a registry with synthetic strategies.
func syntheticRegistry(strategies []*Strategy) *Registry {
	r := NewRegistry()
	for _, s := range strategies {
		r.Register(s)
	}
	return r
}

func makeTestStrategy(id string, regimePrefs ...domain.Regime) *Strategy {
	return &Strategy{
		ID:          id,
		Name:        id,
		Enabled:     true,
		Agents:      []string{"test"},
		RegimePrefs: regimePrefs,
	}
}

// ── Scenario A: Two-strategy risk parity ──
// growth (σ=0.25) vs gold_hedge (σ=0.15).
// Expected: w_gold > w_growth because 1/0.15 > 1/0.25.
func TestRiskParity_TwoStrategies(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{
		makeTestStrategy("growth", domain.RegimeRiskOn, domain.RegimeRiskOff),
		makeTestStrategy("gold_hedge", domain.RegimeRiskOn, domain.RegimeRiskOff),
	})
	sa := NewStrategyAllocator(reg)
	sa.SetCaps(0.60, 0.05)

	dailyGrowth := 0.25 / math.Sqrt(252)
	dailyGold := 0.15 / math.Sqrt(252)
	rng := 0
	for range 60 {
		rng++
		sign := 1.0
		if rng%2 == 0 {
			sign = -1.0
		}
		sa.UpdateShadowReturns(map[string]float64{
			"growth":     dailyGrowth * sign * (1 + 0.1*float64(rng%5)),
			"gold_hedge": dailyGold * sign * (1 + 0.1*float64(rng%3)),
		})
	}

	mix := sa.Allocate(domain.RegimeRiskOn, 20)

	if !mix.Validate() {
		t.Error("risk parity mix failed validation (Σw ≠ 1)")
	}

	wGold := mix["gold_hedge"]
	wGrowth := mix["growth"]

	if wGrowth >= wGold {
		t.Errorf("gold_hedge(σ=0.15) weight %.4f should exceed growth(σ=0.25) weight %.4f",
			wGold, wGrowth)
	}
}

// ── Scenario B: Weight cap enforcement ──
// defensive (σ=0.10), gold_hedge (σ=0.15), growth (σ=0.25), w_max=0.50.
// Without caps, defensive would get ~56%. With cap→0.50, excess redistributed.
func TestRiskParity_WeightCaps(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{
		makeTestStrategy("defensive", domain.RegimeRiskOff),
		makeTestStrategy("growth", domain.RegimeRiskOff),
		makeTestStrategy("gold_hedge", domain.RegimeRiskOff),
	})
	sa := NewStrategyAllocator(reg)
	sa.SetCaps(0.50, 0.05)

	dailyDef := 0.10 / math.Sqrt(252)
	dailyGold := 0.15 / math.Sqrt(252)
	dailyGrowth := 0.25 / math.Sqrt(252)
	rngB := 0
	for range 60 {
		rngB++
		sgn := 1.0
		if rngB%2 == 0 {
			sgn = -1.0
		}
		sa.UpdateShadowReturns(map[string]float64{
			"defensive":  dailyDef * sgn,
			"gold_hedge": dailyGold * sgn,
			"growth":     dailyGrowth * sgn,
		})
	}

	mix := sa.Allocate(domain.RegimeRiskOff, 20)

	if !mix.Validate() {
		t.Error("capped mix failed validation")
	}

	for id, w := range mix {
		if w > 0.50+0.001 {
			t.Errorf("%s weight %.4f exceeds max 0.50", id, w)
		}
		if w < 0.05-0.001 {
			t.Errorf("%s weight %.4f below min 0.05", id, w)
		}
	}

	if mix["defensive"] < 0.47 {
		t.Errorf("defensive weight %.4f should be near cap 0.50", mix["defensive"])
	}
}

// ── Scenario C: COVID crash — risk parity auto-defends ──
// Pre-crash: growth dominates (low vol in calm market).
// During crash: volatility spike → growth weight drops, gold_hedge rises.
func TestRiskParity_COVIDAutoDefense(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{
		makeTestStrategy("growth", domain.RegimeRiskOn, domain.RegimeRiskOff),
		makeTestStrategy("gold_hedge", domain.RegimeRiskOn, domain.RegimeRiskOff),
		makeTestStrategy("defensive", domain.RegimeRiskOn, domain.RegimeRiskOff),
	})
	sa := NewStrategyAllocator(reg)

	// Pre-crash: calm market — growth has low vol.
	calmVol := 0.12 / math.Sqrt(252)
	for range 60 {
		sa.UpdateShadowReturns(map[string]float64{
			"growth":     calmVol,
			"gold_hedge": calmVol * 1.2,
			"defensive":  calmVol * 0.9,
		})
	}
	preCrash := sa.Allocate(domain.RegimeRiskOn, 18)

	// Crash: growth vol spikes 3x, gold and defensive less affected.
	crashVol := 0.36 / math.Sqrt(252) // 3x
	for range 20 {
		sa.UpdateShadowReturns(map[string]float64{
			"growth":     crashVol,
			"gold_hedge": calmVol * 1.5,
			"defensive":  calmVol * 1.1,
		})
	}
	duringCrash := sa.Allocate(domain.RegimeRiskOn, 60)

	if duringCrash["growth"] >= preCrash["growth"] {
		t.Errorf("growth weight should drop during crash (%.4f → %.4f)",
			preCrash["growth"], duringCrash["growth"])
	}
	if duringCrash["gold_hedge"] <= preCrash["gold_hedge"] {
		t.Errorf("gold_hedge weight should rise during crash (%.4f → %.4f)",
			preCrash["gold_hedge"], duringCrash["gold_hedge"])
	}
}

func TestRiskParity_EmptyRegistry(t *testing.T) {
	reg := NewRegistry()
	sa := NewStrategyAllocator(reg)
	mix := sa.Allocate(domain.RegimeNeutral, 20)
	if len(mix) != 0 {
		t.Error("empty registry should produce empty mix")
	}
}

func TestEqualMix(t *testing.T) {
	candidates := []*Strategy{
		{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"},
	}
	mix := equalMix(candidates)
	if len(mix) != 4 {
		t.Fatalf("len(mix) = %d, want 4", len(mix))
	}
	if !mix.Validate() {
		t.Error("equal mix should validate")
	}
	expectedW := 0.25
	for _, id := range []string{"a", "b", "c", "d"} {
		if mix[id] != expectedW {
			t.Errorf("mix[%s] = %f, want %f", id, mix[id], expectedW)
		}
	}
}

func TestEqualMix_Empty(t *testing.T) {
	mix := equalMix(nil)
	if mix == nil {
		t.Error("equalMix(nil) should return non-nil map")
	}
	if len(mix) != 0 {
		t.Errorf("len(mix) = %d, want 0", len(mix))
	}
}

func TestVolatilities_Empty(t *testing.T) {
	reg := NewRegistry()
	sa := NewStrategyAllocator(reg)

	vols := sa.Volatilities()
	if len(vols) != 0 {
		t.Errorf("len(Volatilities) = %d, want 0 for empty registry", len(vols))
	}
}

func TestVolatilities_WithReturns(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{
		makeTestStrategy("growth", domain.RegimeRiskOn),
		makeTestStrategy("defensive", domain.RegimeRiskOn),
	})
	sa := NewStrategyAllocator(reg)

	sa.UpdateShadowReturns(map[string]float64{
		"growth":    0.01,
		"defensive": 0.005,
	})
	sa.UpdateShadowReturns(map[string]float64{
		"growth":    -0.005,
		"defensive": 0.003,
	})

	vols := sa.Volatilities()
	if len(vols) != 2 {
		t.Fatalf("len(Volatilities) = %d, want 2", len(vols))
	}
	for id, v := range vols {
		if v <= 0 {
			t.Errorf("%s volatility = %f, want > 0", id, v)
		}
	}
}

func TestStrategyMix_Validate(t *testing.T) {
	valid := StrategyMix{"a": 0.6, "b": 0.4}
	if !valid.Validate() {
		t.Error("valid mix should pass")
	}

	over := StrategyMix{"a": 0.6, "b": 0.5}
	if over.Validate() {
		t.Error("over-allocated should fail")
	}

	neg := StrategyMix{"a": -0.1, "b": 1.1}
	if neg.Validate() {
		t.Error("negative weight should fail")
	}

	nan := StrategyMix{"a": math.NaN(), "b": 1.0}
	if nan.Validate() {
		t.Error("NaN weight should fail")
	}
}
