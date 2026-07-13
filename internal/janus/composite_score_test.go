package janus

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/prism"
)

// synthesizeCompositeScore formula (Oracle-reviewed):
//
//	score = tanh(foreignFlow/5e9) * 30 - max(0, VIX-20) * 1.5
//
// Foreign flow continuous (tanh saturates at ±30 when flow ≈ ±5B NTD).
// VIX penalty 1.5/unit above 20 — so VIX 40 (= -30) matches foreign ±5B
// magnitude, giving 1:1 balance per Oracle recommendation.

func TestSynthesizeCompositeScore_Bullish(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.5},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	}
	// tanh(1.5/5) ≈ 0.2913 → 8.74
	if got, want := synthesizeCompositeScore(snap), 8.74; absDiff(got, want) > 0.1 {
		t.Errorf("bullish: got %v, want ~%v", got, want)
	}
}

func TestSynthesizeCompositeScore_BullishWithPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.5},
		VIX:                marketdata.MacroDataPoint{Value: 40},
	}
	// 8.74 - 30 = -21.26
	if got, want := synthesizeCompositeScore(snap), -21.26; absDiff(got, want) > 0.1 {
		t.Errorf("bullish+panic: got %v, want ~%v", got, want)
	}
}

func TestSynthesizeCompositeScore_Bearish(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	}
	// tanh(-2/5) ≈ -0.380 → -11.40
	if got, want := synthesizeCompositeScore(snap), -11.40; absDiff(got, want) > 0.1 {
		t.Errorf("bearish: got %v, want ~%v", got, want)
	}
}

func TestSynthesizeCompositeScore_BearishWithPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2.0},
		VIX:                marketdata.MacroDataPoint{Value: 50},
	}
	// -11.40 - 45 = -56.40
	if got, want := synthesizeCompositeScore(snap), -56.40; absDiff(got, want) > 0.1 {
		t.Errorf("bearish+panic: got %v, want ~%v", got, want)
	}
}

func TestSynthesizeCompositeScore_NeutralFlowNoPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		VIX:                marketdata.MacroDataPoint{Value: 18},
	}
	if got, want := synthesizeCompositeScore(snap), 0.0; absDiff(got, want) > 1e-9 {
		t.Errorf("neutral: got %v, want %v", got, want)
	}
}

func TestSynthesizeCompositeScore_VIXAtBaselineNoPenalty(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.0},
		VIX:                marketdata.MacroDataPoint{Value: 20},
	}
	// tanh(0.2) ≈ 0.1974 → 5.92 (VIX exactly 20 → no penalty)
	if got, want := synthesizeCompositeScore(snap), 5.92; absDiff(got, want) > 0.1 {
		t.Errorf("VIX=20 baseline: got %v, want ~%v", got, want)
	}
}

func TestSynthesizeCompositeScore_VIXBaseline_UsesCustomThreshold(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		VIXBaseline:        15,
	}
	if got, want := synthesizeCompositeScore(snap), -7.5; absDiff(got, want) > 0.1 {
		t.Errorf("VIXBaseline=15: got %v, want ~%v", got, want)
	}

	snap.VIXBaseline = 25
	if got, want := synthesizeCompositeScore(snap), 0.0; absDiff(got, want) > 0.1 {
		t.Errorf("VIXBaseline=25 (above VIX): got %v, want ~%v", got, want)
	}

	snap.VIXBaseline = 0
	snap.VIX.Value = 20
	if got, want := synthesizeCompositeScore(snap), 0.0; absDiff(got, want) > 0.1 {
		t.Errorf("VIXBaseline=0 fallback: got %v, want ~%v", got, want)
	}
}

func TestEngine_UpdateFromMacro_StoresCompositeScore(t *testing.T) {
	e := NewEngine()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 2.0},
		VIX:                marketdata.MacroDataPoint{Value: 25},
	}
	e.UpdateFromMacro(snap)

	// tanh(0.4)*30 - 7.5 ≈ 11.49 - 7.5 = 3.99
	want := 3.99
	if got := e.GetCompositeScore(); absDiff(got, want) > 0.1 {
		t.Errorf("UpdateFromMacro: got %v, want ~%v", got, want)
	}
	if e.lastUpdated.IsZero() {
		t.Error("UpdateFromMacro should update lastUpdated")
	}
}

func TestEngine_GetCompositeScore_DefaultZero(t *testing.T) {
	e := NewEngine()
	if got := e.GetCompositeScore(); got != 0 {
		t.Errorf("GetCompositeScore before UpdateFromMacro: got %v, want 0", got)
	}
}

func TestEngine_UpdateFromMacro_OverwritesPreviousScore(t *testing.T) {
	e := NewEngine()
	e.UpdateFromMacro(marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	})
	first := e.GetCompositeScore()

	e.UpdateFromMacro(marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -1.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	})
	second := e.GetCompositeScore()

	if first == second {
		t.Errorf("UpdateFromMacro should overwrite: first=%v second=%v", first, second)
	}
	// tanh(0.2)*30 ≈ 5.92, tanh(-0.2)*30 ≈ -5.92
	if absDiff(first, 5.92) > 0.1 || absDiff(second, -5.92) > 0.1 {
		t.Errorf("first=%v (want ~5.92) second=%v (want ~-5.92)", first, second)
	}
}

func TestEngine_GetCurrentRegimeScore_FallsBackToSynthetic(t *testing.T) {
	e := NewEngine()
	e.EnsureAllRegimes()

	e.UpdateFromMacro(marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 5.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	})

	score, synthetic := e.GetCurrentRegimeScore()
	if !synthetic {
		t.Errorf("GetCurrentRegimeScore should fall back to synthetic, got synthetic=false")
	}
	// tanh(5/5)*30 = tanh(1)*30 ≈ 22.85 (tanh saturates; not 30)
	if score < 20 || score > 25 {
		t.Errorf("synthetic fallback should be ~22.85 (tanh saturated), got %v", score)
	}
}

func TestEngine_GetCurrentRegimeScore_BothZero(t *testing.T) {
	e := NewEngine()
	score, synthetic := e.GetCurrentRegimeScore()
	if score != 0 || synthetic {
		t.Errorf("both zero: got score=%v synthetic=%v, want 0/false", score, synthetic)
	}
}

func absDiff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// reference prism import keeps the test file compiling even if not used
// directly when individual subtests are skipped via -run flags.
var _ = prism.RegimeRiskOn
var _ = time.Second
