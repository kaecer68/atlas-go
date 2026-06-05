package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestNewSeasonalCalibrationTask_DefaultInterval confirms that an interval
// <= 0 is replaced by the documented default (7 calendar days).
func TestNewSeasonalCalibrationTask_DefaultInterval(t *testing.T) {
	task := NewSeasonalCalibrationTask("/bin/true", 0, "")
	if task.Interval != SeasonalCalibrationDefaults.Interval {
		t.Errorf("Interval = %v, want default %v", task.Interval, SeasonalCalibrationDefaults.Interval)
	}
	if task.Interval != 7*24*time.Hour {
		t.Errorf("Interval = %v, want 7d", task.Interval)
	}
}

// TestNewSeasonalCalibrationTask_CustomInterval confirms a positive
// interval is preserved.
func TestNewSeasonalCalibrationTask_CustomInterval(t *testing.T) {
	task := NewSeasonalCalibrationTask("/bin/true", 24*time.Hour, "")
	if task.Interval != 24*time.Hour {
		t.Errorf("Interval = %v, want 24h", task.Interval)
	}
}

// TestNewSeasonalCalibrationTask_DefaultMaturity confirms an empty
// maturity is replaced by MaturityCalibrating.
func TestNewSeasonalCalibrationTask_DefaultMaturity(t *testing.T) {
	task := NewSeasonalCalibrationTask("/bin/true", time.Hour, "")
	if task.MinMaturity != domain.MaturityCalibrating {
		t.Errorf("MinMaturity = %q, want %q", task.MinMaturity, domain.MaturityCalibrating)
	}
}

// TestNewSeasonalCalibrationTask_TaskName asserts the task is registered
// under a stable name (consumed by BackgroundTaskManager lookups).
func TestNewSeasonalCalibrationTask_TaskName(t *testing.T) {
	task := NewSeasonalCalibrationTask("/bin/true", time.Hour, "")
	if task.Name != "seasonal_calibration" {
		t.Errorf("Name = %q, want %q", task.Name, "seasonal_calibration")
	}
}

// TestNewSeasonalCalibrationTask_EmptyPathDeferToRun asserts the factory
// does not panic on empty path; the failure surfaces in Run instead.
func TestNewSeasonalCalibrationTask_EmptyPathDeferToRun(t *testing.T) {
	task := NewSeasonalCalibrationTask("", time.Hour, "")
	if task == nil {
		t.Fatal("factory returned nil task")
	}
	if task.Run == nil {
		t.Fatal("factory returned task with nil Run")
	}
}

// TestRunSeasonalCalibration_EmptyPath returns an error so the
// BackgroundTaskManager can mark the run as failed.
func TestRunSeasonalCalibration_EmptyPath(t *testing.T) {
	err := runSeasonalCalibration(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty binary path")
	}
}

// TestRunSeasonalCalibration_SuccessPath uses /usr/bin/true as a
// deterministic zero-exit binary (path resolved for macOS/Linux).
// CombinedOutput is consumed; the call returns nil.
func TestRunSeasonalCalibration_SuccessPath(t *testing.T) {
	if err := runSeasonalCalibration(context.Background(), "/usr/bin/true"); err != nil {
		t.Errorf("runSeasonalCalibration with /usr/bin/true: %v", err)
	}
}

// TestRunSeasonalCalibration_NonZeroExit uses /usr/bin/false to verify
// the subprocess error is propagated to the caller.
func TestRunSeasonalCalibration_NonZeroExit(t *testing.T) {
	err := runSeasonalCalibration(context.Background(), "/usr/bin/false")
	if err == nil {
		t.Fatal("expected error from /usr/bin/false, got nil")
	}
}

// TestRunSeasonalCalibration_MissingBinary verifies the exec error path
// when the binary does not exist.
func TestRunSeasonalCalibration_MissingBinary(t *testing.T) {
	err := runSeasonalCalibration(context.Background(), "/nonexistent/calibrate-seasonal-binary-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if !errors.Is(err, err) {
		t.Logf("got error: %v", err)
	}
}

// TestSeasonalCalibrationTaskFunc_EmptyPath confirms the closure form
// also surfaces empty path as an error.
func TestSeasonalCalibrationTaskFunc_EmptyPath(t *testing.T) {
	fn := SeasonalCalibrationTaskFunc("")
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error for empty binary path")
	}
}
