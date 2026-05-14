package apigateway

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// BackgroundTaskFunc is the function signature for background tasks.
type BackgroundTaskFunc func(ctx context.Context) error

// TaskFailureHandler is called when a task fails, receiving the task name and error.
type TaskFailureHandler func(taskName string, consecutiveFailures int, err error)

// RetryPolicy defines exponential backoff retry for a ScheduledTask.
type RetryPolicy struct {
	MaxAttempts  int           // Maximum retry attempts (0 = no retry)
	InitialDelay time.Duration // Delay before first retry (e.g., 1s)
	MaxDelay     time.Duration // Maximum delay cap (e.g., 30s)
	Multiplier   float64       // Delay multiplier per attempt (e.g., 2.0)
}

// ScheduledTask represents a registered background task.
type ScheduledTask struct {
	Name                string
	ChannelID           string
	Interval            time.Duration
	Jitter              time.Duration
	Task                BackgroundTaskFunc
	Enabled             bool
	RetryPolicy         *RetryPolicy
	FailureHandler      TaskFailureHandler
	MarketHoursOnly     bool
	MarketOpenTime      string // HHMM format, default "0900" (TWSE)
	MarketCloseTime     string // HHMM format, default "1330" (TWSE)
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

// isMarketOpen checks if the current time falls within the task's configured market hours.
// Uses task's MarketOpenTime/MarketCloseTime if set, otherwise defaults to TWSE hours (09:00-13:30 Asia/Taipei).
func isMarketOpen(task *ScheduledTask) bool {
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err == nil {
		now = now.In(loc)
	}

	openStr := task.MarketOpenTime
	closeStr := task.MarketCloseTime
	if openStr == "" {
		openStr = "0900"
	}
	if closeStr == "" {
		closeStr = "1330"
	}

	current := now.Hour()*100 + now.Minute()
	openVal, _ := strconv.Atoi(openStr)
	closeVal, _ := strconv.Atoi(closeStr)

	return current >= openVal && current < closeVal
}

// BackgroundTaskManager coordinates all background data fetch tasks.
type BackgroundTaskManager struct {
	gateway        *Gateway
	registry       map[string]*ScheduledTask
	mu             sync.RWMutex
	wg             sync.WaitGroup
	cancel         context.CancelFunc
	failureHandler TaskFailureHandler
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
	if task.ChannelID != "" && !m.gateway.HasChannel(task.ChannelID) {
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

// Stop gracefully shuts down all tasks.
func (m *BackgroundTaskManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *BackgroundTaskManager) runTask(ctx context.Context, task *ScheduledTask) {
	defer m.wg.Done()

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

	// Market hours guard — skip execution outside configured trading window
	if task.MarketHoursOnly && !isMarketOpen(task) {
		logging.Debug("background_task", "task_skipped_outside_market_hours", "name", task.Name)
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

	// Execute task with optional retry
	err := m.executeWithRetry(ctx, task)
	if err != nil {
		task.RecordFailure()
		logging.Error("background_task", "task_failed",
			"name", task.Name,
			"err", err.Error(),
			"consecutive_failures", task.Failures(),
		)
		if task.FailureHandler != nil {
			task.FailureHandler(task.Name, task.Failures(), err)
		}
		if m.failureHandler != nil {
			m.failureHandler(task.Name, task.Failures(), err)
		}
	} else {
		task.RecordSuccess()
	}
}

func (m *BackgroundTaskManager) executeWithRetry(ctx context.Context, task *ScheduledTask) error {
	maxAttempts := 1
	rp := task.RetryPolicy
	if rp != nil && rp.MaxAttempts > 0 {
		maxAttempts = rp.MaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := task.Task(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt >= maxAttempts {
			break
		}

		// Exponential backoff: InitialDelay * Multiplier^(attempt-1), capped at MaxDelay
		delay := rp.InitialDelay
		for i := 1; i < attempt; i++ {
			delay = time.Duration(float64(delay) * rp.Multiplier)
		}
		if delay > rp.MaxDelay {
			delay = rp.MaxDelay
		}

		logging.Warn("background_task", "task_retrying",
			"name", task.Name,
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"delay", delay.String(),
			"err", err.Error(),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// TaskStatus represents the runtime status of a task.
type TaskStatus struct {
	Name                string        `json:"name"`
	ChannelID           string        `json:"channel_id"`
	Enabled             bool          `json:"enabled"`
	Interval            time.Duration `json:"interval"`
	LastRun             time.Time     `json:"last_run"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	RetryPolicy         *RetryPolicy  `json:"retry_policy,omitempty"`
	MarketHoursOnly     bool          `json:"market_hours_only"`
	MarketOpenTime      string        `json:"market_open_time,omitempty"`
	MarketCloseTime     string        `json:"market_close_time,omitempty"`
}

// Status returns runtime status for all tasks.
func (m *BackgroundTaskManager) Status() []TaskStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TaskStatus, 0, len(m.registry))
	for _, t := range m.registry {
		status := TaskStatus{
			Name:                t.Name,
			ChannelID:           t.ChannelID,
			Enabled:             t.IsEnabled(),
			Interval:            t.Interval,
			LastRun:             t.LastRun(),
			ConsecutiveFailures: t.Failures(),
			MarketHoursOnly:     t.MarketHoursOnly,
			MarketOpenTime:      t.MarketOpenTime,
			MarketCloseTime:     t.MarketCloseTime,
		}
		if t.RetryPolicy != nil {
			rp := *t.RetryPolicy
			status.RetryPolicy = &rp
		}
		result = append(result, status)
	}
	return result
}
