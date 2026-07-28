package apigateway

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// ErrTaskSkipped is returned by a task to indicate "nothing to do this tick"
// (e.g. outside its daily window). A skip is neither success nor failure:
// consecutiveFailures is left untouched so a real failure is not washed away
// by subsequent no-op ticks (fix manifest #B01).
var ErrTaskSkipped = errors.New("background task: skipped")

// BackgroundTaskFunc is the function signature for background tasks.
type BackgroundTaskFunc func(ctx context.Context) error

// ScheduledTask represents a registered background task.
type ScheduledTask struct {
	Name                string
	ChannelID           string
	Interval            time.Duration
	Jitter              time.Duration
	Task                BackgroundTaskFunc
	Enabled             bool
	enabledMu           sync.RWMutex
	lastRun             time.Time
	lastRunMu           sync.RWMutex
	consecutiveFailures int
	failuresMu          sync.Mutex
	lastError           string
	lastErrorMu         sync.RWMutex

	// Data-health fields (#1265): separated from run-success so
	// "task ran without error" is not confused with "new data arrived".
	dataHealthMu     sync.RWMutex
	lastDataAsOf     time.Time // timestamp of the newest ingested data point
	lastNewSamples   int       // count of new records/rows added
	lastPersistedAt  time.Time // time when data was written to store
	noProgressReason string    // e.g. "source not yet published", "empty response"
}

// IsEnabled returns whether the task is enabled.
func (t *ScheduledTask) IsEnabled() bool {
	t.enabledMu.RLock()
	defer t.enabledMu.RUnlock()
	return t.Enabled
}

// SetEnabled sets the task enabled state.
func (t *ScheduledTask) SetEnabled(enabled bool) {
	t.enabledMu.Lock()
	defer t.enabledMu.Unlock()
	t.Enabled = enabled
}

// LastRun returns the last execution time.
func (t *ScheduledTask) LastRun() time.Time {
	t.lastRunMu.RLock()
	defer t.lastRunMu.RUnlock()
	return t.lastRun
}

// SetLastRun updates the last execution time.
func (t *ScheduledTask) SetLastRun(t2 time.Time) {
	t.lastRunMu.Lock()
	defer t.lastRunMu.Unlock()
	t.lastRun = t2
}

// Failures returns consecutive failure count.
func (t *ScheduledTask) Failures() int {
	t.failuresMu.Lock()
	defer t.failuresMu.Unlock()
	return t.consecutiveFailures
}

// RecordSuccess resets failure count and clears the last error.
func (t *ScheduledTask) RecordSuccess() {
	t.failuresMu.Lock()
	t.consecutiveFailures = 0
	t.failuresMu.Unlock()
	t.lastErrorMu.Lock()
	t.lastError = ""
	t.lastErrorMu.Unlock()
}

// RecordFailure increments failure count.
func (t *ScheduledTask) RecordFailure() {
	t.failuresMu.Lock()
	defer t.failuresMu.Unlock()
	t.consecutiveFailures++
}

// SetLastError records the most recent failure message for status reporting.
func (t *ScheduledTask) SetLastError(err error) {
	t.lastErrorMu.Lock()
	defer t.lastErrorMu.Unlock()
	if err == nil {
		t.lastError = ""
		return
	}
	t.lastError = err.Error()
}

// LastError returns the most recent failure message (empty if none).
func (t *ScheduledTask) LastError() string {
	t.lastErrorMu.RLock()
	defer t.lastErrorMu.RUnlock()
	return t.lastError
}

// SetDataHealth records data-health metrics after a successful fetch.
// All fields are optional — tasks that don't track data health can pass zero values.
func (t *ScheduledTask) SetDataHealth(asOf time.Time, newSamples int, persistedAt time.Time, noProgressReason string) {
	t.dataHealthMu.Lock()
	defer t.dataHealthMu.Unlock()
	t.lastDataAsOf = asOf
	t.lastNewSamples = newSamples
	t.lastPersistedAt = persistedAt
	t.noProgressReason = noProgressReason
}

// DataHealth returns the last recorded data-health snapshot.
func (t *ScheduledTask) DataHealth() (asOf time.Time, newSamples int, persistedAt time.Time, noProgressReason string) {
	t.dataHealthMu.RLock()
	defer t.dataHealthMu.RUnlock()
	return t.lastDataAsOf, t.lastNewSamples, t.lastPersistedAt, t.noProgressReason
}

// TaskFailureHandler is called when a task fails, receiving the task name and error.
type TaskFailureHandler func(taskName string, consecutiveFailures int, err error)

// TaskRecoveryHandler is called when a task recovers after consecutive failures.
type TaskRecoveryHandler func(taskName string, recoveredFrom int)

// BackgroundTaskManager coordinates all background data fetch tasks.
type BackgroundTaskManager struct {
	gateway         *Gateway
	registry        map[string]*ScheduledTask
	mu              sync.RWMutex
	wg              sync.WaitGroup
	cancel          context.CancelFunc
	failureHandler  TaskFailureHandler
	recoveryHandler TaskRecoveryHandler
}

// NewBackgroundTaskManager creates a task manager.
func NewBackgroundTaskManager(gateway *Gateway) *BackgroundTaskManager {
	return &BackgroundTaskManager{
		gateway:  gateway,
		registry: make(map[string]*ScheduledTask),
	}
}

// Register adds a task to the registry.
//
// ChannelID contract:
//
// When a ScheduledTask has a non-empty ChannelID, the task's BackgroundTaskFunc
// MUST call gateway.Fetch(channelID) to retrieve data. It MUST NOT bypass the
// gateway by using a raw http.Client or other direct-fetch mechanism.
//
// This contract exists because:
//   - Circuit breaker state transitions (Open→HalfOpen→Closed) are driven by
//     breaker.Call() inside Gateway.Fetch(). A task that bypasses the gateway
//     can leave the breaker permanently Open, since no probe ever fires.
//   - Cache, rate limiting, and health tracking are also applied at the
//     gateway layer. Bypassing them produces stale data, missed rate-limit
//     backpressure, and inaccurate failure counts.
//
// Register validates the channel exists in the gateway when ChannelID is set.
// Builds that violate this contract will be rejected in review.
func (m *BackgroundTaskManager) Register(task *ScheduledTask) error {
	if task.ChannelID != "" && m.gateway != nil && !m.gateway.HasChannel(task.ChannelID) {
		return fmt.Errorf("task %s: channel %s not registered in gateway", task.Name, task.ChannelID)
	}

	if task.Jitter == 0 && task.Interval > 0 {
		task.Jitter = time.Duration(0.1 * float64(task.Interval))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.registry[task.Name]; exists {
		return fmt.Errorf("duplicate task name %q: BackgroundTaskManager.Register silently overwrites by name; if intentional, remove this check or rename the task", task.Name)
	}
	m.registry[task.Name] = task
	return nil
}

// Get returns a registered task.
func (m *BackgroundTaskManager) Get(name string) (*ScheduledTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.registry[name]
	return t, ok
}

// List returns all registered task names.
func (m *BackgroundTaskManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.registry))
	for name := range m.registry {
		names = append(names, name)
	}
	return names
}

// Start begins executing all registered tasks.
func (m *BackgroundTaskManager) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	m.mu.RLock()
	tasks := make([]*ScheduledTask, 0, len(m.registry))
	for _, t := range m.registry {
		tasks = append(tasks, t)
	}
	m.mu.RUnlock()

	for _, task := range tasks {
		m.wg.Add(1)
		go m.runTask(ctx, task)
	}
}

// SetFailureHandler sets a callback invoked when any task fails.
func (m *BackgroundTaskManager) SetFailureHandler(h TaskFailureHandler) {
	m.failureHandler = h
}

// safeCallFailureHandler invokes m.failureHandler inside a nested defer recover
// so a misbehaving handler cannot propagate a panic and crash the process.
// Handler panics are logged with full stack trace and the task name for
// postmortem. A nil handler is a no-op.
func (m *BackgroundTaskManager) safeCallFailureHandler(taskName string, consecutiveFailures int, err error) {
	if m.failureHandler == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			logging.Error(
				"background_task", "failureHandler_panic_recovered",
				"task_name", taskName,
				"consecutive_failures", consecutiveFailures,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(stack),
			)
		}
	}()
	m.failureHandler(taskName, consecutiveFailures, err)
}

func logAndWrapPanic(taskName, event string, r interface{}) error {
	if r == nil {
		return nil
	}
	stack := debug.Stack()
	logging.Error(
		"background_task", event,
		"name", taskName,
		"panic", fmt.Sprintf("%v", r),
		"stack", string(stack),
	)
	return fmt.Errorf("task panicked: %v", r)
}

// SetRecoveryHandler sets a callback invoked when a task recovers from failures.
func (m *BackgroundTaskManager) SetRecoveryHandler(h TaskRecoveryHandler) {
	m.recoveryHandler = h
}

// Stop gracefully shuts down all tasks.
func (m *BackgroundTaskManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *BackgroundTaskManager) runTask(ctx context.Context, task *ScheduledTask) {
	defer m.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			if err := logAndWrapPanic(task.Name, "runTask_panic_recovered", r); err != nil {
				m.safeCallFailureHandler(task.Name, -1, err)
			}
		}
	}()

	// Apply startup jitter to prevent thundering herd
	// Apply startup jitter to prevent thundering herd, but ONLY after
	// the first execution (the initial run fires immediately so that
	// long-interval tasks like government_flow_aggregate don't sit
	// idle for hours on a fresh deploy).
	if !task.LastRun().IsZero() && task.Jitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(task.Jitter)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
	}
	ticker := time.NewTicker(task.Interval)
	logging.Info("background_task", "task_started", "name", task.Name)
	defer ticker.Stop()

	// Execute immediately on start
	m.executeTask(ctx, task)

	for {
		select {
		case <-ctx.Done():
			logging.Info("background_task", "task_stopped", "name", task.Name)
			return
		case <-ticker.C:
			m.executeTask(ctx, task)
		}
	}
}

func (m *BackgroundTaskManager) executeTask(ctx context.Context, task *ScheduledTask) {
	if !task.IsEnabled() {
		return
	}

	// Debounce ticks that fire significantly earlier than the configured
	// interval (phase shift after a long run). A tolerance absorbs
	// millisecond-level scheduling jitter that used to kill legitimate
	// ticks (fix manifest #B02).
	if !task.LastRun().IsZero() && time.Since(task.LastRun()) < task.Interval-overlapTolerance(task.Interval) {
		logging.Warn("background_task", "task_skipped_overlap", "name", task.Name)
		return
	}

	task.SetLastRun(time.Now())

	// Intentionally NOT pre-checking breaker.IsOpen(): if we returned early
	// here, the half-open probe inside breaker.Call() (gateway.Fetch path)
	// would never fire for tasks that are the channel's only caller,
	// leaving the breaker permanently open. See
	// docs/incidents/2026-06-fubon-channel-recurring-failure.md.

	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = logAndWrapPanic(task.Name, "task_panic_recovered", r)
			}
		}()
		return task.Task(ctx)
	}()
	if errors.Is(err, ErrTaskSkipped) {
		// No-op tick: not a success (failure count must survive), not a failure.
		logging.Info("background_task", "task_skipped", "name", task.Name)
		return
	}
	if err != nil {
		task.RecordFailure()
		task.SetLastError(err)
		logging.Error(
			"background_task", "task_failed",
			"name", task.Name,
			"err", err.Error(),
			"consecutive_failures", task.Failures(),
		)
		m.safeCallFailureHandler(task.Name, task.Failures(), err)
	} else {
		prev := task.Failures()
		task.RecordSuccess()
		if prev > 0 && m.recoveryHandler != nil {
			m.recoveryHandler(task.Name, prev)
		}
	}
}

// overlapTolerance returns how much earlier than the full interval a tick
// may fire before it is treated as an overlap: max(1s, interval/20).
func overlapTolerance(interval time.Duration) time.Duration {
	tol := interval / 20
	if tol < time.Second {
		tol = time.Second
	}
	return tol
}

// TaskStatus represents the runtime status of a task.
type TaskStatus struct {
	Name                string        `json:"name"`
	ChannelID           string        `json:"channel_id"`
	Enabled             bool          `json:"enabled"`
	Interval            time.Duration `json:"interval"`
	LastRun             time.Time     `json:"last_run"`
	NextRun             time.Time     `json:"next_run"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	LastError           string        `json:"last_error,omitempty"`

	// Data-health fields (#1265): separated from run-success.
	LastDataAsOf     time.Time `json:"last_data_as_of,omitempty"`
	LastNewSamples   int       `json:"last_new_samples"`
	LastPersistedAt  time.Time `json:"last_persisted_at,omitempty"`
	NoProgressReason string    `json:"no_progress_reason,omitempty"`
}

// Status returns runtime status for all tasks.
func (m *BackgroundTaskManager) Status() []TaskStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TaskStatus, 0, len(m.registry))
	for _, t := range m.registry {
		var nextRun time.Time
		if !t.LastRun().IsZero() {
			nextRun = t.LastRun().Add(t.Interval)
		}
		asOf, newSamples, persistedAt, noProgress := t.DataHealth()
		result = append(result, TaskStatus{
			Name:                t.Name,
			ChannelID:           t.ChannelID,
			Enabled:             t.IsEnabled(),
			Interval:            t.Interval,
			LastRun:             t.LastRun(),
			NextRun:             nextRun,
			ConsecutiveFailures: t.Failures(),
			LastError:           t.LastError(),
			LastDataAsOf:        asOf,
			LastNewSamples:      newSamples,
			LastPersistedAt:     persistedAt,
			NoProgressReason:    noProgress,
		})
	}
	return result
}
