package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestDefaultFallbackParams_RespectsConfig(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	cfg.ForwardReturn.RiskOnMean.Value = 0.0
	cfg.ForwardReturn.RiskOffMean.Value = 0.0
	cfg.ForwardReturn.RiskOnStdDev.Value = 0.02
	cfg.ForwardReturn.RiskOffStdDev.Value = 0.015

	fallback := DefaultFallbackParams(cfg)

	if fallback.RiskOnParams.Mean != 0.0 {
		t.Errorf("RiskOn Mean should reflect config (0.0), got %f", fallback.RiskOnParams.Mean)
	}
	if fallback.RiskOffParams.Mean != 0.0 {
		t.Errorf("RiskOff Mean should reflect config (0.0), got %f", fallback.RiskOffParams.Mean)
	}
	if fallback.RiskOnParams.StdDev != 0.02 {
		t.Errorf("RiskOn StdDev should reflect config (0.02), got %f", fallback.RiskOnParams.StdDev)
	}
	if fallback.RiskOffParams.StdDev != 0.015 {
		t.Errorf("RiskOff StdDev should reflect config (0.015), got %f", fallback.RiskOffParams.StdDev)
	}
}

func TestDefaultFallbackParams_DefaultConfigPreservesHistoricalValues(t *testing.T) {
	cfg := config.DefaultParametersConfig()

	fallback := DefaultFallbackParams(cfg)

	if fallback.RiskOnParams.Mean != 0.0008 {
		t.Errorf("RiskOn Mean default should be 0.0008, got %f", fallback.RiskOnParams.Mean)
	}
	if fallback.RiskOffParams.Mean != 0.0001 {
		t.Errorf("RiskOff Mean default should be 0.0001, got %f", fallback.RiskOffParams.Mean)
	}
}
