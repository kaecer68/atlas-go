// Package llm provides the core types, constants, and interfaces for LLM integration
// within the Atlas trading system. It defines the public API surface for capability-based
// routing across multiple LLM providers.
//
// Design authority: docs/llm-integration-strategy-framework.md §4 (interface contract)
// and §6.1 (routing table).
package llm

import (
	"context"
	"errors"
	"time"
)

// Capability defines the functional capabilities that an LLM provider can fulfill.
// Each capability represents a distinct mode of LLM-assisted analysis or generation
// within the Atlas system. See docs/llm-integration-strategy-framework.md §4.2.
//
// The Capability type is used for routing decisions: the Router consults the
// ProviderImpl.Supports method to determine which providers can handle a given
// request's Capability field.
type Capability string

// Capability constants enumerate all supported LLM capabilities.
// These values are derived from the routing table in §6.1 of the framework document.
// Typed string constants with explicit sequential values.
const (
	CapabilityFailureAttribution    Capability = "failure_attribution"
	CapabilityCodeReviewAnnotation  Capability = "code_review_annotation"
	CapabilityPromptLint            Capability = "prompt_lint"
	CapabilityRationaleGeneration   Capability = "rationale_generation"
	CapabilityStrategySummary       Capability = "strategy_summary"
	CapabilityRiskSurfaceExtraction Capability = "risk_surface_extraction"
	CapabilityRegimeExplanation     Capability = "regime_explanation"
	CapabilityScenarioSimulation    Capability = "scenario_simulation"
	CapabilityPerformanceForensics  Capability = "performance_forensics"
	CapabilityContraAttribution     Capability = "contra_attribution"
)

// Provider identifies a specific LLM provider implementation.
// Each Provider value corresponds to a concrete implementation that can be
// instantiated and used through the ProviderImpl interface.
type Provider string

// Provider constants enumerate all supported LLM provider implementations.
// The ProviderOpenAI constant is deprecated and retained only for backward
// compatibility; do not use it in new code. All new integrations should
// use one of the other explicitly named providers.
const (
	ProviderKimi        Provider = "kimi"
	ProviderMiniMax     Provider = "minimax"
	ProviderDeepSeek    Provider = "deepseek"
	ProviderOpenCodeGo  Provider = "opencode_go"
	ProviderOpenCodeZen Provider = "opencode_zen"
	ProviderMock        Provider = "mock"
	// DEPRECATED: retained for backward compatibility; do not use in new code.
	ProviderOpenAI Provider = "openai"
)

// DataClass classifies the sensitivity level of data passed through the LLM request.
// This classification drives data governance decisions: which providers are authorized
// to receive data of a given classification, and whether additional safeguards (e.g.,
// encryption, audit logging) must be applied.
type DataClass int

// DataClass constants follow a four-tier sensitivity ladder:
//   - Unmarked:   no classification; treated as public
//   - NonRegulated: non-sensitive business data; no regulatory constraints
//   - Regulated:  data subject to regulatory controls (e.g., PII, financial data)
//   - Secret:     highly sensitive data requiring maximum safeguards
const (
	DataClassUnmarked     DataClass = iota + 1 // 1
	DataClassNonRegulated                      // 2
	DataClassRegulated                         // 3
	DataClassSecret                            // 4
)

// Request encapsulates a single LLM inference request.
// The Router receives a Request, selects a capable Provider via the routing
// table (§6.1), and dispatches the request through ProviderImpl.Call.
type Request struct {
	// Capability is the required capability for this request.
	// The Router uses this field to consult RoutingChain entries for the
	// corresponding capability.
	Capability Capability

	// Payload is the input data for the LLM. The concrete type is
	// capability-specific; callers should ensure the Payload type is
	// compatible with the target provider's expectations.
	Payload any

	// Options provides fine-grained control over the LLM call, including
	// provider override, timeout, temperature, and retry behavior.
	Options Options

	// DataClass classifies the sensitivity of the Payload and any
	// derived data. Providers must enforce data governance requirements
	// commensurate with the DataClass level.
	DataClass DataClass
}

// Response holds the result of a successful LLM inference.
// It is returned by ProviderImpl.Call on success. The Response is
// always populated; errors are returned only for failure cases.
type Response struct {
	// Output is the textual output produced by the LLM provider.
	// The format and content of Output is capability-specific.
	Output string

	// Provider identifies which provider fulfilled this request.
	// When a fallback chain is used (see RoutingChain), AttemptedProviders
	// records all providers that were tried before success.
	Provider Provider

	// Usage reports token consumption and cost for this request.
	Usage Usage

	// Latency is the wall-clock duration of the inference call,
	// measured from the moment the request was dispatched to the
	// provider until the response was received.
	Latency time.Duration

	// CacheHit indicates whether the response was served from a
	// cache rather than computed fresh. When true, Usage and Latency
	// fields may reflect cached values rather than live computation.
	CacheHit bool

	// AttemptedProviders lists all providers that were tried in order
	// before this response was obtained. The last entry is the provider
	// that produced this Response; earlier entries are fallbacks that
	// failed. This field is populated by the Router, not by providers.
	AttemptedProviders []Provider

	// Trace is an optional structured log of internal decision points,
	// intermediate results, and timing annotations. It is populated
	// when Request.Options.Trace is true.
	Trace map[string]any
}

// Usage reports token usage and cost for a single LLM inference call.
// All token counts are reported as supplied by the underlying provider's
// usage metadata. CostUSD is the provider-reported cost in US dollars.
type Usage struct {
	InputTokens  int64   // Number of input (prompt) tokens consumed.
	OutputTokens int64   // Number of output (completion) tokens generated.
	TotalTokens  int64   // Sum of InputTokens and OutputTokens.
	CostUSD      float64 // Cost in USD as reported by the provider.
}

// Options controls the behavior of a single LLM call.
// Zero-value Options uses sensible defaults for all fields.
type Options struct {
	// ForceProvider, if non-nil, overrides the Router's routing table
	// and dispatches the request directly to the specified provider.
	// Use this for testing, debugging, or when a specific provider
	// is required by business logic.
	ForceProvider *Provider

	// Trace, when true, instructs the Router and provider to populate
	// the Response.Trace map with structured diagnostic information.
	Trace bool

	// Timeout is the maximum duration allowed for the call. If zero,
	// a provider-specific default is used. A timeout applies to a
	// single provider attempt; the Router's retry logic operates
	// independently of this field.
	Timeout time.Duration

	// Temperature controls the randomness of the LLM output.
	// Valid values are provider-dependent; typical ranges are 0.0–2.0.
	// A value of 0.0 requests deterministic output (if supported).
	Temperature float64

	// MaxTokens caps the number of tokens in the LLM output.
	// If zero, the provider's default is used.
	MaxTokens int

	// RetryAttempts controls how many times a failed provider call is
	// retried before the Router abandons the routing chain and returns
	// ErrAllProvidersFailed. The Router retries only on transient errors;
	// capability mismatches are not retried.
	RetryAttempts int
}

// ProviderImpl is the interface implemented by each concrete LLM provider.
// All providers must implement all three methods. Providers are stateless;
// any state (caches, rate limiters, circuit breakers) is maintained by
// the Router, not by the provider implementation itself.
//
// Implementors should be safe for concurrent use by multiple goroutines.
type ProviderImpl interface {
	// Supports reports whether this provider can fulfill the given capability.
	// The Router calls Supports to determine eligibility during routing.
	// A provider that returns false for a capability will never be selected
	// for requests with that Capability, even if it is listed in a RoutingChain.
	Supports(cap Capability) bool

	// Call dispatches the request to the underlying LLM and returns the response.
	// Call is called by the Router after it has selected this provider via Supports.
	// The context carries timeout and cancellation from the Router; providers
	// should respect context cancellation and return context-related errors promptly.
	Call(ctx context.Context, req Request) (Response, error)

	// Health returns the current health status of the provider.
	// The Router uses Health to implement circuit-breaking: if BreakerOpen is true,
	// the provider is excluded from routing even if it would otherwise Support the capability.
	Health() HealthStatus
}

// HealthStatus reports the operational state of a single provider.
// The Router polls Health() to update its internal breaker state; the
// HealthStatus struct is not directly consumed by callers of the Router.
type HealthStatus struct {
	Provider    Provider  // The provider this status pertains to.
	Healthy     bool      // True when the provider is operational.
	LastError   string    // Error message from the most recent failure; empty if last call succeeded.
	LastSuccess time.Time // Wall-clock time of the last successful call.
	BreakerOpen bool      // True when the circuit breaker has tripped and excluded this provider.
}

// RoutingChain defines the prioritized sequence of providers to try for a
// given capability. The Router attempts providers in order: Primary first,
// then Backup1, then Backup2, then LastResort. If all providers in the chain
// fail, the Router returns ErrAllProvidersFailed.
//
// The chain is derived from the routing table in §6.1 of the framework document.
// Each capability maps to exactly one RoutingChain.
type RoutingChain struct {
	Primary    Provider // First provider to try; the preferred choice.
	Backup1    Provider // Second provider; used if Primary fails.
	Backup2    Provider // Third provider; used if both Primary and Backup1 fail.
	LastResort Provider // Final provider; used only when all other chain members have failed.
}

// RouterConfig holds the complete routing configuration used by the Router.
// It maps each Capability to its corresponding RoutingChain. The Router
// consults this map when dispatching a request: given a Request with
// Capability C, the Router looks up RoutingChains[C] and iterates the
// chain members in order until a call succeeds or the chain is exhausted.
//
// RouterConfig is typically loaded from a configuration file or environment
// at startup; the Router itself does not modify the map after initialization.
type RouterConfig struct {
	// RoutingChains maps each Capability to its routing chain.
	// Every Capability defined in this package must have an entry.
	RoutingChains map[Capability]RoutingChain
}

// Sentinel errors returned by the Router and provider implementations.
var (
	// ErrCapabilityNotSupported is returned when the requested Capability
	// is not supported by any provider in the routing configuration.
	ErrCapabilityNotSupported = errors.New("llm: capability not supported by any provider")

	// ErrAllProvidersFailed is returned when all providers in the routing
	// chain for a capability have failed, even after retrying.
	ErrAllProvidersFailed = errors.New("llm: all providers in routing chain failed")

	// ErrProviderDisabled is returned when the Router's circuit breaker
	// has excluded the target provider. The request should be retried with
	// a different provider or after the breaker resets.
	ErrProviderDisabled = errors.New("llm: provider is disabled by circuit breaker")
)
