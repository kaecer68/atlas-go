package adapters

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// RouterAnnotator wraps an llm.Router to implement the llm_annotator.Annotator
// interface. It routes failure-attribution requests through the Router's
// capability-based provider chain (MiniMax → DeepSeek → OpenCode-Go with
// Mock last-resort), delegating provider selection, fallback, and retry logic
// to the Router.
//
// A nil router is a valid state; Annotate returns ("", nil) in that case,
// making it safe to construct a RouterAnnotator before the Router is ready.
type RouterAnnotator struct {
	router llm.Router
}

// NewRouterAnnotator creates a RouterAnnotator that delegates to the given
// Router. If router is nil, Annotate is a safe no-op returning ("", nil).
func NewRouterAnnotator(r llm.Router) *RouterAnnotator {
	return &RouterAnnotator{router: r}
}

// Name implements llm_annotator.Annotator.
//
// Returns a human-readable chain descriptor for logging/monitoring.
// Effective chain is determined by the underlying Router's routing table
// (see Wave 11 L2.1 doc audit, Issue #720: OpenCode providers are
// reserved [PLANNED] constants without client implementations).
func (r *RouterAnnotator) Name() string {
	return "router(minimax→deepseek→mock)"
}

// Annotate implements llm_annotator.Annotator. It wraps the FailureContext into
// an llm.Request with CapabilityFailureAttribution and DataClassNonRegulated,
// dispatches through the Router, and returns the response Output.
//
// If the router is nil, it returns ("", nil) — a safe no-op.
func (r *RouterAnnotator) Annotate(ctx context.Context, fc llm_annotator.FailureContext) (string, error) {
	if r.router == nil {
		return "", nil
	}
	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    fc,
		DataClass:  llm.DataClassNonRegulated,
	}
	resp, err := r.router.Call(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Output, nil
}
