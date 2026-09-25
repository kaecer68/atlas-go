package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// N-C1 (issue #1944 Batch 3): industry.max_daily_weight_change was a fake risk
// control — the config declared a "Maximum 5% daily weight change to prevent
// excessive volatility" while nothing in the codebase read the field.
//
// The two tests below are the machine-checkable contract that keeps the
// declaration honest:
//
//  1. TestShippedConfigMaxDailyWeightChangeIsDeclaredUnenforced — the shipped
//     config (what operators see through GET /api/parameters/metadata) must say
//     NOT ENFORCED while no consumer exists.
//  2. TestMaxDailyWeightChangeHasNoConsumer — the code scan must find zero
//     non-test consumers, and the declaration must stay NOT ENFORCED exactly
//     while that holds. Wiring a consumer therefore forces a deliberate update
//     of the metadata instead of silently turning a dead knob into a live one.
const (
	maxDailyWeightChangeField = "MaxDailyWeightChange"
	maxDailyWeightChangeKey   = "max_daily_weight_change"
	maxDailyWeightChangeGone  = "NOT ENFORCED"
)

// moduleRoot walks up from the test working directory until it finds go.mod.
// It deliberately does NOT reuse findRepoRoot (which probes for
// "<dir>/configs/parameters.json"): a stale duplicate copy lives at
// internal/config/configs/parameters.json, so that probe returns
// internal/config instead of the repository root.
func moduleRoot() (string, error) {
	dir, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	for range 10 {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}

func TestShippedConfigMaxDailyWeightChangeIsDeclaredUnenforced(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")
	cfg, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load shipped parameters: %v", err)
	}
	meta := cfg.Industry.MaxDailyWeightChange
	if meta.Value != 0.05 {
		t.Fatalf("shipped value = %v, want 0.05 (value must not change while unenforced)", meta.Value)
	}
	if !strings.Contains(meta.Rationale, maxDailyWeightChangeGone) {
		t.Errorf("shipped rationale must declare %q, got %q", maxDailyWeightChangeGone, meta.Rationale)
	}
	if !strings.Contains(meta.Todo, maxDailyWeightChangeGone) {
		t.Errorf("shipped todo must declare %q, got %q", maxDailyWeightChangeGone, meta.Todo)
	}
}

// TestMaxDailyWeightChangeHasNoConsumer couples the code scan with the metadata.
// A non-test reference outside internal/config (the declaration itself) is a
// consumer appearing; the declaration wording must then be updated in the same
// commit, and this test fails until it is.
func TestMaxDailyWeightChangeHasNoConsumer(t *testing.T) {
	repoRoot, err := moduleRoot()
	if err != nil {
		t.Skipf("module root not found: %v", err)
	}

	var consumers []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(repoRoot, sub)
		walkErr := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// The declaration itself lives in internal/config; skip that package.
			if strings.Contains(filepath.ToSlash(path), "/internal/config/") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(data)
			if strings.Contains(text, maxDailyWeightChangeField) || strings.Contains(text, maxDailyWeightChangeKey) {
				rel, relErr := filepath.Rel(repoRoot, path)
				if relErr != nil {
					rel = path
				}
				consumers = append(consumers, rel)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", base, walkErr)
		}
	}

	declaredUnenforced := strings.Contains(
		DefaultParametersConfig().Industry.MaxDailyWeightChange.Rationale, maxDailyWeightChangeGone)

	if len(consumers) == 0 && !declaredUnenforced {
		t.Fatalf("no consumer reads %s, but the declaration no longer says %q — the config would again claim a risk control that does not exist",
			maxDailyWeightChangeField, maxDailyWeightChangeGone)
	}
	if len(consumers) > 0 && declaredUnenforced {
		t.Fatalf("consumer(s) now read %s: %v — update the declaration in configs/parameters.json, configs/parameters/industry.json and defaults_narrative.go to describe what is actually enforced, then drop %q",
			maxDailyWeightChangeField, consumers, maxDailyWeightChangeGone)
	}
	t.Logf("consumers=%d declaredUnenforced=%v", len(consumers), declaredUnenforced)
}
