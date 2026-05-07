package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecommendationOutcomeUnmarshalCanonicalSnakeCase(t *testing.T) {
	canonical := []byte(`{
		"agent_id": "agent-2",
		"skill": "value_yield",
		"layer": "style",
		"symbol": "2317.TW",
		"side": "BUY",
		"conviction": 72,
		"target_price": 155.5,
		"stop_loss_price": 146.0,
		"window": "5d",
		"forward_return": 0.015,
		"benchmark_delta": 0.004,
		"hit": true,
		"reason": "cheap",
		"price": 150.0,
		"passed_guards": true,
		"guard_reason": "ok",
		"recorded_at": "2026-04-22T04:02:30.434394+08:00",
		"factor_scores": {"total": 0.67}
	}`)

	var got RecommendationOutcome
	if err := json.Unmarshal(canonical, &got); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	if got.AgentID != "agent-2" {
		t.Fatalf("agent_id: got %q", got.AgentID)
	}
	if got.RecordedAt.IsZero() {
		t.Fatalf("recorded_at should be populated")
	}
	if got.FactorScores.Total != 0.67 {
		t.Fatalf("factor_scores.total: got %v", got.FactorScores.Total)
	}
}

func TestRecommendationOutcomeMarshalUsesCanonicalSnakeCase(t *testing.T) {
	outcome := RecommendationOutcome{
		AgentID:       "agent-3",
		Skill:         "semiconductor_desk",
		Layer:         LayerSector,
		Symbol:        "2330.TW",
		Side:          SideBuy,
		Conviction:    91,
		TargetPrice:   1100,
		StopLossPrice: 980,
		Window:        "1d",
		ForwardReturn: 0.031,
		Hit:           true,
		Reason:        "earnings",
		Price:         1020,
		PassedGuards:  true,
		GuardReason:   "ok",
		FactorScores:  FactorScores{Total: 0.93},
	}

	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal outcome: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"agent_id"`) {
		t.Fatalf("expected canonical snake_case agent_id; got %s", text)
	}
	if strings.Contains(text, `"AgentID"`) {
		t.Fatalf("expected no PascalCase AgentID; got %s", text)
	}
	if !strings.Contains(text, `"factor_scores"`) {
		t.Fatalf("expected factor_scores key; got %s", text)
	}
}

func TestExperimentRecordMarshalUsesSnakeCase(t *testing.T) {
	record := ExperimentRecord{
		ID:            "exp-1",
		ProposalID:    "proposal-1",
		TargetAgentID: "growth-momentum-01",
		MutationType:  "prompt_tightening",
		Status:        ExperimentAccepted,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"id"`) {
		t.Fatalf("expected snake_case id; got %s", text)
	}
	if strings.Contains(text, `"TargetAgentID"`) {
		t.Fatalf("unexpected PascalCase TargetAgentID; got %s", text)
	}
	if !strings.Contains(text, `"proposal_id"`) {
		t.Fatalf("expected snake_case proposal_id; got %s", text)
	}
	if !strings.Contains(text, `"mutation_type"`) {
		t.Fatalf("expected snake_case mutation_type; got %s", text)
	}
	if !strings.Contains(text, `"status"`) {
		t.Fatalf("expected snake_case status; got %s", text)
	}
}

func TestExperimentRecordUnmarshalCanonicalWithoutProposalID(t *testing.T) {
	canonical := []byte(`{
		"id": "exp-no-proposal",
		"target_agent_id": "agent-99",
		"skill": "test_skill",
		"mutation_type": "prompt_tightening",
		"status": "accepted"
	}`)

	var got ExperimentRecord
	if err := json.Unmarshal(canonical, &got); err != nil {
		t.Fatalf("unmarshal canonical without proposal_id: %v", err)
	}
	if got.ID != "exp-no-proposal" {
		t.Fatalf("id: got %q", got.ID)
	}
	if got.TargetAgentID != "agent-99" {
		t.Fatalf("target_agent_id: got %q", got.TargetAgentID)
	}
	if got.Skill != "test_skill" {
		t.Fatalf("skill: got %q", got.Skill)
	}
	if got.MutationType != "prompt_tightening" {
		t.Fatalf("mutation_type: got %q", got.MutationType)
	}
	if got.Status != ExperimentAccepted {
		t.Fatalf("status: got %q", got.Status)
	}
}

func TestPromptExperimentResultMarshalUsesSnakeCaseEnvelope(t *testing.T) {
	result := PromptExperimentResult{
		Experiment:      ExperimentRecord{ID: "exp-1", Status: ExperimentAccepted},
		Brief:           MutationBrief{WindowID: "window-1", TargetAgentID: "growth-momentum-01", TargetSkill: "growth_momentum", TargetLayer: LayerStyle, PromptFile: "prompts/agents/growth_momentum.md", MutationType: "prompt_tightening", FailurePattern: "weak_momentum", Hypothesis: "tighten exit", AcceptanceMetric: "sharpe_like", AcceptanceGates: []string{"improve_sharpe_like"}},
		CandidatePrompt: "v2 prompt",
		EvaluationMode:  "replay",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"experiment"`) || !strings.Contains(text, `"brief"`) {
		t.Fatalf("expected snake_case envelope; got %s", text)
	}
	if strings.Contains(text, `"Experiment"`) || strings.Contains(text, `"Brief"`) {
		t.Fatalf("unexpected PascalCase envelope; got %s", text)
	}
	if !strings.Contains(text, `"candidate_prompt"`) {
		t.Fatalf("expected snake_case candidate_prompt; got %s", text)
	}
	if !strings.Contains(text, `"evaluation_mode"`) {
		t.Fatalf("expected snake_case evaluation_mode; got %s", text)
	}
}
