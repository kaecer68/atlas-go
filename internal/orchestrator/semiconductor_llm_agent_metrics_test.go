package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// captureMetricsLogger returns a JSON-handler-backed slog.Logger that
// writes to a buffer, plus the buffer itself, so tests can assert the
// metrics events emitted by SemiconductorLLMAgent.Recommend. The returned
// slog.Logger is suitable for assigning to SemiconductorLLMAgent.Metrics.
func captureMetricsLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// findEvent scans the JSON-lines log buffer for a slog event with the
// given msg field. Returns the decoded entry (or nil if not found).
// Comparison of optional kv pairs is string-based for portability
// (JSON numbers decode as float64; bools decode as bool; strings as
// strings).
func findEvent(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] == msg {
			return entry
		}
	}
	return nil
}

// TestSemiconductorLLMAgent_Metrics_HappyPath verifies that the
// full plan → tool → reflect loop emits the 5 Issue #740 spec events
// with the exact field names, and that agent_loop.exhausted is NOT
// emitted when Continue=false exits the loop early.
func TestSemiconductorLLMAgent_Metrics_HappyPath(t *testing.T) {
	logger, buf := captureMetricsLogger(t)
	mock := NewMockLLMDriver().
		WithPlanResponse([]PlanStep{
			{Kind: "tool", ToolName: "get_factor_weight", Args: map[string]any{"symbol": "2330"}},
		}).
		WithReflectResponse(Reflection{
			Continue:        false,
			FinalConviction: 75,
			Reasoning:       "Factors look strong; momentum confirmed.",
		})

	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		MaxIter:        3,
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if !ok {
		t.Fatalf("Recommend: got ok=false, rec.Reason=%q", rec.Reason)
	}
	if rec.Conviction != 75 {
		t.Errorf("rec.Conviction = %d, want 75", rec.Conviction)
	}

	if e := findEvent(t, buf, "agent_loop.start"); e == nil {
		t.Error("missing agent_loop.start event")
	} else {
		if e["symbol"] != "2330" {
			t.Errorf("agent_loop.start.symbol = %v, want %q", e["symbol"], "2330")
		}
		if e["skill"] != SemiconductorLLMAgentSkill {
			t.Errorf("agent_loop.start.skill = %v, want %q", e["skill"], SemiconductorLLMAgentSkill)
		}
		if _, exists := e["max_iter"]; exists {
			t.Error("agent_loop.start must not include max_iter")
		}
	}

	if e := findEvent(t, buf, "agent_loop.plan"); e == nil {
		t.Error("missing agent_loop.plan event")
	} else {
		if got := e["size"]; got != float64(1) {
			t.Errorf("agent_loop.plan.size = %v, want 1", got)
		}
		if _, exists := e["latency_ms"]; !exists {
			t.Error("agent_loop.plan missing latency_ms")
		}
		if _, exists := e["err"]; !exists {
			t.Error("agent_loop.plan missing err field")
		}
		if e["err"] != nil {
			t.Errorf("agent_loop.plan.err = %v, want nil", e["err"])
		}
		// Issue #740 fields only; PR #743 leftovers must be gone.
		if _, exists := e["plan_size"]; exists {
			t.Error("agent_loop.plan must not include plan_size")
		}
		if _, exists := e["round"]; exists {
			t.Error("agent_loop.plan must not include round")
		}
		if _, exists := e["skill"]; exists {
			t.Error("agent_loop.plan must not include skill")
		}
		if _, exists := e["symbol"]; exists {
			t.Error("agent_loop.plan must not include symbol")
		}
	}

	if e := findEvent(t, buf, "agent_loop.tool"); e == nil {
		t.Error("missing agent_loop.tool event")
	} else {
		if e["name"] != "get_factor_weight" {
			t.Errorf("agent_loop.tool.name = %v, want %q", e["name"], "get_factor_weight")
		}
		if e["success"] != true {
			t.Errorf("agent_loop.tool.success = %v, want true", e["success"])
		}
		if _, exists := e["latency_ms"]; !exists {
			t.Error("agent_loop.tool missing latency_ms")
		}
		if _, exists := e["tool_name"]; exists {
			t.Error("agent_loop.tool must not include tool_name")
		}
		if _, exists := e["skill"]; exists {
			t.Error("agent_loop.tool must not include skill")
		}
		if _, exists := e["symbol"]; exists {
			t.Error("agent_loop.tool must not include symbol")
		}
	}

	if e := findEvent(t, buf, "agent_loop.reflect"); e == nil {
		t.Error("missing agent_loop.reflect event")
	} else {
		if e["continue"] != false {
			t.Errorf("agent_loop.reflect.continue = %v, want false", e["continue"])
		}
		if got := e["conviction"]; got != float64(75) {
			t.Errorf("agent_loop.reflect.conviction = %v, want 75", got)
		}
		// Issue #740 reflect has no latency_ms.
		if _, exists := e["latency_ms"]; exists {
			t.Error("agent_loop.reflect must not include latency_ms")
		}
		if _, exists := e["skill"]; exists {
			t.Error("agent_loop.reflect must not include skill")
		}
		if _, exists := e["symbol"]; exists {
			t.Error("agent_loop.reflect must not include symbol")
		}
	}

	if e := findEvent(t, buf, "agent_loop.end"); e == nil {
		t.Error("missing agent_loop.end event")
	} else {
		if got := e["conviction"]; got != float64(75) {
			t.Errorf("agent_loop.end.conviction = %v, want 75", got)
		}
		if e["symbol"] != "2330" {
			t.Errorf("agent_loop.end.symbol = %v, want %q", e["symbol"], "2330")
		}
		if _, exists := e["skill"]; exists {
			t.Error("agent_loop.end must not include skill")
		}
	}

	// Issue #740 explicitly removes the exhausted event.
	if e := findEvent(t, buf, "agent_loop.exhausted"); e != nil {
		t.Errorf("agent_loop.exhausted should NOT be emitted, got %v", e)
	}

	// Old PR #743 event names must not appear.
	for _, old := range []string{"agent_loop.plan_complete", "agent_loop.tool_call", "agent_loop.final"} {
		if e := findEvent(t, buf, old); e != nil {
			t.Errorf("old event %q should not be emitted, got %v", old, e)
		}
	}
}

// TestSemiconductorLLMAgent_Metrics_ToolFailure verifies that
// tool errors emit agent_loop.tool with success=false. Uses an
// unknown tool name to trigger the RunToolCall error path.
func TestSemiconductorLLMAgent_Metrics_ToolFailure(t *testing.T) {
	logger, buf := captureMetricsLogger(t)
	mock := NewMockLLMDriver().
		WithPlanResponse([]PlanStep{
			{Kind: "tool", ToolName: "does_not_exist"},
		}).
		WithReflectResponse(Reflection{Continue: false, FinalConviction: 50})

	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	_, _ = agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)

	e := findEvent(t, buf, "agent_loop.tool")
	if e == nil {
		t.Fatal("missing agent_loop.tool event")
	}
	if e["name"] != "does_not_exist" {
		t.Errorf("agent_loop.tool.name = %v, want %q", e["name"], "does_not_exist")
	}
	if e["success"] != false {
		t.Errorf("agent_loop.tool.success = %v, want false", e["success"])
	}
}

// TestSemiconductorLLMAgent_Metrics_PlanFailure verifies that a plan
// error emits agent_loop.plan with size=0, a non-nil err, and a
// latency_ms. It also asserts that agent_loop.end still fires via defer
// on the early-return failure path.
func TestSemiconductorLLMAgent_Metrics_PlanFailure(t *testing.T) {
	logger, buf := captureMetricsLogger(t)
	planErr := errors.New("LLM planner unavailable")
	mock := NewMockLLMDriver().WithPlanError(planErr)

	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if ok {
		t.Error("Recommend with plan error: got ok=true, want false")
	}
	if rec.Reason == "" {
		t.Error("rec.Reason should describe the plan error, got empty")
	}

	e := findEvent(t, buf, "agent_loop.plan")
	if e == nil {
		t.Fatal("missing agent_loop.plan event")
	}
	if got := e["size"]; got != float64(0) {
		t.Errorf("agent_loop.plan.size = %v, want 0", got)
	}
	if _, exists := e["latency_ms"]; !exists {
		t.Error("agent_loop.plan missing latency_ms")
	}
	if e["err"] == nil {
		t.Errorf("agent_loop.plan.err = %v, want non-nil", e["err"])
	}

	if findEvent(t, buf, "agent_loop.end") == nil {
		t.Error("agent_loop.end should fire via defer even on plan failure")
	}

	// No tool/reflect events should fire when planning fails.
	for _, msg := range []string{"agent_loop.tool", "agent_loop.reflect"} {
		if findEvent(t, buf, msg) != nil {
			t.Errorf("%q should not be emitted when plan fails", msg)
		}
	}
}
