package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers — create fixture files in t.TempDir()
// ---------------------------------------------------------------------------

// twseCSVFixture returns a minimal but valid TWSE daily CSV (header + 2
// symbols × 2 dates) saved under t.TempDir()/name. The returned path is
// absolute so LoadTWSEOpenDataCSV can open it from any working directory.
func twseCSVFixture(t *testing.T, name string) string {
	t.Helper()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction",
		"2026-03-20,2330,台積電,32001234,25801234567,790,795,788,792,2,19555",
		"2026-03-20,2317,鴻海,38002345,6100123456,158,159.5,157,158.5,0.5,17234",
		"2026-03-21,2330,台積電,28000000,22000000000,792,800,791,798,6,18000",
		"2026-03-21,2317,鴻海,35000000,5600000000,158.5,160,158,159.5,1.0,16000",
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// twseCSVFixtureMissingCol returns a CSV missing the "Close" column.
func twseCSVFixtureMissingCol(t *testing.T) string {
	t.Helper()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,Open,High,Low",
		"2026-03-20,2330,台積電,32001234,790,795,788",
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), "missing_col.csv")
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// twseCSVFixtureBadDate returns a CSV whose first data row has a malformed date.
func twseCSVFixtureBadDate(t *testing.T) string {
	t.Helper()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction",
		"not-a-date,2330,台積電,32001234,25801234567,790,795,788,792,2,19555",
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), "bad_date.csv")
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// jsonlFixture writes valid JSONL lines to t.TempDir()/<name> and returns the absolute path.
func jsonlFixture(t *testing.T, name string, lines []jsonlRow) string {
	t.Helper()
	var buf strings.Builder
	for _, row := range lines {
		b, _ := json.Marshal(row)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------------------
// LoadTWSEOpenDataCSV
// ---------------------------------------------------------------------------

func TestLoadTWSEOpenDataCSV(t *testing.T) {
	ds, err := LoadTWSEOpenDataCSV("../../samples/replay/twse_stock_day_all_sample.csv")
	if err != nil {
		t.Fatalf("load replay dataset: %v", err)
	}
	if len(ds.Dates) != 7 {
		t.Fatalf("expected 7 dates, got %d", len(ds.Dates))
	}

	date, _ := time.Parse("2006-01-02", "2026-03-26")
	ret, ok := ds.ForwardReturn("2330.TW", date, 1)
	if !ok {
		t.Fatalf("expected forward return")
	}
	if ret == 0 {
		t.Fatalf("expected non-zero forward return")
	}
}

func TestLoadTWSEOpenDataCSV_FileNotFound(t *testing.T) {
	_, err := LoadTWSEOpenDataCSV("/nonexistent/path.csv")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), ErrReplayDataMissing.Error()) {
		t.Fatalf("expected ErrReplayDataMissing, got: %v", err)
	}
}

func TestLoadTWSEOpenDataCSV_MissingColumn(t *testing.T) {
	path := twseCSVFixtureMissingCol(t)
	_, err := LoadTWSEOpenDataCSV(path)
	if err == nil {
		t.Fatal("expected error for missing column")
	}
}

func TestLoadTWSEOpenDataCSV_BadDate(t *testing.T) {
	path := twseCSVFixtureBadDate(t)
	_, err := LoadTWSEOpenDataCSV(path)
	if err == nil {
		t.Fatal("expected error for bad date format")
	}
}

func TestLoadTWSEOpenDataCSV_TempFixture(t *testing.T) {
	path := twseCSVFixture(t, "test.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Dates) != 2 {
		t.Fatalf("expected 2 dates, got %d", len(ds.Dates))
	}
	if len(ds.ByDate) != 2 {
		t.Fatalf("expected 2 date entries, got %d", len(ds.ByDate))
	}
	// Dates must be sorted ascending
	if ds.Dates[0].After(ds.Dates[1]) {
		t.Fatal("dates not sorted ascending")
	}
}

// ---------------------------------------------------------------------------
// QuotesForDate
// ---------------------------------------------------------------------------

func TestQuotesForDate_Success(t *testing.T) {
	path := twseCSVFixture(t, "quotes.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	quotes := ds.QuotesForDate(date, []string{"2330.TW", "2317.TW"})
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(quotes))
	}
	if quotes[0].Symbol != "2330.TW" {
		t.Fatalf("expected 2330.TW, got %s", quotes[0].Symbol)
	}
	if quotes[0].Market != "TW" {
		t.Fatalf("expected Market=TW, got %s", quotes[0].Market)
	}
}

func TestQuotesForDate_MissingSymbol(t *testing.T) {
	path := twseCSVFixture(t, "quotes.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	quotes := ds.QuotesForDate(date, []string{"9999.TW"})
	if len(quotes) != 0 {
		t.Fatalf("expected 0 quotes for unknown symbol, got %d", len(quotes))
	}
}

func TestQuotesForDate_EmptySymbols(t *testing.T) {
	path := twseCSVFixture(t, "quotes.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	quotes := ds.QuotesForDate(date, nil)
	if len(quotes) != 0 {
		t.Fatalf("expected 0 quotes for nil symbols, got %d", len(quotes))
	}
}

func TestQuotesForDate_IsTradable(t *testing.T) {
	path := twseCSVFixture(t, "quotes.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	quotes := ds.QuotesForDate(date, []string{"2330.TW"})
	if len(quotes) == 0 {
		t.Fatal("expected at least 1 quote")
	}
	if !quotes[0].IsTradable {
		t.Fatal("expected IsTradable=true for stock with close>0 and volume>0")
	}
}

// ---------------------------------------------------------------------------
// ForwardReturn
// ---------------------------------------------------------------------------

func TestForwardReturn_MissingSymbol(t *testing.T) {
	path := twseCSVFixture(t, "fw.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	_, ok := ds.ForwardReturn("9999.TW", date, 1)
	if ok {
		t.Fatal("expected false for unknown symbol")
	}
}

func TestForwardReturn_ZEROClose(t *testing.T) {
	// Build a CSV where the current bar has Close=0
	dir := t.TempDir()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction",
		"2026-03-20,0000,Zero,0,0,0,0,0,0,0,0",
		"2026-03-21,0000,Zero,10000,100000,50,52,49,51,1,500",
	}, "\n") + "\n"
	path := filepath.Join(dir, "zero_close.csv")
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	_, ok := ds.ForwardReturn("0000.TW", date, 1)
	if ok {
		t.Fatal("expected false for zero close")
	}
}

func TestForwardReturn_DuplicateOHLCV(t *testing.T) {
	// Two dates with identical OHLCV across both — the stale data rejection should kick in.
	dir := t.TempDir()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction",
		"2026-03-20,0000,Dup,10000,100000,10,20,5,15,1,500",
		"2026-03-21,0000,Dup,10000,100000,10,20,5,15,1,500",
	}, "\n") + "\n"
	path := filepath.Join(dir, "dup_ohlcv.csv")
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	_, ok := ds.ForwardReturn("0000.TW", date, 1)
	if ok {
		t.Fatal("expected false for duplicate OHLCV (stale data)")
	}
}

func TestForwardReturn_WindowOutOfRange(t *testing.T) {
	path := twseCSVFixture(t, "fw.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	// Request forward window=10 on the last date — out of range
	date, _ := time.Parse("2006-01-02", "2026-03-21")
	_, ok := ds.ForwardReturn("2330.TW", date, 10)
	if ok {
		t.Fatal("expected false for out-of-range forward window")
	}
}

// ---------------------------------------------------------------------------
// NextDate
// ---------------------------------------------------------------------------

func TestNextDate_Success(t *testing.T) {
	path := twseCSVFixture(t, "next.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-20")
	next, ok := ds.NextDate(date, 1)
	if !ok {
		t.Fatal("expected next date")
	}
	if next.Format("2006-01-02") != "2026-03-21" {
		t.Fatalf("expected 2026-03-21, got %s", next.Format("2006-01-02"))
	}
}

func TestNextDate_OutOfRange(t *testing.T) {
	path := twseCSVFixture(t, "next.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2026-03-21")
	_, ok := ds.NextDate(date, 1)
	if ok {
		t.Fatal("expected false for out of range")
	}
}

func TestNextDate_DateNotFound(t *testing.T) {
	path := twseCSVFixture(t, "next.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	date, _ := time.Parse("2006-01-02", "2025-01-01")
	_, ok := ds.NextDate(date, 1)
	if ok {
		t.Fatal("expected false for date not in dataset")
	}
}

// ---------------------------------------------------------------------------
// helpers: value, parseFloat, parseInt
// ---------------------------------------------------------------------------

func TestValue_InBounds(t *testing.T) {
	record := []string{"a", "b", "c"}
	index := map[string]int{"k0": 0, "k1": 1, "k2": 2}
	if v := value(record, index, "k1"); v != "b" {
		t.Fatalf("expected 'b', got '%s'", v)
	}
}

func TestValue_OutOfBounds(t *testing.T) {
	record := []string{"a"}
	index := map[string]int{"k0": 0, "k9": 9}
	if v := value(record, index, "k9"); v != "" {
		t.Fatalf("expected empty string, got '%s'", v)
	}
}

func TestParseFloat_Normal(t *testing.T) {
	if v := parseFloat("123.45"); v != 123.45 {
		t.Fatalf("expected 123.45, got %f", v)
	}
}

func TestParseFloat_WithCommas(t *testing.T) {
	if v := parseFloat("1,234.56"); v != 1234.56 {
		t.Fatalf("expected 1234.56, got %f", v)
	}
}

func TestParseFloat_Empty(t *testing.T) {
	if v := parseFloat(""); v != 0 {
		t.Fatalf("expected 0, got %f", v)
	}
}

func TestParseFloat_Dash(t *testing.T) {
	if v := parseFloat("--"); v != 0 {
		t.Fatalf("expected 0, got %f", v)
	}
}

func TestParseInt_Normal(t *testing.T) {
	if v := parseInt("12345"); v != 12345 {
		t.Fatalf("expected 12345, got %d", v)
	}
}

func TestParseInt_WithCommas(t *testing.T) {
	if v := parseInt("1,234,567"); v != 1234567 {
		t.Fatalf("expected 1234567, got %d", v)
	}
}

func TestParseInt_Empty(t *testing.T) {
	if v := parseInt(""); v != 0 {
		t.Fatalf("expected 0, got %d", v)
	}
}

func TestParseInt_Dash(t *testing.T) {
	if v := parseInt("--"); v != 0 {
		t.Fatalf("expected 0, got %d", v)
	}
}

// ---------------------------------------------------------------------------
// GetLatestDate — CSV path
// ---------------------------------------------------------------------------

func TestGetLatestDate_CSV(t *testing.T) {
	path := twseCSVFixture(t, "latest.csv")
	latest, err := GetLatestDate(path)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "2026-03-21" {
		t.Fatalf("expected 2026-03-21, got %s", latest)
	}
}

func TestGetLatestDate_CSVFileNotFound(t *testing.T) {
	_, err := GetLatestDate("/nonexistent.csv")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// GetLatestDate — JSONL path  (the task focus)
// ---------------------------------------------------------------------------

func TestGetLatestDate_JSONL(t *testing.T) {
	path := jsonlFixture(t, "test.jsonl", []jsonlRow{
		{Date: "2026-03-20", Symbol: "2330", Open: 790, High: 795, Low: 788, Close: 792, Volume: 32001234},
		{Date: "2026-03-21", Symbol: "2330", Open: 792, High: 800, Low: 791, Close: 798, Volume: 28000000},
	})
	latest, err := GetLatestDate(path)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "2026-03-21" {
		t.Fatalf("expected 2026-03-21, got %s", latest)
	}
}

func TestGetLatestDate_JSONLFileNotFound(t *testing.T) {
	_, err := GetLatestDate("/nonexistent.jsonl")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ErrReplayDataMissing.Error()) {
		t.Fatalf("expected ErrReplayDataMissing, got: %v", err)
	}
}

func TestGetLatestDate_JSONLEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GetLatestDate(path)
	if err == nil {
		t.Fatal("expected error for empty JSONL file")
	}
}

func TestGetLatestDate_JSONLOnlyWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "whitespace.jsonl")
	if err := os.WriteFile(path, []byte("\n\n   \n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GetLatestDate(path)
	if err == nil {
		t.Fatal("expected error for whitespace-only JSONL file")
	}
}

func TestGetLatestDate_JSONLCorruptLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.jsonl")
	content := `{"date":"2026-03-20","symbol":"2330","open":790,"high":795,"low":788,"close":792,"volume":32001234}
not valid json
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GetLatestDate(path)
	if err == nil {
		t.Fatal("expected error for corrupt last line")
	}
}

func TestGetLatestDate_JSONLMissingDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodate.jsonl")
	content := `{"symbol":"2330","open":790,"high":795,"low":788,"close":792,"volume":32001234}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GetLatestDate(path)
	if err == nil {
		t.Fatal("expected error for missing date field")
	}
}

func TestGetLatestDate_JSONLSingleLine(t *testing.T) {
	path := jsonlFixture(t, "single.jsonl", []jsonlRow{
		{Date: "2026-03-20", Symbol: "2330", Open: 790, High: 795, Low: 788, Close: 792, Volume: 32001234},
	})
	latest, err := GetLatestDate(path)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "2026-03-20" {
		t.Fatalf("expected 2026-03-20, got %s", latest)
	}
}

func TestGetLatestDate_JSONLTrailingNewline(t *testing.T) {
	// JSONL with trailing newline after last line — should still parse correctly
	path := filepath.Join(t.TempDir(), "trailing.jsonl")
	content := `{"date":"2026-03-20","symbol":"2330","open":790,"high":795,"low":788,"close":792,"volume":32001234}
{"date":"2026-03-21","symbol":"2330","open":792,"high":800,"low":791,"close":798,"volume":28000000}

`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	latest, err := GetLatestDate(path)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "2026-03-21" {
		t.Fatalf("expected 2026-03-21, got %s", latest)
	}
}

// ---------------------------------------------------------------------------
// Helper type tests
// ---------------------------------------------------------------------------

func TestJSONLRow_RoundTrip(t *testing.T) {
	row := jsonlRow{
		Date: "2026-03-20", Symbol: "2330",
		Open: 790, High: 795, Low: 788, Close: 792, Volume: 32001234,
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var got jsonlRow
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Date != row.Date || got.Symbol != row.Symbol || got.Close != row.Close {
		t.Fatal("round-trip mismatch")
	}
}
