package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExchangeRateProvider_ChangePctFromCache 驗證 PR-B Bug#5 fix：
// ExchangeRate-API free tier 沒有 historical endpoint，所以 ChangePct 改從
// in-memory cache 上次 fetch 計算。
// - 第一次 fetch：cache 為 0 → ChangePct=0（cold start）
// - 第二次 fetch：cache = 上次的 rate → ChangePct = (new-old)/old*100
func TestExchangeRateProvider_ChangePctFromCache(t *testing.T) {
	var currentRates = map[string]float64{
		"TWD": 31.5,
		"JPY": 150.0,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := exchangeRateResponse{
			Result:   "success",
			BaseCode: "USD",
			Rates:    currentRates,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewExchangeRateProvider()
	p.latestURL = srv.URL
	p.SetHTTPClient(srv.Client())

	// First fetch: cold start
	snap1, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if snap1.USD_TWD.ChangePct != 0 {
		t.Errorf("cold start: USD/TWD.ChangePct = %v, want 0", snap1.USD_TWD.ChangePct)
	}
	if snap1.JPY.ChangePct != 0 {
		t.Errorf("cold start: JPY.ChangePct = %v, want 0", snap1.JPY.ChangePct)
	}

	// Mutate server rates to simulate next fetch
	currentRates["TWD"] = 32.0  // +1.587%
	currentRates["JPY"] = 148.0 // -1.333%

	// Second fetch: ChangePct derived from cached previous
	snap2, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	// TWD: (32.0-31.5)/31.5*100 ≈ 1.587
	got := snap2.USD_TWD.ChangePct
	want := (32.0 - 31.5) / 31.5 * 100
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("USD/TWD.ChangePct = %v, want %v (diff %v)", got, want, diff)
	}

	// JPY: (148.0-150.0)/150.0*100 ≈ -1.333
	gotJ := snap2.JPY.ChangePct
	wantJ := (148.0 - 150.0) / 150.0 * 100
	if diff := gotJ - wantJ; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("JPY.ChangePct = %v, want %v (diff %v)", gotJ, wantJ, diff)
	}
}

// TestExchangeRateProvider_ZeroRateNoUpdate 驗證當 API 回傳 0 時不會污染 cache。
func TestExchangeRateProvider_ZeroRateNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := exchangeRateResponse{
			Result:   "success",
			BaseCode: "USD",
			Rates:    map[string]float64{"TWD": 0, "JPY": 150.0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewExchangeRateProvider()
	p.latestURL = srv.URL
	p.SetHTTPClient(srv.Client())

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// TWD rate 0 → 該欄位不寫入 snapshot（既有邏輯）
	if snap.USD_TWD.Value != 0 {
		t.Errorf("TWD.Value = %v, want 0", snap.USD_TWD.Value)
	}
	// JPY 正常
	if snap.JPY.Value != 150.0 {
		t.Errorf("JPY.Value = %v, want 150.0", snap.JPY.Value)
	}

	// Cache lastUSDJPY 應為 150.0（不會被 0 污染）
	if p.lastUSDJPY != 150.0 {
		t.Errorf("cache lastUSDJPY = %v, want 150.0", p.lastUSDJPY)
	}
	if p.lastUSDTWD != 0 {
		t.Errorf("cache lastUSDTWD = %v, want 0", p.lastUSDTWD)
	}
}
