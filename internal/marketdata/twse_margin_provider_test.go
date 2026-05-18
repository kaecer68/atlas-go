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

func TestExtractValueByFieldName(t *testing.T) {
	table := twseMarginTable{
		Fields: []string{"項目", "昨日餘額", "今日餘額"},
		Data: [][]string{
			{"header"},
			{"ignored"},
			{"合計", "123", "456"},
		},
	}

	value, ok := extractValueByFieldName(table, "今日餘額", 2)
	if !ok {
		t.Fatal("expected match")
	}
	if value != "456" {
		t.Fatalf("unexpected value: %s", value)
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
      "title": "信用交易統計",
      "fields": ["項目", "昨日餘額", "今日餘額"],
      "data": [
        ["header"],
        ["ignored"],
        ["合計", "100000", "120000"]
      ]
    },
    {
      "title": "融券餘額",
      "fields": ["項目", "昨日餘額", "今日餘額"],
      "data": [
        ["header"],
        ["ignored"],
        ["合計", "20000", "15000"]
      ]
    }
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()

	margin, short, marginChange, shortChange, err := p.fetchDateExpanded(context.Background(), "20260513")
	if err != nil {
		t.Fatalf("fetchDateExpanded failed: %v", err)
	}

	if margin != 1.2 {
		t.Fatalf("unexpected margin: %v", margin)
	}
	if short != 0.15 {
		t.Fatalf("unexpected short: %v", short)
	}
	if marginChange < 19.999 || marginChange > 20.001 {
		t.Fatalf("unexpected marginChange: %v", marginChange)
	}
	if shortChange < -25.001 || shortChange > -24.999 {
		t.Fatalf("unexpected shortChange: %v", shortChange)
	}
}

func TestTWSEMarginBalanceProvider_FetchSnapshotIncludesShortBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260513",
  "tables": [
    {
      "title": "信用交易統計",
      "fields": ["項目", "昨日餘額", "今日餘額"],
      "data": [
        ["header"],
        ["ignored"],
        ["合計", "100000", "120000"]
      ]
    },
    {
      "title": "融券餘額",
      "fields": ["項目", "昨日餘額", "今日餘額"],
      "data": [
        ["header"],
        ["ignored"],
        ["合計", "20000", "15000"]
      ]
    }
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()

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
}
