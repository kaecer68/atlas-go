package marketdata

// Tests for the G01/G02 live wiring: both providers fetch full-market
// datasets from the shared FinMind client.
//
// EMPIRICAL contract (verified 2026-09-01 against the real API): both
// TaiwanStockHoldingSharesPer and TaiwanDailyShortSaleBalances return rows
// ONLY for single-day windows (a multi-day full-market window returns an
// empty result, or only the start date's rows). The mock therefore filters
// rows by the requested start_date, exactly like the real API.

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
)

// finmindMock serves rows whose "date" equals the requested start_date.
func finmindMock(t *testing.T, allRows string) *FinMindClient {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(allRows), &rows); err != nil {
		t.Fatalf("mock rows: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start_date")
		out := []map[string]any{}
		for _, row := range rows {
			if fmt.Sprint(row["date"]) == start {
				out = append(out, row)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"msg": "success", "status": 200, "data": out})
	}))
	t.Cleanup(srv.Close)
	c := NewFinMindClientWithStateDir("", t.TempDir())
	c.SetBaseURL(srv.URL)
	// Tests must not sleep on the production rate limiter (~2.7s/call):
	// a 62-day history walk would take minutes.
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	return c
}

func TestTDCClient_FetchDispersion_MapsFinMindRows(t *testing.T) {
	c := finmindMock(t, `[
		{"date":"2026-08-28","stock_id":"2330","HoldingSharesLevel":"1-999","people":2511714,"percent":1.12,"unit":292743125},
		{"date":"2026-08-28","stock_id":"2330","HoldingSharesLevel":"400001-600000","people":120,"percent":1.5,"unit":345678901}
	]`)
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
	if _, lastErr := p.LastFetchState(); lastErr != "" {
		t.Errorf("lastErr = %q, want empty after success", lastErr)
	}
}

func TestTDCClient_FetchDispersion_WalksBackToLatestSnapshot(t *testing.T) {
	// Target 8/28 (no snapshot yet); the newest published snapshot is 8/21.
	c := finmindMock(t, `[
		{"date":"2026-08-21","stock_id":"2330","HoldingSharesLevel":"1-999","people":100,"percent":1.0,"unit":1000}
	]`)
	p := NewTDCClient()
	p.SetFinMindClient(c)

	recs, err := p.FetchDispersion(context.Background(), "20260828")
	if err != nil {
		t.Fatalf("FetchDispersion walk-back: %v", err)
	}
	if len(recs) != 1 || recs[0].Date != "2026-08-21" {
		t.Errorf("walk-back should land on the 2026-08-21 snapshot, got %+v", recs)
	}
}

func TestTDCClient_NoFinMind_StubError(t *testing.T) {
	p := NewTDCClient()
	if _, err := p.FetchDispersion(context.Background(), "20260828"); err == nil {
		t.Fatal("expected explicit not-configured error without FinMind client")
	}
}

func TestTDCClient_HistoryBackfill_WritesWeeklyFiles(t *testing.T) {
	// Two weekly snapshots (7/24, 8/21) across a two-month range; the mock
	// only serves rows for the exact probed Friday.
	c := finmindMock(t, `[
		{"date":"2026-07-24","stock_id":"2330","HoldingSharesLevel":"1-999","people":100,"percent":1.0,"unit":1000},
		{"date":"2026-08-21","stock_id":"2330","HoldingSharesLevel":"1-999","people":200,"percent":1.1,"unit":2000},
		{"date":"2026-08-21","stock_id":"2454","HoldingSharesLevel":"1-999","people":50,"percent":2.0,"unit":300}
	]`)
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
	c := finmindMock(t, `[
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":130,"SBLShortSalesShortSales":20,"SBLShortSalesReturns":30,"date":"2026-08-28"}
	]`)
	p := NewTWSESBLProvider(0.5)
	p.SetFinMindClient(c)

	stats, err := p.FetchSBLSummary(context.Background(), "20260828")
	if err != nil {
		t.Fatalf("FetchSBLSummary: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("stats = %d, want 1", len(stats))
	}
	s := stats[0]
	if s.Symbol != "2330" || s.SBLShortBalance != 130 || s.SBLShortVolume != 20 || s.SBLReturnVolume != 30 {
		t.Errorf("mapping wrong: %+v", s)
	}
}

func TestTWSESBLProvider_NoFinMind_StubError(t *testing.T) {
	p := NewTWSESBLProvider(0.5)
	if _, err := p.FetchSBLSummary(context.Background(), "20260828"); err == nil {
		t.Fatal("expected explicit not-wired error without FinMind client")
	}
}

func TestTWSESBLProvider_HistoryBackfill_WritesDayFiles(t *testing.T) {
	// Three report days across a two-month range; the mock serves each day
	// only on its own date probe (single-day window contract).
	c := finmindMock(t, `[
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":100,"SBLShortSalesShortSales":10,"SBLShortSalesReturns":5,"date":"2026-07-30"},
		{"stock_id":"2454","SBLShortSalesCurrentDayBalance":200,"SBLShortSalesShortSales":20,"SBLShortSalesReturns":0,"date":"2026-07-30"},
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":110,"SBLShortSalesShortSales":12,"SBLShortSalesReturns":3,"date":"2026-08-28"},
		{"stock_id":"2330","SBLShortSalesCurrentDayBalance":120,"SBLShortSalesShortSales":15,"SBLShortSalesReturns":1,"date":"2026-08-31"}
	]`)
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
	written2, err := p.FetchSBLHistory(context.Background(), start, end)
	if err != nil || written2 != 0 {
		t.Fatalf("idempotent re-run: written=%d err=%v, want 0/nil", written2, err)
	}
	raw, _ := os.ReadFile(filepath.Join(p.storageDir, "20260828_sbl.json"))
	if !strings.Contains(string(raw), `"sbl_short_balance": 110`) {
		t.Errorf("20260828 file content wrong: %s", raw)
	}
}
