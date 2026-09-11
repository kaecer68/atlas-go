package llm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"

	obsotel "github.com/kaecer68/atlas-go/internal/observability/otel"
)

// Router defines the capability-based request routing interface.
// Call dispatches a request to the appropriate provider, following the
// routing chain for the request's capability. Health reports the health
// status of all registered providers. Register adds a ProviderImpl to the
// router's provider set (keyed by the provider's Health().Provider value),
// enabling it for future Call dispatches.
type Router interface {
	Call(ctx context.Context, req Request) (Response, error)
	Health() map[Provider]HealthStatus
	Register(p ProviderImpl) error
}

// DefaultRouter implements Router with a capability-based multi-provider
// fallback chain strategy. It consults a hard-coded routing table (§6.1)
// to determine the provider priority order for each Capability, then
// attempts providers in sequence (Primary → Backup1 → Backup2) until one
// succeeds. If all chain members fail, a last-resort handler produces a
// deterministic fallback response.
//
// Selection criteria: capability achievability plus subscription quota.
// DataClass is carried through as audit metadata (metric/span attribute,
// future redaction input) and does NOT gate any provider — see ADR-012 and
// docs/specs/llm-routing-spec.md §6.3.
//
// A provider response with an empty (or whitespace-only) Output and no
// ToolCalls is treated as a failure, so the router continues to the next
// chain member instead of returning a silent empty success.
// Providers are injected via NewDefaultRouter.
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
	return NewDefaultRouterFromConfig(defaultRoutingTable(), impls...)
}

// NewDefaultRouterFromConfig creates a DefaultRouter with the given routing
// configuration and provider implementations. Use this when loading routing
// from a config file (e.g. configs/llm_router.yaml) via LoadRouterConfig or
// TryLoadRouterConfig.
func NewDefaultRouterFromConfig(config RouterConfig, impls ...ProviderImpl) *DefaultRouter {
	providers := make(map[Provider]ProviderImpl, len(impls))
	for _, impl := range impls {
		providers[impl.Health().Provider] = impl
	}
	return &DefaultRouter{
		providers:    providers,
		routingTable: config,
	}
}

// Register adds a ProviderImpl to the router's provider set, keyed by
// p.Health().Provider. Idempotent: registering an already-registered
// provider overwrites the existing entry silently.
func (r *DefaultRouter) Register(p ProviderImpl) error {
	r.providers[p.Health().Provider] = p
	return nil
}

// Call dispatches a request through the routing chain. The dispatch order is:
//
//  1. If req.Options.ForceProvider is set, route directly to that provider
//     (bypassing the routing table and the Supports check).
//  2. Look up the RoutingChain for req.Capability.
//  3. Try chain members in order: Primary → Backup1 → Backup2.
//     Empty-string Backup2 entries are skipped (3-tier fallback when
//     Backup2 is intentionally empty).
//  4. Skip a provider if it is not registered or does not Support the capability.
//  5. On each failure, append the provider to attempted and continue.
//     A provider error AND a success with empty (whitespace-only) Output
//     and no ToolCalls both count as failures (ADR-012).
//  6. If all available chain members fail, invoke the lastResortHandler.
func (r *DefaultRouter) Call(ctx context.Context, req Request) (Response, error) {
	spanAttrs := []attribute.KeyValue{
		attribute.String("llm.capability", string(req.Capability)),
		attribute.String("llm.data_class", strconv.Itoa(int(req.DataClass))),
		// Human-readable DataClass for audit/redaction review. The numeric
		// attribute above is kept unchanged for existing dashboards. Since
		// ADR-012 DataClass no longer gates any provider, so this span
		// attribute is the primary place the classification stays visible.
		attribute.String("llm.data_class_name", req.DataClass.String()),
	}
	if req.Options.ForceProvider != nil {
		spanAttrs = append(spanAttrs, attribute.String("llm.forced_provider", string(*req.Options.ForceProvider)))
	}
	ctx, span := obsotel.StartSpan(ctx, "llm."+string(req.Capability), spanAttrs...)
	defer span.End()

	// Step 1: ForceProvider bypasses routing table.
	//
	// Deliberate asymmetry with the chain path: an empty output from a forced
	// provider is returned as-is. A forced call has no next chain member to
	// fall through to, and ForceProvider is a test/sticky-routing escape hatch
	// rather than a production capability path.
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

	// Steps 3-5: Try Primary → Backup1 → Backup2 in order.
	// Effective chain length depends on routing config: empty Backup2 (e.g.,
	// for [PLANNED] OpenCode providers) reduces to 2 tiers. The router
	// iteration tolerates empty-string entries by skipping them via the
	// "not registered" branch below.
	chainProviders := []Provider{chain.Primary, chain.Backup1, chain.Backup2}
	var attempted []Provider

	for i, providerName := range chainProviders {
		// Skip empty-string entries (4-tier → 3-tier fallback when Backup2
		// is intentionally empty per configs/llm_router.yaml).
		if providerName == "" {
			continue
		}

		// Increment fallback counter when trying a backup (not primary)
		if i > 0 {
			atomic.AddInt64(FallbackTriggeredTotal, 1)
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
		// An empty (whitespace-only) Output with no tool calls is a silent
		// failure, not a success: reasoning models can burn the whole
		// max_tokens budget in their thinking phase and return an empty
		// message with HTTP 200. Treat it as a failure so the next chain
		// member gets a chance (ADR-012).
		if isBlankResponse(resp) {
			continue
		}

		providerNames := make([]string, len(attempted))
		for i, p := range attempted {
			providerNames[i] = string(p)
		}
		span.SetAttributes(attribute.StringSlice("llm.attempted_providers", providerNames))
		resp.AttemptedProviders = attempted
		return resp, nil
	}

	// Step 7: All chain members exhausted — invoke last-resort handler
	atomic.AddInt64(BackupChainExhaustedTotal, 1)
	span.SetAttributes(attribute.Bool("llm.exhausted", true))
	return r.lastResortHandler(attempted), nil
}

// isBlankResponse reports whether a provider response carries no usable
// payload: Output is empty or whitespace-only AND the provider requested no
// tool calls. Tool-call-only responses legitimately have an empty Output, so
// they must not be treated as failures.
func isBlankResponse(resp Response) bool {
	if len(resp.ToolCalls) > 0 {
		return false
	}
	return strings.TrimSpace(resp.Output) == ""
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
// mapping as defined in docs/specs/llm-routing-spec.md §6.1 (ADR-012).
//
// Selection rule (ADR-012): provider priority follows per-task achievability
// plus subscription quota, not data residency. DataClass is carried as audit
// metadata only.
//
// Chain groups:
//   - Narrative / explanation JSON (primary MiniMax M3): the M3 subscription
//     quota covers繁中 narrative and financial explanation at zero marginal
//     cost, and M3 leads on Traditional Chinese financial narrative.
//   - Code (primary Kimi kimi-for-coding): K2.7 is the code-specialized model
//     and is the only provider whose Supports() allows those capabilities
//     (ADR-009). When no kimi key is configured the router falls through to
//     MiniMax.
//   - Global fallback is ProviderDeepSeek, whose adapter is wired with the
//     canonical model name `deepseek-flash` (= DeepSeek-V4.1-Flash).
//     `deepseek-v4-pro` is retired and must not appear in new code/config.
//
// Mapping notes:
//   - The doc specifies per-capability last-resort behaviors (rule_based,
//     passthrough, null, pass, discard, empty). Phase 1 normalizes all
//     last-resort handling to ProviderMock with empty Output; the per-
//     capability behaviors are deferred to Phase 2 alongside the
//     capability-specific handlers.
//   - Backup2 is intentionally empty (Wave 11 L2.1 doc audit, Issue #720):
//     ProviderOpenCodeGo/Zen are reserved constants for future use but no
//     client implementation exists in internal/llm/clients/. The router
//     skips empty-string providers gracefully (router.go:Call).
//   - This table MUST stay in sync with configs/llm_router.yaml;
//     TestDefaultRoutingTable_MatchesYAML enforces it.
func defaultRoutingTable() RouterConfig {
	return RouterConfig{
		RoutingChains: map[Capability]RoutingChain{
			// doc §6.1: strategy.failure_attribution
			//   M3 → deepseek-flash → rule_based
			CapabilityFailureAttribution: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: dev.code_review_annotation
			//   kimi-for-coding → M3 → deepseek-flash → empty
			CapabilityCodeReviewAnnotation: {
				Primary:    ProviderKimi,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderDeepSeek,
				LastResort: ProviderMock,
			},
			// doc §6.1: dev.prompt_lint
			//   kimi-for-coding → M3 → deepseek-flash → pass
			CapabilityPromptLint: {
				Primary:    ProviderKimi,
				Backup1:    ProviderMiniMax,
				Backup2:    ProviderDeepSeek,
				LastResort: ProviderMock,
			},
			// doc §6.1: narrative.rationale_translation_fallback
			//   M3 → deepseek-flash → passthrough
			CapabilityRationaleGeneration: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: strategy.frame_summary
			//   M3 → deepseek-flash → null
			CapabilityStrategySummary: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: spawning.gap_description_enrichment
			//   M3 → deepseek-flash → passthrough
			CapabilityRiskSurfaceExtraction: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: narrative.event_headline
			//   M3 → deepseek-flash → passthrough
			CapabilityRegimeExplanation: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: risk.confidence_calibration_commentary
			//   M3 → deepseek-flash → passthrough
			CapabilityPerformanceForensics: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: orchestrator.prism_cohort_insight
			//   M3 → deepseek-flash → discard
			CapabilityScenarioSimulation: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// doc §6.1: narrative.sentiment_explanation
			//   M3 → deepseek-flash → passthrough
			CapabilitySentimentExplanation: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// Not in doc §6.1; Phase 2 capability set (adversarial narrative
			// analysis — routed with the narrative group).
			CapabilityContraAttribution: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
			// Phase 3.3: risk.confidence_commentary (non-blocking bypass)
			CapabilityConfidenceCommentary: {
				Primary:    ProviderMiniMax,
				Backup1:    ProviderDeepSeek,
				Backup2:    "",
				LastResort: ProviderMock,
			},
		},
	}
}
