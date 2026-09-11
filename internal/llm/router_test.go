package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider implements ProviderImpl with configurable behavior for testing.
type mockProvider struct {
	name       Provider
	supported  map[Capability]bool
	callErr    error
	callResp   Response
	callDelay  time.Duration
	healthResp HealthStatus
}

func (m *mockProvider) Supports(cap Capability) bool {
	if m.supported == nil {
		return true
	}
	supported, ok := m.supported[cap]
	return ok && supported
}

func (m *mockProvider) Call(_ context.Context, _ Request) (Response, error) {
	if m.callDelay > 0 {
		time.Sleep(m.callDelay)
	}
	if m.callErr != nil {
		return Response{}, m.callErr
	}
	return m.callResp, nil
}

func (m *mockProvider) Health() HealthStatus {
	return m.healthResp
}

// TestDefaultRouter_PrimarySuccess tests that when Primary succeeds,
// the response comes from the Primary provider and AttemptedProviders
// contains only the Primary.
func TestDefaultRouter_PrimarySuccess(t *testing.T) {
	// Given: a router with a mock provider that succeeds
	primaryResp := Response{
		Output:   "primary output",
		Provider: ProviderMiniMax,
	}
	primary := &mockProvider{
		name:     ProviderMiniMax,
		callResp: primaryResp,
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
			Healthy:  true,
		},
	}

	router := NewDefaultRouter(primary)

	// When: calling with a capability that routes to this provider
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}
	resp, err := router.Call(context.Background(), req)
	// Then: the call succeeds with primary output
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Output != "primary output" {
		t.Errorf("expected primary output, got %q", resp.Output)
	}
	if resp.Provider != ProviderMiniMax {
		t.Errorf("expected ProviderMiniMax, got %v", resp.Provider)
	}
	if len(resp.AttemptedProviders) != 1 {
		t.Errorf("expected 1 attempted provider, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != ProviderMiniMax {
		t.Errorf("expected first attempted to be ProviderMiniMax, got %v", resp.AttemptedProviders[0])
	}
}

// TestDefaultRouter_PrimaryFail_Backup1Success tests the fallback chain:
// Primary fails → Backup1 succeeds.
func TestDefaultRouter_PrimaryFail_Backup1Success(t *testing.T) {
	// Given: Primary fails, Backup1 succeeds
	primaryErr := errors.New("primary down")
	primary := &mockProvider{
		name:    ProviderMiniMax,
		callErr: primaryErr,
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
			Healthy:  true,
		},
	}

	backupResp := Response{
		Output:   "backup output",
		Provider: ProviderDeepSeek,
	}
	backup := &mockProvider{
		name:     ProviderDeepSeek,
		callResp: backupResp,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}

	router := NewDefaultRouter(primary, backup)

	// When
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}
	resp, err := router.Call(context.Background(), req)
	// Then: Backup1 succeeds, and both Primary and Backup1 appear in AttemptedProviders
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Output != "backup output" {
		t.Errorf("expected backup output, got %q", resp.Output)
	}
	if resp.Provider != ProviderDeepSeek {
		t.Errorf("expected ProviderDeepSeek, got %v", resp.Provider)
	}
	if len(resp.AttemptedProviders) != 2 {
		t.Errorf("expected 2 attempted providers, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != ProviderMiniMax {
		t.Errorf("expected first attempted to be ProviderMiniMax, got %v", resp.AttemptedProviders[0])
	}
	if resp.AttemptedProviders[1] != ProviderDeepSeek {
		t.Errorf("expected second attempted to be ProviderDeepSeek, got %v", resp.AttemptedProviders[1])
	}
}

// TestDefaultRouter_AllFail_LastResort tests that when Primary and Backup1
// both fail, the LastResort handler is invoked and returns a deterministic
// response (for CapabilityFailureAttribution: empty Output with ProviderMock).
//
// Wave 11 L2.1 doc audit (Issue #720): the default routing chain is now
// effectively 3 tiers (Backup2 is empty in defaultRoutingTable and
// configs/llm_router.yaml). Empty Backup2 entries are skipped before being
// registered as "attempted", so AttemptedProviders contains only the
// providers that were actually invoked.
func TestDefaultRouter_AllFail_LastResort(t *testing.T) {
	// Given: both registered chain members fail
	chainErr := errors.New("provider error")
	primary := &mockProvider{
		name:    ProviderDeepSeek,
		callErr: chainErr,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	backup1 := &mockProvider{
		name:    ProviderMiniMax,
		callErr: chainErr,
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
			Healthy:  true,
		},
	}

	router := NewDefaultRouter(primary, backup1)

	// When
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}
	resp, err := router.Call(context.Background(), req)
	// Then: Last resort returns a deterministic fallback with ProviderMock
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Output != "" {
		t.Errorf("expected empty output from last resort, got %q", resp.Output)
	}
	if resp.Provider != ProviderMock {
		t.Errorf("expected ProviderMock from last resort, got %v", resp.Provider)
	}
	// Both registered chain members should be in AttemptedProviders.
	// The empty Backup2 slot (skipped by router.go:Call) is not recorded.
	if len(resp.AttemptedProviders) != 2 {
		t.Errorf("expected 2 attempted providers, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
}

// TestDefaultRouter_DataClassDoesNotGateProvider verifies ADR-012: DataClass is
// audit metadata only, so regulated/secret payloads route to the chain primary
// (MiniMax) exactly like unmarked data.
func TestDefaultRouter_DataClassDoesNotGateProvider(t *testing.T) {
	for _, dc := range []DataClass{DataClassUnmarked, DataClassNonRegulated, DataClassRegulated, DataClassSecret} {
		t.Run(dc.String(), func(t *testing.T) {
			miniMax := &mockProvider{
				name:     ProviderMiniMax,
				callResp: Response{Output: "minimax output", Provider: ProviderMiniMax},
				healthResp: HealthStatus{
					Provider: ProviderMiniMax,
					Healthy:  true,
				},
			}
			deepseek := &mockProvider{
				name:     ProviderDeepSeek,
				callResp: Response{Output: "deepseek output", Provider: ProviderDeepSeek},
				healthResp: HealthStatus{
					Provider: ProviderDeepSeek,
					Healthy:  true,
				},
			}
			router := NewDefaultRouter(miniMax, deepseek)

			resp, err := router.Call(context.Background(), Request{
				Capability: CapabilityFailureAttribution,
				DataClass:  dc,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Provider != ProviderMiniMax {
				t.Errorf("DataClass %v: expected ProviderMiniMax (no gate), got %v", dc, resp.Provider)
			}
			if len(resp.AttemptedProviders) != 1 || resp.AttemptedProviders[0] != ProviderMiniMax {
				t.Errorf("DataClass %v: expected only ProviderMiniMax attempted, got %v", dc, resp.AttemptedProviders)
			}
		})
	}
}

// TestDefaultRouter_ForceProvider tests that when Options.ForceProvider is set,
// the routing table is ignored and only the forced provider is tried.
func TestDefaultRouter_ForceProvider(t *testing.T) {
	// Given: a router with multiple providers, but ForceProvider set to a specific one
	forcedResp := Response{
		Output:   "forced output",
		Provider: ProviderKimi,
	}
	kimi := &mockProvider{
		name:     ProviderKimi,
		callResp: forcedResp,
		healthResp: HealthStatus{
			Provider: ProviderKimi,
			Healthy:  true,
		},
	}
	deepseek := &mockProvider{
		name:    ProviderDeepSeek,
		callErr: errors.New("should not be called"),
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}

	router := NewDefaultRouter(kimi, deepseek)

	// When: calling with ForceProvider set to Kimi
	forceKimi := ProviderKimi
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
		Options: Options{
			ForceProvider: &forceKimi,
		},
	}
	resp, err := router.Call(context.Background(), req)
	// Then: Kimi is used directly
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != ProviderKimi {
		t.Errorf("expected ProviderKimi from force, got %v", resp.Provider)
	}
	if resp.Output != "forced output" {
		t.Errorf("expected forced output, got %q", resp.Output)
	}
	if len(resp.AttemptedProviders) != 1 {
		t.Errorf("expected 1 attempted provider with force, got %d", len(resp.AttemptedProviders))
	}
}

// TestDefaultRouter_ForceProvider_NotFound tests that when ForceProvider
// requests a provider that is not registered, an error is returned.
func TestDefaultRouter_ForceProvider_NotFound(t *testing.T) {
	// Given: a router with DeepSeek registered
	deepseek := &mockProvider{
		name: ProviderDeepSeek,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	router := NewDefaultRouter(deepseek)

	// When: ForceProvider is set to an unregistered provider
	forceKimi := ProviderKimi
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
		Options: Options{
			ForceProvider: &forceKimi,
		},
	}
	_, err := router.Call(context.Background(), req)

	// Then: error because provider not found
	if err == nil {
		t.Fatal("expected error when ForceProvider is not registered")
	}
}

// TestNewDefaultRouter_NoAdapters tests that a router with no providers
// can still be created and returns errors appropriately.
func TestNewDefaultRouter_NoAdapters(t *testing.T) {
	// Given
	router := NewDefaultRouter()

	// When: calling with a known capability but no registered providers
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}
	resp, err := router.Call(context.Background(), req)
	// Then: the last-resort handler is invoked, since no providers can fulfill
	if err != nil {
		t.Fatalf("expected last-resort response, got error: %v", err)
	}
	if resp.Provider != ProviderMock {
		t.Errorf("expected ProviderMock from last resort with no providers, got %v", resp.Provider)
	}
	if resp.Output != "" {
		t.Errorf("expected empty output from last resort, got %q", resp.Output)
	}

	// When: calling with an unsupported capability and no providers
	req2 := Request{
		Capability: Capability("unknown_cap"),
		DataClass:  DataClassUnmarked,
	}
	_, err2 := router.Call(context.Background(), req2)

	// Then: ErrCapabilityNotSupported
	if !errors.Is(err2, ErrCapabilityNotSupported) {
		t.Errorf("expected ErrCapabilityNotSupported, got %v", err2)
	}
}

// TestDefaultRouter_UnsupportedCapability tests that a request with an
// unknown capability returns ErrCapabilityNotSupported when the last-resort
// handler also does not handle it.
func TestDefaultRouter_UnsupportedCapability(t *testing.T) {
	// Given: a router with registered providers
	deepseek := &mockProvider{
		name: ProviderDeepSeek,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	router := NewDefaultRouter(deepseek)

	// When: calling with an unsupported capability
	req := Request{
		Capability: Capability("nonexistent"),
		DataClass:  DataClassUnmarked,
	}
	_, err := router.Call(context.Background(), req)

	// Then: error is ErrCapabilityNotSupported
	if !errors.Is(err, ErrCapabilityNotSupported) {
		t.Errorf("expected ErrCapabilityNotSupported, got %v", err)
	}
}

// TestDefaultRouter_AttemptedProviders_OnSuccess tests that AttemptedProviders
// is populated even on the Primary-success path.
func TestDefaultRouter_AttemptedProviders_OnSuccess(t *testing.T) {
	// Given
	primaryResp := Response{
		Output:   "success",
		Provider: ProviderDeepSeek,
	}
	primary := &mockProvider{
		name:     ProviderDeepSeek,
		callResp: primaryResp,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	router := NewDefaultRouter(primary)

	// When
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}
	resp, err := router.Call(context.Background(), req)
	// Then: AttemptedProviders contains the provider
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.AttemptedProviders) == 0 {
		t.Error("expected AttemptedProviders to be populated on success")
	}
}

// TestDefaultRouter_Counter_Increment verifies internal fallback counters exist and are
// accessible (package-level counters for fallback_triggered_total, backup_chain_exhausted_total).
func TestDefaultRouter_Counter_Increment(t *testing.T) {
	// Verify counters exist at package level — they should be defined
	if FallbackTriggeredTotal == nil {
		t.Error("FallbackTriggeredTotal counter is nil")
	}
	if BackupChainExhaustedTotal == nil {
		t.Error("BackupChainExhaustedTotal counter is nil")
	}
}

// TestDefaultRouter_ForceProvider_IgnoresDataClass verifies ADR-012: a forced
// provider is called even for DataClassRegulated/Secret (the gate that used to
// return ErrProviderDisabled is gone).
func TestDefaultRouter_ForceProvider_IgnoresDataClass(t *testing.T) {
	miniMax := &mockProvider{
		name:       ProviderMiniMax,
		callResp:   Response{Output: "ok", Provider: ProviderMiniMax},
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	deepseek := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek ok", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(miniMax, deepseek)

	t.Run("Regulated+ForceProviderMiniMax proceeds", func(t *testing.T) {
		forceM3 := ProviderMiniMax
		resp, err := router.Call(context.Background(), Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassRegulated,
			Options:    Options{ForceProvider: &forceM3},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderMiniMax {
			t.Errorf("expected ProviderMiniMax, got %v", resp.Provider)
		}
	})

	t.Run("Secret+ForceProviderMiniMax proceeds", func(t *testing.T) {
		forceM3 := ProviderMiniMax
		resp, err := router.Call(context.Background(), Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassSecret,
			Options:    Options{ForceProvider: &forceM3},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderMiniMax {
			t.Errorf("expected ProviderMiniMax, got %v", resp.Provider)
		}
	})

	t.Run("Regulated+ForceProviderDeepSeek proceeds", func(t *testing.T) {
		forceDS := ProviderDeepSeek
		resp, err := router.Call(context.Background(), Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassRegulated,
			Options:    Options{ForceProvider: &forceDS},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderDeepSeek {
			t.Errorf("expected ProviderDeepSeek, got %v", resp.Provider)
		}
	})
}

// TestDefaultRouter_EmptyOutputFallsThroughToBackup verifies ADR-012: a
// provider that "succeeds" with empty output is treated as failed, so the next
// chain member is tried and AttemptedProviders records both.
func TestDefaultRouter_EmptyOutputFallsThroughToBackup(t *testing.T) {
	for _, blank := range []string{"", "   \n\t "} {
		t.Run("output="+strconv.Quote(blank), func(t *testing.T) {
			before := atomic.LoadInt64(FallbackTriggeredTotal)

			primary := &mockProvider{
				name:       ProviderMiniMax,
				callResp:   Response{Output: blank, Provider: ProviderMiniMax},
				healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
			}
			backup := &mockProvider{
				name:       ProviderDeepSeek,
				callResp:   Response{Output: "backup output", Provider: ProviderDeepSeek},
				healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
			}
			router := NewDefaultRouter(primary, backup)

			resp, err := router.Call(context.Background(), Request{
				Capability: CapabilityFailureAttribution,
				DataClass:  DataClassRegulated,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Output != "backup output" {
				t.Errorf("expected backup output, got %q", resp.Output)
			}
			if resp.Provider != ProviderDeepSeek {
				t.Errorf("expected ProviderDeepSeek, got %v", resp.Provider)
			}
			if len(resp.AttemptedProviders) != 2 ||
				resp.AttemptedProviders[0] != ProviderMiniMax ||
				resp.AttemptedProviders[1] != ProviderDeepSeek {
				t.Errorf("expected [minimax deepseek] attempted, got %v", resp.AttemptedProviders)
			}
			if after := atomic.LoadInt64(FallbackTriggeredTotal); after != before+1 {
				t.Errorf("FallbackTriggeredTotal delta = %d, want 1", after-before)
			}
		})
	}
}

// TestDefaultRouter_AllEmptyOutputExhaustsChain verifies that a chain whose
// members all answer with empty output ends in the last-resort handler and
// still reports every attempted provider (no silent "success").
func TestDefaultRouter_AllEmptyOutputExhaustsChain(t *testing.T) {
	chain := []*mockProvider{
		{name: ProviderMiniMax, callResp: Response{Output: "", Provider: ProviderMiniMax}, healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true}},
		{name: ProviderDeepSeek, callResp: Response{Output: "\n", Provider: ProviderDeepSeek}, healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true}},
	}
	router := NewDefaultRouter(chain[0], chain[1])

	resp, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != ProviderMock {
		t.Errorf("expected ProviderMock last resort, got %v", resp.Provider)
	}
	if resp.Output != "" {
		t.Errorf("expected empty last-resort output, got %q", resp.Output)
	}
	if len(resp.AttemptedProviders) != 2 {
		t.Errorf("expected both chain members attempted, got %v", resp.AttemptedProviders)
	}
}

// TestDefaultRouter_ToolCallOnlyResponseIsSuccess verifies the empty-output
// check does not break function-calling flows: a response with ToolCalls and no
// text is a legitimate success.
func TestDefaultRouter_ToolCallOnlyResponseIsSuccess(t *testing.T) {
	primary := &mockProvider{
		name: ProviderMiniMax,
		callResp: Response{
			Output:    "",
			Provider:  ProviderMiniMax,
			ToolCalls: []ToolCall{{ID: "call_1", Name: "get_weather", Arguments: json.RawMessage(`{}`)}},
		},
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "should not be reached", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup)

	resp, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != ProviderMiniMax {
		t.Errorf("expected ProviderMiniMax (tool call is a valid response), got %v", resp.Provider)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
}

// TestDefaultRouter_SkippedPrimaryNotInAttemptedProviders pins the ADR-012
// follow-up accounting rule: AttemptedProviders lists only providers that were
// actually invoked (so it answers "who received this data?"), while a chain
// member that could not be invoked at all (not registered) is reported on the
// span as llm.skipped_providers. The fallback counter still moves, because a
// real fallback did happen.
func TestDefaultRouter_SkippedPrimaryNotInAttemptedProviders(t *testing.T) {
	before := atomic.LoadInt64(FallbackTriggeredTotal)

	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	// Only the backup is registered; the chain's primary is absent (the
	// production situation for code capabilities when LLM_KIMI_API_KEY is unset).
	router := NewDefaultRouter(backup)

	resp, err := router.Call(context.Background(), Request{
		Capability: CapabilityPromptLint,
		DataClass:  DataClassNonRegulated,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Output != "deepseek answered" {
		t.Errorf("Output = %q, want the backup answer", resp.Output)
	}
	if len(resp.AttemptedProviders) != 1 || resp.AttemptedProviders[0] != ProviderDeepSeek {
		t.Errorf("AttemptedProviders = %v, want [deepseek] (kimi was never invoked)", resp.AttemptedProviders)
	}
	for _, p := range resp.AttemptedProviders {
		if p == ProviderKimi {
			t.Error("skipped provider must not appear in AttemptedProviders")
		}
	}
	if delta := atomic.LoadInt64(FallbackTriggeredTotal) - before; delta != 1 {
		t.Errorf("FallbackTriggeredTotal delta = %d, want 1 (a real fallback happened)", delta)
	}
}

// TestDefaultRouter_UnsupportedProviderSkippedNotCountedAsFallback verifies
// that a registered provider which does not Support the capability is skipped
// without inflating the fallback counter or AttemptedProviders.
func TestDefaultRouter_UnsupportedProviderSkippedNotCountedAsFallback(t *testing.T) {
	before := atomic.LoadInt64(FallbackTriggeredTotal)

	primary := &mockProvider{
		name:       ProviderKimi,
		supported:  map[Capability]bool{CapabilityCodeReviewAnnotation: true, CapabilityPromptLint: true},
		callResp:   Response{Output: "kimi answered", Provider: ProviderKimi},
		healthResp: HealthStatus{Provider: ProviderKimi, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup)

	// failure_attribution is NOT in the kimi provider's supported set.
	resp, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.AttemptedProviders) != 1 || resp.AttemptedProviders[0] != ProviderDeepSeek {
		t.Errorf("AttemptedProviders = %v, want [deepseek]", resp.AttemptedProviders)
	}
	if delta := atomic.LoadInt64(FallbackTriggeredTotal) - before; delta != 1 {
		t.Errorf("FallbackTriggeredTotal delta = %d, want 1", delta)
	}
}
