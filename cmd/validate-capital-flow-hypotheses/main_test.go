package main

// CLI-level tests for the offline validator. Loader plumbing and the
// statistical verdict logic live (and are tested) in
// internal/capitalflow — validation_runner_test.go and validation_test.go;
// these tests cover the report-writer contract and pin that the CLI
// binary still compiles against the pre-registered constants.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

func TestReportWriters(t *testing.T) {
	dir := t.TempDir()
	results := []capitalflow.HypothesisResult{{
		ID: "H-CF-TEST", Status: capitalflow.ValidationInsufficientData,
		SampleCount: 5, StartedAt: time.Now().UTC(),
		Thresholds: map[string]float64{"min_sample_days": 252},
	}}
	report := capitalflow.BuildValidationReport("/tmp/work",
		map[string]int{"calendar_days": 90}, results)
	if report.Version != capitalflow.ValidationReportVersion {
		t.Fatalf("version=%s", report.Version)
	}
	if report.EligibleRecommendation {
		t.Fatalf("INSUFFICIENT_DATA must never recommend eligibility")
	}

	jsonPath := filepath.Join(dir, "sub", "report.json")
	if err := capitalflow.WriteValidationReportJSON(jsonPath, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed capitalflow.ValidationReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("report must be valid JSON: %v", err)
	}
	if len(parsed.Hypotheses) != 1 || parsed.Hypotheses[0].ID != "H-CF-TEST" {
		t.Fatalf("unexpected report body: %s", data)
	}
	if parsed.EligibleRecommendation {
		t.Fatalf("eligibility flag must survive round-trip as false")
	}

	mdPath := filepath.Join(dir, "sub", "report.md")
	if err := capitalflow.WriteValidationReportMarkdown(mdPath, report); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"H-CF-TEST", "INSUFFICIENT_DATA", "eligible_recommendation", "Pre-registered Thresholds"} {
		if !bytes.Contains(md, []byte(want)) {
			t.Fatalf("markdown report missing %q\n%s", want, md)
		}
	}
}

func TestEligibleRecommendationRequiresAllPass(t *testing.T) {
	pass := []capitalflow.HypothesisResult{
		{ID: "A", Status: capitalflow.ValidationPass},
		{ID: "B", Status: capitalflow.ValidationPassImproved},
	}
	if r := capitalflow.BuildValidationReport("", nil, pass); !r.EligibleRecommendation {
		t.Fatalf("all-PASS run must recommend eligibility")
	}
	mixed := append(append([]capitalflow.HypothesisResult(nil), pass...),
		capitalflow.HypothesisResult{ID: "C", Status: capitalflow.ValidationFail})
	if r := capitalflow.BuildValidationReport("", nil, mixed); r.EligibleRecommendation {
		t.Fatalf("FAIL in run must block eligibility recommendation")
	}
	if r := capitalflow.BuildValidationReport("", nil, nil); r.EligibleRecommendation {
		t.Fatalf("empty run must not recommend eligibility")
	}
}

// TestCLIConsumesPreRegisteredThresholds pins that the CLI surfaces
// the same pre-registered constants the validators judge with (the
// constants themselves are locked in internal/capitalflow).
func TestCLIConsumesPreRegisteredThresholds(t *testing.T) {
	if capitalflow.ValidationMinSampleDays != 252 {
		t.Fatalf("min sample days drifted: %d", capitalflow.ValidationMinSampleDays)
	}
	if capitalflow.HCF01MinOOSHitRate != 0.55 || capitalflow.HCF02MinHitRate != 0.55 {
		t.Fatalf("55%% hit-rate gates drifted")
	}
}

// TestRegisterValidationFlags_V2 pins the CLI flag contract: -v2
// defaults to false (v1 behavior unchanged) and parses to true; the
// -out/-out-md paths pass through verbatim, which is how the caller
// selects the -r3 report filename (cf-hypotheses-<date>-r3.{json,md}).
func TestRegisterValidationFlags_V2(t *testing.T) {
	fs := flag.NewFlagSet("v2test", flag.ContinueOnError)
	flags := registerValidationFlags(fs)
	if *flags.v2 {
		t.Fatal("-v2 must default to false (v1 behavior unchanged)")
	}
	if err := fs.Parse([]string{"-v2", "-out", "data/reports/cf-hypotheses-2026-09-04-r3.json", "-out-md", "data/reports/cf-hypotheses-2026-09-04-r3.md"}); err != nil {
		t.Fatal(err)
	}
	if !*flags.v2 {
		t.Fatal("-v2 must parse to true")
	}
	if *flags.out != "data/reports/cf-hypotheses-2026-09-04-r3.json" || *flags.outMD != "data/reports/cf-hypotheses-2026-09-04-r3.md" {
		t.Fatalf("report paths must pass through verbatim: %q %q", *flags.out, *flags.outMD)
	}
}

// TestRunV2FamilyValidation pins the v2-mode runner contract on a
// calendar-only workdir: exactly the three v2 family judgments (v1
// H-CF-01 absent), all INSUFFICIENT_DATA on empty data, the report
// NEVER recommends eligibility, and the governance operator note is
// present.
func TestRunV2FamilyValidation(t *testing.T) {
	workdir := t.TempDir()
	replayDir := filepath.Join(workdir, "data", "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	calendar := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n"
	for _, d := range []string{"2026-06-01", "2026-06-02", "2026-06-03"} {
		calendar += d + ",0050,0050,1,1,1,1,1\n"
	}
	if err := os.WriteFile(filepath.Join(replayDir, capitalflow.ValidationDefaultReplayPath[len("data/replay/"):]), []byte(calendar), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := runV2FamilyValidation(workdir, "", "")
	if err != nil {
		t.Fatalf("v2 run on calendar-only workdir must not error: %v", err)
	}
	if len(report.Hypotheses) != 3 {
		t.Fatalf("expected the 3 v2 family judgments, got %d", len(report.Hypotheses))
	}
	wantIDs := map[string]bool{
		"H-CF-01-v2a": false, "H-CF-01-v2a-prime": false, "H-CF-01-v2b": false,
	}
	for _, h := range report.Hypotheses {
		if _, ok := wantIDs[h.ID]; !ok {
			t.Fatalf("unexpected hypothesis %q in v2 report (v1 H-CF-01 must stay out)", h.ID)
		}
		wantIDs[h.ID] = true
		if h.Status != capitalflow.ValidationInsufficientData {
			t.Fatalf("%s status = %s, want INSUFFICIENT_DATA on empty data", h.ID, h.Status)
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Fatalf("v2 report missing %s", id)
		}
	}
	if report.EligibleRecommendation {
		t.Fatal("v2-mode report must never recommend eligibility")
	}
	joined := strings.Join(report.OperatorNotes, "\n")
	if !strings.Contains(joined, "v2 判決模式") || !strings.Contains(joined, "永不自動建議 eligible") {
		t.Fatalf("v2 governance operator note missing: %s", joined)
	}
}
