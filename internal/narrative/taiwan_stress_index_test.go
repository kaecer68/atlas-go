package narrative

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTaiwanStressCalculator_Calculate(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         1713000000,
	}
	geo := GeopoliticalRiskScore{Intensity: 30}

	idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geo)

	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range: %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	if idx.Timestamp != snap.RecordedAt {
		t.Fatalf("timestamp mismatch: got %v, want %v", idx.Timestamp, snap.RecordedAt)
	}

	// Verify component contributions exist.
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "geopolitical", "oil", "gold"}
	for _, k := range expectedKeys {
		if _, ok := idx.Components[k]; !ok {
			t.Fatalf("missing component %s", k)
		}
	}
}

type mockGeoProvider struct {
	calls int
	score GeopoliticalRiskScore
}

func (m *mockGeoProvider) Name() string { return "mock" }
func (m *mockGeoProvider) FetchScore(ctx context.Context) (GeopoliticalRiskScore, error) {
	m.calls++
	return m.score, nil
}

func TestTaiwanStressCalculator_CalculateFromSnapshot_CachesResult(t *testing.T) {
	mock := &mockGeoProvider{score: GeopoliticalRiskScore{Intensity: 10}}
	calc := NewTaiwanStressCalculator(mock, "")
	calc.cacheTTL = 100 * time.Millisecond

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: 1},
		US10Y:              marketdata.MacroDataPoint{Value: 1},
		VIX:                marketdata.MacroDataPoint{Value: 10},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		RecordedAt:         1713000000,
	}

	ctx := context.Background()
	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 provider call after first invocation, got %d", mock.calls)
	}

	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected cached result, but provider was called again (calls=%d)", mock.calls)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("third call after ttl failed: %v", err)
	}
	if mock.calls != 2 {
		t.Fatalf("expected provider to be called after ttl expired, got %d calls", mock.calls)
	}
}

func TestTaiwanStressCalculator_RegimeThresholds(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")

	tests := []struct {
		name string
		snap marketdata.MacroDataSnapshot
		geo  float64
		want string
	}{
		{
			name: "low",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 8.5}, US10Y: marketdata.MacroDataPoint{Value: 0}, VIX: marketdata.MacroDataPoint{Value: 0}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  10,
			want: "low",
		},
		{
			name: "alert",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 45}, VIX: marketdata.MacroDataPoint{Value: 15}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2}, Oil: marketdata.MacroDataPoint{ChangePct: 5}, Gold: marketdata.MacroDataPoint{ChangePct: 3}},
			geo:  30,
			want: "alert",
		},
		{
			name: "high",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 60}, VIX: marketdata.MacroDataPoint{Value: 35}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -6}, Oil: marketdata.MacroDataPoint{ChangePct: 10}, Gold: marketdata.MacroDataPoint{ChangePct: 5}},
			geo:  60,
			want: "high",
		},
		{
			name: "crisis",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 80}, VIX: marketdata.MacroDataPoint{Value: 80}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -10}},
			geo:  100,
			want: "crisis",
		},
	}

	for _, tt := range tests {
		geo := GeopoliticalRiskScore{Intensity: tt.geo}
		idx := calc.Calculate(tt.snap, marketdata.MacroDataSnapshot{}, geo)
		if idx.Regime != tt.want {
			t.Fatalf("%s: expected regime %q, got %q (score=%.1f)", tt.name, tt.want, idx.Regime, idx.Score)
		}
	}
}

func TestLoadWeightsConfigFromParameters(t *testing.T) {
	cfg := loadWeightsFromParameters()
	if cfg == nil {
		t.Fatal("expected non-nil config from parameters system")
	}
	if !cfg.isValid() {
		t.Fatal("expected valid config (weights sum to 1.0)")
	}
	expected := defaultCalibrationWeights()
	if cfg.Weights != expected {
		t.Fatalf("weights mismatch: got %+v, want %+v", cfg.Weights, expected)
	}
}

func TestLoadWeightsConfigFileNotFound(t *testing.T) {
	cfg := LoadWeightsConfig("/nonexistent/dir")
	if cfg == nil {
		t.Fatal("expected non-nil config from parameters system even when workDir doesn't exist")
	}
}

func TestLoadWeightsConfigIntegratesWithCalculator(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")
	snap := marketdata.MacroDataSnapshot{
		DXY:   marketdata.MacroDataPoint{Value: 104, ChangePct: 1.0},
		US10Y: marketdata.MacroDataPoint{Value: 4.5},
		VIX:   marketdata.MacroDataPoint{Value: 20},
	}
	geo := GeopoliticalRiskScore{Intensity: 30}
	idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geo)
	if idx.Score < 0 {
		t.Fatal("expected positive score with parameters config")
	}
}

func TestGetCurrentStressIndex(t *testing.T) {
	eng := NewNarrativeEngine()

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		Oil:                marketdata.MacroDataPoint{ChangePct: 1.5},
		Gold:               marketdata.MacroDataPoint{ChangePct: 0.8},
		JPY:                marketdata.MacroDataPoint{ChangePct: -0.3},
		RecordedAt:         time.Now().Unix(),
	}
	geo := GeopoliticalRiskScore{Intensity: 30}
	eng.UpdateMacro(snap, geo)

	idx := eng.GetCurrentStressIndex()

	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range [0,100]: got %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	for _, k := range expectedKeys {
		if _, ok := idx.Components[k]; !ok {
			t.Fatalf("missing component %s", k)
		}
	}
	if idx.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestGetStressIndexHistory(t *testing.T) {
	eng := NewNarrativeEngine()
	baseTime := time.Now().Unix()

	for i := range int64(3) {
		snap := marketdata.MacroDataSnapshot{
			DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: float64(i) * 0.5},
			US10Y:              marketdata.MacroDataPoint{Value: 4.5 + float64(i)*0.1},
			VIX:                marketdata.MacroDataPoint{Value: 20 + float64(i)*2},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5 - float64(i)},
			RecordedAt:         baseTime + i,
		}
		eng.UpdateMacro(snap, GeopoliticalRiskScore{Intensity: 30})
		eng.GetCurrentStressIndex()
	}

	t.Run("returns exact limit", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(2)
		if len(hist) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(hist))
		}
	})

	t.Run("returns min of history and limit", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(10)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (history has 3), got %d", len(hist))
		}
	})

	t.Run("limit zero defaults to 30", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(0)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (default 30, history has 3), got %d", len(hist))
		}
	})

	t.Run("negative limit defaults to 30", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(-1)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (default 30 for negative, history has 3), got %d", len(hist))
		}
	})

	t.Run("empty history returns empty slice", func(t *testing.T) {
		eng2 := NewNarrativeEngine()
		hist := eng2.GetStressIndexHistory(10)
		if len(hist) != 0 {
			t.Fatalf("expected 0 entries for empty history, got %d", len(hist))
		}
	})
}

func TestGetStressIndexThresholds(t *testing.T) {
	eng := NewNarrativeEngine()
	th := eng.GetStressIndexThresholds()

	if th.Crisis <= th.High {
		t.Fatalf("expected Crisis > High, got Crisis=%v High=%v", th.Crisis, th.High)
	}
	if th.High <= th.Alert {
		t.Fatalf("expected High > Alert, got High=%v Alert=%v", th.High, th.Alert)
	}
	if th.Alert <= 0 {
		t.Fatalf("expected Alert > 0, got Alert=%v", th.Alert)
	}
	if th.Crisis == 0 || th.High == 0 || th.Alert == 0 {
		t.Fatal("expected non-zero threshold values")
	}

	t.Run("nil stressCalc returns empty struct", func(t *testing.T) {
		eng2 := NewNarrativeEngine()
		eng2.stressCalc = nil
		th := eng2.GetStressIndexThresholds()
		if th.Crisis != 0 || th.High != 0 || th.Alert != 0 {
			t.Fatalf("expected zero thresholds for nil stressCalc, got %+v", th)
		}
	})
}
