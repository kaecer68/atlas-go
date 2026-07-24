// Package llm_annotator provides LLM-powered natural-language annotations for
// strategy_techniques.StrategyFrame failure attribution. It is the
// "llm_annotated" arm of the hybrid self-correction pipeline
// (rule_based + llm_annotated, see strategy_techniques.AttributionMode).
//
// Status: archived / retained for backward compatibility. New code should use
// internal/llm/capabilities/failure_attribution via the llm.Router instead.
// The canonical failure-attribution capability lives in the llm/capabilities
// sub-package and is the owner of record for Wave 12+. See
// internal/MATURITY.md:118 and Issue #731 for the CircuitBreaker unification
// follow-up. This package is still imported by production wiring during the
// deprecation window; callers may continue to use it, but new capabilities
// should be added to internal/llm rather than here.
//
// The Annotator interface decouples the LLM provider from the consumer so
// production code can swap between Kimi (Moonshot API) and a test mock
// without touching the registry. Failure attribution is the only supported
// use case in Wave 4; the interface is intentionally narrow.
//
// The KimiClient requires an API key. Per the apigateway constitution, this
// key MUST come through the apigateway (no direct os.Getenv in production
// hot paths). For local development and CI, LLM_ANNOTATOR_API_KEY is
// permitted; production wiring should be:
//
//	cfg := llm_annotator.Config{APIKey: apigateway.MustGet("kimi"), ...}
//
// The default client enforces 1 request/second + burst of 4 via
// golang.org/x/time/rate.
//
// Annotate returns an error when the upstream fails (network, rate limit,
// non-2xx HTTP, malformed response). Callers MUST treat this as a fallback
// signal: the registry's rule_based attribution remains authoritative.
//
// Maturity: archived
package llm_annotator

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// FailureContext describes a single failed validation of a StrategyFrame.
// It is the input to Annotate and contains enough information for an LLM to
// produce a human-readable explanation of why the strategy did not hit.
type FailureContext struct {
	FrameID    string
	FrameName  string
	Layer      string
	Label      string
	Conditions []ConditionSnapshot
	Snap       MacroSnapshot
	OccurredAt time.Time
}

// ConditionSnapshot is a serializable view of a strategy_techniques.Condition
// at the moment of failure. Field values are the numeric/string the condition
// tests against; ActualValue is what the live data showed.
type ConditionSnapshot struct {
	Field       string
	Operator    string
	Threshold   float64
	ActualValue float64
	Timeframe   string
}

// MacroSnapshot is the slice of MacroDataSnapshot the LLM needs to reason
// about the failure. We do not pass the full snapshot to keep prompts
// compact; the consumer picks the fields it considers relevant.
type MacroSnapshot struct {
	ForeignInvestorNet  float64
	TSMADR              float64
	NVDA                float64
	DXY                 float64
	USD_TWD             float64
	RetailMarginBalance float64
	DomesticFundNet     float64
	DealerNet           float64
	VIX                 float64
	US10Y               float64
}

// Annotator produces a natural-language explanation for a single
// StrategyFrame failure. Implementations must be safe for concurrent use.
type Annotator interface {
	Annotate(ctx context.Context, fc FailureContext) (string, error)
	Name() string
}

// ErrUnavailable is returned when the annotator cannot reach its upstream.
// Callers should treat this as a fallback signal and fall back to
// rule_based attribution.
var ErrUnavailable = errors.New("llm annotator unavailable")

// Config bundles the parameters a real Annotator implementation needs.
// APIKey is required; the others have safe defaults applied in New.
//
// BudgetThreshold and BudgetCallback together configure a one-shot alert:
// when cumulative TotalTokens first meets or exceeds BudgetThreshold the
// callback is invoked exactly once with the current Usage snapshot. The
// callback is dispatched outside the client's internal lock so it may
// safely call back into the client (e.g. Usage()).
//
// Metrics, when set, receives counter/gauge observations for every
// Annotate call. NewKimiClient defaults to noopMetrics{} if Metrics is nil
// so callers can leave it unset without nil-checks at the call site.
type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	Timeout         time.Duration
	MaxTokens       int
	BudgetThreshold int64
	BudgetCallback  func(Usage)
	Metrics         MetricsRecorder
	Breaker         *CircuitBreaker
}

// WithDefaults returns a copy of cfg with zero-value fields filled in. It
// is the single source of truth for the package's defaults.
func (c Config) WithDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.kimi.com/coding/v1"
	}
	if c.Model == "" {
		// Deprecated: retained for backward compatibility only.
		// Actual usage should follow docs/llm-integration-strategy-framework.md §1a.
		c.Model = "moonshot-v1-8k"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 512
	}
	return c
}

// Validate returns an error if the config cannot drive a real client.
func (c Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("%w: APIKey is required", ErrUnavailable)
	}
	return nil
}
