package llm

import "testing"

// TestDefaultRouter_Health tests that Health() aggregates HealthStatus from
// all registered providers. Covers the healthy and unhealthy provider cases.
func TestDefaultRouter_Health(t *testing.T) {
	// Given: router with two providers — one healthy, one unhealthy
	deepseek := &mockProvider{
		name: ProviderDeepSeek,
		healthResp: HealthStatus{
			Provider: ProviderDeepSeek,
			Healthy:  true,
		},
	}
	kimi := &mockProvider{
		name: ProviderKimi,
		healthResp: HealthStatus{
			Provider:  ProviderKimi,
			Healthy:   false,
			LastError: "connection refused",
		},
	}
	router := NewDefaultRouter(deepseek, kimi)

	// When
	healthMap := router.Health()

	// Then
	if len(healthMap) != 2 {
		t.Errorf("expected 2 health entries, got %d", len(healthMap))
	}
	if h, ok := healthMap[ProviderDeepSeek]; !ok || !h.Healthy {
		t.Error("expected deepseek to be healthy")
	}
	if h, ok := healthMap[ProviderKimi]; !ok || h.Healthy {
		t.Error("expected kimi to be unhealthy")
	}
}

// TestDefaultRouter_Health_Empty tests Health() when no providers are registered.
// The returned map must be non-nil and empty.
func TestDefaultRouter_Health_Empty(t *testing.T) {
	// Given: an empty router
	router := NewDefaultRouter()

	// When
	healthMap := router.Health()

	// Then: returns a non-nil empty map
	if healthMap == nil {
		t.Fatal("expected non-nil map from Health() with no providers")
	}
	if len(healthMap) != 0 {
		t.Errorf("expected empty map, got %d entries", len(healthMap))
	}
}
