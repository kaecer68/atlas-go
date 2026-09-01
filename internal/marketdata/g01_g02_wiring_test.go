package marketdata

// Tests for the G01/G02 live wiring (#1793): both providers fetch full-market
// datasets from the shared FinMind client and map the generic rows.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
