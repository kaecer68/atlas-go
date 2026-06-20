package llm

import (
	"context"
	"errors"
	"sync/atomic"
)

// Router defines the capability-based request routing interface.
// Call dispatches a request to the appropriate provider, following the
// routing chain for the request's capability. Health reports the health
// status of all registered providers.
type Router interface {
	Call(ctx context.Context, req Request) (Response, error)
	Health() map[Provider]HealthStatus
}

// DefaultRouter implements Router with a capability-based multi-provider
// fallback chain strategy. It consults a hard-coded routing table (§6.1)
// to determine the provider priority order for each Capability, then
// attempts providers in sequence (Primary → Backup1 → Backup2) until one
// succeeds. If all chain members fail, a last-resort handler produces a
// deterministic fallback response.
//
// DefaultRouter enforces a DataClass gate: ProviderMiniMax is skipped for
// DataClassRegulated requests (the hosted M3 path must be avoided for
// regulated data). Providers are injected via NewDefaultRouter.
type DefaultRouter struct {
	providers    map[Provider]ProviderImpl
	routingTable RouterConfig
}

// Package-level counters for internal observability.
// These are plain int64 pointers (not expvar-registered); they can be
// read by tests and replaced by production metric registration later.
var (
	FallbackTriggeredTotal    = new(int64)
	BackupChainExhaustedTotal = new(int64)
)

// ErrProviderNotFound is returned when ForceProvider targets a provider
// that is not registered in the DefaultRouter.
var ErrProviderNotFound = errors.New("llm: forced provider not found in registered providers")

// NewDefaultRouter creates a DefaultRouter populated with the given providers
// and the default routing table. Accepts variadic ProviderImpl arguments for
// dependency injection. Providers are keyed by their Health().Provider value.
func NewDefaultRouter(impls ...ProviderImpl) *DefaultRouter {
	providers := make(map[Provider]ProviderImpl, len(impls))
	for _, impl := range impls {
		providers[impl.Health().Provider] = impl
	}
	return &DefaultRouter{
		providers:    providers,
		routingTable: defaultRoutingTable(),
	}
}

// Call dispatches a request through the routing chain. The dispatch order is:
//
//  1. If req.Options.ForceProvider is set, route directly to that provider
//     (bypassing the routing table, Supports check, and DataClass gate).
//  2. Look up the RoutingChain for req.Capability.
//  3. Try chain members in order: Primary → Backup1 → Backup2.
//  4. Skip a provider if the DataClass gate rejects it.
//  5. Skip a provider if it is not registered or does not Support the capability.
//  6. On each failure, append the provider to attempted and continue.
//  7. If all three chain members fail, invoke the lastResortHandler.
func (r *DefaultRouter) Call(ctx context.Context, req Request) (Response, error) {
	// Step 1: ForceProvider bypasses routing table
	if req.Options.ForceProvider != nil {
		impl, ok := r.providers[*req.Options.ForceProvider]
		if !ok {
			return Response{}, ErrProviderNotFound
		}
		resp, err := impl.Call(ctx, req)
		if err != nil {
			return Response{}, err
		}
		resp.AttemptedProviders = []Provider{*req.Options.ForceProvider}
		return resp, nil
	}

	// Step 2: Look up the routing chain
	chain, ok := r.routingTable.RoutingChains[req.Capability]
	if !ok {
		return Response{}, ErrCapabilityNotSupported
	}

	// Steps 3-5: Try Primary → Backup1 → Backup2 in order
	chainProviders := []Provider{chain.Primary, chain.Backup1, chain.Backup2}
	var attempted []Provider

	for i, providerName := range chainProviders {
		// Increment fallback counter when trying a backup (not primary)
		if i > 0 {
			atomic.AddInt64(FallbackTriggeredTotal, 1)
		}

		// Step 4: DataClass gate — skip MiniMax for regulated data
		if r.shouldGateProvider(providerName, req.DataClass) {
			continue
		}

		impl, ok := r.providers[providerName]
		if !ok {
			attempted = append(attempted, providerName)
			continue
		}

		// Check provider capability support
		if !impl.Supports(req.Capability) {
			attempted = append(attempted, providerName)
			continue
		}

		attempted = append(attempted, providerName)
		resp, err := impl.Call(ctx, req)
		if err != nil {
			continue
		}

		resp.AttemptedProviders = attempted
		return resp, nil
	}

	// Step 7: All chain members exhausted — invoke last-resort handler
	atomic.AddInt64(BackupChainExhaustedTotal, 1)
	return r.lastResortHandler(attempted), nil
}

// shouldGateProvider returns true when the given provider should be excluded
// from routing for the specified DataClass. Currently, only MiniMax is gated
// for DataClassRegulated (and higher) — the hosted M3 path must be avoided
// for regulated data.
func (r *DefaultRouter) shouldGateProvider(providerName Provider, dc DataClass) bool {
	return dc >= DataClassRegulated && providerName == ProviderMiniMax
}

// lastResortHandler produces a deterministic fallback response when all
// chain members have been exhausted. For CapabilityFailureAttribution, it
// returns an empty-string output with ProviderMock. For other known
// capabilities, it returns an empty-string output with ProviderMock as
// a safe fallback. Unknown capabilities are handled before this function
// is called (Call returns ErrCapabilityNotSupported).
func (r *DefaultRouter) lastResortHandler(attempted []Provider) Response {
	return Response{
		Output:             "",
		Provider:           ProviderMock,
		AttemptedProviders: attempted,
	}
}

// defaultRoutingTable returns the hard-coded capability-to-routing-chain
// mapping as defined in docs/llm-integration-strategy-framework.md §6.1.
//
// Mapping notes (Phase 1):
//   - The doc references "DeepSeek V4-Pro" and "DeepSeek V4-Flash" as
//     distinct providers. Phase 1 treats them both as ProviderDeepSeek
//     (the V4-Pro / V4-Flash distinction is a Phase 2 Provider-impl concern).
//   - The doc specifies per-capability last-resort behaviors (rule_based,
//     passthrough, null, pass, discard, empty). Phase 1 normalizes all
//     last-resort handling to ProviderMock with empty Output; the per-
//     capability behaviors are deferred to Phase 2 alongside the
//     capability-specific handlers.
//   - Capability names in code are normalized to the enum in provider.go;
//     the doc's dotted names map approximately.
func defaultRoutingTable() RouterConfig {
	return RouterConfig{
		RoutingChains: map[Capability]RoutingChain{
			// doc §6.1: strategy.failure_attribution
			//   V4-Pro → M3 → OpenCode-Go → rule_based
			CapabilityFailureAttribution: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: dev.code_review_annotation
			//   K2.7 → V4-Flash → OpenCode-Go → empty
			CapabilityCodeReviewAnnotation: {
				Primary:    ProviderKimi,
				Backup1:    ProviderDeepSeek,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: dev.prompt_lint
			//   V4-Flash → K2.7 → OpenCode-Go → pass
			CapabilityPromptLint: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderKimi,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: narrative.rationale_translation_fallback
			//   V4-Flash → M3 → OpenCode-Go → passthrough
			CapabilityRationaleGeneration: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: strategy.frame_summary
			//   V4-Pro → M3 → OpenCode-Go → null
			CapabilityStrategySummary: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: spawning.gap_description_enrichment
			//   M3 → V4-Pro → OpenCode-Go → passthrough
			CapabilityRiskSurfaceExtraction: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: narrative.event_headline
			//   V4-Flash → M3 → OpenCode-Go → passthrough
			CapabilityRegimeExplanation: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// doc §6.1: risk.confidence_calibration_commentary
			//   V4-Pro → M3 → OpenCode-Go → passthrough
			CapabilityPerformanceForensics: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// Not in doc §6.1; Phase 2 capability set.
			// doc §6.1: orchestrator.prism_cohort_insight
			//   M3 → V4-Pro → OpenCode-Go → discard
			// No matching enum yet; default chain mirrors the V4-Pro →
			// M3 → OpenCode-Go pattern, with PRISM executor out of
			// scope for Phase 1 (per ADR-003).
			CapabilityScenarioSimulation: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
			// Not in doc §6.1; Phase 2 capability set.
			CapabilityContraAttribution: {
				Primary:    ProviderDeepSeek,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderOpenCodeGo,
				LastResort: ProviderMock,
			},
		},
	}
}
