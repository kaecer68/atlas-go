package calibration

// #2107：這條 24h 任務（predictor_calibrate）的 reader 曾用
// ledger.OpenSQLiteDB（opens-or-creates）讀 job-local sqlite，於是生產刪掉
// data/state/atlas.db 後它又把檔案建回來。改為唯讀開啟既有檔：不建立、不寫入，
// 檔案不存在時語意與「表是空的」相同（回 (0,false)，與修改前一致）。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestOpenSQLiteReadOnly_MissingFileIsNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.db")

	if _, err := openSQLiteReadOnly(path); err == nil {
		t.Fatal("opening a missing file must fail, not create it")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("read-only open must not create %s (#2107 的 artifact 就是這樣長出來的)", path)
	}
}

func TestOpenSQLiteReadOnly_RejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	_ = db.Close()

	ro, err := openSQLiteReadOnly(path)
	if err != nil {
		t.Fatalf("openSQLiteReadOnly: %v", err)
	}
	defer func() { _ = ro.Close() }()

	// 寫入必須在 driver 層失敗（mode=ro 被強制），否則「不寫 sqlite」只是註解。
	if _, err := ro.Exec(`CREATE TABLE tamper (x TEXT)`); err == nil {
		t.Fatal("write on a read-only handle succeeded; mode=ro is not enforced")
	}
}

func TestTryHitRateEval_MissingDBReturnsNoData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.db")

	score, ok := tryHitRateEval(path)
	if ok || score != 0 {
		t.Fatalf("tryHitRateEval(missing) = (%v,%v), want (0,false)", score, ok)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("tryHitRateEval must not create %s", path)
	}
}

func TestTryHitRateEval_ComputesHitRateFromExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

	// 30 個非中性日（24 命中 ⇒ 0.8）＋ 10 個中性日（必須被排除）。
	day := func(i int) string {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
	}
	for i := 0; i < 30; i++ {
		if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date: day(i), PredictedDirection: "up", ActualDirection: "up",
			Hit: i < 24, ModelVersion: "test", CapturedAt: now, IsSynthetic: 0,
		}); err != nil {
			t.Fatalf("upsert row %d: %v", i, err)
		}
	}
	for i := 30; i < 40; i++ {
		if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date: day(i), PredictedDirection: "neutral", ActualDirection: "neutral",
			Hit: true, ModelVersion: "test", CapturedAt: now, IsSynthetic: 0,
		}); err != nil {
			t.Fatalf("upsert neutral row %d: %v", i, err)
		}
	}
	// synthetic 列必須被排除（LoadPredictionBacktestRange 內建 is_synthetic=0 過濾）。
	if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
		Date: day(41), PredictedDirection: "down", ActualDirection: "up",
		Hit: false, ModelVersion: "test", CapturedAt: now, IsSynthetic: 1,
	}); err != nil {
		t.Fatalf("upsert synthetic row: %v", err)
	}
	_ = db.Close()

	score, ok := tryHitRateEval(path)
	if !ok {
		t.Fatal("want ok=true with >=30 non-neutral live rows")
	}
	if score < 0.799 || score > 0.801 {
		t.Fatalf("score = %v, want 0.8 (24/30; neutral + synthetic rows excluded)", score)
	}
}

func TestTryHitRateEval_TooFewRowsIsNeutral(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 29; i++ { // 29 < 30 ⇒ 不評分
		if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date:               time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02"),
			PredictedDirection: "up", ActualDirection: "up", Hit: true,
			ModelVersion: "test", CapturedAt: now, IsSynthetic: 0,
		}); err != nil {
			t.Fatalf("upsert row %d: %v", i, err)
		}
	}
	_ = db.Close()

	if score, ok := tryHitRateEval(path); ok || score != 0 {
		t.Fatalf("tryHitRateEval(<30 rows) = (%v,%v), want (0,false) neutral baseline", score, ok)
	}
}
