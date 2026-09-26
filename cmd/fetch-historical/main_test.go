package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/marketdata/twse"
)

func TestTradingDayFilter(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected int
	}{
		{
			name:     "mon fri range",
			start:    "2026-01-05",
			end:      "2026-01-09",
			expected: 5,
		},
		{
			name:     "includes saturday",
			start:    "2026-01-02",
			end:      "2026-01-04",
			expected: 1,
		},
		{
			name:     "includes sunday",
			start:    "2026-01-04",
			end:      "2026-01-06",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, _ := twse.ParseDate(tt.start)
			end, _ := twse.ParseDate(tt.end)
			dates := tradingDates(start, end)
			if len(dates) != tt.expected {
				t.Errorf("tradingDates(%s, %s) = %d, want %d", tt.start, tt.end, len(dates), tt.expected)
			}
			for _, d := range dates {
				if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
					t.Errorf("tradingDates included weekend: %s", d.Format("2006-01-02"))
				}
			}
		})
	}
}

func TestMergeDedup(t *testing.T) {
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing.jsonl")

	existingBars := []HistoricalBar{
		{Date: "2026-01-02", Symbol: "2330.TW", Close: 1000, Volume: 1000},
		{Date: "2026-01-02", Symbol: "2317.TW", Close: 200, Volume: 500},
	}
	for _, bar := range existingBars {
		data, _ := json.Marshal(bar)
		f, _ := os.OpenFile(existingPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		fmt.Fprintln(f, string(data))
		f.Close()
	}

	keys, err := loadExistingKeys(existingPath)
	if err != nil {
		t.Fatalf("loadExistingKeys failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	if !keys["2026-01-02+2330.TW"] {
		t.Error("expected key 2026-01-02+2330.TW to exist")
	}
	if !keys["2026-01-02+2317.TW"] {
		t.Error("expected key 2026-01-02+2317.TW to exist")
	}
	if keys["2026-01-03+2330.TW"] {
		t.Error("unexpected key 2026-01-03+2330.TW should not exist")
	}

	keys["2026-01-02+2330.TW"] = true

	newBars := []HistoricalBar{
		{Date: "2026-01-02", Symbol: "2330.TW", Close: 1005},
		{Date: "2026-01-03", Symbol: "2330.TW", Close: 1010},
	}

	for _, bar := range newBars {
		key := bar.Date + "+" + bar.Symbol
		if keys[key] {
			continue
		}
		keys[key] = true
	}

	if len(keys) != 3 {
		t.Errorf("after dedup expected 3 keys, got %d", len(keys))
	}
}

func TestJSONLFormat(t *testing.T) {
	bar := HistoricalBar{
		Date:   "2026-01-02",
		Symbol: "2330.TW",
		Name:   "台積電",
		Open:   990,
		High:   1010,
		Low:    985,
		Close:  1000,
		Volume: 10000000,
	}

	data, err := json.Marshal(bar)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed HistoricalBar
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed.Date != bar.Date {
		t.Errorf("Date: got %s, want %s", parsed.Date, bar.Date)
	}
	if parsed.Symbol != bar.Symbol {
		t.Errorf("Symbol: got %s, want %s", parsed.Symbol, bar.Symbol)
	}
	if parsed.Close != bar.Close {
		t.Errorf("Close: got %f, want %f", parsed.Close, bar.Close)
	}
	if parsed.Volume != bar.Volume {
		t.Errorf("Volume: got %d, want %d", parsed.Volume, bar.Volume)
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"1,000.5", 1000.5},
		{"1000", 1000},
		{"", 0},
		{"--", 0},
		{"-", 0},
		{"1.5", 1.5},
	}

	for _, tt := range tests {
		got := twse.ParseFloat(tt.input)
		if got != tt.expected {
			t.Errorf("twse.ParseFloat(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"10,000,000", 10000000},
		{"10000000", 10000000},
		{"", 0},
		{"--", 0},
		{"-", 0},
	}

	for _, tt := range tests {
		got := twse.ParseInt64(tt.input)
		if got != tt.expected {
			t.Errorf("twse.ParseInt64(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestExistingKeysEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.jsonl")

	f, _ := os.Create(emptyPath)
	f.Close()

	keys, err := loadExistingKeys(emptyPath)
	if err != nil {
		t.Fatalf("loadExistingKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for empty file, got %d", len(keys))
	}
}

func TestExistingKeysNonexistent(t *testing.T) {
	keys, err := loadExistingKeys("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatalf("loadExistingKeys should not fail for nonexistent file: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for nonexistent file, got %d", len(keys))
	}
}

func TestQuotesFilteredByZeroClose(t *testing.T) {
	quotes := []marketdata.TWSEQuote{
		{Code: "2330", ClosingPrice: "1000"},
		{Code: "2317", ClosingPrice: "0"},
		{Code: "2303", ClosingPrice: ""},
	}

	var bars []HistoricalBar
	for _, q := range quotes {
		close := twse.ParseFloat(q.ClosingPrice)
		if close == 0 {
			continue
		}
		bars = append(bars, HistoricalBar{
			Symbol: q.Code + ".TW",
			Close:  close,
		})
	}

	if len(bars) != 1 {
		t.Errorf("expected 1 bar (2330 only), got %d", len(bars))
	}
	if bars[0].Symbol != "2330.TW" {
		t.Errorf("expected symbol 2330.TW, got %s", bars[0].Symbol)
	}
}

func TestFormatYYYYMMDD(t *testing.T) {
	tm, _ := time.Parse("2006-01-02", "2026-01-02")
	got := formatYYYYMMDD(tm)
	if got != "20260102" {
		t.Errorf("formatYYYYMMDD got %s, want 20260102", got)
	}
}

func TestParseDate(t *testing.T) {
	got, err := twse.ParseDate("2026-01-02")
	if err != nil {
		t.Fatalf("parseDate failed: %v", err)
	}
	if got.Year() != 2026 || got.Month() != 1 || got.Day() != 2 {
		t.Errorf("parseDate got %v, want 2026-01-02", got)
	}
}

func TestAppendJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")

	bars := []HistoricalBar{
		{Date: "2026-01-02", Symbol: "2330.TW", Close: 1000},
		{Date: "2026-01-03", Symbol: "2330.TW", Close: 1005},
	}

	err := appendJSONL(path, bars)
	if err != nil {
		t.Fatalf("appendJSONL failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}

	lines := len(data)
	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}
}

// miIndexTestFields is the real 每日收盤行情 header (measured 2026-09-24).
var miIndexTestFields = []string{"證券代號", "證券名稱", "成交股數", "成交筆數", "成交金額", "開盤價", "最高價", "最低價", "收盤價", "漲跌(+/-)", "漲跌價差", "最後揭示買價", "最後揭示買量", "最後揭示賣價", "最後揭示賣量", "本益比"}

// miIndexTestBody renders the shape MI_INDEX actually returns: an index
// section followed by the 每日收盤行情 section, all inside `tables`.
//
// The pre-2026-09-26 test fixture fabricated a flat `fields`/`data` envelope
// that the real endpoint has never returned, which is why the suite stayed
// green while production fetched 0 rows every day.
func miIndexTestBody(titleDate string, rows ...string) []byte {
	body := fmt.Sprintf(`{"stat":"OK","date":"20260102","tables":[
		{"title":%q,"fields":["指數","收盤指數"],"data":[["發行量加權股價指數","23,000.00"]]},
		{"title":%q,"fields":[%s],"data":[%s]}]}`,
		titleDate+" 價格指數(臺灣證券交易所)",
		titleDate+" 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))",
		quoteFields(miIndexTestFields), strings.Join(rows, ","))
	return []byte(body)
}

func quoteFields(fields []string) string {
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, fmt.Sprintf("%q", f))
	}
	return strings.Join(quoted, ",")
}

// newMIINDEXTestFetcher points a Fetcher at a stub MI_INDEX server.
func newMIINDEXTestFetcher(t *testing.T, body []byte) *Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "MI_INDEX") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("type") != "ALLBUT0999" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("date") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return &Fetcher{
		client:      &http.Client{Timeout: 10 * time.Second},
		baseURL:     srv.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}
}

func TestFetchDayDecodesTheMIINDEXTablesEnvelope(t *testing.T) {
	body := miIndexTestBody("115年01月02日",
		`["2330","台積電","10,000,000","5,000","10,000,000,000","1000","1010","990","1005","+","5","0","0","0","0","0"]`,
		`["2317","鴻海","5,000,000","3,000","500,000,000","200","205","198","202","+","2","0","0","0","0","0"]`,
		`["0000","不良股","0","0","0","0","0","0","0","-","0","0","0","0","0","0"]`,
	)
	fetcher := newMIINDEXTestFetcher(t, body)

	quotes, err := fetcher.FetchDay(context.Background(), time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("FetchDay failed: %v", err)
	}
	if len(quotes) != 3 {
		t.Fatalf("expected 3 quotes (including 0000), got %d", len(quotes))
	}
	if quotes[0].Code != "2330" || quotes[0].ClosingPrice != "1005" || quotes[0].TradeVolume != "10,000,000" {
		t.Errorf("quote[0] = %+v, want the 2330 row from the 每日收盤行情 section", quotes[0])
	}
	if quotes[2].Code != "0000" {
		t.Errorf("quote[2].Code = %s, want 0000", quotes[2].Code)
	}
}

func TestFetchDayClosedMarketIsEmptyNotAnError(t *testing.T) {
	// Live behaviour for a holiday/weekend date: HTTP 200 with the Chinese
	// "no data" stat and no tables.
	body := []byte(`{"stat":"很抱歉，沒有符合條件的資料!","type":"ALLBUT0999"}`)
	fetcher := newMIINDEXTestFetcher(t, body)

	quotes, err := fetcher.FetchDay(context.Background(), time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local))
	if !errors.Is(err, marketdata.ErrTWSEEmptyData) {
		t.Fatalf("FetchDay on a closed market = (%v, %v), want ErrTWSEEmptyData", quotes, err)
	}
	if len(quotes) != 0 {
		t.Errorf("expected 0 quotes for a closed market, got %d", len(quotes))
	}
}

func TestFetchDayRejectsAMisDatedPayload(t *testing.T) {
	// The payload describes 2026-01-05 while 2026-01-02 was requested: the
	// always-latest-upstream shape. Stamping those prices with the requested
	// date is how the replay dataset acquires phantom dates.
	body := miIndexTestBody("115年01月05日",
		`["2330","台積電","10,000,000","5,000","10,000,000,000","1000","1010","990","1005","+","5","0","0","0","0","0"]`)
	fetcher := newMIINDEXTestFetcher(t, body)

	quotes, err := fetcher.FetchDay(context.Background(), time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("FetchDay accepted a payload for another date")
	}
	if !strings.Contains(err.Error(), "2026-01-05") || !strings.Contains(err.Error(), "2026-01-02") {
		t.Errorf("error %q must name both dates", err.Error())
	}
	if len(quotes) != 0 {
		t.Errorf("expected no quotes from a mis-dated payload, got %d", len(quotes))
	}
}

func TestFetchDayRejectsTheLegacyFlatEnvelope(t *testing.T) {
	// Regression guard for the silent-success bug: the flat `fields`/`data`
	// shape (what this tool used to expect, and what its old test fabricated)
	// must fail loudly instead of yielding 0 rows with a nil error.
	body := []byte(`{"stat":"OK","date":"20260102","title":"每日收盤行情(全部)","fields":["證券代號","證券名稱"],"data":[["2330","台積電"]]}`)
	fetcher := newMIINDEXTestFetcher(t, body)

	quotes, err := fetcher.FetchDay(context.Background(), time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatalf("legacy flat envelope produced %d quotes with no error, want a loud failure", len(quotes))
	}
	if !strings.Contains(err.Error(), "schema change") {
		t.Errorf("error %q must name the schema change", err.Error())
	}
}

func TestHistoricalBarJSONFields(t *testing.T) {
	bar := HistoricalBar{
		Date:   "2026-01-02",
		Symbol: "2330.TW",
		Name:   "台積電",
		Open:   990,
		High:   1010,
		Low:    985,
		Close:  1000,
		Volume: 10000000,
	}

	data, _ := json.Marshal(bar)
	jsonStr := string(data)

	expectedFields := []string{
		`"date":"2026-01-02"`,
		`"symbol":"2330.TW"`,
		`"name":"台積電"`,
		`"open":990`,
		`"high":1010`,
		`"low":985`,
		`"close":1000`,
		`"volume":10000000`,
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON missing field %s, got: %s", field, jsonStr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
