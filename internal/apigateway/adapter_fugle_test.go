package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestFugleChannelAdapter_Metadata(t *testing.T) {
	a := &FugleChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "fugle" {
		t.Errorf("ChannelID = %q, want fugle", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "Fugle 富果" {
		t.Errorf("Platform = %q, want Fugle 富果", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "api.fugle.tw" {
		t.Errorf("Path = %q, want api.fugle.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestFugleChannelAdapter_Fetch(t *testing.T) {
	payload := marketdata.FugleQuoteResponse{
		Symbol:     "1476",
		ClosePrice: 42.0,
		OpenPrice:  41.0,
		HighPrice:  43.0,
		LowPrice:   40.0,
	}
	payload.Total.TradeVolume = 1000
	body, _ := json.Marshal(payload)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewFugleClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.fugle.tw"))

	adapter := NewFugleChannelAdapter(client)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "fugle" {
		t.Errorf("ChannelID = %q, want fugle", res.Meta.ChannelID)
	}
}

func TestFugleChannelAdapter_HealthCheck(t *testing.T) {
	payload := marketdata.FugleQuoteResponse{
		Symbol:     "1476",
		ClosePrice: 42.0,
	}
	body, _ := json.Marshal(payload)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewFugleClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.fugle.tw"))

	adapter := NewFugleChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestFugleChannelAdapter_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	writeParametersJSON(t, nil)
	client := marketdata.NewFugleClient("test-key")
	client.SetHTTPClient(withClientMockTransport(server, "api.fugle.tw"))

	adapter := NewFugleChannelAdapter(client)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestFugleChannelAdapter_RateLimit(t *testing.T) {
	writeParametersJSON(t, nil)
	client := marketdata.NewFugleClient("test-key")
	adapter := NewFugleChannelAdapter(client)
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
