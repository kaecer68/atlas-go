package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

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
	quotes := []TWSEQuote{
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

func TestFetchDayIntegration(t *testing.T) {
	mockResponse := MIINDEXResponse{
		Stat:   "OK",
		Date:   "20260102",
		Title:  "每日收盤行情(全部)",
		Fields: []string{"證券代號", "證券名稱", "成交股數", "成交金額", "開盤價", "最高價", "最低價", "收盤價", "漲跌價差", "成交筆數"},
		Data: [][]string{
			{"2330", "台積電", "10,000,000", "10,000,000,000", "1000", "1010", "990", "1005", "+5", "5,000"},
			{"2317", "鴻海", "5,000,000", "500,000,000", "200", "205", "198", "202", "+2", "3,000"},
			{"0000", "不良股", "0", "0", "0", "0", "0", "0", "0", "0"},
		},
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "MI_INDEX") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("type") != "ALLBUT0999" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer mockServer.Close()

	fetcher := &Fetcher{
		client:      &http.Client{Timeout: 10 * time.Second},
		baseURL:     mockServer.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}

	d := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	quotes, err := fetcher.FetchDay(context.Background(), d)
	if err != nil {
		t.Fatalf("FetchDay failed: %v", err)
	}

	if len(quotes) != 3 {
		t.Fatalf("expected 3 quotes (including 0000), got %d", len(quotes))
	}
	if quotes[0].Code != "2330" {
		t.Errorf("quote[0].Code = %s, want 2330", quotes[0].Code)
	}
	if quotes[0].ClosingPrice != "1005" {
		t.Errorf("quote[0].ClosingPrice = %s, want 1005", quotes[0].ClosingPrice)
	}
	if quotes[2].Code != "0000" {
		t.Errorf("quote[2].Code = %s, want 0000", quotes[2].Code)
	}
}

func TestMIINDEXStatNotOK(t *testing.T) {
	mockResponse := MIINDEXResponse{Stat: "ERROR", Data: nil}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer mockServer.Close()

	fetcher := &Fetcher{
		client:      &http.Client{Timeout: 10 * time.Second},
		baseURL:     mockServer.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}

	quotes, err := fetcher.FetchDay(context.Background(), time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("FetchDay should not error on stat != OK: %v", err)
	}
	if len(quotes) != 0 {
		t.Errorf("expected 0 quotes for stat=ERROR, got %d", len(quotes))
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
