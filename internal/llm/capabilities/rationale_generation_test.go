package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// TestRationaleGenerationHandler_BuildsCorrectRequest verifies that the handler
// constructs the correct llm.Request with CapabilityRationaleGeneration,
// the expected DataClass, and a non-empty JSON payload.
func TestRationaleGenerationHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"translated_text":"測試翻譯"}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRationaleGenerationHandler(router)

	input := schemas.RationaleGenerationInput{
		EnglishText: "Buy signal triggered by volume breakout",
		DataClass:   llm.DataClassNonRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityRationaleGeneration {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityRationaleGeneration)
	}

	if router.lastReq.DataClass != llm.DataClassNonRegulated {
		t.Errorf("lastReq.DataClass = %v, want %v",
			router.lastReq.DataClass, llm.DataClassNonRegulated)
	}

	// Payload should be non-nil JSON bytes
	payloadBytes, ok := router.lastReq.Payload.([]byte)
	if !ok {
		t.Fatalf("lastReq.Payload type = %T, want []byte", router.lastReq.Payload)
	}
	if len(payloadBytes) == 0 {
		t.Error("lastReq.Payload is empty")
	}
}

// TestRationaleGenerationHandler_ParsesResponse verifies that a JSON response
// from the Router is correctly unmarshaled into RationaleGenerationResponse.
func TestRationaleGenerationHandler_ParsesResponse(t *testing.T) {
	expected := schemas.RationaleGenerationResponse{
		TranslatedText: "成交量突破觸發買入訊號",
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRationaleGenerationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RationaleGenerationInput{
		EnglishText: "Buy signal triggered by volume breakout",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.TranslatedText != expected.TranslatedText {
		t.Errorf("TranslatedText = %q, want %q", resp.TranslatedText, expected.TranslatedText)
	}
}

// TestRationaleGenerationHandler_StringFallback verifies that a raw string
// response is used as TranslatedText when JSON parsing fails.
func TestRationaleGenerationHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "純文字翻譯結果",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewRationaleGenerationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RationaleGenerationInput{
		EnglishText: "test",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.TranslatedText != "純文字翻譯結果" {
		t.Errorf("TranslatedText = %q, want %q", resp.TranslatedText, "純文字翻譯結果")
	}
}

// TestRationaleGenerationHandler_AllProvidersFailed verifies fallback on
// ErrAllProvidersFailed returns an empty response without error.
func TestRationaleGenerationHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{
		callErr: llm.ErrAllProvidersFailed,
	}
	handler := NewRationaleGenerationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RationaleGenerationInput{
		EnglishText: "test",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.TranslatedText != "" {
		t.Errorf("TranslatedText = %q, want empty on fallback", resp.TranslatedText)
	}
}

// TestRationaleGenerationHandler_ErrorPropagation verifies that non-fallback
// errors are propagated to the caller.
func TestRationaleGenerationHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("upstream connection refused")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewRationaleGenerationHandler(router)

	_, err := handler.Handle(context.Background(), schemas.RationaleGenerationInput{
		EnglishText: "test",
	})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
