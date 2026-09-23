package apigateway

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"net/http"
	"net/http/httptest"

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

// TestBDIChannelAdapter_HealthCheck_EmptyQuoteIsWarn covers the 2026-09-20
// CNBC `.BADI` empty-quote outage for the health-check entry point: a quote
// that carries no price means the upstream is reachable but has no data, so the
// check must report "warn" (not "error") while still surfacing the reason.
// Genuine breakage keeps returning "error" — see
// TestBDIChannelAdapter_HealthCheck_RealFailureIsError.
func TestBDIChannelAdapter_HealthCheck_EmptyQuoteIsWarn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Production payload shape since 2026-09-20T08:35Z: no `last`.
		_, _ = w.Write([]byte(`{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","open":"0.00","high":"0.00","low":"0.00","provider":"CNBC Quote Cache"}]}}`))
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"bdi_endpoint":        map[string]any{"value": server.URL},
			"bdi_api_timeout_sec": map[string]any{"value": 10},
		},
	})

	old := marketdata.SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { marketdata.SetBDILimiterForTest(old) })

	provider := marketdata.NewBDIProvider()
	adapter := NewBDIChannelAdapter(provider)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck must still return the empty-quote error to the caller")
	}
	if status.Status != "warn" {
		t.Errorf("Status = %q, want warn (upstream answered; only the price is missing)", status.Status)
	}
	if !strings.Contains(status.LastError, "missing last price") {
		t.Errorf("LastError = %q, want it to carry the empty-quote reason", status.LastError)
	}
}

// TestBDIChannelAdapter_HealthCheck_RealFailureIsError guards the boundary: a
// transport/HTTP failure must keep reporting "error" so the fix cannot dampen
// real outages.
func TestBDIChannelAdapter_HealthCheck_RealFailureIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	writeParametersJSON(t, map[string]any{
		"marketdata": map[string]any{
			"bdi_endpoint":        map[string]any{"value": server.URL},
			"bdi_api_timeout_sec": map[string]any{"value": 10},
		},
	})

	old := marketdata.SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { marketdata.SetBDILimiterForTest(old) })

	provider := marketdata.NewBDIProvider()
	adapter := NewBDIChannelAdapter(provider)
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck must return the HTTP error")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error for a genuine HTTP failure", status.Status)
	}
}
