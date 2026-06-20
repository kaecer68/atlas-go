// Package adapters provides ProviderImpl wrappers that bridge existing
// domain-specific LLM clients (e.g., llm_annotator) into the unified
// llm.ProviderImpl interface consumed by the Router.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// ErrAnnotatorDisabled is returned by AnnotatorAdapter.Call when the adapter
// was constructed with a nil annotator.
var ErrAnnotatorDisabled = errors.New("annotator adapter is disabled")

// AnnotatorAdapter wraps an llm_annotator.Annotator and exposes it as an
// llm.ProviderImpl. It bridges the existing narrow annotator interface into
// the capability-based routing system.
//
// The adapter enforces ADR-009: the kimi-k2.7 model is a code-specialized
// model and is only allowed for CapabilityCodeReviewAnnotation and
// CapabilityPromptLint. For all other capabilities, including
// CapabilityFailureAttribution, the kimi-k2.7 model is rejected.
type AnnotatorAdapter struct {
	annotator llm_annotator.Annotator
	model     string
	lastCall  time.Time
}

// NewAnnotatorAdapter creates an AnnotatorAdapter that wraps the given
// annotator. If annotator is nil, the adapter is disabled: Supports always
// returns false and Call returns ErrAnnotatorDisabled.
func NewAnnotatorAdapter(annotator llm_annotator.Annotator, model string) *AnnotatorAdapter {
	return &AnnotatorAdapter{
		annotator: annotator,
		model:     model,
	}
}

// Supports implements llm.ProviderImpl. For a non-disabled adapter:
//
//   - If model is "kimi-k2.7" (ADR-009): only CapabilityCodeReviewAnnotation
//     and CapabilityPromptLint are supported.
//   - Otherwise: only CapabilityFailureAttribution is supported.
//
// A disabled adapter (nil annotator) returns false for all capabilities.
func (a *AnnotatorAdapter) Supports(cap llm.Capability) bool {
	if a.annotator == nil {
		return false
	}
	if a.model == "kimi-k2.7" {
		switch cap {
		case llm.CapabilityCodeReviewAnnotation, llm.CapabilityPromptLint:
			return true
		default:
			return false
		}
	}
	return cap == llm.CapabilityFailureAttribution
}

// Call implements llm.ProviderImpl. It asserts that req.Payload is an
// llm_annotator.FailureContext, delegates to the wrapped Annotator, and
// returns a Response with ProviderKimi on success. If the adapter is
// disabled, it returns ErrAnnotatorDisabled.
func (a *AnnotatorAdapter) Call(ctx context.Context, req llm.Request) (llm.Response, error) {
	if a.annotator == nil {
		return llm.Response{}, ErrAnnotatorDisabled
	}
	fc, ok := req.Payload.(llm_annotator.FailureContext)
	if !ok {
		return llm.Response{}, fmt.Errorf(
			"annotator adapter: expected llm_annotator.FailureContext, got %T", req.Payload)
	}

	start := time.Now()
	annotation, err := a.annotator.Annotate(ctx, fc)
	if err != nil {
		return llm.Response{}, err
	}

	latency := time.Since(start)
	a.lastCall = time.Now()

	return llm.Response{
		Output:   annotation,
		Provider: llm.ProviderKimi,
		Latency:  latency,
	}, nil
}

// Health implements llm.ProviderImpl. A non-nil annotator reports healthy
// with ProviderKimi. A disabled adapter (nil annotator) reports unhealthy
// with LastError set to "disabled".
func (a *AnnotatorAdapter) Health() llm.HealthStatus {
	if a.annotator == nil {
		return llm.HealthStatus{
			Provider:  llm.ProviderKimi,
			Healthy:   false,
			LastError: "disabled",
		}
	}
	return llm.HealthStatus{
		Provider:    llm.ProviderKimi,
		Healthy:     true,
		LastSuccess: a.lastCall,
	}
}
