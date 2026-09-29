//go:build integration

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

// 本檔是 #2107 的 PG 端到端證據：在**生產形狀**（ATLAS_STORE_BACKEND=postgres、
// $DATABASE_URL、不帶任何顯式 flag）下，backfill-period-history-range 必須真的把
// period_history 寫進 PostgreSQL，而不是 job-local 的 sqlite artifact。
//
// 與同 package 的 main_backend_test.go 的差別：那裡替換 initPostgresPool seam
// （不連線）驗證 wiring 決策；本檔刻意**不替換**，走真的 atlasdb.Init
// （ping ＋ <workdir>/sql/migrations），再用**另一條連線**查證列真的落地。
//
// 資料紀律：OHLCV 只造明顯假造的 1990-02-05..09（不在真實 ingest 視窗內），
// 並在 t.Cleanup 內用自己的連線刪掉自己插入的列。
//
// 日期分工：`go test ./cmd/...` 會**平行**跑各 package（CI 用 -p 1 序列化，
// 本機預設不會）。各 package 的 integration test 因此必須使用互斥的假日期，
// 否則 A 的 t.Cleanup DELETE 會在半途清掉 B 的種子列。本檔用 1990-02-05..09
// （cmd/backfill-period-history 用 1990-01-02..04，cmd/backfill-outcome-period
// 用 1994-01-02/03）。

const (
	// itRangeStart 是本檔專用的假資料起日（1990 年，不會與真資料衝突；也不與同批
	// 平行執行的其他 integration test package 撞期 —— 見檔尾的日期分工說明）。
	itRangeStart = "1990-02-05"
	// itRangeDays 是假資料的天數。
	itRangeDays = 5
)

// itRangeDates 回傳假資料覆蓋的日期（含起日，逐日遞增）。
func itRangeDates(t *testing.T) []string {
	t.Helper()
	start, err := time.Parse("2006-01-02", itRangeStart)
	if err != nil {
		t.Fatalf("parse %s: %v", itRangeStart, err)
	}
	out := make([]string, 0, itRangeDays)
	for i := 0; i < itRangeDays; i++ {
		out = append(out, start.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return out
}

// itCleanupRangePG 刪掉本檔自己寫入的 period_history 列。
func itCleanupRangePG(t *testing.T, pool *pgxpool.Pool, dates []string) {
	t.Helper()
	del := func() {
		ctx := context.Background()
		for _, date := range dates {
			_, _ = pool.Exec(ctx, `DELETE FROM period_history WHERE date = $1`, date)
		}
	}
	del()
	t.Cleanup(del)
}

func TestRun_PostgresDeclaredWritesOHLCVPeriodHistory(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	dates := itRangeDates(t)
	itCleanupRangePG(t, raw, dates)

	// OHLCV 來源檔走既有的 fixture helper（絕對路徑 ⇒ 不受 workDir 影響）。
	src := fixtureJSONL(t, itRangeStart, itRangeDays, []string{"0050.TW", "2330.TW"})

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)

	now := time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)
	stats, err := run(context.Background(), runConfig{
		// workDir = repo root ⇒ atlasdb.Init 讀 <workdir>/sql/migrations。
		workDir:    filepath.Join("..", ".."),
		sourcePath: src,
		start:      itRangeStart,
		end:        dates[len(dates)-1],
		taiexProxy: "0050.TW",
		// 不帶 -db / -pg，store 留 nil ⇒ 後端完全由宣告決定（生產形狀）。
		now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("run(ATLAS_STORE_BACKEND=postgres, no explicit flag): %v", err)
	}
	if stats.upsertPeriod != len(dates) {
		t.Fatalf("stats upsertPeriod = %d, want %d (errors=%d dates=%v)",
			stats.upsertPeriod, len(dates), stats.errors, stats.errorDates)
	}

	// 用**獨立連線**查證（不信任 store 自己的讀取路徑）。
	ctx := context.Background()
	var count int
	if err := raw.QueryRow(ctx,
		`SELECT count(*) FROM period_history WHERE date = ANY($1)`, dates).Scan(&count); err != nil {
		t.Fatalf("count period_history: %v", err)
	}
	if count != len(dates) {
		t.Fatalf("period_history rows = %d, want %d (the backfill must really write to PostgreSQL)", count, len(dates))
	}

	// 每一列都必須帶 backfill marker 與本指令專屬的 source 標記。
	type periodRow struct {
		date      string
		period    string
		synthetic int
		source    string
	}
	rows, err := raw.Query(ctx,
		`SELECT date, period, is_synthetic, source FROM period_history WHERE date = ANY($1) ORDER BY date`, dates)
	if err != nil {
		t.Fatalf("select period_history: %v", err)
	}
	var got []periodRow
	for rows.Next() {
		var r periodRow
		if err := rows.Scan(&r.date, &r.period, &r.synthetic, &r.source); err != nil {
			rows.Close()
			t.Fatalf("scan period_history: %v", err)
		}
		got = append(got, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate period_history: %v", err)
	}
	if len(got) != len(dates) {
		t.Fatalf("scanned %d rows, want %d", len(got), len(dates))
	}
	for i, r := range got {
		if r.date != dates[i] {
			t.Errorf("row %d date = %q, want %q", i, r.date, dates[i])
		}
		if r.period == "" {
			t.Errorf("row %s has an empty period", r.date)
		}
		if r.synthetic != 1 {
			t.Errorf("row %s is_synthetic = %d, want 1 (backfill marker)", r.date, r.synthetic)
		}
		if r.source != "period_history_range_backfill_ohlcv" {
			t.Errorf("row %s source = %q, want period_history_range_backfill_ohlcv", r.date, r.source)
		}
	}
}
