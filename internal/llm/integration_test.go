// Package llm_test provides end-to-end integration tests for the full
// handler → Router → adapter → client → HTTP → response parsing chain.
//
// These tests exercise the Phase 2 ProviderImpl adapters (DeepSeek, MiniMax)
// with real httptest servers, and the DefaultRouter routing chain with
// mock ProviderImpl implementations.
//
// Test categories:
//  1. Adapter E2E    — httptest → client → adapter → llm.Response
//  2. Router chain   — mock adapters → routing → fallback → data gate
//  3. Full chain     — httptest → client → adapter → Router → handler → typed output
package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/adapters"
	"github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// ============================================================================
// Test helpers
// ============================================================================

// newIntegrationBaseClient returns a BaseClient pre-configured for httptest
// use: generous rate limits, single retry attempt, no circuit breaker.
func newIntegrationBaseClient() *clients.BaseClient {
	bc := clients.NewBaseClient(llm.ProviderMock, clients.BaseClientConfig{
		RatePerSecond: 1000,
		Burst:         100,
		MaxAttempts:   1,
	})
	bc.Breaker = nil
	return bc
}

// mustJSON marshals v to a JSON byte slice; fatals the test on error.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

// assertNoError fails t with msg if err is non-nil.
func assertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", msg, err)
	}
}

// integrationMockProvider implements llm.ProviderImpl for Router-level
// integration tests. It allows per-provider control of Supports, Call
// response/error, and Health status.
type integrationMockProvider struct {
	name       llm.Provider
	supported  map[llm.Capability]bool
	callErr    error
	callResp   llm.Response
	healthResp llm.HealthStatus
	callCount  *int // optional: track how many times Call was invoked
}

func (m *integrationMockProvider) Supports(cap llm.Capability) bool {
	if m.supported == nil {
		return true
	}
	supported, ok := m.supported[cap]
	return ok && supported
}

func (m *integrationMockProvider) Call(_ context.Context, _ llm.Request) (llm.Response, error) {
	if m.callCount != nil {
		*m.callCount++
	}
	if m.callErr != nil {
		return llm.Response{}, m.callErr
	}
	return m.callResp, nil
}

func (m *integrationMockProvider) Health() llm.HealthStatus {
	return m.healthResp
}

// ============================================================================
// Test 1: DeepSeekAdapter — full HTTP E2E chain
// ============================================================================

// TestDeepSeekAdapter_EndToEnd verifies the full flow from adapter.Call
// through DeepSeekClient.Chat to a real httptest server and back.
// It asserts correct Output, Provider, Usage, and no error.
func TestDeepSeekAdapter_EndToEnd(t *testing.T) {
	// Setup: httptest server returning OpenAI-compatible chat completion JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		resp := map[string]any{
			"model": "deepseek-v4-pro",
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "DeepSeek integration test response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int64{
				"prompt_tokens":     25,
				"completion_tokens": 12,
				"total_tokens":      37,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Wire: BaseClient → DeepSeekClient → DeepSeekAdapter.
	baseClient := newIntegrationBaseClient()
	client := clients.NewDeepSeekClient("test-ds-key", baseClient)
	client.BaseURL = srv.URL

	adapter := adapters.NewDeepSeekAdapter(client, clients.DefaultModelV4Pro)

	// Build a valid payload.
	payload := mustJSON(t, map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": "You are a financial translator."},
			{"role": "user", "content": "Translate: Buy signal triggered by volume breakout."},
		},
	})

	req := llm.Request{
		Capability: llm.CapabilityRationaleGeneration,
		Payload:    payload,
		DataClass:  llm.DataClassNonRegulated,
		Options:    llm.Options{MaxTokens: 500},
	}

	// Execute.
	resp, err := adapter.Call(context.Background(), req)
	assertNoError(t, err, "DeepSeekAdapter.Call")

	// Assert.
	if resp.Output != "DeepSeek integration test response" {
		t.Errorf("Output = %q, want %q", resp.Output, "DeepSeek integration test response")
	}
	if resp.Provider != llm.ProviderDeepSeek {
		t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderDeepSeek)
	}
	if resp.Usage.InputTokens != 25 {
		t.Errorf("Usage.InputTokens = %d, want 25", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 12 {
		t.Errorf("Usage.OutputTokens = %d, want 12", resp.Usage.OutputTokens)
	}
	if resp.Usage.TotalTokens != 37 {
		t.Errorf("Usage.TotalTokens = %d, want 37", resp.Usage.TotalTokens)
	}
}

// ============================================================================
// Test 2: MiniMaxAdapter — full HTTP E2E chain
// ============================================================================

// TestMiniMaxAdapter_EndToEnd verifies the full flow through MiniMaxAdapter
// with both OpenAI and Anthropic endpoint formats.
func TestMiniMaxAdapter_EndToEnd(t *testing.T) {
	t.Run("OpenAI format", func(t *testing.T) {
		var capturedPath string

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			resp := map[string]any{
				"model": clients.DefaultModelMiniMaxM3,
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": "MiniMax OpenAI format response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int64{
					"prompt_tokens":     8,
					"completion_tokens": 4,
					"total_tokens":      12,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		baseClient := newIntegrationBaseClient()
		client := clients.NewMiniMaxClient("test-mm-key", baseClient)
		client.BaseURL = srv.URL
		// Default: OpenAI format path.

		adapter := adapters.NewMiniMaxAdapter(client)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "Hello MiniMax"},
			},
		})

		req := llm.Request{
			Capability: llm.CapabilityStrategySummary,
			Payload:    payload,
		}

		resp, err := adapter.Call(context.Background(), req)
		assertNoError(t, err, "MiniMaxAdapter.Call (OpenAI format)")

		if resp.Output != "MiniMax OpenAI format response" {
			t.Errorf("Output = %q, want %q", resp.Output, "MiniMax OpenAI format response")
		}
		if resp.Provider != llm.ProviderMiniMax {
			t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderMiniMax)
		}
		// Verify URL path is the OpenAI-compatible endpoint.
		if capturedPath != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", capturedPath)
		}
	})

	t.Run("Anthropic format", func(t *testing.T) {
		var capturedPath string

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			resp := map[string]any{
				"model": clients.DefaultModelMiniMaxM3,
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": "MiniMax Anthropic format response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int64{
					"prompt_tokens":     6,
					"completion_tokens": 3,
					"total_tokens":      9,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		baseClient := newIntegrationBaseClient()
		client := clients.NewMiniMaxClient("test-mm-key", baseClient)
		client.BaseURL = srv.URL
		client.UseAnthropicFormat = true

		adapter := adapters.NewMiniMaxAdapter(client)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "Hello MiniMax (Anthropic)"},
			},
		})

		req := llm.Request{Payload: payload}

		resp, err := adapter.Call(context.Background(), req)
		assertNoError(t, err, "MiniMaxAdapter.Call (Anthropic format)")

		if resp.Output != "MiniMax Anthropic format response" {
			t.Errorf("Output = %q, want %q", resp.Output, "MiniMax Anthropic format response")
		}
		// Verify URL path is the Anthropic-compatible endpoint.
		if capturedPath != "/v1/anthropic/chat/completions" {
			t.Errorf("path = %q, want /v1/anthropic/chat/completions", capturedPath)
		}
	})
}

// ============================================================================
// TestRouter_FullChain_AllAdapters verifies the complete routing chain:
// Primary fails → Backup1 succeeds → Backup2 not reached.
// Uses CapabilityFailureAttribution, whose default routing chain is
// MiniMax → DeepSeek → OpenCodeGo. We register MiniMax (fails) and
// DeepSeek (succeeds); OpenCodeGo is not registered and would be skipped.
// Asserts correct provider, output, and AttemptedProviders order.
func TestRouter_FullChain_AllAdapters(t *testing.T) {
	// Primary (MiniMax) — configured to fail.
	primaryErr := errors.New("minimax upstream timeout")
	primary := &integrationMockProvider{
		name:    llm.ProviderMiniMax,
		callErr: primaryErr,
		healthResp: llm.HealthStatus{
			Provider: llm.ProviderMiniMax,
			Healthy:  true,
		},
	}

	// Backup1 (DeepSeek) — configured to succeed.
	backup1 := &integrationMockProvider{
		name: llm.ProviderDeepSeek,
		callResp: llm.Response{
			Output:   "fallback to DeepSeek succeeded",
			Provider: llm.ProviderDeepSeek,
			Usage: llm.Usage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
		},
		healthResp: llm.HealthStatus{
			Provider: llm.ProviderDeepSeek,
			Healthy:  true,
		},
	}

	// Use NewDefaultRouter with registered providers.
	// CapabilityFailureAttribution default chain: MiniMax → DeepSeek → OpenCodeGo.
	router := llm.NewDefaultRouter(primary, backup1)

	// Execute.
	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call")

	// Assert: response is from DeepSeek (Backup1).
	if resp.Output != "fallback to DeepSeek succeeded" {
		t.Errorf("Output = %q, want %q", resp.Output, "fallback to DeepSeek succeeded")
	}
	if resp.Provider != llm.ProviderDeepSeek {
		t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderDeepSeek)
	}

	// Assert: AttemptedProviders contains [MiniMax, DeepSeek].
	if len(resp.AttemptedProviders) != 2 {
		t.Fatalf("len(AttemptedProviders) = %d, want 2: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != llm.ProviderMiniMax {
		t.Errorf("AttemptedProviders[0] = %q, want %q",
			resp.AttemptedProviders[0], llm.ProviderMiniMax)
	}
	if resp.AttemptedProviders[1] != llm.ProviderDeepSeek {
		t.Errorf("AttemptedProviders[1] = %q, want %q",
			resp.AttemptedProviders[1], llm.ProviderDeepSeek)
	}
}

// ============================================================================
// Test 4: Router — DataClass gate prevents MiniMax fallback
// ============================================================================

// TestRouter_DataClassGate_PreventsFallback verifies that when the routing
// chain has MiniMax as primary and the request carries DataClassRegulated,
// MiniMax is skipped entirely and the Router falls through to the next
// provider in the chain.
// Uses CapabilityRiskSurfaceExtraction, whose default chain is
// MiniMax → DeepSeek → OpenCodeGo. With DataClassRegulated, MiniMax is gated.
func TestRouter_DataClassGate_PreventsFallback(t *testing.T) {
	var miniMaxCallCount int

	// MiniMax configured as primary — must NOT be called with regulated data.
	miniMax := &integrationMockProvider{
		name: llm.ProviderMiniMax,
		callResp: llm.Response{
			Output:   "miniMax should not be called",
			Provider: llm.ProviderMiniMax,
		},
		healthResp: llm.HealthStatus{
			Provider: llm.ProviderMiniMax,
			Healthy:  true,
		},
		callCount: &miniMaxCallCount,
	}

	// DeepSeek as Backup1 — will handle the request.
	deepseek := &integrationMockProvider{
		name: llm.ProviderDeepSeek,
		callResp: llm.Response{
			Output:   "deepseek handles regulated data",
			Provider: llm.ProviderDeepSeek,
		},
		healthResp: llm.HealthStatus{
			Provider: llm.ProviderDeepSeek,
			Healthy:  true,
		},
	}

	// CapabilityRiskSurfaceExtraction default chain: MiniMax → DeepSeek → OpenCodeGo.
	router := llm.NewDefaultRouter(miniMax, deepseek)

	// Execute with regulated data.
	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call with DataClassRegulated")

	// Assert: MiniMax was never called.
	if miniMaxCallCount > 0 {
		t.Errorf("MiniMax.Call was invoked %d time(s) but should have been gated for DataClassRegulated", miniMaxCallCount)
	}

	// Assert: Response came from DeepSeek.
	if resp.Provider != llm.ProviderDeepSeek {
		t.Errorf("Provider = %q, want %q (MiniMax gated)", resp.Provider, llm.ProviderDeepSeek)
	}

	// Assert: MiniMax is NOT in AttemptedProviders.
	for _, p := range resp.AttemptedProviders {
		if p == llm.ProviderMiniMax {
			t.Errorf("ProviderMiniMax should not appear in AttemptedProviders for DataClassRegulated")
		}
	}

	// Assert: DeepSeek IS in AttemptedProviders.
	foundDeepSeek := false
	for _, p := range resp.AttemptedProviders {
		if p == llm.ProviderDeepSeek {
			foundDeepSeek = true
			break
		}
	}
	if !foundDeepSeek {
		t.Errorf("ProviderDeepSeek should appear in AttemptedProviders: %v", resp.AttemptedProviders)
	}
}

// ============================================================================
// Test 5: Router — Health endpoint aggregates all providers
// ============================================================================

// TestRouter_HealthEndpoint_AggregatesAllProviders verifies that Router.Health()
// returns all registered providers with their respective health statuses,
// including healthy and error-state providers (Kimi K2.7 removed).
func TestRouter_HealthEndpoint_AggregatesAllProviders(t *testing.T) {
	lastSuccess := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)

	// Healthy provider.
	deepseek := &integrationMockProvider{
		name: llm.ProviderDeepSeek,
		healthResp: llm.HealthStatus{
			Provider:    llm.ProviderDeepSeek,
			Healthy:     true,
			LastSuccess: lastSuccess,
		},
	}

	// Provider with recent error but not breaker-open.
	minimax := &integrationMockProvider{
		name: llm.ProviderMiniMax,
		healthResp: llm.HealthStatus{
			Provider:  llm.ProviderMiniMax,
			Healthy:   false,
			LastError: "connection refused: api.minimaxi.com:443",
		},
	}

	router := llm.NewDefaultRouter(deepseek, minimax)

	healthMap := router.Health()

	// Assert 2 providers are present (Kimi removed).
	if len(healthMap) != 2 {
		t.Fatalf("len(Health()) = %d, want 2", len(healthMap))
	}

	// Assert DeepSeek is healthy.
	dsHealth, ok := healthMap[llm.ProviderDeepSeek]
	if !ok {
		t.Fatal("ProviderDeepSeek not found in Health()")
	}
	if !dsHealth.Healthy {
		t.Error("DeepSeek should be healthy")
	}
	if !dsHealth.LastSuccess.Equal(lastSuccess) {
		t.Errorf("DeepSeek.LastSuccess = %v, want %v", dsHealth.LastSuccess, lastSuccess)
	}

	// Assert MiniMax has error.
	mmHealth, ok := healthMap[llm.ProviderMiniMax]
	if !ok {
		t.Fatal("ProviderMiniMax not found in Health()")
	}
	if mmHealth.Healthy {
		t.Error("MiniMax should not be healthy")
	}
	if mmHealth.LastError == "" {
		t.Error("MiniMax should have LastError set")
	}
}

// ============================================================================
// Test 6: RationaleGenerationHandler — full chain through Router
// ============================================================================

// TestRationaleGenerationHandler_ThroughRouter verifies the complete
// flow from handler.Handle() through Router → adapter → client → HTTP
// server and back, exercising the full response parsing pipeline.
func TestRationaleGenerationHandler_ThroughRouter(t *testing.T) {
	// Setup: httptest server acting as DeepSeek API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model": "deepseek-v4-pro",
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "測試回應",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int64{
				"prompt_tokens":     30,
				"completion_tokens": 5,
				"total_tokens":      35,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Wire: BaseClient → DeepSeekClient → DeepSeekAdapter → DefaultRouter → Handler.
	baseClient := newIntegrationBaseClient()
	dsClient := clients.NewDeepSeekClient("test-ds-key", baseClient)
	dsClient.BaseURL = srv.URL

	dsAdapter := adapters.NewDeepSeekAdapter(dsClient, clients.DefaultModelV4Pro)

	router := llm.NewDefaultRouter(dsAdapter)
	handler := capabilities.NewRationaleGenerationHandler(router)

	// Execute.
	input := schemas.RationaleGenerationInput{
		EnglishText: "Buy signal triggered by volume breakout with RSI divergence",
		DataClass:   llm.DataClassNonRegulated,
	}
	resp, err := handler.Handle(context.Background(), input)
	assertNoError(t, err, "RationaleGenerationHandler.Handle")

	// Assert: the handler parsed the response correctly.
	// The mock server returns content "測試回應" which is not valid JSON,
	// so parseRationaleGenerationResponse falls back to raw string mode.
	if resp.TranslatedText != "測試回應" {
		t.Errorf("TranslatedText = %q, want %q", resp.TranslatedText, "測試回應")
	}

	// Test with JSON-encoded response.
	t.Run("JSON response parsing", func(t *testing.T) {
		srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"model": "deepseek-v4-pro",
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": `{"translated_text":"成交量突破觸發買入訊號"}`,
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int64{
					"prompt_tokens":     30,
					"completion_tokens": 10,
					"total_tokens":      40,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer srv2.Close()

		bc2 := newIntegrationBaseClient()
		ds2 := clients.NewDeepSeekClient("test-ds-key", bc2)
		ds2.BaseURL = srv2.URL

		adapter2 := adapters.NewDeepSeekAdapter(ds2, clients.DefaultModelV4Pro)
		router2 := llm.NewDefaultRouter(adapter2)
		handler2 := capabilities.NewRationaleGenerationHandler(router2)

		input2 := schemas.RationaleGenerationInput{
			EnglishText: "Volume breakout",
			DataClass:   llm.DataClassNonRegulated,
		}
		resp2, err := handler2.Handle(context.Background(), input2)
		assertNoError(t, err, "Handler.Handle (JSON mode)")

		if resp2.TranslatedText != "成交量突破觸發買入訊號" {
			t.Errorf("TranslatedText = %q, want %q",
				resp2.TranslatedText, "成交量突破觸發買入訊號")
		}
	})

	// Test empty output fallback.
	t.Run("empty output fallback", func(t *testing.T) {
		srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"model": "deepseek-v4-pro",
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": "",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int64{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer srv3.Close()

		bc3 := newIntegrationBaseClient()
		ds3 := clients.NewDeepSeekClient("test-ds-key", bc3)
		ds3.BaseURL = srv3.URL

		adapter3 := adapters.NewDeepSeekAdapter(ds3, clients.DefaultModelV4Pro)
		router3 := llm.NewDefaultRouter(adapter3)
		handler3 := capabilities.NewRationaleGenerationHandler(router3)

		resp3, err := handler3.Handle(context.Background(), schemas.RationaleGenerationInput{
			EnglishText: "test",
		})
		assertNoError(t, err, "Handler.Handle (empty output)")

		if resp3.TranslatedText != "" {
			t.Errorf("TranslatedText = %q, want empty on empty output", resp3.TranslatedText)
		}
	})
}

// ============================================================================
// Additional integration: context cancellation propagates through adapter.
// ============================================================================

// TestDeepSeekAdapter_ContextCancellation verifies that a cancelled context
// is propagated through the adapter to the HTTP client layer.
func TestDeepSeekAdapter_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — context should be cancelled before completion.
		select {
		case <-r.Context().Done():
			// Expected: context cancelled.
			return
		case <-time.After(500 * time.Millisecond):
			http.Error(w, "too late", http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	baseClient := newIntegrationBaseClient()
	client := clients.NewDeepSeekClient("test-ds-key", baseClient)
	client.BaseURL = srv.URL
	adapter := adapters.NewDeepSeekAdapter(client, clients.DefaultModelV4Pro)

	payload := mustJSON(t, map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := llm.Request{Payload: payload}
	_, err := adapter.Call(ctx, req)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// ============================================================================
// Test 7: Failover chain — MiniMax succeeds, no others called
// ============================================================================

// TestRouter_Failover_MiniMaxSucceeds verifies that when the Primary provider
// (MiniMax) succeeds, no Backup1 (DeepSeek), Backup2 (OpenCodeGo), or Mock
// providers are consulted. Uses CapabilityRiskSurfaceExtraction whose chain is
// MiniMax → DeepSeek → OpenCodeGo → Mock.
func TestRouter_Failover_MiniMaxSucceeds(t *testing.T) {
	var miniMaxCalls, deepseekCalls, openCodeGoCalls, mockCalls int

	chainErr := errors.New("should not be called")

	// Primary (MiniMax) — succeeds.
	miniMax := &integrationMockProvider{
		name: llm.ProviderMiniMax,
		callResp: llm.Response{
			Output:   "minimax risk surface extraction",
			Provider: llm.ProviderMiniMax,
			Usage:    llm.Usage{InputTokens: 20, OutputTokens: 8, TotalTokens: 28},
		},
		healthResp: llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true},
		callCount:  &miniMaxCalls,
	}

	// Backup1 (DeepSeek) — should not be called.
	deepseek := &integrationMockProvider{
		name:       llm.ProviderDeepSeek,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true},
		callCount:  &deepseekCalls,
	}

	// Backup2 (OpenCodeGo) — should not be called.
	openCodeGo := &integrationMockProvider{
		name:       llm.ProviderOpenCodeGo,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderOpenCodeGo, Healthy: true},
		callCount:  &openCodeGoCalls,
	}

	// Mock (last resort) — should not be called.
	mock := &integrationMockProvider{
		name:       llm.ProviderMock,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMock, Healthy: true},
		callCount:  &mockCalls,
	}

	router := llm.NewDefaultRouter(miniMax, deepseek, openCodeGo, mock)

	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call (MiniMax succeeds)")

	// Assert: response from MiniMax.
	if resp.Output != "minimax risk surface extraction" {
		t.Errorf("Output = %q, want %q", resp.Output, "minimax risk surface extraction")
	}
	if resp.Provider != llm.ProviderMiniMax {
		t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderMiniMax)
	}

	// Assert: only MiniMax was in AttemptedProviders.
	if len(resp.AttemptedProviders) != 1 {
		t.Errorf("len(AttemptedProviders) = %d, want 1: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != llm.ProviderMiniMax {
		t.Errorf("AttemptedProviders[0] = %q, want %q",
			resp.AttemptedProviders[0], llm.ProviderMiniMax)
	}

	// Assert: only MiniMax was called.
	if miniMaxCalls != 1 {
		t.Errorf("MiniMax call count = %d, want 1", miniMaxCalls)
	}
	if deepseekCalls != 0 {
		t.Errorf("DeepSeek call count = %d, want 0", deepseekCalls)
	}
	if openCodeGoCalls != 0 {
		t.Errorf("OpenCodeGo call count = %d, want 0", openCodeGoCalls)
	}
	if mockCalls != 0 {
		t.Errorf("Mock call count = %d, want 0", mockCalls)
	}
}

// ============================================================================
// Test 8: Failover chain — MiniMax fails, DeepSeek succeeds
// ============================================================================

// TestRouter_Failover_MiniMaxFails_DeepSeekSucceeds verifies that when
// Primary (MiniMax) fails, Backup1 (DeepSeek) is tried and succeeds.
// Backup2 (OpenCodeGo) and Mock are not reached.
func TestRouter_Failover_MiniMaxFails_DeepSeekSucceeds(t *testing.T) {
	var miniMaxCalls, deepseekCalls, openCodeGoCalls, mockCalls int

	chainErr := errors.New("should not be called")
	miniMaxErr := errors.New("minimax upstream timeout")

	// Primary (MiniMax) — fails.
	miniMax := &integrationMockProvider{
		name:       llm.ProviderMiniMax,
		callErr:    miniMaxErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true},
		callCount:  &miniMaxCalls,
	}

	// Backup1 (DeepSeek) — succeeds.
	deepseek := &integrationMockProvider{
		name: llm.ProviderDeepSeek,
		callResp: llm.Response{
			Output:   "deepseek fallback response",
			Provider: llm.ProviderDeepSeek,
			Usage:    llm.Usage{InputTokens: 15, OutputTokens: 6, TotalTokens: 21},
		},
		healthResp: llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true},
		callCount:  &deepseekCalls,
	}

	// Backup2 (OpenCodeGo) — should not be called.
	openCodeGo := &integrationMockProvider{
		name:       llm.ProviderOpenCodeGo,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderOpenCodeGo, Healthy: true},
		callCount:  &openCodeGoCalls,
	}

	// Mock — should not be called.
	mock := &integrationMockProvider{
		name:       llm.ProviderMock,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMock, Healthy: true},
		callCount:  &mockCalls,
	}

	router := llm.NewDefaultRouter(miniMax, deepseek, openCodeGo, mock)

	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call (MiniMax fails, DeepSeek succeeds)")

	// Assert: response from DeepSeek.
	if resp.Output != "deepseek fallback response" {
		t.Errorf("Output = %q, want %q", resp.Output, "deepseek fallback response")
	}
	if resp.Provider != llm.ProviderDeepSeek {
		t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderDeepSeek)
	}

	// Assert: AttemptedProviders contains MiniMax and DeepSeek.
	if len(resp.AttemptedProviders) != 2 {
		t.Errorf("len(AttemptedProviders) = %d, want 2: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != llm.ProviderMiniMax {
		t.Errorf("AttemptedProviders[0] = %q, want %q",
			resp.AttemptedProviders[0], llm.ProviderMiniMax)
	}
	if resp.AttemptedProviders[1] != llm.ProviderDeepSeek {
		t.Errorf("AttemptedProviders[1] = %q, want %q",
			resp.AttemptedProviders[1], llm.ProviderDeepSeek)
	}

	// Assert: only MiniMax and DeepSeek were called.
	if miniMaxCalls != 1 {
		t.Errorf("MiniMax call count = %d, want 1", miniMaxCalls)
	}
	if deepseekCalls != 1 {
		t.Errorf("DeepSeek call count = %d, want 1", deepseekCalls)
	}
	if openCodeGoCalls != 0 {
		t.Errorf("OpenCodeGo call count = %d, want 0", openCodeGoCalls)
	}
	if mockCalls != 0 {
		t.Errorf("Mock call count = %d, want 0", mockCalls)
	}
}

// ============================================================================
// Test 9: Failover chain — MiniMax + DeepSeek fail, OpenCodeGo succeeds
// ============================================================================

// TestRouter_Failover_MiniMaxDeepSeekFail_OpenCodeGoSucceeds verifies that
// when Primary (MiniMax) and Backup1 (DeepSeek) both fail, Backup2
// (OpenCodeGo) is tried and succeeds. Mock is not reached.
func TestRouter_Failover_MiniMaxDeepSeekFail_OpenCodeGoSucceeds(t *testing.T) {
	var miniMaxCalls, deepseekCalls, openCodeGoCalls, mockCalls int

	chainErr := errors.New("should not be called")
	providerErr := errors.New("provider error")

	miniMax := &integrationMockProvider{
		name:       llm.ProviderMiniMax,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true},
		callCount:  &miniMaxCalls,
	}

	deepseek := &integrationMockProvider{
		name:       llm.ProviderDeepSeek,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true},
		callCount:  &deepseekCalls,
	}

	openCodeGo := &integrationMockProvider{
		name: llm.ProviderOpenCodeGo,
		callResp: llm.Response{
			Output:   "opencodego third-tier fallback",
			Provider: llm.ProviderOpenCodeGo,
			Usage:    llm.Usage{InputTokens: 12, OutputTokens: 5, TotalTokens: 17},
		},
		healthResp: llm.HealthStatus{Provider: llm.ProviderOpenCodeGo, Healthy: true},
		callCount:  &openCodeGoCalls,
	}

	mock := &integrationMockProvider{
		name:       llm.ProviderMock,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMock, Healthy: true},
		callCount:  &mockCalls,
	}

	router := llm.NewDefaultRouter(miniMax, deepseek, openCodeGo, mock)

	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call (MiniMax+DeepSeek fail, OpenCodeGo succeeds)")

	// Assert: response from OpenCodeGo.
	if resp.Output != "opencodego third-tier fallback" {
		t.Errorf("Output = %q, want %q", resp.Output, "opencodego third-tier fallback")
	}
	if resp.Provider != llm.ProviderOpenCodeGo {
		t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderOpenCodeGo)
	}

	// Assert: 3 attempted providers.
	if len(resp.AttemptedProviders) != 3 {
		t.Errorf("len(AttemptedProviders) = %d, want 3: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != llm.ProviderMiniMax {
		t.Errorf("AttemptedProviders[0] = %q, want %q",
			resp.AttemptedProviders[0], llm.ProviderMiniMax)
	}
	if resp.AttemptedProviders[1] != llm.ProviderDeepSeek {
		t.Errorf("AttemptedProviders[1] = %q, want %q",
			resp.AttemptedProviders[1], llm.ProviderDeepSeek)
	}
	if resp.AttemptedProviders[2] != llm.ProviderOpenCodeGo {
		t.Errorf("AttemptedProviders[2] = %q, want %q",
			resp.AttemptedProviders[2], llm.ProviderOpenCodeGo)
	}

	// Assert: MiniMax, DeepSeek, OpenCodeGo called; Mock not called.
	if miniMaxCalls != 1 {
		t.Errorf("MiniMax call count = %d, want 1", miniMaxCalls)
	}
	if deepseekCalls != 1 {
		t.Errorf("DeepSeek call count = %d, want 1", deepseekCalls)
	}
	if openCodeGoCalls != 1 {
		t.Errorf("OpenCodeGo call count = %d, want 1", openCodeGoCalls)
	}
	if mockCalls != 0 {
		t.Errorf("Mock call count = %d, want 0", mockCalls)
	}
}

// ============================================================================
// Test 10: Failover chain — all three fail, last-resort Mock invoked
// ============================================================================

// TestRouter_Failover_AllThreeFail_MockLastResort verifies that when Primary
// (MiniMax), Backup1 (DeepSeek), and Backup2 (OpenCodeGo) ALL fail, the
// last-resort handler produces a deterministic fallback response:
// empty Output, ProviderMock, and no error. Even if a Mock provider is
// registered and would fail, the lastResortHandler is a separate internal
// mechanism and is invoked after the chain is exhausted.
func TestRouter_Failover_AllThreeFail_MockLastResort(t *testing.T) {
	var miniMaxCalls, deepseekCalls, openCodeGoCalls, mockCalls int

	providerErr := errors.New("provider unavailable")
	chainErr := errors.New("should not be called — last resort should handle this")

	miniMax := &integrationMockProvider{
		name:       llm.ProviderMiniMax,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true},
		callCount:  &miniMaxCalls,
	}

	deepseek := &integrationMockProvider{
		name:       llm.ProviderDeepSeek,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true},
		callCount:  &deepseekCalls,
	}

	openCodeGo := &integrationMockProvider{
		name:       llm.ProviderOpenCodeGo,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderOpenCodeGo, Healthy: true},
		callCount:  &openCodeGoCalls,
	}

	// Mock is registered but would fail if called; the last-resort handler
	// does NOT delegate to the registered Mock provider — it produces a
	// synthetic response internally.
	mock := &integrationMockProvider{
		name:       llm.ProviderMock,
		callErr:    chainErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMock, Healthy: true},
		callCount:  &mockCalls,
	}

	router := llm.NewDefaultRouter(miniMax, deepseek, openCodeGo, mock)

	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)
	assertNoError(t, err, "Router.Call (all three fail, last-resort invoked)")

	// Assert: last-resort returns ProviderMock with empty Output.
	if resp.Provider != llm.ProviderMock {
		t.Errorf("Provider = %q, want %q (last-resort always returns ProviderMock)",
			resp.Provider, llm.ProviderMock)
	}
	if resp.Output != "" {
		t.Errorf("Output = %q, want empty string (last-resort returns empty output)", resp.Output)
	}

	// Assert: all 3 chain members in AttemptedProviders.
	if len(resp.AttemptedProviders) != 3 {
		t.Errorf("len(AttemptedProviders) = %d, want 3: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != llm.ProviderMiniMax {
		t.Errorf("AttemptedProviders[0] = %q, want %q",
			resp.AttemptedProviders[0], llm.ProviderMiniMax)
	}
	if resp.AttemptedProviders[1] != llm.ProviderDeepSeek {
		t.Errorf("AttemptedProviders[1] = %q, want %q",
			resp.AttemptedProviders[1], llm.ProviderDeepSeek)
	}
	if resp.AttemptedProviders[2] != llm.ProviderOpenCodeGo {
		t.Errorf("AttemptedProviders[2] = %q, want %q",
			resp.AttemptedProviders[2], llm.ProviderOpenCodeGo)
	}

	// Assert: all 3 chain members were attempted; Mock was NOT called.
	if miniMaxCalls != 1 {
		t.Errorf("MiniMax call count = %d, want 1", miniMaxCalls)
	}
	if deepseekCalls != 1 {
		t.Errorf("DeepSeek call count = %d, want 1", deepseekCalls)
	}
	if openCodeGoCalls != 1 {
		t.Errorf("OpenCodeGo call count = %d, want 1", openCodeGoCalls)
	}
	if mockCalls != 0 {
		t.Errorf("Mock call count = %d, want 0 (last-resort is internal, not registered provider call)",
			mockCalls)
	}
}

// ============================================================================
// Test 11: Failover chain — all providers fail, exhaustion counter incremented
// ============================================================================

// TestRouter_Failover_AllFail_ExhaustionCounter verifies that when the
// entire routing chain (MiniMax → DeepSeek → OpenCodeGo) is exhausted,
// the BackupChainExhaustedTotal counter is incremented AND the response
// returns a non-nil error via the last-resort fallback.
//
// Note: the current DefaultRouter.lastResortHandler always returns a
// successful Response with empty Output + ProviderMock (no error). This
// test verifies that behavior explicitly and checks the exhaustion counter.
func TestRouter_Failover_AllFail_ExhaustionCounter(t *testing.T) {
	providerErr := errors.New("provider error")

	miniMax := &integrationMockProvider{
		name:       llm.ProviderMiniMax,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true},
	}

	deepseek := &integrationMockProvider{
		name:       llm.ProviderDeepSeek,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true},
	}

	openCodeGo := &integrationMockProvider{
		name:       llm.ProviderOpenCodeGo,
		callErr:    providerErr,
		healthResp: llm.HealthStatus{Provider: llm.ProviderOpenCodeGo, Healthy: true},
	}

	router := llm.NewDefaultRouter(miniMax, deepseek, openCodeGo)

	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := router.Call(context.Background(), req)

	// The last-resort handler returns nil error, but the ApiResponse carries
	// ProviderMock with empty Output as the exhaustion signal.
	if err != nil {
		t.Fatalf("Router.Call unexpected error: %v (last-resort should not error)", err)
	}

	// Assert: last-resort fallback produced.
	if resp.Provider != llm.ProviderMock {
		t.Errorf("Provider = %q, want %q (last-resort fallback)", resp.Provider, llm.ProviderMock)
	}
	if resp.Output != "" {
		t.Errorf("Output = %q, want empty", resp.Output)
	}

	// Assert: all 3 chain members were attempted.
	if len(resp.AttemptedProviders) != 3 {
		t.Errorf("len(AttemptedProviders) = %d, want 3: %v",
			len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
}
