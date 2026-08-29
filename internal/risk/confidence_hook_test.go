package risk

import (
	"context"
	"fmt"
	"testing"
)

func TestEnrichDecision_HookCalled(t *testing.T) {
	called := false
	ConfidenceCommentary = func(ctx context.Context, decision any) (string, error) {
		called = true
		return "defensive mode VaR approaching limit", nil
	}
	defer func() { ConfidenceCommentary = nil }()

	dec := RiskDecision{Phase: PhasePreTrade, Verdict: VerdictReduce, Reason: "var limit"}
	got := EnrichDecision(context.Background(), dec)
	if !called {
		t.Error("ConfidenceCommentary was not called")
	}
	if got != "defensive mode VaR approaching limit" {
		t.Errorf("expected commentary, got %q", got)
	}
}

func TestEnrichDecision_NilHook_ReturnsEmpty(t *testing.T) {
	ConfidenceCommentary = nil
	dec := RiskDecision{Phase: PhasePreTrade, Verdict: VerdictReduce}
	got := EnrichDecision(context.Background(), dec)
	if got != "" {
		t.Errorf("expected empty string with nil hook, got %q", got)
	}
}

func TestEnrichDecision_ErrorReturnsEmpty(t *testing.T) {
	ConfidenceCommentary = func(ctx context.Context, decision any) (string, error) {
		return "", fmt.Errorf("LLM unavailable")
	}
	defer func() { ConfidenceCommentary = nil }()

	dec := RiskDecision{Phase: PhasePreTrade, Verdict: VerdictReduce}
	got := EnrichDecision(context.Background(), dec)
	if got != "" {
		t.Errorf("expected empty string on error, got %q", got)
	}
}
