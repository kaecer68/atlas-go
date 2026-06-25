package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRouterConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm_router.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: minimax
    backup1: deepseek
    backup2: opencode_go
    last_resort: mock
  rationale_generation:
    primary: minimax
    backup1: deepseek
    backup2: opencode_go
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadRouterConfig(path)
	if err != nil {
		t.Fatalf("LoadRouterConfig() error = %v", err)
	}

	if len(cfg.RoutingChains) != 2 {
		t.Fatalf("expected 2 chains, got %d", len(cfg.RoutingChains))
	}

	fa, ok := cfg.RoutingChains[CapabilityFailureAttribution]
	if !ok {
		t.Fatal("expected failure_attribution chain")
	}
	if fa.Primary != ProviderMiniMax {
		t.Errorf("primary = %q, want %q", fa.Primary, ProviderMiniMax)
	}
	if fa.Backup1 != ProviderDeepSeek {
		t.Errorf("backup1 = %q, want %q", fa.Backup1, ProviderDeepSeek)
	}
}

func TestLoadRouterConfig_MissingFile(t *testing.T) {
	_, err := LoadRouterConfig("/nonexistent/llm_router.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadRouterConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRouterConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadRouterConfig_UnknownCapability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown_cap.yaml")
	yaml := `
routing_chains:
  unknown_capability:
    primary: minimax
    backup1: deepseek
    backup2: opencode_go
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRouterConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown capability")
	}
}

func TestLoadRouterConfig_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown_prov.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: unknown_provider
    backup1: deepseek
    backup2: opencode_go
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRouterConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestLoadRouterConfig_EmptyPrimary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_primary.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: ""
    backup1: deepseek
    backup2: opencode_go
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRouterConfig(path)
	if err == nil {
		t.Fatal("expected error for empty primary provider")
	}
}

func TestLoadRouterConfig_EmptyChains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_chains.yaml")
	if err := os.WriteFile(path, []byte("routing_chains: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRouterConfig(path)
	if err == nil {
		t.Fatal("expected error for empty routing_chains")
	}
}

func TestTryLoadRouterConfig_MissingFile_FallsBackToDefault(t *testing.T) {
	cfg := TryLoadRouterConfig("/nonexistent/llm_router.yaml")

	// Should fall back to defaultRoutingTable() which has 12 capabilities.
	if _, ok := cfg.RoutingChains[CapabilityFailureAttribution]; !ok {
		t.Fatal("fallback config missing failure_attribution chain")
	}
	if _, ok := cfg.RoutingChains[CapabilityConfidenceCommentary]; !ok {
		t.Fatal("fallback config missing confidence_commentary chain")
	}
}

func TestTryLoadRouterConfig_LoadsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm_router.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: deepseek
    backup1: minimax
    backup2: opencode_go
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := TryLoadRouterConfig(path)
	fa, ok := cfg.RoutingChains[CapabilityFailureAttribution]
	if !ok {
		t.Fatal("missing failure_attribution chain")
	}
	if fa.Primary != ProviderDeepSeek {
		t.Errorf("primary = %q, want %q", fa.Primary, ProviderDeepSeek)
	}
}

func TestNewDefaultRouterFromConfig(t *testing.T) {
	cfg := RouterConfig{
		RoutingChains: map[Capability]RoutingChain{
			CapabilityFailureAttribution: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
		},
	}

	r := NewDefaultRouterFromConfig(cfg)
	if r == nil {
		t.Fatal("NewDefaultRouterFromConfig() returned nil")
	}
	if len(r.providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(r.providers))
	}
	if len(r.routingTable.RoutingChains) != 1 {
		t.Errorf("expected 1 chain, got %d", len(r.routingTable.RoutingChains))
	}
}

func TestNewDefaultRouter_BackwardCompatible(t *testing.T) {
	r := NewDefaultRouter()
	if r == nil {
		t.Fatal("NewDefaultRouter() returned nil")
	}
	// NewDefaultRouter() delegates to NewDefaultRouterFromConfig + defaultRoutingTable()
	// so it should have 12 chains (11 original + confidence_commentary).
	if len(r.routingTable.RoutingChains) < 11 {
		t.Errorf("expected at least 11 chains, got %d", len(r.routingTable.RoutingChains))
	}
}

func TestLoadRouterConfig_AllTwelveCapabilities(t *testing.T) {
	// This test validates that all 12 known Capability constants can be
	// loaded from a YAML file with 3-tier fallback chains (Backup2 empty).
	// This ensures config.go's isKnownCapability switch stays in sync
	// with provider.go's Capability constants, and that empty Backup2 is
	// accepted (Wave 11 L2.1 doc audit, Issue #720).
	dir := t.TempDir()
	path := filepath.Join(dir, "all_caps.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  code_review_annotation:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  prompt_lint:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  rationale_generation:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  strategy_summary:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  risk_surface_extraction:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  regime_explanation:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  scenario_simulation:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  sentiment_explanation:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  performance_forensics:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  contra_attribution:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
  confidence_commentary:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadRouterConfig(path)
	if err != nil {
		t.Fatalf("LoadRouterConfig() error = %v", err)
	}

	if len(cfg.RoutingChains) != 12 {
		t.Errorf("expected 12 chains, got %d", len(cfg.RoutingChains))
	}

	// Verify all 12 capabilities are present.
	caps := []Capability{
		CapabilityFailureAttribution,
		CapabilityCodeReviewAnnotation,
		CapabilityPromptLint,
		CapabilityRationaleGeneration,
		CapabilityStrategySummary,
		CapabilityRiskSurfaceExtraction,
		CapabilityRegimeExplanation,
		CapabilityScenarioSimulation,
		CapabilitySentimentExplanation,
		CapabilityPerformanceForensics,
		CapabilityContraAttribution,
		CapabilityConfidenceCommentary,
	}
	for _, cap := range caps {
		if _, ok := cfg.RoutingChains[cap]; !ok {
			t.Errorf("missing capability %q in loaded config", cap)
		}
	}
}
