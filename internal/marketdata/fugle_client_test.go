package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestFugleClient_SetHTTPClient(t *testing.T) {
	c := NewFugleClient("test-key")
	c.SetHTTPClient(nil)
	if c.httpClient != nil {
		t.Error("expected httpClient nil after SetHTTPClient(nil)")
	}
}

func TestFugleClient_RateLimiter(t *testing.T) {
	c := NewFugleClient("test-key")
	if c.RateLimiter() == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestGetFugleRateLimit(t *testing.T) {
	// Default free tier
	if got := getFugleRateLimit(); got != 60 {
		t.Errorf("getFugleRateLimit() = %d, want 60", got)
	}
}

func TestFugleClient_GetMeta_Success(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol: "0050",
		Name:   "元大台灣50",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/intraday/ticker/0050" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("X-API-KEY header missing or wrong")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	meta, err := c.GetMeta(context.Background(), "0050")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Symbol != "0050" {
		t.Errorf("Symbol = %q, want 0050", meta.Symbol)
	}
}

func TestFugleClient_CheckMarketStatus_Open(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol:         "0050",
		SecurityStatus: "NORMAL",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	open, err := c.CheckMarketStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !open {
		t.Error("expected market open")
	}
}

func TestFugleClient_CheckMarketStatus_Suspended(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol:         "0050",
		SecurityStatus: "SUSPENDED",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	open, err := c.CheckMarketStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open {
		t.Error("expected market not open when suspended")
	}
}

func TestNewFugleProviderWithClient(t *testing.T) {
	c := NewFugleClient("test-key")
	p := NewFugleProviderWithClient(c)
	if p.GetClient() != c {
		t.Error("expected provider client to equal injected client")
	}
}

func TestResetSharedFugleClient(t *testing.T) {
	ResetSharedFugleClient()
	c1 := GetSharedFugleClient("key1")
	ResetSharedFugleClient()
	c2 := GetSharedFugleClient("key2")
	if c1 == c2 {
		t.Error("expected different shared client instances after reset")
	}
}

// TestGetSharedFugleClient_SameInstance verifies the singleton returns the
// same *FugleClient (and thus the same rate limiter) across repeated calls —
// the invariant that keeps all Fugle call sites under one 60/min budget
// instead of each site minting its own limiter and blowing past the free
// tier (SK-22 Fugle audit).
func TestGetSharedFugleClient_SameInstance(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()
	c1 := GetSharedFugleClient("test-key")
	c2 := GetSharedFugleClient("test-key")
	if c1 != c2 {
		t.Fatal("GetSharedFugleClient must return the same instance across calls")
	}
	if c1.RateLimiter() != c2.RateLimiter() {
		t.Fatal("both calls must share the same rate limiter")
	}
}

// TestHybridProvider_UsesSharedFugleClient verifies NewHybridProvider wires
// the Fugle provider through the shared singleton client — one limiter for
// all Fugle call sites — rather than a per-instance NewFugleClient with its
// own token bucket (SK-22 Fugle audit regression guard).
func TestHybridProvider_UsesSharedFugleClient(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()
	p := NewHybridProvider("", "test-key")
	if p.fugleProvider == nil {
		t.Skip("fugle provider not configured (fubon proxy reachable?)")
	}
	got := p.fugleProvider.GetClient()
	shared := GetSharedFugleClient("test-key")
	if got != shared {
		t.Fatal("hybrid provider Fugle client must be the shared singleton")
	}
}
