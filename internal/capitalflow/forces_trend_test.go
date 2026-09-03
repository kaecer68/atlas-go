package capitalflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// ===========================================================================
// M8 — trendFor thresholds are configurable (audit 2026-09-04).
// Defaults (+0.5 / -0.5) must reproduce the legacy hardcoded behaviour
// exactly; changing capitalflow.trend_bullish_threshold must change the
// point at which a Z-score is labelled bullish.
// ===========================================================================

func TestClassifyTrend_DefaultsMatchLegacyBehavior(t *testing.T) {
	cases := []struct {
		name string
		z    float64
		want string
	}{
		{"above_bullish", 0.51, "bullish"},
		{"exactly_bullish_boundary_is_neutral", 0.5, "neutral"},
		{"small_positive_neutral", 0.1, "neutral"},
		{"zero_neutral", 0, "neutral"},
		{"exactly_bearish_boundary_is_neutral", -0.5, "neutral"},
		{"below_bearish", -0.51, "bearish"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyTrend(c.z, 0.5, -0.5); got != c.want {
				t.Errorf("classifyTrend(%v, 0.5, -0.5) = %q, want %q", c.z, got, c.want)
			}
		})
	}
}

// TestClassifyTrend_Parameterization proves the acceptance case of M8:
// with default thresholds z=0.4 is neutral; lowering the bullish
// threshold to 0.3 turns the same z bullish.
func TestClassifyTrend_Parameterization(t *testing.T) {
	if got := classifyTrend(0.4, 0.5, -0.5); got != "neutral" {
		t.Errorf("z=0.4 with bullish threshold 0.5: want neutral, got %q", got)
	}
	if got := classifyTrend(0.4, 0.3, -0.3); got != "bullish" {
		t.Errorf("z=0.4 with bullish threshold 0.3: want bullish, got %q", got)
	}
	if got := classifyTrend(-0.4, 0.5, -0.5); got != "neutral" {
		t.Errorf("z=-0.4 with bearish threshold -0.5: want neutral, got %q", got)
	}
	if got := classifyTrend(-0.4, 0.3, -0.3); got != "bearish" {
		t.Errorf("z=-0.4 with bearish threshold -0.3: want bearish, got %q", got)
	}
}

// TestTrendFor_ReadsConfiguredThresholds exercises the full wiring:
// trendFor reads the capitalflow config thresholds through the config
// singleton (same path as Score in production). A config whose bullish
// threshold is 0.3 must label z=0.4 bullish.
func TestTrendFor_ReadsConfiguredThresholds(t *testing.T) {
	prevPath := config.GetParametersConfigPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})

	tmp := t.TempDir()
	path := filepath.Join(tmp, "parameters.json")

	cfg := config.DefaultParametersConfig()
	cfg.Capitalflow.TrendBullishThreshold.Value = 0.3
	cfg.Capitalflow.TrendBearishThreshold.Value = -0.3
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	defer config.ResetParametersConfig()

	if got := trendFor(0.4); got != "bullish" {
		t.Errorf("trendFor(0.4) with configured threshold 0.3 = %q, want bullish", got)
	}
	if got := trendFor(-0.4); got != "bearish" {
		t.Errorf("trendFor(-0.4) with configured threshold -0.3 = %q, want bearish", got)
	}
	// Sanity: the config really loaded (not silently defaulted).
	if b := config.GetCapitalflowTrendBullishThreshold(); b != 0.3 {
		t.Errorf("GetCapitalflowTrendBullishThreshold() = %v, want 0.3 (config did not load)", b)
	}
}

// TestTrendFor_UnconfiguredDefaultsToMetadata asserts the no-config path
// still yields the metadata defaults (0.5 / -0.5) so process bootstrap
// before any config load keeps the legacy labels.
func TestTrendFor_UnconfiguredDefaultsToMetadata(t *testing.T) {
	prevPath := config.GetParametersConfigPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})
	config.SetParametersConfigPath(filepath.Join(t.TempDir(), "does-not-exist.json"))
	config.ResetParametersConfig()

	if got := trendFor(0.6); got != "bullish" {
		t.Errorf("trendFor(0.6) unconfigured = %q, want bullish (default 0.5)", got)
	}
	if got := trendFor(0.4); got != "neutral" {
		t.Errorf("trendFor(0.4) unconfigured = %q, want neutral", got)
	}
}
