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
func NewPredictorEvaluator(dbPath string) func(cfg *config.ParametersConfig) (float64, error) {
	return func(cfg *config.ParametersConfig) (float64, error) {
		if score, ok := tryHitRateEval(dbPath); ok {
			return score, nil
		}
		return 0.5, nil
	}
}

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

	store := ledger.NewSQLiteHistoricalStore(db)
	rows, err := store.LoadPredictionBacktestRange(
		context.Background(), "", "", 90,
	)
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
	if total < 30 {
		return 0, false
	}
	return float64(hits) / float64(total), true
}

// CalibratePredictor runs Bayesian optimization on predictor parameters.
// It returns calibration results with before/after values and confidence.
func CalibratePredictor(ctx context.Context, dbPath string) (*config.CalibratorResult, error) {
	pc := NewPredictorCalibrator(dbPath)
	evaluator := NewPredictorEvaluator(dbPath)
	return config.CalibrateParameters(ctx, pc, evaluator, config.DefaultCalibrateConfig())
}

var _ = fmt.Sprintf
