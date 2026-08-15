package portfolio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFundamentalProvider(t *testing.T) {
	fp := NewFundamentalProvider()
	if fp == nil {
		t.Fatal("NewFundamentalProvider returned nil")
	}
	if fp.data == nil {
		t.Error("data map should be initialized")
	}
	if fp.HasData() {
		t.Error("new provider should have no data")
	}
	// Unknown symbol returns empty sector
	if got := fp.GetSector("2330.TW"); got != "" {
		t.Errorf("expected empty sector for unknown symbol, got %q", got)
	}
}

func TestFundamentalProvider_HasData(t *testing.T) {
	fp := NewFundamentalProvider()
	if fp.HasData() {
		t.Error("empty provider should not have data")
	}

	fp.data["2330.TW"] = FundamentalData{PE: 25.0}
	if !fp.HasData() {
		t.Error("provider with data should return true")
	}
}

func TestFundamentalProvider_Get(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{PE: 25.0, PB: 6.1, Sector: "semiconductor"}

	got := fp.Get("2330.TW")
	if got.PE != 25.0 {
		t.Errorf("expected PE 25.0, got %v", got.PE)
	}
	if got.Sector != "semiconductor" {
		t.Errorf("expected sector semiconductor, got %q", got.Sector)
	}

	// Missing symbol returns zero value
	missing := fp.Get("9999.TW")
	if missing.PE != 0 {
		t.Errorf("expected zero PE for missing symbol, got %v", missing.PE)
	}
}

func TestFundamentalProvider_GetSector(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{Sector: "semiconductor"}
	fp.data["2882.TW"] = FundamentalData{Sector: "financials"}

	if got := fp.GetSector("2330.TW"); got != "semiconductor" {
		t.Errorf("expected sector semiconductor, got %q", got)
	}
	if got := fp.GetSector("2882.TW"); got != "financials" {
		t.Errorf("expected sector financials, got %q", got)
	}
	if got := fp.GetSector("9999.TW"); got != "" {
		t.Errorf("expected empty sector for missing symbol, got %q", got)
	}
}

func TestFundamentalProvider_SectorMedianPE(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{PE: 25.0, Sector: "semiconductor"}
	fp.data["2454.TW"] = FundamentalData{PE: 30.0, Sector: "semiconductor"}
	fp.data["3008.TW"] = FundamentalData{PE: 20.0, Sector: "semiconductor"}
	fp.data["2882.TW"] = FundamentalData{PE: 10.0, Sector: "financials"}

	// For semiconductor: [20, 25, 30] → median = 25
	median := fp.SectorMedianPE("semiconductor")
	if median != 25.0 {
		t.Errorf("expected median PE 25.0 for semiconductor, got %v", median)
	}
}

func TestFundamentalProvider_SectorMedianPE_EvenCount(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{PE: 25.0, Sector: "semiconductor"}
	fp.data["2454.TW"] = FundamentalData{PE: 35.0, Sector: "semiconductor"}
	fp.data["3008.TW"] = FundamentalData{PE: 20.0, Sector: "semiconductor"}
	fp.data["3017.TW"] = FundamentalData{PE: 30.0, Sector: "semiconductor"}

	// Sorted: [20, 25, 30, 35] → median = (25+30)/2 = 27.5
	median := fp.SectorMedianPE("semiconductor")
	if median != 27.5 {
		t.Errorf("expected median PE 27.5 for even-count sector, got %v", median)
	}
}

func TestFundamentalProvider_SectorMedianPE_UnknownSector(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{PE: 25.0, Sector: "semiconductor"}

	median := fp.SectorMedianPE("nonexistent")
	if median != 0 {
		t.Errorf("expected 0 for unknown sector, got %v", median)
	}
}

func TestFundamentalProvider_SectorMedianPE_ZeroPEExcluded(t *testing.T) {
	fp := NewFundamentalProvider()
	fp.data["2330.TW"] = FundamentalData{PE: 0, Sector: "semiconductor"} // invalid PE
	fp.data["2454.TW"] = FundamentalData{PE: 30.0, Sector: "semiconductor"}

	median := fp.SectorMedianPE("semiconductor")
	if median != 30.0 {
		t.Errorf("expected median PE 30.0 excluding zero PE, got %v", median)
	}
}

func TestFundamentalProvider_SectorMedianPE_EmptyProvider(t *testing.T) {
	fp := NewFundamentalProvider()
	median := fp.SectorMedianPE("semiconductor")
	if median != 0 {
		t.Errorf("expected 0 for empty provider, got %v", median)
	}
}

func TestFundamentalProvider_LoadFromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")

	ps2330 := 8.0
	ps2882 := 1.5
	data := map[string]FundamentalData{
		"2330.TW": {PE: 25.0, PB: 6.1, PS: &ps2330, DividendYield: 1.5, Sector: "semiconductor"},
		"2882.TW": {PE: 10.0, PB: 1.2, PS: &ps2882, DividendYield: 4.0, Sector: "financials"},
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	fp := NewFundamentalProvider()
	if err := fp.LoadFromJSON(path); err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	if !fp.HasData() {
		t.Error("provider should have data after load")
	}

	got := fp.Get("2330.TW")
	if got.PE != 25.0 {
		t.Errorf("expected PE 25.0, got %v", got.PE)
	}
	if got.PB != 6.1 {
		t.Errorf("expected PB 6.1, got %v", got.PB)
	}
}

func TestFundamentalProvider_LoadFromJSON_FileNotFound(t *testing.T) {
	fp := NewFundamentalProvider()
	err := fp.LoadFromJSON("/nonexistent/path/fundamentals.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFundamentalProvider_LoadFromJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not valid json"), 0o644)

	fp := NewFundamentalProvider()
	err := fp.LoadFromJSON(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFundamentalProvider_LoadFromJSON_PreviousDataReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fundamentals.json")

	// First write one dataset
	data1 := map[string]FundamentalData{
		"2330.TW": {PE: 25.0, Sector: "semiconductor"},
	}
	b1, _ := json.Marshal(data1)
	os.WriteFile(path, b1, 0o644)

	fp := NewFundamentalProvider()
	fp.LoadFromJSON(path)

	// Then overwrite with different data
	data2 := map[string]FundamentalData{
		"2882.TW": {PE: 10.0, Sector: "financials"},
	}
	b2, _ := json.Marshal(data2)
	os.WriteFile(path, b2, 0o644)

	fp.LoadFromJSON(path)

	// Old data should be gone
	if fp.Get("2330.TW").PE != 0 {
		t.Error("old data should be replaced by new load")
	}
	if fp.Get("2882.TW").PE != 10.0 {
		t.Error("new data should be present")
	}
}

// TestSectorConstants removed — tautological test (constant == its own literal).
// Sector constants are verified through usage in GetSector/Get tests above.
