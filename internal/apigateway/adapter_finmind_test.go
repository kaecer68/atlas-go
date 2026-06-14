package apigateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestFinMindChannelAdapter_Metadata(t *testing.T) {
	a := &FinMindChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "finmind" {
		t.Errorf("ChannelID = %q, want finmind", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "FinMind" {
		t.Errorf("Platform = %q, want FinMind", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "api.finmindtrade.com" {
		t.Errorf("Path = %q, want api.finmindtrade.com", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestFinMindChannelAdapter_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("dataset"); got != "TaiwanStockPrice" {
			t.Errorf("dataset = %q, want TaiwanStockPrice", got)
		}
		if got := r.URL.Query().Get("data_id"); got != "2330" {
			t.Errorf("data_id = %q, want 2330", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[{"close":600.0,"open":595.0,"max":605.0,"min":590.0,"Trading_Volume":10000.0}]}`))
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	marketdata.ResetSharedFinMindClient()
	client := marketdata.NewFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.finmindtrade.com"))

	adapter := NewFinMindChannelAdapter(client)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "finmind" {
		t.Errorf("ChannelID = %q, want finmind", res.Meta.ChannelID)
	}
}

func TestFinMindChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[{"close":600.0}]}`))
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	marketdata.ResetSharedFinMindClient()
	client := marketdata.NewFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.finmindtrade.com"))

	adapter := NewFinMindChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestFinMindChannelAdapter_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	marketdata.ResetSharedFinMindClient()
	client := marketdata.NewFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.finmindtrade.com"))

	adapter := NewFinMindChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestFinMindChannelAdapter_RateLimit(t *testing.T) {
	writeParametersJSON(t, nil)
	marketdata.ResetSharedFinMindClient()
	client := marketdata.NewFinMindClient("test-key")
	adapter := NewFinMindChannelAdapter(client)
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
	if got := adapter.RateLimit().Limit(); fmt.Sprintf("%.6f", got) == "0.000000" {
		t.Errorf("RateLimit() returned zero limit")
	}
}
