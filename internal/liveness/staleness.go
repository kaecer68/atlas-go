package liveness

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// StaleFactor is the "overdue" multiplier: a task is stale when it has not
// run for more than interval x StaleFactor.
const StaleFactor = 3

// IsStale reports whether a task is overdue: now - lastRun > interval x 3.
// A zero lastRun (never ran) is stale by definition; a zero interval (no
// cadence information) is never stale.
func IsStale(lastRun time.Time, interval time.Duration, now time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	if interval <= 0 {
		return false
	}
	return now.Sub(lastRun) > interval*StaleFactor
}

// IntervalProvider resolves a task name to its runtime scheduling interval.
// *apigateway.BackgroundTaskManager satisfies this via Get.
type IntervalProvider interface {
	Get(name string) (*apigateway.ScheduledTask, bool)
}

// StalenessAlertFn is the alert sink; wired to monitor.Alert by main.
type StalenessAlertFn func(taskName string, staleFor time.Duration, interval time.Duration)

// lister is the read surface StalenessMonitor needs from the store.
type lister interface {
	List(ctx context.Context) ([]Row, error)
}

// StalenessMonitor periodically scans task_liveness and alerts when a task
// has not run for > interval x 3. Alerts are deduplicated per task: one
// alert per stale episode, cleared when the task runs again. The same
// monitor.Alert channel as the BTM failure handler is used (wired by main).
type StalenessMonitor struct {
	store      lister
	intervals  IntervalProvider
	alertFn    StalenessAlertFn
	checkEvery time.Duration

	mu      sync.Mutex
	alerted map[string]bool
}

// NewStalenessMonitor creates a monitor. checkEvery <= 0 defaults to 5m.
func NewStalenessMonitor(store *Store, intervals IntervalProvider, alertFn StalenessAlertFn) *StalenessMonitor {
	return &StalenessMonitor{
		store:      store,
		intervals:  intervals,
		alertFn:    alertFn,
		checkEvery: 5 * time.Minute,
		alerted:    make(map[string]bool),
	}
}

// Run loops until ctx is cancelled, calling CheckOnce every checkEvery.
func (m *StalenessMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.checkEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.CheckOnce(ctx, time.Now()); err != nil {
				logging.Warn("liveness", "staleness_check_failed", "err", err.Error())
			}
		}
	}
}

// CheckOnce performs one staleness scan and fires alerts for newly-stale
// tasks. Tasks whose interval is unknown (cron-only rows) are skipped.
func (m *StalenessMonitor) CheckOnce(ctx context.Context, now time.Time) error {
	if m.store == nil {
		return nil
	}
	rows, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	nowStale := make(map[string]bool, len(rows))
	for _, r := range rows {
		task, ok := m.intervals.Get(r.TaskName)
		if !ok {
			continue // cron-only row: no BTM interval, cannot judge staleness
		}
		interval := task.Interval
		if interval <= 0 {
			continue
		}
		if !IsStale(r.LastRunAt, interval, now) {
			continue
		}
		nowStale[r.TaskName] = true
		staleFor := now.Sub(r.LastRunAt)
		m.mu.Lock()
		alreadyAlerted := m.alerted[r.TaskName]
		m.mu.Unlock()
		if !alreadyAlerted && m.alertFn != nil {
			m.alertFn(r.TaskName, staleFor, interval)
			m.mu.Lock()
			m.alerted[r.TaskName] = true
			m.mu.Unlock()
		}
	}
	// Clear alert state for tasks that recovered (ran again or interval gone).
	m.mu.Lock()
	for name := range m.alerted {
		if !nowStale[name] {
			delete(m.alerted, name)
		}
	}
	m.mu.Unlock()
	return nil
}

// AlertedForTesting returns the set of currently-alerted task names.
func (m *StalenessMonitor) AlertedForTesting() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.alerted))
	for k, v := range m.alerted {
		out[k] = v
	}
	return out
}

var _ IntervalProvider = (*apigateway.BackgroundTaskManager)(nil)
