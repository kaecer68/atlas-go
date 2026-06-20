package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestRiskSurfaceExtractionHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"enriched_description":"風險表面分析","coverage":0.15}`,
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewRiskSurfaceExtractionHandler(router)

	input := schemas.RiskSurfaceExtractionInput{
		DataClass: llm.DataClassRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityRiskSurfaceExtraction {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityRiskSurfaceExtraction)
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

func TestRiskSurfaceExtractionHandler_ParsesResponse(t *testing.T) {
	expected := schemas.RiskSurfaceExtractionResponse{
		EnrichedDescription: "能源板塊覆蓋率僅 15%，存在重大風險盲點",
		Coverage:            0.15,
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewRiskSurfaceExtractionHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RiskSurfaceExtractionInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.EnrichedDescription != expected.EnrichedDescription {
		t.Errorf("EnrichedDescription = %q, want %q",
			resp.EnrichedDescription, expected.EnrichedDescription)
	}
	if resp.Coverage != expected.Coverage {
		t.Errorf("Coverage = %v, want %v", resp.Coverage, expected.Coverage)
	}
}

func TestRiskSurfaceExtractionHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "風險表面純文字",
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewRiskSurfaceExtractionHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RiskSurfaceExtractionInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.EnrichedDescription != "風險表面純文字" {
		t.Errorf("EnrichedDescription = %q, want %q",
			resp.EnrichedDescription, "風險表面純文字")
	}
}

func TestRiskSurfaceExtractionHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewRiskSurfaceExtractionHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.RiskSurfaceExtractionInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.EnrichedDescription != "" {
		t.Errorf("EnrichedDescription = %q, want empty on fallback", resp.EnrichedDescription)
	}
}

func TestRiskSurfaceExtractionHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("provider error")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewRiskSurfaceExtractionHandler(router)

	_, err := handler.Handle(context.Background(), schemas.RiskSurfaceExtractionInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
