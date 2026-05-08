package orchestrator

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReasoningTrace_JSONRoundTrip(t *testing.T) {
	original := ReasoningTrace{
		SessionID:  "session-20260413-daily",
		Timestamp:  time.Date(2026, 4, 13, 9, 30, 0, 0, time.UTC),
		Phase:      PhaseRegimeDetection,
		Step:       1,
		Component:  "janus",
		Action:     "detect_regime",
		Reasoning:  "Market showing bullish momentum indicators",
		Data:       map[string]any{"regime": "bullish", "confidence": 0.78},
		Confidence: 0.78,
		IsFallback: false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal ReasoningTrace: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	expectedFields := []string{"session_id", "timestamp", "phase", "step", "component",
		"action", "reasoning", "data", "confidence", "is_fallback"}
	for _, field := range expectedFields {
		if _, ok := m[field]; !ok {
			t.Errorf("expected snake_case field %q in JSON output", field)
		}
	}

	pascalFields := []string{"SessionID", "Timestamp", "Phase", "Step",
		"Component", "Action", "Reasoning", "Data", "Confidence", "IsFallback"}
	for _, field := range pascalFields {
		if _, ok := m[field]; ok {
			t.Errorf("found PascalCase field %q in JSON output, expected snake_case", field)
		}
	}

	var restored ReasoningTrace
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal ReasoningTrace: %v", err)
	}

	if restored.SessionID != original.SessionID {
		t.Errorf("SessionID: got %q, want %q", restored.SessionID, original.SessionID)
	}
	if !restored.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", restored.Timestamp, original.Timestamp)
	}
	if restored.Phase != original.Phase {
		t.Errorf("Phase: got %q, want %q", restored.Phase, original.Phase)
	}
	if restored.Step != original.Step {
		t.Errorf("Step: got %d, want %d", restored.Step, original.Step)
	}
	if restored.Component != original.Component {
		t.Errorf("Component: got %q, want %q", restored.Component, original.Component)
	}
	if restored.Action != original.Action {
		t.Errorf("Action: got %q, want %q", restored.Action, original.Action)
	}
	if restored.Reasoning != original.Reasoning {
		t.Errorf("Reasoning: got %q, want %q", restored.Reasoning, original.Reasoning)
	}
	if restored.Confidence != original.Confidence {
		t.Errorf("Confidence: got %f, want %f", restored.Confidence, original.Confidence)
	}
	if restored.IsFallback != original.IsFallback {
		t.Errorf("IsFallback: got %v, want %v", restored.IsFallback, original.IsFallback)
	}
}

func TestPhaseConstants(t *testing.T) {
	if PhaseRegimeDetection != "regime_detection" {
		t.Errorf("PhaseRegimeDetection: got %q, want %q", PhaseRegimeDetection, "regime_detection")
	}
	if PhaseAgentRecommendation != "agent_recommendation" {
		t.Errorf("PhaseAgentRecommendation: got %q, want %q", PhaseAgentRecommendation, "agent_recommendation")
	}
	if PhaseControlFilter != "control_filter" {
		t.Errorf("PhaseControlFilter: got %q, want %q", PhaseControlFilter, "control_filter")
	}
	if PhasePortfolioBuild != "portfolio_build" {
		t.Errorf("PhasePortfolioBuild: got %q, want %q", PhasePortfolioBuild, "portfolio_build")
	}
}
