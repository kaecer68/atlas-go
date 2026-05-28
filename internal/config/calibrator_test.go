package config

import (
	"math"
	"testing"
	"time"
)

func TestComputeImprovementPct(t *testing.T) {
	tests := []struct {
		name     string
		baseline float64
		opt      float64
		want     float64
	}{
		// baseline > 0
		{"positive baseline - improvement", 10, 12, 20},
		{"positive baseline - regression", 10, 8, -20},
		{"positive baseline - no change", 10, 10, 0},
		{"positive baseline - large improvement", 100, 150, 50},
		{"positive baseline - small improvement", 1000, 1005, 0.5},

		// baseline < 0
		{"negative baseline - improvement (towards zero)", -10, -8, 20},
		{"negative baseline - regression (more negative)", -10, -12, -20},
		{"negative baseline - no change", -10, -10, 0},
		{"negative baseline - cross zero", -10, 5, 150},
		{"negative baseline - large improvement", -100, -50, 50},

		// baseline ≈ 0 (epsilon zone)
		{"near-zero positive baseline", 1e-11, 5, 500},
		{"near-zero negative baseline", -1e-11, -3, -300},
		{"baseline is exactly 0 - improvement", 0, 5, 500},
		{"baseline is exactly 0 - regression", 0, -3, -300},
		{"baseline is exactly 0 - no change", 0, 0, 0},

		// edge cases at epsilon boundary
		{"just above epsilon - improvement", 1e-9, 2e-9, 100},
		{"just below -epsilon - improvement", -1e-9, -0.5e-9, 50},

		// symmetry
		{"symmetric: +10→+12 and -10→-8", 10, 12, 20},
		{"same absolute improvement", 20, 24, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeImprovementPct(tt.baseline, tt.opt)
			tol := 1e-9
		if tt.baseline != 0 && math.Abs(tt.baseline) < 1e-9 {
			tol = 1e-6 // near-epsilon division yields larger float drift
		}
		if math.Abs(got-tt.want) > tol {
				t.Errorf("computeImprovementPct(%v, %v) = %v, want %v", tt.baseline, tt.opt, got, tt.want)
			}
		})
	}
}

func TestCalibrationConfidence(t *testing.T) {
	tests := []struct {
		name         string
		deltaPct     float64
		observations int
		want         string
	}{
		// high: obs >= 20 && abs > 5
		{"high - exact threshold", 5.001, 20, "high"},
		{"high - well above", 10, 30, "high"},
		{"high - negative delta", -5.001, 20, "high"},

		// NOT high: obs < 20 or delta <= 5
		{"not high - obs=19 delta=6", 6, 19, "medium"},
		{"not high - obs=20 delta=5", 5, 20, "medium"},
		{"not high - obs=20 delta=-5", -5, 20, "medium"},

		// medium: obs >= 10 && abs > 3
		{"medium - exact threshold", 3.001, 10, "medium"},
		{"medium - well above", 4, 15, "medium"},
		{"medium - negative delta", -3.001, 10, "medium"},

		// NOT medium: obs < 10 or delta <= 3
		{"not medium - obs=9 delta=4", 4, 9, "low"},
		{"not medium - obs=10 delta=3", 3, 10, "low"},
		{"not medium - obs=10 delta=-3", -3, 10, "low"},

		// low: everything else
		{"low - zero", 0, 0, "low"},
		{"low - small delta", 1, 5, "low"},
		{"low - many obs tiny delta", 0.1, 100, "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calibrationConfidence(tt.deltaPct, tt.observations)
			if got != tt.want {
				t.Errorf("calibrationConfidence(%v, %d) = %q, want %q", tt.deltaPct, tt.observations, got, tt.want)
			}
		})
	}
}

func TestMarkCalibrated(t *testing.T) {
	ts := time.Date(2026, 5, 28, 20, 0, 0, 0, time.UTC)

	// Verify a representative sample from each group.
	tests := []struct {
		name    string
		paramID string
		check   func(*ParametersConfig) *time.Time
	}{
		// MacroRisk
		{"macro risk - carry trade", "engine_macro_risk_carry_trade_unwind_threshold",
			func(c *ParametersConfig) *time.Time { return c.Engine.MacroRisk.CarryTradeUnwindThreshold.LastCalibrated }},
		{"macro risk - VIX", "engine_macro_risk_vix_threshold",
			func(c *ParametersConfig) *time.Time { return c.Engine.MacroRisk.VIXThreshold.LastCalibrated }},

		// StructuralTrend
		{"structural trend - min trend", "engine_structural_trend_min_trend_strength",
			func(c *ParametersConfig) *time.Time { return c.Engine.StructuralTrend.MinTrendStrength.LastCalibrated }},

		// Drawdown
		{"drawdown - orange", "engine_drawdown_orange_override_min_score",
			func(c *ParametersConfig) *time.Time { return c.Engine.Drawdown.OrangeOverrideMinScore.LastCalibrated }},

		// Executors
		{"executors - VIX crash", "engine_executors_vix_momentum_crash_threshold",
			func(c *ParametersConfig) *time.Time { return c.Engine.Executors.VIXMomentumCrashThreshold.LastCalibrated }},

		// Simulation
		{"simulation - neutral regime", "engine_simulation_neutral_regime_sizing_factor",
			func(c *ParametersConfig) *time.Time { return c.Engine.Simulation.NeutralRegimeSizingFactor.LastCalibrated }},

		// Narrative
		{"narrative - AI revenue", "narrative_ai_revenue_growth_threshold",
			func(c *ParametersConfig) *time.Time { return c.Narrative.AIRevenueGrowthThreshold.LastCalibrated }},

		// FactorWeight
		{"factor weight - conservative value", "factor_weight_conservative_value",
			func(c *ParametersConfig) *time.Time { return c.FactorWeight.ConservativeValue.LastCalibrated }},
	}

	method := "bayesian_optimization"
	names := make([]string, len(tests))
	for i, tt := range tests {
		names[i] = tt.paramID
	}

	cfg := DefaultParametersConfig()
	markCalibrated(cfg, names, method, &ts)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check(cfg)
			if got == nil || !got.Equal(ts) {
				t.Errorf("markCalibrated(%q): got LastCalibrated=%v, want %v", tt.paramID, got, ts)
			}
		})
	}
}

func TestMarkCalibrated_UnknownName(t *testing.T) {
	// Unknown names should not panic — markCalibrated falls through the switch.
	cfg := DefaultParametersConfig()
	ts := time.Now()
	markCalibrated(cfg, []string{"nonexistent_param"}, "test", &ts)
	// No assertion needed — if it doesn't panic, the test passes.
}

func TestMarkCalibrated_EmptyList(t *testing.T) {
	cfg := DefaultParametersConfig()
	ts := time.Now()
	markCalibrated(cfg, nil, "test", &ts)         // nil slice
	markCalibrated(cfg, []string{}, "test", &ts)   // empty slice
	// No assertion needed — if it doesn't panic, the test passes.
}
