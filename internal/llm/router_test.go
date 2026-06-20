package llm

import (
	"context"
	"errors"
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
	if resp.Provider != ProviderDeepSeek {
		t.Errorf("expected ProviderDeepSeek, got %v", resp.Provider)
	}
	if len(resp.AttemptedProviders) != 1 {
		t.Errorf("expected 1 attempted provider, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != ProviderDeepSeek {
		t.Errorf("expected first attempted to be ProviderDeepSeek, got %v", resp.AttemptedProviders[0])
	}
}

// TestDefaultRouter_PrimaryFail_Backup1Success tests the fallback chain:
// Primary fails → Backup1 succeeds.
func TestDefaultRouter_PrimaryFail_Backup1Success(t *testing.T) {
	// Given: Primary fails, Backup1 succeeds
	primaryErr := errors.New("primary down")
	primary := &mockProvider{
		name:    ProviderDeepSeek,
		callErr: primaryErr,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}

	backupResp := Response{
		Output:   "backup output",
		Provider: ProviderMiniMax,
	}
	backup := &mockProvider{
		name:     ProviderMiniMax,
		callResp: backupResp,
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
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
	if resp.Provider != ProviderMiniMax {
		t.Errorf("expected ProviderMiniMax, got %v", resp.Provider)
	}
	if len(resp.AttemptedProviders) != 2 {
		t.Errorf("expected 2 attempted providers, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
	if resp.AttemptedProviders[0] != ProviderDeepSeek {
		t.Errorf("expected first attempted to be ProviderDeepSeek, got %v", resp.AttemptedProviders[0])
	}
	if resp.AttemptedProviders[1] != ProviderMiniMax {
		t.Errorf("expected second attempted to be ProviderMiniMax, got %v", resp.AttemptedProviders[1])
	}
}

// TestDefaultRouter_AllFail_LastResort tests that when Primary, Backup1, and Backup2
// all fail, the LastResort handler is invoked and returns a deterministic response
// (for CapabilityFailureAttribution: empty Output with ProviderMock).
func TestDefaultRouter_AllFail_LastResort(t *testing.T) {
	// Given: all three chain members fail
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
		name:    ProviderOpenCodeGo,
		callErr: chainErr,
		healthResp: HealthStatus{
			Provider: ProviderOpenCodeGo,
			Healthy:  true,
		},
	}
	backup2 := &mockProvider{
		name:    ProviderOpenCodeZen,
		callErr: chainErr,
		healthResp: HealthStatus{
			Provider: ProviderOpenCodeZen,
			Healthy:  true,
		},
	}

	router := NewDefaultRouter(primary, backup1, backup2)

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
	// All three providers should be in AttemptedProviders
	if len(resp.AttemptedProviders) != 3 {
		t.Errorf("expected 3 attempted providers, got %d: %v", len(resp.AttemptedProviders), resp.AttemptedProviders)
	}
}

// TestDefaultRouter_DataClassGate tests that DataClassRegulated skips ProviderMiniMax.
func TestDefaultRouter_DataClassGate(t *testing.T) {
	// Given: a chain where MiniMax is the Primary and deepseek is Backup1,
	// but the request has DataClassRegulated, so MiniMax should be skipped.
	deepseekResp := Response{
		Output:   "deepseek output",
		Provider: ProviderDeepSeek,
	}
	miniMax := &mockProvider{
		name:    ProviderMiniMax,
		callErr: nil, // would succeed, but should be skipped
		callResp: Response{
			Output:   "minimax should not be called",
			Provider: ProviderMiniMax,
		},
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
			Healthy:  true,
		},
	}
	deepseek := &mockProvider{
		name:     ProviderDeepSeek,
		callResp: deepseekResp,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}

	// Create router with MiniMax as Primary and DeepSeek as Backup1
	router := &DefaultRouter{
		providers: map[Provider]ProviderImpl{
			ProviderMiniMax:  miniMax,
			ProviderDeepSeek: deepseek,
		},
		routingTable: RouterConfig{
			RoutingChains: map[Capability]RoutingChain{
				CapabilityFailureAttribution: {
					Primary:    ProviderMiniMax,
					Backup1:    ProviderDeepSeek,
					Backup2:    ProviderMock,
					LastResort: ProviderMock,
				},
			},
		},
	}

	// When: calling with DataClassRegulated
	req := Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassRegulated,
	}
	resp, err := router.Call(context.Background(), req)

	// Then: MiniMax should be skipped, DeepSeek should be used
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MiniMax should NOT appear in AttemptedProviders because it was gated
	if resp.Provider != ProviderDeepSeek {
		t.Errorf("expected ProviderDeepSeek (MiniMax gated), got %v", resp.Provider)
	}
	// MiniMax should not be in AttemptedProviders
	for _, p := range resp.AttemptedProviders {
		if p == ProviderMiniMax {
			t.Errorf("ProviderMiniMax should not appear in AttemptedProviders for DataClassRegulated")
		}
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

// TestDefaultRouter_ForceProvider_RespectsDataClassGate verifies that ForceProvider
// still respects the DataClass gate (ADR-010). A caller cannot route regulated data
// to hosted MiniMax M3 by setting ForceProvider=ProviderMiniMax + DataClass=Regulated.
func TestDefaultRouter_ForceProvider_RespectsDataClassGate(t *testing.T) {
	miniMax := &mockProvider{
		name: ProviderMiniMax,
		healthResp: HealthStatus{
			Provider: ProviderMiniMax,
			Healthy:  true,
		},
	}
	deepseek := &mockProvider{
		name: ProviderDeepSeek,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	router := NewDefaultRouter(miniMax, deepseek)

	t.Run("Regulated+ForceProviderMiniMax returns ErrProviderDisabled", func(t *testing.T) {
		forceM3 := ProviderMiniMax
		req := Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassRegulated,
			Options:    Options{ForceProvider: &forceM3},
		}
		_, err := router.Call(context.Background(), req)
		if !errors.Is(err, ErrProviderDisabled) {
			t.Fatalf("expected ErrProviderDisabled, got %v", err)
		}
	})

	t.Run("Secret+ForceProviderMiniMax returns ErrProviderDisabled", func(t *testing.T) {
		forceM3 := ProviderMiniMax
		req := Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassSecret,
			Options:    Options{ForceProvider: &forceM3},
		}
		_, err := router.Call(context.Background(), req)
		if !errors.Is(err, ErrProviderDisabled) {
			t.Fatalf("expected ErrProviderDisabled, got %v", err)
		}
	})

	t.Run("Unmarked+ForceProviderMiniMax proceeds normally", func(t *testing.T) {
		miniMax.callResp = Response{Output: "ok", Provider: ProviderMiniMax}
		forceM3 := ProviderMiniMax
		req := Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassUnmarked,
			Options:    Options{ForceProvider: &forceM3},
		}
		resp, err := router.Call(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderMiniMax {
			t.Errorf("expected ProviderMiniMax, got %v", resp.Provider)
		}
	})

	t.Run("NonRegulated+ForceProviderDeepSeek proceeds normally", func(t *testing.T) {
		deepseek.callResp = Response{Output: "deepseek ok", Provider: ProviderDeepSeek}
		forceDS := ProviderDeepSeek
		req := Request{
			Capability: CapabilityFailureAttribution,
			DataClass:  DataClassRegulated,
			Options:    Options{ForceProvider: &forceDS},
		}
		resp, err := router.Call(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderDeepSeek {
			t.Errorf("expected ProviderDeepSeek, got %v", resp.Provider)
		}
	})
}
