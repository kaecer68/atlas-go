package janus

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestSynthesizeCompositeScore_Bullish(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.5},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	}
	if got, want := synthesizeCompositeScore(snap), 30.0; got != want {
		t.Errorf("bullish (foreign=+1.5, VIX=15): got %v, want %v", got, want)
	}
}

func TestSynthesizeCompositeScore_BullishWithPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.5},
		VIX:                marketdata.MacroDataPoint{Value: 40},
	}
	if got, want := synthesizeCompositeScore(snap), 20.0; got != want {
		t.Errorf("bullish+panic (foreign=+1.5, VIX=40): got %v, want %v (30 - 10*0.5)", got, want)
	}
}

func TestSynthesizeCompositeScore_Bearish(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	}
	if got, want := synthesizeCompositeScore(snap), -30.0; got != want {
		t.Errorf("bearish (foreign=-2.0, VIX=15): got %v, want %v", got, want)
	}
}

func TestSynthesizeCompositeScore_BearishWithPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2.0},
		VIX:                marketdata.MacroDataPoint{Value: 50},
	}
	if got, want := synthesizeCompositeScore(snap), -45.0; got != want {
		t.Errorf("bearish+panic (foreign=-2.0, VIX=50): got %v, want %v (-30 - 15)", got, want)
	}
}

func TestSynthesizeCompositeScore_NeutralFlowNoPanic(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		VIX:                marketdata.MacroDataPoint{Value: 18},
	}
	if got, want := synthesizeCompositeScore(snap), 0.0; got != want {
		t.Errorf("neutral (foreign=0, VIX=18<20): got %v, want %v", got, want)
	}
}

func TestSynthesizeCompositeScore_VIXAtBaselineNoPenalty(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 1.0},
		VIX:                marketdata.MacroDataPoint{Value: 20},
	}
	if got, want := synthesizeCompositeScore(snap), 30.0; got != want {
		t.Errorf("VIX=20 boundary: got %v, want %v (no panic penalty below or at 20)", got, want)
	}
}

func TestEngine_UpdateFromMacro_StoresCompositeScore(t *testing.T) {
	e := NewEngine()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 2.0},
		VIX:                marketdata.MacroDataPoint{Value: 25},
	}
	e.UpdateFromMacro(snap)

	want := 30.0 - (25-20)*0.5
	if got := e.GetCompositeScore(); got != want {
		t.Errorf("UpdateFromMacro should store synthesized score: got %v, want %v", got, want)
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
		t.Errorf("UpdateFromMacro should overwrite: first=%v second=%v (must differ)", first, second)
	}
	if second != -30.0 {
		t.Errorf("second call (foreign=-1.0): got %v, want -30", second)
	}
}
