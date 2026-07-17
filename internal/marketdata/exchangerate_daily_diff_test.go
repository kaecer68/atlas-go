package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestExchangeRateProvider_DailyDiffViaPersistedCache 驗證 PR-Layer2b：
// ExchangeRate 用跨日 persisted cache 比較昨天的 rate 計算 daily ChangePct。
// 修前 ChangePct 永遠 = 0（5-min window forex rate 多無變化）。
// 修後跨日（或重啟後）有 yesterday baseline → ChangePct 真實 daily diff。
func TestExchangeRateProvider_DailyDiffViaPersistedCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "exchange_rate_daily.json")

	// Seed persisted cache with "yesterday's" rates.
	if err := os.WriteFile(cachePath, []byte(`{
		"date": "2026-07-11",
		"usd_twd": 32.00,
		"usd_jpy": 161.50
	}`), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	// Today's API returns different rates.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(exchangeRateResponse{
			Result:   "success",
			BaseCode: "USD",
			Rates:    map[string]float64{"TWD": 32.20, "JPY": 161.87},
		})
	}))
	defer srv.Close()

	p := NewExchangeRateProvider()
	p.SetCachePath(cachePath)
	p.latestURL = srv.URL
	p.SetHTTPClient(srv.Client())

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}

	// TWD: (32.20 - 32.00) / 32.00 * 100 = 0.625%
	if got, want := snap.USD_TWD.ChangePct, 0.625; diff(got, want) > 0.01 {
		t.Errorf("USD_TWD.ChangePct = %v, want ~%v (daily diff via persisted cache)", got, want)
	}
	// JPY: (161.87 - 161.50) / 161.50 * 100 ≈ 0.229%
	if got, want := snap.JPY.ChangePct, 0.229; diff(got, want) > 0.01 {
		t.Errorf("JPY.ChangePct = %v, want ~%v (daily diff via persisted cache)", got, want)
	}

	// Verify cache file was updated with today's data.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache after fetch: %v", err)
	}
	var updated dailyRateSnapshot
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if updated.Date == "2026-07-11" {
		t.Errorf("cache.date should be today (2026-07-13ish), got %q (file not updated)", updated.Date)
	}
	if updated.USDTWD != 32.20 {
		t.Errorf("cache.USDTWD = %v, want 32.20", updated.USDTWD)
	}
}

// TestExchangeRateProvider_NoCacheColdStart 驗證沒有 cache file 時 ChangePct=0。
func TestExchangeRateProvider_NoCacheColdStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(exchangeRateResponse{
			Result:   "success",
			BaseCode: "USD",
			Rates:    map[string]float64{"TWD": 32.20, "JPY": 161.87},
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	p := NewExchangeRateProvider()
	p.SetCachePath(filepath.Join(tmpDir, "no_cache.json"))
	p.latestURL = srv.URL
	p.SetHTTPClient(srv.Client())

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.USD_TWD.ChangePct != 0 {
		t.Errorf("cold start USD_TWD.ChangePct = %v, want 0", snap.USD_TWD.ChangePct)
	}
	if snap.JPY.ChangePct != 0 {
		t.Errorf("cold start JPY.ChangePct = %v, want 0", snap.JPY.ChangePct)
	}
}

// TestExchangeRateProvider_SameDayIntraDayDiff 驗證同日第二個 fetch 走 5-min cache。
// httptest server handler 不能 mutate，所以第二個 fetch 用相同 rates，
// ChangePct 應為 0（同日無變化）。
func TestExchangeRateProvider_SameDayIntraDayDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(exchangeRateResponse{
			Result:   "success",
			BaseCode: "USD",
			Rates:    map[string]float64{"TWD": 32.20, "JPY": 161.87},
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	p := NewExchangeRateProvider()
	p.SetCachePath(filepath.Join(tmpDir, "intra_day.json"))
	p.latestURL = srv.URL
	p.SetHTTPClient(srv.Client())

	// First fetch on today: persists daily cache.
	_, _ = p.FetchSnapshot(context.Background())

	// Second fetch same day: 5-min cache comparison (rate unchanged → 0%).
	snap2, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if snap2.USD_TWD.ChangePct != 0 {
		t.Errorf("same-day intra-day USD_TWD.ChangePct = %v, want 0", snap2.USD_TWD.ChangePct)
	}
	if snap2.JPY.ChangePct != 0 {
		t.Errorf("same-day intra-day JPY.ChangePct = %v, want 0", snap2.JPY.ChangePct)
	}
}

func diff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}
