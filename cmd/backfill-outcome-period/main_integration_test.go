//go:build integration

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

// 本檔是 #2107 的 PG 端到端證據：在**生產形狀**（ATLAS_STORE_BACKEND=postgres、
// $DATABASE_URL、**不帶** -pg）下，backfill-outcome-period 必須走 PostgreSQL 路徑，
// 用 period_history 的 join 把 recommendation_outcomes.market_period /
// market_period_source 填起來。
//
// 這條測試呼叫真正的 run(args, stdout)（真 flag parsing），所以「模式由宣告決定」
// 這件事是被真的驗證的；stdout 會斷言出現 "postgres backfill"。
//
// 資料紀律：只用明顯假造的 1994 交易日與假 session_id / symbol，並在 t.Cleanup
// 內用自己的連線刪掉自己插入的列。
//
// 日期分工：`go test ./cmd/...` 會**平行**跑各 package（CI 用 -p 1 序列化，
// 本機預設不會）。各 package 的 integration test 因此必須使用互斥的假日期，
// 否則 A 的 t.Cleanup DELETE 會在半清掉 B 的種子列。本檔用 1994-01-02/03
// （cmd/backfill-period-history 用 1990-01-02..04，
// cmd/backfill-period-history-range 用 1990-02-05..09）。
//
// 時區對齊：backfillPostgres 用
// to_char(time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') 取交易日，
// 所以播種的 time 用 00:00Z（台北同一天 08:00）⇔ period_history.date 精準相等。

const (
	itOutcomeSessionID = "it-backfill-outcome-period-1990"
	itOutcomeSymbol    = "PGOUT1990"
	itOutcomeAgentID   = "it-agent-1990"
)

// itOutcomeSeed 是一組「假交易日 + period_history 列 + outcome 寫入時刻」。
type itOutcomeSeed struct {
	date      string
	period    string
	synthetic int
	at        time.Time
}

var (
	// itOutcomeLiveDay：period_history 是 live 列（is_synthetic=0）⇒ source 應為 'live'。
	itOutcomeLiveDay = itOutcomeSeed{
		date: "1994-01-02", period: "bull", synthetic: 0,
		at: time.Date(1994, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	// itOutcomeSyntheticDay：period_history 是 backfill 列 ⇒ source 應為 'synthetic'。
	itOutcomeSyntheticDay = itOutcomeSeed{
		date: "1994-01-03", period: "black_swan", synthetic: 1,
		at: time.Date(1994, 1, 3, 0, 0, 0, 0, time.UTC),
	}
	// itOutcomeUnmatchedDay：沒有 period_history 列 ⇒ 必須保持 NULL（不猜）。
	itOutcomeUnmatchedAt = time.Date(1994, 2, 15, 0, 0, 0, 0, time.UTC)
)

// itSeedOutcomePeriodPG 在 PostgreSQL 播種本檔專用的列（retry 安全、可重跑）。
func itSeedOutcomePeriodPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, day := range []itOutcomeSeed{itOutcomeLiveDay, itOutcomeSyntheticDay} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO period_history (date, period, recorded_at, captured_at, is_synthetic, source, detector_version)
			VALUES ($1, $2, now(), now(), $3, 'it_backfill_outcome_period_seed', 'v1')
			ON CONFLICT(date) DO UPDATE SET
				period = excluded.period,
				is_synthetic = excluded.is_synthetic,
				source = excluded.source`,
			day.date, day.period, day.synthetic); err != nil {
			t.Fatalf("seed period_history %s: %v", day.date, err)
		}
	}
	for _, at := range []time.Time{itOutcomeLiveDay.at, itOutcomeSyntheticDay.at, itOutcomeUnmatchedAt} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO recommendation_outcomes
				(time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, market_period, market_period_source)
			VALUES ($1, $2, $3, $4, 'sector', 80, true, 'it-seed', 100, NULL, NULL)`,
			at, itOutcomeSessionID, itOutcomeSymbol, itOutcomeAgentID); err != nil {
			t.Fatalf("seed recommendation_outcomes %s: %v", at, err)
		}
	}
}

// itCleanupOutcomePeriodPG 刪掉本檔自己插入的列。
func itCleanupOutcomePeriodPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM recommendation_outcomes WHERE session_id = $1`, itOutcomeSessionID)
		for _, day := range []itOutcomeSeed{itOutcomeLiveDay, itOutcomeSyntheticDay} {
			_, _ = pool.Exec(ctx, `DELETE FROM period_history WHERE date = $1`, day.date)
		}
	}
	del()
	t.Cleanup(del)
}

// itReadOutcomePeriod 用獨立連線讀回一列（NULL 以 "" 表示）。
func itReadOutcomePeriod(t *testing.T, pool *pgxpool.Pool, at time.Time) (period, source string) {
	t.Helper()
	var p, s *string
	if err := pool.QueryRow(context.Background(), `
		SELECT market_period, market_period_source FROM recommendation_outcomes
		WHERE session_id = $1 AND symbol = $2 AND time = $3
		ORDER BY time DESC LIMIT 1`,
		itOutcomeSessionID, itOutcomeSymbol, at).Scan(&p, &s); err != nil {
		t.Fatalf("select outcome %s: %v", at.Format(time.RFC3339), err)
	}
	if p != nil {
		period = *p
	}
	if s != nil {
		source = *s
	}
	return period, source
}

func TestRun_PostgresDeclaredBackfillsMarketPeriodFromPeriodHistory(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	itCleanupOutcomePeriodPG(t, raw)
	itSeedOutcomePeriodPG(t, raw)

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)

	// 走真 flag parsing：只給 -workdir（**不給** -pg），模式完全由宣告決定。
	var out bytes.Buffer
	if err := run([]string{"-workdir", filepath.Join("..", "..")}, &out); err != nil {
		t.Fatalf("run(-workdir ../..) with ATLAS_STORE_BACKEND=postgres: %v\nstdout:\n%s", err, out.String())
	}
	// 輸出必須證明走的是 postgres 模式（sqlite 模式會印 "sqlite backfill"）。
	if !strings.Contains(out.String(), "postgres backfill") {
		t.Fatalf("stdout must report the postgres pass, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "sqlite backfill") {
		t.Fatalf("declared postgres must not run the sqlite pass, got:\n%s", out.String())
	}

	// 用**獨立連線**查證 join 的結果（period 與 source 都要對）。
	livePeriod, liveSource := itReadOutcomePeriod(t, raw, itOutcomeLiveDay.at)
	if livePeriod != "bull" || liveSource != "live" {
		t.Errorf("live day (1990-01-02) got period=%q source=%q, want bull/live", livePeriod, liveSource)
	}
	synPeriod, synSource := itReadOutcomePeriod(t, raw, itOutcomeSyntheticDay.at)
	if synPeriod != "black_swan" || synSource != "synthetic" {
		t.Errorf("synthetic day (1990-01-03) got period=%q source=%q, want black_swan/synthetic", synPeriod, synSource)
	}
	// 沒有 period_history 列的交易日必須保持 NULL（unknown，不猜）。
	unPeriod, unSource := itReadOutcomePeriod(t, raw, itOutcomeUnmatchedAt)
	if unPeriod != "" || unSource != "" {
		t.Errorf("unmatched day (1990-02-15) got period=%q source=%q, want NULL/NULL", unPeriod, unSource)
	}

	// 整體：本檔播種的 3 列中恰好 2 列被填、1 列仍為 NULL。
	var filled, nulls int
	if err := raw.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE market_period IS NOT NULL),
		       count(*) FILTER (WHERE market_period IS NULL)
		FROM recommendation_outcomes WHERE session_id = $1`, itOutcomeSessionID).Scan(&filled, &nulls); err != nil {
		t.Fatalf("count seeded outcomes: %v", err)
	}
	if filled != 2 || nulls != 1 {
		t.Fatalf("seeded rows: filled=%d null=%d, want 2/1", filled, nulls)
	}
}
