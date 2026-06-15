package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FinMindDividendProvider parses TaiwanStockDividend records.
// Reference: 2025 TSMC (2330) dividend = 6.0 cash + 0 stock (already approved).
// TSMC typically pays NT$4-6 per share annually.

func TestFinMindDividendProvider_parseDividendRecord_CashOnly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	rec := p.parseDividendRecord(map[string]any{
		"stock_id":                  "2330",
		"CashEarningsDistribution":  4.5,
		"CashStatutorySurplus":      1.5,
		"StockEarningsDistribution": 0.0,
		"CashExDividendTradingDate": "2025-07-17",
		"CashDividendPaymentDate":   "2025-08-15",
		"year":                      "2025",
	})
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", rec.Symbol)
	}
	if rec.CashDividend != 6.0 {
		t.Errorf("CashDividend = %v, want 6.0 (4.5 + 1.5)", rec.CashDividend)
	}
	if rec.StockDividend != 0.0 {
		t.Errorf("StockDividend = %v, want 0.0", rec.StockDividend)
	}
	if rec.Year != 2025 {
		t.Errorf("Year = %d, want 2025", rec.Year)
	}
	if rec.ExDividendDate != "2025-07-17" {
		t.Errorf("ExDividendDate = %q, want 2025-07-17", rec.ExDividendDate)
	}
	if rec.PaymentDate != "2025-08-15" {
		t.Errorf("PaymentDate = %q, want 2025-08-15", rec.PaymentDate)
	}
}

func TestFinMindDividendProvider_parseDividendRecord_StockDividend(t *testing.T) {
	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	rec := p.parseDividendRecord(map[string]any{
		"stock_id":                  "1101",
		"StockEarningsDistribution": 0.5,
	})
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.StockDividend != 0.5 {
		t.Errorf("StockDividend = %v, want 0.5", rec.StockDividend)
	}
	if rec.CashDividend != 0 {
		t.Errorf("CashDividend = %v, want 0", rec.CashDividend)
	}
}

func TestFinMindDividendProvider_parseDividendRecord_NoSymbol(t *testing.T) {
	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	if rec := p.parseDividendRecord(map[string]any{"CashEarningsDistribution": 1.0}); rec != nil {
		t.Error("expected nil when stock_id missing or empty")
	}
}

func TestFinMindDividendProvider_parseDividendRecord_EmptySymbol(t *testing.T) {
	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	if rec := p.parseDividendRecord(map[string]any{"stock_id": ""}); rec != nil {
		t.Error("expected nil when stock_id is empty string")
	}
}

func TestFinMindDividendProvider_parseDividendRecord_InvalidYear(t *testing.T) {
	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	rec := p.parseDividendRecord(map[string]any{
		"stock_id": "2330",
		"year":     "not-a-number",
	})
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Year != 0 {
		t.Errorf("Year = %d, want 0 for invalid input", rec.Year)
	}
}

func TestFinMindDividendProvider_parseDividendRecord_EmptyOptionalFields(t *testing.T) {
	p := NewFinMindDividendProvider(NewFinMindClient("k"), t.TempDir())
	rec := p.parseDividendRecord(map[string]any{
		"stock_id": "2330",
		// No cash/stock distribution, no dates, no year
	})
	if rec == nil {
		t.Fatal("expected non-nil record with defaults")
	}
	if rec.CashDividend != 0 || rec.StockDividend != 0 {
		t.Error("cash/stock dividends should default to 0")
	}
	if rec.ExDividendDate != "" || rec.PaymentDate != "" {
		t.Error("dates should be empty strings when absent")
	}
	if rec.Year != 0 {
		t.Errorf("Year = %d, want 0 when absent", rec.Year)
	}
}

func TestFinMindDividendProvider_GetDividends_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataset") != "TaiwanStockDividend" {
			t.Errorf("dataset = %q, want TaiwanStockDividend", r.URL.Query().Get("dataset"))
		}
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[
				{"stock_id":"2330","CashEarningsDistribution":4.5,"CashStatutorySurplus":1.5,"year":"2025"},
				{"stock_id":"2330","CashEarningsDistribution":3.5,"CashStatutorySurplus":0.5,"year":"2024"}
			]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	cacheDir := t.TempDir()
	p := NewFinMindDividendProvider(c, cacheDir)
	records, err := p.GetDividends(context.Background(), "2330", "2024-01-01", "2025-12-31")
	if err != nil {
		t.Fatalf("GetDividends error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].CashDividend != 6.0 {
		t.Errorf("records[0].CashDividend = %v, want 6.0", records[0].CashDividend)
	}
	if records[1].CashDividend != 4.0 {
		t.Errorf("records[1].CashDividend = %v, want 4.0", records[1].CashDividend)
	}

	// Verify cache file was written
	cacheFile := filepath.Join(cacheDir, "2330_2024-01-01_2025-12-31.json")
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Errorf("expected cache file at %s", cacheFile)
	}
}

func TestFinMindDividendProvider_GetDividends_FromCache(t *testing.T) {
	cacheDir := t.TempDir()
	symbol := "2330"
	startDate := "2024-01-01"
	endDate := "2025-12-31"

	cached := []domain.DividendRecord{
		{Symbol: "2330", Year: 2025, CashDividend: 6.0},
	}
	data, _ := json.MarshalIndent(cached, "", "  ")
	cacheFile := filepath.Join(cacheDir, "2330_2024-01-01_2025-12-31.json")
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	p := NewFinMindDividendProvider(NewFinMindClient("k"), cacheDir)
	records, err := p.GetDividends(context.Background(), symbol, startDate, endDate)
	if err != nil {
		t.Fatalf("GetDividends error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 cached record, got %d", len(records))
	}
	if records[0].CashDividend != 6.0 {
		t.Errorf("CashDividend = %v, want 6.0 (from cache)", records[0].CashDividend)
	}
}

func TestFinMindDividendProvider_GetDividends_CacheExpired(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "2330_2024-01-01_2025-12-31.json")
	if err := os.WriteFile(cacheFile, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	// Make the cache file 25 hours old (past 24h TTL)
	oldTime := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(cacheFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"stock_id":"2330","CashEarningsDistribution":5.0,"year":"2025"}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindDividendProvider(c, cacheDir)
	records, err := p.GetDividends(context.Background(), "2330", "2024-01-01", "2025-12-31")
	if err != nil {
		t.Fatalf("GetDividends error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 fresh record (cache expired), got %d", len(records))
	}
	if records[0].CashDividend != 5.0 {
		t.Errorf("CashDividend = %v, want 5.0 (fresh fetch)", records[0].CashDividend)
	}
}

func TestFinMindDividendProvider_GetLatestDividend_NoRecords(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindDividendProvider(c, t.TempDir())
	_, err := p.GetLatestDividend(context.Background(), "9999")
	if err == nil {
		t.Fatal("expected error for no dividend data")
	}
}

func TestFinMindDividendProvider_cacheFilePath_DotReplacement(t *testing.T) {
	p := NewFinMindDividendProvider(nil, "/tmp/cache")
	// Symbol with dot → dot replaced with underscore for filesystem safety
	got := p.cacheFilePath("BRK.B", "2024-01-01", "2024-12-31")
	want := filepath.Join("/tmp/cache", "BRK_B_2024-01-01_2024-12-31.json")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFinMindDividendProvider_loadFromCache_StaleCache(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "X_2024-01-01_2024-12-31.json")
	if err := os.WriteFile(cacheFile, []byte(`[{"Symbol":"X"}]`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(cacheFile, oldTime, oldTime)

	p := NewFinMindDividendProvider(nil, cacheDir)
	records, ok := p.loadFromCache("X", "2024-01-01", "2024-12-31")
	if ok {
		t.Error("expected cache miss for stale file")
	}
	if records != nil {
		t.Error("expected nil records for stale cache")
	}
}

func TestFinMindDividendProvider_loadFromCache_InvalidJSON(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "X_2024-01-01_2024-12-31.json")
	if err := os.WriteFile(cacheFile, []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p := NewFinMindDividendProvider(nil, cacheDir)
	records, ok := p.loadFromCache("X", "2024-01-01", "2024-12-31")
	if ok {
		t.Error("expected cache miss for malformed JSON")
	}
	if records != nil {
		t.Error("expected nil records for malformed JSON")
	}
}

func TestFinMindDividendProvider_loadFromCache_MissingFile(t *testing.T) {
	p := NewFinMindDividendProvider(nil, t.TempDir())
	records, ok := p.loadFromCache("NONEXISTENT", "2024-01-01", "2024-12-31")
	if ok || records != nil {
		t.Error("expected cache miss for missing file")
	}
}

func TestFinMindDividendProvider_saveToCache(t *testing.T) {
	cacheDir := t.TempDir()
	p := NewFinMindDividendProvider(nil, cacheDir)
	records := []domain.DividendRecord{
		{Symbol: "2330", Year: 2025, CashDividend: 6.0},
	}
	if err := p.saveToCache("2330", "2024-01-01", "2025-12-31", records); err != nil {
		t.Fatalf("saveToCache error: %v", err)
	}
	expected := filepath.Join(cacheDir, "2330_2024-01-01_2025-12-31.json")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	var got []domain.DividendRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].CashDividend != 6.0 {
		t.Errorf("got %+v, want 1 record with CashDividend=6.0", got)
	}
}

func TestFinMindDividendProvider_saveToCache_NestedDirectory(t *testing.T) {
	// saveToCache must create the directory if missing
	cacheDir := filepath.Join(t.TempDir(), "nested", "deeper")
	p := NewFinMindDividendProvider(nil, cacheDir)
	if err := p.saveToCache("Y", "2024-01-01", "2024-12-31", []domain.DividendRecord{{Symbol: "Y"}}); err != nil {
		t.Fatalf("saveToCache error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "Y_2024-01-01_2024-12-31.json")); err != nil {
		t.Errorf("expected cache file to exist: %v", err)
	}
}
