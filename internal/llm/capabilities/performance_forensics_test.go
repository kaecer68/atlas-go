package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestPerformanceForensicsHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"commentary":"績效分析","calibration":"校準資訊"}`,
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewPerformanceForensicsHandler(router)

	input := schemas.PerformanceForensicsInput{
		DataClass: llm.DataClassRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityPerformanceForensics {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityPerformanceForensics)
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

func TestPerformanceForensicsHandler_ParsesResponse(t *testing.T) {
	expected := schemas.PerformanceForensicsResponse{
		Commentary:  "VaR95 達 -5%，尾端風險偏高，需注意極端事件",
		Calibration: "基於過去五年歷史數據校準，包含 2020 年 COVID 崩盤",
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewPerformanceForensicsHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.PerformanceForensicsInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Commentary != expected.Commentary {
		t.Errorf("Commentary = %q, want %q", resp.Commentary, expected.Commentary)
	}
	if resp.Calibration != expected.Calibration {
		t.Errorf("Calibration = %q, want %q", resp.Calibration, expected.Calibration)
	}
}

func TestPerformanceForensicsHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "績效異常純文字",
			Provider: llm.ProviderDeepSeek,
		},
	}
	handler := NewPerformanceForensicsHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.PerformanceForensicsInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Commentary != "績效異常純文字" {
		t.Errorf("Commentary = %q, want %q", resp.Commentary, "績效異常純文字")
	}
}

func TestPerformanceForensicsHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewPerformanceForensicsHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.PerformanceForensicsInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Commentary != "" {
		t.Errorf("Commentary = %q, want empty on fallback", resp.Commentary)
	}
}

func TestPerformanceForensicsHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("circuit breaker open")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewPerformanceForensicsHandler(router)

	_, err := handler.Handle(context.Background(), schemas.PerformanceForensicsInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
