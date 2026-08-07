package ledger

import (
	"context"
	"fmt"
	"time"
)

// SyncPredictionBacktestFromEventFlow reads T+1-reconciled event-flow
// predictions from the JSONL ledger (those with ActualCapturedAt != nil)
// and upserts them into the prediction_backtest SQLite table with
// is_synthetic=0. The hit rate is computed as the sign agreement of
// DirectionSign and ActualSign. Returns the number of rows upserted.
//
// This closes the calibration feedback loop: without it, the only writer
// of prediction_backtest is cmd/backtest-event-flow (replay fixtures,
// is_synthetic=1) and predictor_calibrate (24h task) filters out
// is_synthetic=1, so it always sees zero real rows and falls back to a
// neutral 0.5 score. With this task, the calibrator gets real production
// hit-rate feedback and can downgrade parameters when the model degrades
// (product positioning §8).
//
// Idempotency: a re-run of the same day is safe — UpsertPredictionBacktest
// replaces existing rows by (date) primary key, so the latest reconciliation
// wins. Historical replays (is_synthetic=1) are untouched because the task
// only writes is_synthetic=0.
func SyncPredictionBacktestFromEventFlow(
	ctx context.Context,
	predictionStore EventFlowPredictionStore,
	historicalStore HistoricalStore,
	workDir string,
) (int, error) {
	if predictionStore == nil {
		return 0, fmt.Errorf("prediction store is nil")
	}
	if historicalStore == nil {
		return 0, fmt.Errorf("historical store is nil")
	}

	// The JSONL prediction store uses 1 prediction per trading day. Read a
	// generous window (180 records ≈ 9 months) so the calibrator can blend
	// recent + older history even after the first few months of operation.
	const window = 180
	records, err := predictionStore.LoadRecentPredictions(window)
	if err != nil {
		return 0, fmt.Errorf("load predictions: %w", err)
	}

	upserted := 0
	for _, r := range records {
		if r.ActualCapturedAt == nil {
			continue // not yet T+1-reconciled
		}
		row := r.toPredictionBacktestRow()
		if err := historicalStore.UpsertPredictionBacktest(ctx, row); err != nil {
			return upserted, fmt.Errorf("upsert %s: %w", row.Date, err)
		}
		upserted++
	}
	return upserted, nil
}

// toPredictionBacktestRow converts a reconciled event-flow prediction into
// the shape consumed by predictor_calibrate (SQLite prediction_backtest).
// Hit is the sign agreement of DirectionSign (predicted) and ActualSign
// (T+1 realized): same sign = inflow/outflow hit, opposite sign = miss.
// Neutral predictions or neutral actuals are excluded by the calibrator
// downstream (tryHitRateEval skips both "neutral" directions).
func (r EventFlowPredictionRecord) toPredictionBacktestRow() PredictionBacktestRow {
	date := ""
	if !r.PredictedAt.IsZero() {
		date = r.PredictedAt.UTC().Format("2006-01-02")
	}
	direction := directionFromSign(r.DirectionSign)
	actual := directionFromSign(r.ActualSign)
	hit := (r.DirectionSign > 0 && r.ActualSign > 0) ||
		(r.DirectionSign < 0 && r.ActualSign < 0)
	_ = hit // assigned to PredictionBacktestRow.Hit below
	return PredictionBacktestRow{
		Date:                  date,
		PredictedDirection:    direction,
		PredictedConfidence:   absFloat(r.Confidence),
		ActualDirection:       actual,
		ActualCapitalFlowChan: r.ActualSign,
		Hit:                   hit,
		ModelVersion:          "eventflow-realtime",
		CapturedAt:            r.CapturedAtUTC(),
		IsSynthetic:           0, // production reverse-write
	}
}

// directionFromSign maps a signed magnitude to its canonical direction
// label ("inflow" / "outflow" / "neutral"). 0 is treated as neutral.
// A neutral prediction against a non-zero actual is a miss downstream
// (tryHitRateEval skips "neutral" on either side), which is the intended
// semantic — the model said "no signal" and reality was anything.
func directionFromSign(sign float64) string {
	switch {
	case sign > 0:
		return "inflow"
	case sign < 0:
		return "outflow"
	default:
		return "neutral"
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// CapturedAtUTC returns the T+1 reconciliation timestamp (when the
// reconciler filled the actual), with a zero-value fallback for
// not-yet-reconciled records (callers must filter those out).
func (r EventFlowPredictionRecord) CapturedAtUTC() time.Time {
	if r.ActualCapturedAt == nil {
		return time.Time{}
	}
	return r.ActualCapturedAt.UTC()
}
