package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

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
