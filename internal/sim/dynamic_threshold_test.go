package sim

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewDynamicThresholdEngine(t *testing.T) {
	e := NewDynamicThresholdEngine()
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
	if e.baseThreshold != 0.70 {
		t.Errorf("expected base threshold 0.70, got %f", e.baseThreshold)
	}
	if e.minThreshold != 0.40 {
		t.Errorf("expected min threshold 0.40, got %f", e.minThreshold)
	}
	if e.maxThreshold != 0.85 {
		t.Errorf("expected max threshold 0.85, got %f", e.maxThreshold)
	}
	if e.currentRegime != RegimeNeutral {
		t.Errorf("expected neutral regime, got %s", e.currentRegime)
	}
	if e.riskAppetite != RiskAppetiteBalanced {
		t.Errorf("expected balanced risk appetite, got %d", e.riskAppetite)
	}
}

func TestDynamicThresholdEngine_GetThreshold(t *testing.T) {
	tests := []struct {
		name    string
		vix     float64
		regime  RegimeType
		wantMin float64
		wantMax float64
	}{
		{"neutral low vix", 15, RegimeNeutral, 0.40, 0.70},
		{"neutral high vix", 35, RegimeNeutral, 0.70, 0.85},
		{"bull low vix", 10, RegimeBull, 0.40, 0.60},
		{"bear high vix", 30, RegimeBear, 0.75, 0.85},
		{"high vol", 40, RegimeHighVol, 0.80, 0.85},
	}

	for _, tt := range tests {
		e := NewDynamicThresholdEngine()
		got := e.GetThreshold(tt.vix, tt.regime)
		if got < tt.wantMin {
			t.Errorf("%s: threshold %.4f below min %.4f", tt.name, got, tt.wantMin)
		}
		if got > tt.wantMax {
			t.Errorf("%s: threshold %.4f above max %.4f", tt.name, got, tt.wantMax)
		}
	}
}

func TestDynamicThresholdEngine_GetThreshold_Clamped(t *testing.T) {
	e := NewDynamicThresholdEngine()
	// VIX of 100 should push threshold high, but max is 0.85
	got := e.GetThreshold(100, RegimeNeutral)
	if got != e.maxThreshold {
		t.Errorf("expected threshold clamped to max %.4f, got %.4f", e.maxThreshold, got)
	}
}

func TestDynamicThresholdEngine_GetThreshold_ClampedLow(t *testing.T) {
	e := NewDynamicThresholdEngine()
	e.baseThreshold = 0.50 // Lower base to test min clamp
	e.minThreshold = 0.50
	// VIX of 0 should push threshold down
	got := e.GetThreshold(0, RegimeBull)
	if got < e.minThreshold {
		t.Errorf("expected threshold clamped to min %.4f, got %.4f", e.minThreshold, got)
	}
}

func TestDynamicThresholdEngine_DetectRegime(t *testing.T) {
	e := NewDynamicThresholdEngine()

	tests := []struct {
		indicators MarketIndicators
		want       RegimeType
	}{
		{MarketIndicators{VIX: 35, SPXTrend: 0}, RegimeHighVol},
		{MarketIndicators{VIX: 31, SPXTrend: 1}, RegimeHighVol}, // VIX > 30 always highvol
		{MarketIndicators{VIX: 28, SPXTrend: -1}, RegimeBear},
		{MarketIndicators{VIX: 28, SPXTrend: 1}, RegimeNeutral}, // VIX < 30, trend not negative
		{MarketIndicators{VIX: 10, SPXTrend: 1}, RegimeBull},
		{MarketIndicators{VIX: 10, SPXTrend: -1}, RegimeNeutral},
		{MarketIndicators{VIX: 20, SPXTrend: 0}, RegimeNeutral},
		{MarketIndicators{VIX: 25, SPXTrend: 1}, RegimeNeutral},  // VIX=25 not > 25, so neutral
		{MarketIndicators{VIX: 26, SPXTrend: 1}, RegimeNeutral},  // VIX > 25 but trend not < 0
		{MarketIndicators{VIX: 15, SPXTrend: 1}, RegimeNeutral},  // VIX not < 15
		{MarketIndicators{VIX: 14, SPXTrend: -1}, RegimeNeutral}, // VIX < 15 but trend not > 0
	}

	for _, tt := range tests {
		got := e.DetectRegime(tt.indicators)
		if got != tt.want {
			t.Errorf("DetectRegime({VIX: %.0f, SPXTrend: %.0f}) = %s, want %s",
				tt.indicators.VIX, tt.indicators.SPXTrend, got, tt.want)
		}
	}
}

func TestDynamicThresholdEngine_GetCurrentThreshold(t *testing.T) {
	e := NewDynamicThresholdEngine()
	// Set via GetThreshold first
	e.GetThreshold(25, RegimeBull)
	got := e.GetCurrentThreshold()

	// Should be base + vix_adj + regime_adj (no appetite adj for balanced)
	expected := 0.70 + (25.0-20.0)/100.0 - 0.05
	expected = math.Max(expected, e.minThreshold)
	expected = math.Min(expected, e.maxThreshold)
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("GetCurrentThreshold() = %.4f, want %.4f", got, expected)
	}
}

func TestRegimeFromDomain(t *testing.T) {
	tests := []struct {
		regime domain.Regime
		want   RegimeType
	}{
		{domain.RegimeRiskOn, RegimeBull},
		{domain.RegimeRiskOff, RegimeBear},
		{domain.Regime("unknown"), RegimeNeutral},
		{domain.Regime(""), RegimeNeutral},
	}

	for _, tt := range tests {
		got := RegimeFromDomain(tt.regime)
		if got != tt.want {
			t.Errorf("RegimeFromDomain(%s) = %s, want %s", tt.regime, got, tt.want)
		}
	}
}

func TestDynamicThresholdEngine_GetRegimeMultiplier(t *testing.T) {
	e := NewDynamicThresholdEngine()

	tests := []struct {
		regime RegimeType
		want   float64
	}{
		{RegimeBull, -0.05},
		{RegimeBear, 0.10},
		{RegimeNeutral, 0.00},
		{RegimeHighVol, 0.15},
	}

	for _, tt := range tests {
		got := e.GetRegimeMultiplier(tt.regime)
		if got != tt.want {
			t.Errorf("GetRegimeMultiplier(%s) = %.2f, want %.2f", tt.regime, got, tt.want)
		}
	}
}

func TestDynamicThresholdEngine_SetBaseThreshold(t *testing.T) {
	e := NewDynamicThresholdEngine()
	e.SetBaseThreshold(0.60)

	if e.baseThreshold != 0.60 {
		t.Errorf("expected base 0.60, got %f", e.baseThreshold)
	}
}

func TestDynamicThresholdEngine_SetAndGetRiskAppetite(t *testing.T) {
	e := NewDynamicThresholdEngine()

	tests := []RiskAppetite{
		RiskAppetiteConservative,
		RiskAppetiteBalanced,
		RiskAppetiteAggressive,
	}

	for _, ra := range tests {
		e.SetRiskAppetite(ra)
		got := e.GetRiskAppetite()
		if got != ra {
			t.Errorf("Set/Get for %d: got %d", ra, got)
		}
	}
}

func TestDynamicThresholdEngine_AppetiteAdjustment(t *testing.T) {
	e := NewDynamicThresholdEngine()

	// Conservative should increase threshold
	e.SetRiskAppetite(RiskAppetiteConservative)
	conservativeThreshold := e.GetThreshold(20, RegimeNeutral)

	// Balanced is neutral
	e.SetRiskAppetite(RiskAppetiteBalanced)
	balancedThreshold := e.GetThreshold(20, RegimeNeutral)

	// Aggressive should decrease threshold
	e.SetRiskAppetite(RiskAppetiteAggressive)
	aggressiveThreshold := e.GetThreshold(20, RegimeNeutral)

	if conservativeThreshold <= balancedThreshold {
		t.Errorf("conservative (%.4f) should be > balanced (%.4f)", conservativeThreshold, balancedThreshold)
	}
	if aggressiveThreshold >= balancedThreshold {
		t.Errorf("aggressive (%.4f) should be < balanced (%.4f)", aggressiveThreshold, balancedThreshold)
	}
}

func TestDynamicThresholdEngine_GetStats(t *testing.T) {
	e := NewDynamicThresholdEngine()
	e.GetThreshold(25, RegimeBull)

	stats := e.GetStats()

	requiredKeys := []string{"base_threshold", "min_threshold", "max_threshold", "current_regime", "last_vix", "current_threshold", "update_count"}
	for _, key := range requiredKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("GetStats missing key: %s", key)
		}
	}

	if stats["update_count"].(int) != 1 {
		t.Errorf("expected update_count 1, got %v", stats["update_count"])
	}
	if stats["current_regime"].(RegimeType) != RegimeBull {
		t.Errorf("expected regime bull, got %v", stats["current_regime"])
	}
}

func TestDynamicThresholdEngine_UpdateCount(t *testing.T) {
	e := NewDynamicThresholdEngine()

	for i := range 5 {
		e.GetThreshold(20, RegimeNeutral)
		stats := e.GetStats()
		if stats["update_count"].(int) != i+1 {
			t.Errorf("expected update_count %d, got %v", i+1, stats["update_count"])
		}
	}
}

func TestRiskAppetiteConstants(t *testing.T) {
	appetites := []RiskAppetite{
		RiskAppetiteConservative,
		RiskAppetiteBalanced,
		RiskAppetiteAggressive,
	}
	seen := make(map[RiskAppetite]bool)
	for _, a := range appetites {
		if a < 1 || a > 3 {
			t.Errorf("invalid risk appetite value: %d", a)
		}
		if seen[a] {
			t.Errorf("duplicate risk appetite: %d", a)
		}
		seen[a] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique risk appetite values, got %d", len(seen))
	}
}

func TestRegimeTypeConstants(t *testing.T) {
	regimes := []RegimeType{
		RegimeBull,
		RegimeBear,
		RegimeNeutral,
		RegimeHighVol,
	}
	seen := make(map[RegimeType]bool)
	for _, r := range regimes {
		if r == "" {
			t.Error("regime type should not be empty")
		}
		if seen[r] {
			t.Errorf("duplicate regime type: %s", r)
		}
		seen[r] = true
	}
}
