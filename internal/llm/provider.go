// Package llm provides the core types, constants, and interfaces for LLM integration
// within the Atlas trading system. It defines the public API surface for capability-based
// routing across multiple LLM providers.
//
// Design authority: docs/llm-integration-strategy-framework.md §4 (interface contract)
// and §6.1 (routing table).
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
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
	CapabilitySentimentExplanation  Capability = "sentiment_explanation"
	CapabilityPerformanceForensics  Capability = "performance_forensics"
	CapabilityContraAttribution     Capability = "contra_attribution"
	CapabilityConfidenceCommentary  Capability = "confidence_commentary"
)

// Provider identifies a specific LLM provider implementation.
// Each Provider value corresponds to a concrete implementation that can be
// instantiated and used through the ProviderImpl interface.
type Provider string

// Provider constants enumerate all supported LLM provider implementations.
// The ProviderOpenAI constant is deprecated and retained only for backward
// compatibility; do not use it in new code. All new integrations should
// use one of the other explicitly named providers.
//
// ProviderOpenCodeGo and ProviderOpenCodeZen are [PLANNED] constants
// reserved for future client implementations (Wave 11 L2.1 doc audit,
// Issue #720). They are not registered in the default routing chain because
// no client implementation exists in internal/llm/clients/. To enable
// them, implement OpenCodeGoClient / OpenCodeZenClient and register via
// NewDefaultRouter(...).
const (
	ProviderKimi     Provider = "kimi"
	ProviderMiniMax  Provider = "minimax"
	ProviderDeepSeek Provider = "deepseek"
	// [PLANNED] Reserved constant; no client implementation. Do not
	// register this provider in DefaultRouter until OpenCodeGoClient exists.
	ProviderOpenCodeGo Provider = "opencode_go"
	// [PLANNED] Reserved constant; no client implementation. Do not
	// register this provider in DefaultRouter until OpenCodeZenClient exists.
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
	DataClassUnmarked     DataClass = iota // 0
	DataClassNonRegulated                  // 1
	DataClassRegulated                     // 2
	DataClassSecret                        // 3
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

	// Tools is the list of tools/functions the model may invoke.
	// Empty slice means no tools (regular chat). Each provider adapter
	// is responsible for serializing this into its native tools format
	// (e.g. OpenAI "tools" field).
	Tools []Tool

	// ToolChoice controls how the model selects tools. Valid values:
	// "none" — model must not call any tool
	// "auto" — model decides (default when Tools non-empty)
	// "required" — model must call at least one tool
	// "<tool_name>" — model must call the named tool
	// Empty string means provider default (typically "auto" if Tools).
	// Call Request.Validate() to enforce this contract before dispatch.
	ToolChoice string
}

// Validate checks the Request for configuration errors. Provider
// adapters should call Validate before dispatching; on nil return,
// adapters may trust ToolChoice (and other validated fields) without
// re-checking.
//
// Issue #711 #11: moves ToolChoice value validation out of provider
// adapters and into the Request itself. ToolChoice is valid if:
//   - "" (empty) — provider default
//   - "none" / "auto" / "required" — reserved keywords
//   - a tool name matching one of r.Tools[].Name
//
// Returns a descriptive error if the value is not in any of those
// categories. The error message includes the registered tool names
// (if any) to help callers diagnose typos.
func (r *Request) Validate() error {
	if r.ToolChoice == "" {
		return nil // provider default
	}
	switch r.ToolChoice {
	case "none", "auto", "required":
		return nil
	}
	// Otherwise must match a registered tool name.
	for _, tool := range r.Tools {
		if tool.Name == r.ToolChoice {
			return nil
		}
	}
	return fmt.Errorf("llm.Request.Validate: ToolChoice %q is not a reserved keyword (none/auto/required) and does not match any registered tool name (registered: %v)", r.ToolChoice, toolNames(r.Tools))
}

// toolNames returns the names of the given tools, for use in error
// messages. Package-private helper for Request.Validate.
func toolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// Tool describes a single tool/function the LLM may invoke.
// Modeled on the OpenAI function-calling schema; compatible with most
// OpenAI-compatible providers (DeepSeek, MiniMax M3, Kimi K2.7).
type Tool struct {
	// Name is the function name the LLM emits in its tool_call.
	Name string

	// Description is shown to the LLM so it knows when to call this tool.
	Description string

	// InputSchema is a JSON Schema (Zod-compatible) describing the
	// function's argument shape. Stored as raw JSON to keep the LLM
	// package free of struct-codegen dependencies.
	InputSchema json.RawMessage

	// Handler is the Go-side executor for this tool. It receives the
	// raw JSON arguments and returns either a JSON result or an error.
	//
	// **LLM validation is a hint, not a guarantee.** The LLM's compliance
	// with InputSchema is probabilistic — the Handler MUST validate
	// arguments itself before using them. See SafeInvokeHandler for the
	// recommended invocation pattern (it also recovers from panics).
	Handler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// ToolCall is a single tool invocation emitted by the LLM.
type ToolCall struct {
	// ID uniquely identifies this tool call (for correlation with
	// tool result messages in multi-turn flows).
	ID string

	// Name is the tool's registered name (must match a Tool.Name
	// in the request).
	Name string

	// Arguments is the raw JSON arguments the LLM produced.
	Arguments json.RawMessage
}

// InputArgsFactory creates a zero-value instance of the typed args struct
// that a tool's handler expects. This enables typed-binding on top of the
// raw json.RawMessage Handler signature: production code calls BindTypedArgs
// with the factory and a typed handler, then assigns the result to Tool.Handler.
//
// Phase 1 introduces the type and BindTypedArgs helper; the sector_agent_llm
// tool-dispatch path (PR5a) will use it. No production tool is required to
// migrate to typed binding — RawMessage-based handlers remain fully supported.
type InputArgsFactory[T any] func() T

// BindTypedArgs wraps a typed handler (ctx, In) -> (Out, error) in a raw JSON
// adapter that satisfies the Tool.Handler signature. The framework calls the
// returned closure with json.RawMessage; BindTypedArgs unmarshals into In,
// invokes the typed handler, and re-marshals Out back to RawMessage.
//
// Two type parameters (In, Out) reflect the common L2.3 use case where the
// args and result structs are different types (e.g. get_weatherArgs ->
// weatherResult). Unmarshal errors and typed-handler errors are wrapped with
// the tool name so logs and error chains identify which tool failed.
func BindTypedArgs[In, Out any](toolName string, factory InputArgsFactory[In], handler func(ctx context.Context, args In) (Out, error)) func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		in := factory()
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, fmt.Errorf("tool %q: typed args unmarshal: %w", toolName, err)
		}
		out, err := handler(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", toolName, err)
		}
		marshaled, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("tool %q: typed result marshal: %w", toolName, err)
		}
		return marshaled, nil
	}
}

// SafeInvokeHandler invokes t.Handler with panic recovery. A panicking
// handler is converted to a regular error so one buggy tool cannot crash
// the LLM driver. The recovered stack is logged via slog for post-mortem.
//
// This is the recommended invocation pattern for all production tool
// dispatchers (see internal/orchestrator for usage). Tests may call
// t.Handler directly since panics in tests fail the test.
func SafeInvokeHandler(ctx context.Context, t *Tool, args json.RawMessage) (result json.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Error(
				"tool handler panic recovered",
				"tool", t.Name,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(stack),
			)
			err = fmt.Errorf("tool %q handler panicked: %v", t.Name, r)
			result = nil
		}
	}()
	return t.Handler(ctx, args)
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

	// ToolCalls lists the tool invocations the LLM requested.
	// Empty when the model responded with text only. For the
	// CapabilityFunctionCalling flow, this is the primary output.
	ToolCalls []ToolCall
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
