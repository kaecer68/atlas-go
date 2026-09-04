package capitalflow

// Tests for the shared validation runner (RunHypothesisValidation) and
// the verdict-change detection used by the scheduled
// cf_hypothesis_validation task.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateValidationDateArg(t *testing.T) {
	if err := ValidateValidationDateArg("2026-09-05"); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
	if err := ValidateValidationDateArg(""); err != nil {
		t.Fatalf("empty allowed: %v", err)
	}
	if err := ValidateValidationDateArg("2026-13-99"); err == nil {
		t.Fatalf("invalid date accepted")
	}
}

// TestRunHypothesisValidationMissingCalendar pins that a missing
// trading calendar is a hard error (unchanged CLI contract): without a
// calendar no verdict can be replayed, and the scheduled task should
// surface this as an operational failure rather than silently reporting.
func TestRunHypothesisValidationMissingCalendar(t *testing.T) {
	_, err := RunHypothesisValidation(context.Background(), ValidationInputs{WorkDir: t.TempDir()})
	if err == nil {
		t.Fatalf("missing replay calendar must be a hard error")
	}
	if !strings.Contains(err.Error(), "load trading calendar") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunHypothesisValidationCalendarOnly pins the failure-tolerance
// contract: a workdir with a calendar but no data files at all is NOT
// an error — every hypothesis reports INSUFFICIENT_DATA, which is the
// honest outcome and exactly what the scheduled rerun sees on a
// data-not-yet-backfilled day. (Missing OI/T86/macro/rolling dirs are
// tolerated; only the calendar is mandatory.)
func TestRunHypothesisValidationCalendarOnly(t *testing.T) {
	workdir := t.TempDir()
	replayDir := filepath.Join(workdir, "data", "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	calendar := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n"
	for _, d := range []string{"2026-06-01", "2026-06-02", "2026-06-03"} {
		calendar += d + ",0050,0050,1,1,1,1,1\n"
	}
	if err := os.WriteFile(filepath.Join(replayDir, ValidationDefaultReplayPath[len("data/replay/"):]), []byte(calendar), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := RunHypothesisValidation(context.Background(), ValidationInputs{WorkDir: workdir})
	if err != nil {
		t.Fatalf("calendar-only workdir must not error: %v", err)
	}
	if len(report.Hypotheses) != 3 {
		t.Fatalf("expected 3 hypotheses, got %d", len(report.Hypotheses))
	}
	for _, h := range report.Hypotheses {
		if h.Status != ValidationInsufficientData {
			t.Fatalf("%s status = %s, want INSUFFICIENT_DATA on empty data", h.ID, h.Status)
		}
	}
	if report.DataCoverage["calendar_days"] != 3 {
		t.Fatalf("calendar_days = %d, want 3", report.DataCoverage["calendar_days"])
	}
	if report.EligibleRecommendation {
		t.Fatalf("empty run must never recommend eligibility")
	}
}

func writeTestOI(t *testing.T, dir, date string, oiNet int64) {
	t.Helper()
	body := `{"date":"` + date + `","contracts":{"TX":{"foreign":{"oi_net":` + itoa(oiNet) + `}}}}`
	if err := os.WriteFile(filepath.Join(dir, date+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestLoadValidationFuturesOI(t *testing.T) {
	dir := t.TempDir()
	writeTestOI(t, dir, "2026-06-01", -10220)
	if err := os.WriteFile(filepath.Join(dir, "2026-06-02.json"), []byte(`{"date":"2026-06-02","contracts":{"MTX":{"foreign":{"oi_net":999}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-date.json"), []byte(`{"contracts":{"TX":{"foreign":{"oi_net":1}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	oi, err := LoadValidationFuturesOI(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(oi) != 1 {
		t.Fatalf("expected 1 TX day, got %v", oi)
	}
	if oi["2026-06-01"] != -10220 {
		t.Fatalf("oi = %v, want -10220", oi)
	}
}

func TestLoadValidationFuturesOIMissingDir(t *testing.T) {
	oi, err := LoadValidationFuturesOI(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing dir should be empty, not error: %v", err)
	}
	if len(oi) != 0 {
		t.Fatalf("expected empty map, got %v", oi)
	}
}

func TestLoadValidationMacroSnapshots(t *testing.T) {
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

	macro, err := LoadValidationMacroSnapshots(dir)
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

// ---------------------------------------------------------------------------
// DetectVerdictChanges
// ---------------------------------------------------------------------------

func hr(id, status string, n int) HypothesisResult {
	return HypothesisResult{ID: id, Status: status, SampleCount: n}
}

func TestDetectVerdictChanges_NoChangeQuiet(t *testing.T) {
	prev := []HypothesisResult{
		hr("H-CF-01", ValidationInsufficientData, 100),
		hr("H-CF-02", ValidationInsufficientData, 80),
		hr("H-CF-05", ValidationInsufficientData, 200),
	}
	cur := append([]HypothesisResult(nil), prev...)
	if changes := DetectVerdictChanges(prev, cur); len(changes) != 0 {
		t.Fatalf("identical reports must stay quiet, got %v", changes)
	}
	// First run: no previous report → no change.
	if changes := DetectVerdictChanges(nil, cur); len(changes) != 0 {
		t.Fatalf("first run must stay quiet, got %v", changes)
	}
}

func TestDetectVerdictChanges_DataUnlock(t *testing.T) {
	prev := []HypothesisResult{
		hr("H-CF-01", ValidationInsufficientData, 100),
		hr("H-CF-02", ValidationInsufficientData, 80),
		hr("H-CF-05", ValidationInsufficientData, 200),
	}
	cur := []HypothesisResult{
		hr("H-CF-01", ValidationFail, 260),
		hr("H-CF-02", ValidationInsufficientData, 120), // not yet unlocked
		hr("H-CF-05", ValidationPass, 252),
	}
	changes := DetectVerdictChanges(prev, cur)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %v", changes)
	}
	byID := map[string]VerdictChange{}
	for _, c := range changes {
		byID[c.HypothesisID] = c
	}
	c1, ok := byID["H-CF-01"]
	if !ok || c1.Kind != VerdictChangeDataUnlock || c1.FromStatus != ValidationInsufficientData || c1.ToStatus != ValidationFail || c1.SampleCount != 260 {
		t.Fatalf("H-CF-01 unlock = %+v", c1)
	}
	c5, ok := byID["H-CF-05"]
	if !ok || c5.Kind != VerdictChangeDataUnlock || c5.ToStatus != ValidationPass {
		t.Fatalf("H-CF-05 unlock = %+v", c5)
	}
	if _, ok := byID["H-CF-02"]; ok {
		t.Fatalf("H-CF-02 stayed INSUFFICIENT_DATA — must not appear")
	}
}

func TestDetectVerdictChanges_VerdictFlip(t *testing.T) {
	prev := []HypothesisResult{
		hr("H-CF-01", ValidationPass, 300),
		hr("H-CF-05", ValidationPassImproved, 300),
	}
	cur := []HypothesisResult{
		hr("H-CF-01", ValidationFail, 320), // judged → judged flip
		hr("H-CF-05", ValidationInsufficientData, 0),
	}
	changes := DetectVerdictChanges(prev, cur)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %v", changes)
	}
	byID := map[string]VerdictChange{}
	for _, c := range changes {
		byID[c.HypothesisID] = c
	}
	if c := byID["H-CF-01"]; c.Kind != VerdictChangeFlip {
		t.Fatalf("PASS→FAIL must be verdict_flip, got %s", c.Kind)
	}
	if c := byID["H-CF-05"]; c.Kind != VerdictChangeFlip {
		t.Fatalf("judged→INSUFFICIENT_DATA is a flip, got %s", c.Kind)
	}
}

func TestDetectVerdictChanges_NewHypothesisID(t *testing.T) {
	prev := []HypothesisResult{hr("H-CF-01", ValidationPass, 300)}
	cur := []HypothesisResult{hr("H-CF-01", ValidationPass, 300), hr("H-CF-99", ValidationFail, 260)}
	if changes := DetectVerdictChanges(prev, cur); len(changes) != 0 {
		t.Fatalf("hypothesis absent from prev must not count as a change, got %v", changes)
	}
}

// ---------------------------------------------------------------------------
// FindLatestValidationReport
// ---------------------------------------------------------------------------

func writeReportFile(t *testing.T, dir, name, status string) {
	t.Helper()
	report := BuildValidationReport("", nil, []HypothesisResult{hr("H-CF-01", status, 260)})
	report.RunAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Filename date is what FindLatestValidationReport orders by.
	if err := WriteValidationReportJSON(filepath.Join(dir, name), report); err != nil {
		t.Fatal(err)
	}
}

func TestFindLatestValidationReport(t *testing.T) {
	dir := t.TempDir()
	writeReportFile(t, dir, "cf-hypotheses-2026-09-01.json", ValidationInsufficientData)
	writeReportFile(t, dir, "cf-hypotheses-2026-09-03.json", ValidationFail)
	// Today's own output must be excluded by beforeDate.
	writeReportFile(t, dir, "cf-hypotheses-2026-09-05.json", ValidationPass)
	// Noise: wrong prefix / non-dated / non-JSON.
	if err := os.WriteFile(filepath.Join(dir, "other-2026-09-04.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cf-hypotheses-notadate.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, path, found, err := FindLatestValidationReport(dir, "2026-09-05")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected to find the 2026-09-03 report")
	}
	if !strings.HasSuffix(path, "cf-hypotheses-2026-09-03.json") {
		t.Fatalf("path = %s, want the 09-03 report", path)
	}
	if report.Hypotheses[0].Status != ValidationFail {
		t.Fatalf("status = %s, want FAIL", report.Hypotheses[0].Status)
	}

	// No earlier report (first run) → quiet.
	if _, _, found, err := FindLatestValidationReport(dir, "2026-09-01"); err != nil || found {
		t.Fatalf("beforeDate before all files: found=%v err=%v", found, err)
	}
}

func TestFindLatestValidationReportMissingDir(t *testing.T) {
	_, _, found, err := FindLatestValidationReport(filepath.Join(t.TempDir(), "nope"), "2026-09-05")
	if err != nil || found {
		t.Fatalf("missing dir must be quiet: found=%v err=%v", found, err)
	}
}
