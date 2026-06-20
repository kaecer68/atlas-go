package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

func TestScenarioSimulationHandler_BuildsCorrectRequest(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   `{"insight":"模擬結果分析","cohort_summary":"200 個訊號"}`,
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewScenarioSimulationHandler(router)

	input := schemas.ScenarioSimulationInput{
		DataClass: llm.DataClassRegulated,
	}

	_, err := handler.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if router.lastReq.Capability != llm.CapabilityScenarioSimulation {
		t.Errorf("lastReq.Capability = %q, want %q",
			router.lastReq.Capability, llm.CapabilityScenarioSimulation)
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

func TestScenarioSimulationHandler_ParsesResponse(t *testing.T) {
	expected := schemas.ScenarioSimulationResponse{
		Insight:       "在牛市環境下策略表現優異，夏普比率達 1.8",
		CohortSummary: "200 個訊號中 144 個獲利，勝率 72%",
	}
	rawJSON, _ := json.Marshal(expected)

	router := &mockRouter{
		callResp: llm.Response{
			Output:   string(rawJSON),
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewScenarioSimulationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ScenarioSimulationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Insight != expected.Insight {
		t.Errorf("Insight = %q, want %q", resp.Insight, expected.Insight)
	}
	if resp.CohortSummary != expected.CohortSummary {
		t.Errorf("CohortSummary = %q, want %q", resp.CohortSummary, expected.CohortSummary)
	}
}

func TestScenarioSimulationHandler_StringFallback(t *testing.T) {
	router := &mockRouter{
		callResp: llm.Response{
			Output:   "模擬結果純文字說明",
			Provider: llm.ProviderMiniMax,
		},
	}
	handler := NewScenarioSimulationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ScenarioSimulationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if resp.Insight != "模擬結果純文字說明" {
		t.Errorf("Insight = %q, want %q", resp.Insight, "模擬結果純文字說明")
	}
}

func TestScenarioSimulationHandler_AllProvidersFailed(t *testing.T) {
	router := &mockRouter{callErr: llm.ErrAllProvidersFailed}
	handler := NewScenarioSimulationHandler(router)

	resp, err := handler.Handle(context.Background(), schemas.ScenarioSimulationInput{})
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if resp.Insight != "" {
		t.Errorf("Insight = %q, want empty on fallback", resp.Insight)
	}
}

func TestScenarioSimulationHandler_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("provider timeout")
	router := &mockRouter{callErr: upstreamErr}
	handler := NewScenarioSimulationHandler(router)

	_, err := handler.Handle(context.Background(), schemas.ScenarioSimulationInput{})
	if !errors.Is(err, upstreamErr) {
		t.Errorf("Handle() error = %v, want %v", err, upstreamErr)
	}
}
