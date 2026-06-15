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
		APIVersion: "v0.3",
	}
	payload.Data.Info.Symbol = "0050"
	payload.Data.Meta.IsSuspended = false
	payload.Data.Meta.IsDelisted = false
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/intraday/meta" {
			t.Errorf("unexpected path: %s", r.URL.Path)
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
	if meta.Data.Info.Symbol != "0050" {
		t.Errorf("Symbol = %q, want 0050", meta.Data.Info.Symbol)
	}
}

func TestFugleClient_CheckMarketStatus_Open(t *testing.T) {
	payload := FugleMetaResponse{}
	payload.Data.Info.Symbol = "0050"
	payload.Data.Meta.IsSuspended = false
	payload.Data.Meta.IsDelisted = false
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
	payload := FugleMetaResponse{}
	payload.Data.Info.Symbol = "0050"
	payload.Data.Meta.IsSuspended = true
	payload.Data.Meta.IsDelisted = false
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
