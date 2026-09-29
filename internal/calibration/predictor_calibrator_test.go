package calibration

// #2107：這條 24h 任務（predictor_calibrate）的 reader 曾用
// ledger.OpenSQLiteDB（opens-or-creates）讀 job-local sqlite，於是生產刪掉
// data/state/atlas.db 後它又把檔案建回來。改為唯讀開啟既有檔：不建立、不寫入，
// 檔案不存在時語意與「表是空的」相同（回 (0,false)，與修改前一致）。

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
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

// ---------------------------------------------------------------------------
// #2123：backend-aware reader（prediction_backtest 走 HistoricalStore）
// ---------------------------------------------------------------------------

// itSeedPredictionRows 播 n 筆 is_synthetic=0 的 prediction_backtest（前 hits 筆命中）。
func itSeedPredictionRows(t *testing.T, store ledger.HistoricalStore, n, hits int) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		d := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
		if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date: d, PredictedDirection: "up", ActualDirection: "up",
			Hit: i < hits, ModelVersion: "eventflow-realtime", CapturedAt: now, IsSynthetic: 0,
		}); err != nil {
			t.Fatalf("seed prediction row %s: %v", d, err)
		}
	}
}

// itSQLiteHistoricalStore 開一個 temp sqlite ledger 並回傳 HistoricalStore。
func itSQLiteHistoricalStore(t *testing.T) ledger.HistoricalStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ledger.NewSQLiteHistoricalStore(db)
}

// TestHitRateFromStore_UsesStoreRowsNotLocalPath 是 #2123 的接線釘子：
// 分數必須來自傳入的 store（生產 = PostgresHistoricalStore），而不是任何 job-local 路徑。
func TestHitRateFromStore_UsesStoreRowsNotLocalPath(t *testing.T) {
	store := itSQLiteHistoricalStore(t)
	itSeedPredictionRows(t, store, 40, 30)

	score, ok := hitRateFromStore(store)
	if !ok {
		t.Fatal("want ok=true with 40 non-neutral rows (threshold is 30)")
	}
	if math.Abs(score-0.75) > 1e-9 {
		t.Fatalf("score = %v, want 0.75 (30/40 from the injected store)", score)
	}

	// evaluator 包裝層要回同一個值（呼叫端用這個函式，不直接呼叫 hitRateFromStore）
	eval := NewPredictorEvaluatorWithStore(store)
	got, err := eval(&config.ParametersConfig{})
	if err != nil {
		t.Fatalf("evaluator: %v", err)
	}
	if math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("evaluator score = %v, want 0.75", got)
	}
}

// TestHitRateFromStore_FallsBackToNeutral 樣本不足／store 不可用 ⇒ ok=false
// （呼叫端因此回中性 0.5，optimizer 不會被雜訊帶動）。
func TestHitRateFromStore_FallsBackToNeutral(t *testing.T) {
	t.Run("below_threshold", func(t *testing.T) {
		store := itSQLiteHistoricalStore(t)
		itSeedPredictionRows(t, store, 29, 20)
		if score, ok := hitRateFromStore(store); ok {
			t.Fatalf("29 rows (<30) must not produce a score, got (%v,true)", score)
		}
	})
	t.Run("nil_store", func(t *testing.T) {
		if score, ok := hitRateFromStore(nil); ok {
			t.Fatalf("nil store must not produce a score, got (%v,true)", score)
		}
	})
	t.Run("empty_store", func(t *testing.T) {
		if score, ok := hitRateFromStore(itSQLiteHistoricalStore(t)); ok {
			t.Fatalf("empty store must not produce a score, got (%v,true)", score)
		}
	})
	t.Run("neutral_rows_excluded", func(t *testing.T) {
		store := itSQLiteHistoricalStore(t)
		ctx := context.Background()
		now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
		// 20 筆非中性（全命中）＋ 20 筆中性 ⇒ 非中性 20 < 30 ⇒ 不評分
		itSeedPredictionRows(t, store, 20, 20)
		for i := 0; i < 20; i++ {
			d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
			if err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
				Date: d, PredictedDirection: "neutral", ActualDirection: "neutral",
				Hit: true, ModelVersion: "eventflow-realtime", CapturedAt: now, IsSynthetic: 0,
			}); err != nil {
				t.Fatalf("seed neutral row: %v", err)
			}
		}
		if score, ok := hitRateFromStore(store); ok {
			t.Fatalf("neutral days must not count toward the sample floor, got (%v,true)", score)
		}
	})
}

// TestCalibratePredictorWithStore_RealMeasurementEntersTheOptimizer：
// 用真的 store（40 筆 / 30 命中）跑一次，斷言 **baseline 分數 = 真命中率**（不是中性 0.5）。
//
// 註（#2123 的量測結論，2026-09-29 實測 + 更正）：這條路徑今天不會寫入任何參數，
// 但**原因不是**「evaluator 對候選參數不敏感」（我原先的假設，已被實驗推翻：見 PR body
// 的更正段落）—— 真正原因是這四個 `predictor_*` 名字**不在 `config` 的 `parameterTable`**，
// `CalibrateParameters` 以 `if !ok { continue }` **靜默跳過**它們。
//
// 因此本測試**不斷言 changes==0**：那會（a）把⑦的未來修正鎖死，且（b）以錯誤的因果為名。
// 真正被釘住的是「**量測值確實進到 optimizer**」（上面的 baseline 斷言）。
func TestCalibratePredictorWithStore_RealMeasurementEntersTheOptimizer(t *testing.T) {
	store := itSQLiteHistoricalStore(t)
	itSeedPredictionRows(t, store, 40, 30)

	res, err := CalibratePredictorWithStore(context.Background(), store)
	if err != nil {
		t.Fatalf("CalibratePredictorWithStore: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if math.Abs(res.BaselineScore-0.75) > 1e-9 {
		t.Fatalf("baseline = %v, want 0.75 (the store measurement must reach the optimizer, not the neutral 0.5 fallback)",
			res.BaselineScore)
	}
}
