// Package capabilities provides capability-specific handler implementations
// for the llm.Router. Each handler translates a caller-facing typed payload into
// an llm.Request, dispatches it through the Router, and parses the Response
// into a typed result.
//
// Handlers are the public API of the capabilities package. Caller code
// constructs a handler with a Router, then calls Handle with a typed payload.
// The handler encapsulates the capability-to-provider routing, response parsing,
// and fallback logic.
package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// FailureAttributionPayload is the input to the Failure Attribution capability
// handler. It wraps the existing llm_annotator.FailureContext with a DataClass
// for routing decisions.
type FailureAttributionPayload struct {
	FailureContext llm_annotator.FailureContext
	DataClass      llm.DataClass
}

// FailureAttributionResponse is the output of the Failure Attribution
// capability. Annotation is the natural-language explanation of why the
// strategy did not hit. Confidence is a 0.0-1.0 score indicating how likely
// the annotation is to be correct.
type FailureAttributionResponse struct {
	Annotation string  `json:"annotation"`
	Confidence float64 `json:"confidence"`
}

// FailureAttributionHandler translates a caller-facing
// FailureAttributionPayload into an llm.Request, dispatches it through the
// Router, and parses the Response into a FailureAttributionResponse. When all
// providers in the routing chain fail, it returns a deterministic rule-based
// fallback response.
type FailureAttributionHandler struct {
	router llm.Router
}

// NewFailureAttributionHandler creates a handler backed by the given Router.
func NewFailureAttributionHandler(router llm.Router) *FailureAttributionHandler {
	return &FailureAttributionHandler{router: router}
}

// Handle executes the failure attribution capability. It:
//  1. Constructs an llm.Request with CapabilityFailureAttribution and the
//     payload's FailureContext.
//  2. Defaults DataClass to DataClassNonRegulated when unset.
//  3. Dispatches through the Router.
//  4. Parses Response.Output into FailureAttributionResponse (first attempting
//     JSON unmarshal, then falling back to raw string).
//  5. On all-providers-failed, returns a rule-based fallback
//     ("rule_based: unable to attribute", Confidence=0).
func (h *FailureAttributionHandler) Handle(
	ctx context.Context,
	payload FailureAttributionPayload,
) (FailureAttributionResponse, error) {
	dc := payload.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    payload.FailureContext,
		DataClass:  dc,
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return FailureAttributionResponse{
				Annotation: "rule_based: unable to attribute",
				Confidence: 0,
			}, nil
		}
		return FailureAttributionResponse{}, err
	}

	return parseAttributionResponse(resp.Output)
}

// parseAttributionResponse converts the Router's Response.Output string into a
// FailureAttributionResponse. It first attempts JSON unmarshal into a
// FailureAttributionResponse. If that succeeds, the parsed value is returned.
// Otherwise the raw output is used as the Annotation with Confidence=0.
func parseAttributionResponse(output string) (FailureAttributionResponse, error) {
	if output == "" {
		return FailureAttributionResponse{
			Annotation: "",
			Confidence: 0,
		}, nil
	}

	var parsed FailureAttributionResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return FailureAttributionResponse{
		Annotation: output,
		Confidence: 0,
	}, nil
}
