package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestBackgroundTaskFailureTracker_BelowThreshold verifies that a single
// or two failures do NOT emit any event (transient failures are ignored).
// Per Decision 4 (alert-redesign-v2.md Part 3.2): 1-2 failures are
// transient and should not page anyone.
func TestBackgroundTaskFailureTracker_BelowThreshold(t *testing.T) {
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

	tr := NewBackgroundTaskFailureTracker(bus, "test-task", 3)
	tr.OnFailure()
	tr.OnFailure()
	select {
	case e := <-received:
		t.Fatalf("unexpected event for transient failure: type=%s", e.Type)
	case <-time.After(100 * time.Millisecond):
		// Expected: no event.
	}
}

// TestBackgroundTaskFailureTracker_ThirdFailureEmitsAlert verifies that the
// THIRD consecutive failure emits EventBackgroundTaskSustainedFailure
// with severity=error. This is the key alert signal.
func TestBackgroundTaskFailureTracker_ThirdFailureEmitsAlert(t *testing.T) {
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

	tr := NewBackgroundTaskFailureTracker(bus, "test-task", 3)
	tr.OnFailure()
	tr.OnFailure()
	tr.OnFailure()
	select {
	case e := <-received:
		if e.Type != eventbus.EventBackgroundTaskSustainedFailure {
			t.Errorf("expected EventBackgroundTaskSustainedFailure, got %s", e.Type)
		}
		if e.Severity != "error" {
			t.Errorf("expected severity=error, got %s", e.Severity)
		}
		payload, ok := e.Payload.(eventbus.BackgroundTaskPayload)
		if !ok {
			t.Fatalf("expected payload type BackgroundTaskPayload, got %T", e.Payload)
		}
		if payload.TaskName != "test-task" {
			t.Errorf("expected TaskName=test-task, got %s", payload.TaskName)
		}
		if payload.ConsecutiveFailures != 3 {
			t.Errorf("expected ConsecutiveFailures=3, got %d", payload.ConsecutiveFailures)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sustained-failure event after 3rd failure")
	}
}

// TestBackgroundTaskFailureTracker_RecoveryResetsCounter verifies that
// after the alert is emitted, a successful run resets the counter AND
// emits EventBackgroundTaskRecovered (the "auto-resolve" signal).
func TestBackgroundTaskFailureTracker_RecoveryResetsCounter(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()
	received := make(chan eventbus.BusEvent, 8)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	tr := NewBackgroundTaskFailureTracker(bus, "test-task", 3)
	tr.OnFailure()
	tr.OnFailure()
	tr.OnFailure()
	// Drain the sustained-failure event.
	<-received
	// Now succeed — should reset counter and emit recovered.
	tr.OnSuccess()
	select {
	case e := <-received:
		if e.Type != eventbus.EventBackgroundTaskRecovered {
			t.Errorf("expected EventBackgroundTaskRecovered, got %s", e.Type)
		}
		if e.Severity != "info" {
			t.Errorf("expected severity=info, got %s", e.Severity)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovery event")
	}
	// After recovery, a single failure should not alert (counter reset).
	tr.OnFailure()
	select {
	case e := <-received:
		t.Fatalf("unexpected event after recovery: type=%s", e.Type)
	case <-time.After(100 * time.Millisecond):
		// Expected: no event.
	}
}

// TestBackgroundTaskFailureTracker_NilBusNoop verifies the no-bus branch
// is safe (no panic). Same nil-safety pattern as other consumers in
// the monitoring package.
func TestBackgroundTaskFailureTracker_NilBusNoop(t *testing.T) {
	tr := NewBackgroundTaskFailureTracker(nil, "test-task", 3)
	tr.OnFailure()
	tr.OnFailure()
	tr.OnFailure()
	tr.OnSuccess() // must not panic
}

// TestBackgroundTaskFailureTracker_RepeatedFailuresEmitOnce verifies that
// after the threshold is reached and the alert is emitted, additional
// consecutive failures do NOT spam duplicate alerts. The "auto-resolve
// upon recovery" pattern is the way to clear the alert.
func TestBackgroundTaskFailureTracker_RepeatedFailuresEmitOnce(t *testing.T) {
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

	tr := NewBackgroundTaskFailureTracker(bus, "test-task", 3)
	for range 6 {
		tr.OnFailure()
	}
	// Expect exactly 1 event (from the 3rd failure), not 4 events.
	count := 0
	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-received:
			if e.Type == eventbus.EventBackgroundTaskSustainedFailure {
				count++
			}
		case <-deadline:
			if count != 1 {
				t.Errorf("expected exactly 1 sustained-failure event, got %d", count)
			}
			return
		}
	}
}
