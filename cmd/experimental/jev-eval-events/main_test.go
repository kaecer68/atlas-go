package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// TestFirstSessionAtOrAfter_RollsForward pins the anchor policy: the anchor must
// be a session the price universe actually contains, because the forward return
// is measured from that session's close.
func TestFirstSessionAtOrAfter_RollsForward(t *testing.T) {
	sessions := []string{"2021-06-10", "2021-06-11", "2021-06-15", "2021-06-16"}

	got, shift, ok := firstSessionAtOrAfter(sessions, "2021-06-11")
	if !ok || got != "2021-06-11" || shift != 0 {
		t.Fatalf("exact session: got %q shift %d ok %v", got, shift, ok)
	}
	// 2021-06-12 is a Saturday and 06-14 a holiday: the anchor must land on the
	// first session at or after the nominal date and report how far it moved.
	got, shift, ok = firstSessionAtOrAfter(sessions, "2021-06-12")
	if !ok || got != "2021-06-15" || shift != 3 {
		t.Fatalf("weekend roll: got %q shift %d ok %v", got, shift, ok)
	}
	if _, _, ok := firstSessionAtOrAfter(sessions, "2021-07-01"); ok {
		t.Fatal("nominal date past the universe must report ok=false")
	}
}

// TestNominalAnchor_PicksTheRequestedDate keeps start/peak/end from silently
// collapsing onto the same date.
func TestNominalAnchor_PicksTheRequestedDate(t *testing.T) {
	evt := industry.CalendarEvent{
		StartDate: time.Date(2021, 5, 28, 0, 0, 0, 0, time.UTC),
		PeakDate:  time.Date(2021, 5, 31, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2021, 6, 3, 0, 0, 0, 0, time.UTC),
	}
	cases := map[string]string{
		anchorStart: "2021-05-28",
		anchorPeak:  "2021-05-31",
		anchorEnd:   "2021-06-03",
	}
	for kind, want := range cases {
		if got := nominalAnchor(evt, kind).Format(dateLayout); got != want {
			t.Fatalf("nominalAnchor(%s) = %s, want %s", kind, got, want)
		}
	}
}

// TestSessionOffsets_AreWindowRelative covers the two window-offset helpers the
// JSONL exposes as state facts.
func TestSessionOffsets_AreWindowRelative(t *testing.T) {
	sessions := []string{"2021-05-28", "2021-05-31", "2021-06-01", "2021-06-02", "2021-06-03", "2021-06-04"}
	index := map[string]int{}
	for i, d := range sessions {
		index[d] = i
	}
	if got := sessionsFromWindowStart(sessions, index, "2021-06-02", "2021-05-28"); got != 3 {
		t.Fatalf("sessionsFromWindowStart = %d, want 3", got)
	}
	if got := sessionsToWindowEnd(sessions, index, "2021-06-02", "2021-06-04"); got != 2 {
		t.Fatalf("sessionsToWindowEnd = %d, want 2", got)
	}
	if got := sessionsToWindowEnd(sessions, index, "2021-06-04", "2021-06-01"); got != 0 {
		t.Fatalf("closed window: sessionsToWindowEnd = %d, want 0", got)
	}
}

// TestRun_ExportsDeterministicSkeleton exercises the real CLI path on a fixture
// universe, and proves the export is byte-reproducible from the same inputs.
func TestRun_ExportsDeterministicSkeleton(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "sector_index")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessions := juneWeekdays(t, dataDir)
	if len(sessions) < 10 {
		t.Fatalf("fixture built only %d sessions", len(sessions))
	}

	outA := filepath.Join(dir, "a.jsonl")
	outB := filepath.Join(dir, "b.jsonl")
	summary := filepath.Join(dir, "summary.json")
	adj := filepath.Join(dir, "adjustments.jsonl")
	args := func(out string) []string {
		return []string{
			"-dir", dataDir, "-start", "2021-06-01", "-end", "2021-06-30",
			"-anchor", anchorPeak, "-out", out, "-summary-out", summary,
			"-adjustments-out", adj, "-quiet",
		}
	}
	if err := run(args(outA), os.Stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := run(args(outB), os.Stdout); err != nil {
		t.Fatalf("second run: %v", err)
	}
	a, err := os.ReadFile(outA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outB)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("export is not byte-reproducible")
	}

	rows := decodeRows(t, outA)
	if len(rows) == 0 {
		t.Fatal("empty export")
	}
	inUniverse := map[string]bool{}
	for _, d := range sessions {
		inUniverse[d] = true
	}
	previous := ""
	for _, r := range rows {
		if !inUniverse[r.AnchorDate] {
			t.Fatalf("anchor %s is not a session of the universe", r.AnchorDate)
		}
		if r.AnchorDate < "2021-06-01" || r.AnchorDate > "2021-06-30" {
			t.Fatalf("anchor %s escaped the window", r.AnchorDate)
		}
		if r.AnchorDate < previous {
			t.Fatalf("rows are not sorted by anchor date: %s after %s", r.AnchorDate, previous)
		}
		previous = r.AnchorDate
		if r.NominalAnchorDate == "" || r.EventID == "" {
			t.Fatalf("row is missing its provenance: %+v", r)
		}
	}

	// The platform-adjustment artifact covers every in-window session for every
	// industry the price universe actually carries.
	adjRows := decodeAdjustments(t, adj)
	if len(adjRows) != len(sessions)*len(canonicalTestIndustries) {
		t.Fatalf("adjustment rows = %d, want %d", len(adjRows), len(sessions)*len(canonicalTestIndustries))
	}
	for _, r := range adjRows {
		if r.Date < "2021-06-01" || r.Date > "2021-06-30" {
			t.Fatalf("adjustment row escaped the window: %s", r.Date)
		}
	}
}

// TestRun_RejectsUnknownAnchor keeps the flag contract explicit.
func TestRun_RejectsUnknownAnchor(t *testing.T) {
	err := run([]string{"-start", "2021-06-01", "-end", "2021-06-30", "-anchor", "middle", "-out", filepath.Join(t.TempDir(), "o.jsonl")}, os.Stdout)
	if err == nil {
		t.Fatal("expected an error for an unknown -anchor")
	}
}

// juneWeekdays writes a one-row-per-industry sector_index fixture for every
// weekday of June 2021 and returns those dates.
func juneWeekdays(t *testing.T, dir string) []string {
	t.Helper()
	var dates []string
	for day := 1; day <= 30; day++ {
		d := time.Date(2021, time.June, day, 0, 0, 0, 0, time.UTC)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		date := d.Format(dateLayout)
		dates = append(dates, date)
		var body strings.Builder
		body.WriteString("{")
		for k, id := range canonicalTestIndustries {
			if k > 0 {
				body.WriteString(",")
			}
			fmt.Fprintf(&body,
				`%q:[{"date":%q,"industry":%q,"index":100.0,"return_pct":0.25}]`, id, date, id)
		}
		body.WriteString("}")
		name := "sector_indices_" + strings.ReplaceAll(date, "-", "") + "_" + strings.ReplaceAll(date, "-", "") + ".json"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dates
}

func decodeRows(t *testing.T, path string) []EventRow {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []EventRow
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r EventRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	return rows
}

func decodeAdjustments(t *testing.T, path string) []AdjustmentRow {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []AdjustmentRow
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r AdjustmentRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	return rows
}

var canonicalTestIndustries = []string{
	"auto", "biotech", "cement", "construction", "electronics", "energy",
	"financials", "food", "machinery", "optoelectronics", "other_electronics",
	"plastics", "retail", "semiconductor", "shipping", "steel", "telecom", "textiles",
}
