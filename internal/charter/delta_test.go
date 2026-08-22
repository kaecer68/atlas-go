package charter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fixtureReport builds a step report JSON with the fields the delta
// conversion reads. The numbers mirror the real C3 reports
// (/tmp/charter-ab/step-*.json) where noted.
func fixtureReport(t *testing.T, step int, switches []string, baseline, feature map[string]float64, pairedT map[string]any, sharpeBCa, maxddBCa map[string]any, rawBase, rawFeat int) []byte {
	t.Helper()
	sw, _ := json.Marshal(switches)
	b, _ := json.Marshal(baseline)
	f, _ := json.Marshal(feature)
	pt, _ := json.Marshal(pairedT)
	sh, _ := json.Marshal(sharpeBCa)
	md, _ := json.Marshal(maxddBCa)

	return []byte(`{
  "step": ` + itoa(step) + `,
  "arm": "test-arm",
  "feature_switches": ` + string(sw) + `,
  "window": {"start": "2026-01-01", "end": "2026-08-21", "days": 166},
  "baseline": ` + string(b) + `,
  "feature": ` + string(f) + `,
  "stats": {
    "paired_t_test_daily_return_diff": ` + string(pt) + `,
    "sharpe_diff_bca_95": ` + string(sh) + `,
    "max_drawdown_diff_bca_95": ` + string(md) + `
  },
  "attribution": {"raw_recs_baseline": ` + itoa(rawBase) + `, "raw_recs_feature": ` + itoa(rawFeat) + `}
}`)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func runMetrics(sharpe, maxDD, winRate, totalReturn float64) map[string]float64 {
	return map[string]float64{
		"sharpe": sharpe, "max_drawdown": maxDD, "win_rate": winRate, "total_return": totalReturn,
	}
}

// ─── conversion + verdict tests ───────────────────────────────────────────

// TestParseDeltaReport_Step2StrategyFilter uses the real step-2 numbers
// (Sharpe 2.742→3.320, MaxDD 0.474→0.308, WinRate 0.510→0.600): three
// directional indicators improve → directional_watch despite p=0.327.
func TestParseDeltaReport_Step2StrategyFilter(t *testing.T) {
	data := fixtureReport(t, 2, []string{"PeriodOnly", "StrategyFilter"},
		runMetrics(2.742, 0.474, 0.510, 3.780218),
		runMetrics(3.320, 0.308, 0.600, 1.009736),
		map[string]any{"t": -0.9836, "df": 164, "p": 0.3268, "mean_daily_return_diff": -0.025568, "significant": false},
		map[string]any{"observed_diff": 0.5785, "ci95_low": -0.7116, "ci95_high": 2.0688, "resamples": 10000, "significant": false, "degenerate": false},
		map[string]any{"observed_diff": 0.1662, "ci95_low": 0.0064, "ci95_high": 0.0133, "resamples": 10000, "significant": false, "degenerate": true},
		7581, 130)

	d, err := ParseDeltaReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Step != 2 || d.Switch != "StrategyFilter" {
		t.Errorf("step/switch = %d/%s, want 2/StrategyFilter", d.Step, d.Switch)
	}
	if d.Verdict != VerdictDirectionalWatch {
		t.Errorf("verdict = %s, want directional_watch", d.Verdict)
	}
	if d.Window.Days != 166 || d.Window.Start != "2026-01-01" {
		t.Errorf("window = %+v", d.Window)
	}
	if got := d.MetricDiffs.Sharpe; !closeTo(got, 0.5785, 0.001) {
		t.Errorf("sharpe diff = %.4f, want ≈0.5785", got)
	}
	if got := d.MetricDiffs.MaxDD; !closeTo(got, 0.1662, 0.001) {
		t.Errorf("maxdd diff = %.4f, want ≈0.1662 (baseline−feature)", got)
	}
	if got := d.MetricDiffs.WinRate; !closeTo(got, 0.0900, 0.001) {
		t.Errorf("winrate diff = %.4f, want ≈0.090", got)
	}
	if d.MetricDiffs.RawRecs != -7451 {
		t.Errorf("raw recs diff = %d, want -7451", d.MetricDiffs.RawRecs)
	}
	if d.PairedT.P != 0.3268 || d.PairedT.T != -0.9836 {
		t.Errorf("paired t = %+v", d.PairedT)
	}
	if !d.MaxDDDegenerate {
		t.Error("maxdd BCa should be marked degenerate")
	}
	if d.Evidence == "" || !contains(d.Evidence, "p=0.3268") {
		t.Errorf("evidence missing p value: %q", d.Evidence)
	}
	if d.ExperimentID() != "charter-ab-step-2" {
		t.Errorf("experiment id = %s", d.ExperimentID())
	}
}

// TestParseDeltaReport_Step3MacroFlowInert — identical runs (all deltas ≈ 0)
// classify as inert.
func TestParseDeltaReport_Step3MacroFlowInert(t *testing.T) {
	data := fixtureReport(t, 3, []string{"PeriodOnly", "StrategyFilter", "MacroFlow"},
		runMetrics(3.320, 0.308, 0.600, 1.009736),
		runMetrics(3.320, 0.308, 0.600, 1.009736),
		map[string]any{"t": -0.3267, "df": 164, "p": 0.7443, "mean_daily_return_diff": -0.000000, "significant": false},
		map[string]any{"observed_diff": 0.0, "ci95_low": -0.0000, "ci95_high": 0.0000, "resamples": 10000, "significant": false, "degenerate": false},
		map[string]any{"observed_diff": 0.0, "ci95_low": -0.0000, "ci95_high": -0.0000, "resamples": 10000, "significant": false, "degenerate": true},
		130, 130)

	d, err := ParseDeltaReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Verdict != VerdictInert {
		t.Errorf("verdict = %s, want inert", d.Verdict)
	}
}

// TestParseDeltaReport_Step1PeriodOnlyDegenerate — mixed directional signals
// with a degenerate MaxDD BCa CI classify as degenerate.
func TestParseDeltaReport_Step1PeriodOnlyDegenerate(t *testing.T) {
	data := fixtureReport(t, 1, []string{"PeriodOnly"},
		runMetrics(3.006, 0.318, 0.506, 103.0599),
		runMetrics(2.816, 0.487, 0.510, 582.6449),
		map[string]any{"t": 1.1875, "df": 164, "p": 0.2368, "mean_daily_return_diff": 0.030681, "significant": false},
		map[string]any{"observed_diff": -0.1902, "ci95_low": -1.2810, "ci95_high": 0.1924, "resamples": 10000, "significant": false, "degenerate": false},
		map[string]any{"observed_diff": -0.1691, "ci95_low": -0.008, "ci95_high": -0.005, "resamples": 10000, "significant": false, "degenerate": true},
		7736, 7579)

	d, err := ParseDeltaReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Verdict != VerdictDegenerate {
		t.Errorf("verdict = %s, want degenerate", d.Verdict)
	}
}

// TestParseDeltaReport_SignificantEnable — p<0.05 with positive mean diff
// classifies as significant_enable.
func TestParseDeltaReport_SignificantEnable(t *testing.T) {
	data := fixtureReport(t, 2, []string{"PeriodOnly", "StrategyFilter"},
		runMetrics(2.0, 0.4, 0.50, 1.0),
		runMetrics(3.0, 0.3, 0.55, 1.2),
		map[string]any{"t": 2.9, "df": 164, "p": 0.0042, "mean_daily_return_diff": 0.012, "significant": true},
		map[string]any{"observed_diff": 1.0, "ci95_low": 0.3, "ci95_high": 1.7, "resamples": 10000, "significant": true, "degenerate": false},
		map[string]any{"observed_diff": 0.1, "ci95_low": 0.0, "ci95_high": 0.2, "resamples": 10000, "significant": false, "degenerate": false},
		100, 90)

	d, err := ParseDeltaReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Verdict != VerdictSignificantEnable {
		t.Errorf("verdict = %s, want significant_enable", d.Verdict)
	}
	if !d.PairedT.Significant || d.PairedT.MeanDiff <= 0 {
		t.Errorf("paired t = %+v", d.PairedT)
	}
}

// TestParseDeltaReport_MissingRequiredFields — a report without the stats
// blocks must fail rather than produce an empty delta.
func TestParseDeltaReport_MissingRequiredFields(t *testing.T) {
	if _, err := ParseDeltaReport([]byte(`{"step": 1}`)); err == nil {
		t.Fatal("expected error for report missing stats/arms fields")
	}
}

// TestLoadDeltasFromReports — reads step-1..3 in order from a temp dir and
// preserves verdicts.
func TestLoadDeltasFromReports(t *testing.T) {
	dir := t.TempDir()
	for step := 1; step <= 3; step++ {
		var data []byte
		switch step {
		case 1:
			data = fixtureReport(t, 1, []string{"PeriodOnly"},
				runMetrics(3.006, 0.318, 0.506, 103.0), runMetrics(2.816, 0.487, 0.510, 582.0),
				map[string]any{"t": 1.18, "df": 164, "p": 0.237, "mean_daily_return_diff": 0.03, "significant": false},
				map[string]any{"observed_diff": -0.19, "ci95_low": -1.28, "ci95_high": 0.19, "resamples": 10000, "significant": false, "degenerate": false},
				map[string]any{"observed_diff": -0.17, "ci95_low": -0.008, "ci95_high": -0.005, "resamples": 10000, "significant": false, "degenerate": true},
				7736, 7579)
		case 2:
			data = fixtureReport(t, 2, []string{"PeriodOnly", "StrategyFilter"},
				runMetrics(2.742, 0.474, 0.510, 3.78), runMetrics(3.320, 0.308, 0.600, 1.01),
				map[string]any{"t": -0.98, "df": 164, "p": 0.327, "mean_daily_return_diff": -0.026, "significant": false},
				map[string]any{"observed_diff": 0.58, "ci95_low": -0.71, "ci95_high": 2.07, "resamples": 10000, "significant": false, "degenerate": false},
				map[string]any{"observed_diff": 0.17, "ci95_low": 0.006, "ci95_high": 0.013, "resamples": 10000, "significant": false, "degenerate": true},
				7581, 130)
		default:
			data = fixtureReport(t, 3, []string{"PeriodOnly", "StrategyFilter", "MacroFlow"},
				runMetrics(3.32, 0.308, 0.6, 1.009736), runMetrics(3.32, 0.308, 0.6, 1.009736),
				map[string]any{"t": -0.33, "df": 164, "p": 0.744, "mean_daily_return_diff": 0, "significant": false},
				map[string]any{"observed_diff": 0, "ci95_low": 0, "ci95_high": 0, "resamples": 10000, "significant": false, "degenerate": false},
				map[string]any{"observed_diff": 0, "ci95_low": 0, "ci95_high": 0, "resamples": 10000, "significant": false, "degenerate": true},
				130, 130)
		}
		if err := os.WriteFile(filepath.Join(dir, "step-"+itoa(step)+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	deltas, err := LoadDeltasFromReports(dir, 3)
	if err != nil {
		t.Fatalf("load deltas: %v", err)
	}
	if len(deltas) != 3 {
		t.Fatalf("deltas = %d, want 3", len(deltas))
	}
	want := []Verdict{VerdictDegenerate, VerdictDirectionalWatch, VerdictInert}
	for i, d := range deltas {
		if d.Step != i+1 {
			t.Errorf("delta[%d].Step = %d, want %d", i, d.Step, i+1)
		}
		if d.Verdict != want[i] {
			t.Errorf("delta[%d].Verdict = %s, want %s", i, d.Verdict, want[i])
		}
	}
}

// TestLoadDeltasFromReports_MissingFile — a missing step report must error
// (the writeback must not silently drop evidence).
func TestLoadDeltasFromReports_MissingFile(t *testing.T) {
	if _, err := LoadDeltasFromReports(t.TempDir(), 1); err == nil {
		t.Fatal("expected error for missing step report")
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// closeTo reports whether |got − want| ≤ eps (float arithmetic on JSON
// round-tripped values is exact to ~1e-15, so 1e-3 is a generous tolerance).
func closeTo(got, want, eps float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= eps
}
