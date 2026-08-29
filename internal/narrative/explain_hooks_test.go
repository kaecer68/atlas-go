package narrative

import (
	"context"
	"fmt"
	"testing"
)

func TestAnnotateEvent_BothHooksNil(t *testing.T) {
	RegimeExplainer = nil
	SentimentExplainer = nil

	event := &NarrativeEvent{
		ID:    "evt-001",
		Theme: "US_rates_up",
	}
	AnnotateEvent(context.Background(), event)

	if event.Explanation != "" {
		t.Errorf("expected empty Explanation with nil hooks, got %q", event.Explanation)
	}
	if event.SentimentExplanation != "" {
		t.Errorf("expected empty SentimentExplanation with nil hooks, got %q", event.SentimentExplanation)
	}
}

func TestAnnotateEvent_BothHooksCalled(t *testing.T) {
	regimeCalled := false
	sentimentCalled := false

	RegimeExplainer = func(ctx context.Context, event any) (string, error) {
		regimeCalled = true
		return "Regime: Risk-On", nil
	}
	SentimentExplainer = func(ctx context.Context, event any) (string, error) {
		sentimentCalled = true
		return "Sentiment: Bullish", nil
	}
	defer func() {
		RegimeExplainer = nil
		SentimentExplainer = nil
	}()

	event := &NarrativeEvent{
		ID:    "evt-002",
		Theme: "AI_surge",
	}
	AnnotateEvent(context.Background(), event)

	if !regimeCalled {
		t.Error("RegimeExplainer was not called")
	}
	if !sentimentCalled {
		t.Error("SentimentExplainer was not called")
	}
	if event.Explanation != "Regime: Risk-On" {
		t.Errorf("expected Explanation %q, got %q", "Regime: Risk-On", event.Explanation)
	}
	if event.SentimentExplanation != "Sentiment: Bullish" {
		t.Errorf("expected SentimentExplanation %q, got %q", "Sentiment: Bullish", event.SentimentExplanation)
	}
}

func TestAnnotateEvent_ErrorFallsThrough(t *testing.T) {
	RegimeExplainer = func(ctx context.Context, event any) (string, error) {
		return "", fmt.Errorf("LLM timeout")
	}
	defer func() { RegimeExplainer = nil }()

	event := &NarrativeEvent{
		ID:    "evt-003",
		Theme: "crash",
	}
	AnnotateEvent(context.Background(), event)

	if event.Explanation != "" {
		t.Errorf("expected empty Explanation on error, got %q", event.Explanation)
	}
}
