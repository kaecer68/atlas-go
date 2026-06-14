package apigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTSMCRevenueChannelAdapter_Metadata(t *testing.T) {
	a := &TSMCRevenueChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "tsmc_revenue" {
		t.Errorf("ChannelID = %q, want tsmc_revenue", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE 台積電月營收" {
		t.Errorf("Platform = %q, want TWSE 台積電月營收", m.Platform)
	}
	if m.APIFormat != "REST JSON / FinMind TWT49U" {
		t.Errorf("APIFormat = %q, want REST JSON / FinMind TWT49U", m.APIFormat)
	}
	if m.Path != "api.finmindtrade.com / www.twse.com.tw" {
		t.Errorf("Path = %q, want api.finmindtrade.com / www.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTSMCRevenueChannelAdapter_RateLimit(t *testing.T) {
	a := NewTSMCRevenueChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTSMCRevenueChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestTSMCRevenueChannelAdapter_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"OK","status":200,"data":[{"revenue":1000000}]}`))
	}))
	defer server.Close()

	marketdata.ResetSharedFinMindClient()
	provider := marketdata.NewTSMCRevenueProviderWithStorage("test-key", t.TempDir())
	client := marketdata.GetSharedFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.finmindtrade.com"))

	a := NewTSMCRevenueChannelAdapter(provider)
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "tsmc_revenue" {
		t.Errorf("ChannelID = %q, want tsmc_revenue", res.Meta.ChannelID)
	}
}

func TestTSMCRevenueChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"OK","status":200,"data":[{"revenue":1000000}]}`))
	}))
	defer server.Close()

	marketdata.ResetSharedFinMindClient()
	provider := marketdata.NewTSMCRevenueProviderWithStorage("test-key", t.TempDir())
	client := marketdata.GetSharedFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.finmindtrade.com"))

	a := NewTSMCRevenueChannelAdapter(provider)
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}
