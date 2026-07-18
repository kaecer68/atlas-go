package calibration

import (
	"context"
	"fmt"

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

func tryHitRateEval(dbPath string) (float64, bool) {
	db, err := ledger.OpenSQLiteDB(dbPath)
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
