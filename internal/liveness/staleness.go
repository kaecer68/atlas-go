package liveness

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// StaleFactor is the "overdue" multiplier: a task is stale when it has not
// run for more than interval x StaleFactor.
const StaleFactor = 3

// GatedStaleWindow is the wall-clock absence of a real *successful* run after
// which a time-gated task (ScheduledTask.TimeGated) is considered stale.
// Gated tasks run on a narrow daily window (e.g. daily_report_generate at
// 14:00 Taipei, autobacktest_daily inside its market-close window), so up to
// ~24h can elapse between real runs by design. 72h (3 days) gives a
// comfortable margin against false positives while still flagging a genuinely
// dead gated task. A gated task whose run keeps failing is caught by the BTM
// consecutive-failure alert instead.
const GatedStaleWindow = 72 * time.Hour

// IsStale reports whether a task is overdue: now - lastRun > interval x 3.
// A zero lastRun (never ran) is stale by definition; a zero interval (no
// cadence information) is never stale.
//
// This is the criterion for regular (non-gated) tasks whose tick interval is
// their effective cadence. Time-gated tasks must use gatedIsStale instead.
func IsStale(lastRun time.Time, interval time.Duration, now time.Time) bool {
	if lastRun.IsZero() {
		return true
	}
	if interval <= 0 {
		return false
	}
	return now.Sub(lastRun) > interval*StaleFactor
}

// gatedIsStale reports whether a time-gated task is overdue. Unlike a regular
// task, staleness is judged on the last real *success* (the gate's actual
// work) against the generous GatedStaleWindow, not on the short tick interval
// that would false-alarm during the idle window between real runs. A gated
// task that has never succeeded is not flagged here — a failing gated task is
// caught by the BTM consecutive-failure alert.
// GatedIsStale is the exported form used by API consumers (task-liveness
// handler) that must apply gate-window staleness to time-gated tasks.
func GatedIsStale(lastSuccess time.Time, now time.Time) bool {
	return gatedIsStale(lastSuccess, now)
}

func gatedIsStale(lastSuccess time.Time, now time.Time) bool {
	if lastSuccess.IsZero() {
		return false
	}
	return now.Sub(lastSuccess) > GatedStaleWindow
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
		// Time-gated tasks (ScheduledTask.TimeGated) return ErrTaskSkipped
		// outside their run window, so their tick interval is not their
		// effective cadence. Judge them by last successful run against the
		// generous GatedStaleWindow; this removes the false-positive "stale"
		// alert for e.g. daily_report_generate / autobacktest_daily while
		// they idle between daily windows.
		if task.TimeGated {
			if !gatedIsStale(r.LastSuccessAt, now) {
				continue
			}
			nowStale[r.TaskName] = true
			m.alertIfNew(r.TaskName, now.Sub(r.LastSuccessAt), GatedStaleWindow)
			continue
		}
		interval := task.Interval
		if interval <= 0 {
			continue
		}
		if !IsStale(r.LastRunAt, interval, now) {
			continue
		}
		nowStale[r.TaskName] = true
		m.alertIfNew(r.TaskName, now.Sub(r.LastRunAt), interval)
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

// alertIfNew fires a staleness alert for the task if it is not already
// alerted, and marks it as alerted. Deduplication keeps one alert per stale
// episode; the alert state is cleared by CheckOnce once the task recovers.
func (m *StalenessMonitor) alertIfNew(taskName string, staleFor, interval time.Duration) {
	m.mu.Lock()
	alreadyAlerted := m.alerted[taskName]
	m.mu.Unlock()
	if alreadyAlerted || m.alertFn == nil {
		return
	}
	m.alertFn(taskName, staleFor, interval)
	m.mu.Lock()
	m.alerted[taskName] = true
	m.mu.Unlock()
}

// AlertedForTesting returns the set of currently-alerted task names.
func (m *StalenessMonitor) AlertedForTesting() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.alerted))
	maps.Copy(out, m.alerted)
	return out
}

var _ IntervalProvider = (*apigateway.BackgroundTaskManager)(nil)
