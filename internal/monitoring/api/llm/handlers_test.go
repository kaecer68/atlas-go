package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corellm "github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// mockRouter implements corellm.Router for testing the health handler.
type mockRouter struct {
	healthMap map[corellm.Provider]corellm.HealthStatus
}

func (m *mockRouter) Call(_ context.Context, _ corellm.Request) (corellm.Response, error) {
	return corellm.Response{}, nil
}

func (m *mockRouter) Health() map[corellm.Provider]corellm.HealthStatus {
	return m.healthMap
}

func (m *mockRouter) Register(_ corellm.ProviderImpl) error {
	return nil
}

// TestHandleGetHealth_Healthy tests that GET /api/llm/health returns 200
// when providers are healthy and the response schema is correct.
func TestHandleGetHealth_Healthy(t *testing.T) {
	// Given: a router with two healthy providers
	lastSuccess := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	router := &mockRouter{
		healthMap: map[corellm.Provider]corellm.HealthStatus{
			corellm.ProviderKimi: {
				Provider:    corellm.ProviderKimi,
				Healthy:     true,
				LastError:   "",
				LastSuccess: lastSuccess,
				BreakerOpen: false,
			},
			corellm.ProviderDeepSeek: {
				Provider:    corellm.ProviderDeepSeek,
				Healthy:     true,
				LastError:   "",
				LastSuccess: lastSuccess,
				BreakerOpen: false,
			},
		},
	}
	h := NewHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/llm/health", nil)
	rec := httptest.NewRecorder()

	// When
	handler := shared.Get(h.HandleGetHealth)
	handler.ServeHTTP(rec, req)

	// Then: status 200
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check router_version
	if resp.RouterVersion != "v2.1" {
		t.Errorf("expected router_version 'v2.1', got %q", resp.RouterVersion)
	}

	// Check providers count
	if len(resp.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(resp.Providers))
	}

	// Check kimi provider
	kimi, ok := resp.Providers["kimi"]
	if !ok {
		t.Error("expected kimi in response providers")
	} else {
		if kimi.Provider != "kimi" {
			t.Errorf("expected provider 'kimi', got %q", kimi.Provider)
		}
		if !kimi.Healthy {
			t.Error("expected kimi to be healthy")
		}
		if kimi.BreakerOpen {
			t.Error("expected kimi breaker to be closed")
		}
	}

	// Check deepseek provider
	ds, ok := resp.Providers["deepseek"]
	if !ok {
		t.Error("expected deepseek in response providers")
	} else {
		if !ds.Healthy {
			t.Error("expected deepseek to be healthy")
		}
	}
}

// TestHandleGetHealth_Unhealthy tests that an unhealthy provider correctly
// reports its error state in the response.
func TestHandleGetHealth_Unhealthy(t *testing.T) {
	// Given: a router with an unhealthy provider
	router := &mockRouter{
		healthMap: map[corellm.Provider]corellm.HealthStatus{
			corellm.ProviderMiniMax: {
				Provider:    corellm.ProviderMiniMax,
				Healthy:     false,
				LastError:   "connection refused",
				LastSuccess: time.Time{},
				BreakerOpen: true,
			},
		},
	}
	h := NewHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/llm/health", nil)
	rec := httptest.NewRecorder()

	// When
	handler := shared.Get(h.HandleGetHealth)
	handler.ServeHTTP(rec, req)

	// Then
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	mm, ok := resp.Providers["minimax"]
	if !ok {
		t.Fatal("expected minimax in response providers")
	}
	if mm.Provider != "minimax" {
		t.Errorf("expected provider 'minimax', got %q", mm.Provider)
	}
	if mm.Healthy {
		t.Error("expected minimax to be unhealthy")
	}
	if mm.LastError != "connection refused" {
		t.Errorf("expected last_error 'connection refused', got %q", mm.LastError)
	}
	if !mm.BreakerOpen {
		t.Error("expected breaker_open to be true")
	}
}

// TestHandleGetHealth_Empty tests that GET /api/llm/health returns 200
// with an empty providers map when no providers are registered.
func TestHandleGetHealth_Empty(t *testing.T) {
	// Given: a router with no providers
	router := &mockRouter{
		healthMap: make(map[corellm.Provider]corellm.HealthStatus),
	}
	h := NewHandler(router)

	req := httptest.NewRequest(http.MethodGet, "/api/llm/health", nil)
	rec := httptest.NewRecorder()

	// When
	handler := shared.Get(h.HandleGetHealth)
	handler.ServeHTTP(rec, req)

	// Then
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Providers == nil {
		t.Error("expected non-nil providers in response")
	}
	if len(resp.Providers) != 0 {
		t.Errorf("expected empty providers, got %d entries", len(resp.Providers))
	}
	if resp.RouterVersion != "v2.1" {
		t.Errorf("expected router_version 'v2.1', got %q", resp.RouterVersion)
	}
}

// TestHandleGetHealth_MethodNotAllowed tests that POST to the GET-only
// endpoint returns 405.
func TestHandleGetHealth_MethodNotAllowed(t *testing.T) {
	// Given
	t.Setenv("ATLAS_API_KEY", "test-key")
	router := &mockRouter{
		healthMap: make(map[corellm.Provider]corellm.HealthStatus),
	}
	h := NewHandler(router)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/health", nil)
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()

	// When
	handler := shared.Get(h.HandleGetHealth)
	handler.ServeHTTP(rec, req)

	// Then
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// TestNewHandler_Nil tests that NewHandler returns nil when the router is nil.
func TestNewHandler_Nil(t *testing.T) {
	h := NewHandler(nil)
	if h != nil {
		t.Error("expected nil handler when router is nil")
	}
}
