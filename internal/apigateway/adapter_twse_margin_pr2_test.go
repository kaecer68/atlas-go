package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// twseMarginTestServer serves MI_MARGN-style JSON: the balance table works,
// but the selectType=ALL maintenance-ratio query has no ratio table — mirroring
// the real endpoint (B4c investigation), so the provider snapshot arrives
// without MarginMaintenanceRatio.
func twseMarginTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("selectType") == "ALL" {
			// Real endpoint behavior: no maintenance-ratio table.
			_, _ = w.Write([]byte(`{"stat":"OK","date":"20260904","tables":[{"title":"信用交易統計","data":[]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{
		  "stat": "OK",
		  "date": "20260904",
		  "tables": [
		    {
		      "title": "信用交易統計",
		      "fields": ["項目", "買進", "賣出", "現金(券)償還", "前日餘額", "今日餘額"],
		      "data": [
		        ["融資(交易單位)", "429,048", "495,782", "5,934", "9,149,157", "9,076,489"],
		        ["融券(交易單位)", "23,361", "25,191", "2,062", "239,437", "239,205"],
		        ["融資金額(仟元)", "33,268,274", "37,431,433", "569,362", "100,000,000", "120,000,000"]
		      ]
		    }
		  ]
		}`))
	}))
}

// TestTWSEMarginChannelAdapter_Fetch_FillsMaintenanceRatioFromFinMind covers
// PR-2: live snapshot carries margin_maintenance_ratio (Symbol=TSE_MARGIN_MAINT)
// filled from FinMind when TWSE has none; the daily dedup keeps the fill to
// one FinMind call per day.
func TestTWSEMarginChannelAdapter_Fetch_FillsMaintenanceRatioFromFinMind(t *testing.T) {
	twse := twseMarginTestServer(t)
	defer twse.Close()

	var finmindCalls atomic.Int64
	finmind := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finmindCalls.Add(1)
		if got := r.URL.Query().Get("dataset"); got != "TaiwanTotalExchangeMarginMaintenance" {
			t.Errorf("dataset = %q, want TaiwanTotalExchangeMarginMaintenance", got)
		}
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[
		  {"date":"2026-09-03","TotalExchangeMarginMaintenance":184.437},
		  {"date":"2026-09-04","TotalExchangeMarginMaintenance":186.12}
		]}`))
	}))
	defer finmind.Close()

	provider := marketdata.NewTWSEMarginBalanceProvider("")
	provider.SetBaseURL(twse.URL)
	provider.SetHTTPClient(twse.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	fc := marketdata.NewFinMindClient("test-key")
	fc.SetBaseURL(finmind.URL)
	fc.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	adapter := NewTWSEMarginChannelAdapter(provider)
	adapter.SetFinMindClient(fc)

	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(res.Data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.MarginMaintenanceRatio.Symbol != "TSE_MARGIN_MAINT" {
		t.Fatalf("MarginMaintenanceRatio.Symbol = %q, want TSE_MARGIN_MAINT", snap.MarginMaintenanceRatio.Symbol)
	}
	if snap.MarginMaintenanceRatio.Value != 186.12 {
		t.Errorf("MarginMaintenanceRatio.Value = %v, want 186.12 (latest FinMind row)", snap.MarginMaintenanceRatio.Value)
	}
	if snap.RetailMarginBalance.Symbol == "" {
		t.Errorf("TWSE balance fields should still be present")
	}

	// Second fetch on the same day must not hit FinMind again (daily dedup).
	if _, err := adapter.Fetch(context.Background()); err != nil {
		t.Fatalf("second Fetch error: %v", err)
	}
	if n := finmindCalls.Load(); n != 1 {
		t.Errorf("FinMind calls = %d, want 1 (daily dedup)", n)
	}
}

// TestTWSEMarginChannelAdapter_Fetch_NoFinMind covers the nil-client path:
// no FinMind wired → snapshot simply lacks the ratio, no error.
func TestTWSEMarginChannelAdapter_Fetch_NoFinMind(t *testing.T) {
	twse := twseMarginTestServer(t)
	defer twse.Close()

	provider := marketdata.NewTWSEMarginBalanceProvider("")
	provider.SetBaseURL(twse.URL)
	provider.SetHTTPClient(twse.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	adapter := NewTWSEMarginChannelAdapter(provider)

	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(res.Data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.MarginMaintenanceRatio.Symbol != "" {
		t.Errorf("MarginMaintenanceRatio.Symbol = %q, want empty without FinMind client", snap.MarginMaintenanceRatio.Symbol)
	}
}
