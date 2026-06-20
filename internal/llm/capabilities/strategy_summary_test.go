package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestStrategySummaryHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"summary":"策略摘要","key_conditions":["條件一"]}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewStrategySummaryHandler(router)

	input := schemas.StrategySummaryInput{
		DataClass: llm.DataClassRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityStrategySummary {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityStrategySummary)
	}

	if router.lastReq.DataClass != llm.DataClassRegulated {
		t.Errorf("lastReq.DataClass = %v, want %v",
			router.lastReq.DataClass, llm.DataClassRegulated)
	}

	payloadBytes, ok := router.lastReq.Payload.([]byte)
	if !ok {
		t.Fatalf("lastReq.Payload type = %T, want []byte", router.lastReq.Payload)
	}
	if len(payloadBytes) == 0 {
		t.Error("lastReq.Payload is empty")
	}
}

func TestStrategySummaryHandler_ParsesResponse(t *testing.T) {
	expected := schemas.StrategySummaryResponse{
		Summary:       "動量突破策略，專注於科技股",
		KeyConditions: []string{"成交量 > 100k", "價格突破 20 日均線"},
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewStrategySummaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.StrategySummaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Summary != expected.Summary {
		t.Errorf("Summary = %q, want %q", resp.Summary, expected.Summary)
	}
	if len(resp.KeyConditions) != 2 {
		t.Errorf("KeyConditions len = %d, want 2", len(resp.KeyConditions))
	}
}

func TestStrategySummaryHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "策略摘要文字",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewStrategySummaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.StrategySummaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Summary != "策略摘要文字" {
		t.Errorf("Summary = %q, want %q", resp.Summary, "策略摘要文字")
	}
}

func TestStrategySummaryHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewStrategySummaryHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.StrategySummaryInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Summary != "" {
		t.Errorf("Summary = %q, want empty on fallback", resp.Summary)
	}
}

func TestStrategySummaryHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("upstream timeout")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewStrategySummaryHandler(router)

	_, err := handler.Handle(context.Background(), schemas.StrategySummaryInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
