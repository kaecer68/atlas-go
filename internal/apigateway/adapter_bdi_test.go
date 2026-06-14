package apigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestBDIChannelAdapter_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","last":"1234.00","change_pct":"2.15","last_time_msec":"1730000000000"}]}}`))
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"bdi_endpoint":        map[string]any{"value": server.URL},
			"bdi_api_timeout_sec": map[string]any{"value": 10},
		},
	})

	provider := marketdata.NewBDIProvider()
	adapter := NewBDIChannelAdapter(provider)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "bdi" {
		t.Errorf("ChannelID = %q, want bdi", res.Meta.ChannelID)
	}
}

func TestBDIChannelAdapter_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","last":"1234.00","change_pct":"2.15","last_time_msec":"1730000000000"}]}}`))
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"bdi_endpoint":        map[string]any{"value": server.URL},
			"bdi_api_timeout_sec": map[string]any{"value": 10},
		},
	})

	provider := marketdata.NewBDIProvider()
	adapter := NewBDIChannelAdapter(provider)
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestBDIChannelAdapter_RateLimit(t *testing.T) {
	writeParametersJSON(t, nil)
	provider := marketdata.NewBDIProvider()
	adapter := NewBDIChannelAdapter(provider)
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestBDIChannelAdapter_Metadata(t *testing.T) {
	a := &BDIChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "bdi" {
		t.Errorf("ChannelID = %q, want bdi", m.ChannelID)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}
