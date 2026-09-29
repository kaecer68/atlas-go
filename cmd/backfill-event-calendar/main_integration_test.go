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
// $DATABASE_URL、不帶任何顯式 flag）下，backfill-event-calendar 必須真的把
// event_calendar_history 寫進 PostgreSQL，而不是 job-local 的 sqlite artifact。
//
// 與同 package 的 main_backend_test.go 的差別：那裡替換 initPostgresPool seam
// （不連線）驗證 wiring 決策；本檔刻意**不替換**，走真的 atlasdb.Init
// （ping ＋ <workdir>/sql/migrations），再用**另一條連線**查證列真的落地。
//
// 不打網路：provider 由 stubProvider + fixedProviderFactory 注入。
//
// 資料紀律：event_id 一律帶 "stub_backfill_" 前綴 —— 真實 provider 名
// （twse / twse_openapi / msci_static / nsf_static）不可能產生它，所以這些列
// 既不會撞到真資料，也能在 t.Cleanup 內被精準刪除。

// itEventIDPrefix 是本檔 stub provider 造出的 event_id 前綴。
const itEventIDPrefix = "stub_backfill_"

// itEventStartYear / itEventEndYear：sampleProviderEvents("stub") 提供 2023 與 2024 各 2 個事件。
const (
	itEventStartYear = 2023
	itEventEndYear   = 2024
)

// itCleanupEventCalendarPG 刪掉本檔自己寫入的列。
func itCleanupEventCalendarPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM event_calendar_history WHERE event_id LIKE $1`, itEventIDPrefix+"%")
	}
	del()
	t.Cleanup(del)
}

func TestRun_PostgresDeclaredWritesEventCalendarHistory(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	itCleanupEventCalendarPG(t, raw)

	// stub provider + 固定 factory：完全不碰網路（下面的 eventsFetched 斷言證明真的走了 stub）。
	np := namedProvider{name: "stub", provider: &stubProvider{
		name:   "stub",
		events: sampleProviderEvents("stub"),
	}}

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)

	stats, err := run(context.Background(), runConfig{
		// workDir = repo root ⇒ atlasdb.Init 讀 <workdir>/sql/migrations。
		workDir:   filepath.Join("..", ".."),
		startYear: itEventStartYear,
		endYear:   itEventEndYear,
		// provider 只影響輸出標籤；真正的事件來源是注入的 factory。
		provider: "auto",
		factory:  fixedProviderFactory(np),
		// 不帶 -db / -pg，store 留 nil ⇒ 後端完全由宣告決定（生產形狀）。
		now: func() time.Time { return time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("run(ATLAS_STORE_BACKEND=postgres, no explicit flag): %v", err)
	}
	if got := stats.eventsFetched["stub"]; got != 4 {
		t.Fatalf("eventsFetched[stub] = %d, want 4 (the injected stub factory must be used — no network)", got)
	}
	if got := stats.eventsWritten["stub"]; got != 4 || stats.errors != 0 {
		t.Fatalf("written=%d errors=%d detail=%v, want 4/0", got, stats.errors, stats.errorProviders)
	}

	// 用**獨立連線**查證（不信任 store 自己的讀取路徑）。
	ctx := context.Background()
	var count int
	if err := raw.QueryRow(ctx,
		`SELECT count(*) FROM event_calendar_history WHERE event_id LIKE $1`,
		itEventIDPrefix+"%").Scan(&count); err != nil {
		t.Fatalf("count event_calendar_history: %v", err)
	}
	if count != 4 {
		t.Fatalf("event_calendar_history rows = %d, want 4 (the backfill must really write to PostgreSQL)", count)
	}

	// 每一列都要帶 backfill marker（is_synthetic=1）與 provider 名。
	var badMarkers int
	if err := raw.QueryRow(ctx, `
		SELECT count(*) FROM event_calendar_history
		WHERE event_id LIKE $1 AND (is_synthetic <> 1 OR source <> 'stub')`,
		itEventIDPrefix+"%").Scan(&badMarkers); err != nil {
		t.Fatalf("check markers: %v", err)
	}
	if badMarkers != 0 {
		t.Fatalf("%d rows carry the wrong backfill marker (want is_synthetic=1, source='stub')", badMarkers)
	}

	// 釘住一列的完整形狀（日期、event_id、active_theme 都真的進了 PG）。
	var theme string
	if err := raw.QueryRow(ctx,
		`SELECT active_theme FROM event_calendar_history WHERE date = $1 AND event_id = $2`,
		"2023-06-13", "stub_backfill_ex_dividend_2023-06-13_2330").Scan(&theme); err != nil {
		t.Fatalf("select pinned event row: %v", err)
	}
	if theme != "ex_dividend" {
		t.Fatalf("active_theme = %q, want ex_dividend", theme)
	}
}
