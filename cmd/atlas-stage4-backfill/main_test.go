package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// ------------------------------------------------------------------
// TestParseSessionDate — table-driven; covers happy + bad inputs.
// ------------------------------------------------------------------

func TestParseSessionDate(t *testing.T) {
	cases := []struct {
		in       string
		wantDate string
		wantOK   bool
	}{
		{"session-20260415-daily", "2026-04-15", true},
		{"session-20260101-daily", "2026-01-01", true},
		{"session-20261231-daily", "2026-12-31", true},
		{"session-20261301-daily", "", false}, // bad month
		{"session-2026040-daily", "", false},  // bad length
		{"junk-20260415-daily", "", false},    // wrong prefix
		{"session-20260415-daily-rerun", "", false},
		{"session--daily", "", false},
	}
	for _, tc := range cases {
		gotDate, gotOK := parseSessionDate(tc.in)
		if gotDate != tc.wantDate || gotOK != tc.wantOK {
			t.Errorf("parseSessionDate(%q) = (%q, %v); want (%q, %v)",
				tc.in, gotDate, gotOK, tc.wantDate, tc.wantOK)
		}
	}
}

// ------------------------------------------------------------------
// TestPickPredominant — tally picks the mode; empty map returns "".
// ------------------------------------------------------------------

func TestPickPredominant(t *testing.T) {
	cases := []struct {
		name  string
		tally map[string]int
		want  string
		check func(string) bool
	}{
		{"empty", map[string]int{}, "", func(s string) bool { return s == "" }},
		{"single", map[string]int{"RISK_ON": 5}, "RISK_ON", func(s string) bool { return s == "RISK_ON" }},
		{"two-tied", map[string]int{"A": 1, "B": 1}, "", func(s string) bool { return s == "A" || s == "B" }},
		{"clear-mode", map[string]int{"RISK_OFF": 9, "RISK_ON": 3, "NEUTRAL": 2}, "RISK_OFF", func(s string) bool { return s == "RISK_OFF" }},
	}
	for _, tc := range cases {
		got := pickPredominant(tc.tally)
		if !tc.check(got) {
			t.Errorf("%s: got %q, want match in %v", tc.name, got, tc.tally)
		}
	}
}

// ------------------------------------------------------------------
// TestMeanStdDev — basic sanity stats.
// ------------------------------------------------------------------

func TestMeanStdDev(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	if got := meanFloat64(xs); mathAbs(got-3.0) > 1e-9 {
		t.Errorf("mean = %f, want 3.0", got)
	}
	if got := stdDevFloat64(xs, 3.0); mathAbs(got-1.5811388) > 1e-3 {
		t.Errorf("stddev = %f, want ~1.581", got)
	}
	if got := stdDevFloat64([]float64{42}, 42); got != 0 {
		t.Errorf("stddev singleton = %f, want 0", got)
	}
	if got := meanFloat64(nil); got != 0 {
		t.Errorf("mean empty = %f, want 0", got)
	}
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ------------------------------------------------------------------
// TestAggregateOutcomeSummary — stats correctness.
// ------------------------------------------------------------------

func TestAggregateOutcomeSummary(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sf := &sessionFile{
		Outcomes: []outcomeLite{
			{ForwardReturn: 0.05, HasForwardRet: true, Hit: true, Conviction: 80, Side: "BUY", Regime: "RISK_ON"},
			{ForwardReturn: -0.02, HasForwardRet: true, Hit: false, Conviction: 60, Side: "BUY", Regime: "RISK_ON"},
			{ForwardReturn: 0.01, HasForwardRet: true, Hit: true, Conviction: 50, Side: "SELL", Regime: "RISK_OFF"},
			{HasForwardRet: false, Hit: true, Conviction: 30, Side: "BUY"}, // missing forward_return
		},
	}
	rec := aggregateOutcomeSummary("2026-06-01", sf)
	stampDefaults(rec, now)

	if rec.TotalOutcomes != 4 {
		t.Errorf("TotalOutcomes = %d, want 4", rec.TotalOutcomes)
	}
	if rec.HitOutcomesCount != 3 {
		t.Errorf("HitOutcomesCount = %d, want 3", rec.HitOutcomesCount)
	}
	if mathAbs(rec.WinRate-0.75) > 1e-9 {
		t.Errorf("WinRate = %f, want 0.75", rec.WinRate)
	}
	if rec.BullishOutcomes != 2 {
		t.Errorf("BullishOutcomes = %d, want 2 (rows 1 + 4 both BUY+hit=true)", rec.BullishOutcomes)
	}
	if rec.BearishOutcomes != 1 {
		t.Errorf("BearishOutcomes = %d, want 1 (SELL+hit=true row)", rec.BearishOutcomes)
	}
	if rec.PredominantRegime != "RISK_ON" {
		t.Errorf("PredominantRegime = %q, want RISK_ON", rec.PredominantRegime)
	}
	if rec.CapturedAt.IsZero() {
		t.Errorf("CapturedAt not stamped")
	}
	if rec.IsSynthetic != 1 {
		t.Errorf("IsSynthetic = %d, want 1", rec.IsSynthetic)
	}
}

// ------------------------------------------------------------------
// TestRunEmpty — Run on an empty source dir must complete with 0 rows
// and 4 empty output files.
// ------------------------------------------------------------------

func TestRunEmpty(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()

	stats, err := Run(RunOptions{
		Source:       src,
		StagingDir:   out,
		LookbackDays: 90,
		Now:          time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SessionsInWindow != 0 || stats.MacroFilesInWindow != 0 {
		t.Errorf("expected 0 scans, got sessions=%d macro=%d",
			stats.SessionsInWindow, stats.MacroFilesInWindow)
	}
	for _, name := range []string{
		RegimeHistoryFile, EventCalendarHistoryFile,
		StressIndexHistoryFile, PredictionActualFile,
	} {
		p := filepath.Join(out, name)
		st, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected output file %s, got %v", name, err)
			continue
		}
		if st.Size() != 0 {
			t.Errorf("expected empty %s, got size %d", name, st.Size())
		}
	}
}

// ------------------------------------------------------------------
// TestRunWithSampleData — fabricate a tiny data/state, run, golden-check.
// ------------------------------------------------------------------

func TestRunWithSampleData(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	writeSession(
		t, src, "session-20260515-daily",
		`{"session_id":"session-20260515-daily","regime":"RISK_ON","recorded_at":"2026-05-15T00:00:00Z"}`,
		`{"forward_return":0.05,"hit":true,"conviction":70,"side":"BUY","supporting_events":["evt-tech-peak-100"],"regime":"RISK_ON"}`+"\n"+
			`{"forward_return":-0.01,"hit":false,"conviction":60,"side":"BUY","supporting_events":["evt-dividend-200"],"regime":"RISK_ON"}`+"\n",
	)
	writeSession(
		t, src, "session-20260516-daily",
		`{"session_id":"session-20260516-daily","regime":"RISK_OFF","recorded_at":"2026-05-16T00:00:00Z"}`,
		`{"forward_return":0.02,"hit":true,"conviction":55,"side":"SELL","supporting_events":["evt-election-300"],"regime":"RISK_OFF"}`+"\n",
	)
	// Out-of-window session: should NOT be in output.
	writeSession(
		t, src, "session-20260101-daily",
		`{"session_id":"session-20260101-daily","regime":"NEUTRAL","recorded_at":"2026-01-01T00:00:00Z"}`,
		`{"forward_return":0.99,"hit":true,"conviction":100,"side":"BUY","supporting_events":["evt-old"],"regime":"NEUTRAL"}`+"\n",
	)

	mustMkdir(t, filepath.Join(src, "macro"))
	writeMacroFile(t, filepath.Join(src, "macro", "2026-05-15.json"), map[string]any{
		"vix":          18.0,
		"stress_index": map[string]any{"score": 0.42, "regime": "medium", "components": map[string]any{"us": 0.5}},
	})

	stats, err := Run(RunOptions{
		Source:       src,
		StagingDir:   out,
		LookbackDays: 90,
		Now:          now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.SessionsInWindow != 2 {
		t.Errorf("SessionsInWindow = %d, want 2 (May 15 + 16)", stats.SessionsInWindow)
	}
	if stats.MacroFilesInWindow != 1 {
		t.Errorf("MacroFilesInWindow = %d, want 1", stats.MacroFilesInWindow)
	}
	if stats.RegimeRowsWritten != 2 {
		t.Errorf("RegimeRowsWritten = %d, want 2", stats.RegimeRowsWritten)
	}
	if stats.EventCalendarRowsWritten != 2 {
		t.Errorf("EventCalendarRowsWritten = %d, want 2", stats.EventCalendarRowsWritten)
	}
	if stats.StressIndexRowsWritten != 1 {
		t.Errorf("StressIndexRowsWritten = %d, want 1", stats.StressIndexRowsWritten)
	}
	if stats.PredictionActualRowsWritten != 2 {
		t.Errorf("PredictionActualRowsWritten = %d, want 2", stats.PredictionActualRowsWritten)
	}

	// Spot-check regime file.
	regime := readJSONLFile(t, filepath.Join(out, RegimeHistoryFile))
	if len(regime) != 2 {
		t.Fatalf("regime file lines = %d, want 2", len(regime))
	}
	var first RegimeRecord
	if err := json.Unmarshal(regime[0], &first); err != nil {
		t.Fatalf("unmarshal first regime row: %v", err)
	}
	if first.Date != "2026-05-15" || first.Regime != "RISK_ON" {
		t.Errorf("first regime row = %+v, want date=2026-05-15 regime=RISK_ON", first)
	}
	if first.IsSynthetic != 1 || first.CapturedAt.IsZero() {
		t.Errorf("first regime row missing stamps: %+v", first)
	}

	// Spot-check event calendar file.
	events := readJSONLFile(t, filepath.Join(out, EventCalendarHistoryFile))
	if len(events) != 2 {
		t.Fatalf("events file lines = %d, want 2", len(events))
	}
	var eRow EventCalendarRecord
	if err := json.Unmarshal(events[0], &eRow); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	if eRow.Source != "session-derive" {
		t.Errorf("events source = %q, want session-derive", eRow.Source)
	}
	if !reflect.DeepEqual(eRow.EventIDs, []string{"evt-tech-peak-100", "evt-dividend-200"}) &&
		!reflect.DeepEqual(eRow.EventIDs, []string{"evt-dividend-200", "evt-tech-peak-100"}) {
		// Map iteration order — accept either.
		t.Errorf("events ids = %v, want both evt-tech-peak-100 and evt-dividend-200", eRow.EventIDs)
	}
	if !containsString(eRow.ActiveThemes, "tech-peak") || !containsString(eRow.ActiveThemes, "dividend") {
		t.Errorf("expected themes tech-peak and dividend, got %v", eRow.ActiveThemes)
	}

	// Spot-check prediction actual.
	actual := readJSONLFile(t, filepath.Join(out, PredictionActualFile))
	if len(actual) != 2 {
		t.Fatalf("prediction actual lines = %d, want 2", len(actual))
	}
	var pa PredictionActualRecord
	if err := json.Unmarshal(actual[0], &pa); err != nil {
		t.Fatalf("unmarshal pa: %v", err)
	}
	if pa.TotalOutcomes != 2 {
		t.Errorf("pa[0] TotalOutcomes = %d, want 2", pa.TotalOutcomes)
	}
	if pa.HitOutcomesCount != 1 {
		t.Errorf("pa[0] HitOutcomesCount = %d, want 1", pa.HitOutcomesCount)
	}
	if pa.PredominantRegime != "RISK_ON" {
		t.Errorf("pa[0] PredominantRegime = %q, want RISK_ON", pa.PredominantRegime)
	}

	// Spot-check stress index.
	si := readJSONLFile(t, filepath.Join(out, StressIndexHistoryFile))
	if len(si) != 1 {
		t.Fatalf("stress index lines = %d, want 1", len(si))
	}
	var sr StressIndexRecord
	if err := json.Unmarshal(si[0], &sr); err != nil {
		t.Fatalf("unmarshal si: %v", err)
	}
	if mathAbs(sr.Score-0.42) > 1e-9 {
		t.Errorf("si[0] Score = %f, want 0.42", sr.Score)
	}
	if sr.Regime != "medium" || sr.Source != "macro-file" {
		t.Errorf("si[0]=%+v, want regime=medium source=macro-file", sr)
	}
}

// ------------------------------------------------------------------
// TestRunDryRun — dry-run mode must not create files.
// ------------------------------------------------------------------

func TestRunDryRun(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeSession(
		t, src, "session-20260515-daily",
		`{"session_id":"session-20260515-daily","regime":"RISK_ON","recorded_at":"2026-05-15T00:00:00Z"}`,
		`{"forward_return":0.05,"hit":true,"conviction":70,"side":"BUY","supporting_events":["evt-x"],"regime":"RISK_ON"}`+"\n",
	)

	stats, err := Run(RunOptions{
		Source:       src,
		StagingDir:   out,
		LookbackDays: 90,
		Now:          time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("Run dry-run: %v", err)
	}
	if stats.SessionsInWindow != 1 {
		t.Errorf("expected 1 session scanned in dry-run, got %d", stats.SessionsInWindow)
	}
	// Files should not exist in dry-run.
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		t.Errorf("dry-run should not create %s", e.Name())
	}
}

// ------------------------------------------------------------------
// TestRunHandlesMalformedJSONL — even one malformed line must not break
// the whole extractor (we skip and continue).
// ------------------------------------------------------------------

func TestRunHandlesMalformedJSONL(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeSession(
		t, src, "session-20260515-daily",
		`{"session_id":"session-20260515-daily","regime":"RISK_ON","recorded_at":"2026-05-15T00:00:00Z"}`,
		`THIS IS NOT JSON`+"\n"+
			`{"forward_return":0.05,"hit":true,"conviction":70,"side":"BUY","supporting_events":["evt-good"],"regime":"RISK_ON"}`+"\n",
	)
	stats, err := Run(RunOptions{
		Source:       src,
		StagingDir:   out,
		LookbackDays: 90,
		Now:          time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SessionsInWindow != 1 {
		t.Fatalf("SessionsInWindow = %d, want 1", stats.SessionsInWindow)
	}
	// 1 valid row → 1 prediction_actual row.
	if stats.PredictionActualRowsWritten != 1 {
		t.Errorf("PredictionActualRowsWritten = %d, want 1", stats.PredictionActualRowsWritten)
	}
}

// ------------------------------------------------------------------
// TestResolveStagingFiles — empty string rejected.
// ------------------------------------------------------------------

func TestResolveStagingFiles(t *testing.T) {
	if _, err := ResolveStagingFiles(""); err == nil {
		t.Fatal("expected error for empty staging dir")
	}
	files, err := ResolveStagingFiles("/tmp/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(files.RegimeHistory, RegimeHistoryFile) {
		t.Errorf("RegimeHistory path = %q, want suffix %s", files.RegimeHistory, RegimeHistoryFile)
	}
	if !strings.HasSuffix(files.PredictionActual, PredictionActualFile) {
		t.Errorf("PredictionActual path = %q, want suffix %s", files.PredictionActual, PredictionActualFile)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func writeSession(t *testing.T, base, id, summary, outcomes string) {
	t.Helper()
	dir := filepath.Join(base, "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if outcomes != "" {
		if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(outcomes), 0o644); err != nil {
			t.Fatalf("write outcomes: %v", err)
		}
	}
}

func writeMacroFile(t *testing.T, path string, body any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal macro: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write macro: %v", err)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func readJSONLFile(t *testing.T, path string) [][]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	out := make([][]byte, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		out = append(out, []byte(l))
	}
	return out
}

func containsString(xs []string, want string) bool {
	return slices.Contains(xs, want)
}
