package experiment

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestLifecyclePublisher_TransitionAndPublish_Accepted verifies that
// transitioning an experiment to Accepted publishes
// EventExperimentAccepted with severity=info. Financial-engineering
// regression: audit trail for baseline promotion.
func TestLifecyclePublisher_TransitionAndPublish_Accepted(t *testing.T) {
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

	pub := NewLifecyclePublisher(bus)
	rec := &experiment.ExperimentRecord{
		ID:            "exp-001",
		ProposalID:    "prop-001",
		TargetAgentID: "agent-1",
		Skill:         "macro_radar",
		Status:        experiment.ExperimentRunning,
	}
	if err := pub.TransitionAndPublish(rec, experiment.ExperimentAccepted); err != nil {
		t.Fatalf("TransitionAndPublish: %v", err)
	}
	if rec.Status != experiment.ExperimentAccepted {
		t.Errorf("expected Status=Accepted, got %s", rec.Status)
	}

	select {
	case e := <-received:
		if e.Type != eventbus.EventExperimentAccepted {
			t.Errorf("expected EventExperimentAccepted, got %s", e.Type)
		}
		if e.Severity != "info" {
			t.Errorf("expected severity=info, got %s", e.Severity)
		}
		payload, ok := e.Payload.(eventbus.ExperimentLifecyclePayload)
		if !ok {
			t.Fatalf("expected payload type ExperimentLifecyclePayload, got %T", e.Payload)
		}
		if payload.ExperimentID != "exp-001" {
			t.Errorf("ExperimentID mismatch: %s", payload.ExperimentID)
		}
		if payload.TargetAgentID != "agent-1" {
			t.Errorf("TargetAgentID mismatch: %s", payload.TargetAgentID)
		}
		if payload.Skill != "macro_radar" {
			t.Errorf("Skill mismatch: %s", payload.Skill)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventExperimentAccepted")
	}
}

// TestLifecyclePublisher_TransitionAndPublish_Rejected verifies that
// transitioning to Rejected publishes EventExperimentRejected with
// severity=error (per brief: experiment_rejected → CRITICAL alert).
func TestLifecyclePublisher_TransitionAndPublish_Rejected(t *testing.T) {
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

	pub := NewLifecyclePublisher(bus)
	rec := &experiment.ExperimentRecord{
		ID:            "exp-002",
		ProposalID:    "prop-002",
		TargetAgentID: "agent-2",
		Skill:         "semiconductor",
		Status:        experiment.ExperimentRunning,
		RevertReason:  "sharpe_degradation",
	}
	if err := pub.TransitionAndPublish(rec, experiment.ExperimentRejected); err != nil {
		t.Fatalf("TransitionAndPublish: %v", err)
	}
	if rec.Status != experiment.ExperimentRejected {
		t.Errorf("expected Status=Rejected, got %s", rec.Status)
	}

	select {
	case e := <-received:
		if e.Type != eventbus.EventExperimentRejected {
			t.Errorf("expected EventExperimentRejected, got %s", e.Type)
		}
		if e.Severity != "error" {
			t.Errorf("expected severity=error, got %s", e.Severity)
		}
		payload := e.Payload.(eventbus.ExperimentLifecyclePayload)
		if payload.RevertReason != "sharpe_degradation" {
			t.Errorf("RevertReason mismatch: %s", payload.RevertReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventExperimentRejected")
	}
}

// TestLifecyclePublisher_TransitionAndPublish_InvalidTransition verifies
// that an invalid transition returns an error AND does NOT publish any event.
func TestLifecyclePublisher_TransitionAndPublish_InvalidTransition(t *testing.T) {
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

	pub := NewLifecyclePublisher(bus)
	rec := &experiment.ExperimentRecord{
		ID:     "exp-003",
		Status: experiment.ExperimentAccepted, // terminal: cannot transition out
	}
	err := pub.TransitionAndPublish(rec, experiment.ExperimentRunning)
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}
	if rec.Status != experiment.ExperimentAccepted {
		t.Errorf("status should remain unchanged, got %s", rec.Status)
	}
	select {
	case e := <-received:
		t.Fatalf("unexpected event published: type=%s severity=%s", e.Type, e.Severity)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event.
	}
}

// TestLifecyclePublisher_NilBusNoop verifies the no-bus branch is safe.
func TestLifecyclePublisher_NilBusNoop(t *testing.T) {
	pub := NewLifecyclePublisher(nil)
	rec := &experiment.ExperimentRecord{
		ID:     "exp-004",
		Status: experiment.ExperimentRunning,
	}
	if err := pub.TransitionAndPublish(rec, experiment.ExperimentAccepted); err != nil {
		t.Errorf("nil bus should not block transition: %v", err)
	}
	if rec.Status != experiment.ExperimentAccepted {
		t.Errorf("expected Status=Accepted, got %s", rec.Status)
	}
}

// TestLifecyclePublisher_NoEventForNonLifecycleStatus verifies that
// transitioning to a non-lifecycle status (e.g. Running) does NOT publish
// an event (only Accepted/Rejected are lifecycle events per the brief).
func TestLifecyclePublisher_NoEventForNonLifecycleStatus(t *testing.T) {
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

	pub := NewLifecyclePublisher(bus)
	rec := &experiment.ExperimentRecord{
		ID:     "exp-005",
		Status: experiment.ExperimentPlanned,
	}
	if err := pub.TransitionAndPublish(rec, experiment.ExperimentRunning); err != nil {
		t.Errorf("TransitionAndPublish: %v", err)
	}
	select {
	case e := <-received:
		t.Fatalf("unexpected event for non-lifecycle status: type=%s", e.Type)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event.
	}
}
