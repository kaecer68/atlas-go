package main

import (
	"encoding/json"
	"testing"
	"time"

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
