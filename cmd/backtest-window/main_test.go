package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestRunBacktestWindow(t *testing.T) {
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	defer func() {
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
	}()

	// Use sample replay data
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	dir := t.TempDir()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	// Keep the test hermetic when the host environment defaults to sqlite.
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))

	if err := run([]string{"-start", "2026-03-26", "-end", "2026-03-27"}); err != nil {
		t.Fatalf("run backtest-window: %v", err)
	}
}

func TestRunRejectsInvalidDate(t *testing.T) {
	if err := run([]string{"-start", "invalid", "-end", "2026-03-27"}); err == nil {
		t.Fatalf("expected error for invalid start date")
	}
}

// TestParseParamOverride_Valid verifies the helper splits a single override
// "name=value" into its parts and parses the value as float64.
func TestParseParamOverride_Valid(t *testing.T) {
	cases := []struct {
		input  string
		name   string
		floatV float64
	}{
		{"sizing_kelly_fraction=0.25", "sizing_kelly_fraction", 0.25},
		{"narrative_min_trend_strength=0.4701", "narrative_min_trend_strength", 0.4701},
		{"optimizer_min_trade_size=100", "optimizer_min_trade_size", 100.0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			name, value, err := parseParamOverride(tc.input)
			if err != nil {
				t.Fatalf("parseParamOverride(%q) returned error: %v", tc.input, err)
			}
			if name != tc.name {
				t.Errorf("name: got %q, want %q", name, tc.name)
			}
			if value != tc.floatV {
				t.Errorf("value: got %v, want %v", value, tc.floatV)
			}
		})
	}
}

// TestParseParamOverride_Invalid verifies malformed entries are rejected with
// a descriptive error.
func TestParseParamOverride_Invalid(t *testing.T) {
	cases := []string{
		"sizing_kelly_fraction",     // missing '='
		"=0.25",                     // empty name
		"sizing_kelly_fraction=",    // empty value
		"sizing_kelly_fraction=abc", // non-numeric value
		"no_such_param=0.5",         // unknown registered name
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, _, err := parseParamOverride(input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", input)
			}
		})
	}
}

// TestApplyParamOverrides_Accumulates verifies multiple overrides land on
// the same in-memory ParametersConfig.
func TestApplyParamOverrides_Accumulates(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())

	overrides := []string{
		"sizing_kelly_fraction=0.25",
		"narrative_min_trend_strength=0.4701",
	}
	if err := applyParamOverrides(ie, overrides); err != nil {
		t.Fatalf("applyParamOverrides: %v", err)
	}

	gotKelly, ok := ie.GetParameter("sizing_kelly_fraction")
	if !ok {
		t.Fatalf("sizing_kelly_fraction not registered")
	}
	if gotKelly != 0.25 {
		t.Errorf("kelly: got %v, want 0.25", gotKelly)
	}

	gotTrend, ok := ie.GetParameter("narrative_min_trend_strength")
	if !ok {
		t.Fatalf("narrative_min_trend_strength not registered")
	}
	if gotTrend != 0.4701 {
		t.Errorf("trend: got %v, want 0.4701", gotTrend)
	}
}

// TestApplyParamOverrides_RejectsUnknownParam ensures unknown names fail fast.
func TestApplyParamOverrides_RejectsUnknownParam(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	err := applyParamOverrides(ie, []string{"definitely_not_a_real_param=0.5"})
	if err == nil {
		t.Fatalf("expected error for unknown param, got nil")
	}
	if !strings.Contains(err.Error(), "definitely_not_a_real_param") {
		t.Errorf("error %v should name the offending param", err)
	}
}

// TestMaterializeParamConfig_OverriddenValuesWritten verifies the helper that
// serializes a ParametersConfig with overrides to a temp JSON file. The
// resulting file must round-trip and contain the overridden numeric values.
func TestMaterializeParamConfig_OverriddenValuesWritten(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	if err := applyParamOverrides(ie, []string{
		"sizing_kelly_fraction=0.25",
		"narrative_min_trend_strength=0.4701",
	}); err != nil {
		t.Fatalf("applyParamOverrides: %v", err)
	}

	path, err := materializeParamConfig(ie)
	if err != nil {
		t.Fatalf("materializeParamConfig: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(path)) }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp params: %v", err)
	}
	loaded, err := config.LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}

	// Validate the values in the file match what we set, not the defaults.
	if loaded.Sizing.KellyFraction.Value != 0.25 {
		t.Errorf("kelly in file: got %v, want 0.25", loaded.Sizing.KellyFraction.Value)
	}
	if loaded.Narrative.MinTrendStrength.Value != 0.4701 {
		t.Errorf("trend in file: got %v, want 0.4701", loaded.Narrative.MinTrendStrength.Value)
	}

	// JSON must parse without error and be an object, not a bare array.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("temp params not valid JSON object: %v", err)
	}
	if _, ok := raw["sizing"]; !ok {
		t.Errorf("expected 'sizing' top-level key in serialized params, got keys: %v", reflect.ValueOf(raw).MapKeys())
	}
}

// TestRunWithParamOverride verifies the -param-override flag is plumbed all
// the way through main.run() and produces a backtest that does not error.
// We point cfg.ParametersConfigPath at a temp copy via the override path.
func TestRunWithParamOverride(t *testing.T) {
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	defer func() {
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
	}()

	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	dir := t.TempDir()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	// Keep the test hermetic when the host environment defaults to sqlite.
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))

	err := run([]string{
		"-start", "2026-03-26",
		"-end", "2026-03-27",
		"-param-override", "sizing_kelly_fraction=0.25",
		"-param-override", "narrative_min_trend_strength=0.4701",
	})
	if err != nil {
		t.Fatalf("run with -param-override: %v", err)
	}
}

// TestRunWithInvalidParamOverrideErrors verifies the run() surface surfaces
// override errors (not silent ignores).
func TestRunWithInvalidParamOverrideErrors(t *testing.T) {
	err := run([]string{
		"-start", "2026-03-26",
		"-end", "2026-03-27",
		"-param-override", "garbage_format",
	})
	if err == nil {
		t.Fatalf("expected error for malformed -param-override, got nil")
	}
	if !strings.Contains(err.Error(), "param-override") {
		t.Errorf("error %v should mention param-override", err)
	}
}
