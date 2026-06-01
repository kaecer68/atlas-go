package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestBackgroundCalibrationScheduler_BurnInSkipsAll(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	called := false
	sched.Register(&CalibrationTask{
		Name:        "dummy",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			called = true
			return nil
		},
	})

	if err := sched.RunDaily(context.Background()); err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if called {
		t.Error("expected task to be skipped during burn_in")
	}
}

func TestBackgroundCalibrationScheduler_FullAutoRunsAll(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	var ran int
	sched.Register(&CalibrationTask{
		Name:        "task_a",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			ran++
			return nil
		},
	})
	sched.Register(&CalibrationTask{
		Name:        "task_b",
		MinMaturity: domain.MaturityFullAuto,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			ran++
			return nil
		},
	})

	if err := sched.RunDaily(context.Background()); err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if ran != 2 {
		t.Errorf("expected 2 tasks to run, got %d", ran)
	}
}

func TestBackgroundCalibrationScheduler_CalibratingSkipsFullAuto(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-90 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	var ran int
	sched.Register(&CalibrationTask{
		Name:        "calibrating_task",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			ran++
			return nil
		},
	})
	sched.Register(&CalibrationTask{
		Name:        "full_auto_task",
		MinMaturity: domain.MaturityFullAuto,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			ran++
			return nil
		},
	})

	if err := sched.RunDaily(context.Background()); err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if ran != 1 {
		t.Errorf("expected 1 task to run (calibrating only), got %d", ran)
	}
}

func TestBackgroundCalibrationScheduler_IntervalRespected(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	var ran int
	sched.Register(&CalibrationTask{
		Name:        "infrequent",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    24 * time.Hour,
		LastRun:     time.Now().Add(-1 * time.Hour),
		Run: func(_ context.Context) error {
			ran++
			return nil
		},
	})

	if err := sched.RunDaily(context.Background()); err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if ran != 0 {
		t.Error("expected task to be skipped because interval not yet elapsed")
	}
}

func TestBackgroundCalibrationScheduler_TaskFailureLogged(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	sched.Register(&CalibrationTask{
		Name:        "failing",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    time.Second,
		Run: func(_ context.Context) error {
			return errors.New("simulated failure")
		},
	})

	if err := sched.RunDaily(context.Background()); err != nil {
		t.Fatalf("RunDaily should not propagate task errors: %v", err)
	}
}

func TestBackgroundCalibrationScheduler_Status(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	sched := NewBackgroundCalibrationScheduler(tr)

	sched.Register(&CalibrationTask{
		Name:        "task_1",
		MinMaturity: domain.MaturityCalibrating,
		Interval:    24 * time.Hour,
		Run:         func(_ context.Context) error { return nil },
	})

	status := sched.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 status entry, got %d", len(status))
	}
	if status[0]["name"] != "task_1" {
		t.Errorf("expected name=task_1, got %v", status[0]["name"])
	}
}
