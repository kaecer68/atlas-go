package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestParameters_PublicAPI locks the exported surface of parameters.go:
// 6 package funcs + 7 methods on *ParametersConfig + 42 type declarations.
// Any change to public signatures during #611 refactor fails this test.
func TestParameters_PublicAPI(t *testing.T) {
	snap, err := snapshot.CaptureAPI("parameters.go")
	if err != nil {
		t.Fatalf("CaptureAPI: %v", err)
	}

	snapshot.AssertAPI(t, snap, "testdata/parameters_api.golden.json")
}

// TestParametersConfig_Validate_DefaultGolden locks the happy-path behavior:
// a fresh DefaultParametersConfig must Validate() without error. This protects
// against refactor regressions that introduce validation errors in default
// values (e.g. weight_min >= weight_max, kelly_fraction <= 0).
func TestParametersConfig_Validate_DefaultGolden(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config failed validation: %v\nthis is a refactor regression — the default config must always validate", err)
	}
}

// TestParametersConfig_Validate_InvalidGolden locks the error-message format
// for known-invalid configs. Refactor must preserve error messages verbatim
// because downstream code and operators rely on grep-able, stable error
// strings for alerting.
func TestParametersConfig_Validate_InvalidGolden(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*config.ParametersConfig)
		want string // expected error substring
	}{
		{
			name: "weight_min_gte_weight_max",
			mut: func(p *config.ParametersConfig) {
				p.Darwinian.WeightMin.Value = p.Darwinian.WeightMax.Value
			},
			want: "darwinian.weight_min",
		},
		{
			name: "ema_alpha_negative",
			mut: func(p *config.ParametersConfig) {
				p.Darwinian.EMAAlpha.Value = -0.1
			},
			want: "darwinian.ema_alpha must be in [0,1]",
		},
		{
			name: "kelly_fraction_zero",
			mut: func(p *config.ParametersConfig) {
				p.Sizing.KellyFraction.Value = 0
			},
			want: "sizing.kelly_fraction must be in (0,1]",
		},
	}

	results := make(map[string]string)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultParametersConfig()
			tc.mut(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.name)
			}
			results[tc.name] = err.Error()
			if !strings.Contains(results[tc.name], tc.want) {
				t.Fatalf("error message mismatch\n  got:  %s\n  want: %s", results[tc.name], tc.want)
			}
		})
	}

	snapshot.AssertGoldenJSON(t, results, "testdata/validate_errors.golden.json")
}

// TestLoadParametersConfig_FallbackGolden locks the silent-fallback behavior:
// a missing path returns DefaultParametersConfig with no error. Per
// internal/config/AGENTS.md, this is intentional but must not change shape.
func TestLoadParametersConfig_FallbackGolden(t *testing.T) {
	cfg, err := config.LoadParametersConfig("/nonexistent/path/parameters.json")
	if err != nil {
		t.Fatalf("LoadParametersConfig fallback failed: %v (must return defaults silently per AGENTS.md)", err)
	}
	if cfg == nil {
		t.Fatal("LoadParametersConfig returned nil cfg on fallback")
	}
	// Sanity: returned config must be valid defaults.
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fallback config failed validation: %v", err)
	}
}

// BenchmarkValidate captures the cost of the validation pass. Called on every
// LoadParametersConfig and on every config hot-reload. Refactor must not
// regress.
func BenchmarkValidate(b *testing.B) {
	cfg := config.DefaultParametersConfig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Validate()
	}
}

// BenchmarkLoadParametersConfig captures the cost of reading + parsing +
// merging defaults + validating a saved JSON. Refactor must not regress.
//
// The fixture is generated once before the timed loop: serialize
// DefaultParametersConfig to JSON. An empty `{}` fixture would not
// exercise the full merge + validate path because some defaults
// (darwinian.weight_min/max, kelly_fraction, etc.) are NOT filled by
// the merge*Defaults helpers when the source JSON omits them — they
// only fill fields that newer code versions expect to find. The empty-
// fixture gap is itself a Layer 3 finding (see Phase 2a KNOWN ISSUE).
func BenchmarkLoadParametersConfig(b *testing.B) {
	tmpDir := b.TempDir()
	cfg := config.DefaultParametersConfig()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		b.Fatalf("marshal default config: %v", err)
	}
	fixturePath := filepath.Join(tmpDir, "valid_parameters.json")
	if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := config.LoadParametersConfig(fixturePath)
		if err != nil {
			b.Fatalf("LoadParametersConfig: %v", err)
		}
	}
}
