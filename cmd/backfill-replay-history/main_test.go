package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ------------------------------------------------------------------
// TestConvertFinMindBar — FinMind sample row → replay format.
// ------------------------------------------------------------------
func TestConvertFinMindBar(t *testing.T) {
	in := replayBar{
		Date: "2024-07-01", Symbol: "2454.TW", Name: "",
		Open: 1300, High: 1308.52, Low: 1288.21, Close: 1296.82, Volume: 114432867,
	}
	got, ok := convertFinMindBar(in, "2024-07-01", "2024-12-31", "聯發科")
	if !ok {
		t.Fatal("row inside window should convert")
	}
	if got.Date != "2024-07-01T00:00:00Z" {
		t.Errorf("date = %q; want %q", got.Date, "2024-07-01T00:00:00Z")
	}
	if got.Source != finmindSource {
		t.Errorf("source = %q; want %q", got.Source, finmindSource)
	}
	if got.Name != "聯發科" {
		t.Errorf("name = %q; want 聯發科 (carried from universe)", got.Name)
	}
	if got.Open != 1300 || got.High != 1308.52 || got.Low != 1288.21 || got.Close != 1296.82 {
		t.Errorf("OHLC not preserved: %+v", got)
	}
	if got.Volume != 114432867 {
		t.Errorf("volume = %d; want 114432867", got.Volume)
	}
}

func TestConvertFinMindBarWindow(t *testing.T) {
	in := replayBar{Date: "2024-06-30", Symbol: "0050.TW"}
	if _, ok := convertFinMindBar(in, "2024-07-01", "2024-12-31", "元大台灣50"); ok {
		t.Error("2024-06-30 must be excluded (before window start)")
	}
	in = replayBar{Date: "2025-01-02", Symbol: "0050.TW"}
	if _, ok := convertFinMindBar(in, "2024-07-01", "2024-12-31", "元大台灣50"); ok {
		t.Error("2025-01-02 must be excluded (after window end)")
	}
	in = replayBar{Date: "2024-07-01", Symbol: "0050.TW"}
	if _, ok := convertFinMindBar(in, "2024-07-01", "2024-12-31", "元大台灣50"); !ok {
		t.Error("2024-07-01 (window start, inclusive) must be kept")
	}
	in = replayBar{Date: "2024-12-31", Symbol: "0050.TW"}
	if _, ok := convertFinMindBar(in, "2024-07-01", "2024-12-31", "元大台灣50"); !ok {
		t.Error("2024-12-31 (window end, inclusive) must be kept")
	}
}

// ------------------------------------------------------------------
// TestMergeDedupNoOverwrite — existing twse rows win over FinMind rows.
// ------------------------------------------------------------------
func TestMergeDedupNoOverwrite(t *testing.T) {
	existing := []replayBar{
		{Date: "2024-07-01T00:00:00Z", Symbol: "2454.TW", Name: "聯發科",
			Open: 1300, High: 1308.52, Low: 1288.21, Close: 1296.82, Volume: 114432867, Source: "twse_open_data_csv"},
		{Date: "2024-07-02T00:00:00Z", Symbol: "2454.TW", Name: "聯發科",
			Open: 1301, High: 1309, Low: 1289, Close: 1297, Volume: 100, Source: "twse_open_data_csv"},
	}
	finmind := []replayBar{
		// Same (symbol,date) as the first existing row — must be skipped.
		{Date: "2024-07-01T00:00:00Z", Symbol: "2454.TW", Name: "",
			Open: 999, High: 999, Low: 999, Close: 999, Volume: 1, Source: finmindSource},
		// Missing (symbol,date) — must be added.
		{Date: "2024-07-03T00:00:00Z", Symbol: "2454.TW", Name: "",
			Open: 1302, High: 1310, Low: 1290, Close: 1298, Volume: 200, Source: finmindSource},
	}
	merged, added, skipped := mergeBars(existing, finmind, nil)
	if added != 1 || skipped != 1 {
		t.Fatalf("added=%d skipped=%d; want 1/1", added, skipped)
	}
	if len(merged) != 3 {
		t.Fatalf("merged len = %d; want 3", len(merged))
	}
	// The original twse row keeps its values.
	for _, b := range merged {
		if b.Date == "2024-07-01T00:00:00Z" && b.Symbol == "2454.TW" {
			if b.Open != 1300 || b.Source != "twse_open_data_csv" {
				t.Errorf("existing row overwritten: %+v", b)
			}
		}
	}
	// The new row is present with finmind source.
	found := false
	for _, b := range merged {
		if b.Date == "2024-07-03T00:00:00Z" && b.Symbol == "2454.TW" {
			found = true
			if b.Source != finmindSource {
				t.Errorf("new row source = %q; want %q", b.Source, finmindSource)
			}
		}
	}
	if !found {
		t.Error("2024-07-03 row missing from merged output")
	}
}

// ------------------------------------------------------------------
// TestMergeSymbolFilter — --symbols restricts the FinMind window.
// ------------------------------------------------------------------
func TestMergeSymbolFilter(t *testing.T) {
	existing := []replayBar{
		{Date: "2025-10-01T00:00:00Z", Symbol: "0056.TW", Name: "0056", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
		{Date: "2025-10-01T00:00:00Z", Symbol: "1216.TW", Name: "統一", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
	}
	finmind := []replayBar{
		{Date: "2024-07-01", Symbol: "0056.TW", Name: "", Open: 2, High: 2, Low: 2, Close: 2, Volume: 2},
		{Date: "2024-07-01", Symbol: "1216.TW", Name: "", Open: 3, High: 3, Low: 3, Close: 3, Volume: 3},
		{Date: "2024-07-01", Symbol: "1301.TW", Name: "", Open: 4, High: 4, Low: 4, Close: 4, Volume: 4},
	}
	merged, added, skipped := mergeBars(existing, finmind, map[string]bool{"0056.TW": true, "1216.TW": true})
	if added != 2 || skipped != 0 {
		t.Fatalf("added=%d skipped=%d; want 2/0 (1301 filtered out)", added, skipped)
	}
	for _, b := range merged {
		if b.Symbol == "1301.TW" {
			t.Error("1301.TW must be filtered out by --symbols")
		}
	}
}

// ------------------------------------------------------------------
// TestExtraSourceGapBackfill — extra JSONL patches interrupted symbols
// (1216/2357/3231 stopped at 2026-04-24) without touching existing rows.
// ------------------------------------------------------------------
func TestExtraSourceGapBackfill(t *testing.T) {
	existing := []replayBar{
		{Date: "2026-04-24T00:00:00Z", Symbol: "1216.TW", Name: "統一",
			Open: 75, High: 76, Low: 74.5, Close: 75.5, Volume: 1000, Source: "twse_open_data_csv"},
		{Date: "2026-04-24T00:00:00Z", Symbol: "2357.TW", Name: "華碩",
			Open: 598, High: 601, Low: 584, Close: 585, Volume: 2000, Source: "twse_open_data_csv"},
	}
	extra := []replayBar{
		// Same (symbol,date) as an existing row — must NOT overwrite.
		{Date: "2026-04-24T00:00:00Z", Symbol: "1216.TW", Name: "",
			Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
		// Post-interruption rows — must be appended.
		{Date: "2026-04-27T00:00:00Z", Symbol: "1216.TW", Name: "統一",
			Open: 75.8, High: 76.4, Low: 75.2, Close: 76, Volume: 3000, Source: "twse_open_data_csv"},
		{Date: "2026-04-27T00:00:00Z", Symbol: "2357.TW", Name: "華碩",
			Open: 590, High: 600, Low: 588, Close: 595, Volume: 4000, Source: "twse_open_data_csv"},
		{Date: "2026-04-28T00:00:00Z", Symbol: "1216.TW", Name: "統一",
			Open: 76.1, High: 76.5, Low: 75.6, Close: 76.2, Volume: 3500, Source: "twse_open_data_csv"},
	}
	merged, added, _ := mergeBars(existing, extra, nil)
	if added != 3 {
		t.Fatalf("added=%d; want 3 (one duplicate skipped)", added)
	}
	if len(merged) != 5 {
		t.Fatalf("merged len = %d; want 5", len(merged))
	}
	for _, b := range merged {
		if b.Date == "2026-04-24T00:00:00Z" && b.Symbol == "1216.TW" {
			if b.Open != 75 || b.Close != 75.5 {
				t.Errorf("existing 1216 row overwritten by extra source: %+v", b)
			}
		}
	}
}

// ------------------------------------------------------------------
// TestMergeOutputSorted — output sorted by (date, symbol).
// ------------------------------------------------------------------
func TestMergeOutputSorted(t *testing.T) {
	existing := []replayBar{
		{Date: "2024-07-02T00:00:00Z", Symbol: "0050.TW", Name: "元大台灣50", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
		{Date: "2024-07-01T00:00:00Z", Symbol: "2330.TW", Name: "台積電", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
	}
	finmind := []replayBar{
		{Date: "2024-07-01", Symbol: "0050.TW", Name: "", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1},
	}
	merged, added, _ := mergeBars(existing, finmind, nil)
	if added != 1 {
		t.Fatalf("added=%d; want 1", added)
	}
	want := []string{key("0050.TW", "2024-07-01"), key("2330.TW", "2024-07-01"), key("0050.TW", "2024-07-02")}
	for i, b := range merged {
		if got := key(b.Symbol, b.Date); got != want[i] {
			t.Errorf("merged[%d] = %s; want %s (sorted by date, then symbol)", i, got, want[i])
		}
	}
}

// ------------------------------------------------------------------
// TestLoadBarsSkipsTrailer — loader tolerates non-JSON trailer lines.
// ------------------------------------------------------------------
func TestLoadBarsSkipsTrailer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.jsonl")
	content := "{\"date\":\"2024-07-01\",\"symbol\":\"0050.TW\",\"name\":\"\",\"open\":1,\"high\":1,\"low\":1,\"close\":1,\"volume\":1}\n" +
		"Total 2335 stock records from 20 stocks\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	bars, err := loadBars(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("loaded %d bars; want 1 (trailer skipped)", len(bars))
	}
}

// ------------------------------------------------------------------
// TestWriteBarsRoundtrip — writeBars produces loadable sorted JSONL.
// ------------------------------------------------------------------
func TestWriteBarsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")
	bars := []replayBar{
		{Date: "2024-07-01T00:00:00Z", Symbol: "0050.TW", Name: "元大台灣50",
			Open: 190, High: 191.23, Low: 189.07, Close: 190.64, Volume: 81160741, Source: finmindSource},
	}
	if err := writeBars(path, bars); err != nil {
		t.Fatal(err)
	}
	got, err := loadBars(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("roundtrip len = %d; want 1", len(got))
	}
	if got[0].Date != "2024-07-01T00:00:00Z" || got[0].Source != finmindSource {
		t.Errorf("roundtrip mismatch: %+v", got[0])
	}
	if !strings.Contains(got[0].Date, "T00:00:00Z") {
		t.Errorf("date must keep replay suffix: %q", got[0].Date)
	}
}

// ------------------------------------------------------------------
// TestUniverseDefaultFilter — non-universe FinMind symbols must not leak
// into the merged output (default filter = symbols in the replay target).
// ------------------------------------------------------------------
func TestUniverseDefaultFilter(t *testing.T) {
	existing := []replayBar{
		{Date: "2025-10-01T00:00:00Z", Symbol: "0056.TW", Name: "0056", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Source: "twse_open_data_csv"},
	}
	finmind := []replayBar{
		{Date: "2024-07-01", Symbol: "0056.TW", Name: "", Open: 2, High: 2, Low: 2, Close: 2, Volume: 2},
		{Date: "2024-07-01", Symbol: "9918.TW", Name: "", Open: 3, High: 3, Low: 3, Close: 3, Volume: 3}, // not in universe
	}
	filter := make(map[string]bool, len(existing))
	for _, b := range existing {
		filter[b.Symbol] = true
	}
	merged, added, _ := mergeBars(existing, finmind, filter)
	if added != 1 {
		t.Fatalf("added=%d; want 1 (9918.TW must be excluded by default universe filter)", added)
	}
	for _, b := range merged {
		if b.Symbol == "9918.TW" {
			t.Error("9918.TW leaked into merged output")
		}
	}
}
