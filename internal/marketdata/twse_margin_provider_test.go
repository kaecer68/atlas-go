package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/time/rate"
)

func TestNewTWSEMarginBalanceProvider(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")
	if p.Name() != "twse_margin_balance" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.storageDir != "" {
		t.Fatalf("expected empty storageDir, got %s", p.storageDir)
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin(t *testing.T) {
	dir := t.TempDir()
	p := NewTWSEMarginBalanceProvider(dir)

	if err := p.saveMargin("20260513", 3500.5, 120.5, 1.25, -0.75); err != nil {
		t.Fatalf("saveMargin failed: %v", err)
	}

	fpath := filepath.Join(dir, "20260513_margin.json")
	data, err := os.ReadFile(fpath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if result["date"] != "20260513" {
		t.Fatalf("unexpected date: %v", result["date"])
	}
	if result["margin_balance"] != 3500.5 {
		t.Fatalf("unexpected margin_balance: %v", result["margin_balance"])
	}
	if result["change_pct"] != 1.25 {
		t.Fatalf("unexpected change_pct: %v", result["change_pct"])
	}
	if result["short_balance"] != 120.5 {
		t.Fatalf("unexpected short_balance: %v", result["short_balance"])
	}
	if result["short_change_pct"] != -0.75 {
		t.Fatalf("unexpected short_change_pct: %v", result["short_change_pct"])
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin_EmptyDir(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")

	if err := p.saveMargin("20260513", 3500.5, 120.5, 1.25, -0.75); err != nil {
		t.Fatalf("saveMargin with empty dir should not error: %v", err)
	}
}

func TestTWSEMarginBalanceProvider_FetchDateExpanded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("date") != "20260513" {
			t.Fatalf("unexpected date: %s", r.URL.Query().Get("date"))
		}
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260513",
  "tables": [
    {
      "title": "115年05月13日 信用交易統計",
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
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	margin, short, marginChange, shortChange, err := p.fetchDateExpanded(context.Background(), "20260513")
	if err != nil {
		t.Fatalf("fetchDateExpanded failed: %v", err)
	}

	// 融資金額: 120,000,000 仟元 / 1e5 = 1200.0
	if margin != 1200.0 {
		t.Fatalf("unexpected margin: %v (want 1200.0)", margin)
	}
	// 融券: 239,205 交易單位 / 1e5 = 2.39205
	if short != 2.39205 {
		t.Fatalf("unexpected short: %v (want 2.39205)", short)
	}
	// marginChange: (1200-1000)/1000*100 = 20%
	if marginChange < 19.9 || marginChange > 20.1 {
		t.Fatalf("unexpected marginChange: %v (want ~20)", marginChange)
	}
	// shortChange: (2.39205-2.39437)/2.39437*100 ≈ -0.097% (239437/1e5 = 2.39437)
	if shortChange <= -1.0 || shortChange >= 1.0 {
		t.Fatalf("unexpected shortChange: %v (want near 0)", shortChange)
	}
}

func TestTWSEMarginBalanceProvider_FetchSnapshotIncludesShortBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260513",
  "tables": [
    {
      "title": "115年05月13日 信用交易統計",
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
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snap.RetailMarginBalance.Symbol != "TAIWAN_MARGIN_BALANCE" {
		t.Fatalf("unexpected margin symbol: %s", snap.RetailMarginBalance.Symbol)
	}
	if snap.RetailShortBalance.Symbol != "TAIWAN_SHORT_BALANCE" {
		t.Fatalf("unexpected short symbol: %s", snap.RetailShortBalance.Symbol)
	}
	if snap.RetailShortBalance.Value <= 0 {
		t.Fatalf("expected positive short_balance, got %v", snap.RetailShortBalance.Value)
	}
}
