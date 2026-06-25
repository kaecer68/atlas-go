package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// stubLLMDriver is a minimal LLMDriver for tests. The RunToolCall paths
// we cover here don't actually invoke PlanComplete/ReflectComplete, so
// these methods can return zero values.
type stubLLMDriver struct{}

func (stubLLMDriver) PlanComplete(_ context.Context, _, _ string) ([]PlanStep, error) {
	return nil, errors.New("stubLLMDriver.PlanComplete not implemented in tests")
}

func (stubLLMDriver) ReflectComplete(_ context.Context, _, _, _ string) (Reflection, error) {
	return Reflection{}, errors.New("stubLLMDriver.ReflectComplete not implemented in tests")
}

// TestRunToolCall_LLMNil_ReturnsErrNotImplemented verifies the backward-
// compat path: when no LLM is wired, RunToolCall returns ErrNotImplemented
// (not a panic, not a silent stub).
func TestRunToolCall_LLMNil_ReturnsErrNotImplemented(t *testing.T) {
	agent := &SectorAgentLLM{Skill: "semiconductor"}
	got, err := agent.RunToolCall(context.Background(), PlanStep{ToolName: "get_weather"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("got err=%v, want ErrNotImplemented", err)
	}
	if got != "" {
		t.Errorf("got result=%q, want empty string", got)
	}
}

// TestRunToolCall_LLMWiredNoTools_Panics verifies the Issue #711 #4
// trust contract: an LLM wired into the agent but no Tools registered
// would silently feed stub data back to the LLM. We refuse to operate
// in that state and panic with an actionable message.
func TestRunToolCall_LLMWiredNoTools_Panics(t *testing.T) {
	agent := &SectorAgentLLM{
		Skill: "semiconductor",
		LLM:   stubLLMDriver{},
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when LLM wired but Tools empty, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "semiconductor") {
			t.Errorf("panic message should mention skill, got: %s", msg)
		}
		if !strings.Contains(msg, "Tools") {
			t.Errorf("panic message should mention Tools, got: %s", msg)
		}
	}()
	_, _ = agent.RunToolCall(context.Background(), PlanStep{ToolName: "get_weather"})
}

// TestRunToolCall_LLMWiredWithTools_NotImpl verifies the wired-but-not-
// yet-dispatched path: when LLM and Tools are both set, the agent
// returns a clear "not yet implemented" error instead of silently
// returning stub data. PR5a will wire the actual tool dispatch.
func TestRunToolCall_LLMWiredWithTools_NotImpl(t *testing.T) {
	agent := &SectorAgentLLM{
		Skill: "semiconductor",
		LLM:   stubLLMDriver{},
		Tools: []llm.Tool{
			{Name: "get_weather", Description: "Look up weather"},
		},
	}
	got, err := agent.RunToolCall(context.Background(), PlanStep{ToolName: "get_weather"})
	if err == nil {
		t.Fatal("expected 'not yet implemented' error, got nil")
	}
	if !strings.Contains(err.Error(), "semiconductor") {
		t.Errorf("error should mention skill, got: %v", err)
	}
	if !strings.Contains(err.Error(), "get_weather") {
		t.Errorf("error should mention tool name, got: %v", err)
	}
	if got != "" {
		t.Errorf("got result=%q, want empty string on not-impl", got)
	}
}
