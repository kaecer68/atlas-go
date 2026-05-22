package risk

import (
	"testing"
)

func TestPostTradeGate_DrawdownHalt(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.25, // 25% > 20% halt threshold
		RollingSharpe:      0.5,
		ConsecutiveLosses:  2,
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictHalt {
		t.Errorf("expected HALT for 25%% drawdown, got %s", dec.Verdict)
	}
	if dec.Mode != string(ModeSuspended) {
		t.Errorf("expected SUSPENDED mode, got %s", dec.Mode)
	}
	assertRulePassed(t, dec, "max_drawdown_halt", false)
}

func TestPostTradeGate_DrawdownDefensive(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.15, // 15% > 10% defensive threshold
		RollingSharpe:      0.5,
		ConsecutiveLosses:  0,
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict < VerdictAlertOnly {
		t.Errorf("expected alert for 15%% drawdown, got %s", dec.Verdict)
	}
	if dec.Mode != string(ModeDefensive) {
		t.Errorf("expected DEFENSIVE mode, got %s", dec.Mode)
	}
}

func TestPostTradeGate_DrawdownNormal(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.03, // 3% within limits
		RollingSharpe:      0.5,
		ConsecutiveLosses:  0,
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "max_drawdown", true)
}

func TestPostTradeGate_SharpeBelowZero(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.05,
		RollingSharpe:      -0.5, // Negative Sharpe
		ConsecutiveLosses:  0,
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "rolling_sharpe", false)
}

func TestPostTradeGate_ConsecutiveLosses(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.03,
		RollingSharpe:      0.2,
		ConsecutiveLosses:  7, // >= 5
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "consecutive_losses", false)
}

func TestPostTradeGate_AllNormal(t *testing.T) {
	g := NewPostTradeGate()
	input := PostTradeInput{
		CurrentDrawdownPct: 0.03,
		RollingSharpe:      0.8,
		ConsecutiveLosses:  1,
	}

	dec, err := g.Evaluate(input, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAlertOnly {
		t.Errorf("expected ALERT_ONLY for normal metrics, got %s", dec.Verdict)
	}
}

func TestCheckModeTransition(t *testing.T) {
	tests := []struct {
		name        string
		input       PostTradeInput
		currentMode string
		wantMode    string
		wantChange  bool
	}{
		{"drawdown halt", PostTradeInput{CurrentDrawdownPct: 0.25, RollingSharpe: 0, ConsecutiveLosses: 0}, "NORMAL", string(ModeSuspended), true},
		{"drawdown defensive", PostTradeInput{CurrentDrawdownPct: 0.12, RollingSharpe: 0, ConsecutiveLosses: 0}, "NORMAL", string(ModeDefensive), true},
		{"negative sharpe", PostTradeInput{CurrentDrawdownPct: 0.03, RollingSharpe: -0.3, ConsecutiveLosses: 0}, "NORMAL", string(ModeCautious), true},
		{"consecutive losses", PostTradeInput{CurrentDrawdownPct: 0.03, RollingSharpe: 0.5, ConsecutiveLosses: 5}, "NORMAL", string(ModeCautious), true},
		{"all normal", PostTradeInput{CurrentDrawdownPct: 0.03, RollingSharpe: 0.8, ConsecutiveLosses: 1}, "NORMAL", "NORMAL", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, _ := CheckModeTransition(tt.input, tt.currentMode)
			changed := mode != tt.currentMode
			if changed != tt.wantChange {
				t.Errorf("CheckModeTransition(%+v, %s) mode=%s, want change=%v", tt.input, tt.currentMode, mode, tt.wantChange)
			}
			if mode != tt.wantMode {
				t.Errorf("CheckModeTransition() = %s, want %s", mode, tt.wantMode)
			}
		})
	}
}

func TestNewPostTradeGate_ConfigValues(t *testing.T) {
	g := NewPostTradeGate()
	// These come from parameters_defaults.go
	if g.maxDrawdownHaltPct <= 0 {
		t.Error("maxDrawdownHaltPct should be > 0")
	}
	if g.maxDrawdownDefensivePct <= 0 {
		t.Error("maxDrawdownDefensivePct should be > 0")
	}
	if g.consecutiveLossDays < 1 {
		t.Error("consecutiveLossDays should be >= 1")
	}
	// Verify ordering: defensive < halt
	if g.maxDrawdownDefensivePct >= g.maxDrawdownHaltPct {
		t.Errorf("defensive %.2f should be < halt %.2f", g.maxDrawdownDefensivePct, g.maxDrawdownHaltPct)
	}
}
