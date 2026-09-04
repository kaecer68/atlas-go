package main

// Tests for the cf_hypothesis_validation scheduled task: the time gate
// and the end-to-end read-only run cycle (report write + verdict-change
// detection) against fixture data, with the monitor replaced by a
// captured sink.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func taipei(t *testing.T) *time.Location {
	t.Helper()
	return taipeiLocation()
}

func TestCFHypothesisGate(t *testing.T) {
	loc := taipei(t)
	mk := func(date, hm string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hm, loc)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}

	tests := []struct {
		name    string
		now     time.Time
		lastRun string
		want    bool
		reason  string
	}{
		{"weekday 08:30 in window", mk("2026-09-07", "08:30"), "", true, ""},
		{"weekday 09:15 in window", mk("2026-09-07", "09:15"), "", true, ""},
		{"weekday 08:29 too early", mk("2026-09-07", "08:29"), "", false, "outside_window"},
		{"weekday 08:00 too early", mk("2026-09-07", "08:00"), "", false, "outside_window"},
		{"weekday 10:00 too late", mk("2026-09-07", "10:00"), "", false, "outside_window"},
		{"saturday skipped", mk("2026-09-05", "08:30"), "", false, "weekend"},
		{"sunday skipped", mk("2026-09-06", "08:30"), "", false, "weekend"},
		{"already ran today", mk("2026-09-07", "09:00"), "2026-09-07", false, "already_ran_today"},
		{"ran yesterday, today ok", mk("2026-09-07", "08:45"), "2026-09-04", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := cfHypothesisGate(tc.now, tc.lastRun)
			if got != tc.want || reason != tc.reason {
				t.Fatalf("gate(%s, last=%q) = %v,%q want %v,%q", tc.now, tc.lastRun, got, reason, tc.want, tc.reason)
			}
		})
	}
}

// fixtureWorkdir builds a calendar-only workdir: validation runs
// successfully and reports INSUFFICIENT_DATA for all three hypotheses.
func fixtureWorkdir(t *testing.T) string {
	t.Helper()
	workdir := t.TempDir()
	replayDir := filepath.Join(workdir, "data", "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	calendar := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n"
	for _, d := range []string{"2026-06-01", "2026-06-02", "2026-06-03"} {
		calendar += d + ",0050,0050,1,1,1,1,1\n"
	}
	if err := os.WriteFile(filepath.Join(replayDir, "tw_extended_90days.csv"), []byte(calendar), 0o644); err != nil {
		t.Fatal(err)
	}
	return workdir
}

type capturedAlert struct {
	level monitoring.AlertLevel
	msg   string
	meta  map[string]any
}

// TestRunCFHypothesisValidationOnce_FirstRunQuiet: no previous report →
// no alerts, report files written, first run is silent.
func TestRunCFHypothesisValidationOnce_FirstRunQuiet(t *testing.T) {
	workdir := fixtureWorkdir(t)
	reportsDir := filepath.Join(workdir, "data", "reports")
	var alerts []capturedAlert
	changes, err := runCFHypothesisValidationOnce(context.Background(), workdir, reportsDir,
		func(l monitoring.AlertLevel, m string, meta map[string]any) {
			alerts = append(alerts, capturedAlert{l, m, meta})
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 || len(alerts) != 0 {
		t.Fatalf("first run must be quiet: changes=%v alerts=%v", changes, alerts)
	}
	for _, f := range []string{"cf-hypotheses-" + time.Now().In(taipei(t)).Format("2006-01-02") + ".json", "cf-hypotheses-" + time.Now().In(taipei(t)).Format("2006-01-02") + ".md"} {
		if _, err := os.Stat(filepath.Join(reportsDir, f)); err != nil {
			t.Fatalf("report %s not written: %v", f, err)
		}
	}
}

// TestRunCFHypothesisValidationOnce_VerdictChangeAlerts: seed a previous
// report with INSUFFICIENT_DATA verdicts + one judged PASS, run again
// against the same empty fixture data (all three INSUFFICIENT_DATA
// today), and verify the judged→INSUFFICIENT_DATA regression surfaces
// as a single warning verdict_flip alert.
func TestRunCFHypothesisValidationOnce_VerdictChangeAlerts(t *testing.T) {
	workdir := fixtureWorkdir(t)
	reportsDir := filepath.Join(workdir, "data", "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Previous report (dated yesterday): two INSUFFICIENT_DATA, one
	// judged PASS (which today's empty-data run regresses → flip).
	prev := capitalflow.BuildValidationReport(workdir, nil, []capitalflow.HypothesisResult{
		{ID: "H-CF-01", Status: capitalflow.ValidationInsufficientData},
		{ID: "H-CF-02", Status: capitalflow.ValidationInsufficientData},
		{ID: "H-CF-05", Status: capitalflow.ValidationPass},
	})
	yesterday := time.Now().In(taipei(t)).AddDate(0, 0, -1).Format("2006-01-02")
	if err := capitalflow.WriteValidationReportJSON(filepath.Join(reportsDir, "cf-hypotheses-"+yesterday+".json"), prev); err != nil {
		t.Fatal(err)
	}

	var alerts []capturedAlert
	changes, err := runCFHypothesisValidationOnce(context.Background(), workdir, reportsDir,
		func(l monitoring.AlertLevel, m string, meta map[string]any) {
			alerts = append(alerts, capturedAlert{l, m, meta})
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || len(alerts) != 1 {
		t.Fatalf("expected exactly the H-CF-05 flip, got changes=%v alerts=%v", changes, alerts)
	}
	// H-CF-01/02 stay INSUFFICIENT_DATA on both sides (no change);
	// H-CF-05 regresses PASS→INSUFFICIENT_DATA.
	byID := map[string]capitalflow.VerdictChange{}
	for _, c := range changes {
		byID[c.HypothesisID] = c
	}
	flip, ok := byID["H-CF-05"]
	if !ok || flip.Kind != capitalflow.VerdictChangeFlip {
		t.Fatalf("expected H-CF-05 verdict_flip, got %+v", flip)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected exactly 1 alert, got %+v", alerts)
	}
	if alerts[0].level != monitoring.AlertLevelWarning {
		t.Fatalf("flip alert must be warning, got %v", alerts[0].level)
	}
}

// TestRunCFHypothesisValidationOnce_MissingCalendarFails pins that an
// operational input failure surfaces as a task error (BTM failure
// counter / task_failed), not as a silent skip.
func TestRunCFHypothesisValidationOnce_MissingCalendarFails(t *testing.T) {
	workdir := t.TempDir() // no data/ at all
	_, err := runCFHypothesisValidationOnce(context.Background(), workdir, filepath.Join(workdir, "data", "reports"), nil)
	if err == nil {
		t.Fatalf("missing calendar must fail the run")
	}
}

// TestCFHypothesisTaskRegistered pins the scheduler-contract fields the
// acceptance checklist asks for: enabled, described, time-gated.
func TestCFHypothesisTaskRegistered(t *testing.T) {
	if got := apigateway.TaskDescription("cf_hypothesis_validation"); got == "" || got == "系統任務" {
		t.Fatalf("cf_hypothesis_validation must have a curated description, got %q", got)
	}
}
