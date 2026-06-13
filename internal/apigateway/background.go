package apigateway

import (
	"context"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

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

// RecordSuccess resets failure count.
func (t *ScheduledTask) RecordSuccess() {
	t.failuresMu.Lock()
	defer t.failuresMu.Unlock()
	t.consecutiveFailures = 0
}

// RecordFailure increments failure count.
func (t *ScheduledTask) RecordFailure() {
	t.failuresMu.Lock()
	defer t.failuresMu.Unlock()
	t.consecutiveFailures++
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
func (m *BackgroundTaskManager) Register(task *ScheduledTask) error {
	if task.ChannelID != "" && m.gateway != nil && !m.gateway.HasChannel(task.ChannelID) {
		return fmt.Errorf("task %s: channel %s not registered in gateway", task.Name, task.ChannelID)
	}

	if task.Jitter == 0 && task.Interval > 0 {
		task.Jitter = time.Duration(0.1 * float64(task.Interval))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
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
			stack := debug.Stack()
			logging.Error(
				"background_task", "runTask_panic_recovered",
				"name", task.Name,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(stack),
			)
			m.safeCallFailureHandler(task.Name, -1, fmt.Errorf("runTask panicked: %v", r))
		}
	}()

	// Apply startup jitter to prevent thundering herd
	if task.Jitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(task.Jitter)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
	}

	ticker := time.NewTicker(task.Interval)
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

	// Check if previous run is still executing (mutual exclusion)
	if !task.LastRun().IsZero() && time.Since(task.LastRun()) < task.Interval {
		logging.Warn("background_task", "task_skipped_overlap", "name", task.Name)
		return
	}

	task.SetLastRun(time.Now())

	// If channel has circuit breaker and it's open, skip
	if task.ChannelID != "" {
		breaker, err := m.gateway.breakers.Get(task.ChannelID)
		if err == nil && breaker.IsOpen() {
			logging.Warn("background_task", "circuit_open_skipping", "name", task.Name, "channel", task.ChannelID)
			return
		}
	}

	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logging.Error(
					"background_task", "task_panic_recovered",
					"name", task.Name,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(stack),
				)
				err = fmt.Errorf("task panicked: %v", r)
			}
		}()
		return task.Task(ctx)
	}()
	if err != nil {
		task.RecordFailure()
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

// TaskStatus represents the runtime status of a task.
type TaskStatus struct {
	Name                string        `json:"name"`
	ChannelID           string        `json:"channel_id"`
	Enabled             bool          `json:"enabled"`
	Interval            time.Duration `json:"interval"`
	LastRun             time.Time     `json:"last_run"`
	NextRun             time.Time     `json:"next_run"` // Zero = task never ran; past time = missed schedule (overlap or extended runtime); future time = upcoming scheduled execution
	ConsecutiveFailures int           `json:"consecutive_failures"`
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
		result = append(result, TaskStatus{
			Name:                t.Name,
			ChannelID:           t.ChannelID,
			Enabled:             t.IsEnabled(),
			Interval:            t.Interval,
			LastRun:             t.LastRun(),
			NextRun:             nextRun,
			ConsecutiveFailures: t.Failures(),
		})
	}
	return result
}
