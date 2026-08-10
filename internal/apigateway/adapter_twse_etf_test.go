package apigateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTWSEETFChannelAdapter_Metadata(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	m := a.Metadata()
	if m.ChannelID != "twse_etf" {
		t.Errorf("ChannelID = %q, want twse_etf", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE" {
		t.Errorf("Platform = %q, want TWSE", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw/exchangeReport/TWT44U" {
		t.Errorf("Path = %q, want www.twse.com.tw/exchangeReport/TWT44U", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEETFChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	if a == nil {
		t.Fatal("NewTWSEETFChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

// A05：adapter 分流測試 — 只有 ErrETFNoTradingData 轉 stale，upstream/schema
// 錯誤必須以 error 回傳（觸發 circuit breaker）。
func TestTWSEETFChannelAdapter_Fetch_NoTradingDataIsStale(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"FAIL","date":"","tables":[]}`))
	}))
	defer server.Close()

	p := marketdata.NewTWSEETFProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	a := &TWSEETFChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v, want stale result for no-trading-data", err)
	}
	if !res.Stale {
		t.Error("Fetch() result should be marked Stale (holiday/no-data), not an error")
	}
}

func TestTWSEETFChannelAdapter_Fetch_Upstream403IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Forbidden"))
	}))
	defer server.Close()

	p := marketdata.NewTWSEETFProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	a := &TWSEETFChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	res, err := a.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() error = nil, want upstream error (403 must not masquerade as stale)")
	}
	if res != nil {
		t.Errorf("Fetch() result = %+v, want nil on error", res)
	}
	if !errors.Is(err, marketdata.ErrETFUpstream) {
		t.Errorf("Fetch() err = %v, want ErrETFUpstream", err)
	}
}

func TestTWSEETFChannelAdapter_Fetch_SchemaMismatchIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>WAF challenge</html>"))
	}))
	defer server.Close()

	p := marketdata.NewTWSEETFProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	a := &TWSEETFChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	_, err := a.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() error = nil, want schema error")
	}
	if !errors.Is(err, marketdata.ErrETFSchema) {
		t.Errorf("Fetch() err = %v, want ErrETFSchema", err)
	}
}

func TestTWSEETFChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260807","tables":[{"fields":["a","b","c","d"],"data":[["0050","1000","2000","300"]]}]}`))
	}))
	defer server.Close()

	p := marketdata.NewTWSEETFProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	a := &TWSEETFChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" || status.CheckType != "liveness" {
		t.Fatalf("HealthCheck() = %#v, want ok liveness", status)
	}
}
