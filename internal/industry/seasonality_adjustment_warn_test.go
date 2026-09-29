package industry

// seasonality_adjustment_warn_test.go — issue #1944 "zero-behaviour honesty"
// marker for out-of-range adjustment_factor values.
//
// What is being pinned, and why it needs a test at all: NewSeasonalEngineFromConfig
// already emits a WARN naming the offending patterns (I17), and the claim in its
// comment is that the out-of-range value is machine-visible. Nothing asserted that
// — the marker could be deleted by a refactor and every test would stay green,
// while production went back to silently multiplying a broken factor. These tests
// are the pin, and they assert the two halves that make the marker honest:
//
//  1. the value is STILL LOADED (the engine keeps the raw number — validation and
//     loading behaviour is unchanged; the clamp happens at consumption);
//  2. exactly one WARN names the out-of-range pattern and not the in-range one,
//     and an all-in-range configuration emits nothing.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// captureIndustryLogs swaps the global logger for the duration of the test.
func captureIndustryLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return buf
}

func seasonalConfigWithFactors(factors map[string]float64) *config.ParametersConfig {
	cfg := &config.ParametersConfig{}
	patterns := make([]config.SeasonalPatternConfig, 0, len(factors))
	// Deterministic order so the assertion on the offenders string is stable.
	for _, id := range []string{"in_range", "below_min", "above_max"} {
		f, ok := factors[id]
		if !ok {
			continue
		}
		patterns = append(patterns, config.SeasonalPatternConfig{ID: id, AdjustmentFactor: f})
	}
	cfg.Industry.SeasonalPatterns.Value = patterns
	return cfg
}

func TestNewSeasonalEngineFromConfig_OutOfRangeAdjustmentFactorIsLoadedAndWarned(t *testing.T) {
	buf := captureIndustryLogs(t)
	cfg := seasonalConfigWithFactors(map[string]float64{
		"in_range":  1.2,
		"below_min": -0.75, // the production shape: a NEGATIVE factor
		"above_max": 3.44,  // the production shape: far above the 2.5 ceiling
	})

	engine := NewSeasonalEngineFromConfig(cfg)

	// (1) zero behaviour change: the raw value is what the engine holds.
	got := map[string]float64{}
	for _, p := range engine.patterns {
		got[p.ID] = p.AdjustmentFactor
	}
	if got["below_min"] != -0.75 || got["above_max"] != 3.44 {
		t.Fatalf("out-of-range values must still load unchanged, got %v", got)
	}
	// The consumer-side clamp is a separate, already-pinned mechanism; assert it
	// here only to make the division of labour explicit.
	if c := ClampAdjustmentFactor(-0.75); c != 1.0 {
		t.Errorf("ClampAdjustmentFactor(-0.75) = %v, want 1.0 (negative means the direction was falsified)", c)
	}

	// (2) exactly one WARN, naming the two offenders and not the in-range one.
	logs := buf.String()
	if n := strings.Count(logs, "seasonal_adjustment_factor_out_of_range"); n != 1 {
		t.Fatalf("expected exactly one marker line, got %d\n--- log ---\n%s", n, logs)
	}
	for _, want := range []string{"below_min=-0.7500", "above_max=3.4400", "action=clamped_at_consumption", "value_is_still_loaded"} {
		if !strings.Contains(logs, want) {
			t.Errorf("marker line must contain %q\n--- log ---\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "in_range=1.2000") {
		t.Errorf("an in-range pattern must not be reported as an offender\n--- log ---\n%s", logs)
	}
}

func TestNewSeasonalEngineFromConfig_InRangeAdjustmentFactorsAreSilent(t *testing.T) {
	buf := captureIndustryLogs(t)
	cfg := seasonalConfigWithFactors(map[string]float64{"in_range": 1.2})

	_ = NewSeasonalEngineFromConfig(cfg)

	if logs := buf.String(); strings.Contains(logs, "seasonal_adjustment_factor_out_of_range") {
		t.Fatalf("an all-in-range configuration must emit no marker\n--- log ---\n%s", logs)
	}
}

func TestNewSeasonalEngineFromConfig_NilConfigDoesNotPanicOrWarn(t *testing.T) {
	buf := captureIndustryLogs(t)

	engine := NewSeasonalEngineFromConfig(nil)

	if len(engine.patterns) == 0 {
		t.Fatal("nil config must fall back to the default patterns")
	}
	if logs := buf.String(); strings.Contains(logs, "seasonal_adjustment_factor_out_of_range") {
		t.Fatalf("the default patterns are in range; no marker expected\n--- log ---\n%s", logs)
	}
}
