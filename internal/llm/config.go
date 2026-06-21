package llm

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// yamlRoutingChain mirrors RoutingChain with YAML-friendly field names.
type yamlRoutingChain struct {
	Primary    string `yaml:"primary"`
	Backup1    string `yaml:"backup1"`
	Backup2    string `yaml:"backup2"`
	LastResort string `yaml:"last_resort"`
}

// yamlRouterConfig is the YAML document root for configs/llm_router.yaml.
type yamlRouterConfig struct {
	RoutingChains map[string]yamlRoutingChain `yaml:"routing_chains"`
}

// LoadRouterConfig reads a YAML file at path and returns a validated
// RouterConfig. It validates that every capability name and provider name
// in the YAML file is known to this package. Unknown capabilities or
// providers return an error.
func LoadRouterConfig(path string) (RouterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RouterConfig{}, fmt.Errorf("llm: read router config %s: %w", path, err)
	}

	var raw yamlRouterConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return RouterConfig{}, fmt.Errorf("llm: parse router config %s: %w", path, err)
	}

	if len(raw.RoutingChains) == 0 {
		return RouterConfig{}, fmt.Errorf("llm: router config %s: routing_chains is empty", path)
	}

	chains := make(map[Capability]RoutingChain, len(raw.RoutingChains))
	for capName, yc := range raw.RoutingChains {
		cap := Capability(capName)
		if !isKnownCapability(cap) {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: unknown capability %q", path, capName)
		}

		primary := Provider(yc.Primary)
		backup1 := Provider(yc.Backup1)
		backup2 := Provider(yc.Backup2)
		lastResort := Provider(yc.LastResort)

		if !isKnownProvider(primary) {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: unknown provider %q for capability %q", path, yc.Primary, capName)
		}
		if yc.Backup1 != "" && !isKnownProvider(backup1) {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: unknown provider %q for capability %q", path, yc.Backup1, capName)
		}
		if yc.Backup2 != "" && !isKnownProvider(backup2) {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: unknown provider %q for capability %q", path, yc.Backup2, capName)
		}
		if yc.LastResort != "" && !isKnownProvider(lastResort) {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: unknown provider %q for capability %q", path, yc.LastResort, capName)
		}

		if primary == "" {
			return RouterConfig{}, fmt.Errorf("llm: router config %s: capability %q has empty primary provider", path, capName)
		}

		chains[cap] = RoutingChain{
			Primary:    primary,
			Backup1:    backup1,
			Backup2:    backup2,
			LastResort: lastResort,
		}
	}

	return RouterConfig{RoutingChains: chains}, nil
}

// TryLoadRouterConfig attempts to load a RouterConfig from path.
// On success, returns the loaded config. On any error, falls back to
// defaultRoutingTable() so the system always starts with a valid routing
// configuration. This is the preferred API for main.go wiring.
func TryLoadRouterConfig(path string) RouterConfig {
	config, err := LoadRouterConfig(path)
	if err != nil {
		return defaultRoutingTable()
	}
	return config
}

// isKnownCapability returns true when cap is one of the Capability
// constants defined in this package.
func isKnownCapability(cap Capability) bool {
	switch cap {
	case CapabilityFailureAttribution,
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
		CapabilityConfidenceCommentary:
		return true
	default:
		return false
	}
}

// isKnownProvider returns true when p is one of the Provider constants
// defined in this package.
func isKnownProvider(p Provider) bool {
	switch p {
	case ProviderKimi,
		ProviderMiniMax,
		ProviderDeepSeek,
		ProviderOpenCodeGo,
		ProviderOpenCodeZen,
		ProviderMock,
		ProviderOpenAI:
		return true
	default:
		return false
	}
}
