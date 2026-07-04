package apigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestFubonChannelAdapter_Metadata(t *testing.T) {
	a := &FubonChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "fubon" {
		t.Errorf("ChannelID = %q, want fubon", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "富邦證券" {
		t.Errorf("Platform = %q, want 富邦證券", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.fubon.com.tw (via Python proxy)" {
		t.Errorf("Path = %q, want api.fubon.com.tw (via Python proxy)", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestFubonChannelAdapter_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"symbol":"2330","last":600.0,"open":595.0,"high":605.0,"low":590.0,"volume":10000,"is_open":true,"is_close":false},
			{"symbol":"0050","last":150.0,"open":149.0,"high":151.0,"low":148.0,"volume":5000,"is_open":true,"is_close":false}
		]`))
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"fubon_api_timeout_sec": map[string]any{"value": 10},
			"fubon_intraday_limit":  map[string]any{"value": 1000},
		},
	})

	client := marketdata.NewFubonClient()
	client.SetHTTPClient(withClientMockTransport(server, "fubon-proxy:18081"))

	adapter := NewFubonChannelAdapter(client)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "fubon" {
		t.Errorf("ChannelID = %q, want fubon", res.Meta.ChannelID)
	}
}

func TestFubonChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"fubon_api_timeout_sec": map[string]any{"value": 10},
			"fubon_intraday_limit":  map[string]any{"value": 1000},
		},
	})

	client := marketdata.NewFubonClient()
	client.SetHTTPClient(withClientMockTransport(server, "fubon-proxy:18081"))

	adapter := NewFubonChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestFubonChannelAdapter_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"fubon_api_timeout_sec": map[string]any{"value": 10},
			"fubon_intraday_limit":  map[string]any{"value": 1000},
		},
	})

	client := marketdata.NewFubonClient()
	client.SetHTTPClient(withClientMockTransport(server, "fubon-proxy:18081"))

	adapter := NewFubonChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error on failed health check")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestFubonChannelAdapter_RateLimit(t *testing.T) {
	writeParametersJSON(t, nil)
	client := marketdata.NewFubonClient()
	adapter := NewFubonChannelAdapter(client)
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
