package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestRiskExclusionFilter_DefaultsNoConfigure(t *testing.T) {
	rf := NewRiskExclusionFilter(nil, nil, nil)
	if rf.varThreshold != 2.0 {
		t.Errorf("default varThreshold = %v, want 2.0", rf.varThreshold)
	}
	if rf.volMultiplier != 2.0 {
		t.Errorf("default volMultiplier = %v, want 2.0", rf.volMultiplier)
	}
	if rf.drawdownWindow != 60 {
		t.Errorf("default drawdownWindow = %d, want 60", rf.drawdownWindow)
	}
	if rf.drawdownThreshold != 0.30 {
		t.Errorf("default drawdownThreshold = %v, want 0.30", rf.drawdownThreshold)
	}
	if rf.minDailyAmount != 5_000_000.0 {
		t.Errorf("default minDailyAmount = %v, want 5e6", rf.minDailyAmount)
	}
}

func TestRiskExclusionFilter_Configure(t *testing.T) {
	cfg := config.SmartUniverseConfig{
		VaRContributionMultiplier: config.ParameterMetadata[float64]{Value: 1.5},
		VolatilityMultiplier:      config.ParameterMetadata[float64]{Value: 1.8},
		DrawdownWindow:            config.ParameterMetadata[int]{Value: 90},
		DrawdownThreshold:         config.ParameterMetadata[float64]{Value: 0.25},
		MinDailyAmountTWD:         config.ParameterMetadata[float64]{Value: 3_000_000.0},
	}
	rf := NewRiskExclusionFilter(nil, nil, nil)
	rf.Configure(cfg)
	if rf.varThreshold != 1.5 {
		t.Errorf("varThreshold = %v, want 1.5", rf.varThreshold)
	}
	if rf.volMultiplier != 1.8 {
		t.Errorf("volMultiplier = %v, want 1.8", rf.volMultiplier)
	}
	if rf.drawdownWindow != 90 {
		t.Errorf("drawdownWindow = %d, want 90", rf.drawdownWindow)
	}
	if rf.drawdownThreshold != 0.25 {
		t.Errorf("drawdownThreshold = %v, want 0.25", rf.drawdownThreshold)
	}
	if rf.minDailyAmount != 3_000_000.0 {
		t.Errorf("minDailyAmount = %v, want 3e6", rf.minDailyAmount)
	}
}

func TestRiskExclusionFilter_ConfigureZeroValues(t *testing.T) {
	// A zero-value SmartUniverseConfig replaces thresholds with zero values
	// (not defaults). This test verifies the pass-through behavior.
	cfg := config.SmartUniverseConfig{}
	rf := NewRiskExclusionFilter(nil, nil, nil)
	rf.Configure(cfg)
	if rf.varThreshold != 0 {
		t.Errorf("varThreshold after zero-value Configure = %v, want 0", rf.varThreshold)
	}
	if rf.volMultiplier != 0 {
		t.Errorf("volMultiplier after zero-value Configure = %v, want 0", rf.volMultiplier)
	}
	if rf.minDailyAmount != 0 {
		t.Errorf("minDailyAmount after zero-value Configure = %v, want 0", rf.minDailyAmount)
	}
}

func TestRiskExclusionFilter_ConfigureNegativeValues(t *testing.T) {
	// Negative values pass through as-is (no validation in Configure).
	cfg := config.SmartUniverseConfig{
		VaRContributionMultiplier: config.ParameterMetadata[float64]{Value: -1.0},
		VolatilityMultiplier:      config.ParameterMetadata[float64]{Value: -0.5},
		MinDailyAmountTWD:         config.ParameterMetadata[float64]{Value: -1000.0},
	}
	rf := NewRiskExclusionFilter(nil, nil, nil)
	rf.Configure(cfg)
	if rf.varThreshold != -1.0 {
		t.Errorf("varThreshold with negative = %v, want -1.0", rf.varThreshold)
	}
	if rf.volMultiplier != -0.5 {
		t.Errorf("volMultiplier with negative = %v, want -0.5", rf.volMultiplier)
	}
	if rf.minDailyAmount != -1000.0 {
		t.Errorf("minDailyAmount with negative = %v, want -1000.0", rf.minDailyAmount)
	}
}
