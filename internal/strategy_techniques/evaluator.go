// Package strategy_techniques — ConditionEvaluator evaluates strategy conditions
// against historical macro snapshots and computes per-strategy hit rates.
//
// Phase 2 of the attribution loop (#1259): replacing the dummy zero-value
// attribution markers with genuine strategy performance data.
//
// Design rationale:
//   - Strategy conditions are rule-based (Field/Operator/Value), not ML-driven.
//     The correct evaluation is against macro data history, not the ML backtest.
//   - The agent pipeline (recommendation → outcome → hit) doesn't yet carry
//     strategy provenance (planned Wave 4). Until then, standalone macro-based
//     evaluation gives the cleanest, most interpretable signal.
//   - "Hit" = strategy condition triggered AND forward market moved in the
//     strategy's direction. forwardLookback controls how many days ahead
//     we check TAIEX return.
package strategy_techniques

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// EvalResult is the output of evaluating a StrategyFrame against
// historical macro data.
type EvalResult struct {
	StrategyID string `json:"strategy_id"`
	// TotalTests 是進入命中率分母的觸發樣本數：只有 Direction=up/down
	// （以及未設定方向的預設路徑）會累計。Direction=volatile 的樣本不在此
	// 分母內（見 VolatileTests）。
	TotalTests int `json:"total_tests"`
	TotalHits  int `json:"total_hits"`
	// HitRate 只在 DirectionScored 為 true 時有方向意義。
	HitRate float64 `json:"hit_rate"`
	// VolatileTests 是 Direction=volatile 且條件成立的觸發樣本數
	// （issue #1944 / I16）。volatile 策略預期的是「高波動」，不是漲跌，
	// 本評估器的方向命中率對它沒有判定依據，因此這些樣本既不進 TotalTests
	// 分母、也不計為 miss —— 在修正前它們等於「必然落空」，會把對外
	// hit_rate 壓成 0。
	VolatileTests int `json:"volatile_tests"`
	// DirectionScored 為 false 表示本次評估沒有任何可評分（up/down）樣本，
	// 因此 HitRate 在語意上是「未定義」而非「零命中率」。呼叫端必須用這個
	// 旗標區分兩者，不得把 HitRate=0 當成量測結果。
	DirectionScored bool `json:"direction_scored"`
}

// ConditionEvaluator evaluates StrategyFrame conditions against
// historical MacroDataSnapshot data and computes hit rates.
//
// Zero-allocation design: the evaluator has no fields; all state is
// passed via function parameters. This keeps it trivially testable
// and avoids any lifecycle management.
type ConditionEvaluator struct{}

// NewConditionEvaluator returns a ready-to-use evaluator.
func NewConditionEvaluator() *ConditionEvaluator {
	return &ConditionEvaluator{}
}

// EvaluateReturns evaluates a StrategyFrame against historical macro snapshots and
// returns the strategy return series, the corresponding TAIEX return series, and
// the total number of trigger days. Strategy return is derived from forward
// TAIEX return adjusted by the frame's expected direction (up/down/volatile).
// Returns empty slices when there are insufficient snapshots or no triggers.
func (e *ConditionEvaluator) EvaluateReturns(
	frame StrategyFrame,
	snapshots []marketdata.MacroDataSnapshot,
	forwardLookback int,
) (strategyReturns, taiexReturns []float64, totalTests int) {
	if len(snapshots) < forwardLookback+1 {
		return nil, nil, 0
	}

	maxIdx := len(snapshots) - forwardLookback
	for i := range maxIdx {
		if !e.matchesAll(frame.Conditions, snapshots[i]) {
			continue
		}
		totalTests++

		// Missing TAIEX readings (e.g. taiex_index channel failures) must not
		// silently drop the sample: resolve the most recent valid reading at
		// or before the trigger/forward day instead of skipping.
		current := resolveTAIEXValue(snapshots, i)
		forward := resolveTAIEXValue(snapshots, i+forwardLookback)
		if current == 0 || forward == 0 {
			continue
		}

		marketReturn := (forward - current) / current
		var strategyReturn float64
		switch frame.Direction {
		case DirectionUp:
			strategyReturn = marketReturn
		case DirectionDown:
			strategyReturn = -marketReturn
		case DirectionVolatile:
			// 波動率策略的報酬代理：取絕對值。這與 Evaluate 的方向命中率是
			// 兩個不同的量（issue #1944 / I16）：Evaluate 不評分 volatile
			// 樣本，這裡仍保留 |return| 供 Sharpe 類指標使用。
			strategyReturn = math.Abs(marketReturn)
		default:
			strategyReturn = marketReturn
		}

		strategyReturns = append(strategyReturns, strategyReturn)
		taiexReturns = append(taiexReturns, marketReturn)
	}

	return strategyReturns, taiexReturns, totalTests
}

// resolveTAIEXValue returns the most recent valid TAIEX reading at or before
// index i in snapshots (walking backwards when the taiex_index channel failed
// on a given day and the snapshot has no reading). Returns 0 when no valid
// reading exists up to i.
func resolveTAIEXValue(snapshots []marketdata.MacroDataSnapshot, i int) float64 {
	for j := i; j >= 0; j-- {
		if v := snapshots[j].TAIEX.Value; v != 0 {
			return v
		}
	}
	return 0
}

// Evaluate checks all conditions of a strategy against each historical
// macro snapshot in order. For each snapshot where ALL conditions match
// (AND semantics), it checks the forward TAIEX return over
// forwardLookback trading days and compares direction.
//
// Parameters:
//   - frame: the strategy to evaluate
//   - snapshots: ordered list of macro snapshots (oldest first)
//   - forwardLookback: number of subsequent snapshots to check for
//     forward return (e.g. 1 = next trading day). Must be ≥ 1.
//
// Returns EvalResult with aggregated hit/miss counts.
//
// Direction semantics (issue #1944 / I16):
//   - DirectionUp / DirectionDown: a trigger day counts toward TotalTests; a hit
//     requires the forward return sign to match.
//   - DirectionVolatile: the frame predicts high volatility, not a direction, so
//     the directional hit rate has no basis to score it. Those triggers are
//     counted in VolatileTests only — they are NOT added to TotalTests and NOT
//     counted as misses.
//   - Any other (unset/unknown) Direction falls back to the up/down path so
//     behavior for existing registries is unchanged.
//
// Edge cases:
//   - fewer than forwardLookback+1 snapshots → skip evaluation (return 0/0/0)
//   - TAIEX value is zero at the forward point → trigger still counts toward
//     TotalTests for up/down frames (unchanged legacy semantics) but is not a hit
//   - condition with unknown operator → skip (log warning in caller)
func (e *ConditionEvaluator) Evaluate(
	frame StrategyFrame,
	snapshots []marketdata.MacroDataSnapshot,
	forwardLookback int,
) EvalResult {
	result := EvalResult{StrategyID: frame.ID}

	if len(snapshots) < forwardLookback+1 {
		return result
	}

	maxIdx := len(snapshots) - forwardLookback
	for i := range maxIdx {
		current := snapshots[i]
		if !e.matchesAll(frame.Conditions, current) {
			continue
		}

		// I16: volatile frames predict high volatility, not a direction. Their
		// triggers are counted separately and never enter the hit-rate
		// denominator, so they cannot be scored as automatic misses.
		if frame.Direction == DirectionVolatile {
			result.VolatileTests++
			continue
		}

		result.TotalTests++

		// Check forward return against strategy direction.
		forward := snapshots[i+forwardLookback]
		currentTAIEX := current.TAIEX.Value
		forwardTAIEX := forward.TAIEX.Value
		if currentTAIEX == 0 || forwardTAIEX == 0 {
			continue
		}

		returnPct := (forwardTAIEX - currentTAIEX) / currentTAIEX
		isUpward := returnPct > 0

		if (frame.Direction == DirectionUp && isUpward) ||
			(frame.Direction == DirectionDown && !isUpward) {
			result.TotalHits++
		}
	}

	if result.TotalTests > 0 {
		result.HitRate = float64(result.TotalHits) / float64(result.TotalTests)
		result.DirectionScored = true
	}

	return result
}

// matchesAll returns true when every condition is satisfied by the snapshot.
func (e *ConditionEvaluator) matchesAll(conditions []Condition, snap marketdata.MacroDataSnapshot) bool {
	for _, c := range conditions {
		if !e.checkCondition(c, snap) {
			return false
		}
	}
	return len(conditions) > 0
}

// checkCondition resolves a single condition against a snapshot.
func (e *ConditionEvaluator) checkCondition(c Condition, snap marketdata.MacroDataSnapshot) bool {
	fieldValue, ok := resolveField(snap, c.Field)
	if !ok {
		return false
	}

	switch c.Operator {
	case "gt":
		return fieldValue > c.Value
	case "lt":
		return fieldValue < c.Value
	case "gte":
		return fieldValue >= c.Value
	case "lte":
		return fieldValue <= c.Value
	case "eq":
		return fieldValue == c.Value
	case "neq":
		return fieldValue != c.Value
	default:
		// Operators cross_above / cross_below need adjacent-snapshot
		// comparison and are not yet supported here.
		// Caller should pre-filter or log a warning.
		return false
	}
}

// resolveField maps a condition field name to the corresponding
// MacroDataPoint.Value in the snapshot. Returns the value and a bool
// indicating whether the field was found.
//
// Field names match MacroDataSnapshot JSON tags (e.g. "RetailMarginBalance"
// → snap.RetailMarginBalance.Value).
func resolveField(snap marketdata.MacroDataSnapshot, field string) (float64, bool) {
	switch field {
	case "US10Y":
		return snap.US10Y.Value, snap.US10Y.Symbol != ""
	case "DXY":
		return snap.DXY.Value, snap.DXY.Symbol != ""
	case "VIX":
		return snap.VIX.Value, snap.VIX.Symbol != ""
	case "USD_TWD":
		return snap.USD_TWD.Value, snap.USD_TWD.Symbol != ""
	case "Oil":
		return snap.Oil.Value, snap.Oil.Symbol != ""
	case "Gold":
		return snap.Gold.Value, snap.Gold.Symbol != ""
	case "JPY":
		return snap.JPY.Value, snap.JPY.Symbol != ""
	case "ForeignInvestorNet":
		return snap.ForeignInvestorNet.Value, snap.ForeignInvestorNet.Symbol != ""
	case "DomesticFundNet":
		return snap.DomesticFundNet.Value, snap.DomesticFundNet.Symbol != ""
	case "DealerNet":
		return snap.DealerNet.Value, snap.DealerNet.Symbol != ""
	case "ForeignFuturesOINet":
		return snap.ForeignFuturesOINet.Value, snap.ForeignFuturesOINet.Symbol != ""
	case "GovernmentNet":
		return snap.GovernmentNet.Value, snap.GovernmentNet.Symbol != ""
	case "ExportElectronics":
		return snap.ExportElectronics.Value, snap.ExportElectronics.Symbol != ""
	case "RetailMarginBalance":
		return snap.RetailMarginBalance.Value, snap.RetailMarginBalance.Symbol != ""
	case "RetailShortBalance":
		return snap.RetailShortBalance.Value, snap.RetailShortBalance.Symbol != ""
	case "TSMCRevenue":
		return snap.TSMCRevenue.Value, snap.TSMCRevenue.Symbol != ""
	case "SPXIndex", "SPX":
		return snap.SPXIndex.Value, snap.SPXIndex.Symbol != ""
	case "NDXIndex", "NDX":
		return snap.NDXIndex.Value, snap.NDXIndex.Symbol != ""
	case "DJIIndex", "DJI":
		return snap.DJIIndex.Value, snap.DJIIndex.Symbol != ""
	case "SOXIndex", "SOX":
		return snap.SOXIndex.Value, snap.SOXIndex.Symbol != ""
	case "DRAMSpotPrice":
		return snap.DRAMSpotPrice.Value, snap.DRAMSpotPrice.Symbol != ""
	case "TaiwanSemiIndex":
		return snap.TaiwanSemiIndex.Value, snap.TaiwanSemiIndex.Symbol != ""
	case "CoWoSUtilization":
		return snap.CoWoSUtilization.Value, snap.CoWoSUtilization.Symbol != ""
	case "CapexGrowth":
		return snap.CapexGrowth.Value, snap.CapexGrowth.Symbol != ""
	case "CPIYoY":
		return snap.CPIYoY.Value, snap.CPIYoY.Symbol != ""
	case "Bdi":
		return snap.Bdi.Value, snap.Bdi.Symbol != ""
	case "Silver":
		return snap.Silver.Value, snap.Silver.Symbol != ""
	case "Copper":
		return snap.Copper.Value, snap.Copper.Symbol != ""
	case "TSMADR":
		return snap.TSMADR.Value, snap.TSMADR.Symbol != ""
	case "NVDA":
		return snap.NVDA.Value, snap.NVDA.Symbol != ""
	case "AAPL":
		return snap.AAPL.Value, snap.AAPL.Symbol != ""
	case "MSFT":
		return snap.MSFT.Value, snap.MSFT.Symbol != ""
	case "TAIEX":
		return snap.TAIEX.Value, snap.TAIEX.Symbol != ""
	case "HistoricalVolatility":
		return snap.HistoricalVolatility.Value, snap.HistoricalVolatility.Symbol != ""
	default:
		return 0, false
	}
}

// FormatEvalError wraps an evaluation failure with context.
func FormatEvalError(strategyID string, err error) error {
	return fmt.Errorf("strategy_techniques: evaluate %s: %w", strategyID, err)
}

// LoadSnapshotsFromDir reads dated MacroDataSnapshot JSON files from a
// directory. Returns snapshots sorted by trading date (ascending).
// Non-date files (latest.json, metadata.json) and parse errors are
// silently skipped. Returns an empty slice if the directory is empty
// or unreadable.
func LoadSnapshotsFromDir(dir string) ([]marketdata.MacroDataSnapshot, error) {
	// Delegate to the existing loader in monitoring/service to avoid
	// duplicating the date-parsing + file-reading logic.
	// This is a convenience wrapper for callers that don't have
	// access to MacroService.
	return loadSnapshotsFromDir(dir)
}

// MacroSnapshotEvaluator wraps ConditionEvaluator with pre-loaded macro
// snapshots. It satisfies the autobacktest.ConditionEvaluator interface
// without exposing marketdata to the autobacktest package.
type MacroSnapshotEvaluator struct {
	eval      *ConditionEvaluator
	snapshots []marketdata.MacroDataSnapshot
	lookback  int
}

// NewMacroSnapshotEvaluator creates an evaluator backed by historical
// macro snapshots. forwardLookback controls how many trading days ahead
// to check for forward return (1 = next day). snapshots must be in
// chronological order (oldest first).
func NewMacroSnapshotEvaluator(
	snapshots []marketdata.MacroDataSnapshot,
	forwardLookback int,
) *MacroSnapshotEvaluator {
	return &MacroSnapshotEvaluator{
		eval:      NewConditionEvaluator(),
		snapshots: snapshots,
		lookback:  forwardLookback,
	}
}

// Evaluate satisfies the condition evaluator interface used by
// autobacktest.Runner. It returns nil when the strategy has no
// conditions (edge case).
func (e *MacroSnapshotEvaluator) Evaluate(frame StrategyFrame) *EvalResult {
	if len(frame.Conditions) == 0 || len(e.snapshots) == 0 {
		return nil
	}
	result := e.eval.Evaluate(frame, e.snapshots, e.lookback)
	return &result
}
