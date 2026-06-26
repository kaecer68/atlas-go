package monitoring

import (
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// BackgroundTaskFailureTracker tracks consecutive failures of a single
// background task. Decision 4 (alert-redesign-v2.md Part 3.2): a single
// or two failures are transient and should NOT page anyone; only when
// consecutive_failures >= threshold does the tracker emit a SustainedFailure
// event. The first success after SustainedFailure emits Recovered and
// resets the counter (the "auto-resolve" pattern).
//
// Nil bus is allowed — tracker is then a no-op for publish, still tracks
// the counter for the caller's introspection.
type BackgroundTaskFailureTracker struct {
	bus       *eventbus.ChannelEventBus
	taskName  string
	threshold int

	mu                 sync.Mutex
	consecutiveFails   int
	hasAlertedSustFail bool
}

// NewBackgroundTaskFailureTracker creates a tracker for a single named
// task. threshold is the number of consecutive failures that triggers
// the SustainedFailure event (typically 3 per the brief).
func NewBackgroundTaskFailureTracker(bus *eventbus.ChannelEventBus, taskName string, threshold int) *BackgroundTaskFailureTracker {
	return &BackgroundTaskFailureTracker{
		bus:       bus,
		taskName:  taskName,
		threshold: threshold,
	}
}

// OnFailure records a failure for the tracked task. Increments the
// consecutive-failure counter and, on reaching the threshold, publishes
// EventBackgroundTaskSustainedFailure (severity=error). Subsequent
// failures do NOT re-publish until OnSuccess resets the counter (avoids
// alert fatigue from sustained outages).
func (t *BackgroundTaskFailureTracker) OnFailure() {
	t.mu.Lock()
	t.consecutiveFails++
	count := t.consecutiveFails
	alreadyAlerted := t.hasAlertedSustFail
	if count >= t.threshold {
		t.hasAlertedSustFail = true
	}
	bus := t.bus
	taskName := t.taskName
	threshold := t.threshold
	t.mu.Unlock()

	if bus != nil && count >= t.threshold && !alreadyAlerted {
		bus.PublishBackgroundTaskSustainedFailure(eventbus.BackgroundTaskPayload{
			TaskName:            taskName,
			ConsecutiveFailures: count,
			Threshold:           threshold,
			Timestamp:           time.Now(),
		})
	}
}

// OnSuccess records a success for the tracked task. If a SustainedFailure
// alert was previously emitted, publish EventBackgroundTaskRecovered
// (the auto-resolve signal) and reset the counter.
func (t *BackgroundTaskFailureTracker) OnSuccess() {
	t.mu.Lock()
	wasAlerted := t.hasAlertedSustFail
	t.consecutiveFails = 0
	t.hasAlertedSustFail = false
	bus := t.bus
	taskName := t.taskName
	t.mu.Unlock()

	if bus != nil && wasAlerted {
		bus.PublishBackgroundTaskRecovered(eventbus.BackgroundTaskPayload{
			TaskName:            taskName,
			ConsecutiveFailures: 0,
			Threshold:           0, // Not relevant on recovery; set to 0 for clarity.
			Timestamp:           time.Now(),
		})
	}
}

// ConsecutiveFailures returns the current consecutive-failure count
// (for the caller's introspection / dashboard). Thread-safe.
func (t *BackgroundTaskFailureTracker) ConsecutiveFailures() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.consecutiveFails
}
