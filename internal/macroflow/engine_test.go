package macroflow

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func ts(offset time.Duration) int64 {
	return time.Now().Add(-offset).Unix()
}

func snapshot(vix float64, recorded int64) *marketdata.MacroDataSnapshot {
	return &marketdata.MacroDataSnapshot{
		VIX:        marketdata.MacroDataPoint{Value: vix},
		RecordedAt: recorded,
	}
}

func TestEngine_Compute_8Scenarios(t *testing.T) {
	tests := []struct {
		name       string
		level      RiskLevel
		vix        float64
		wantDef    float64
		wantAgg    float64
		wantCash   float64
		wantStress bool
	}{
		{"yellow_calm", RiskYellow, 20, 5, 0, 0, false},
		{"yellow_stress", RiskYellow, 40, 20, -15, 0, true},
		{"orange_calm", RiskOrange, 25, 15, -10, 0, false},
		{"orange_stress", RiskOrange, 38, 20, -20, 5, true},
		{"red_calm", RiskRed, 30, 20, -25, 10, false},
		{"red_stress", RiskRed, 45, 25, -30, 15, true},
		{"yellow_near_boundary", RiskYellow, 34.9, 5, 0, 0, false},
		{"yellow_at_threshold", RiskYellow, 35, 20, -15, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := NewEngine(7 * 24 * time.Hour)
			snap := snapshot(tt.vix, ts(1*time.Hour))
			result := eng.Compute(snap, tt.level)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.RiskLevel != tt.level {
				t.Errorf("RiskLevel = %q, want %q", result.RiskLevel, tt.level)
			}
			if result.IsStress != tt.wantStress {
				t.Errorf("IsStress = %v, want %v", result.IsStress, tt.wantStress)
			}
			if result.Adjustment.Defensive != tt.wantDef {
				t.Errorf("Defensive = %.1f, want %.1f", result.Adjustment.Defensive, tt.wantDef)
			}
			if result.Adjustment.Aggressive != tt.wantAgg {
				t.Errorf("Aggressive = %.1f, want %.1f", result.Adjustment.Aggressive, tt.wantAgg)
			}
			if result.Adjustment.Cash != tt.wantCash {
				t.Errorf("Cash = %.1f, want %.1f", result.Adjustment.Cash, tt.wantCash)
			}
			if len(result.Reasoning) == 0 {
				t.Error("expected non-empty Reasoning")
			}
		})
	}
}

func TestEngine_Compute_StaleData(t *testing.T) {
	eng := NewEngine(7 * 24 * time.Hour)
	snap := snapshot(25, ts(10*24*time.Hour)) // 10 days old > 7 day max
	result := eng.Compute(snap, RiskYellow)
	if result != nil {
		t.Error("expected nil for stale data")
	}
}

func TestEngine_Compute_EmptySnapshot(t *testing.T) {
	eng := NewEngine(7 * 24 * time.Hour)
	result := eng.Compute(nil, RiskYellow)
	if result != nil {
		t.Error("expected nil for nil snapshot")
	}
}

func TestEngine_Compute_ReasoningNonEmpty(t *testing.T) {
	eng := NewEngine(7 * 24 * time.Hour)
	snap := snapshot(25, ts(1*time.Hour))
	result := eng.Compute(snap, RiskOrange)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Reasoning) != 1 {
		t.Errorf("expected 1 reasoning entry, got %d", len(result.Reasoning))
	}
	if result.Reasoning[0] != "orange: defensiva +15%, aggressive -10% — high risk regime" {
		t.Errorf("unexpected reasoning: %q", result.Reasoning[0])
	}
}
