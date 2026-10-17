//go:build integration

package marketdata

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestSectorIndexReader_ReadRange_RuntimeShapedDataset runs the reader over a
// dataset whose *shape* mirrors the real runtime directory
// (data/state/sector_index/**): an 18-industry batch file written by
// TWSESectorIndexProvider, 8-industry legacy single-day files from the pre-W1
// writer, a day that only the legacy writer covered, and unrelated or corrupt
// leftovers that a long-lived runtime directory accumulates.
//
// Why the fixture is synthetic (FU-20261016-17)
// --------------------------------------------
// The predecessor of this test read the real repo-root data/state/sector_index
// and skipped whenever it was absent. Two things made that a non-test:
//
//   - `data/state/**` is gitignored, so a clean checkout (CI, or any fresh
//     worktree) never has it. The integration job therefore recorded
//     "--- SKIP" on every run - a green suite that asserted nothing.
//   - With the data present it *failed*: the assertion pinned a specific
//     historical day ("2026-06-24 is present after the backfill"). The runtime
//     directory is mutable - the referenced day is no longer in the dataset on
//     the main checkout (verified 2026-10-16: no row for 2026-06-24 anywhere
//     under data/state/sector_index) - so the test was unrunnable in CI and
//     wrong locally at the same time.
//
// A synthetic fixture keeps every assertion and removes the dependency on
// mutable runtime data. Nothing here reads the repository: the dataset lives in
// t.TempDir(), so the test behaves identically in CI, in a lane worktree, and
// on the main checkout.
//
// Assertions (each has teeth - they are exact values, not presence checks):
//  1. Values for every native-covered date equal the fixture spec, for all 18
//     industries.
//  2. Source priority: the 18-industry value wins over the 8-industry value on
//     a date both files cover.
//  3. A date covered only by a legacy single-day file is filled from it, and
//     the legacy IDs that collapse onto one canonical sector (ai_supply_chain
//     and electronics -> electronics) are averaged, not overwritten.
//  4. The window filter is inclusive of both endpoints and drops rows outside.
//  5. A day that the backfill touched is present with a value for every
//     industry, and a day no file covers stays absent (missing != 0).
//  6. Corrupt / foreign-schema files and files outside the naming contract are
//     ignored instead of failing or leaking rows.
func TestSectorIndexReader_ReadRange_RuntimeShapedDataset(t *testing.T) {
	dir := t.TempDir()
	writeRuntimeShapedDataset(t, dir)

	start, err := time.Parse(dateLayoutISO, "2026-06-03")
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	end, err := time.Parse(dateLayoutISO, "2026-07-10")
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}

	reader := NewSectorIndexReader(dir)
	data, err := reader.ReadRange(start, end)
	if err != nil {
		t.Fatalf("ReadRange(%s..%s) error = %v", start, end, err)
	}

	// 4. Window filter: boundaries included, outside rows dropped.
	for _, want := range []string{"2026-06-03", "2026-06-24", "2026-07-01", "2026-07-02", "2026-07-10"} {
		if _, ok := data[want]; !ok {
			t.Errorf("ReadRange result has no entry for %s (window %s..%s)", want, start.Format(dateLayoutISO), end.Format(dateLayoutISO))
		}
	}
	for _, unwanted := range []string{"2026-05-29", "2026-07-13"} {
		if _, ok := data[unwanted]; ok {
			t.Errorf("ReadRange returned %s, which is outside the requested window", unwanted)
		}
	}

	// 6. A file outside the naming contract must not leak rows even though its
	// payload is well formed and inside the window.
	if _, ok := data["2026-06-12"]; ok {
		t.Errorf("ReadRange returned 2026-06-12 from sector_data.json, which does not match sector_indices_*.json")
	}

	// 1 + 2. Native (18-industry) dates: exact values, native wins.
	for _, date := range []string{"2026-06-03", "2026-06-24", "2026-07-01", "2026-07-10"} {
		returns, ok := data[date]
		if !ok {
			continue // already reported above
		}
		if len(returns) != len(runtimeShapedSectorIDs) {
			t.Errorf("%s: got %d industries, want %d", date, len(returns), len(runtimeShapedSectorIDs))
		}
		for ordinal, industry := range runtimeShapedSectorIDs {
			got, ok := returns[industry]
			if !ok {
				t.Errorf("%s: missing industry %q", date, industry)
				continue
			}
			want := runtimeShapedReturn(date, ordinal)
			if got != want {
				t.Errorf("%s %s: return = %v, want %v", date, industry, got, want)
			}
		}
	}

	// 2 (explicit): the legacy file for 2026-06-03 carries different numbers for
	// the sectors it shares with the native file; the native ones must win.
	legacyElectronics := runtimeShapedLegacyReturn("2026-06-03", 1)
	if legacyElectronics == runtimeShapedReturn("2026-06-03", runtimeShapedOrdinal(t, "electronics")) {
		t.Fatalf("fixture is broken: legacy and native electronics values for 2026-06-03 are equal, so the priority assertion cannot discriminate")
	}
	if got, want := data["2026-06-03"]["electronics"], runtimeShapedReturn("2026-06-03", runtimeShapedOrdinal(t, "electronics")); got != want {
		t.Errorf("2026-06-03 electronics = %v: the legacy file overrode the 18-industry file (native value is %v, legacy value is %v)", got, want, legacyElectronics)
	}

	// 3. Legacy-only date: filled from the 8-industry file, with same-sector
	// collisions averaged. ai_supply_chain resolves to electronics (canonical
	// L1), so electronics = mean(ai_supply_chain row, electronics row).
	legacyOnly, ok := data["2026-07-02"]
	if !ok {
		t.Fatal("2026-07-02 (covered only by a legacy single-day file) is missing from ReadRange")
	}
	wantLegacyOnly := map[string]float64{
		"electronics":       (runtimeShapedLegacyReturn("2026-07-02", 0) + runtimeShapedLegacyReturn("2026-07-02", 1)) / 2,
		"energy":            runtimeShapedLegacyReturn("2026-07-02", 2),
		"financials":        runtimeShapedLegacyReturn("2026-07-02", 3),
		"other_electronics": runtimeShapedLegacyReturn("2026-07-02", 4),
		"machinery":         runtimeShapedLegacyReturn("2026-07-02", 5), // robotics -> machinery
		"semiconductor":     runtimeShapedLegacyReturn("2026-07-02", 6),
		"shipping":          runtimeShapedLegacyReturn("2026-07-02", 7),
	}
	if len(legacyOnly) != len(wantLegacyOnly) {
		t.Errorf("2026-07-02: got %d sectors %v, want %d", len(legacyOnly), sortedKeys(legacyOnly), len(wantLegacyOnly))
	}
	for industry, want := range wantLegacyOnly {
		got, ok := legacyOnly[industry]
		if !ok {
			t.Errorf("2026-07-02: missing sector %q (legacy file covers it)", industry)
			continue
		}
		if got != want {
			t.Errorf("2026-07-02 %s: return = %v, want %v", industry, got, want)
		}
	}

	// 5. A day covered by no file stays absent: the reader must not zero-fill.
	if _, ok := data["2026-06-05"]; ok {
		t.Error("2026-06-05 is covered by no fixture file, but ReadRange returned an entry for it")
	}

	// 1 (global). No value may be NaN/Inf: downstream period detection divides
	// by these. JSON cannot encode NaN, so this guards the aggregation path.
	for date, returns := range data {
		for industry, v := range returns {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("%s %s: return = %v, want finite", date, industry, v)
			}
		}
	}

	// Determinism: file iteration order must not change the result.
	again, err := reader.ReadRange(start, end)
	if err != nil {
		t.Fatalf("second ReadRange error = %v", err)
	}
	if !sameReturns(data, again) {
		t.Error("ReadRange is not deterministic: two identical calls returned different results")
	}
}

const dateLayoutISO = "2006-01-02"

// runtimeShapedSectorIDs is the 18-industry vocabulary carried by the real
// native batch files: the 20 canonical L1 sectors minus chemicals and tourism,
// which the upstream feeds do not publish (see NamespaceFinMindSectorSeries in
// internal/sectormap).
var runtimeShapedSectorIDs = []string{
	"auto", "biotech", "cement", "construction", "electronics", "energy",
	"financials", "food", "machinery", "optoelectronics", "other_electronics",
	"plastics", "retail", "semiconductor", "shipping", "steel", "telecom",
	"textiles",
}

// runtimeShapedLegacySectorIDs is the 8-industry vocabulary of the pre-W1
// writer (internal/sectormap twseSectorIndexLegacyKeys). ai_supply_chain and
// robotics are canonical L2 IDs that resolve to electronics and machinery.
var runtimeShapedLegacySectorIDs = []string{
	"ai_supply_chain", "electronics", "energy", "financials",
	"other_electronics", "robotics", "semiconductor", "shipping",
}

// runtimeShapedReturn is the fixture spec for native-file values. It depends on
// both the date and the industry ordinal so that a wrong date/industry pairing
// cannot pass by accident.
func runtimeShapedReturn(date string, ordinal int) float64 {
	day, err := time.Parse(dateLayoutISO, date)
	if err != nil {
		panic("runtimeShapedReturn: bad date " + date)
	}
	return float64(ordinal+1)*0.5 + float64(day.Day())*0.25 - 10
}

// runtimeShapedLegacyReturn is the fixture spec for legacy-file values. It uses
// a different offset from runtimeShapedReturn so the source-priority assertion
// can tell the two writers' values apart.
func runtimeShapedLegacyReturn(date string, ordinal int) float64 {
	day, err := time.Parse(dateLayoutISO, date)
	if err != nil {
		panic("runtimeShapedLegacyReturn: bad date " + date)
	}
	return float64(ordinal+1)*0.75 - float64(day.Day())*0.5 + 3
}

func runtimeShapedOrdinal(t *testing.T, industry string) int {
	t.Helper()
	for i, id := range runtimeShapedSectorIDs {
		if id == industry {
			return i
		}
	}
	t.Fatalf("industry %q is not in runtimeShapedSectorIDs", industry)
	return -1
}

// writeRuntimeShapedDataset lays out the fixture directory:
//
//	sector_indices_20260601_20260710.json  18 industries, four in-window dates
//	                                       plus one before and one after
//	sector_indices_20260603_20260603.json 8-industry legacy file, same day
//	sector_indices_20260702_20260702.json 8-industry legacy file, own day
//	sector_indices_20260610_20260610.json corrupt JSON
//	sector_indices_20260611_20260611.json foreign schema
//	sector_data.json                      right payload, wrong file name
func writeRuntimeShapedDataset(t *testing.T, dir string) {
	t.Helper()

	nativeDates := []string{"2026-05-29", "2026-06-03", "2026-06-24", "2026-07-01", "2026-07-10", "2026-07-13"}
	native := make(map[string][]SectorIndexData, len(runtimeShapedSectorIDs))
	for ordinal, industry := range runtimeShapedSectorIDs {
		for _, date := range nativeDates {
			native[industry] = append(native[industry], SectorIndexData{
				Date:      date,
				Industry:  industry,
				Index:     1000 + float64(ordinal),
				ReturnPct: runtimeShapedReturn(date, ordinal),
			})
		}
	}
	writeSectorIndexFixture(t, dir, "sector_indices_20260601_20260710.json", native)

	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", legacyRows("2026-06-03"))
	writeSectorIndexFixture(t, dir, "sector_indices_20260702_20260702.json", legacyRows("2026-07-02"))

	if err := os.WriteFile(filepath.Join(dir, "sector_indices_20260610_20260610.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sector_indices_20260611_20260611.json"), []byte(`{"unexpected":"schema"}`), 0o644); err != nil {
		t.Fatalf("write foreign-schema fixture: %v", err)
	}
	leak := map[string][]SectorIndexData{
		"auto": {{Date: "2026-06-12", Industry: "auto", Index: 1, ReturnPct: 99}},
	}
	writeSectorIndexFixture(t, dir, "sector_data.json", leak)
}

// legacyRows builds one 8-industry legacy file body for a single date.
func legacyRows(date string) map[string][]SectorIndexData {
	rows := make(map[string][]SectorIndexData, len(runtimeShapedLegacySectorIDs))
	for ordinal, industry := range runtimeShapedLegacySectorIDs {
		rows[industry] = []SectorIndexData{{
			Date:      date,
			Industry:  industry,
			Index:     1000 + float64(ordinal),
			ReturnPct: runtimeShapedLegacyReturn(date, ordinal),
		}}
	}
	return rows
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameReturns(a, b map[string]map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for date, returns := range a {
		other, ok := b[date]
		if !ok || len(other) != len(returns) {
			return false
		}
		for industry, v := range returns {
			if other[industry] != v {
				return false
			}
		}
	}
	return true
}
