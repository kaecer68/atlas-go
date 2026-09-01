package marketdata

// Tests for the G01/G02 live wiring (#1793): both providers fetch full-market
// datasets from the shared FinMind client and map the generic rows.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestFinMind(t *testing.T, body string) *FinMindClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewFinMindClientWithStateDir("", t.TempDir())
	c.SetBaseURL(srv.URL)
	return c
}

func TestTDCClient_FetchDispersion_MapsFinMindRows(t *testing.T) {
	c := newTestFinMind(t, `{"msg":"success","status":200,"data":[
		{"date":"2026-08-21","stock_id":"2330","HoldingSharesLevel":"1-999","people":2511714,"percent":1.12,"unit":292743125},
		{"date":"2026-08-21","stock_id":"2330","HoldingSharesLevel":"400001-600000","people":120,"percent":1.5,"unit":345678901}
	]}`)
	p := NewTDCClient()
	p.SetFinMindClient(c)

	recs, err := p.FetchDispersion(context.Background(), "20260828")
	if err != nil {
		t.Fatalf("FetchDispersion: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	r0 := recs[0]
	if r0.Symbol != "2330" || r0.Tier != "1-999" || r0.Holders != 2511714 || r0.PctHeld != 1.12 || r0.SharesHeld != 292743125 {
		t.Errorf("row mapping wrong: %+v", r0)
	}
	// Last-fetch state must reflect success (adapter HealthCheck reads it).
	if _, lastErr := p.LastFetchState(); lastErr != "" {
		t.Errorf("lastErr = %q, want empty after success", lastErr)
	}
}

func TestTDCClient_NoFinMind_StubError(t *testing.T) {
	p := NewTDCClient()
	if _, err := p.FetchDispersion(context.Background(), "20260828"); err == nil {
		t.Fatal("expected explicit not-configured error without FinMind client")
	}
}

func TestTDCClient_HistoryBackfill_WritesWeeklyFiles(t *testing.T) {
	// Two weekly snapshots in one month chunk; one pre-existing (idempotency).
	body := `{"msg":"success","status":200,"data":[
		{"date":"2026-07-24","stock_id":"2330","HoldingSharesLevel":"1-999","people":100,"percent":1.0,"unit":1000},
		{"date":"2026-08-21","stock_id":"2330","HoldingSharesLevel":"1-999","people":200,"percent":1.1,"unit":2000},
		{"date":"2026-08-21","stock_id":"2454","HoldingSharesLevel":"1-999","people":50,"percent":2.0,"unit":300}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewFinMindClientWithStateDir("", t.TempDir())
	c.SetBaseURL(srv.URL)

	p := NewTDCClient()
	p.SetFinMindClient(c)
	p.SetStorageDir(t.TempDir())

	start, _ := time.Parse("2006-01-02", "2026-07-01")
	end, _ := time.Parse("2006-01-02", "2026-08-31")
	written, err := p.FetchDispersionHistory(context.Background(), start, end)
	if err != nil {
		t.Fatalf("FetchDispersionHistory: %v", err)
	}
	if written != 2 {
		t.Fatalf("written = %d, want 2 weekly files", written)
	}
	written2, err := p.FetchDispersionHistory(context.Background(), start, end)
	if err != nil || written2 != 0 {
		t.Fatalf("idempotent re-run: written=%d err=%v, want 0/nil", written2, err)
	}
	raw, _ := os.ReadFile(filepath.Join(p.storageDir, "20260821_tdcc_dispersion.json"))
	if !strings.Contains(string(raw), `"symbol": "2454"`) || !strings.Contains(string(raw), `"tier": "1-999"`) {
		t.Errorf("20260821 file content wrong: %s", raw)
	}
}

func TestTWSESBLProvider_FetchSBLSummary_MapsFinMindRows(t *testing.T) {
	c := newTestFinMind(t, `{"msg":"success","status":200,"data":[
		{"stock_id":"2330","SBLShortSalesPreviousDayBalance":100,"SBLShortSalesShortSales":50,"SBLShortSalesReturns":10,"SBLShortSalesCurrentDayBalance":140,"SBLShortSalesQuota":1000000,"date":"2026-08-27"},
		{"stock_id":"2330","SBLShortSalesPreviousDayBalance":140,"SBLShortSalesShortSales":20,"SBLShortSalesReturns":30,"SBLShortSalesCurrentDayBalance":130,"SBLShortSalesQuota":1000000,"date":"2026-08-28"},
		{"stock_id":"2454","SBLShortSalesPreviousDayBalance":0,"SBLShortSalesShortSales":5000,"SBLShortSalesReturns":0,"SBLShortSalesCurrentDayBalance":5000,"SBLShortSalesQuota":200000,"date":"2026-08-28"}
	]}`)
	p := NewTWSESBLProvider(0.5)
	p.SetFinMindClient(c)

	stats, err := p.FetchSBLSummary(context.Background(), "20260828")
	if err != nil {
		t.Fatalf("FetchSBLSummary: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("stats = %d, want 2 (newest row per symbol)", len(stats))
	}
	bySym := map[string]SBLStats{}
	for _, s := range stats {
		bySym[s.Symbol] = s
	}
	s2330 := bySym["2330"]
	if s2330.Date != "2026-08-28" || s2330.SBLShortBalance != 130 || s2330.SBLShortVolume != 20 || s2330.SBLReturnVolume != 30 {
		t.Errorf("2330 mapping wrong: %+v (want newest row 2026-08-28)", s2330)
	}
	if bySym["2454"].SBLShortBalance != 5000 {
		t.Errorf("2454 mapping wrong: %+v", bySym["2454"])
	}
}

func TestTWSESBLProvider_NoFinMind_StubError(t *testing.T) {
	p := NewTWSESBLProvider(0.5)
	if _, err := p.FetchSBLSummary(context.Background(), "20260828"); err == nil {
		t.Fatal("expected explicit not-wired error without FinMind client")
	}
}

func TestTWSESBLProvider_HistoryBackfill_WritesDayFiles(t *testing.T) {
	// Two months of windowed data in one chunk: 3 report days across the
	// boundary; one day pre-exists (idempotency).
	body := `{"msg":"success","status":200,"data":[
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":100,"SBLShortSalesShortSales":10,"SBLShortSalesReturns":5,"date":"2026-07-30"},
		{"stock_id":"2454","SBLShortSalesCurrentDayBalance":200,"SBLShortSalesShortSales":20,"SBLShortSalesReturns":0,"date":"2026-07-30"},
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":110,"SBLShortSalesShortSales":12,"SBLShortSalesReturns":3,"date":"2026-08-28"},
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":120,"SBLShortSalesShortSales":15,"SBLShortSalesReturns":1,"date":"2026-08-31"}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewFinMindClientWithStateDir("", t.TempDir())
	c.SetBaseURL(srv.URL)

	p := NewTWSESBLProvider(0.5)
	p.SetFinMindClient(c)
	p.SetStorageDir(t.TempDir())

	start, _ := time.Parse("2006-01-02", "2026-07-01")
	end, _ := time.Parse("2006-01-02", "2026-08-31")
	written, err := p.FetchSBLHistory(context.Background(), start, end)
	if err != nil {
		t.Fatalf("FetchSBLHistory: %v", err)
	}
	if written != 3 {
		t.Fatalf("written = %d, want 3 day files", written)
	}
	// Second run: all files exist, nothing new.
	written2, err := p.FetchSBLHistory(context.Background(), start, end)
	if err != nil || written2 != 0 {
		t.Fatalf("idempotent re-run: written=%d err=%v, want 0/nil", written2, err)
	}
	// File content sanity.
	raw, _ := os.ReadFile(filepath.Join(p.storageDir, "20260828_sbl.json"))
	if !strings.Contains(string(raw), `"sbl_short_balance": 110`) {
		t.Errorf("20260828 file content wrong: %s", raw)
	}
}
