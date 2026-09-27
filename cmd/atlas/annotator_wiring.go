package main

import (
	"github.com/kaecer68/atlas-go/internal/llm"
	llmAdapters "github.com/kaecer68/atlas-go/internal/llm/adapters"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// annotatorWiring is the observable outcome of setupStrategiesAnnotator. It is
// returned (instead of logged inline) so the wiring decision can be asserted by
// tests — the production path must never silently fall back to a direct
// provider client.
type annotatorWiring struct {
	// Enabled is true when an annotate backend was wired. When false the
	// endpoint answers 503 ("llm annotator not configured").
	Enabled bool
	// Backend is the wired annotator's Name() (empty when not enabled).
	Backend string
	// Reason is a machine-readable tag for the decision: "router",
	// "no_dashboard", "router_unavailable" or
	// "no_failure_attribution_provider".
	Reason string
}

// annotateWiringDeps carries the inputs of the production annotate wiring.
type annotateWiringDeps struct {
	// Dashboard receives the annotate backend and the cost usage source.
	Dashboard *monitoring.DashboardAPI
	// Router is the application LLM Router. It MUST be the same router the
	// other LLM capabilities use, so /annotate shares one prompt, one
	// capability chain and one fallback policy.
	Router llm.Router
	// Config is the router's effective routing table (llm.ResolveRouterConfig).
	// Wiring reads it to decide whether the failure_attribution chain can be
	// served at all.
	Config llm.RouterConfig
	// Legacy is the pre-#1897 llm_annotator.KimiClient, or nil when no
	// annotator API key is configured.
	//
	// It MUST NOT serve /annotate — routing it there is exactly the bypass
	// issue #1897 removes (see docs/reference/traps.md: "LLM 路由繞過"). It is
	// kept for one non-LLM purpose: it is the read-only usage source behind
	// GET /api/llm_annotator/cost.
	Legacy *llm_annotator.KimiClient
}

// setupStrategiesAnnotator wires POST /api/strategies/{id}/annotate.
//
// Issue #1897: the endpoint runs on the LLM Router. Both entry points into
// failure attribution — this HTTP endpoint and the Router capability handler —
// therefore share one prompt (llm_annotator.FailureAttributionSystemPrompt), one
// request shape (chat messages), and one fallback chain (minimax → deepseek).
//
// Response contract (unchanged by this migration):
//   - wired and the chain answered        → 200 {id, annotation, backend}
//   - wired, chain exhausted / empty text → 502 {error, fallback, backend}
//   - not wired (no chain provider)       → 503 {error, fallback}
//
// The "not wired" case is preserved deliberately: the endpoint must keep
// signaling "no LLM configured" with 503 rather than 502, which is what a
// RouterAnnotator over a provider-less Router would produce.
func setupStrategiesAnnotator(deps annotateWiringDeps) annotatorWiring {
	if deps.Dashboard == nil {
		return annotatorWiring{Reason: "no_dashboard"}
	}

	// Cost endpoint source. Set independently of the annotate backend so the
	// two concerns cannot drift back into one concrete-type dependency.
	// deps.Legacy is a pointer comparison: a nil *KimiClient is never passed
	// into the UsageReporter interface (typed-nil would panic, see
	// DashboardAPI.SetAnnotatorUsageSource).
	if deps.Legacy != nil {
		deps.Dashboard.SetAnnotatorUsageSource(deps.Legacy)
	}

	if deps.Router == nil {
		return annotatorWiring{Reason: "router_unavailable"}
	}
	if !failureAttributionRouterReady(deps.Router, deps.Config) {
		return annotatorWiring{Reason: "no_failure_attribution_provider"}
	}

	annotator := llmAdapters.NewRouterAnnotator(deps.Router)
	deps.Dashboard.SetStrategiesAnnotator(annotator)

	return annotatorWiring{
		Enabled: true,
		Backend: annotator.Name(),
		Reason:  "router",
	}
}

// failureAttributionRouterReady reports whether r can actually serve
// llm.CapabilityFailureAttribution, i.e. whether at least one member of the
// capability's chain is registered with the router.
//
// A registered provider is the necessary condition: the router skips
// unregistered chain members and (with every member skipped) the last-resort
// handler answers with an empty output, which /annotate reports as 502. That
// would turn "no LLM configured" (503) into "LLM failed" (502), so readiness is
// checked before the annotator is wired.
//
// Readiness is a superset test, not a proof: a registered provider may still
// refuse the capability (ProviderImpl.Supports). Wired-but-off-chain providers
// are deliberately NOT counted — e.g. the legacy AnnotatorAdapter registers
// itself as ProviderKimi, which no chain contains, and it must not make
// /annotate look configured.
func failureAttributionRouterReady(r llm.Router, cfg llm.RouterConfig) bool {
	if r == nil {
		return false
	}
	chain, ok := cfg.RoutingChains[llm.CapabilityFailureAttribution]
	if !ok {
		return false
	}
	registered := r.Health()
	for _, p := range []llm.Provider{chain.Primary, chain.Backup1, chain.Backup2} {
		if p == "" {
			continue
		}
		if _, ok := registered[p]; ok {
			return true
		}
	}
	return false
}
