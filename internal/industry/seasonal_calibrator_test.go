package industry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTempParams(t *testing.T, seasonalPatterns map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	config := map[string]any{
		"industry": map[string]any{
			"seasonal_patterns": seasonalPatterns,
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoadCalibrationEvidence_GoStructFormat(t *testing.T) {
	path := writeTempParams(t, map[string]any{
		"value":               []any{},
		"last_calibrated":     "2026-05-30T12:00:00Z",
		"calibration_method":  "backtest_empirical",
		"citation": map[string]any{
			"source_reference": "finmind data",
			"evidence_quality": "high",
		},
	})

	result := LoadCalibrationEvidence(path)
	if result == nil {
		t.Fatal("expected non-nil result for Go struct format")
	}
	if !result["calibrated"].(bool) {
		t.Error("expected calibrated=true")
	}
	if result["timestamp"] != "2026-05-30T12:00:00Z" {
		t.Errorf("expected last_calibrated timestamp, got: %v", result["timestamp"])
	}
	if result["data_source"] == nil {
		t.Error("expected data_source to be populated from citation")
	}
}

func TestLoadCalibrationEvidence_RawJSONFormat(t *testing.T) {
	path := writeTempParams(t, map[string]any{
		"value":                   []any{},
		"calibration_timestamp":   "2026-04-15T08:00:00Z",
		"calibration_data_source": "finmind",
	})

	result := LoadCalibrationEvidence(path)
	if result == nil {
		t.Fatal("expected non-nil result for raw JSON format (backward compat)")
	}
	if !result["calibrated"].(bool) {
		t.Error("expected calibrated=true")
	}
	if result["timestamp"] != "2026-04-15T08:00:00Z" {
		t.Errorf("expected calibration_timestamp, got: %v", result["timestamp"])
	}
	if result["data_source"] != "finmind" {
		t.Errorf("expected calibration_data_source, got: %v", result["data_source"])
	}
}

func TestLoadCalibrationEvidence_NoCalibration(t *testing.T) {
	path := writeTempParams(t, map[string]any{
		"value": []any{},
	})

	result := LoadCalibrationEvidence(path)
	if result != nil {
		t.Fatal("expected nil result when no calibration timestamp exists")
	}
}

func TestLoadCalibrationEvidence_PrefersLastCalibrated(t *testing.T) {
	// When BOTH formats exist, last_calibrated takes priority.
	path := writeTempParams(t, map[string]any{
		"value":                   []any{},
		"last_calibrated":         "2026-05-30T12:00:00Z",
		"calibration_timestamp":   "2026-04-15T08:00:00Z",
		"calibration_data_source": "finmind",
	})

	result := LoadCalibrationEvidence(path)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["timestamp"] != "2026-05-30T12:00:00Z" {
		t.Errorf("expected last_calibrated to take priority, got: %v", result["timestamp"])
	}
	if result["data_source"] != "finmind" {
		t.Errorf("expected calibration_data_source, got: %v", result["data_source"])
	}
}

func TestLoadCalibrationEvidence_DataSourceFallbackToCitation(t *testing.T) {
	// When calibration_data_source is missing, falls back to citation.source_reference.
	path := writeTempParams(t, map[string]any{
		"value":           []any{},
		"last_calibrated": "2026-05-30T12:00:00Z",
		"citation": map[string]any{
			"source_reference": "FinMind TaiwanStockPrice 5-year",
		},
	})

	result := LoadCalibrationEvidence(path)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["data_source"] != "FinMind TaiwanStockPrice 5-year" {
		t.Errorf("expected citation.source_reference fallback, got: %v", result["data_source"])
	}
}

func TestLoadCalibrationEvidence_FileNotFound(t *testing.T) {
	result := LoadCalibrationEvidence("/nonexistent/path/params.json")
	if result != nil {
		t.Fatal("expected nil when file not found")
	}
}

func TestLoadCalibrationEvidence_NilTimestamp(t *testing.T) {
	// last_calibrated is present but null — should not count as calibrated.
	path := writeTempParams(t, map[string]any{
		"value":           []any{},
		"last_calibrated": nil,
	})

	result := LoadCalibrationEvidence(path)
	if result != nil {
		t.Fatal("expected nil result when last_calibrated is null")
	}
}

func TestLoadCalibrationEvidence_EmptyTimestampString(t *testing.T) {
	// Backward compat: empty string timestamp should return nil.
	path := writeTempParams(t, map[string]any{
		"value":                 []any{},
		"calibration_timestamp": "",
	})

	result := LoadCalibrationEvidence(path)
	if result != nil {
		t.Fatal("expected nil result when calibration_timestamp is empty string")
	}
}

// TestLoadCalibrationEvidence_RealConfigShape verifies the actual
// configs/parameters.json can be parsed (if it contains calibration data).
func TestLoadCalibrationEvidence_RealConfigShape(t *testing.T) {
	// This test verifies the structure matches what ParametersConfig.Save() produces.
	path := writeTempParams(t, map[string]any{
		"value": []any{
			map[string]any{
				"id":                  "spring_festival",
				"name":                "春節行情",
				"historical_accuracy": 0.7,
			},
		},
		"rationale":          "7 canonical Taiwan seasonal patterns",
		"source":             "heuristic",
		"last_calibrated":    "2026-05-30T05:25:59Z",
		"calibration_method": "backtest_empirical",
		"citation": map[string]any{
			"source_type":       "backtest",
			"source_reference":  "FinMind data",
			"evidence_quality":  "high",
			"update_policy":     "manual",
			"validation_method": "backtest",
			"dependencies":      []any{},
			"last_validated":    "2026-04-21",
		},
	})

	result := LoadCalibrationEvidence(path)
	if result == nil {
		t.Fatal("expected non-nil result for real config shape")
	}
	if !result["calibrated"].(bool) {
		t.Error("expected calibrated=true")
	}
	if result["data_source"] != "FinMind data" {
		t.Errorf("expected citation fallback, got: %v", result["data_source"])
	}
}
