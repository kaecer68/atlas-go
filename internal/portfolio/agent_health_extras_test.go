package portfolio

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestWithEventBus verifies that WithEventBus attaches the bus and returns the
// same manager (chainable). It also confirms the bus is stored on the manager.
func TestWithEventBus(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	result := m.WithEventBus(bus)
	if result != m {
		t.Fatalf("WithEventBus must return the same *AgentHealthManager (chainable), got %v", result)
	}
	if m.eventBus != bus {
		t.Errorf("expected eventBus to be attached, got %v / want %v", m.eventBus, bus)
	}
}

// TestPublishHealthChange_FiresOnNegativeSharpe is a financial-engineering-grade
// regression test: when a manager has an event bus, a status transition must
// publish an AgentHealthChange event. We trigger the negative-Sharpe mute path
// (Sharpe = -0.6 < default threshold -0.5 → status=muted → publishHealthChange).
func TestPublishHealthChange_FiresOnNegativeSharpe(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	m.WithEventBus(bus)

	const agentID = "agent-publish-test"
	// Single negative-Sharpe outcome with sharpe = -0.6 (< default threshold -0.5)
	// triggers the negative_sharpe mute path → publishHealthChange.
	m.RecordOutcome(agentID, false, -0.6, 0.4)

	h := m.GetHealth(agentID)
	if h == nil {
		t.Fatal("agent health not created")
	}
	if h.Status != HealthStatusMuted {
		t.Fatalf("expected status=muted, got %s", h.Status)
	}

	// publishHealthChange dispatches via the bus's internal goroutine; wait
	// briefly for the AgentHealthChange event to reach our handler.
	select {
	case e := <-received:
		if e.Type != eventbus.EventAgentHealthChange {
			t.Errorf("expected AgentHealthChange event, got type=%s", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AgentHealthChange event")
	}
}

// TestPublishHealthChange_NilBusIsSafe verifies the no-bus branch does not
// panic when a health change would otherwise fire. With eventBus == nil,
// RecordOutcome must complete without panicking and still transition state.
func TestPublishHealthChange_NilBusIsSafe(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	const agentID = "agent-no-bus"

	// No bus attached — should silently skip publishing.
	m.RecordOutcome(agentID, false, -1.0, 0.3)
	m.RecordOutcome(agentID, false, -1.0, 0.3)
	m.RecordOutcome(agentID, false, -1.0, 0.3)
	m.RecordOutcome(agentID, false, -1.0, 0.3)
	m.RecordOutcome(agentID, false, -1.0, 0.3)

	h := m.GetHealth(agentID)
	if h == nil || h.Status != HealthStatusMuted {
		t.Fatalf("expected agent to be muted without bus, got %+v", h)
	}
	if m.eventBus != nil {
		t.Errorf("expected eventBus to remain nil, got %v", m.eventBus)
	}
}

// TestString_NoAgents verifies String() output for an empty manager.
func TestString_NoAgents(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	out := m.String()
	if !strings.HasPrefix(out, "AgentHealthManager{") {
		t.Errorf("String() must start with 'AgentHealthManager{', got %q", out)
	}
	if !strings.HasSuffix(out, "}") {
		t.Errorf("String() must end with '}', got %q", out)
	}
	if !strings.Contains(out, "config:") {
		t.Errorf("String() must include config section, got %q", out)
	}
}

// TestString_ContainsAllAgents verifies String() enumerates every tracked agent
// with the expected status, sharpe, hit-rate, and composite fields.
func TestString_ContainsAllAgents(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())

	m.RecordOutcome("agent-1", true, 1.2, 0.6)
	m.RecordOutcome("agent-2", false, -0.4, 0.45)
	for range 5 {
		m.RecordOutcome("agent-3", false, -0.7, 0.3)
	}

	out := m.String()
	if !strings.Contains(out, "agent-1") {
		t.Errorf("String() must include agent-1, got %q", out)
	}
	if !strings.Contains(out, "agent-2") {
		t.Errorf("String() must include agent-2, got %q", out)
	}
	if !strings.Contains(out, "agent-3") {
		t.Errorf("String() must include agent-3, got %q", out)
	}
	// status, sharpe, hitrate, losses, wins, composite fields should appear
	for _, want := range []string{"status=", "sharpe=", "hitrate=", "losses=", "wins=", "composite="} {
		if !strings.Contains(out, want) {
			t.Errorf("String() must include %q, got %q", want, out)
		}
	}

	// Verify the muted agent's status appears as "muted"
	if !strings.Contains(out, "status=muted") {
		t.Errorf("String() must mark muted agents, got %q", out)
	}
}

// TestJoinLines_Empty verifies joinLines on zero inputs returns an empty string.
func TestJoinLines_Empty(t *testing.T) {
	if got := joinLines(); got != "" {
		t.Errorf("joinLines() = %q, want \"\"", got)
	}
}

// TestJoinLines_SingleAndMultiple verifies joinLines concatenates with "\n"
// separators and includes all inputs in order.
func TestJoinLines_SingleAndMultiple(t *testing.T) {
	if got := joinLines("alpha"); got != "alpha" {
		t.Errorf("joinLines(\"alpha\") = %q, want \"alpha\"", got)
	}
	got := joinLines("a", "b", "c")
	want := "a\nb\nc"
	if got != want {
		t.Errorf("joinLines(a,b,c) = %q, want %q", got, want)
	}
}

// TestJoinLines_ConcurrentNoRace exercises joinLines from many goroutines
// (the helper itself is pure; we keep a small race-detector-friendly check).
func TestJoinLines_ConcurrentNoRace(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = joinLines("x", "y", "z")
		}()
	}
	wg.Wait()
}
