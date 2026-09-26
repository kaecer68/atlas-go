package risk

import (
	"context"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
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
	// Rejected lists proposals the sanity floor refused. A blocked drift is
	// reported with the same weight as an applied change: "nothing changed"
	// must not be the only trace of a guard that just stopped the optimizer
	// from walking a risk limit down.
	Rejected []ParameterRejection `json:"rejected,omitempty"`
	Errors   []string             `json:"errors,omitempty"`
	Verdict  string               `json:"verdict"`
	Summary  string               `json:"summary"`
}

// ParameterRejection records a proposed calibration value that was refused
// before it could be applied (see calibrationSanityFloor).
type ParameterRejection struct {
	Name     string  `json:"name"`
	Current  float64 `json:"current"`
	Proposed float64 `json:"proposed"`
	Floor    float64 `json:"floor"`
	Reason   string  `json:"reason"`
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

		if !validateCalibrationBoundsForParam(name, current, best) {
			floor := calibrationSanityFloorFor(name)
			reason := fmt.Sprintf(
				"proposed %.6f is below the sanity floor %.6f for this parameter (relative window was [%.6f, %.6f])",
				best, floor, current*0.3, current*3.0)
			report.Rejected = append(report.Rejected, ParameterRejection{
				Name:     name,
				Current:  current,
				Proposed: best,
				Floor:    floor,
				Reason:   reason,
			})
			logging.Warn("self_calibrate", "calibration_rejected_sanity_floor",
				logging.FStr("param", name),
				logging.FFloat64("current", current),
				logging.FFloat64("proposed", best),
				logging.FFloat64("floor", floor))
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
		if len(report.Rejected) > 0 {
			report.Summary += fmt.Sprintf(
				" %d proposal(s) rejected by the sanity floor: %s.",
				len(report.Rejected), describeRejections(report.Rejected))
		}
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
//
// This is the generic, parameter-agnostic rule. Production callers must use
// validateCalibrationBoundsForParam, which adds the per-parameter sanity floor
// that this function cannot express (see calibrationSanityFloor).
func validateCalibrationBounds(current, proposed float64) bool {
	// Absolute floor: relative bounds (current*0.3 ~ current*3.0) alone let
	// the optimizer drift to absurdly small values (e.g. max_position_size
	// 0.15 → … → 9.14e-6, a 0.0009% max position) by shrinking ≤3x per round.
	// This floor matches the documented sane scale (15% position / 3% daily
	// loss): anything below 0.5% is a pathological drift, not a genuine
	// optimum, so reject it regardless of current.
	const absFloor = defaultCalibrationFloor

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

// defaultCalibrationFloor is the parameter-agnostic catch-all floor (0.5%):
// nothing in the risk parameter space is meaningfully below it. Tunables with a
// documented operating envelope carry a named, higher floor in
// calibrationSanityFloor.
const defaultCalibrationFloor = 0.005

// calibrationSanityFloor is the absolute floor per tunable: the lowest value
// that is still a genuine operating point rather than an artifact of the
// optimizer's surrogate score.
//
// Why a floor is needed at all (FU-20260926-07): the relative window
// [current*0.3, current*3.0] is a per-round rate limit, not a guard. A value
// that shrinks by less than 3× stays inside the window *every* round, so the
// optimizer can walk a risk limit down indefinitely. That is exactly what
// production did: risk.max_daily_loss_pct 0.03 → 0.0108 (0.36×, inside
// [0.009, 0.09]) and risk.max_position_size 0.15 → 0.054 (0.36×, inside
// [0.045, 0.45]). Every single step was "allowed"; only the accumulated drift
// was pathological. The previous shared 0.5% floor did not stop it because
// 0.0108 > 0.005 — it only bounded where the walk could end, not that it
// happened.
//
// The floors below are not invented here; they are anchored to values already
// documented in this repository, so the guard can be reviewed against the
// system's own charter:
//
//   - risk_max_position_size = 0.12 — the most conservative position size the
//     system itself documents (engine.strategy_evolution.configs.value.cautious
//     .max_position_size in configs/parameters.json). Below it the gate has left
//     the documented operating envelope. The SSOT value is 0.15, so the loop can
//     still tighten once (0.15 → 0.12) but cannot walk to 0.054.
//   - risk_max_daily_loss_pct = 0.03 — the SSOT value itself ("3% max daily
//     loss"). A tighter daily loss limit halts trading earlier. This loop scores
//     replayed orders, so it can measure "was blocking right?" but it cannot
//     price the cost of a spurious halt; a tightening is therefore unvalidated
//     by construction and must be a deliberate charter edit in
//     configs/parameters.json, not an automatic drift.
//
// Why a floor and not an evidence ratchet ("accept a sub-floor value only after
// N consecutive rounds")? A ratchet still ends below the sane value — it only
// takes longer — and it would need per-parameter state that outlives the
// process, i.e. exactly the kind of derived state this change is trying to make
// legible. The floor is stateless, deterministic and testable, and the escape
// hatch is explicit (edit the SSOT value, which is a reviewed diff).
var calibrationSanityFloor = map[string]float64{
	"risk_max_position_size":  0.12,
	"risk_max_daily_loss_pct": 0.03,
}

// calibrationSanityFloorFor returns the sanity floor for a tunable. Tunables
// without a named floor fall back to defaultCalibrationFloor (0.5%), i.e. the
// historical behavior.
func calibrationSanityFloorFor(name string) float64 {
	if f, ok := calibrationSanityFloor[name]; ok {
		return f
	}
	return defaultCalibrationFloor
}

// validateCalibrationBoundsForParam reports whether `proposed` is an acceptable
// next value for the named tunable whose live value is `current`. Two rules,
// both must hold:
//
//  1. the absolute sanity floor for that parameter (never below it), and
//  2. the per-round relative window [current*0.3, current*3.0].
//
// Recovery: when the live value is itself below the floor (state written by a
// build that predates the floor, i.e. an already-drifted deployment), the
// relative window is meaningless — 0.3× of a pathological value cannot express
// a sane bound — so the floor becomes the baseline and any proposal inside
// [floor, floor*3] is accepted. That lets a drifted parameter climb back to a
// sane value in one round instead of being frozen at the drifted value.
func validateCalibrationBoundsForParam(name string, current, proposed float64) bool {
	floor := calibrationSanityFloorFor(name)
	if proposed < floor {
		return false
	}
	if current < floor {
		return proposed <= floor*3.0
	}
	return validateCalibrationBounds(current, proposed)
}

// describeRejections renders rejections as "name proposed (floor)" for the
// calibration report summary.
func describeRejections(rejections []ParameterRejection) string {
	parts := make([]string, 0, len(rejections))
	for _, r := range rejections {
		parts = append(parts, fmt.Sprintf("%s proposed %.6f < floor %.6f", r.Name, r.Proposed, r.Floor))
	}
	return strings.Join(parts, "; ")
}
