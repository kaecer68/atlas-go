package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestSentimentExplanationHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"explanation":"市場情緒分析","factors":["因素一"]}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewSentimentExplanationHandler(router)

	input := schemas.SentimentExplanationInput{
		DataClass: llm.DataClassNonRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilitySentimentExplanation {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilitySentimentExplanation)
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

func TestSentimentExplanationHandler_ParsesResponse(t *testing.T) {
	expected := schemas.SentimentExplanationResponse{
		Explanation: "財報優於預期，EPS 驚喜帶動看漲情緒",
		Factors:     []string{"EPS 超出預期 15%", "營收年增 8%", "公司上調未來展望"},
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewSentimentExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.SentimentExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Explanation != expected.Explanation {
		t.Errorf("Explanation = %q, want %q", resp.Explanation, expected.Explanation)
	}
	if len(resp.Factors) != 3 {
		t.Fatalf("Factors len = %d, want 3", len(resp.Factors))
	}
	if resp.Factors[0] != expected.Factors[0] {
		t.Errorf("Factors[0] = %q, want %q", resp.Factors[0], expected.Factors[0])
	}
}

func TestSentimentExplanationHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "情緒分析純文字",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewSentimentExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.SentimentExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Explanation != "情緒分析純文字" {
		t.Errorf("Explanation = %q, want %q", resp.Explanation, "情緒分析純文字")
	}
}

func TestSentimentExplanationHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewSentimentExplanationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.SentimentExplanationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Explanation != "" {
		t.Errorf("Explanation = %q, want empty on fallback", resp.Explanation)
	}
}

func TestSentimentExplanationHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("DNS resolution failure")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewSentimentExplanationHandler(router)

	_, err := handler.Handle(context.Background(), schemas.SentimentExplanationInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
