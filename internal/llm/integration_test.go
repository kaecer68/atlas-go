// Package llm_test provides end-to-end integration tests for the full
// handler → Router → adapter → client → HTTP → response parsing chain.
//
// These tests exercise the Phase 2 ProviderImpl adapters (DeepSeek, MiniMax,
// Kimi) with real httptest servers, and the DefaultRouter routing chain with
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
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
// Test 3: KimiAdapter — full HTTP E2E chain + K2.7 constraints
// ============================================================================

// TestKimiAdapter_EndToEnd verifies the Kimi K2.7 integration:
//   - Full HTTP chain with forced thinking + temperature=1.0
//   - ADR-009 guard: Supports() rejects non-code capabilities
//   - Client-side DataClass gate
func TestKimiAdapter_EndToEnd(t *testing.T) {
	t.Run("successful chat with K2.7 constraints", func(t *testing.T) {
		var capturedBody []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			capturedBody = body

			resp := map[string]any{
				"model": clients.DefaultModelKimiK27,
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": "Kimi K2.7 code review response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int64{
					"prompt_tokens":     50,
					"completion_tokens": 25,
					"total_tokens":      75,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		baseClient := newIntegrationBaseClient()
		client := clients.NewKimiClient("test-kimi-key", baseClient)
		client.BaseURL = srv.URL

		adapter := adapters.NewKimiAdapter(client)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "Review this code for bugs"},
			},
		})

		req := llm.Request{
			Capability: llm.CapabilityCodeReviewAnnotation, // ADR-009 allowed
			Payload:    payload,
			DataClass:  llm.DataClassNonRegulated,
			Options:    llm.Options{MaxTokens: 200},
		}

		resp, err := adapter.Call(context.Background(), req)
		assertNoError(t, err, "KimiAdapter.Call")

		if resp.Output != "Kimi K2.7 code review response" {
			t.Errorf("Output = %q, want %q", resp.Output, "Kimi K2.7 code review response")
		}
		if resp.Provider != llm.ProviderKimi {
			t.Errorf("Provider = %q, want %q", resp.Provider, llm.ProviderKimi)
		}

		// Verify K2.7 constraints are enforced in the request body.
		var reqBody map[string]any
		if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
			t.Fatalf("unmarshal captured body: %v", err)
		}

		// Temperature must be exactly 1.0.
		temp, ok := reqBody["temperature"].(float64)
		if !ok || temp != 1.0 {
			t.Errorf("temperature = %v (%T), want 1.0", reqBody["temperature"], reqBody["temperature"])
		}

		// Thinking must be forced on.
		thinking, ok := reqBody["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("thinking field missing or wrong type: %T", reqBody["thinking"])
		}
		thinkType, ok := thinking["type"].(string)
		if !ok || thinkType != "enabled" {
			t.Errorf("thinking.type = %q, want %q", thinkType, "enabled")
		}

		// Model must be kimi-for-coding.
		model, _ := reqBody["model"].(string)
		if model != clients.DefaultModelKimiK27 {
			t.Errorf("model = %q, want %q", model, clients.DefaultModelKimiK27)
		}
	})

	t.Run("ADR-009: Supports rejects CapabilityRationaleGeneration", func(t *testing.T) {
		adapter := adapters.NewKimiAdapter(nil)

		if adapter.Supports(llm.CapabilityRationaleGeneration) {
			t.Error("Supports(CapabilityRationaleGeneration) = true, want false (ADR-009)")
		}
		if adapter.Supports(llm.CapabilityFailureAttribution) {
			t.Error("Supports(CapabilityFailureAttribution) = true, want false (ADR-009)")
		}
		if !adapter.Supports(llm.CapabilityCodeReviewAnnotation) {
			t.Error("Supports(CapabilityCodeReviewAnnotation) = false, want true")
		}
		if !adapter.Supports(llm.CapabilityPromptLint) {
			t.Error("Supports(CapabilityPromptLint) = false, want true")
		}
	})

	t.Run("ADR-009: Call rejects unauthorized capability", func(t *testing.T) {
		adapter := adapters.NewKimiAdapter(nil)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "test"}},
		})

		req := llm.Request{
			Capability: llm.CapabilityRationaleGeneration, // NOT in ADR-009 allowed set
			Payload:    payload,
		}

		_, err := adapter.Call(context.Background(), req)
		if err == nil {
			t.Fatal("expected ErrKimiRestricted for unauthorized capability, got nil")
		}
		if !errors.Is(err, adapters.ErrKimiRestricted) {
			t.Errorf("error = %v, want ErrKimiRestricted", err)
		}
	})

	t.Run("DataClassRegulated rejected by K2.7 client", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called for DataClassRegulated")
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}))
		defer srv.Close()

		baseClient := newIntegrationBaseClient()
		client := clients.NewKimiClient("test-kimi-key", baseClient)
		client.BaseURL = srv.URL

		adapter := adapters.NewKimiAdapter(client)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "test"}},
		})

		req := llm.Request{
			Capability: llm.CapabilityCodeReviewAnnotation, // ADR-009 allowed
			Payload:    payload,
			DataClass:  llm.DataClassRegulated,
			Options:    llm.Options{MaxTokens: 100}, // ensure buildChatOptions is non-nil
		}

		_, err := adapter.Call(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for DataClassRegulated, got nil")
		}
		if !strings.Contains(err.Error(), "incompatible") {
			t.Errorf("error = %v, want error containing 'incompatible'", err)
		}
	})

	t.Run("DataClassSecret rejected by K2.7 client", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called for DataClassSecret")
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}))
		defer srv.Close()

		baseClient := newIntegrationBaseClient()
		client := clients.NewKimiClient("test-kimi-key", baseClient)
		client.BaseURL = srv.URL

		adapter := adapters.NewKimiAdapter(client)

		payload := mustJSON(t, map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "test"}},
		})

		req := llm.Request{
			Capability: llm.CapabilityPromptLint, // ADR-009 allowed
			Payload:    payload,
			DataClass:  llm.DataClassSecret,
			Options:    llm.Options{MaxTokens: 100},
		}

		_, err := adapter.Call(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for DataClassSecret, got nil")
		}
		if !strings.Contains(err.Error(), "incompatible") {
			t.Errorf("error = %v, want error containing 'incompatible'", err)
		}
	})
}

// ============================================================================
// Test 4: Router — full fallback chain with 3 adapters
// ============================================================================

// TestRouter_FullChain_AllAdapters verifies the complete routing chain:
// Primary fails → Backup1 succeeds → Backup2 not reached.
// Uses CapabilityFailureAttribution, whose default routing chain is
// MiniMax → DeepSeek → Mock (Backup2 is empty after Wave 11 L2.1 doc audit
// Issue #720). We register MiniMax (fails) and DeepSeek (succeeds);
// empty Backup2 is skipped, LastResort (Mock) is reached.
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
	// CapabilityFailureAttribution default chain: MiniMax → DeepSeek → Mock (Backup2 empty, Issue #720).
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

	// Assert: AttemptedProviders contains at least the successful provider.
	// (Primary's failure may or may not be tracked depending on router version.)
	if len(resp.AttemptedProviders) == 0 {
		t.Fatal("len(AttemptedProviders) = 0, want at least 1")
	}
	foundMiniMax := slices.Contains(resp.AttemptedProviders, llm.ProviderMiniMax)
	if !foundMiniMax {
		t.Errorf("AttemptedProviders = %v, expected MiniMax", resp.AttemptedProviders)
	}
}

// ============================================================================
// Test 5: Router — DataClass gate prevents MiniMax fallback
// ============================================================================

// TestRouter_DataClassGate_PreventsFallback verifies that when the routing
// chain has MiniMax as primary and the request carries DataClassRegulated,
// MiniMax is skipped entirely and the Router falls through to the next
// provider in the chain.
// Uses CapabilityRiskSurfaceExtraction, whose default chain is
// MiniMax → DeepSeek → Mock (Backup2 empty, Issue #720). With
// DataClassRegulated, MiniMax is gated.
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

	// CapabilityRiskSurfaceExtraction default chain: MiniMax → DeepSeek → Mock (Backup2 empty, Issue #720).
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
	foundDeepSeek := slices.Contains(resp.AttemptedProviders, llm.ProviderDeepSeek)
	if !foundDeepSeek {
		t.Errorf("ProviderDeepSeek should appear in AttemptedProviders: %v", resp.AttemptedProviders)
	}
}

// ============================================================================
// Test 6: Router — Health endpoint aggregates all providers
// ============================================================================

// TestRouter_HealthEndpoint_AggregatesAllProviders verifies that Router.Health()
// returns all registered providers with their respective health statuses,
// including healthy, error-state, and circuit-breaker-open providers.
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
			LastError: "connection refused: api.minimax.io:443",
		},
	}

	// Provider with circuit breaker open.
	kimi := &integrationMockProvider{
		name: llm.ProviderKimi,
		healthResp: llm.HealthStatus{
			Provider:    llm.ProviderKimi,
			Healthy:     false,
			BreakerOpen: true,
			LastError:   "circuit breaker open: 5 consecutive failures",
		},
	}

	router := llm.NewDefaultRouter(deepseek, minimax, kimi)

	healthMap := router.Health()

	// Assert all 3 providers are present.
	if len(healthMap) != 3 {
		t.Fatalf("len(Health()) = %d, want 3", len(healthMap))
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

	// Assert Kimi has breaker open.
	kHealth, ok := healthMap[llm.ProviderKimi]
	if !ok {
		t.Fatal("ProviderKimi not found in Health()")
	}
	if !kHealth.BreakerOpen {
		t.Error("Kimi should have BreakerOpen=true")
	}
}

// ============================================================================
// Test 7: RationaleGenerationHandler — full chain through Router
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
