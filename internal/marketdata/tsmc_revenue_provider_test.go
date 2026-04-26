package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTSMCRevenueProvider_Name(t *testing.T) {
	p := NewTSMCRevenueProvider("data/state/tsmc_revenue")
	if p.Name() != "tsmc_revenue" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestTSMCRevenueProvider_FetchSnapshot_MockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// New OpenAPI format: direct array with Chinese field names
		w.Write([]byte(`[{
			"出表日期":"1150417",
			"資料年月":"11503",
			"公司代號":"2330",
			"公司名稱":"台積電",
			"產業別":"半導體業",
			"營業收入-當月營收":"401255000",
			"營業收入-上月營收":"317657000",
			"營業收入-去年當月營收":"293521000",
			"營業收入-上月比較增減(%)":"26.3",
			"營業收入-去年同月增減(%)":"36.8",
			"累計營業收入-當月累計營收":"1134103000",
			"累計營業收入-去年累計營收":"839523000",
			"累計營業收入-前期比較增減(%)":"35.1",
			"備註":"-"
		}]`))
	}))
	defer ts.Close()

	p := TSMCRevenueProviderWithClient(ts.Client(), t.TempDir())
	p.baseURL = ts.URL
	p.client.Timeout = 5 * time.Second

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snap.TSMCRevenue.Symbol != "TSMC_REVENUE" {
		t.Fatalf("unexpected symbol: %s", snap.TSMCRevenue.Symbol)
	}
	if snap.TSMCRevenue.Value != 401.255 {
		t.Fatalf("unexpected revenue value: %v (expected 401.255)", snap.TSMCRevenue.Value)
	}
	if snap.TSMCRevenue.ChangePct != 36.8 {
		t.Fatalf("unexpected YoY change: %v", snap.TSMCRevenue.ChangePct)
	}
	if snap.RecordedAt == 0 {
		t.Fatalf("RecordedAt should be set")
	}
}

func TestTSMCRevenueProvider_FetchSnapshot_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return data without TSMC (2330)
		w.Write([]byte(`[{"公司代號":"2303","公司名稱":"聯電","營業收入-當月營收":"50000000","營業收入-去年同月增減(%)":"10.0"}]`))
	}))
	defer ts.Close()

	p := TSMCRevenueProviderWithClient(ts.Client(), t.TempDir())
	p.baseURL = ts.URL
	p.client.Timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error when TSMC (2330) not found")
	}
}
