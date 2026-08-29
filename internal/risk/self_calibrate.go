package risk

import (
	"context"
	"fmt"
	"maps"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// SessionOutcome is a lightweight snapshot of a past trading session
// used by the self-calibration loop to compare risk decisions against reality.
type SessionOutcome struct {
	SessionID           string
	PortfolioValue      float64
	EndingCash          float64
	SectorExposures     map[string]float64
	PositionValues      map[string]float64
	Orders              []HistoricOrder
	ForwardReturnAvg    float64
	ForwardReturnStdDev float64
	Timestamp           time.Time
}

// HistoricOrder represents a recommendation that was acted on in a past session.
type HistoricOrder struct {
	Symbol        string
	Side          string
	Notional      float64
	Sector        string
	ForwardReturn float64
	Hit           bool
	WasBlocked    bool
}

// CalibrationProvider supplies historical session data for self-calibration.
type CalibrationProvider interface {
	RecentSessions(ctx context.Context, limit int) ([]SessionOutcome, error)
}

// CalibrationReport records what the self-calibration loop changed and why.
type CalibrationReport struct {
	Timestamp   time.Time         `json:"timestamp"`
	SessionSpan string            `json:"session_span"`
	Evaluated   int               `json:"orders_evaluated"`
	Changes     []ParameterChange `json:"changes"`
	Errors      []string          `json:"errors,omitempty"`
	Verdict     string            `json:"verdict"`
	Summary     string            `json:"summary"`
}

// ParameterChange records a single parameter adjustment with rationale.
type ParameterChange struct {
	Name       string  `json:"name"`
	Before     float64 `json:"before"`
	After      float64 `json:"after"`
	Rationale  string  `json:"rationale"`
	Confidence string  `json:"confidence"`
}

type replayResult struct {
	Blocked          bool
	ForwardReturn    float64
	OrderPrice       float64
	Quantity         int
	WouldHaveBlocked bool
}

// SelfCalibrate runs the autonomous risk gate calibration loop.
// It loads recent session data, replays pre-trade decisions against them,
// compares blocked/allowed decisions with actual forward returns, and
// uses Bayesian optimization to tune thresholds for the best outcome.
//
// The calibration cycle runs independently per RiskGate instance.
// Results are logged and surfaced via the CalibrationReport.
func (g *RiskGate) SelfCalibrate(ctx context.Context, provider CalibrationProvider, lookback int) (*CalibrationReport, error) {
	sessions, err := provider.RecentSessions(ctx, lookback)
	if err != nil {
		return nil, fmt.Errorf("self_calibrate: load sessions: %w", err)
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("self_calibrate: no sessions available")
	}

	report := &CalibrationReport{
		Timestamp: time.Now(),
		SessionSpan: fmt.Sprintf("%s → %s",
			sessions[len(sessions)-1].SessionID,
			sessions[0].SessionID),
	}

	report.Summary = "loaded sessions"

	var allResults []replayResult
	for _, s := range sessions {
		pf := buildPortfolioState(s)
		for _, o := range s.Orders {
			order := OrderIntent{
				Symbol:   o.Symbol,
				Side:     o.Side,
				Notional: o.Notional,
				Sector:   o.Sector,
			}
			decision, err := g.preTrade.Check(ctx, order, pf, string(g.mode))
			if err != nil {
				continue
			}
			blocked := decision.Verdict == VerdictBlock || decision.Verdict == VerdictHalt
			allResults = append(allResults, replayResult{
				Blocked:          blocked,
				ForwardReturn:    o.ForwardReturn,
				OrderPrice:       order.Price,
				Quantity:         order.Quantity,
				WouldHaveBlocked: blocked,
			})
			if !blocked {
				pf = applyOrderToState(pf, order)
			}
			report.Evaluated++
		}
	}

	if report.Evaluated == 0 {
		return nil, fmt.Errorf("self_calibrate: no orders to evaluate")
	}

	baseline := scoreThresholds(allResults)

	ie := config.NewInferenceEngine(config.GetParametersConfig())

	evaluator := func(cfg *config.ParametersConfig) (float64, error) {
		adjusted := replayWithThresholds(allResults, cfg)
		return scoreThresholds(adjusted), nil
	}

	paramNames := []string{
		"risk_max_position_size",
		"risk_max_daily_loss_pct",
	}

	optCfg := config.DefaultOptimizerConfig()
	optCfg.InitialPoints = 8
	optCfg.Iterations = 12

	result, err := ie.OptimizeBayesian(paramNames, evaluator, optCfg)
	if err != nil {
		return report, fmt.Errorf("self_calibrate: optimize: %w", err)
	}

	for _, name := range paramNames {
		current, _ := ie.GetParameter(name)
		best := result.ParamValues[name]

		if !validateCalibrationBounds(current, best) {
			fmt.Printf("self_calibrate: %s value %.6f rejected (outside [%.6f, %.6f])\n",
				name, best, current*0.3, current*3.0)
			continue
		}

		if math.Abs(best-current) < current*0.01 {
			continue
		}

		change := ParameterChange{
			Name:   name,
			Before: current,
			After:  best,
		}

		delta := (best - current) / current * 100
		change.Rationale = fmt.Sprintf(
			"baseline_score=%.4f, optimized_score=%.4f (%+.1f%% delta). %d sessions evaluated.",
			baseline, result.BestScore, delta, len(sessions),
		)
		change.Confidence = classifyDelta(delta, len(sessions))

		report.Changes = append(report.Changes, change)
		applyCalibrationChange(ie, name, best, report)
	}

	// Persist calibrated parameters to disk so they survive server restarts.
	if len(report.Changes) > 0 {
		now := time.Now()
		for _, name := range paramNames {
			switch name {
			case "risk_max_position_size", "risk_max_daily_loss_pct":
				config.SetRiskCalibrationMetadata(name, now, "bayesian_optimization")
			}
		}
		if p := config.GetParametersConfigPath(); p != "" {
			if err := config.SnapshotToBackup(p); err != nil {
				fmt.Printf("self_calibrate: snapshot_to_backup failed: %v\n", err)
			}
		}
		if err := config.GetParametersConfig().LockedSaveWithRollback(config.GetParametersConfigPath()); err != nil {
			// Non-fatal: calibration results remain valid in memory.
			fmt.Printf("self_calibrate: failed to persist parameters: %v\n", err)
		}
	}

	if len(report.Changes) == 0 {
		report.Verdict = "stable"
		report.Summary = fmt.Sprintf(
			"risk gate thresholds optimal (baseline=%.4f). no adjustments needed across %d sessions.",
			baseline, len(sessions),
		)
	} else {
		report.Verdict = "calibrated"
		report.Summary = fmt.Sprintf(
			"adjusted %d parameters based on %d session outcomes (baseline=%.4f, optimized=%.4f)",
			len(report.Changes), len(sessions), baseline, result.BestScore,
		)
	}

	return report, nil
}

func buildPortfolioState(s SessionOutcome) PortfolioState {
	pf := PortfolioState{
		TotalValue:     s.PortfolioValue,
		Cash:           s.EndingCash,
		Var95:          s.PortfolioValue * 0.02,
		SectorExposure: copyStrFloatMap(s.SectorExposures),
		Positions:      copyStrFloatMap(s.PositionValues),
	}
	if pf.TotalValue <= 0 {
		pf.TotalValue = 3_000_000
		pf.Cash = pf.TotalValue * 0.3
	}
	return pf
}

func applyOrderToState(pf PortfolioState, o OrderIntent) PortfolioState {
	pf.Cash -= o.Notional
	if pf.Positions == nil {
		pf.Positions = make(map[string]float64)
	}
	if pf.SectorExposure == nil {
		pf.SectorExposure = make(map[string]float64)
	}
	pf.Positions[o.Symbol] += o.Notional
	pf.SectorExposure[o.Sector] += o.Notional
	return pf
}

func copyStrFloatMap(src map[string]float64) map[string]float64 {
	if src == nil {
		return make(map[string]float64)
	}
	dst := make(map[string]float64, len(src))
	maps.Copy(dst, src)
	return dst
}

// scoreThresholds evaluates a set of replay results: higher score = better threshold set.
// Rewards: blocking orders with negative forward return (true positive).
// Penalizes: blocking orders with positive forward return (false positive),
//
//	allowing orders with negative forward return (false negative).
func scoreThresholds(results []replayResult) float64 {
	if len(results) == 0 {
		return 0
	}
	tp, fp, fn, tn := 0, 0, 0, 0
	for _, r := range results {
		wasBad := r.ForwardReturn < -0.02
		blocked := r.WouldHaveBlocked

		switch {
		case blocked && wasBad:
			tp++
		case blocked && !wasBad:
			fp++
		case !blocked && wasBad:
			fn++
		default:
			tn++
		}
	}

	total := float64(len(results))
	precision := div(float64(tp), float64(tp+fp))
	recall := div(float64(tp), float64(tp+fn))

	f1 := 2 * div(precision*recall, precision+recall)
	interceptPenalty := float64(tp+fp) / total

	if interceptPenalty > 0.5 {
		return f1 - 2.0
	}
	return f1 - interceptPenalty*0.3
}

func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// replayWithThresholds re-evaluates a set of orders against different parameter thresholds,
// returning the results with updated WouldHaveBlocked flags.
func replayWithThresholds(results []replayResult, cfg *config.ParametersConfig) []replayResult {
	maxPosition := cfg.Risk.MaxPositionSize.Value
	maxDailyLoss := cfg.Risk.MaxDailyLossPct.Value

	adjusted := make([]replayResult, len(results))
	copy(adjusted, results)

	for i, r := range results {
		notional := r.OrderPrice * float64(r.Quantity)
		posPct := notional / 3_000_000
		lossPct := notional * 1.5 / 3_000_000

		adjusted[i].WouldHaveBlocked = posPct > maxPosition || lossPct > maxDailyLoss
	}
	return adjusted
}

// applyCalibrationChange writes a single optimized parameter through the
// inference engine and surfaces any write failure into the report rather
// than silently dropping it. Calibration continues for the remaining
// parameters; one bad write does not poison the whole cycle.
func applyCalibrationChange(ie *config.InferenceEngine, name string, value float64, report *CalibrationReport) {
	if err := ie.SetParameter(name, value); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", name, err))
	}
}

// detectOscillation returns true when the recent history alternates sign
// more often than it continues — i.e., consecutive Bayesian runs are
// pushing the parameter back and forth instead of converging. Used to
// refuse changes that would destabilize the risk gate.
func detectOscillation(history []float64) bool {
	if len(history) < 3 {
		return false
	}
	signChanges := 0
	intervals := len(history) - 1
	for i := 2; i < len(history); i++ {
		delta := history[i] - history[i-1]
		if delta == 0 {
			continue
		}
		prev := history[i-1] - history[i-2]
		if prev == 0 {
			continue
		}
		if (delta > 0) != (prev > 0) {
			signChanges++
		}
	}
	return signChanges*2 >= intervals
}

func classifyDelta(deltaPct float64, nSessions int) string {
	switch {
	case nSessions >= 30 && math.Abs(deltaPct) > 5:
		return "high"
	case nSessions >= 10 && math.Abs(deltaPct) > 2:
		return "medium"
	default:
		return "low"
	}
}

// validateCalibrationBounds checks whether the proposed value is within
// [current*0.3, current*3.0]. When current is zero the check is skipped
// (no meaningful bound to compare against). This prevents the bug class
// where repeated Bayesian optimization converges to near-zero values that
// are 20× outside any sane operating range.
func validateCalibrationBounds(current, proposed float64) bool {
	// Absolute floor: relative bounds (current*0.3 ~ current*3.0) alone let
	// the optimizer drift to absurdly small values (e.g. max_position_size
	// 0.15 → … → 9.14e-6, a 0.0009% max position) by shrinking ≤3x per round.
	// This floor matches the documented sane scale (15% position / 3% daily
	// loss): anything below 0.5% is a pathological drift, not a genuine
	// optimum, so reject it regardless of current.
	const absFloor = 0.005 // 0.5% — sane lower bound for both params

	// Relative bounds stay as the primary acceptance rule.
	if current == 0 {
		return proposed >= absFloor
	}
	lower := current * 0.3
	upper := current * 3.0
	// Reject any proposed value that has drifted below the absolute floor,
	// regardless of current (prevents multi-round shrink-to-zero).
	if proposed < absFloor {
		return false
	}
	return proposed >= lower && proposed <= upper
}
