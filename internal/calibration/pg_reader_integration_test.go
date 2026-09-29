//go:build integration

package calibration

// #2123 的 PG 端到端證據：分數必須來自 **PostgresHistoricalStore**（生產形狀），
// 而不是 job-local sqlite artifact。

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// itPGModelVersion 是本測試專用的標記（避免與其他 integration test／生產列互踩）。
const itPGModelVersion = "it-2123-predictor-reader"

func itPGCleanup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM prediction_backtest WHERE model_version = $1`, itPGModelVersion)
	}
	del()
	t.Cleanup(del)
}

// TestHitRateFromStore_PostgresHistoricalStore 播 40 筆（30 命中）進 PG，
// 斷言 backend-aware reader 取到 0.75 —— 這條在修好 #2123 之前是不可能的
// （舊 reader 只看 job-local sqlite ⇒ 恒回中性 0.5）。
func TestHitRateFromStore_PostgresHistoricalStore(t *testing.T) {
	dsn := testdb.URL(t)
	testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))
	pool := testdb.Pool(t, filepath.Join("..", "..", "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	itPGCleanup(t, raw)

	store := ledger.NewPostgresHistoricalStore(pool)
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	const rows, hits = 40, 30
	for i := 0; i < rows; i++ {
		d := time.Date(1985, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
		if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date: d, PredictedDirection: "up", ActualDirection: "up",
			Hit: i < hits, ModelVersion: itPGModelVersion, CapturedAt: now, IsSynthetic: 0,
		}); err != nil {
			t.Fatalf("seed %s: %v", d, err)
		}
	}

	score, ok := hitRateFromStore(store)
	if !ok {
		t.Fatal("want ok=true: >=30 non-neutral live rows were seeded into PostgreSQL")
	}
	if math.Abs(score-0.75) > 1e-9 {
		t.Fatalf("score = %v, want 0.75 (30/40 read through PostgresHistoricalStore)", score)
	}

	// 反假陽性對照：同一時刻的**空 sqlite** reader 得不到分數 ⇒ 上面 0.75 只可能來自 PG。
	// （若哪天有人把 reader 接回 job-local sqlite，這裡與上面的斷言會同時說明差異。）
	if score, ok := hitRateFromStore(itSQLiteHistoricalStore(t)); ok {
		t.Fatalf("empty sqlite reader must not produce a score, got (%v,true) — the 0.75 above must come from PostgreSQL", score)
	}
}
