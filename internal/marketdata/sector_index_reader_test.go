package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSectorIndexReader_ReadRange_8IndustrySchema(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"ai_supply_chain":   {{Date: "2026-06-03", Industry: "ai_supply_chain", ReturnPct: 3.0}},
		"electronics":       {{Date: "2026-06-03", Industry: "electronics", ReturnPct: -2.0}},
		"robotics":          {{Date: "2026-06-03", Industry: "robotics", ReturnPct: -2.5}},
		"semiconductor":     {{Date: "2026-06-03", Industry: "semiconductor", ReturnPct: 0.6}},
		"shipping":          {{Date: "2026-06-03", Industry: "shipping", ReturnPct: 0.1}},
		"financials":        {{Date: "2026-06-03", Industry: "financials", ReturnPct: 2.2}},
		"energy":            {{Date: "2026-06-03", Industry: "energy", ReturnPct: 1.9}},
		"other_electronics": {{Date: "2026-06-03", Industry: "other_electronics", ReturnPct: 0.7}},
	})

	reader := NewSectorIndexReader(dir)
	start, _ := time.Parse("2006-01-02", "2026-06-03")
	end, _ := time.Parse("2006-01-02", "2026-06-03")
	data, err := reader.ReadRange(start, end)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	returns, ok := data["2026-06-03"]
	if !ok {
		t.Fatal("Expected 2026-06-03 in result")
	}
	if _, ok := returns["ai_supply_chain"]; ok {
		t.Error("ai_supply_chain should be normalized to electronics")
	}
	if got, ok := returns["electronics"]; !ok || got != 0.5 {
		t.Errorf("electronics return = %v, want 0.5 (average of 3 and -2)", got)
	}
	if got, ok := returns["machinery"]; !ok || got != -2.5 {
		t.Errorf("machinery return = %v, want -2.5 (mapped from robotics)", got)
	}
	if got, ok := returns["semiconductor"]; !ok || got != 0.6 {
		t.Errorf("semiconductor return = %v, want 0.6", got)
	}
	if len(returns) != 7 {
		t.Errorf("Expected 7 canonical industries from 8-industry schema, got %d: %v", len(returns), returns)
	}
}

func TestSectorIndexReader_ReadRange_18IndustrySchema(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260701_20260702.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-07-01", ReturnPct: 1.0}, {Date: "2026-07-02", ReturnPct: 1.5}},
		"biotech":       {{Date: "2026-07-01", ReturnPct: 2.0}, {Date: "2026-07-02", ReturnPct: 2.5}},
		"food":          {{Date: "2026-07-01", ReturnPct: 3.0}, {Date: "2026-07-02", ReturnPct: 3.5}},
	})

	reader := NewSectorIndexReader(dir)
	start, _ := time.Parse("2006-01-02", "2026-07-01")
	end, _ := time.Parse("2006-01-02", "2026-07-02")
	data, err := reader.ReadRange(start, end)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	if len(data) != 2 {
		t.Fatalf("Expected 2 dates, got %d", len(data))
	}
	if got := data["2026-07-01"]["semiconductor"]; got != 1.0 {
		t.Errorf("2026-07-01 semiconductor = %v, want 1.0", got)
	}
	if got := data["2026-07-02"]["biotech"]; got != 2.5 {
		t.Errorf("2026-07-02 biotech = %v, want 2.5", got)
	}
}

func TestSectorIndexReader_ReadRange_MissingDateNotFilled(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260623_20260623.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-23", ReturnPct: 1.0}},
	})
	writeSectorIndexFixture(t, dir, "sector_indices_20260625_20260625.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-25", ReturnPct: 1.0}},
	})

	reader := NewSectorIndexReader(dir)
	start, _ := time.Parse("2006-01-02", "2026-06-23")
	end, _ := time.Parse("2006-01-02", "2026-06-25")
	data, err := reader.ReadRange(start, end)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	if _, ok := data["2026-06-24"]; ok {
		t.Error("2026-06-24 should be missing, not zero-filled")
	}
	if len(data) != 2 {
		t.Errorf("Expected 2 dates, got %d", len(data))
	}
}

func TestSectorIndexReader_Get(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-03", ReturnPct: 0.6}},
	})

	reader := NewSectorIndexReader(dir)
	returns, ok, err := reader.Get("2026-06-03")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if !ok {
		t.Fatal("Expected data for 2026-06-03")
	}
	if returns["semiconductor"] != 0.6 {
		t.Errorf("semiconductor return = %v, want 0.6", returns["semiconductor"])
	}

	_, ok, err = reader.Get("2026-06-04")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if ok {
		t.Error("Expected no data for 2026-06-04")
	}
}

func TestSectorIndexReader_AvailableDates(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-03", ReturnPct: 0.6}},
	})
	writeSectorIndexFixture(t, dir, "sector_indices_20260605_20260605.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-05", ReturnPct: 0.7}},
	})

	reader := NewSectorIndexReader(dir)
	dates, err := reader.AvailableDates()
	if err != nil {
		t.Fatalf("AvailableDates error = %v", err)
	}
	want := []string{"2026-06-03", "2026-06-05"}
	if len(dates) != len(want) {
		t.Fatalf("AvailableDates = %v, want %v", dates, want)
	}
	for i, d := range want {
		if dates[i] != d {
			t.Errorf("AvailableDates[%d] = %q, want %q", i, dates[i], d)
		}
	}
}

func TestSectorIndexReader_ReadRange_LegacyCollisionUsesArithmeticMean(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"ai_supply_chain": {{Date: "2026-06-03", ReturnPct: 3.0}},
		"electronics":     {{Date: "2026-06-03", ReturnPct: -2.0}, {Date: "2026-06-03", ReturnPct: 5.0}},
	})

	reader := NewSectorIndexReader(dir)
	date, _ := time.Parse("2006-01-02", "2026-06-03")
	data, err := reader.ReadRange(date, date)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	if got := data["2026-06-03"]["electronics"]; got != 2.0 {
		t.Errorf("electronics return = %v, want 2 (average of 3, -2, and 5)", got)
	}
}

func TestSectorIndexReader_ReadRange_LegacyFilesKeepFirstFileValue(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"electronics": {{Date: "2026-06-03", ReturnPct: 4.0}},
	})
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260604.json", map[string][]SectorIndexData{
		"electronics": {{Date: "2026-06-03", ReturnPct: 8.0}},
	})

	reader := NewSectorIndexReader(dir)
	date, _ := time.Parse("2006-01-02", "2026-06-03")
	data, err := reader.ReadRange(date, date)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	if got := data["2026-06-03"]["electronics"]; got != 4.0 {
		t.Errorf("electronics return = %v, want 4 from first legacy file", got)
	}
}

func writeSectorIndexFixture(t *testing.T, dir, filename string, data map[string][]SectorIndexData) {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeJSON(path, data); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func TestSectorIndexReader_IgnoresUnknownIndustries(t *testing.T) {
	dir := t.TempDir()
	writeSectorIndexFixture(t, dir, "sector_indices_20260603_20260603.json", map[string][]SectorIndexData{
		"semiconductor": {{Date: "2026-06-03", ReturnPct: 0.6}},
		"bogus_sector":  {{Date: "2026-06-03", ReturnPct: 99.9}},
	})

	reader := NewSectorIndexReader(dir)
	start, _ := time.Parse("2006-01-02", "2026-06-03")
	data, err := reader.ReadRange(start, start)
	if err != nil {
		t.Fatalf("ReadRange error = %v", err)
	}
	if _, ok := data["2026-06-03"]["bogus_sector"]; ok {
		t.Error("Unknown industry should be dropped")
	}
	if strings.Contains(strings.Join(func() []string {
		var keys []string
		for k := range data["2026-06-03"] {
			keys = append(keys, k)
		}
		return keys
	}(), ""), "bogus") {
		t.Error("Unknown industry leaked into canonical map")
	}
}

func TestCanonicalSectorID_Mappings(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"ai_supply_chain", "electronics"},
		{"robotics", "machinery"},
		{"semiconductor", "semiconductor"},
		{"bogus", ""},
	}
	for _, c := range cases {
		if got := canonicalSectorID(c.raw); got != c.want {
			t.Errorf("canonicalSectorID(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
