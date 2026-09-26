package calibration

import (
	"os"
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

	report, err := Run(tmp, "garch", jsonlPath, true, false, config.WritebackSSOT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(report, "Parameter Calibration Report") {
		t.Error("missing report header")
	}
	if !strings.Contains(report, "[DRY-RUN]") {
		t.Error("missing dry-run notice")
	}

	_, err = Run(tmp, "unknown", jsonlPath, true, false, config.WritebackSSOT)
	if err == nil {
		t.Fatal("expected error for unknown module")
	}
}

// TestRun_OverlayWritebackDoesNotWriteSSOT is the FU-20260926-07 contract for
// this command: in overlay mode the SSOT file must stay byte-identical and the
// calibrated leaves must land in the overlay under data/.
func TestRun_OverlayWritebackDoesNotWriteSSOT(t *testing.T) {
	tmp := t.TempDir()
	jsonlPath := filepath.Join(tmp, "returns.jsonl")
	lines := make([]map[string]float64, 35)
	for i := range lines {
		lines[i] = map[string]float64{"return": 0.01 * float64(i)}
	}
	writeJSONL(t, jsonlPath, lines)

	ssotPath := filepath.Join(tmp, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(ssotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ssotJSON := `{"garch":{"omega":{"value":0.0001},"alpha":{"value":0.1},"beta":{"value":0.8}}}`
	if err := os.WriteFile(ssotPath, []byte(ssotJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	overlayPath := config.CalibrationOverlayPath(tmp)

	previousSSOT := config.GetParametersConfigPath()
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetParametersConfigPath(ssotPath)
	config.SetCalibratedOverlayPath(overlayPath)
	defer func() {
		config.SetParametersConfigPath(previousSSOT)
		config.SetCalibratedOverlayPath(previousOverlay)
	}()

	report, err := Run(tmp, "garch", jsonlPath, false, false, config.WritebackOverlay)
	if err != nil {
		t.Fatalf("Run(overlay): %v", err)
	}
	// The report proves the run routed to the overlay (it names the overlay path),
	// independent of whether this particular input produced a change.
	if !strings.Contains(report, "configs/parameters.json untouched") || !strings.Contains(report, overlayPath) {
		t.Errorf("report does not show the overlay route: %s", report)
	}

	after, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != ssotJSON {
		t.Errorf("SSOT document rewritten in overlay mode:\nbefore=%s\nafter=%s", ssotJSON, after)
	}

	ov, err := config.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov != nil {
		// When this input does produce changes, they must be the calibrated garch
		// leaves — never a wholesale copy of the document.
		for path := range ov.Entries {
			if !strings.HasPrefix(path, "garch.") {
				t.Errorf("overlay entry %q is not a calibrated garch leaf: %+v", path, ov.Entries)
			}
		}
	}
}
