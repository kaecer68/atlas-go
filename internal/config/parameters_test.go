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
	if cfg.Engine.Executors.ConvictionFloorDefault.Value != 50 {
		t.Errorf("expected Engine.Executors.ConvictionFloorDefault = 50, got %d", cfg.Engine.Executors.ConvictionFloorDefault.Value)
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
				for k, v := range c.Engine.Drawdown.Levels.Value {
					levels[k] = v
				}
				levels["light"] = DrawdownLevel{Percentage: 1.5, MaxExposure: 0.85}
				c.Engine.Drawdown.Levels.Value = levels
			},
			wantErr: true,
		},
		{
			name: "engine_sector_rotation_alloc_not_sum_1",
			mutate: func(c *ParametersConfig) {
				allocs := make(map[string]float64)
				for k, v := range c.Engine.SectorRotation.BaseAllocations.Value {
					allocs[k] = v
				}
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
