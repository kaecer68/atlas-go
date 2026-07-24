// Package llm provides a capability-based multi-provider routing layer for
// large-language-model calls.
//
// This package is the core routing infrastructure for the LLM integration
// framework. It defines the Provider interface, Capability system, and Router
// that allow the atlas-go core to route LLM requests to different backends
// (Kimi, OpenAI, etc.) based on capability requirements and availability,
// with built-in health-aware fallback and async-safe hot-path guards.
//
// Design source: docs/llm-integration-strategy-framework.md
// Sub-packages:
//   - adapters/ — adapter layer consuming llm_annotator for strategy failure
//     attribution (the \"llm_annotated\" arm of the hybrid pipeline)
//   - capabilities/ — typed capability handlers (10 handlers)
//   - clients/ — HTTP client implementations (DeepSeek, MiniMax)
//   - schemas/ — typed I/O contracts for each capability
//
//
// Maturity rules: internal/MATURITY.md:75-89
//
// Public API surface:
//
//   - Router       — capability-based request router with health-aware routing
//   - Provider     — interface for LLM backend implementations
//   - Capability   — capability descriptor (model, latency class, context window)
//   - Request      — LLM call request (model, messages, temperature, etc.)
//   - Response     — LLM call response (content, usage, model, etc.)
//   - ProviderImpl — concrete provider implementations (KimiClient, etc.)
//   - HealthStatus — provider health snapshot (latency, error rate, availability)
//   - RoutingChain — ordered chain of providers to try in sequence
//   - RouterConfig — router configuration (providers, chains, health thresholds)
//   - DataClass    — data classification for capability routing
//   - Options        — per-request options (tracing, retry budget, etc.)
//   - Usage          — token usage record returned with every response
//   - LoadRouterConfig     — load a RouterConfig from a YAML file
//   - TryLoadRouterConfig  — load or fall back to defaultRoutingTable()
//   - NewDefaultRouterFromConfig — create a DefaultRouter with explicit config
//
// Hot-path guard: This package should not be imported directly by S/E-level
// modules; any hot-path call must be async or fallback-safe.
//
// Example:
//
//	router, err := llm.NewDefaultRouter(myProviderImpl)
//	if err != nil {
//		log.Fatal(err)
//	}
//	resp, err := router.Call(ctx, llm.Request{
//		Capability: llm.CapabilityFailureAttribution,
//		Payload:    myTypedPayload,
//		DataClass:  llm.DataClassNonRegulated,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(resp.Output)
//
// Maturity: experimental

package llm
