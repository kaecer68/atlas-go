package orchestrator

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// captureMetricsLogger returns a JSON-handler-backed slog.Logger that
// writes to a buffer, plus the buffer itself, so tests can assert the
// L2.4 events emitted by SemiconductorLLMAgent.Recommend. The returned
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
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
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

// TestSemiconductorLLMAgent_L24Metrics_HappyPath verifies that the
// full plan → tool_call → reflect loop emits the expected L2.4
// observation events with field names aligned to
// docs/wave-11/L2_4_OBSERVATION.md.
func TestSemiconductorLLMAgent_L24Metrics_HappyPath(t *testing.T) {
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
		LLMDriver:      mock,
		Tools:          llm.TestTools(),
		MaxIter:        3,
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if !ok {
		t.Fatalf("Recommend: got ok=false, rec.Reason=%q", rec.Reason)
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
		if _, ok := e["max_iter"]; !ok {
			t.Error("agent_loop.start missing max_iter")
		}
	}

	if e := findEvent(t, buf, "agent_loop.plan_complete"); e == nil {
		t.Error("missing agent_loop.plan_complete event")
	} else {
		if got := e["plan_size"]; got != float64(1) {
			t.Errorf("agent_loop.plan_complete.plan_size = %v, want 1", got)
		}
		if _, ok := e["round"]; !ok {
			t.Error("agent_loop.plan_complete missing round")
		}
		if _, ok := e["latency_ms"]; !ok {
			t.Error("agent_loop.plan_complete missing latency_ms")
		}
	}

	if e := findEvent(t, buf, "agent_loop.tool_call"); e == nil {
		t.Error("missing agent_loop.tool_call event")
	} else {
		if e["tool_name"] != "get_factor_weight" {
			t.Errorf("agent_loop.tool_call.tool_name = %v, want %q", e["tool_name"], "get_factor_weight")
		}
		if e["success"] != true {
			t.Errorf("agent_loop.tool_call.success = %v, want true", e["success"])
		}
		if _, ok := e["latency_ms"]; !ok {
			t.Error("agent_loop.tool_call missing latency_ms")
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
		if _, ok := e["latency_ms"]; !ok {
			t.Error("agent_loop.reflect missing latency_ms")
		}
	}

	if e := findEvent(t, buf, "agent_loop.final"); e == nil {
		t.Error("missing agent_loop.final event")
	} else {
		if got := e["conviction"]; got != float64(75) {
			t.Errorf("agent_loop.final.conviction = %v, want 75", got)
		}
		if e["symbol"] != "2330" {
			t.Errorf("agent_loop.final.symbol = %v, want %q", e["symbol"], "2330")
		}
	}

	// Exhausted should NOT be emitted on the happy path (Continue=false
	// exits the loop before Round reaches MaxIter).
	if e := findEvent(t, buf, "agent_loop.exhausted"); e != nil {
		t.Errorf("agent_loop.exhausted should NOT be emitted on happy path, got %v", e)
	}
}

// TestSemiconductorLLMAgent_L24Metrics_Exhausted verifies that the
// exhausted event fires when the loop hits MaxIter (Round >= MaxIter)
// before Continue=false. Continue=true + MaxIter=1 makes a single
// plan step push Round to 1, then on iteration 2 the for-loop guard
// terminates without a new plan — Exhausted() returns true since
// Round >= MaxIter.
func TestSemiconductorLLMAgent_L24Metrics_Exhausted(t *testing.T) {
	logger, buf := captureMetricsLogger(t)
	mock := NewMockLLMDriver().
		WithPlanResponse([]PlanStep{
			{Kind: "tool", ToolName: "get_factor_weight"},
		}).
		WithReflectResponse(Reflection{
			Continue:        true, // loop continues; Round keeps growing
			FinalConviction: 50,
		})

	agent := SemiconductorLLMAgent{
		LLMDriver:      mock,
		Tools:          llm.TestTools(),
		MaxIter:        1,
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	_, _ = agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)

	if e := findEvent(t, buf, "agent_loop.exhausted"); e == nil {
		t.Error("missing agent_loop.exhausted event")
	} else {
		if got := e["round"]; got != float64(1) {
			t.Errorf("agent_loop.exhausted.round = %v, want 1", got)
		}
		if got := e["max_iter"]; got != float64(1) {
			t.Errorf("agent_loop.exhausted.max_iter = %v, want 1", got)
		}
	}
}

// TestSemiconductorLLMAgent_L24Metrics_ToolFailure verifies that
// tool errors emit agent_loop.tool_call with success=false. Uses an
// unknown tool name to trigger the RunToolCall error path.
func TestSemiconductorLLMAgent_L24Metrics_ToolFailure(t *testing.T) {
	logger, buf := captureMetricsLogger(t)
	mock := NewMockLLMDriver().
		WithPlanResponse([]PlanStep{
			{Kind: "tool", ToolName: "does_not_exist"},
		}).
		WithReflectResponse(Reflection{Continue: false, FinalConviction: 50})

	agent := SemiconductorLLMAgent{
		LLMDriver:      mock,
		Tools:          llm.TestTools(),
		UseLLMOverride: truePtr(),
		Metrics:        logger,
	}

	_, _ = agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)

	e := findEvent(t, buf, "agent_loop.tool_call")
	if e == nil {
		t.Fatal("missing agent_loop.tool_call event")
	}
	if e["success"] != false {
		t.Errorf("agent_loop.tool_call.success = %v, want false", e["success"])
	}
}
