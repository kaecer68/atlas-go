package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// TestConfidenceCommentaryHandler_Success verifies that when the mock
// router returns valid JSON, the handler parses the Commentary field.
func TestConfidenceCommentaryHandler_Success(t *testing.T) {
	expected := schemas.ConfidenceCommentaryResponse{
		Commentary: "決策信心水準偏高，因近期波動率低於歷史平均，且模型校準誤差在可接受範圍內。",
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewConfidenceCommentaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Commentary != expected.Commentary {
		t.Errorf("Commentary = %q, want %q", resp.Commentary, expected.Commentary)
	}
}

// TestConfidenceCommentaryHandler_AllProvidersFailed verifies that when
// the router returns ErrAllProvidersFailed, the handler returns an empty
// response without error (graceful degradation).
func TestConfidenceCommentaryHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewConfidenceCommentaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Commentary != "" {
		t.Errorf("Commentary = %q, want empty on AllProvidersFailed", resp.Commentary)
	}
}

// TestConfidenceCommentaryHandler_RawStringFallback verifies that when
// the router returns non-JSON output, the raw string becomes Commentary.
func TestConfidenceCommentaryHandler_RawStringFallback(t *testing.T) {
	rawCommentary := "信心中等，因市場震盪期間資料不足。"
	router := &mockRouter{
		callResp: llm.Response{
			Output:   rawCommentary,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewConfidenceCommentaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Commentary != rawCommentary {
		t.Errorf("Commentary = %q, want %q", resp.Commentary, rawCommentary)
	}
}

// TestConfidenceCommentaryHandler_EmptyResponse verifies that an empty
// output from the router produces an empty commentary.
func TestConfidenceCommentaryHandler_EmptyResponse(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewConfidenceCommentaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Commentary != "" {
		t.Errorf("Commentary = %q, want empty", resp.Commentary)
	}
}

// TestConfidenceCommentaryHandler_CorrectCapability verifies that
// the handler dispatches with CapabilityConfidenceCommentary.
func TestConfidenceCommentaryHandler_CorrectCapability(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"commentary":"test"}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewConfidenceCommentaryHandler(router)

	_, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityConfidenceCommentary {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityConfidenceCommentary)
	}
}

// TestConfidenceCommentaryHandler_ErrorPropagation verifies that
// non-ErrAllProvidersFailed errors are propagated to the caller.
func TestConfidenceCommentaryHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("circuit breaker open")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewConfidenceCommentaryHandler(router)

	_, err := handler.Handle(context.Background(), schemas.ConfidenceCommentaryInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
