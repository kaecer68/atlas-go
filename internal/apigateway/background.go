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
	Name      string
	ChannelID string
	Interval  time.Duration
	Jitter    time.Duration
	Task      BackgroundTaskFunc
	// TimeGated marks a task that returns ErrTaskSkipped for most ticks and
	// only does real work inside a narrow run window (e.g. daily_report_generate
	// at 14:00 Taipei, autobacktest_daily inside its market-close window). Its
	// tick Interval is NOT the effective cadence, so the liveness staleness
	// monitor must not judge it by interval x StaleFactor (that would false-
	// alarm during the long idle window between real runs). Gated tasks are
	// instead judged by time since last *successful* run against a generous
	// wall-clock window (see liveness.GatedStaleWindow).
	TimeGated           bool
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

// TaskCompletionHandler is called after every real task execution (success
// or failure; skipped ticks are excluded) with the outcome and wall-clock
// duration. Used by the task-liveness heartbeat (internal/liveness) to
// persist "did this task actually succeed" across restarts.
type TaskCompletionHandler func(taskName string, err error, duration time.Duration)

// BackgroundTaskManager coordinates all background data fetch tasks.
type BackgroundTaskManager struct {
	gateway         *Gateway
	registry        map[string]*ScheduledTask
	mu              sync.RWMutex
	wg              sync.WaitGroup
	cancel          context.CancelFunc
	failureHandler  TaskFailureHandler
	recoveryHandler TaskRecoveryHandler
	// completionHandler receives success AND failure outcomes (with duration)
	// so callers can persist cross-restart liveness. Nil is a no-op.
	completionHandler TaskCompletionHandler
	// staggerStartup spreads the first runs of a fresh process start across
	// a bounded window (see startupStaggerDelay) to avoid the startup
	// thundering herd against shared rate limiters (#1763). Default true.
	staggerStartup bool
}

// NewBackgroundTaskManager creates a task manager.
func NewBackgroundTaskManager(gateway *Gateway) *BackgroundTaskManager {
	// Note: staggerStartup intentionally defaults to false — the documented
	// first-run contract is "fresh start executes immediately" (see runTask
	// and TestBackgroundTaskManager_RunTask_AppliesStartupJitter). Production
	// wiring (cmd/atlas/background_tasks.go) opts into the startup stagger to
	// avoid the thundering herd (#1763).
	return &BackgroundTaskManager{
		gateway:  gateway,
		registry: make(map[string]*ScheduledTask),
	}
}

// WithStartupStagger overrides whether fresh starts stagger the first run
// (default true via NewBackgroundTaskManager). Unit tests that assert
// immediate first execution use false.
func (m *BackgroundTaskManager) WithStartupStagger(enabled bool) *BackgroundTaskManager {
	m.staggerStartup = enabled
	return m
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
// startupStaggerDelay returns the deterministic pre-first-run delay for a
// freshly started task (#1763). The delay is derived from a stable hash of
// the task name so the fan-out order is reproducible across restarts, and is
// capped at min(staggerWindow, interval/10) so short-interval probes spread
// over a bounded window while long-interval tasks are unaffected in
// relative terms.
func startupStaggerDelay(name string, interval time.Duration) time.Duration {
	// #1780: widened from 2min — 93 registered tasks compressed into 120s
	// still stamped PG (concurrent QueryAllOutcomes full scans → minutes of
	// endpoint 503s after every deploy). 10min spreads the heavy calibration
	// tasks enough that their full-table reads no longer overlap; short
	// interval probes remain protected by the interval/10 cap below.
	const staggerWindow = 10 * time.Minute
	if interval <= 0 {
		return 0
	}
	window := staggerWindow
	if cap := interval / 10; cap < window {
		window = cap
	}
	if window <= 0 {
		return 0
	}
	h := fnv32a(name)
	return time.Duration(h%uint32(window/time.Second)) * time.Second
}

// fnv32a is the 32-bit FNV-1a hash (dependency-free, stable across runs).
func fnv32a(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

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

// SetCompletionHandler sets a callback invoked after every task execution
// (success or failure). The callback receives the run outcome and duration;
// skipped ticks (ErrTaskSkipped) are NOT reported — they are no-ops by
// contract and must not move the liveness last_run marker.
func (m *BackgroundTaskManager) SetCompletionHandler(h TaskCompletionHandler) {
	m.completionHandler = h
}

// safeCallCompletionHandler invokes m.completionHandler inside a nested defer
// recover so a misbehaving handler cannot crash the task loop. A nil handler
// is a no-op.
func (m *BackgroundTaskManager) safeCallCompletionHandler(taskName string, err error, duration time.Duration) {
	if m.completionHandler == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			logging.Error(
				"background_task", "completionHandler_panic_recovered",
				"task_name", taskName,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(stack),
			)
		}
	}()
	m.completionHandler(taskName, err, duration)
}

func logAndWrapPanic(taskName, event string, r any) error {
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
	// Fresh process start (#1763): every task fires its first run
	// simultaneously and stampedes the shared TWSE rate limiter + upstreams —
	// fetches queue past their context deadlines and the channel health page
	// shows false alarms (bdi timeout, taiex fallback rate-limit, day_trading
	// "no data") for ~30 minutes after every deploy. Spread the first runs
	// deterministically across a bounded stagger window (name-hashed so the
	// order is stable across restarts); long-interval tasks lose at most
	// min(2min, interval/10) of idleness, a negligible cost vs hours.
	if m.staggerStartup && task.LastRun().IsZero() && task.Interval > 0 {
		if delay := startupStaggerDelay(task.Name, task.Interval); delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
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
	startedAt := time.Now()

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
		m.safeCallCompletionHandler(task.Name, err, time.Since(startedAt))
	} else {
		prev := task.Failures()
		task.RecordSuccess()
		if prev > 0 && m.recoveryHandler != nil {
			m.recoveryHandler(task.Name, prev)
		}
		m.safeCallCompletionHandler(task.Name, nil, time.Since(startedAt))
	}
}

// overlapTolerance returns how much earlier than the full interval a tick
// may fire before it is treated as an overlap: max(1s, interval/20).
func overlapTolerance(interval time.Duration) time.Duration {
	tol := max(interval/20, time.Second)
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

	// TimeGated marks tick-tasks that only do real work inside a time window
	// (e.g. post-market-close). Staleness for gated tasks must use the gate
	// window, not Interval×3 (#1776 audit: false "逾期" on the home dashboard
	// for daily_report_generate / autobacktest_daily / report_tracker_verify).
	TimeGated bool `json:"time_gated,omitempty"`

	// Data-health fields (#1265): separated from run-success.
	LastDataAsOf     time.Time `json:"last_data_as_of,omitzero"`
	LastNewSamples   int       `json:"last_new_samples"`
	LastPersistedAt  time.Time `json:"last_persisted_at,omitzero"`
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
			TimeGated:           t.TimeGated,
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
