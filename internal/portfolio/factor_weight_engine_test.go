package portfolio

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

func TestFactorWeightEngine_GetWeights_Default(t *testing.T) {
	engine := NewFactorWeightEngine()
	weights := engine.GetWeights("")

	if len(weights) != 8 {
		t.Errorf("expected 8 factors, got %d", len(weights))
	}

	var total float64
	for _, w := range weights {
		total += w
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should sum to ~1.0, got %f", total)
	}
}

func TestFactorWeightEngine_GetWeights_BullRegime(t *testing.T) {
	engine := NewFactorWeightEngine()
	engine.OnRegimeChange("", "RISK_ON", 0.8)
	weights := engine.GetWeights("")

	if weights[FactorMomentum] <= weights[FactorQuality] {
		t.Error("RISK_ON regime should boost momentum above quality")
	}
}

func TestFactorWeightEngine_GetWeights_BearRegime(t *testing.T) {
	engine := NewFactorWeightEngine()
	engine.OnRegimeChange("", "RISK_OFF", 0.8)
	weights := engine.GetWeights("")

	if weights[FactorQuality] <= weights[FactorMomentum] {
		t.Error("RISK_OFF regime should boost quality above momentum")
	}
}

func TestFactorWeightEngine_GetWeights_WithEvent(t *testing.T) {
	engine := NewFactorWeightEngine()

	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "AI_capex_surge",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)

	weights := engine.GetWeights("")

	if weights[FactorQuality] <= 0.20 {
		t.Error("AI capex surge should boost quality weight")
	}
	if weights[FactorMomentum] <= 0.24 {
		t.Errorf("AI capex surge should boost momentum above baseline, got %.4f", weights[FactorMomentum])
	}
}

func TestFactorWeightEngine_GetWeights_EventClamped(t *testing.T) {
	engine := NewFactorWeightEngine()

	for i := range 10 {
		event := &narrative.NarrativeEvent{
			ID:       "test-event-" + string(rune('a'+i)),
			Theme:    "AI_capex_surge",
			Severity: "critical",
			Status:   "active",
		}
		engine.AddEvent(event)
	}

	weights := engine.GetWeights("")

	for ft, w := range weights {
		if w < 0.015 || w > 0.50 {
			t.Errorf("weight for %s should be clamped [0.015, 0.50], got %f", ft, w)
		}
	}
}

func TestFactorWeightEngine_OnRegimeChange(t *testing.T) {
	engine := NewFactorWeightEngine()

	engine.OnRegimeChange("", "RISK_ON", 0.8)
	weights := engine.GetWeights("")

	if weights[FactorMomentum] <= 0.20 {
		t.Error("RISK_ON regime should boost momentum")
	}

	engine.OnRegimeChange("RISK_ON", "RISK_OFF", 0.8)
	weights = engine.GetWeights("")

	if weights[FactorQuality] <= 0.15 {
		t.Error("RISK_OFF regime should boost quality")
	}
}

func TestFactorWeightEngine_RemoveEvent(t *testing.T) {
	engine := NewFactorWeightEngine()

	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "US_rates_up",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)

	weightsWithEvent := engine.GetWeights("")
	engine.RemoveEvent("test-event-1")
	weightsWithoutEvent := engine.GetWeights("")

	if weightsWithEvent[FactorValue] <= weightsWithoutEvent[FactorValue] {
		t.Error("removing event should reduce value weight")
	}
}

func TestFactorWeightEngine_Normalization(t *testing.T) {
	engine := NewFactorWeightEngine()

	engine.OnRegimeChange("", "bull", 0.9)

	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "oil_price_shock",
		Severity: "critical",
		Status:   "active",
	}
	engine.AddEvent(event)

	weights := engine.GetWeights("bull")

	var total float64
	for _, w := range weights {
		total += w
	}

	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should normalize to ~1.0, got %f", total)
	}
}

func TestFactorWeightEngine_Update_RemovesExpired(t *testing.T) {
	engine := NewFactorWeightEngine()

	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "AI_capex_surge",
		Severity: "high",
		Status:   "expired",
	}
	engine.AddEvent(event)
	engine.Update()

	weights := engine.GetWeights("")
	baseWeights := NewFactorWeightEngine().GetWeights("")

	for ft := range weights {
		diff := weights[ft] - baseWeights[ft]
		if diff < -0.001 || diff > 0.001 {
			t.Errorf("expired event should not affect weights, got diff %f for %s", diff, ft)
		}
	}
}

func TestFactorWeightEngine_AddEvent_Critical(t *testing.T) {
	engine := NewFactorWeightEngine()

	event := &narrative.NarrativeEvent{
		ID:       "test-critical-event",
		Theme:    "AI_capex_surge",
		Severity: "critical",
		Status:   "active",
	}
	engine.AddEvent(event)

	weights := engine.GetWeights("")

	// Critical AI_capex_surge adds 0.10 to quality and 0.10 to momentum
	if weights[FactorQuality] <= 0.20 {
		t.Errorf("critical AI capex surge should boost quality above baseline 0.20, got %.4f", weights[FactorQuality])
	}
	if weights[FactorMomentum] <= 0.24 {
		t.Errorf("critical AI capex surge should boost momentum above baseline, got %.4f", weights[FactorMomentum])
	}

	// Verify weights changed from baseline (event adjustments applied)
	baseEngine := NewFactorWeightEngine()
	baseWeights := baseEngine.GetWeights("")
	same := true
	for ft := range weights {
		if weights[ft] != baseWeights[ft] {
			same = false
			break
		}
	}
	if same {
		t.Error("critical event should change weights from baseline, but all weights are identical")
	}
}

func TestFactorWeightEngine_ApplyStrategy_Conservative(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")

	engine.ApplyStrategy(&strategy.Strategy{
		RiskAppetite: strategy.RiskAppetiteConservative,
	})

	weights := engine.GetWeights("")

	// Conservative: +Value, +Quality, -Momentum
	if weights[FactorValue] <= baseWeights[FactorValue] {
		t.Errorf("conservative strategy should boost value, got %.4f (baseline %.4f)", weights[FactorValue], baseWeights[FactorValue])
	}
	if weights[FactorQuality] <= baseWeights[FactorQuality] {
		t.Errorf("conservative strategy should boost quality, got %.4f (baseline %.4f)", weights[FactorQuality], baseWeights[FactorQuality])
	}
	if weights[FactorMomentum] >= baseWeights[FactorMomentum] {
		t.Errorf("conservative strategy should reduce momentum, got %.4f (baseline %.4f)", weights[FactorMomentum], baseWeights[FactorMomentum])
	}
}

func TestFactorWeightEngine_ApplyStrategy_Aggressive(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")

	engine.ApplyStrategy(&strategy.Strategy{
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	weights := engine.GetWeights("")

	// Aggressive: +Momentum, +InstSent, -Value, -Quality
	if weights[FactorMomentum] <= baseWeights[FactorMomentum] {
		t.Errorf("aggressive strategy should boost momentum, got %.4f (baseline %.4f)", weights[FactorMomentum], baseWeights[FactorMomentum])
	}
	if weights[FactorInstSent] <= baseWeights[FactorInstSent] {
		t.Errorf("aggressive strategy should boost institutional sentiment, got %.4f (baseline %.4f)", weights[FactorInstSent], baseWeights[FactorInstSent])
	}
	if weights[FactorValue] >= baseWeights[FactorValue] {
		t.Errorf("aggressive strategy should reduce value, got %.4f (baseline %.4f)", weights[FactorValue], baseWeights[FactorValue])
	}
	if weights[FactorQuality] >= baseWeights[FactorQuality] {
		t.Errorf("aggressive strategy should reduce quality, got %.4f (baseline %.4f)", weights[FactorQuality], baseWeights[FactorQuality])
	}
}

// TestFactorWeightEngine_WeightSource verifies that WeightSource()
// returns the expected source string.
func TestFactorWeightEngine_WeightSource(t *testing.T) {
	engine := NewFactorWeightEngine()
	source := engine.WeightSource()
	if source == "" {
		t.Error("expected WeightSource() to return non-empty string")
	}
	if source != "builtin_defaults" && source != "config" {
		t.Errorf("expected WeightSource() to be 'builtin_defaults' or 'config', got %q", source)
	}
	t.Logf("WeightSource: %s", source)
}

// TestFactorWeightEngine_PM_GoldRally verifies gold_rally event
// boosts PreciousMetals and reduces Value by severity delta.
func TestFactorWeightEngine_PM_GoldRally(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")

	event := &narrative.NarrativeEvent{
		ID:       "gold-rally-1",
		Theme:    "gold_rally",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)
	weights := engine.GetWeights("")

	if weights[FactorPreciousMetals] <= base[FactorPreciousMetals] {
		t.Errorf("gold_rally should boost PreciousMetals above baseline %.4f, got %.4f",
			base[FactorPreciousMetals], weights[FactorPreciousMetals])
	}
	if weights[FactorValue] >= base[FactorValue] {
		t.Errorf("gold_rally should reduce Value below baseline %.4f, got %.4f",
			base[FactorValue], weights[FactorValue])
	}
}

// TestFactorWeightEngine_PM_DollarSurge verifies dollar_surge event
// reduces PreciousMetals (clamped to minimum) and boosts Liquidity.
func TestFactorWeightEngine_PM_DollarSurge(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")

	event := &narrative.NarrativeEvent{
		ID:       "dollar-surge-1",
		Theme:    "dollar_surge",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)
	weights := engine.GetWeights("")

	// Negative delta on a zero-baseline factor is clamped to clampMin (~0.02).
	// PM baseline = 0, final ≈ clampMin after normalization.
	if weights[FactorPreciousMetals] < 0.01 || weights[FactorPreciousMetals] > 0.03 {
		t.Errorf("dollar_surge: PM should be near clamp minimum, got %.4f", weights[FactorPreciousMetals])
	}
	if weights[FactorLiquidity] <= base[FactorLiquidity] {
		t.Errorf("dollar_surge should boost Liquidity above baseline %.4f, got %.4f",
			base[FactorLiquidity], weights[FactorLiquidity])
	}
}

// --- ApplyStrategyMix ---

func TestFactorWeightEngine_ApplyStrategyMix_Empty(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")

	engine.ApplyStrategyMix(strategy.StrategyMix{}, nil)

	weights := engine.GetWeights("")
	for ft, w := range weights {
		if math.Abs(w-base[ft]) > 1e-6 {
			t.Errorf("empty mix: factor %s should stay at baseline %.4f, got %.4f", ft, base[ft], w)
		}
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_NilMix(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")

	engine.ApplyStrategyMix(nil, nil)

	weights := engine.GetWeights("")
	for ft, w := range weights {
		if math.Abs(w-base[ft]) > 1e-6 {
			t.Errorf("nil mix: factor %s should match baseline %.4f, got %.4f", ft, base[ft], w)
		}
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_Single_Conservative(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "cons",
		RiskAppetite: strategy.RiskAppetiteConservative,
	})

	engine.ApplyStrategyMix(strategy.StrategyMix{"cons": 1.0}, reg)

	weights := engine.GetWeights("")
	if weights[FactorValue] <= baseWeights[FactorValue] {
		t.Errorf("conservative: should boost Value, baseline=%.4f got=%.4f", baseWeights[FactorValue], weights[FactorValue])
	}
	if weights[FactorQuality] <= baseWeights[FactorQuality] {
		t.Errorf("conservative: should boost Quality, baseline=%.4f got=%.4f", baseWeights[FactorQuality], weights[FactorQuality])
	}
	if weights[FactorMomentum] >= baseWeights[FactorMomentum] {
		t.Errorf("conservative: should reduce Momentum, baseline=%.4f got=%.4f", baseWeights[FactorMomentum], weights[FactorMomentum])
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_Single_Aggressive(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "agg",
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	engine.ApplyStrategyMix(strategy.StrategyMix{"agg": 1.0}, reg)

	weights := engine.GetWeights("")
	if weights[FactorMomentum] <= baseWeights[FactorMomentum] {
		t.Errorf("aggressive: should boost Momentum, baseline=%.4f got=%.4f", baseWeights[FactorMomentum], weights[FactorMomentum])
	}
	if weights[FactorInstSent] <= baseWeights[FactorInstSent] {
		t.Errorf("aggressive: should boost InstSent, baseline=%.4f got=%.4f", baseWeights[FactorInstSent], weights[FactorInstSent])
	}
	if weights[FactorValue] >= baseWeights[FactorValue] {
		t.Errorf("aggressive: should reduce Value, baseline=%.4f got=%.4f", baseWeights[FactorValue], weights[FactorValue])
	}
	if weights[FactorQuality] >= baseWeights[FactorQuality] {
		t.Errorf("aggressive: should reduce Quality, baseline=%.4f got=%.4f", baseWeights[FactorQuality], weights[FactorQuality])
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_Balanced_NoEffect(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "bal",
		RiskAppetite: strategy.RiskAppetiteBalanced,
	})

	engine.ApplyStrategyMix(strategy.StrategyMix{"bal": 1.0}, reg)

	weights := engine.GetWeights("")
	for ft, w := range weights {
		if math.Abs(w-base[ft]) > 1e-6 {
			t.Errorf("balanced: factor %s should match baseline %.4f, got %.4f", ft, base[ft], w)
		}
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_Blended(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "cons",
		RiskAppetite: strategy.RiskAppetiteConservative,
	})
	reg.Register(&strategy.Strategy{
		ID:           "agg",
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	// 50/50 blend ≡ (conservative + aggressive) / 2
	// Conservative: +0.05 Value, +0.05 Quality, -0.05 Momentum
	// Aggressive:   +0.05 Momentum, +0.03 InstSent, -0.03 Value, -0.03 Quality
	// Blended: Momentum = (+0.05 - 0.05)/2 = 0
	//          Value    = (+0.05 - 0.03)/2 = 0.01
	//          Quality  = (+0.05 - 0.03)/2 = 0.01
	//          InstSent = (+0.03 + 0)/2 = 0.015
	engine.ApplyStrategyMix(strategy.StrategyMix{"cons": 0.5, "agg": 0.5}, reg)

	weights := engine.GetWeights("")
	if weights[FactorValue] <= baseWeights[FactorValue] {
		t.Errorf("50/50 blend: Value should be above baseline, got %.4f (baseline %.4f)", weights[FactorValue], baseWeights[FactorValue])
	}
	if weights[FactorQuality] <= baseWeights[FactorQuality] {
		t.Errorf("50/50 blend: Quality should be above baseline, got %.4f (baseline %.4f)", weights[FactorQuality], baseWeights[FactorQuality])
	}
	if weights[FactorInstSent] <= baseWeights[FactorInstSent] {
		t.Errorf("50/50 blend: InstSent should be above baseline, got %.4f (baseline %.4f)", weights[FactorInstSent], baseWeights[FactorInstSent])
	}
	momDelta := weights[FactorMomentum] - baseWeights[FactorMomentum]
	if momDelta > 0.01 || momDelta < -0.01 {
		t.Errorf("50/50 blend: Momentum should be near baseline, got delta=%.4f", momDelta)
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_MissingStrategy(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")
	reg := strategy.NewRegistry()

	engine.ApplyStrategyMix(strategy.StrategyMix{"ghost": 1.0}, reg)

	weights := engine.GetWeights("")
	for ft, w := range weights {
		if math.Abs(w-base[ft]) > 1e-6 {
			t.Errorf("missing strategy: factor %s should match baseline %.4f, got %.4f", ft, base[ft], w)
		}
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_Partial_Missing(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "agg",
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	engine.ApplyStrategyMix(strategy.StrategyMix{"agg": 0.5, "ghost": 0.5}, reg)

	weights := engine.GetWeights("")
	if weights[FactorMomentum] <= baseWeights[FactorMomentum] {
		t.Errorf("partial missing: Momentum should be boosted by agg, baseline=%.4f got=%.4f", baseWeights[FactorMomentum], weights[FactorMomentum])
	}
	if weights[FactorValue] >= baseWeights[FactorValue] {
		t.Errorf("partial missing: Value should be reduced by agg, baseline=%.4f got=%.4f", baseWeights[FactorValue], weights[FactorValue])
	}
}

func TestFactorWeightEngine_ApplyStrategyMix_WeightsStayNormalized(t *testing.T) {
	engine := NewFactorWeightEngine()
	reg := strategy.NewRegistry()
	reg.Register(&strategy.Strategy{
		ID:           "cons",
		RiskAppetite: strategy.RiskAppetiteConservative,
	})
	reg.Register(&strategy.Strategy{
		ID:           "agg",
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	engine.ApplyStrategyMix(strategy.StrategyMix{"cons": 0.6, "agg": 0.4}, reg)
	weights := engine.GetWeights("")

	var total float64
	for _, w := range weights {
		total += w
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should sum to ~1.0 after strategy mix, got %f", total)
	}
}

// TestFactorWeightEngine_PM_InflationSpike verifies inflation_spike boosts PM,
// reduces Momentum, and weights stay normalized.
func TestFactorWeightEngine_PM_InflationSpike(t *testing.T) {
	engine := NewFactorWeightEngine()
	base := engine.GetWeights("")

	event := &narrative.NarrativeEvent{
		ID:       "inf-spike-1",
		Theme:    "inflation_spike",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)
	weights := engine.GetWeights("")

	if weights[FactorPreciousMetals] <= base[FactorPreciousMetals] {
		t.Errorf("inflation_spike should boost PM above baseline %.4f, got %.4f",
			base[FactorPreciousMetals], weights[FactorPreciousMetals])
	}
	if weights[FactorMomentum] >= base[FactorMomentum] {
		t.Errorf("inflation_spike should reduce Momentum below baseline %.4f, got %.4f",
			base[FactorMomentum], weights[FactorMomentum])
	}

	var total float64
	for _, w := range weights {
		total += w
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should normalize to ~1.0 after PM event, got %f", total)
	}
}

// TestNewOptimizer_FactorWeightEngineOverHardcoded is the Wave 7.5 Task 7
// regression test. It locks the production path: NewOptimizer() must wire a
// live FactorWeightEngine so Optimize() picks up calibrator updates via
// fwe.GetWeights("") instead of the static params.Optimizer.FactorWeights map.
func TestNewOptimizer_FactorWeightEngineOverHardcoded(t *testing.T) {
	o := NewOptimizer()

	if o.factorWeightEngine == nil {
		t.Fatal("NewOptimizer() must initialize factorWeightEngine; production path broken")
	}

	src := o.factorWeightEngine.WeightSource()
	if src != "builtin_defaults" && src != "config" {
		t.Errorf("unexpected weight source %q", src)
	}

	weights := o.factorWeightEngine.GetWeights("")
	if len(weights) == 0 {
		t.Fatal("FactorWeightEngine.GetWeights() returned empty; fwe path broken")
	}
	for ft := range weights {
		if weights[ft] < 0.02 || weights[ft] > 0.50 {
			t.Errorf("factor %s weight %.4f outside clamp bounds [0.02, 0.50]", ft, weights[ft])
		}
	}
}
