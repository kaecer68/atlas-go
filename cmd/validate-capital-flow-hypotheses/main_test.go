package main

// CLI-level tests for the offline validator. Loader plumbing and the
// statistical verdict logic live (and are tested) in
// internal/capitalflow — validation_runner_test.go and validation_test.go;
// these tests cover the report-writer contract and pin that the CLI
// binary still compiles against the pre-registered constants.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
