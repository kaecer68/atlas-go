package llm

import (
	"os"
	"path/filepath"
	"strings"
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte("{{{not yaml"), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte("routing_chains: {}\n"), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
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

// TestDefaultRoutingTable_MatchesYAML keeps configs/llm_router.yaml in sync with
// defaultRoutingTable() (ADR-012). The YAML mirror is currently not loaded by
// any cmd — llm.NewDefaultRouter() uses defaultRoutingTable(), and no caller
// invokes TryLoadRouterConfig — so a drift between the two would silently make
// the documented chain wrong.
func TestDefaultRoutingTable_MatchesYAML(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "llm_router.yaml")
	yamlCfg, err := LoadRouterConfig(path)
	if err != nil {
		t.Fatalf("LoadRouterConfig(%s) error = %v", path, err)
	}
	goCfg := defaultRoutingTable()

	if len(yamlCfg.RoutingChains) != len(goCfg.RoutingChains) {
		t.Fatalf("chain count mismatch: yaml=%d go=%d", len(yamlCfg.RoutingChains), len(goCfg.RoutingChains))
	}
	for cap, goChain := range goCfg.RoutingChains {
		yamlChain, ok := yamlCfg.RoutingChains[cap]
		if !ok {
			t.Errorf("capability %q missing from configs/llm_router.yaml", cap)
			continue
		}
		if yamlChain != goChain {
			t.Errorf("capability %q chain mismatch:\n  yaml = %+v\n  go   = %+v", cap, yamlChain, goChain)
		}
	}
	for cap := range yamlCfg.RoutingChains {
		if _, ok := goCfg.RoutingChains[cap]; !ok {
			t.Errorf("capability %q present in YAML but missing from defaultRoutingTable()", cap)
		}
	}
}

// TestDefaultRoutingTable_Groups pins the ADR-012 chain groups: narrative /
// explanation JSON capabilities start at MiniMax, code capabilities start at
// Kimi (ADR-009), and DeepSeek is the universal backup / global fallback.
func TestDefaultRoutingTable_Groups(t *testing.T) {
	cfg := defaultRoutingTable()

	narrative := []Capability{
		CapabilityFailureAttribution,
		CapabilityRationaleGeneration,
		CapabilityStrategySummary,
		CapabilityRiskSurfaceExtraction,
		CapabilityRegimeExplanation,
		CapabilityPerformanceForensics,
		CapabilityScenarioSimulation,
		CapabilitySentimentExplanation,
		CapabilityConfidenceCommentary,
		CapabilityContraAttribution,
	}
	for _, cap := range narrative {
		chain := cfg.RoutingChains[cap]
		if chain.Primary != ProviderMiniMax {
			t.Errorf("%s: primary = %q, want %q", cap, chain.Primary, ProviderMiniMax)
		}
		if chain.Backup1 != ProviderDeepSeek {
			t.Errorf("%s: backup1 = %q, want %q", cap, chain.Backup1, ProviderDeepSeek)
		}
	}

	// Code capabilities are M3-first: Kimi was removed from the chain on
	// 2026-09-12 (ADR-012 addendum 4) because the deployment's Kimi coding-plan
	// key cannot be used for app-level HTTP calls, which made a kimi primary
	// permanently unreachable. See TestDefaultRoutingTable_KimiNotInAnyChain.
	for _, cap := range []Capability{CapabilityCodeReviewAnnotation, CapabilityPromptLint} {
		chain := cfg.RoutingChains[cap]
		if chain.Primary != ProviderMiniMax {
			t.Errorf("%s: primary = %q, want %q", cap, chain.Primary, ProviderMiniMax)
		}
		if chain.Backup1 != ProviderDeepSeek {
			t.Errorf("%s: backup1 = %q, want %q", cap, chain.Backup1, ProviderDeepSeek)
		}
		if chain.Backup2 != "" {
			t.Errorf("%s: backup2 = %q, want empty (2-tier)", cap, chain.Backup2)
		}
	}

	// DeepSeek is the global fallback: it must appear somewhere in every chain.
	for cap, chain := range cfg.RoutingChains {
		if chain.Primary != ProviderDeepSeek && chain.Backup1 != ProviderDeepSeek && chain.Backup2 != ProviderDeepSeek {
			t.Errorf("%s: DeepSeek (global fallback) is absent from chain %+v", cap, chain)
		}
	}
}

// TestResolveRouterConfig_LoadsRepoFile verifies that the wiring path actually
// loads configs/llm_router.yaml (ADR-012 follow-up: the file used to be a
// mirror that no cmd read) and that the loaded table equals the built-in one.
func TestResolveRouterConfig_LoadsRepoFile(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "configs", "llm_router.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(RouterConfigPathEnv, path)

	cfg, src := ResolveRouterConfig()
	if !strings.HasPrefix(src, "file ") {
		t.Fatalf("source = %q, want the file to be used", src)
	}
	goCfg := defaultRoutingTable()
	if len(cfg.RoutingChains) != len(goCfg.RoutingChains) {
		t.Fatalf("chain count = %d, want %d", len(cfg.RoutingChains), len(goCfg.RoutingChains))
	}
	for cap, want := range goCfg.RoutingChains {
		if got := cfg.RoutingChains[cap]; got != want {
			t.Errorf("%s chain = %+v, want %+v", cap, got, want)
		}
	}
}

// TestResolveRouterConfig_MissingFileFallsBackToBuiltin verifies the fail-safe
// behavior: an unusable path never breaks startup.
func TestResolveRouterConfig_MissingFileFallsBackToBuiltin(t *testing.T) {
	t.Setenv(RouterConfigPathEnv, filepath.Join(t.TempDir(), "nope.yaml"))

	cfg, src := ResolveRouterConfig()
	if !strings.HasPrefix(src, "builtin") {
		t.Errorf("source = %q, want builtin fallback", src)
	}
	if len(cfg.RoutingChains) != len(defaultRoutingTable().RoutingChains) {
		t.Errorf("chain count = %d, want the built-in table", len(cfg.RoutingChains))
	}
}

// TestResolveRouterConfig_IncompleteFileFallsBackToBuiltin verifies that a
// partially-specified file cannot silently disable capabilities.
func TestResolveRouterConfig_IncompleteFileFallsBackToBuiltin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.yaml")
	yaml := `
routing_chains:
  failure_attribution:
    primary: minimax
    backup1: deepseek
    backup2: ""
    last_resort: mock
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RouterConfigPathEnv, path)

	cfg, src := ResolveRouterConfig()
	if !strings.HasPrefix(src, "builtin") || !strings.Contains(src, "missing") {
		t.Errorf("source = %q, want a builtin fallback mentioning the missing capabilities", src)
	}
	for _, cap := range allCapabilities() {
		if _, ok := cfg.RoutingChains[cap]; !ok {
			t.Errorf("fallback table is missing %s", cap)
		}
	}
}

// TestAllCapabilitiesMatchesConstants guards allCapabilities() against drift
// when a new Capability constant is added.
func TestAllCapabilitiesMatchesConstants(t *testing.T) {
	caps := allCapabilities()
	if len(caps) != 12 {
		t.Errorf("allCapabilities() returned %d entries, want 12", len(caps))
	}
	seen := make(map[Capability]bool, len(caps))
	for _, c := range caps {
		if seen[c] {
			t.Errorf("duplicate capability %q", c)
		}
		seen[c] = true
		if !isKnownCapability(c) {
			t.Errorf("%q is not known to isKnownCapability", c)
		}
	}
}

// TestDefaultRoutingTable_KimiNotInAnyChain pins the ADR-012 addendum 4
// decision: no capability routes to Kimi in this deployment, because the
// available Kimi subscription is a coding plan whose key cannot be used for
// app-level HTTP calls. Re-adding a kimi entry is a deliberate 2-file change
// (internal/llm/router.go + configs/llm_router.yaml) once a usable key exists.
func TestDefaultRoutingTable_KimiNotInAnyChain(t *testing.T) {
	for cap, chain := range defaultRoutingTable().RoutingChains {
		if chain.Primary == ProviderKimi || chain.Backup1 == ProviderKimi || chain.Backup2 == ProviderKimi {
			t.Errorf("%s: kimi must not appear in the chain (%+v)", cap, chain)
		}
	}
}
