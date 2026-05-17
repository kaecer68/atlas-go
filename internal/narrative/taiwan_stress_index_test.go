package narrative

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTaiwanStressCalculator_Calculate(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil)

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         1713000000,
	}
	geo := GeopoliticalRiskScore{Intensity: 30}

	idx := calc.Calculate(snap, geo)

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
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "geopolitical"}
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
	calc := NewTaiwanStressCalculator(mock)
	calc.cacheTTL = 100 * time.Millisecond

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: 1},
		US10Y:              marketdata.MacroDataPoint{Value: 1},
		VIX:                marketdata.MacroDataPoint{Value: 10},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		RecordedAt:         1713000000,
	}

	ctx := context.Background()
	if _, err := calc.CalculateFromSnapshot(ctx, snap); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 provider call after first invocation, got %d", mock.calls)
	}

	if _, err := calc.CalculateFromSnapshot(ctx, snap); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected cached result, but provider was called again (calls=%d)", mock.calls)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := calc.CalculateFromSnapshot(ctx, snap); err != nil {
		t.Fatalf("third call after ttl failed: %v", err)
	}
	if mock.calls != 2 {
		t.Fatalf("expected provider to be called after ttl expired, got %d calls", mock.calls)
	}
}

func TestTaiwanStressCalculator_RegimeThresholds(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil)

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
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 30}, VIX: marketdata.MacroDataPoint{Value: 0}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  30,
			want: "alert",
		},
		{
			name: "high",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 40}, VIX: marketdata.MacroDataPoint{Value: 40}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
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
		idx := calc.Calculate(tt.snap, geo)
		if idx.Regime != tt.want {
			t.Fatalf("%s: expected regime %q, got %q (score=%.1f)", tt.name, tt.want, idx.Regime, idx.Score)
		}
	}
}
