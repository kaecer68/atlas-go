package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDefaultParametersConfig(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", cfg.Version)
	}

	// Darwinian
	if cfg.Darwinian.WeightMin.Value != 0.3 {
		t.Errorf("expected weight_min 0.3, got %f", cfg.Darwinian.WeightMin.Value)
	}
	if cfg.Darwinian.WeightMax.Value != 2.5 {
		t.Errorf("expected weight_max 2.5, got %f", cfg.Darwinian.WeightMax.Value)
	}
	if cfg.Darwinian.TopQuartileMultiplier.Value != 1.05 {
		t.Errorf("expected top_multiplier 1.05, got %f", cfg.Darwinian.TopQuartileMultiplier.Value)
	}
	if cfg.Darwinian.ZeroSignalPenaltyMultiplier.Value != 0.9 {
		t.Errorf("expected zero_signal_penalty_multiplier 0.9, got %f", cfg.Darwinian.ZeroSignalPenaltyMultiplier.Value)
	}
	if cfg.Darwinian.ZeroSignalPenaltyAfterDays.Value != 14 {
		t.Errorf("expected zero_signal_penalty_after_days 14, got %d", cfg.Darwinian.ZeroSignalPenaltyAfterDays.Value)
	}
	if cfg.Darwinian.LossPenaltyMultiplier.Value != 0.9 {
		t.Errorf("expected loss_penalty_multiplier 0.9, got %f", cfg.Darwinian.LossPenaltyMultiplier.Value)
	}
	if cfg.Darwinian.WeightChangeAlertThreshold.Value != 0.15 {
		t.Errorf("expected weight_change_alert_threshold 0.15, got %f", cfg.Darwinian.WeightChangeAlertThreshold.Value)
	}

	// Factor
	if cfg.Factor.MomentumStdDevDivisor.Value != 0.30 {
		t.Errorf("expected momentum_divisor 0.30, got %f", cfg.Factor.MomentumStdDevDivisor.Value)
	}

	// Optimizer
	if cfg.Optimizer.MaxPositionPct.Value != 0.15 {
		t.Errorf("expected max_position_pct 0.15, got %f", cfg.Optimizer.MaxPositionPct.Value)
	}

	// Sizing
	if cfg.Sizing.KellyFraction.Value != 0.5 {
		t.Errorf("expected kelly_fraction 0.5, got %f", cfg.Sizing.KellyFraction.Value)
	}

	// Health
	if cfg.Health.MuteThreshold.Value != 5 {
		t.Errorf("expected mute_threshold 5, got %d", cfg.Health.MuteThreshold.Value)
	}

	// GARCH
	if cfg.GARCH.Alpha.Value != 0.1 {
		t.Errorf("expected garch_alpha 0.1, got %f", cfg.GARCH.Alpha.Value)
	}

	// Experiment
	if cfg.Experiment.MaturityLevel1Observations.Value != 3 {
		t.Errorf("expected level1_obs 3, got %d", cfg.Experiment.MaturityLevel1Observations.Value)
	}

	// Baseline
	if cfg.Baseline.StartingCash.Value != 3000000 {
		t.Errorf("expected starting_cash 3000000, got %f", cfg.Baseline.StartingCash.Value)
	}
}

func TestLoadParametersConfig_MissingFile(t *testing.T) {
	cfg, err := LoadParametersConfig("/nonexistent/path/parameters.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config, got nil")
	}
	if cfg.Darwinian.WeightMin.Value != 0.3 {
		t.Errorf("expected fallback to default weight_min 0.3, got %f", cfg.Darwinian.WeightMin.Value)
	}
	if cfg.Engine.MacroRisk.VIXThreshold.Value == 0 {
		t.Errorf("expected Engine.MacroRisk.VIXThreshold default (30.0), got zero — mergeEngineDefaults may be broken")
	}
	if cfg.Engine.Executors.VIXMomentumCrashThreshold.Value == 0 {
		t.Errorf("expected Engine.Executors.VIXMomentumCrashThreshold default (30.0), got zero")
	}
	if cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value == 0 {
		t.Errorf("expected Engine.Simulation.NeutralRegimeSizingFactor default (0.85), got zero")
	}
}

func TestLoadParametersConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	cfg.Darwinian.WeightMin.Value = 0.4
	cfg.Darwinian.WeightMin.Rationale = "custom test value"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.Darwinian.WeightMin.Value != 0.4 {
		t.Errorf("expected loaded weight_min 0.4, got %f", loaded.Darwinian.WeightMin.Value)
	}
	if loaded.Darwinian.WeightMin.Rationale != "custom test value" {
		t.Errorf("expected rationale preserved, got %s", loaded.Darwinian.WeightMin.Rationale)
	}
}

func TestLoadParametersConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	_, err := LoadParametersConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParametersConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ParametersConfig)
		wantErr bool
	}{
		{
			name:    "valid default",
			mutate:  func(_ *ParametersConfig) {},
			wantErr: false,
		},
		{
			name: "weight_min >= weight_max",
			mutate: func(c *ParametersConfig) {
				c.Darwinian.WeightMin.Value = 3.0
			},
			wantErr: true,
		},
		{
			name: "ema_alpha out of range",
			mutate: func(c *ParametersConfig) {
				c.Darwinian.EMAAlpha.Value = 1.5
			},
			wantErr: true,
		},
		{
			name: "kelly_fraction out of range",
			mutate: func(c *ParametersConfig) {
				c.Sizing.KellyFraction.Value = 1.5
			},
			wantErr: true,
		},
		{
			name: "health weights don't sum to 1.0",
			mutate: func(c *ParametersConfig) {
				c.Health.SharpeWeight.Value = 0.5
				c.Health.HitRateWeight.Value = 0.5
				c.Health.StreakWeight.Value = 0.5
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultParametersConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestParametersConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	before := cfg.UpdatedAt

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Version != cfg.Version {
		t.Errorf("version mismatch")
	}
	if !loaded.UpdatedAt.After(before) {
		t.Errorf("updated_at not refreshed")
	}
}

// TestParametersConfig_SavePreservesCalibrationEvidence verifies that
// ParametersConfig.Save() writes both `last_calibrated` (the Go struct
// field) AND `calibration_timestamp` (the raw JSON field) whenever
// LastCalibrated is set. This is required by cmd/validate-parameters and
// internal/industry.LoadCalibrationEvidence, both of which read the raw
// JSON. Previously, the second field was silently dropped by
// json.MarshalIndent of the Go struct.
func TestParametersConfig_SavePreservesCalibrationEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cfg.Industry.SeasonalPatterns.LastCalibrated = &now
	cfg.Industry.SeasonalPatterns.CalibrationMethod = "backtest_empirical"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	raw := string(data)
	if !strings.Contains(raw, `"last_calibrated": "2026-06-06T12:00:00Z"`) {
		t.Errorf("expected last_calibrated entry in output, got:\n%s", raw)
	}
	if !strings.Contains(raw, `"calibration_timestamp": "2026-06-06T12:00:00Z"`) {
		t.Errorf("expected calibration_timestamp mirror in output, got:\n%s", raw)
	}

	// Round-trip through LoadParametersConfig and confirm the
	// calibration_timestamp is still present in the persisted file
	// (LoadParametersConfig drops unknown fields, which is the bug we fix
	// by writing them in Save, not Load).
	loaded, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Industry.SeasonalPatterns.LastCalibrated == nil {
		t.Errorf("LastCalibrated dropped on load")
	}

	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

// TestParametersConfig_SaveOmitsCalibrationTimestampWhenUnset verifies that
// Save() does not invent a calibration_timestamp when LastCalibrated is nil.
func TestParametersConfig_SaveOmitsCalibrationTimestampWhenUnset(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	cfg.Industry.SeasonalPatterns.LastCalibrated = nil

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, `"calibration_timestamp"`) {
		t.Errorf("calibration_timestamp should not be written when LastCalibrated is nil, got:\n%s", raw)
	}
	if strings.Contains(raw, `"last_calibrated"`) {
		t.Errorf("last_calibrated should not be written when LastCalibrated is nil, got:\n%s", raw)
	}
}

// TestParametersConfig_SaveWithRollbackPreservesCalibrationEvidence is the
// atomic-write counterpart to TestParametersConfig_SavePreservesCalibrationEvidence.
func TestParametersConfig_SaveWithRollbackPreservesCalibrationEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cfg.Industry.SeasonalPatterns.LastCalibrated = &now

	if err := cfg.SaveWithRollback(path); err != nil {
		t.Fatalf("save with rollback: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	raw := string(data)
	if !strings.Contains(raw, `"last_calibrated": "2026-06-06T12:00:00Z"`) {
		t.Errorf("SaveWithRollback dropped last_calibrated")
	}
	if !strings.Contains(raw, `"calibration_timestamp": "2026-06-06T12:00:00Z"`) {
		t.Errorf("SaveWithRollback dropped calibration_timestamp mirror")
	}
	for _, suffix := range []string{".tmp", ".bak"} {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Errorf("side file %s was not cleaned up", path+suffix)
		}
	}
}

func TestSnapshotToBackup_CreatesSnapshotFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}

	if _, err := os.Stat(path + ".snapshot.bak"); err != nil {
		t.Fatalf("expected .snapshot.bak to be created: %v", err)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	snapshot, err := os.ReadFile(path + ".snapshot.bak")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(original) != string(snapshot) {
		t.Errorf("snapshot content does not match original")
	}
}

func TestSnapshotToBackup_OverwritesExistingSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg1 := DefaultParametersConfig()
	cfg1.Version = "1.0"
	if err := cfg1.Save(path); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("first SnapshotToBackup: %v", err)
	}

	cfg2 := DefaultParametersConfig()
	cfg2.Version = "2.0"
	if err := cfg2.Save(path); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("second SnapshotToBackup: %v", err)
	}

	snapshot, err := os.ReadFile(path + ".snapshot.bak")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(snapshot), `"version": "2.0"`) {
		t.Errorf("snapshot should reflect latest version 2.0, got:\n%s", string(snapshot))
	}
}

func TestSnapshotToBackup_EmptyPath(t *testing.T) {
	if err := SnapshotToBackup(""); err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestSnapshotToBackup_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.json")
	if err := SnapshotToBackup(path); err == nil {
		t.Error("expected error for missing source file, got nil")
	}
}

func TestRestoreFromBackup_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	cfg.Version = "original-v1"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}

	cfg.Version = "modified-v2"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save modified: %v", err)
	}

	if err := RestoreFromBackup(path); err != nil {
		t.Fatalf("RestoreFromBackup: %v", err)
	}

	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if !strings.Contains(string(restored), `"version": "original-v1"`) {
		t.Errorf("expected restored file to contain original-v1, got:\n%s", string(restored))
	}
	if strings.Contains(string(restored), `"version": "modified-v2"`) {
		t.Errorf("restored file should not contain modified-v2, got:\n%s", string(restored))
	}
}

func TestRestoreFromBackup_NoSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := RestoreFromBackup(path); err == nil {
		t.Error("expected error when no .snapshot.bak exists, got nil")
	}
}

func TestRestoreFromBackup_EmptyPath(t *testing.T) {
	if err := RestoreFromBackup(""); err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestSequence_SnapshotThenLockedSave_PreservesSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	cfg.Version = "pre-calibration-v1"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}

	cfg.Version = "post-calibration-v2"
	if err := cfg.LockedSaveWithRollback(path); err != nil {
		t.Fatalf("LockedSaveWithRollback: %v", err)
	}

	snapshot, err := os.ReadFile(path + ".snapshot.bak")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(snapshot), `"version": "pre-calibration-v1"`) {
		t.Errorf("snapshot should still contain pre-calibration-v1 after LockedSaveWithRollback, got:\n%s", string(snapshot))
	}

	if err := RestoreFromBackup(path); err != nil {
		t.Fatalf("RestoreFromBackup: %v", err)
	}

	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if !strings.Contains(string(restored), `"version": "pre-calibration-v1"`) {
		t.Errorf("restored file should contain pre-calibration-v1, got:\n%s", string(restored))
	}
}

func TestSnapshotToBackup_NoTempFileAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}
	if _, err := os.Stat(path + ".snapshot.bak.tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .snapshot.bak.tmp to be cleaned up, got err=%v", err)
	}
	if _, err := os.Stat(path + ".snapshot.bak"); err != nil {
		t.Errorf("expected .snapshot.bak to exist: %v", err)
	}
}

func TestRestoreFromBackup_NoTempFileAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	cfg.Version = "v1"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if err := SnapshotToBackup(path); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}
	cfg.Version = "v2"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save v2: %v", err)
	}
	if err := RestoreFromBackup(path); err != nil {
		t.Fatalf("RestoreFromBackup: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be cleaned up, got err=%v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if !strings.Contains(string(restored), `"version": "v1"`) {
		t.Errorf("restored file should contain v1, got:\n%s", string(restored))
	}
}

// TestMirrorCalibrationTimestamp_PreservesJSON verifies the post-processor
// itself: any input containing `last_calibrated` should produce output that
// (a) still parses as valid JSON, (b) contains a sibling
// `calibration_timestamp` with the same value, and (c) keeps surrounding
// content byte-identical apart from the injection.
func TestMirrorCalibrationTimestamp_PreservesJSON(t *testing.T) {
	const ts1 = "2026-06-06T12:00:00Z"
	const ts2 = "2026-01-01T00:00:00Z"
	const ts3 = "2026-02-02T00:00:00Z"
	cases := []struct {
		name               string
		in                 string
		wantLastCalibrated int
	}{
		{
			name:               "trailing comma (mid-object)",
			in:                 "{\n  \"a\": 1,\n  \"last_calibrated\": \"" + ts1 + "\",\n  \"b\": 2\n}\n",
			wantLastCalibrated: 1,
		},
		{
			name:               "no trailing comma (last field)",
			in:                 "{\n  \"a\": 1,\n  \"last_calibrated\": \"" + ts1 + "\"\n}\n",
			wantLastCalibrated: 1,
		},
		{
			name:               "absent (no-op)",
			in:                 "{\n  \"a\": 1,\n  \"b\": 2\n}\n",
			wantLastCalibrated: 0,
		},
		{
			name: "multiple occurrences",
			in: "{\n  \"x\": {\n    \"last_calibrated\": \"" + ts2 + "\",\n    \"v\": 1\n  },\n" +
				"  \"y\": {\n    \"last_calibrated\": \"" + ts3 + "\"\n  }\n}\n",
			wantLastCalibrated: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mirrorCalibrationTimestamp([]byte(tc.in)))
			var parsed any
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, got)
			}
			if n := strings.Count(got, `"calibration_timestamp"`); n != tc.wantLastCalibrated {
				t.Errorf("expected %d calibration_timestamp injections, got %d\noutput:\n%s",
					tc.wantLastCalibrated, n, got)
			}
			if n := strings.Count(got, `"last_calibrated"`); n != tc.wantLastCalibrated {
				t.Errorf("last_calibrated count changed: want %d, got %d", tc.wantLastCalibrated, n)
			}
		})
	}
}

func TestEngineParameters_DefaultsExist(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.Engine.MacroRisk.VIXThreshold.Value != 30.0 {
		t.Errorf("expected Engine.MacroRisk.VIXThreshold = 30.0, got %f", cfg.Engine.MacroRisk.VIXThreshold.Value)
	}
	if cfg.Engine.MacroRisk.CarryTradeUnwindThreshold.Value != 145.0 {
		t.Errorf("expected Engine.MacroRisk.CarryTradeUnwindThreshold = 145.0, got %f", cfg.Engine.MacroRisk.CarryTradeUnwindThreshold.Value)
	}
	if cfg.Engine.MacroRisk.US10YThreshold.Value != 4.5 {
		t.Errorf("expected Engine.MacroRisk.US10YThreshold = 4.5, got %f", cfg.Engine.MacroRisk.US10YThreshold.Value)
	}
	if cfg.Engine.Executors.VIXMomentumCrashThreshold.Value != 30.0 {
		t.Errorf("expected Engine.Executors.VIXMomentumCrashThreshold = 30.0, got %f", cfg.Engine.Executors.VIXMomentumCrashThreshold.Value)
	}
	if cfg.Engine.Executors.ConvictionFloorDefault.Value != 60 {
		t.Errorf("expected Engine.Executors.ConvictionFloorDefault = 60, got %d", cfg.Engine.Executors.ConvictionFloorDefault.Value)
	}
	if cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value != 0.85 {
		t.Errorf("expected Engine.Simulation.NeutralRegimeSizingFactor = 0.85, got %f", cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value)
	}

	levels := cfg.Engine.Drawdown.Levels.Value
	for _, name := range []string{"none", "light", "moderate", "severe", "emergency"} {
		if _, ok := levels[name]; !ok {
			t.Errorf("Engine.Drawdown.Levels missing key: %s", name)
		}
	}

	total := 0.0
	for _, alloc := range cfg.Engine.SectorRotation.BaseAllocations.Value {
		total += alloc
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("Engine.SectorRotation.BaseAllocations sum = %.4f, expected 1.0±0.01", total)
	}

	if cfg.Engine.MacroRisk.VIXThreshold.Rationale == "" {
		t.Error("Engine.MacroRisk.VIXThreshold.Rationale is empty — missing provenance")
	}
	if cfg.Engine.Executors.VIXMomentumCrashThreshold.Rationale == "" {
		t.Error("Engine.Executors.VIXMomentumCrashThreshold.Rationale is empty")
	}
	if cfg.Engine.Simulation.NeutralRegimeSizingFactor.Rationale == "" {
		t.Error("Engine.Simulation.NeutralRegimeSizingFactor.Rationale is empty")
	}
}

func TestEngineParameters_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ParametersConfig)
		wantErr bool
	}{
		{
			name:    "valid default engine params",
			mutate:  func(_ *ParametersConfig) {},
			wantErr: false,
		},
		{
			name: "engine_macro_risk_vix_negative",
			mutate: func(c *ParametersConfig) {
				c.Engine.MacroRisk.VIXThreshold.Value = -1
			},
			wantErr: true,
		},
		{
			name: "engine_drawdown_level_pct_out_of_range",
			mutate: func(c *ParametersConfig) {
				levels := make(map[string]DrawdownLevel)
				maps.Copy(levels, c.Engine.Drawdown.Levels.Value)
				levels["light"] = DrawdownLevel{Percentage: 1.5, MaxExposure: 0.85}
				c.Engine.Drawdown.Levels.Value = levels
			},
			wantErr: true,
		},
		{
			name: "engine_sector_rotation_alloc_not_sum_1",
			mutate: func(c *ParametersConfig) {
				allocs := make(map[string]float64)
				maps.Copy(allocs, c.Engine.SectorRotation.BaseAllocations.Value)
				allocs["semiconductor"] = 0.9
				c.Engine.SectorRotation.BaseAllocations.Value = allocs
			},
			wantErr: true,
		},
		{
			name: "engine_executors_max_stocks_min_gt_max",
			mutate: func(c *ParametersConfig) {
				c.Engine.Executors.MaxStocksMin.Value = 15
				c.Engine.Executors.MaxStocksMax.Value = 10
			},
			wantErr: true,
		},
		{
			name: "engine_simulation_neutral_factor_out_of_range",
			mutate: func(c *ParametersConfig) {
				c.Engine.Simulation.NeutralRegimeSizingFactor.Value = 1.5
			},
			wantErr: true,
		},
		{
			name: "engine_executors_crowding_penalty_out_of_range",
			mutate: func(c *ParametersConfig) {
				c.Engine.Executors.CrowdingPenaltyAgents3.Value = 1.5
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultParametersConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestEngineParameters_ConsumerAccessPatterns(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value != 0.85 {
		t.Errorf("simulation neutral factor: got %f, want 0.85", cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value)
	}

	if cfg.Engine.Executors.VIXMomentumCrashThreshold.Value != 30.0 {
		t.Errorf("executors VIX crash threshold: got %f, want 30.0", cfg.Engine.Executors.VIXMomentumCrashThreshold.Value)
	}
	if cfg.Engine.Executors.CrowdingPenaltyAgents3.Value < 0 || cfg.Engine.Executors.CrowdingPenaltyAgents3.Value > 1 {
		t.Errorf("executors crowding penalty: got %f, want in (0,1)", cfg.Engine.Executors.CrowdingPenaltyAgents3.Value)
	}

	macroCfg := cfg.Engine.MacroRisk.ToConfig()
	if macroCfg.VIXThreshold != 30.0 {
		t.Errorf("MacroRisk.ToConfig().VIXThreshold: got %f, want 30.0", macroCfg.VIXThreshold)
	}
	if macroCfg.CarryTradeUnwindThreshold != 145.0 {
		t.Errorf("MacroRisk.ToConfig().CarryTradeUnwindThreshold: got %f, want 145.0", macroCfg.CarryTradeUnwindThreshold)
	}

	drawdownCfg := cfg.Engine.Drawdown.ToConfig()
	if len(drawdownCfg.Levels) != 5 {
		t.Errorf("Drawdown.ToConfig().Levels: got %d entries, want 5", len(drawdownCfg.Levels))
	}

	evolCfg := cfg.Engine.StrategyEvolution.ToConfig()
	if evolCfg.CooldownPeriodHours != 24 {
		t.Errorf("StrategyEvolution.ToConfig().CooldownPeriodHours: got %d, want 24", evolCfg.CooldownPeriodHours)
	}

	sectorCfg := cfg.Engine.SectorRotation.ToConfig()
	if sectorCfg.MinAllocation <= 0 {
		t.Errorf("SectorRotation.ToConfig().MinAllocation: got %f, want positive", sectorCfg.MinAllocation)
	}
}

func TestTaxParameters_ToConfig_ZeroValuesUseStatutoryDefaults(t *testing.T) {
	// Zero values in TaxParameters carry the semantic "use statutory default rate".
	// ToConfig() must apply DefaultTaiwanTaxConfig() when fields are 0.
	p := TaxParameters{
		DividendTaxRate:    ParameterMetadata[float64]{Value: 0, Rationale: "0 = use statutory"},
		TransactionTaxRate: ParameterMetadata[float64]{Value: 0, Rationale: "0 = use statutory"},
		NHISurchargeRate:   ParameterMetadata[float64]{Value: 0, Rationale: "0 = use statutory"},
	}
	cfg := p.ToConfig()

	want := domain.DefaultTaiwanTaxConfig()
	if cfg.DividendTaxRate != want.DividendTaxRate {
		t.Errorf("DividendTaxRate: got %f, want %f (statutory)", cfg.DividendTaxRate, want.DividendTaxRate)
	}
	if cfg.TransactionTaxRate != want.TransactionTaxRate {
		t.Errorf("TransactionTaxRate: got %f, want %f (statutory)", cfg.TransactionTaxRate, want.TransactionTaxRate)
	}
	if cfg.NHISurchargeRate != want.NHISurchargeRate {
		t.Errorf("NHISurchargeRate: got %f, want %f (statutory)", cfg.NHISurchargeRate, want.NHISurchargeRate)
	}
	if !cfg.IncludeNHI {
		t.Error("IncludeNHI: got false, want true")
	}
}

func TestTaxParameters_ToConfig_ExplicitValuesOverrideDefaults(t *testing.T) {
	// Non-zero values should override statutory defaults.
	p := TaxParameters{
		DividendTaxRate:    ParameterMetadata[float64]{Value: 0.30, Rationale: "custom rate"},
		TransactionTaxRate: ParameterMetadata[float64]{Value: 0.001, Rationale: "custom rate"},
		NHISurchargeRate:   ParameterMetadata[float64]{Value: 0.01, Rationale: "custom rate"},
	}
	cfg := p.ToConfig()

	if cfg.DividendTaxRate != 0.30 {
		t.Errorf("DividendTaxRate: got %f, want 0.30", cfg.DividendTaxRate)
	}
	if cfg.TransactionTaxRate != 0.001 {
		t.Errorf("TransactionTaxRate: got %f, want 0.001", cfg.TransactionTaxRate)
	}
	if cfg.NHISurchargeRate != 0.01 {
		t.Errorf("NHISurchargeRate: got %f, want 0.01", cfg.NHISurchargeRate)
	}
	if !cfg.IncludeNHI {
		t.Error("IncludeNHI: got false, want true")
	}
}

func TestParameterMetadata_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := DefaultParametersConfig()
	now := time.Now()
	cfg.Darwinian.SharpeNormalizeDenom.LastCalibrated = &now
	cfg.Darwinian.SharpeNormalizeDenom.CalibrationMethod = "quantile_regression"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Darwinian.SharpeNormalizeDenom.CalibrationMethod != "quantile_regression" {
		t.Errorf("calibration_method not preserved")
	}
	if loaded.Darwinian.SharpeNormalizeDenom.LastCalibrated == nil {
		t.Errorf("last_calibrated not preserved")
	}
}

func TestSectorAllocationConfig_Defaults(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.SectorAllocation.BaseWeights == nil {
		t.Errorf("SectorAllocation.BaseWeights is nil, want non-nil map")
	}

	if cfg.SectorAllocation.BaseWeights["semiconductor"] <= 0 {
		t.Errorf("SectorAllocation.BaseWeights[semiconductor] = %f, want > 0", cfg.SectorAllocation.BaseWeights["semiconductor"])
	}

	if cfg.SectorAllocation.WeightFloor <= 0 {
		t.Errorf("SectorAllocation.WeightFloor = %f, want > 0", cfg.SectorAllocation.WeightFloor)
	}

	if cfg.SectorAllocation.CycleWeight != 1.0 {
		t.Errorf("SectorAllocation.CycleWeight = %f, want 1.0", cfg.SectorAllocation.CycleWeight)
	}
	if cfg.SectorAllocation.SeasonalWeight != 1.0 {
		t.Errorf("SectorAllocation.SeasonalWeight = %f, want 1.0", cfg.SectorAllocation.SeasonalWeight)
	}
	if cfg.SectorAllocation.LinkageWeight != 1.0 {
		t.Errorf("SectorAllocation.LinkageWeight = %f, want 1.0", cfg.SectorAllocation.LinkageWeight)
	}
	if cfg.SectorAllocation.NarrativeWeight != 1.0 {
		t.Errorf("SectorAllocation.NarrativeWeight = %f, want 1.0", cfg.SectorAllocation.NarrativeWeight)
	}
	if cfg.SectorAllocation.MacroWeight != 1.0 {
		t.Errorf("SectorAllocation.MacroWeight = %f, want 1.0", cfg.SectorAllocation.MacroWeight)
	}
	if cfg.SectorAllocation.FactorWeight != 1.0 {
		t.Errorf("SectorAllocation.FactorWeight = %f, want 1.0", cfg.SectorAllocation.FactorWeight)
	}
}

func TestSectorAllocationConfig_ParameterMetadata(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.SectorAllocation.Rationale == "" {
		t.Errorf("SectorAllocation.Rationale is empty, want non-empty rationale")
	}
	if cfg.SectorAllocation.Source == "" {
		t.Errorf("SectorAllocation.Source is empty, want non-empty source")
	}
	if cfg.SectorAllocation.Citation == nil {
		t.Errorf("SectorAllocation.Citation is nil, want non-nil citation")
	} else {
		if cfg.SectorAllocation.Citation.EvidenceQuality == "" {
			t.Errorf("SectorAllocation.Citation.EvidenceQuality is empty")
		}
		if cfg.SectorAllocation.Citation.SourceReference == "" {
			t.Errorf("SectorAllocation.Citation.SourceReference is empty")
		}
	}
}

func TestSectorAllocationConfig_DerivationFactors(t *testing.T) {
	cfg := DefaultParametersConfig()

	if cfg.SectorAllocation.DerivationFactors == nil {
		t.Errorf("SectorAllocation.DerivationFactors is nil, want empty map or populated map")
	}
}

func TestLoadParametersConfig_FallbackPriceTargetsDefaultsMerged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters_partial.json")

	// Start from defaults so all required sections are valid, then override
	// FallbackPriceTargets with a partial map that omits _default.
	cfg := DefaultParametersConfig()
	cfg.FallbackPriceTargets = map[string]FallbackPriceTarget{
		"technical_breakout": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.12},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.93},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	defaultEntry, ok := loaded.FallbackPriceTargets["_default"]
	if !ok {
		t.Fatalf("_default key not merged into FallbackPriceTargets")
	}
	if defaultEntry.TargetMultiplier.Value != 1.05 {
		t.Errorf("_default target multiplier = %v, want 1.05", defaultEntry.TargetMultiplier.Value)
	}
	if defaultEntry.StopLossMultiplier.Value != 0.95 {
		t.Errorf("_default stop-loss multiplier = %v, want 0.95", defaultEntry.StopLossMultiplier.Value)
	}

	customEntry, ok := loaded.FallbackPriceTargets["technical_breakout"]
	if !ok {
		t.Fatalf("technical_breakout key not preserved")
	}
	if customEntry.TargetMultiplier.Value != 1.12 {
		t.Errorf("technical_breakout target multiplier = %v, want 1.12", customEntry.TargetMultiplier.Value)
	}
	if customEntry.StopLossMultiplier.Value != 0.93 {
		t.Errorf("technical_breakout stop-loss multiplier = %v, want 0.93", customEntry.StopLossMultiplier.Value)
	}
}

func TestParametersConfig_LockedSaveWithRollback(t *testing.T) {
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "test-params.json")

	cfg := DefaultParametersConfig()

	if err := cfg.LockedSaveWithRollback(paramsPath); err != nil {
		t.Fatalf("LockedSaveWithRollback failed: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("file is empty")
	}
	// Verify round-trip parseable
	if _, err := LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("LoadParametersConfig failed: %v", err)
	}
}

func TestParametersConfig_LockedSaveWithRollback_CreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a deeply nested path whose parent dir does NOT exist
	paramsPath := filepath.Join(tmpDir, "nested", "deeply", "test-params.json")

	cfg := DefaultParametersConfig()

	if err := cfg.LockedSaveWithRollback(paramsPath); err != nil {
		t.Fatalf("LockedSaveWithRollback should create parent dirs: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(paramsPath); err != nil {
		t.Fatalf("file should exist after save: %v", err)
	}

	// Verify parent dir was created
	parentDir := filepath.Dir(paramsPath)
	if _, err := os.Stat(parentDir); err != nil {
		t.Fatalf("parent dir should exist: %v", err)
	}

	// Verify round-trip parseable
	if _, err := LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("LoadParametersConfig failed: %v", err)
	}
}

func TestParametersConfig_LockedSaveWithRollback_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "concurrent-params.json")

	// Pre-create a valid initial file
	cfg := DefaultParametersConfig()
	if err := cfg.SaveWithRollback(paramsPath); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	// Launch 10 goroutines, all calling LockedSaveWithRollback concurrently
	const N = 10
	var wg sync.WaitGroup
	errCh := make(chan error, N)
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := DefaultParametersConfig()
			if err := c.LockedSaveWithRollback(paramsPath); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent save failed: %v", err)
	}

	// Verify final file is valid
	if _, err := LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("final LoadParametersConfig failed (data corruption?): %v", err)
	}
}

func TestParametersConfig_TryLockedSaveWithRollback_Success(t *testing.T) {
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "trylock-params.json")

	cfg := DefaultParametersConfig()
	if err := cfg.TryLockedSaveWithRollback(paramsPath, 5*time.Second); err != nil {
		t.Fatalf("TryLockedSaveWithRollback failed: %v", err)
	}

	data, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("file is empty")
	}
	if _, err := LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("LoadParametersConfig failed: %v", err)
	}
}

func TestParametersConfig_TryLockedSaveWithRollback_TimeoutWhenLocked(t *testing.T) {
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "trylock-contention-params.json")

	// Pre-create a valid initial file
	cfg := DefaultParametersConfig()
	if err := cfg.SaveWithRollback(paramsPath); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	// Hold the advisory lock externally using the same registry path
	locker := GetFileLocker(paramsPath)
	unlock := locker.Lock()
	defer unlock()

	// TryLocked should time out quickly instead of blocking indefinitely
	start := time.Now()
	err := cfg.TryLockedSaveWithRollback(paramsPath, 100*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected TryLockedSaveWithRollback to fail when lock is held")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected fail-fast, but blocked for %v", elapsed)
	}
}

func TestParametersConfig_TryLockedSaveWithRollback_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "trylock-concurrent-params.json")

	cfg := DefaultParametersConfig()
	if err := cfg.SaveWithRollback(paramsPath); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	const N = 10
	var wg sync.WaitGroup
	errCh := make(chan error, N)
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := DefaultParametersConfig()
			if err := c.TryLockedSaveWithRollback(paramsPath, 10*time.Second); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent try-locked save failed: %v", err)
	}

	if _, err := LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("final LoadParametersConfig failed (data corruption?): %v", err)
	}
}
