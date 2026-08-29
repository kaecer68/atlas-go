package risk

import (
	"context"
	"fmt"
	"testing"
)

func TestAnnotateSnapshot_HookCalled(t *testing.T) {
	called := false
	PerformanceForensics = func(ctx context.Context, snapshot any) (string, error) {
		called = true
		return "VaR 95 is elevated; review exposure", nil
	}
	defer func() { PerformanceForensics = nil }()

	got := AnnotateSnapshot(context.Background(), map[string]float64{"VaR95": -0.05})
	if !called {
		t.Error("PerformanceForensics was not called")
	}
	if got != "VaR 95 is elevated; review exposure" {
		t.Errorf("expected commentary, got %q", got)
	}
}

func TestAnnotateSnapshot_NilHook_ReturnsEmpty(t *testing.T) {
	PerformanceForensics = nil
	got := AnnotateSnapshot(context.Background(), map[string]float64{"VaR95": -0.05})
	if got != "" {
		t.Errorf("expected empty string with nil hook, got %q", got)
	}
}

func TestAnnotateSnapshot_ErrorReturnsEmpty(t *testing.T) {
	PerformanceForensics = func(ctx context.Context, snapshot any) (string, error) {
		return "", fmt.Errorf("LLM unavailable")
	}
	defer func() { PerformanceForensics = nil }()

	got := AnnotateSnapshot(context.Background(), map[string]float64{"VaR95": -0.05})
	if got != "" {
		t.Errorf("expected empty string on error, got %q", got)
	}
}
