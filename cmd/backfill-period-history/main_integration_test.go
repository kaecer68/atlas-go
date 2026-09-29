//go:build integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// 本檔是 #2107 的 PG 端到端證據：在**生產形狀**（ATLAS_STORE_BACKEND=postgres、
// $DATABASE_URL、不帶任何顯式 flag）下，backfill-period-history 必須真的把
// period_history / regime_history 寫進 PostgreSQL，而不是 job-local 的 sqlite
// artifact（data/state/atlas.db）。
//
// 與同 package 的 main_backend_test.go 的差別：那裡替換 initPostgresPool seam
// （不連線）驗證 wiring 決策；本檔刻意**不替換**，走真的 atlasdb.Init
// （ping ＋ <workdir>/sql/migrations），再用**另一條連線**查證列真的落地。
//
// 資料紀律：只用明顯假造的日期（1990-01-02..04，不在真實 ingest 視窗內），
// 並在 t.Cleanup 內用自己的連線刪掉自己插入的列。
//
// 日期分工：`go test ./cmd/...` 會**平行**跑各 package（CI 用 -p 1 序列化，
// 本機預設不會）。各 package 的 integration test 因此必須使用互斥的假日期，
// 否則 A 的 t.Cleanup DELETE 會在半途清掉 B 的種子列。本檔用 1990-01-02..04
// （cmd/backfill-period-history-range 用 1990-02-05..09，
// cmd/backfill-outcome-period 用 1994-01-02/03）。

// itPeriodDates 是本檔專用的假日期（1990 年，不會與真實 period_history 的
// ingest 視窗 2026-04-25.. 衝突）。
var itPeriodDates = []string{"1990-01-02", "1990-01-03", "1990-01-04"}

// itPeriodMacroTree 造 macro 目錄骨架，再寫入本檔專用的 1990 快照。
//
// 沿用既有 fixtureTree（含 latest/previous/_metadata 這些必須被跳過的檔案的骨架），
// 但不使用它的 2026-05-01..03 日期：那些日期落在真實 ingest 視窗內，
// production-shaped DB 上可能已有 live 列（upsertDay 會刻意跳過 is_synthetic=0
// 的日期），斷言就會變成「看 DB 現況」而不是「看本次寫入」。呼叫端把
// start/end 鎖在 itPeriodDates，fixtureTree 的 2026 快照不會被處理、也不會進 PG。
func itPeriodMacroTree(t *testing.T) string {
	t.Helper()
	workDir, macroDir := fixtureTree(t)
	base := unixDate(itPeriodDates[0])
	for i, date := range itPeriodDates {
		writeSnapshot(t, macroDir, date, marketdata.MacroDataSnapshot{
			VIX:                marketdata.MacroDataPoint{Symbol: "^VIX", Value: 14 + float64(i)},
			DXY:                marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 100},
			TAIEX:              marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 25000 + float64(i)*100},
			ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: 8},
			RecordedAt:         base + int64(i)*86400,
		})
	}
	return workDir
}

// itPeriodWorkDir 回傳本檔的 workDir。
//
// CLI 只有一個 workDir，而且它同時決定了兩件事：macro 快照的來源
// （<workdir>/data/state/macro）與 migrations 的位置（<workdir>/sql/migrations，
// openSink 內硬編）。測試要的是「macro 樹在暫存目錄（不污染 repo）＋ migrations
// 指向真 repo 的 sql/」，所以用一個 symlink 讓 <workdir>/sql → <repo>/sql。
// 生產情境兩者本來就都在 repo root，不需要這招。
func itPeriodWorkDir(t *testing.T) string {
	t.Helper()
	workDir := itPeriodMacroTree(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "sql"), filepath.Join(workDir, "sql")); err != nil {
		t.Fatalf("symlink <workdir>/sql → %s/sql: %v", root, err)
	}
	return workDir
}

// itCleanupPeriodPG 刪掉本檔自己寫入的列（執行前先清一次，t.Cleanup 再清一次），
// 形狀照 cmd/backfill-futures-bars 的 cleanupPGFuturesCLI。
func itCleanupPeriodPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		ctx := context.Background()
		for _, date := range itPeriodDates {
			_, _ = pool.Exec(ctx, `DELETE FROM regime_history WHERE date = $1`, date)
			_, _ = pool.Exec(ctx, `DELETE FROM period_history WHERE date = $1`, date)
		}
	}
	del()
	t.Cleanup(del)
}

func TestRun_PostgresDeclaredWritesPeriodAndRegimeHistory(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	itCleanupPeriodPG(t, raw)

	workDir := itPeriodWorkDir(t)

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)

	now := time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)
	stats, err := run(context.Background(), runConfig{
		workDir: workDir,
		start:   itPeriodDates[0],
		end:     itPeriodDates[len(itPeriodDates)-1],
		// 不帶 -db / -pg，store 留 nil ⇒ 後端完全由宣告決定（生產形狀）。
		now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("run(ATLAS_STORE_BACKEND=postgres, no explicit flag): %v", err)
	}
	if want := len(itPeriodDates); stats.upsertPeriod != want || stats.upsertRegime != want {
		t.Fatalf("stats upserted period=%d regime=%d, want %d each (errors=%d dates=%v)",
			stats.upsertPeriod, stats.upsertRegime, want, stats.errors, stats.errorDates)
	}

	// 用**獨立連線**查證（不信任 store 自己的讀取路徑）。
	ctx := context.Background()
	type periodRow struct {
		date      string
		period    string
		synthetic int
		source    string
	}
	rows, err := raw.Query(ctx,
		`SELECT date, period, is_synthetic, source FROM period_history WHERE date = ANY($1) ORDER BY date`,
		itPeriodDates)
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
	if len(got) != len(itPeriodDates) {
		t.Fatalf("period_history rows for %v = %d, want %d (the backfill must really write to PostgreSQL)",
			itPeriodDates, len(got), len(itPeriodDates))
	}
	for i, r := range got {
		if r.date != itPeriodDates[i] {
			t.Errorf("row %d date = %q, want %q", i, r.date, itPeriodDates[i])
		}
		if r.period == "" {
			t.Errorf("row %s has an empty period", r.date)
		}
		if r.synthetic != 1 {
			t.Errorf("row %s is_synthetic = %d, want 1 (backfill marker)", r.date, r.synthetic)
		}
		if r.source != "period_history_backfill" {
			t.Errorf("row %s source = %q, want period_history_backfill", r.date, r.source)
		}
	}

	// regime_history 必須同步落地（同一輪 upsertDay）。
	var regimeCount int
	if err := raw.QueryRow(ctx, `
		SELECT count(*) FROM regime_history
		WHERE date = ANY($1) AND is_synthetic = 1 AND source = 'period_history_backfill'`,
		itPeriodDates).Scan(&regimeCount); err != nil {
		t.Fatalf("count regime_history: %v", err)
	}
	if regimeCount != len(itPeriodDates) {
		t.Fatalf("regime_history rows = %d, want %d", regimeCount, len(itPeriodDates))
	}

	// 宣告 postgres ⇒ 不得留下 job-local 的 sqlite artifact（#2107 的原始事故形狀）。
	if _, statErr := os.Stat(defaultSQLiteArtifact(workDir)); statErr == nil {
		t.Fatalf("declared postgres must not create the sqlite artifact %s", defaultSQLiteArtifact(workDir))
	}
}
