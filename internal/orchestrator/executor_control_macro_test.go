package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

func TestApplyMacroConvictionScaling_NilAdjustment(t *testing.T) {
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Conviction: 80},
		{Symbol: "2317.TW", Conviction: 50},
	}
	out := applyMacroConvictionScaling(recs, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 recs unchanged, got %d", len(out))
	}
	if out[0].Conviction != 80 || out[1].Conviction != 50 {
		t.Errorf("expected convictions unchanged when adj is nil, got %d / %d", out[0].Conviction, out[1].Conviction)
	}
}

func TestApplyMacroConvictionScaling_EmptyRecs(t *testing.T) {
	adj := &macroflow.AdjustmentResult{
		Adjustment: macroflow.Adjustment{Defensive: 20, Aggressive: -15},
	}
	out := applyMacroConvictionScaling(nil, adj)
	if len(out) != 0 {
		t.Errorf("expected empty slice for nil recs, got %d", len(out))
	}
	out = applyMacroConvictionScaling([]domain.Recommendation{}, adj)
	if len(out) != 0 {
		t.Errorf("expected empty slice for empty recs, got %d", len(out))
	}
}

func TestApplyMacroConvictionScaling_ReducesOnConservativeBias(t *testing.T) {
	adj := &macroflow.AdjustmentResult{
		Adjustment: macroflow.Adjustment{Defensive: 20, Aggressive: -15, Cash: 0},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Conviction: 100},
		{Symbol: "2317.TW", Conviction: 50},
		{Symbol: "2454.TW", Conviction: 0},
	}
	out := applyMacroConvictionScaling(recs, adj)
	conservative := 20.0 + 0.0 + 15.0
	net := 0.0 - conservative
	wantScale := 1.0 + net/100.0*0.3
	wantHigh := clampConvictionInt(int(100 * wantScale))
	wantMid := clampConvictionInt(int(50 * wantScale))
	wantZero := clampConvictionInt(int(0 * wantScale))
	if out[0].Conviction != wantHigh {
		t.Errorf("Conviction[100] = %d, want %d (scale=%.3f)", out[0].Conviction, wantHigh, wantScale)
	}
	if out[1].Conviction != wantMid {
		t.Errorf("Conviction[50] = %d, want %d (scale=%.3f)", out[1].Conviction, wantMid, wantScale)
	}
	if out[2].Conviction != wantZero {
		t.Errorf("Conviction[0] = %d, want 0", out[2].Conviction)
	}
}

func TestApplyMacroConvictionScaling_BoostsOnAggressiveBias(t *testing.T) {
	adj := &macroflow.AdjustmentResult{
		Adjustment: macroflow.Adjustment{Defensive: -10, Aggressive: 20, Cash: 0},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Conviction: 100},
	}
	out := applyMacroConvictionScaling(recs, adj)
	conservative := 0.0 + 0.0 + 0.0
	riskOn := 20.0
	net := riskOn - conservative
	wantScale := 1.0 + net/100.0*0.3
	want := clampConvictionInt(int(100 * wantScale))
	if out[0].Conviction != want {
		t.Errorf("Conviction[100] = %d, want %d (scale=%.3f)", out[0].Conviction, want, wantScale)
	}
}

func TestApplyMacroConvictionScaling_ScaleClampedToBounds(t *testing.T) {
	adj := &macroflow.AdjustmentResult{
		Adjustment: macroflow.Adjustment{Defensive: 200, Aggressive: -100, Cash: 100},
	}
	recs := []domain.Recommendation{{Symbol: "x", Conviction: 100}}
	out := applyMacroConvictionScaling(recs, adj)
	if out[0].Conviction != 50 {
		t.Errorf("expected conviction floor at 50%% scaling (100*0.5=50), got %d", out[0].Conviction)
	}
}

func TestApplyMacroConvictionScaling_PreservesOtherFields(t *testing.T) {
	adj := &macroflow.AdjustmentResult{
		Adjustment: macroflow.Adjustment{Defensive: 10, Aggressive: -5},
	}
	rec := domain.Recommendation{
		Agent: "a", Skill: "s", Symbol: "x", Conviction: 60, Side: domain.SideBuy,
		Reason: "r", TargetPrice: 100.5, StopLossPrice: 90.5,
	}
	out := applyMacroConvictionScaling([]domain.Recommendation{rec}, adj)
	if len(out) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(out))
	}
	got := out[0]
	if got.Agent != "a" || got.Skill != "s" || got.Symbol != "x" {
		t.Errorf("identity fields lost: %+v", got)
	}
	if got.Side != domain.SideBuy {
		t.Errorf("side lost: %v", got.Side)
	}
	if got.Reason != "r" || got.TargetPrice != 100.5 || got.StopLossPrice != 90.5 {
		t.Errorf("payload lost: %+v", got)
	}
}
