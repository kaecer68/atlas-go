package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestPromptLintHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"issues":[{"line":3,"severity":"warning","message":"缺少範例"}],"pass":false}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewPromptLintHandler(router)

	input := schemas.PromptLintInput{
		PromptContent: "You are a trading assistant.",
		PromptPath:    "prompts/trading/analysis.md",
		DataClass:     llm.DataClassNonRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityPromptLint {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityPromptLint)
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

func TestPromptLintHandler_ParsesResponse(t *testing.T) {
	expected := schemas.PromptLintResponse{
		Issues: []schemas.LintIssue{
			{Line: 3, Severity: "warning", Message: "模糊用語：'analyze the data' 未具體說明"},
			{Line: 7, Severity: "error", Message: "缺少輸出格式限制"},
		},
		Pass: false,
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewPromptLintHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.PromptLintInput{
		PromptContent: "test",
		PromptPath:    "test.md",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Pass != expected.Pass {
		t.Errorf("Pass = %v, want %v", resp.Pass, expected.Pass)
	}
	if len(resp.Issues) != 2 {
		t.Fatalf("Issues len = %d, want 2", len(resp.Issues))
	}
	if resp.Issues[0].Severity != "warning" {
		t.Errorf("Issues[0].Severity = %q, want %q", resp.Issues[0].Severity, "warning")
	}
}

func TestPromptLintHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewPromptLintHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.PromptLintInput{
		PromptContent: "test",
		PromptPath:    "test.md",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if !resp.Pass {
		t.Errorf("Pass = %v, want true on fallback", resp.Pass)
	}
	if len(resp.Issues) != 0 {
		t.Errorf("Issues len = %d, want 0 on fallback", len(resp.Issues))
	}
}

func TestPromptLintHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("network error")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewPromptLintHandler(router)

	_, err := handler.Handle(context.Background(), schemas.PromptLintInput{
		PromptContent: "test",
		PromptPath:    "test.md",
	})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
