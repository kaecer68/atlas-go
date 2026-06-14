package calibration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestUpdateParameterMetadata(t *testing.T) {
	cfg := config.DefaultParametersConfig()

	tests := []struct {
		name string
		p    CalibratedParameter
	}{
		{name: "garch omega", p: CalibratedParameter{Path: "garch.omega", Method: "MLE"}},
		{name: "garch alpha", p: CalibratedParameter{Path: "garch.alpha", Method: "MLE"}},
		{name: "garch beta", p: CalibratedParameter{Path: "garch.beta", Method: "MLE"}},
		{name: "sizing target volatility", p: CalibratedParameter{Path: "sizing.target_volatility", Method: "VaR"}},
		{name: "sizing max drawdown", p: CalibratedParameter{Path: "sizing.max_drawdown_limit", Method: "VaR"}},
		{name: "darwinian hit high", p: CalibratedParameter{Path: "darwinian.hit_rate_high_threshold", Method: "percentile"}},
		{name: "darwinian hit low", p: CalibratedParameter{Path: "darwinian.hit_rate_low_threshold", Method: "percentile"}},
		{name: "factor momentum stddev", p: CalibratedParameter{Path: "factor.momentum_stddev_divisor", Method: "distribution"}},
		{name: "factor momentum lookback", p: CalibratedParameter{Path: "factor.momentum_lookback_days", Method: "autocorr"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := UpdateParameterMetadata(cfg, tt.p); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("invalid path format", func(t *testing.T) {
		p := CalibratedParameter{Path: "too.many.parts.here"}
		if err := UpdateParameterMetadata(cfg, p); err == nil {
			t.Fatal("expected error for invalid path")
		}
	})

	t.Run("unknown section", func(t *testing.T) {
		p := CalibratedParameter{Path: "unknown.key"}
		if err := UpdateParameterMetadata(cfg, p); err != nil {
			t.Fatalf("unexpected error for unknown section: %v", err)
		}
	})

	t.Run("metadata set correctly", func(t *testing.T) {
		cfg := config.DefaultParametersConfig()
		p := CalibratedParameter{Path: "garch.omega", Method: "MLE_grid_search"}
		if err := UpdateParameterMetadata(cfg, p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GARCH.Omega.Source != config.SourceCalibrated {
			t.Errorf("source = %s, want calibrated", cfg.GARCH.Omega.Source)
		}
		if cfg.GARCH.Omega.CalibrationMethod != "MLE_grid_search" {
			t.Errorf("method = %s, want MLE_grid_search", cfg.GARCH.Omega.CalibrationMethod)
		}
		if cfg.GARCH.Omega.LastCalibrated == nil {
			t.Fatal("expected LastCalibrated to be set")
		}
	})
}

func TestSaveResults(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	results := []CalibrationResult{
		{
			Module: "garch",
			Parameters: []CalibratedParameter{
				{Path: "garch.omega", Method: "MLE", Before: 0.1, After: 0.2},
			},
		},
	}

	tmp := t.TempDir()
	paramsPath := tmp + "/params.json"
	if err := SaveResults(cfg, results, paramsPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GARCH.Omega.Source != config.SourceCalibrated {
		t.Error("expected omega source to be calibrated")
	}
}

func TestFormatReport(t *testing.T) {
	results := []CalibrationResult{
		{
			Module: "garch",
			Parameters: []CalibratedParameter{
				{Path: "garch.alpha", Before: 0.1, After: 0.12, Method: "MLE", Confidence: 0.95, Significant: true, SampleSize: 100},
			},
		},
	}

	s := FormatReport(results, false)
	if !strings.Contains(s, "Parameter Calibration Report") {
		t.Error("missing report header")
	}
	if !strings.Contains(s, "garch.alpha") {
		t.Error("missing parameter path")
	}
	if !strings.Contains(s, "Total parameters calibrated: 1") {
		t.Error("missing total count")
	}

	verbose := FormatReport(results, true)
	if !strings.Contains(verbose, "statistically significant") {
		t.Error("missing verbose note")
	}
}

func TestRun(t *testing.T) {
	tmp := t.TempDir()
	jsonlPath := filepath.Join(tmp, "returns.jsonl")
	lines := make([]map[string]float64, 35)
	for i := range lines {
		lines[i] = map[string]float64{"return": 0.01 * float64(i)}
	}
	writeJSONL(t, jsonlPath, lines)

	report, err := Run(tmp, "garch", jsonlPath, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(report, "Parameter Calibration Report") {
		t.Error("missing report header")
	}
	if !strings.Contains(report, "[DRY-RUN]") {
		t.Error("missing dry-run notice")
	}

	_, err = Run(tmp, "unknown", jsonlPath, true, false)
	if err == nil {
		t.Fatal("expected error for unknown module")
	}
}
