package main

// Loader and report-writer tests for the offline validator CLI. The
// statistical verdict logic is locked in internal/capitalflow; these
// tests cover the local data plumbing (TAIFEX OI / macro snapshots /
// results JSON) so a schema drift in the data files fails here first.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

func TestLoadFuturesOI(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-06-01.json", `{"date":"2026-06-01","contracts":{"TX":{"foreign":{"oi_net":-10220}}}}`)
	write("2026-06-02.json", `{"date":"2026-06-02","contracts":{"MTX":{"foreign":{"oi_net":999}}}}`)
	write("not-a-date.json", `{"contracts":{"TX":{"foreign":{"oi_net":1}}}}`)
	write("README.md", "skip")

	oi, err := loadFuturesOI(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(oi) != 1 {
		t.Fatalf("expected 1 TX day, got %v", oi)
	}
	if oi["2026-06-01"] != -10220 {
		t.Fatalf("oi = %v, want -10220", oi["2026-06-01"])
	}
}

func TestLoadFuturesOIMissingDir(t *testing.T) {
	oi, err := loadFuturesOI(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing dir should be empty, not error: %v", err)
	}
	if len(oi) != 0 {
		t.Fatalf("expected empty map, got %v", oi)
	}
}

func TestLoadMacroSnapshots(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// New-style file with both channels.
	write("2026-06-01.json", `{"taiex":{"symbol":"^TWII","value":23123.5},"tsm_adr":{"symbol":"TSM","change_pct":-1.27}}`)
	// Old-style file without the ADR channel.
	write("2026-06-02.json", `{"taiex":{"symbol":"^TWII","value":23200.1}}`)
	write("latest.json", `{"taiex":{"value":1}}`) // must be skipped

	macro, err := loadMacroSnapshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(macro) != 2 {
		t.Fatalf("expected 2 dated snapshots, got %d", len(macro))
	}
	r1 := macro["2026-06-01"]
	if !r1.hasTaiex || !r1.hasADR || r1.taiex != 23123.5 || r1.adrPct != -1.27 {
		t.Fatalf("2026-06-01 row = %+v", r1)
	}
	r2 := macro["2026-06-02"]
	if !r2.hasTaiex || r2.hasADR {
		t.Fatalf("2026-06-02 row = %+v (ADR channel must stay absent)", r2)
	}
}

func TestValidateDateArg(t *testing.T) {
	if err := validateDateArg("2026-09-04"); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
	if err := validateDateArg(""); err != nil {
		t.Fatalf("empty allowed: %v", err)
	}
	if err := validateDateArg("2026-13-99"); err == nil {
		t.Fatalf("invalid date accepted")
	}
}

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
