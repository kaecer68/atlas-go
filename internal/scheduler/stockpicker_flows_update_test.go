// Tests for the daily per-symbol flow store refresh (issue #1945).
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/stockflows"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// flowsRunner records the Config it received and returns the canned result.
func flowsRunner(want *stockflows.Config, res *stockflows.Result, err error) func(context.Context, stockflows.Config) (stockflows.Result, error) {
	return func(_ context.Context, got stockflows.Config) (stockflows.Result, error) {
		*want = got
		if err != nil {
			return stockflows.Result{}, err
		}
		return *res, nil
	}
}

// writeStoredFlows seeds a per-symbol flow file ending at date.
func writeStoredFlows(t *testing.T, workDir, symbol, date string) {
	t.Helper()
	dir := stockflows.Dir(workDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir flows dir: %v", err)
	}
	data, err := json.Marshal(stockpicker.FlowFile{
		Symbol: symbol,
		Flows:  []stockpicker.FlowPoint{{Date: date, ForeignNet: 1000}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, symbol+".json"), data, 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}
}

func TestStockpickerFlowsTask_Before18Skips(t *testing.T) {
	deps := StockpickerFlowsUpdateDeps{WorkDir: "/tmp/x", Now: fixedNow("2026-03-16 17:59"), TimeZone: tzAsia}
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (before 18:00)", err)
	}
}

func TestStockpickerFlowsTask_NonTradingDaySkips(t *testing.T) {
	deps := StockpickerFlowsUpdateDeps{WorkDir: "/tmp/x", Now: fixedNow("2026-03-15 18:30"), TimeZone: tzAsia} // Sunday
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (non-trading day)", err)
	}
}

func TestStockpickerFlowsTask_EmptyWorkDirErrors(t *testing.T) {
	deps := StockpickerFlowsUpdateDeps{Now: fixedNow("2026-03-16 18:30"), TimeZone: tzAsia}
	err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background())
	if err == nil || err.Error() != "stockpicker flows update: WorkDir is empty" {
		t.Fatalf("err = %v, want WorkDir empty error", err)
	}
}

// TestStockpickerFlowsTask_EmptyStoreUsesLookback: a fresh deployment fetches
// the recent window instead of a decade of history.
func TestStockpickerFlowsTask_EmptyStoreUsesLookback(t *testing.T) {
	workDir := t.TempDir()
	var got stockflows.Config
	deps := StockpickerFlowsUpdateDeps{
		WorkDir:  workDir,
		Now:      fixedNow("2026-03-16 18:30"),
		TimeZone: tzAsia,
		Runner:   flowsRunner(&got, &stockflows.Result{Days: []stockflows.DayResult{{Date: "2026-03-16", Status: stockflows.StatusOK}}}, nil),
	}
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.WorkDir != workDir {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, workDir)
	}
	if s, e := got.Start.Format("2006-01-02"), got.End.Format("2006-01-02"); s != "2026-03-09" || e != "2026-03-16" {
		t.Errorf("window = %s..%s, want 2026-03-09..2026-03-16 (7-day lookback)", s, e)
	}
}

// TestStockpickerFlowsTask_IncrementalWindow: the store's newest date drives
// the window, so a gap left by a failed run heals on the next tick.
func TestStockpickerFlowsTask_IncrementalWindow(t *testing.T) {
	workDir := t.TempDir()
	writeStoredFlows(t, workDir, "2330", "2026-03-13") // Friday

	var got stockflows.Config
	deps := StockpickerFlowsUpdateDeps{
		WorkDir:  workDir,
		Now:      fixedNow("2026-03-16 18:30"), // Monday
		TimeZone: tzAsia,
		Runner:   flowsRunner(&got, &stockflows.Result{}, nil),
	}
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if s, e := got.Start.Format("2006-01-02"), got.End.Format("2006-01-02"); s != "2026-03-14" || e != "2026-03-16" {
		t.Errorf("window = %s..%s, want 2026-03-14..2026-03-16", s, e)
	}
}

// TestStockpickerFlowsTask_UpToDateSkips: when the store already reaches
// today, the tick is a no-op and the runner is never called.
func TestStockpickerFlowsTask_UpToDateSkips(t *testing.T) {
	workDir := t.TempDir()
	writeStoredFlows(t, workDir, "2330", "2026-03-16")

	called := false
	deps := StockpickerFlowsUpdateDeps{
		WorkDir:  workDir,
		Now:      fixedNow("2026-03-16 18:30"),
		TimeZone: tzAsia,
		Runner: func(context.Context, stockflows.Config) (stockflows.Result, error) {
			called = true
			return stockflows.Result{}, nil
		},
	}
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (store already covers today)", err)
	}
	if called {
		t.Fatal("runner was called for an up-to-date store")
	}
}

// TestStockpickerFlowsTask_ClampsLongGap: a store frozen for months must not
// turn one tick into hundreds of TWSE requests.
func TestStockpickerFlowsTask_ClampsLongGap(t *testing.T) {
	workDir := t.TempDir()
	writeStoredFlows(t, workDir, "2330", "2026-08-27") // the production freeze (issue #1945)

	var got stockflows.Config
	deps := StockpickerFlowsUpdateDeps{
		WorkDir:     workDir,
		Now:         fixedNow("2026-09-24 18:30"),
		TimeZone:    tzAsia,
		MaxSpanDays: 14,
		Runner:      flowsRunner(&got, &stockflows.Result{}, nil),
	}
	if err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if s, e := got.Start.Format("2006-01-02"), got.End.Format("2006-01-02"); s != "2026-09-10" || e != "2026-09-24" {
		t.Errorf("window = %s..%s, want 2026-09-10..2026-09-24 (clamped to 14 days)", s, e)
	}
}

// TestStockpickerFlowsTask_RunnerErrorPropagates: a fetch/write failure must
// reach the BackgroundTaskManager so the next tick retries.
func TestStockpickerFlowsTask_RunnerErrorPropagates(t *testing.T) {
	workDir := t.TempDir()
	writeStoredFlows(t, workDir, "2330", "2026-03-13")

	deps := StockpickerFlowsUpdateDeps{
		WorkDir:  workDir,
		Now:      fixedNow("2026-03-16 18:30"),
		TimeZone: tzAsia,
		Runner:   flowsRunner(new(stockflows.Config), nil, errors.New("twse: 500")),
	}
	err := StockpickerFlowsUpdateTaskFunc(deps)(context.Background())
	if err == nil {
		t.Fatal("err = nil, want the runner error propagated")
	}
	if want := "stockpicker flows update: twse: 500"; err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

// TestRegisterStockpickerFlowsUpdateSchedule pins the BTM registration
// contract (the task must be registered, time-gated and enabled, otherwise
// the store is unmanaged again — the root cause of issue #1945).
func TestRegisterStockpickerFlowsUpdateSchedule(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	RegisterStockpickerFlowsUpdateSchedule(btm, StockpickerFlowsUpdateDeps{WorkDir: "/tmp/x"})
	task, ok := btm.Get(StockpickerFlowsUpdateTaskName)
	if !ok {
		t.Fatalf("%s not registered", StockpickerFlowsUpdateTaskName)
	}
	if !task.Enabled || !task.TimeGated || task.Interval != time.Hour {
		t.Fatalf("task = %+v, want Enabled + TimeGated + 1h interval", task)
	}
	if task.Task == nil {
		t.Fatal("task.Task is nil")
	}
	// Nil manager is a no-op (fire-and-register convention).
	RegisterStockpickerFlowsUpdateSchedule(nil, StockpickerFlowsUpdateDeps{WorkDir: "/tmp/x"})
}
