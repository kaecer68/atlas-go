// Package capabilities provides capability-specific handler implementations
// for the llm.Router. Each handler translates a caller-facing typed payload into
// an llm.Request, dispatches it through the Router, and parses the Response
// into a typed result.
package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// mockRouter implements llm.Router for testing capability handlers.
// It captures the last request and returns configurable responses/errors.
type mockRouter struct {
	callResp llm.Response
	callErr  error
	lastReq  llm.Request // captured for assertion
}

func (m *mockRouter) Call(_ context.Context, req llm.Request) (llm.Response, error) {
	m.lastReq = req
	if m.callErr != nil {
		return llm.Response{}, m.callErr
	}
	return m.callResp, nil
}

func (m *mockRouter) Health() map[llm.Provider]llm.HealthStatus {
	return nil
}

func (m *mockRouter) Register(_ llm.ProviderImpl) error {
	return nil
}

// ---------------------------------------------------------------------------
// TestFailureAttributionHandler_Success_StringOutput verifies that when the
// Router returns a plain string response, the handler uses it as Annotation
// with Confidence=0.
func TestFailureAttributionHandler_Success_StringOutput(t *testing.T) {
	// Given: a router that returns a plain string as Output
	expectedAnnotation := "外資賣超 + 台幣貶值導致策略失效"
	router := &mockRouter{
		callResp: llm.Response{
			Output:   expectedAnnotation,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID:   "frame-1",
			FrameName: "趨勢跟隨",
			Layer:     "signal",
			Label:     "test-label",
		},
		DataClass: llm.DataClassNonRegulated,
	}

	// When: Handle is invoked
	resp, err := handler.Handle(context.Background(), payload)
	// Then: no error, Annotation matches the raw string, Confidence is 0
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Annotation != expectedAnnotation {
		t.Errorf("Handle().Annotation = %q, want %q", resp.Annotation, expectedAnnotation)
	}
	if resp.Confidence != 0 {
		t.Errorf("Handle().Confidence = %v, want 0 for non-JSON string output", resp.Confidence)
	}
}

// TestFailureAttributionHandler_Success_StructuredOutput verifies that when the
// Router returns a JSON-encoded FailureAttributionResponse as Output, the
// handler unmarshals it correctly.
func TestFailureAttributionHandler_Success_StructuredOutput(t *testing.T) {
	// Given: a router that returns a JSON-encoded FailureAttributionResponse
	expected := FailureAttributionResponse{
		Annotation: "外資賣超，台幣貶值，且美股前一交易日收跌",
		Confidence: 0.87,
	}
	rawJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID: "frame-2",
		},
	}

	// When: Handle is invoked
	resp, err := handler.Handle(context.Background(), payload)
	// Then: no error, response is parsed from JSON
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Annotation != expected.Annotation {
		t.Errorf("Handle().Annotation = %q, want %q", resp.Annotation, expected.Annotation)
	}
	if resp.Confidence != expected.Confidence {
		t.Errorf("Handle().Confidence = %v, want %v", resp.Confidence, expected.Confidence)
	}
}

// TestFailureAttributionHandler_AllProvidersFailed_Fallback verifies that when
// the Router returns ErrAllProvidersFailed, the handler returns the rule-based
// fallback response with Annotation="rule_based: unable to attribute" and
// Confidence=0.
func TestFailureAttributionHandler_AllProvidersFailed_Fallback(t *testing.T) {
	// Given: a router that returns ErrAllProvidersFailed
	router := &mockRouter{
		callErr: llm.ErrAllProvidersFailed,
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID: "frame-fallback",
		},
	}

	// When: Handle is invoked
	resp, err := handler.Handle(context.Background(), payload)
	// Then: no error (fallback is not an error), response is rule-based fallback
	if err != nil {
		t.Fatalf("Handle() unexpected error on fallback: %v", err)
	}
	if resp.Annotation != "rule_based: unable to attribute" {
		t.Errorf("Handle().Annotation = %q, want %q", resp.Annotation, "rule_based: unable to attribute")
	}
	if resp.Confidence != 0 {
		t.Errorf("Handle().Confidence = %v, want 0", resp.Confidence)
	}
}

// TestFailureAttributionHandler_NonFallbackError verifies that a non-fallback
// error from the Router is propagated to the caller.
func TestFailureAttributionHandler_NonFallbackError(t *testing.T) {
	// Given: a router that returns a non-fallback error
	upstreamErr := errors.New("upstream connection refused")
	router := &mockRouter{
		callErr: upstreamErr,
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID: "frame-err",
		},
	}

	// When: Handle is invoked
	_, err := handler.Handle(context.Background(), payload)

	// Then: the error is propagated
	if err == nil {
		t.Fatal("Handle() expected error, got nil")
	}
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want upstreamErr", err)
	}
}

// TestFailureAttributionHandler_RoutingChain verifies that the handler
// constructs the correct llm.Request with CapabilityFailureAttribution
// and the correct DataClass.
func TestFailureAttributionHandler_RoutingChain(t *testing.T) {
	// Given: a router that captures the request
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "attribution result",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID: "frame-routing",
		},
		DataClass: llm.DataClassRegulated,
	}

	// When: Handle is invoked
	_, err := handler.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	// Then: the router received a request with the correct Capability
	if router.lastReq.Capability != llm.CapabilityFailureAttribution {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityFailureAttribution)
	}

	// Then: the DataClass is passed through
	if router.lastReq.DataClass != llm.DataClassRegulated {
		t.Errorf("lastReq.DataClass = %v, want %v",
			router.lastReq.DataClass, llm.DataClassRegulated)
	}

	// Then: the Payload is the FailureContext (not the wrapper)
	_, ok := router.lastReq.Payload.(llm_annotator.FailureContext)
	if !ok {
		t.Errorf("lastReq.Payload type = %T, want llm_annotator.FailureContext", router.lastReq.Payload)
	}
}

// TestFailureAttributionHandler_DefaultDataClass verifies that when DataClass
// is zero (unset), the handler defaults to DataClassNonRegulated.
func TestFailureAttributionHandler_DefaultDataClass(t *testing.T) {
	// Given: a router that captures the request, payload with zero DataClass
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "attribution result",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewFailureAttributionHandler(router)

	payload := FailureAttributionPayload{
		FailureContext: llm_annotator.FailureContext{
			FrameID: "frame-default-dc",
		},
		// DataClass is zero-value (0 = unset)
	}

	// When: Handle is invoked
	_, err := handler.Handle(context.Background(), payload)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	// Then: the router received DataClassNonRegulated
	if router.lastReq.DataClass != llm.DataClassNonRegulated {
		t.Errorf("lastReq.DataClass = %v, want %v (default)",
			router.lastReq.DataClass, llm.DataClassNonRegulated)
	}
}
