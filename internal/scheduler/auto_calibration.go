package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalibrationTask is a single background calibration job.
type CalibrationTask struct {
	Name         string
	MinMaturity  domain.SystemMaturity // minimum maturity to run
	Interval     time.Duration         // how often to run
	LastRun      time.Time
	Run          func(ctx context.Context) error
}

// BackgroundCalibrationScheduler coordinates periodic calibration tasks
// (risk thresholds, industry cycle, factor weights, etc.) based on the
// current system maturity phase.
type BackgroundCalibrationScheduler struct {
	tracker *domain.MaturityTracker
	tasks   []*CalibrationTask
	mu      sync.RWMutex
}

// NewBackgroundCalibrationScheduler creates a scheduler wired to a maturity tracker.
func NewBackgroundCalibrationScheduler(tracker *domain.MaturityTracker) *BackgroundCalibrationScheduler {
	return &BackgroundCalibrationScheduler{
		tracker: tracker,
		tasks:   make([]*CalibrationTask, 0),
	}
}

// Register adds a calibration task. Tasks are executed in registration order.
func (s *BackgroundCalibrationScheduler) Register(task *CalibrationTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
}

// RunDaily is the entry point intended for apigateway.BackgroundTaskManager.
// It checks maturity and dispatches eligible tasks.
func (s *BackgroundCalibrationScheduler) RunDaily(ctx context.Context) error {
	if s.tracker == nil {
		return fmt.Errorf("calibration scheduler: maturity tracker is nil")
	}

	maturity := s.tracker.Current()
	daysSince := s.tracker.DaysSinceStart()

	// BURN_IN: log status, skip all calibration
	if maturity == domain.MaturityBurnIn {
		logging.Info("calibration_scheduler", "burn_in_skip",
			"days_since_start", daysSince,
			"days_until_calibrating", s.tracker.DaysUntil(domain.MaturityCalibrating))
		return nil
	}

	s.mu.RLock()
	tasks := make([]*CalibrationTask, len(s.tasks))
	copy(tasks, s.tasks)
	s.mu.RUnlock()

	now := time.Now()
	var ran, skipped int
	for _, task := range tasks {
		if !domain.MaturityGated(maturity, task.MinMaturity) {
			logging.Info("calibration_scheduler", "task_skipped_maturity",
				"task", task.Name,
				"current_maturity", string(maturity),
				"required_maturity", string(task.MinMaturity))
			skipped++
			continue
		}
		if now.Sub(task.LastRun) < task.Interval {
			// Not due yet
			continue
		}

		logging.Info("calibration_scheduler", "task_start",
			"task", task.Name,
			"maturity", string(maturity))
		if err := task.Run(ctx); err != nil {
			logging.Error("calibration_scheduler", "task_failed",
				"task", task.Name,
				"err", err)
		} else {
			task.LastRun = now
			ran++
			logging.Info("calibration_scheduler", "task_ok",
				"task", task.Name)
		}
	}

	logging.Info("calibration_scheduler", "daily_complete",
		"maturity", string(maturity),
		"days_since_start", daysSince,
		"tasks_ran", ran,
		"tasks_skipped", skipped,
		"total_tasks", len(tasks))
	return nil
}

// Status returns a snapshot of all registered tasks for monitoring.
func (s *BackgroundCalibrationScheduler) Status() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]map[string]any, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, map[string]any{
			"name":              t.Name,
			"min_maturity":      string(t.MinMaturity),
			"interval_hours":    t.Interval.Hours(),
			"last_run":          t.LastRun,
			"seconds_since_run": time.Since(t.LastRun).Seconds(),
		})
	}
	return result
}
