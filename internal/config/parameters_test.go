package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

	// Factor
	if cfg.Factor.MomentumStdDevDivisor.Value != 0.30 {
		t.Errorf("expected momentum_divisor 0.30, got %f", cfg.Factor.MomentumStdDevDivisor.Value)
	}

	// Optimizer
	if cfg.Optimizer.MaxPositionPct.Value != 0.15 {
		t.Errorf("expected max_position_pct 0.15, got %f", cfg.Optimizer.MaxPositionPct.Value)
	}

	// Sizing
	if cfg.Sizing.KellyFraction.Value != 0.25 {
		t.Errorf("expected kelly_fraction 0.25, got %f", cfg.Sizing.KellyFraction.Value)
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
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
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
