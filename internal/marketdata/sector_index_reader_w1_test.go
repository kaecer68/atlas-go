package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeSectorIndexJSON writes a single sector_index file with the given
// industries. Used by the W1 source-priority tests.
func writeSectorIndexJSON(t *testing.T, dir, filename string, data map[string][]SectorIndexData) {
	t.Helper()
	out, _ := json.Marshal(data)
	if err := os.WriteFile(filepath.Join(dir, filename), out, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSectorIndexReader_SourcePriority_18Wins verifies that when both an
// 18-industry native file and an 8-industry legacy file are present for the
// same (date, industry), the 18-industry value is preferred.
func TestSectorIndexReader_SourcePriority_18Wins(t *testing.T) {
	dir := t.TempDir()

	// 18-industry native file (7/1): electronics=3.15
	eighteen := map[string][]SectorIndexData{
		"electronics":       {{Date: "2026-07-01", Industry: "electronics", Index: 100, ReturnPct: 3.15}},
		"semiconductor":     {{Date: "2026-07-01", Industry: "semiconductor", Index: 100, ReturnPct: 1.79}},
		"financials":        {{Date: "2026-07-01", Industry: "financials", Index: 100, ReturnPct: 0.38}},
		"auto":              {{Date: "2026-07-01", Industry: "auto", Index: 100, ReturnPct: 1.0}},
		"biotech":           {{Date: "2026-07-01", Industry: "biotech", Index: 100, ReturnPct: 1.0}},
		"cement":            {{Date: "2026-07-01", Industry: "cement", Index: 100, ReturnPct: 1.0}},
		"construction":      {{Date: "2026-07-01", Industry: "construction", Index: 100, ReturnPct: 1.0}},
		"energy":            {{Date: "2026-07-01", Industry: "energy", Index: 100, ReturnPct: 1.0}},
		"food":              {{Date: "2026-07-01", Industry: "food", Index: 100, ReturnPct: 1.0}},
		"machinery":         {{Date: "2026-07-01", Industry: "machinery", Index: 100, ReturnPct: 1.0}},
		"optoelectronics":   {{Date: "2026-07-01", Industry: "optoelectronics", Index: 100, ReturnPct: 1.0}},
		"other_electronics": {{Date: "2026-07-01", Industry: "other_electronics", Index: 100, ReturnPct: 1.0}},
		"plastics":          {{Date: "2026-07-01", Industry: "plastics", Index: 100, ReturnPct: 1.0}},
		"retail":            {{Date: "2026-07-01", Industry: "retail", Index: 100, ReturnPct: 1.0}},
		"shipping":          {{Date: "2026-07-01", Industry: "shipping", Index: 100, ReturnPct: 1.0}},
		"steel":             {{Date: "2026-07-01", Industry: "steel", Index: 100, ReturnPct: 1.0}},
		"telecom":           {{Date: "2026-07-01", Industry: "telecom", Index: 100, ReturnPct: 1.0}},
		"textiles":          {{Date: "2026-07-01", Industry: "textiles", Index: 100, ReturnPct: 1.0}},
	}
	writeSectorIndexJSON(t, dir, "sector_indices_20260701_20260710.json", eighteen)

	// 8-industry legacy file (7/1): ai_supply_chain=3.2 (maps to electronics)
	eight := map[string][]SectorIndexData{
		"ai_supply_chain": {{Date: "2026-07-01", Industry: "ai_supply_chain", Index: 100, ReturnPct: 3.2}},
		"robotics":        {{Date: "2026-07-01", Industry: "robotics", Index: 100, ReturnPct: 1.14}},
	}
	writeSectorIndexJSON(t, dir, "sector_indices_20260701_20260701.json", eight)

	r := NewSectorIndexReader(dir)
	got, ok, err := r.Get("2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("no data for 2026-07-01")
	}

	// 18-industry wins for electronics
	if v := got["electronics"]; v != 3.15 {
		t.Errorf("electronics = %v, want 3.15 (18-industry priority)", v)
	}
	// 8-industry machinery (robotics maps to machinery) used because 18 did not cover it
	// 8-industry raw "ai_supply_chain" maps to electronics (already covered by 18).
	// 8-industry raw "robotics" maps to machinery — but the 18-industry
	// fixture above also has a machinery entry, so 18 wins by priority.
	if v := got["machinery"]; v != 1.0 {
		t.Errorf("machinery = %v, want 1.0 (18-industry priority, not 1.14 from 8-industry robotics)", v)
	}
}

// TestSectorIndexReader_SourcePriority_NoAverage verifies the W1 refactor
// does NOT average: with priority policy, the value is the 18-industry raw,
// not an average of 3.15 and 3.2.
func TestSectorIndexReader_SourcePriority_NoAverage(t *testing.T) {
	dir := t.TempDir()
	eighteen := map[string][]SectorIndexData{
		"semiconductor":     {{Date: "2026-07-15", Industry: "semiconductor", Index: 200, ReturnPct: 1.0}},
		"electronics":       {{Date: "2026-07-15", Industry: "electronics", Index: 200, ReturnPct: 1.0}},
		"financials":        {{Date: "2026-07-15", Industry: "financials", Index: 200, ReturnPct: 1.0}},
		"auto":              {{Date: "2026-07-15", Industry: "auto", Index: 200, ReturnPct: 1.0}},
		"biotech":           {{Date: "2026-07-15", Industry: "biotech", Index: 200, ReturnPct: 1.0}},
		"cement":            {{Date: "2026-07-15", Industry: "cement", Index: 200, ReturnPct: 1.0}},
		"construction":      {{Date: "2026-07-15", Industry: "construction", Index: 200, ReturnPct: 1.0}},
		"energy":            {{Date: "2026-07-15", Industry: "energy", Index: 200, ReturnPct: 1.0}},
		"food":              {{Date: "2026-07-15", Industry: "food", Index: 200, ReturnPct: 1.0}},
		"machinery":         {{Date: "2026-07-15", Industry: "machinery", Index: 200, ReturnPct: 1.0}},
		"optoelectronics":   {{Date: "2026-07-15", Industry: "optoelectronics", Index: 200, ReturnPct: 1.0}},
		"other_electronics": {{Date: "2026-07-15", Industry: "other_electronics", Index: 200, ReturnPct: 1.0}},
		"plastics":          {{Date: "2026-07-15", Industry: "plastics", Index: 200, ReturnPct: 1.0}},
		"retail":            {{Date: "2026-07-15", Industry: "retail", Index: 200, ReturnPct: 1.0}},
		"shipping":          {{Date: "2026-07-15", Industry: "shipping", Index: 200, ReturnPct: 1.0}},
		"steel":             {{Date: "2026-07-15", Industry: "steel", Index: 200, ReturnPct: 1.0}},
		"telecom":           {{Date: "2026-07-15", Industry: "telecom", Index: 200, ReturnPct: 1.0}},
		"textiles":          {{Date: "2026-07-15", Industry: "textiles", Index: 200, ReturnPct: 1.0}},
	}
	writeSectorIndexJSON(t, dir, "sector_indices_20260715_20260720.json", eighteen)
	eight := map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-07-15", Industry: "semiconductor", Index: 200, ReturnPct: 9.0}},
	}
	writeSectorIndexJSON(t, dir, "sector_indices_20260715_20260715.json", eight)

	r := NewSectorIndexReader(dir)
	got, _, _ := r.Get("2026-07-15")
	if v := got["semiconductor"]; v != 1.0 {
		t.Errorf("semiconductor = %v, want 1.0 (pure 18-industry, NOT averaged to 5.0)", v)
	}
}
