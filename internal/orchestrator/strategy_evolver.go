package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// StrategyState represents the current state of the investment strategy
type StrategyState int

const (
	StrategyNormal StrategyState = iota
	StrategyCautious
	StrategyDefensive
	StrategyHedged
	StrategySuspended
)

func (s StrategyState) String() string {
	switch s {
	case StrategyNormal:
		return "normal"
	case StrategyCautious:
		return "cautious"
	case StrategyDefensive:
		return "defensive"
	case StrategyHedged:
		return "hedged"
	case StrategySuspended:
		return "suspended"
	default:
		return "unknown"
	}
}

// StrategyEvolution represents a change in strategy state
type StrategyEvolution struct {
	FromState StrategyState `json:"from_state"`
	ToState   StrategyState `json:"to_state"`
	Reason    string        `json:"reason"`
	Timestamp time.Time     `json:"timestamp"`
}

// StrategyEvolver manages strategy evolution based on macro and structural conditions
type StrategyEvolver struct {
	history           []StrategyEvolution
	currentState      StrategyState
	lastEvolutionTime time.Time
	cooldownPeriod    time.Duration

	// SA08: persistent sector allocation policy storage.
	// When non-nil, ApplySectorRotation writes real snapshots
	// instead of returning a no-op true.
	closureStore    sectorallocation.ClosureStore
	sessionResolver TradingSessionResolver
	weightEngine    sectorallocation.WeightEngine
}

// NewStrategyEvolver creates a new strategy evolver
func NewStrategyEvolver() *StrategyEvolver {
	return NewStrategyEvolverWithConfig(config.GetParametersConfig().Engine.StrategyEvolution.ToConfig())
}

// NewStrategyEvolverWithConfig creates a new strategy evolver with custom config
func NewStrategyEvolverWithConfig(cfg config.StrategyEvolutionConfig) *StrategyEvolver {
	return &StrategyEvolver{
		history:        make([]StrategyEvolution, 0),
		currentState:   StrategyNormal,
		cooldownPeriod: cfg.GetCooldownDuration(),
	}
}

// WithClosureStore sets the persistent policy store for SA08.
func (e *StrategyEvolver) WithClosureStore(store sectorallocation.ClosureStore) *StrategyEvolver {
	e.closureStore = store
	return e
}

// WithSessionResolver sets the trading session resolver for SA08.
func (e *StrategyEvolver) WithSessionResolver(resolver TradingSessionResolver) *StrategyEvolver {
	e.sessionResolver = resolver
	return e
}

// WithSectorWeightEngine sets the WeightEngine for computing projected targets (SA08).
func (e *StrategyEvolver) WithSectorWeightEngine(engine sectorallocation.WeightEngine) *StrategyEvolver {
	e.weightEngine = engine
	return e
}

// Evaluate determines if strategy should evolve based on current conditions
func (e *StrategyEvolver) Evaluate(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
	drawdownDecision *risk.MacroAwareDrawdownDecision,
) *StrategyEvolution {
	newState := e.determineState(macroAssessment, structuralAssessment, drawdownDecision)

	if newState == e.currentState {
		return nil // No change needed
	}

	// Check cooldown period — bypass for emergency escalations
	if time.Since(e.lastEvolutionTime) < e.cooldownPeriod && !e.isEmergencyEscalation(newState, drawdownDecision) {
		return nil // Too soon to evolve
	}

	evolution := &StrategyEvolution{
		FromState: e.currentState,
		ToState:   newState,
		Reason:    e.buildEvolutionReason(macroAssessment, structuralAssessment, drawdownDecision),
		Timestamp: time.Now(),
	}

	// Apply evolution
	e.history = append(e.history, *evolution)
	e.currentState = newState
	e.lastEvolutionTime = time.Now()

	return evolution
}

// isEmergencyEscalation returns true when the strategy transition is urgent enough
// to bypass the cooldown period. This covers:
//   - Any transition to Suspended (drawdown >= severe)
//   - Jump of 2+ levels (e.g., Normal → Defensive or Cautious → Suspended)
func (e *StrategyEvolver) isEmergencyEscalation(newState StrategyState, drawdown *risk.MacroAwareDrawdownDecision) bool {
	if newState == StrategySuspended {
		return true
	}
	// On drawdown severe or above, always treat as emergency
	if drawdown.Action >= risk.DrawdownSevere {
		return true
	}
	// Jump of 2 or more levels is an emergency
	delta := int(newState) - int(e.currentState)
	if delta < 0 {
		delta = -delta
	}
	return delta >= 2
}

func (e *StrategyEvolver) determineState(
	macro *narrative.MacroRiskAssessment,
	structural *narrative.StructuralTrendAssessment,
	drawdown *risk.MacroAwareDrawdownDecision,
) StrategyState {
	// Emergency or severe drawdown suspends strategy
	if drawdown.Action >= risk.DrawdownSevere {
		return StrategySuspended
	}

	// Check if structural trends can support hedged strategy during high risk
	if macro.Level >= narrative.MacroRiskOrange && structural.ShouldOverrideRisk {
		return StrategyHedged
	}

	// Determine state based on macro risk level
	switch macro.Level {
	case narrative.MacroRiskGreen:
		return StrategyNormal
	case narrative.MacroRiskYellow:
		return StrategyCautious
	case narrative.MacroRiskOrange:
		return StrategyDefensive
	case narrative.MacroRiskRed:
		return StrategyDefensive
	default:
		return StrategyNormal
	}
}

func (e *StrategyEvolver) buildEvolutionReason(
	macro *narrative.MacroRiskAssessment,
	structural *narrative.StructuralTrendAssessment,
	drawdown *risk.MacroAwareDrawdownDecision,
) string {
	reason := fmt.Sprintf(
		"Macro risk: %s, Drawdown: %s",
		macro.Level.String(),
		drawdown.Action.String(),
	)

	if structural.ShouldOverrideRisk && structural.DominantTrend != nil {
		reason += fmt.Sprintf(
			", Structural trend: %s (score: %.2f)",
			structural.DominantTrend.Name,
			structural.OverrideScore,
		)
	}

	return reason
}

// GetCurrentState returns the current strategy state
func (e *StrategyEvolver) GetCurrentState() StrategyState {
	return e.currentState
}

// GetHistory returns the evolution history
func (e *StrategyEvolver) GetHistory() []StrategyEvolution {
	return e.history
}

// GetStrategyConfig returns configuration parameters for the current state
func (e *StrategyEvolver) GetStrategyConfig() StrategyConfig {
	switch e.currentState {
	case StrategyNormal:
		return StrategyConfig{
			MaxPositionSize:    0.15,
			MaxSectorExposure:  0.30,
			MinCashReserve:     0.05,
			HedgeRatio:         0.0,
			AllowNewPositions:  true,
			AllowConcentration: true,
		}

	case StrategyCautious:
		return StrategyConfig{
			MaxPositionSize:    0.12,
			MaxSectorExposure:  0.25,
			MinCashReserve:     0.10,
			HedgeRatio:         0.10,
			AllowNewPositions:  true,
			AllowConcentration: false,
		}

	case StrategyDefensive:
		return StrategyConfig{
			MaxPositionSize:    0.08,
			MaxSectorExposure:  0.20,
			MinCashReserve:     0.20,
			HedgeRatio:         0.20,
			AllowNewPositions:  false,
			AllowConcentration: false,
		}

	case StrategyHedged:
		return StrategyConfig{
			MaxPositionSize:    0.10,
			MaxSectorExposure:  0.25,
			MinCashReserve:     0.15,
			HedgeRatio:         0.30,
			AllowNewPositions:  true,
			AllowConcentration: false,
		}

	case StrategySuspended:
		return StrategyConfig{
			MaxPositionSize:    0.0,
			MaxSectorExposure:  0.0,
			MinCashReserve:     1.0,
			HedgeRatio:         0.0,
			AllowNewPositions:  false,
			AllowConcentration: false,
		}

	default:
		return StrategyConfig{
			MaxPositionSize:    0.15,
			MaxSectorExposure:  0.30,
			MinCashReserve:     0.05,
			HedgeRatio:         0.0,
			AllowNewPositions:  true,
			AllowConcentration: true,
		}
	}
}

// StrategyConfig defines parameters for a strategy state
type StrategyConfig struct {
	MaxPositionSize    float64 `json:"max_position_size"`
	MaxSectorExposure  float64 `json:"max_sector_exposure"`
	MinCashReserve     float64 `json:"min_cash_reserve"`
	HedgeRatio         float64 `json:"hedge_ratio"`
	AllowNewPositions  bool    `json:"allow_new_positions"`
	AllowConcentration bool    `json:"allow_concentration"`
}

// ShouldEnterPosition checks if a new position can be entered
// sector parameter reserved for future sector-specific restrictions
func (e *StrategyEvolver) ShouldEnterPosition(symbol string, sector string) (bool, string) {
	config := e.GetStrategyConfig()

	if !config.AllowNewPositions {
		return false, fmt.Sprintf("Strategy state %s prohibits new positions", e.currentState.String())
	}

	return true, fmt.Sprintf("Strategy state %s allows new positions", e.currentState.String())
}

// GetPositionSizeLimit returns the maximum position size for current state
func (e *StrategyEvolver) GetPositionSizeLimit() float64 {
	return e.GetStrategyConfig().MaxPositionSize
}

// ApplySectorRotation persists a sector allocation snapshot for
// consumption by the next trading session.
//
// When the closure store, session resolver, and weight engine are
// wired (SA08), this method:
//
//  1. Computes the projected target via WeightEngine (SA04 single source)
//  2. Resolves the next effective session via TradingSessionResolver
//  3. Builds and persists a SectorAllocationSnapshot via ClosureStore
//  4. Returns the MutationReceipt on success
//
// When dependencies are nil (not yet wired or non-replay path), the
// method falls back to the SA06-safe no-op: returns nil receipt with
// "closure not wired" reason — no false "applied" is reported.
func (e *StrategyEvolver) ApplySectorRotation(
	plan *portfolio.SectorRotationPlan,
	asOf time.Time,
	currentAllocs map[string]float64,
) (receipt *sectorallocation.MutationReceipt, applied bool, reason string) {
	if e.currentState == StrategySuspended {
		return nil, false, "Strategy suspended - sector rotation blocked"
	}

	if e.currentState == StrategyDefensive && plan.PrimaryFlow != "risk_off" {
		return nil, false, "Defensive mode - only risk-off rotations allowed"
	}

	// SA08 fallback: without closure store we cannot persist.
	if e.closureStore == nil {
		return nil, false, "closure not wired — store is nil"
	}
	if e.sessionResolver == nil {
		return nil, false, "closure not wired — resolver is nil"
	}

	// Resolve next effective session (fail-closed: no lookahead without
	// a replay dataset).
	effectiveFrom, err := e.sessionResolver.NextTradingSession(asOf)
	if err != nil {
		return nil, false, fmt.Sprintf("no next session: %v", err)
	}

	snap := sectorallocation.SectorAllocationSnapshot{
		AsOfTradingDate:   asOf.Format("2006-01-02"),
		EffectiveFrom:     effectiveFrom.Format("2006-01-02"),
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
		Applied:           false,
		Current:           convertStringMapToSectorIDs(currentAllocs),
	}

	// Compute projected target from WeightEngine (SA04 single source).
	if e.weightEngine != nil {
		drivers := sectorallocation.DriverInputs{
			CapitalFlowAction: sectorallocation.CapitalFlowAction(plan.PrimaryFlow),
		}
		target, cerr := e.weightEngine.ComputeProjectedTarget(context.TODO(), drivers)
		if cerr != nil {
			snap.FallbackReason = fmt.Sprintf("projection failed: %v", cerr)
		} else {
			snap.Target = target.Target
		}
	} else {
		snap.FallbackReason = "no weight engine"
		// Degraded: use plan allocations as target.
		snap.Target = make(map[industry.SectorID]float64, len(plan.Allocations))
		for _, a := range plan.Allocations {
			snap.Target[industry.SectorID(a.Sector)] = a.TargetPct
		}
	}

	// Derive delta = target - current.
	snap.Delta = make(map[industry.SectorID]float64, len(snap.Target))
	for sector, tgt := range snap.Target {
		cur := snap.Current[sector]
		snap.Delta[sector] = tgt - cur
	}

	// Persist the snapshot.
	storedReceipt, err := e.closureStore.Store(snap)
	if err != nil {
		return nil, false, fmt.Sprintf("store failed: %v", err)
	}

	return storedReceipt, true, "applied"
}

// convertStringMapToSectorIDs converts map[string]float64 to
// map[industry.SectorID]float64.
func convertStringMapToSectorIDs(m map[string]float64) map[industry.SectorID]float64 {
	if m == nil {
		return nil
	}
	out := make(map[industry.SectorID]float64, len(m))
	for k, v := range m {
		out[industry.SectorID(k)] = v
	}
	return out
}

// Reset resets the evolver to initial state (for testing)
func (e *StrategyEvolver) Reset() {
	e.history = make([]StrategyEvolution, 0)
	e.currentState = StrategyNormal
	e.lastEvolutionTime = time.Time{}
}
