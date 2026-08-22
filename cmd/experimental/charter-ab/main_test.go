package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
)

// smokeConfig builds a runner config over the committed sample replay CSV
// (no gitignored data/ files required), so the smoke test runs in CI.
func smokeConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		ReplayMode:           "daily",
		PrimaryMarket:        "TW",
		ReplayDataPath:       "../../../samples/replay/twse_stock_day_all_sample.csv",
		AgentRegistryPath:    "../../../configs/agents.json",
		ParametersConfigPath: "../../../configs/parameters.json",
		WorkDir:              "../../..",
		LedgerDir:            t.TempDir(),
	}
}

// TestSmokeTwoArmsReplay runs the step-1 comparison (Phase A baseline vs
// PeriodOnly feature) over a short window of the committed sample CSV and
// verifies both arms produce a ComparisonReport with stats + attribution.
func TestSmokeTwoArmsReplay(t *testing.T) {
	cfg := smokeConfig(t)
	start, _ := time.Parse("2006-01-02", "2026-03-20")
	end, _ := time.Parse("2006-01-02", "2026-03-27")

	baseline, err := runArm(cfg, start, end, charter.Options{}, "baseline")
	if err != nil {
		t.Fatalf("baseline arm: %v", err)
	}
	if baseline.Metrics.Sessions == 0 {
		t.Fatal("baseline arm produced no sessions")
	}

	feature, err := runArm(cfg, start, end, charter.Options{PeriodOnly: true}, "feature")
	if err != nil {
		t.Fatalf("feature arm: %v", err)
	}
	if feature.Metrics.Sessions == 0 {
		t.Fatal("feature arm produced no sessions")
	}

	report := buildReport(1, "PeriodOnly", charter.Options{PeriodOnly: true}, start, end, baseline, feature)
	if report.Window.Days == 0 {
		t.Error("report window days = 0")
	}
	if report.Baseline.Sessions != report.Feature.Sessions {
		t.Errorf("arms must replay the same days: baseline=%d feature=%d",
			report.Baseline.Sessions, report.Feature.Sessions)
	}
	// Stats must be present (may be non-significant on sample data).
	if report.Stats.PairedT.DF < 0 {
		t.Error("paired t-test df invalid")
	}
	if report.Stats.Sharpe.Resamples != 10000 {
		t.Errorf("sharpe bootstrap resamples = %d, want 10000", report.Stats.Sharpe.Resamples)
	}
	if report.Stats.MaxDrawdown.Resamples != 10000 {
		t.Errorf("maxdd bootstrap resamples = %d, want 10000", report.Stats.MaxDrawdown.Resamples)
	}
	if report.Attribution.RawRecsBaseline < 0 || report.Attribution.RawRecsFeature < 0 {
		t.Error("attribution counts must be non-negative")
	}

	// The report must be JSON-serializable (this is the file contract).
	if _, err := json.MarshalIndent(report, "", "  "); err != nil {
		t.Errorf("report not JSON-serializable: %v", err)
	}
}

// TestSmokeArm5FullCharter runs the full-charter arm (step 5) to verify the
// complete switch set — including CashReserve + ConvictionFloor — executes
// through the replay pipeline without error.
func TestSmokeArm5FullCharter(t *testing.T) {
	cfg := smokeConfig(t)
	start, _ := time.Parse("2006-01-02", "2026-03-20")
	end, _ := time.Parse("2006-01-02", "2026-03-24")

	res, err := runArm(cfg, start, end, charter.AllOn(), "feature")
	if err != nil {
		t.Fatalf("full charter arm: %v", err)
	}
	if res.Metrics.Sessions == 0 {
		t.Fatal("full charter arm produced no sessions")
	}
	if res.Metrics.Arm != "feature" {
		t.Errorf("arm label = %q, want feature", res.Metrics.Arm)
	}
}

// ─── C4 writeback tests ───────────────────────────────────────────────────

// fixtureStepJSON writes a minimal step report (the fields the delta
// conversion reads) into dir and returns its path. The metrics mirror the
// real C3 step-2 (+StrategyFilter) numbers: three directional indicators
// improve → the delta classifies as directional_watch.
func fixtureStepJSON(t *testing.T, dir string, step int, switches []string) string {
	t.Helper()
	sw, _ := json.Marshal(switches)
	report := `{
  "step": ` + strconv.Itoa(step) + `,
  "arm": "fixture",
  "feature_switches": ` + string(sw) + `,
  "window": {"start": "2026-01-01", "end": "2026-08-21", "days": 166},
  "baseline": {"win_rate": 0.51, "total_return": 3.78, "sharpe": 2.742, "max_drawdown": 0.474},
  "feature": {"win_rate": 0.60, "total_return": 1.01, "sharpe": 3.320, "max_drawdown": 0.308},
  "stats": {
    "paired_t_test_daily_return_diff": {"t": -0.98, "df": 164, "p": 0.3268, "mean_daily_return_diff": -0.0256, "significant": false},
    "sharpe_diff_bca_95": {"observed_diff": 0.5785, "ci95_low": -0.71, "ci95_high": 2.07, "resamples": 10000, "significant": false, "degenerate": false},
    "max_drawdown_diff_bca_95": {"observed_diff": 0.166, "ci95_low": 0.006, "ci95_high": 0.013, "resamples": 10000, "significant": false, "degenerate": true}
  },
  "attribution": {"raw_recs_baseline": 7581, "raw_recs_feature": 130}
}`
	path := filepath.Join(dir, "step-"+strconv.Itoa(step)+".json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatalf("write fixture step: %v", err)
	}
	return path
}

// fixtureDegenerateStepJSON writes a step report whose signals are mixed and
// whose MaxDrawdown BCa CI is degenerate → verdict degenerate (finding).
func fixtureDegenerateStepJSON(t *testing.T, dir string, step int, switches []string) string {
	t.Helper()
	sw, _ := json.Marshal(switches)
	report := `{
  "step": ` + strconv.Itoa(step) + `,
  "arm": "fixture",
  "feature_switches": ` + string(sw) + `,
  "window": {"start": "2026-01-01", "end": "2026-08-21", "days": 166},
  "baseline": {"win_rate": 0.506, "total_return": 103.06, "sharpe": 3.006, "max_drawdown": 0.318},
  "feature": {"win_rate": 0.510, "total_return": 582.64, "sharpe": 2.816, "max_drawdown": 0.487},
  "stats": {
    "paired_t_test_daily_return_diff": {"t": 1.19, "df": 164, "p": 0.2368, "mean_daily_return_diff": 0.0307, "significant": false},
    "sharpe_diff_bca_95": {"observed_diff": -0.19, "ci95_low": -1.28, "ci95_high": 0.19, "resamples": 10000, "significant": false, "degenerate": false},
    "max_drawdown_diff_bca_95": {"observed_diff": -0.169, "ci95_low": -0.008, "ci95_high": -0.005, "resamples": 10000, "significant": false, "degenerate": true}
  },
  "attribution": {"raw_recs_baseline": 7736, "raw_recs_feature": 7579}
}`
	path := filepath.Join(dir, "step-"+strconv.Itoa(step)+".json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatalf("write fixture step: %v", err)
	}
	return path
}

// TestWritebackCLI — runs the writeback path end to end against a temp
// policy: step reports → deltas → promotions (watch) + version bump, then
// reloads to confirm the closed loop. Re-running appends nothing (idempotent).
func TestWritebackCLI(t *testing.T) {
	outDir := t.TempDir()
	fixtureDegenerateStepJSON(t, outDir, 1, []string{"PeriodOnly"})
	fixtureStepJSON(t, outDir, 2, []string{"PeriodOnly", "StrategyFilter"})

	policyPath := filepath.Join(t.TempDir(), "baseline_policy.json")
	if err := baseline.SaveWithLock(policyPath, baseline.DefaultPolicy()); err != nil {
		t.Fatalf("save temp policy: %v", err)
	}

	cfg := config.Config{WorkDir: "."}
	if err := runWriteback(outDir, 2, policyPath, cfg); err != nil {
		t.Fatalf("runWriteback: %v", err)
	}

	reloaded, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Version != 3 {
		t.Errorf("version = %d, want 3 (base + step-1 finding + step-2 watch)", reloaded.Version)
	}
	if len(reloaded.Promotions) != 2 {
		t.Fatalf("promotions = %d, want 2", len(reloaded.Promotions))
	}
	rec := reloaded.Promotions[1]
	if rec.ExperimentID != "charter-ab-step-2" || rec.Status != "watch" || rec.MutationType != "charter_delta_recorded" {
		t.Errorf("record = %+v", rec)
	}
	if rec.ConstraintsSnapshot != nil {
		t.Error("watch record must not carry a constraints snapshot (no runtime override)")
	}
	rec1 := reloaded.Promotions[0]
	if rec1.ExperimentID != "charter-ab-step-1" || rec1.Status != "recorded" {
		t.Errorf("step-1 finding record = %+v", rec1)
	}

	// Closed loop: re-run the writeback — the same policy loads again and no
	// records are duplicated.
	if err := runWriteback(outDir, 2, policyPath, cfg); err != nil {
		t.Fatalf("re-run writeback: %v", err)
	}
	again, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("reload after re-run: %v", err)
	}
	if len(again.Promotions) != 2 || again.Version != 3 {
		t.Errorf("re-run duplicated records: promotions=%d version=%d", len(again.Promotions), again.Version)
	}
}
