package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// tzAsia is a fixed Asia/Taipei location for hermetic time-gate tests.
var tzAsia = func() *time.Location {
	l, _ := time.LoadLocation("Asia/Taipei")
	return l
}()

// fixedNow returns a nowFn pinned to a UTC time that renders to the given
// Asia/Taipei wall-clock date/time.
func fixedNow(taipei string) func() time.Time {
	loc, _ := time.LoadLocation("Asia/Taipei")
	tt, err := time.ParseInLocation("2006-01-02 15:04", taipei, loc)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return tt }
}

// fakeRunner records the RunDailyOptions it received and returns the canned
// result; used to assert gating and option plumbing without real I/O.
func fakeRunner(want *stockpicker.RunDailyOptions, res *stockpicker.RunDailyResult) func(context.Context, stockpicker.RunDailyOptions) (stockpicker.RunDailyResult, error) {
	return func(_ context.Context, got stockpicker.RunDailyOptions) (stockpicker.RunDailyResult, error) {
		*want = got
		return *res, nil
	}
}

func TestStockpickerTask_Before18Skips(t *testing.T) {
	deps := StockpickerUpdateDeps{
		WorkDir:  "/tmp/x",
		Now:      fixedNow("2026-03-16 17:59"), // before 18:00
		TimeZone: tzAsia,
	}
	err := StockpickerUpdateTaskFunc(deps)(context.Background())
	if err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (before 18:00)", err)
	}
}

func TestStockpickerTask_NonTradingDaySkips(t *testing.T) {
	// 2026-03-15 is a Sunday in Asia/Taipei.
	deps := StockpickerUpdateDeps{
		WorkDir:  "/tmp/x",
		Now:      fixedNow("2026-03-15 18:30"),
		TimeZone: tzAsia,
	}
	err := StockpickerUpdateTaskFunc(deps)(context.Background())
	if err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (non-trading day)", err)
	}
}

func TestStockpickerTask_EmptyWorkDirErrors(t *testing.T) {
	deps := StockpickerUpdateDeps{
		WorkDir:  "",
		Now:      fixedNow("2026-03-16 18:30"),
		TimeZone: tzAsia,
	}
	err := StockpickerUpdateTaskFunc(deps)(context.Background())
	if err == nil || err.Error() != "stockpicker update: WorkDir is empty" {
		t.Fatalf("err = %v, want WorkDir empty error", err)
	}
}

func TestStockpickerTask_RunsAfter18OnTradingDay(t *testing.T) {
	var got stockpicker.RunDailyOptions
	canned := stockpicker.RunDailyResult{AsOf: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)}
	deps := StockpickerUpdateDeps{
		WorkDir:  "/tmp/x",
		Now:      fixedNow("2026-03-16 18:30"), // trading day (Mon), after 18:00
		TimeZone: tzAsia,
		Runner:   fakeRunner(&got, &canned),
	}
	err := StockpickerUpdateTaskFunc(deps)(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.WorkDir != "/tmp/x" {
		t.Errorf("WorkDir = %q, want /tmp/x", got.WorkDir)
	}
	if got.Idempotency != stockpicker.IdempotencyDay {
		t.Errorf("Idempotency = %v, want IdempotencyDay", got.Idempotency)
	}
}

func TestStockpickerTask_SkipDayReturnsErrTaskSkipped(t *testing.T) {
	var got stockpicker.RunDailyOptions
	skipped := stockpicker.RunDailyResult{Skipped: true}
	deps := StockpickerUpdateDeps{
		WorkDir:  "/tmp/x",
		Now:      fixedNow("2026-03-16 18:30"),
		TimeZone: tzAsia,
		Runner:   fakeRunner(&got, &skipped),
	}
	err := StockpickerUpdateTaskFunc(deps)(context.Background())
	if err != apigateway.ErrTaskSkipped {
		t.Fatalf("err = %v, want ErrTaskSkipped (day already recorded)", err)
	}
}
