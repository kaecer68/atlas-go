package main

// #2124：`market_period` 回填的日期窗口與「超門檻需明示」護欄。
//
// 事故形狀：UPDATE 沒有日期界 ⇒ 一次跑就把**整張表**（所有 market_period IS NULL
// 且當天有 period_history 的列）改寫，而操作者以為只動某個窗口。

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestInWindow(t *testing.T) {
	cases := []struct {
		start, end, date string
		want             bool
	}{
		{"", "", "2026-04-01", true},
		{"2026-04-01", "2026-04-30", "2026-03-31", false},
		{"2026-04-01", "2026-04-30", "2026-04-01", true}, // 含起日
		{"2026-04-01", "2026-04-30", "2026-04-30", true}, // 含迄日
		{"2026-04-01", "2026-04-30", "2026-05-01", false},
		{"2026-04-01", "", "2026-12-31", true}, // 只設起日
		{"", "2026-04-30", "2025-01-01", true}, // 只設迄日
		{"", "2026-04-30", "2026-05-01", false},
	}
	for _, c := range cases {
		if got := inWindow(c.start, c.end, c.date); got != c.want {
			t.Errorf("inWindow(%q,%q,%q) = %v, want %v", c.start, c.end, c.date, got, c.want)
		}
	}
}

func TestValidateWindowAndLabel(t *testing.T) {
	if err := validateWindow("2026-04-01", "2026-04-30"); err != nil {
		t.Fatalf("valid window rejected: %v", err)
	}
	if err := validateWindow("", ""); err != nil {
		t.Fatalf("open window rejected: %v", err)
	}
	if err := validateWindow("not-a-date", ""); err == nil {
		t.Fatal("malformed -start must be rejected")
	}
	if err := validateWindow("2026-05-01", "2026-04-01"); err == nil {
		t.Fatal("-start after -end must be rejected")
	}
	for _, c := range []struct{ start, end, want string }{
		{"", "", "(whole table)"},
		{"2026-04-01", "", "2026-04-01.."},
		{"", "2026-04-30", "..2026-04-30"},
		{"2026-04-01", "2026-04-30", "2026-04-01..2026-04-30"},
	} {
		if got := windowLabel(c.start, c.end); got != c.want {
			t.Errorf("windowLabel(%q,%q) = %q, want %q", c.start, c.end, got, c.want)
		}
	}
}

func TestGuardLargeUpdate(t *testing.T) {
	if err := guardLargeUpdate(backfillResult{Matched: maxUnattendedRows}, false); err != nil {
		t.Fatalf("matched == limit must pass: %v", err)
	}
	err := guardLargeUpdate(backfillResult{Matched: maxUnattendedRows + 1}, false)
	if err == nil {
		t.Fatal("matched > limit without -force must fail")
	}
	if !strings.Contains(err.Error(), "-force") || !strings.Contains(err.Error(), "-start/-end") {
		t.Errorf("error %q must tell the operator both escapes (-force, narrower window)", err.Error())
	}
	if err := guardLargeUpdate(backfillResult{Matched: maxUnattendedRows + 1}, true); err != nil {
		t.Fatalf("-force must allow it: %v", err)
	}
}

// TestRun_WindowFlagsValidated：壞的窗口在動任何 DB 之前就要被拒。
func TestRun_WindowFlagsValidated(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"-workdir", dir, "-start", "not-a-date", "-db", filepath.Join(dir, "x.db")}, io.Discard); err == nil {
		t.Fatal("malformed -start must fail before touching the DB")
	}
	if err := run([]string{"-workdir", dir, "-start", "2026-05-01", "-end", "2026-04-01", "-db", filepath.Join(dir, "x.db")}, io.Discard); err == nil {
		t.Fatal("-start after -end must fail")
	}
}

// TestBackfillSQLiteDB_WindowLeavesOutOfRangeRowsUntouched 是 #2124 的行為釘子：
// 窗口外的列（即使當天有 period_history）**不得**被更動。
func TestBackfillSQLiteDB_WindowLeavesOutOfRangeRowsUntouched(t *testing.T) {
	ctx := context.Background()
	db, _ := openTempDB(t)
	seedOutcomePeriodFixture(t, db) // 2026-04-01(live) / 2020-06-15(synthetic) / 2019-01-02(無 period)

	// 額外造一列「有 period_history 但在窗口外」⇒ 這才是會被抓到的形狀。
	hist := ledger.NewSQLiteHistoricalStore(db)
	if err := hist.UpsertPeriod(ctx, ledger.PeriodRow{
		Date: "2019-01-02", Period: "consolidation", IsSynthetic: 1,
		Source: "window-test", CapturedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed period 2019-01-02: %v", err)
	}

	res, err := backfillSQLiteDB(ctx, db, runConfig{start: "2026-04-01", end: "2026-04-01"})
	if err != nil {
		t.Fatalf("windowed backfill: %v", err)
	}
	if res.Total != 1 || res.Matched != 1 {
		t.Fatalf("windowed candidates = %+v, want total=1 matched=1 (only the in-window row)", res)
	}
	assertPeriod(t, db, "2026-04-01T00:00:00Z", "bull", "live") // 窗口內 ⇒ 填
	assertPeriod(t, db, "2020-06-15T00:00:00Z", "", "")         // 窗口外 ⇒ 不動（雖然有 period_history）
	assertPeriod(t, db, "2019-01-02T00:00:00Z", "", "")         // 窗口外 ⇒ 不動

	// 反假陽性對照：另開一份**一模一樣**的資料，不設窗口 ⇒ 三列都會被填
	// （含剛才在窗口外、動也不動的兩列）。這證明上面紅的是「窗口」，
	// 而不是資料本身不可達或別的錯誤。
	db2, _ := openTempDB(t)
	seedOutcomePeriodFixture(t, db2)
	hist2 := ledger.NewSQLiteHistoricalStore(db2)
	if err := hist2.UpsertPeriod(ctx, ledger.PeriodRow{
		Date: "2019-01-02", Period: "consolidation", IsSynthetic: 1,
		Source: "window-test", CapturedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed period 2019-01-02 (control): %v", err)
	}
	resCtl, err := backfillSQLiteDB(ctx, db2, runConfig{})
	if err != nil {
		t.Fatalf("unwindowed backfill: %v", err)
	}
	if resCtl.Matched != 3 {
		t.Fatalf("unwindowed matched = %d, want 3 (anti-false-positive: the same rows are reachable without a window)", resCtl.Matched)
	}
	assertPeriod(t, db2, "2026-04-01T00:00:00Z", "bull", "live")
	assertPeriod(t, db2, "2020-06-15T00:00:00Z", "black_swan", "synthetic")
	assertPeriod(t, db2, "2019-01-02T00:00:00Z", "consolidation", "synthetic")
}

// TestBackfillSQLiteDB_GuardBlocksBeforeWriting：超門檻且沒有 -force ⇒ 中止，
// 且**一列都不得被寫**（護欄必須在 UPDATE 之前）。
func TestBackfillSQLiteDB_GuardBlocksBeforeWriting(t *testing.T) {
	orig := maxUnattendedRows
	maxUnattendedRows = 2
	t.Cleanup(func() { maxUnattendedRows = orig })

	ctx := context.Background()
	db, _ := openTempDB(t)
	seedOutcomePeriodFixture(t, db) // matched = 2 == limit ⇒ 允許（邊界）
	if _, err := backfillSQLiteDB(ctx, db, runConfig{}); err != nil {
		t.Fatalf("matched == limit must be allowed: %v", err)
	}

	db2, _ := openTempDB(t)
	seedOutcomePeriodFixture(t, db2)
	insertOutcomeRow(t, db2, "2026-04-01T05:00:00Z", nil, nil) // 第三列 matched ⇒ 3 > limit
	res, err := backfillSQLiteDB(ctx, db2, runConfig{})
	if err == nil {
		t.Fatal("want an error above the limit without -force")
	}
	if res.Matched != 3 {
		t.Fatalf("res.Matched = %d, want 3 (the report must still be produced)", res.Matched)
	}
	assertPeriod(t, db2, "2026-04-01T00:00:00Z", "", "") // 沒有寫入
	assertPeriod(t, db2, "2020-06-15T00:00:00Z", "", "")

	// -force ⇒ 允許並真的寫入。
	res, err = backfillSQLiteDB(ctx, db2, runConfig{force: true})
	if err != nil {
		t.Fatalf("with -force: %v", err)
	}
	if res.Matched != 3 {
		t.Fatalf("with -force matched = %d, want 3", res.Matched)
	}
	assertPeriod(t, db2, "2026-04-01T00:00:00Z", "bull", "live")
}
