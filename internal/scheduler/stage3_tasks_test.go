package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// fixedTimeZone provides a deterministic timezone for tests.
func fixedTimeZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	return loc
}

func TestDailyOnceGuard_TriggersOncePerDay(t *testing.T) {
	loc := fixedTimeZone(t)
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, loc)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return now }

	guard := dailyOnceGuard(loc, 6, 0)
	if !guard() {
		t.Fatalf("expected guard to fire at first 06:00")
	}
	if guard() {
		t.Fatalf("expected guard to suppress duplicate 06:00")
	}

	now = now.Add(time.Minute)
	if guard() {
		t.Fatalf("expected guard to suppress 06:01")
	}

	now = time.Date(2026, 7, 14, 6, 0, 0, 0, loc)
	if !guard() {
		t.Fatalf("expected guard to fire on next day 06:00")
	}
}

func TestWeeklyOnceGuard_TriggersOnlyOnMonday(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()

	now := time.Date(2026, 7, 13, 8, 0, 0, 0, loc) // Monday
	timeNow = func() time.Time { return now }
	guard := weeklyOnceGuard(loc, time.Monday, 8, 0)
	if !guard() {
		t.Fatalf("expected guard to fire on Monday 08:00")
	}
	if guard() {
		t.Fatalf("expected guard to suppress duplicate Monday 08:00")
	}

	now = now.Add(24 * time.Hour) // Tuesday
	if guard() {
		t.Fatalf("expected guard to suppress Tuesday 08:00")
	}

	now = time.Date(2026, 7, 20, 8, 0, 0, 0, loc) // next Monday
	if !guard() {
		t.Fatalf("expected guard to fire on next Monday 08:00")
	}
}

func TestMonthlyOnceGuard_TriggersOnlyOnFirstOfMonth(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()

	now := time.Date(2026, 7, 1, 8, 0, 0, 0, loc)
	timeNow = func() time.Time { return now }
	guard := monthlyOnceGuard(loc, 1, 8, 0)
	if !guard() {
		t.Fatalf("expected guard to fire on 1st 08:00")
	}
	if guard() {
		t.Fatalf("expected guard to suppress duplicate 1st 08:00")
	}

	now = now.Add(24 * time.Hour) // 2nd
	if guard() {
		t.Fatalf("expected guard to suppress 2nd 08:00")
	}

	now = time.Date(2026, 8, 1, 8, 0, 0, 0, loc)
	if !guard() {
		t.Fatalf("expected guard to fire on next month 1st 08:00")
	}
}

func TestRunWithRetryAndAudit_SucceedsOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	work := func() error {
		calls.Add(1)
		return nil
	}

	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", calls.Load())
	}
}

func TestRunWithRetryAndAudit_RetriesUntilSuccess(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	var calls atomic.Int32
	work := func() error {
		if calls.Add(1) < 3 {
			return errors.New("transient failure")
		}
		return nil
	}

	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRunWithRetryAndAudit_FailsAfterMaxRetries(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	var calls atomic.Int32
	work := func() error {
		calls.Add(1)
		return errors.New("persistent failure")
	}

	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err == nil {
		t.Fatalf("expected error after max retries")
	}
	if calls.Load() != maxStage3Retries+1 {
		t.Fatalf("expected %d calls, got %d", maxStage3Retries+1, calls.Load())
	}
}

func TestRunWithRetryAndAudit_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	work := func() error { return errors.New("should not run") }
	err := runWithRetryAndAudit(ctx, "test-task", work)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestRunWithRetryAndAudit_OneRetryThenSuccess covers the 1-retry success path: attempt 0 fails, attempt 1 succeeds.
func TestRunWithRetryAndAudit_OneRetryThenSuccess(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	var calls atomic.Int32
	work := func() error {
		if calls.Add(1) == 1 {
			return errors.New("transient failure")
		}
		return nil
	}
	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls (1 initial + 1 retry), got %d", calls.Load())
	}
}

// TestRunWithRetryAndAudit_TwoRetriesThenSuccess covers the 2-retry success path boundary (vs _RetriesUntilSuccess which is 3).
func TestRunWithRetryAndAudit_TwoRetriesThenSuccess(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	var calls atomic.Int32
	work := func() error {
		if calls.Add(1) < 3 {
			return errors.New("transient failure")
		}
		return nil
	}
	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls (1 initial + 2 retries), got %d", calls.Load())
	}
}

// TestRunWithRetryAndAudit_ThreeRetriesThenSuccess covers the maxStage3Retries upper-bound success path; dual to _FailsAfterMaxRetries.
func TestRunWithRetryAndAudit_ThreeRetriesThenSuccess(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	var calls atomic.Int32
	work := func() error {
		if calls.Add(1) < 4 {
			return errors.New("transient failure")
		}
		return nil
	}
	err := runWithRetryAndAudit(context.Background(), "test-task", work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("expected 4 calls (1 initial + 3 retries), got %d", calls.Load())
	}
}

func TestSyncEventsDailyTaskFunc_SkipsOutsideWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 10, 0, 0, 0, loc) }

	var calls atomic.Int32
	deps := Stage3TaskDeps{
		TimeZone: loc,
		RefreshEventCalendar: func(now time.Time) error {
			calls.Add(1)
			return nil
		},
	}

	task := SyncEventsDailyTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected no calls outside 06:00, got %d", calls.Load())
	}
}

func TestSyncEventsDailyTaskFunc_ExecutesAtWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 6, 0, 0, 0, loc) }

	var calls atomic.Int32
	deps := Stage3TaskDeps{
		TimeZone: loc,
		RefreshEventCalendar: func(now time.Time) error {
			calls.Add(1)
			return nil
		},
	}

	task := SyncEventsDailyTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 event-calendar refreshes (today and tomorrow), got %d", calls.Load())
	}
}

func TestSyncEventsDailyTaskFunc_RetriesAndReturnsError(t *testing.T) {
	oldDelays := stage3RetryDelays
	stage3RetryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	defer func() { stage3RetryDelays = oldDelays }()

	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 6, 0, 0, 0, loc) }

	deps := Stage3TaskDeps{
		TimeZone: loc,
		RefreshEventCalendar: func(now time.Time) error {
			return errors.New("refresh failed")
		},
	}

	task := SyncEventsDailyTaskFunc(deps)
	if err := task(context.Background()); err == nil {
		t.Fatalf("expected error from failing refresh")
	}
}

func TestSyncCapitalDailyTaskFunc_ExecutesAtWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 13, 30, 0, 0, loc) }

	var calls atomic.Int32
	deps := Stage3TaskDeps{
		TimeZone: loc,
		RefreshCapitalFlow: func(ctx context.Context) error {
			calls.Add(1)
			return nil
		},
	}

	task := SyncCapitalDailyTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 capital-flow refresh, got %d", calls.Load())
	}
}

func TestSyncRegimeWeeklyTaskFunc_ExecutesAtWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 8, 0, 0, 0, loc) } // Monday

	var calls atomic.Int32
	deps := Stage3TaskDeps{
		TimeZone: loc,
		UpdateRegimeHistory: func(ctx context.Context, lookbackDays int) error {
			if lookbackDays != 90 {
				t.Fatalf("expected lookbackDays=90, got %d", lookbackDays)
			}
			calls.Add(1)
			return nil
		},
	}

	task := SyncRegimeWeeklyTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 regime-history update, got %d", calls.Load())
	}
}

func TestRecalibrateTemplatesMonthlyTaskFunc_ExecutesAtWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 1, 8, 0, 0, 0, loc) }

	var calls atomic.Int32
	deps := Stage3TaskDeps{
		TimeZone: loc,
		RecalculateTemplateHitRates: func() error {
			calls.Add(1)
			return nil
		},
	}

	task := RecalibrateTemplatesMonthlyTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 hit-rate recalculation, got %d", calls.Load())
	}
}

func TestReconcilePrevDayPredictionTaskFunc_ReconcilesWhenBothSidesAvailable(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 14, 30, 0, 0, loc) }

	var updates atomic.Int32
	predAt := time.Date(2026, 7, 10, 5, 45, 0, 0, time.UTC)
	deps := Stage3TaskDeps{
		TimeZone: loc,
		LoadPrevDayPrediction: func() (ledger.EventFlowPredictionRecord, bool) {
			return ledger.EventFlowPredictionRecord{PredictedAt: predAt, DirectionSign: 0.5, Direction: "inflow"}, true
		},
		LoadPrevDayActual: func() (float64, bool) {
			return -0.3, true // outflow realized
		},
		UpdatePrevDayActual: func(predictedAt time.Time, actualSign float64, source string) error {
			if !predictedAt.Equal(predAt) {
				t.Fatalf("expected predictedAt %v, got %v", predAt, predictedAt)
			}
			if actualSign != -0.3 {
				t.Fatalf("expected actualSign -0.3, got %v", actualSign)
			}
			if source != "twse_t86" {
				t.Fatalf("expected source twse_t86, got %q", source)
			}
			updates.Add(1)
			return nil
		},
	}

	task := ReconcilePrevDayPredictionTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updates.Load() != 1 {
		t.Fatalf("expected 1 UpdatePrevDayActual call, got %d", updates.Load())
	}
}

func TestReconcilePrevDayPredictionTaskFunc_SkipsWhenNoPrediction(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 14, 30, 0, 0, loc) }

	deps := Stage3TaskDeps{
		TimeZone: loc,
		LoadPrevDayPrediction: func() (ledger.EventFlowPredictionRecord, bool) {
			return ledger.EventFlowPredictionRecord{}, false
		},
		LoadPrevDayActual: func() (float64, bool) {
			t.Fatal("LoadPrevDayActual should not be called when no prediction exists")
			return 0, false
		},
		UpdatePrevDayActual: func(time.Time, float64, string) error {
			t.Fatal("UpdatePrevDayActual should not be called when no prediction exists")
			return nil
		},
	}

	task := ReconcilePrevDayPredictionTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("expected no error for missing prediction, got %v", err)
	}
}

func TestReconcilePrevDayPredictionTaskFunc_SkipsWhenActualUnavailable(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 14, 30, 0, 0, loc) }

	deps := Stage3TaskDeps{
		TimeZone: loc,
		LoadPrevDayPrediction: func() (ledger.EventFlowPredictionRecord, bool) {
			return ledger.EventFlowPredictionRecord{PredictedAt: time.Now()}, true
		},
		LoadPrevDayActual: func() (float64, bool) {
			return 0, false
		},
		UpdatePrevDayActual: func(time.Time, float64, string) error {
			t.Fatal("UpdatePrevDayActual should not be called when actual is unavailable")
			return nil
		},
	}

	task := ReconcilePrevDayPredictionTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("expected no error for unavailable actual, got %v", err)
	}
}

func TestReconcilePrevDayPredictionTaskFunc_DoesNotFireOutsideWindow(t *testing.T) {
	loc := fixedTimeZone(t)
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	timeNow = func() time.Time { return time.Date(2026, 7, 13, 13, 30, 0, 0, loc) } // wrong hour

	deps := Stage3TaskDeps{
		TimeZone: loc,
		LoadPrevDayPrediction: func() (ledger.EventFlowPredictionRecord, bool) {
			t.Fatal("LoadPrevDayPrediction should not fire outside 14:30 window")
			return ledger.EventFlowPredictionRecord{}, false
		},
	}

	task := ReconcilePrevDayPredictionTaskFunc(deps)
	if err := task(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
