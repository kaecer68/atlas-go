package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestRegimeExplanationHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"headline":"市場進入風險偏好模式"}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRegimeExplanationHandler(router)

	input := schemas.RegimeExplanationInput{
		DataClass: llm.DataClassNonRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityRegimeExplanation {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityRegimeExplanation)
	}

	if router.lastReq.DataClass != llm.DataClassNonRegulated {
		t.Errorf("lastReq.DataClass = %v, want %v",
			router.lastReq.DataClass, llm.DataClassNonRegulated)
	}

	payloadBytes, ok := router.lastReq.Payload.([]byte)
	if !ok {
		t.Fatalf("lastReq.Payload type = %T, want []byte", router.lastReq.Payload)
	}
	if len(payloadBytes) == 0 {
		t.Error("lastReq.Payload is empty")
	}
}

func TestRegimeExplanationHandler_ParsesResponse(t *testing.T) {
	expected := schemas.RegimeExplanationResponse{
		Headline: "美國升息預期升溫，資金流向美元避險",
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRegimeExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RegimeExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Headline != expected.Headline {
		t.Errorf("Headline = %q, want %q", resp.Headline, expected.Headline)
	}
}

func TestRegimeExplanationHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "市場狀態純文字",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRegimeExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RegimeExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Headline != "市場狀態純文字" {
		t.Errorf("Headline = %q, want %q", resp.Headline, "市場狀態純文字")
	}
}

func TestRegimeExplanationHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewRegimeExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RegimeExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Headline != "" {
		t.Errorf("Headline = %q, want empty on fallback", resp.Headline)
	}
}

func TestRegimeExplanationHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("rate limit exceeded")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewRegimeExplanationHandler(router)

	_, err := handler.Handle(context.Background(), schemas.RegimeExplanationInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
