package orchestrator

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
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
}

// NewStrategyEvolver creates a new strategy evolver
func NewStrategyEvolver() *StrategyEvolver {
	return NewStrategyEvolverWithConfig(config.GetEngineConfig().StrategyEvolution)
}

// NewStrategyEvolverWithConfig creates a new strategy evolver with custom config
func NewStrategyEvolverWithConfig(cfg config.StrategyEvolutionConfig) *StrategyEvolver {
	return &StrategyEvolver{
		history:        make([]StrategyEvolution, 0),
		currentState:   StrategyNormal,
		cooldownPeriod: cfg.GetCooldownDuration(),
	}
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

	// Check cooldown period
	if time.Since(e.lastEvolutionTime) < e.cooldownPeriod {
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

// ApplySectorRotation applies sector rotation plan to strategy
func (e *StrategyEvolver) ApplySectorRotation(
	plan *portfolio.SectorRotationPlan,
) (modified bool, rationale string) {
	if e.currentState == StrategySuspended {
		return false, "Strategy suspended - sector rotation blocked"
	}

	if e.currentState == StrategyDefensive && plan.PrimaryFlow != "risk_off" {
		// In defensive mode, only allow defensive rotations
		return false, "Defensive mode - only risk-off rotations allowed"
	}

	return true, fmt.Sprintf("Sector rotation applied for %s flow", plan.PrimaryFlow)
}

// Reset resets the evolver to initial state (for testing)
func (e *StrategyEvolver) Reset() {
	e.history = make([]StrategyEvolution, 0)
	e.currentState = StrategyNormal
	e.lastEvolutionTime = time.Time{}
}
