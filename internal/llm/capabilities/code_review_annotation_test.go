package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestCodeReviewAnnotationHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"annotations":[{"file":"main.go","line":5,"severity":"warning","message":"缺少錯誤處理"}]}`,
			Provider: llm.ProviderKimi,
		},
	}
	handler := NewCodeReviewAnnotationHandler(router)

	input := schemas.CodeReviewAnnotationInput{
		DiffText:  "diff --git a/main.go b/main.go\n+func NewHandler() Handler {",
		PRURL:     "https://github.com/kaecer68/atlas-go/pull/610",
		DataClass: llm.DataClassNonRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityCodeReviewAnnotation {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityCodeReviewAnnotation)
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

func TestCodeReviewAnnotationHandler_ParsesResponse(t *testing.T) {
	expected := schemas.CodeReviewAnnotationResponse{
		Annotations: []schemas.CodeAnnotation{
			{File: "main.go", Line: 5, Severity: "warning", Message: "缺少錯誤處理"},
			{File: "router.go", Line: 12, Severity: "error", Message: "可能存在 race condition"},
		},
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderKimi,
		},
	}
	handler := NewCodeReviewAnnotationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.CodeReviewAnnotationInput{
		DiffText: "diff",
		PRURL:    "https://example.com/pr/1",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if len(resp.Annotations) != 2 {
		t.Fatalf("Annotations len = %d, want 2", len(resp.Annotations))
	}
	if resp.Annotations[0].File != "main.go" {
		t.Errorf("Annotations[0].File = %q, want %q", resp.Annotations[0].File, "main.go")
	}
	if resp.Annotations[1].Severity != "error" {
		t.Errorf("Annotations[1].Severity = %q, want %q", resp.Annotations[1].Severity, "error")
	}
}

func TestCodeReviewAnnotationHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "程式碼審查純文字",
			Provider: llm.ProviderKimi,
		},
	}
	handler := NewCodeReviewAnnotationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.CodeReviewAnnotationInput{
		DiffText: "diff",
		PRURL:    "https://example.com/pr/1",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if len(resp.Annotations) != 1 {
		t.Fatalf("Annotations len = %d, want 1", len(resp.Annotations))
	}
	if resp.Annotations[0].Message != "程式碼審查純文字" {
		t.Errorf("Annotations[0].Message = %q, want %q",
			resp.Annotations[0].Message, "程式碼審查純文字")
	}
}

func TestCodeReviewAnnotationHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewCodeReviewAnnotationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.CodeReviewAnnotationInput{
		DiffText: "diff",
		PRURL:    "https://example.com/pr/1",
	})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if len(resp.Annotations) != 0 {
		t.Errorf("Annotations len = %d, want 0 on fallback", len(resp.Annotations))
	}
}

func TestCodeReviewAnnotationHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("auth failed")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewCodeReviewAnnotationHandler(router)

	_, err := handler.Handle(context.Background(), schemas.CodeReviewAnnotationInput{
		DiffText: "diff",
		PRURL:    "https://example.com/pr/1",
	})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
