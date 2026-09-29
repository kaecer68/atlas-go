package calibration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // driver for the read-only predictor-backtest handle

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// PredictorCalibrator calibrates event-driven predictor parameters using
// hit-rate feedback from the prediction_backtest table (F03).
type PredictorCalibrator struct {
	dbPath string
}

// NewPredictorCalibrator creates a calibrator backed by prediction_backtest
// data in the given SQLite database.
func NewPredictorCalibrator(dbPath string) *PredictorCalibrator {
	return &PredictorCalibrator{dbPath: dbPath}
}

func (pc *PredictorCalibrator) ParamNames() []string {
	return []string{
		"predictor_direction_threshold",
		"predictor_cf_score_tilt_weight",
		"predictor_mixed_weight_fraction",
		"predictor_backfill_discount_factor",
	}
}

func (pc *PredictorCalibrator) ParamBounds() map[string][2]float64 {
	return map[string][2]float64{
		"predictor_direction_threshold":      {0.1, 0.6},
		"predictor_cf_score_tilt_weight":     {0.1, 0.5},
		"predictor_mixed_weight_fraction":    {0.1, 0.5},
		"predictor_backfill_discount_factor": {0.5, 0.9},
	}
}

// NewPredictorEvaluator returns a scoring function that evaluates predictor
// parameters by reading hit-rate data from prediction_backtest.
//
// When >=30 non-neutral days exist, returns the direction hit rate as score.
// Otherwise returns 0.5 (neutral baseline) so Bayesian optimization can
// explore without penalty.
//
// Honor the wiring (#2123): this variant opens the **job-local SQLite** file at
// dbPath. Production must use NewPredictorEvaluatorWithStore with the
// backend-aware ledger.HistoricalStore, because in a postgres deployment the
// job-local artifact is empty and the evaluator silently returns the neutral
// 0.5 forever.
func NewPredictorEvaluator(dbPath string) func(cfg *config.ParametersConfig) (float64, error) {
	return func(cfg *config.ParametersConfig) (float64, error) {
		if score, ok := tryHitRateEval(dbPath); ok {
			return score, nil
		}
		return 0.5, nil
	}
}

// NewPredictorEvaluatorWithStore is the backend-aware variant (#2123): the score
// comes from a ledger.HistoricalStore that already honors ATLAS_STORE_BACKEND
// (postgres in production, sqlite in dev) — never from a hard-coded job-local
// path.
func NewPredictorEvaluatorWithStore(store ledger.HistoricalStore) func(cfg *config.ParametersConfig) (float64, error) {
	return func(cfg *config.ParametersConfig) (float64, error) {
		if score, ok := hitRateFromStore(store); ok {
			return score, nil
		}
		return 0.5, nil
	}
}

// hitRateFromStore computes the direction hit rate from prediction_backtest rows
// with is_synthetic=0 (LoadPredictionBacktestRange applies that filter), the
// same rule the legacy SQLite path used.
//
// Returns ok=false when fewer than minHitRateRows non-neutral days exist, so the
// caller falls back to the neutral 0.5 baseline and the optimizer cannot be
// steered by noise.
func hitRateFromStore(store ledger.HistoricalStore) (float64, bool) {
	if store == nil {
		return 0, false
	}
	rows, err := store.LoadPredictionBacktestRange(context.Background(), "", "", predictionBacktestWindow)
	if err != nil || len(rows) == 0 {
		return 0, false
	}
	total := 0
	hits := 0
	for _, r := range rows {
		if r.PredictedDirection == "neutral" || r.ActualDirection == "neutral" {
			continue
		}
		total++
		if r.Hit {
			hits++
		}
	}
	if total < minHitRateRows {
		return 0, false
	}
	return float64(hits) / float64(total), true
}

// minHitRateRows is the sample floor for a real (non-neutral) score.
//
// #2123 measurement: production prediction_backtest held 29 rows / 29 dates on
// 2026-09-29, so the floor is one row away from being crossed. The floor stays
// 30 (unchanged by this PR) — whether 30 is an adequate floor for the first live
// calibration is a separate question (see the PR body).
const minHitRateRows = 30

// predictionBacktestWindow mirrors the row limit the legacy path passed to
// LoadPredictionBacktestRange.
const predictionBacktestWindow = 90

// openSQLiteReadOnly 以 mode=ro 開啟**既有**的 SQLite 檔；檔案不存在時回錯。
//
// 為什麼不用 ledger.OpenSQLiteDB：它是 opens-or-creates（且帶 WAL pragma），
// 在生產宣告 postgres 的環境下會把被刪掉的 data/state/atlas.db 重建回來
// （#2107 的成因之一）。mode=ro 讓「不會寫」由 driver 保證，與既有的唯讀
// reader 同一個慣例（internal/stocktools/win_rate.go OpenWinRateDB、
// cmd/atlas-mcp/server/tools_stock_winrate.go）。
func openSQLiteReadOnly(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("predictor backtest db %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("predictor backtest db %s: %w", abs, err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open predictor backtest db %s: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open predictor backtest db %s: %w", abs, err)
	}
	return db, nil
}

// tryHitRateEval 讀 prediction_backtest 算命中率。
//
// 只讀既有檔案、**絕不建立**：檔案不存在時的語意與「表是空的」相同 ⇒ (0,false)，
// 與原本（OpenSQLiteDB 建出空檔再查失敗）的結果一致，但不再留下垃圾 artifact。
//
// 已知缺口（本 PR 不修，需另行核准）：生產的 prediction_backtest writer 已走
// PostgreSQL（calibration_tasks.go 的 prediction_backtest_reverse_write 用
// d.HistoricalStore），這個 reader 仍讀 job-local sqlite ⇒ 生產恆為 (0,false)
// （校準靜默 no-op）。把 reader 換成 backend-aware 的 HistoricalStore 會**改變
// 生產校準結果**（從恆定 0.5 變成真的依命中率調整參數），因此不在 #2107 的
// 「不改變既有生產行為」範圍內。
func tryHitRateEval(dbPath string) (float64, bool) {
	db, err := openSQLiteReadOnly(dbPath)
	if err != nil {
		return 0, false
	}
	defer func() { _ = db.Close() }()
	return hitRateFromStore(ledger.NewSQLiteHistoricalStore(db))
}

// CalibratePredictor runs Bayesian optimization on predictor parameters.
// It returns calibration results with before/after values and confidence.
//
// Legacy wiring: the evaluator reads the job-local SQLite file at dbPath. New
// callers must use CalibratePredictorWithStore (#2123).
func CalibratePredictor(ctx context.Context, dbPath string) (*config.CalibratorResult, error) {
	pc := NewPredictorCalibrator(dbPath)
	evaluator := NewPredictorEvaluator(dbPath)
	return config.CalibrateParameters(ctx, pc, evaluator, config.DefaultCalibrateConfig())
}

// CalibratePredictorWithStore is the backend-aware entry point (#2123): the
// evaluator reads prediction_backtest through the given HistoricalStore, which
// already follows ATLAS_STORE_BACKEND (postgres in production).
//
// Measured effect (2026-09-29, evidence in the PR): this makes the **score**
// truthful — the neutral 0.5 is replaced by the real hit rate once >=30
// non-neutral days exist. It does **not** change any parameter value today, but
// the reason matters and is easy to get wrong (I did, and an experiment caught
// it): the four ParamNames() below are **absent from config's parameterTable**,
// and CalibrateParameters hits `current, ok := ie.GetParameter(name); if !ok {
// continue }` — the names are silently skipped. It is NOT because the score
// surface is flat: a constant evaluator over a *resolvable* name does write an
// arbitrary tie-broken value (measured: darwinian_weight_min 0.3 -> 0.2057,
// -31%). See issue #2133 before making these names resolvable.
func CalibratePredictorWithStore(ctx context.Context, store ledger.HistoricalStore) (*config.CalibratorResult, error) {
	pc := NewPredictorCalibrator("")
	evaluator := NewPredictorEvaluatorWithStore(store)
	return config.CalibrateParameters(ctx, pc, evaluator, config.DefaultCalibrateConfig())
}

var _ = fmt.Sprintf
