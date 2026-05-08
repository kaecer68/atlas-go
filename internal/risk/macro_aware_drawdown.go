package risk

import (
	"fmt"
	"maps"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// DrawdownAction represents the recommended drawdown action
type DrawdownAction int

const (
	DrawdownNone DrawdownAction = iota
	DrawdownLight
	DrawdownModerate
	DrawdownSevere
	DrawdownEmergency
)

func (a DrawdownAction) String() string {
	switch a {
	case DrawdownNone:
		return "none"
	case DrawdownLight:
		return "light"
	case DrawdownModerate:
		return "moderate"
	case DrawdownSevere:
		return "severe"
	case DrawdownEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

// DrawdownLevel defines the percentage reduction for each action
type DrawdownLevel struct {
	Action      DrawdownAction
	Percentage  float64
	MaxExposure float64
}

// DefaultDrawdownLevels defines standard drawdown levels
var DefaultDrawdownLevels = map[DrawdownAction]DrawdownLevel{
	DrawdownNone:      {Action: DrawdownNone, Percentage: 0.0, MaxExposure: 1.0},
	DrawdownLight:     {Action: DrawdownLight, Percentage: 0.15, MaxExposure: 0.85},
	DrawdownModerate:  {Action: DrawdownModerate, Percentage: 0.35, MaxExposure: 0.65},
	DrawdownSevere:    {Action: DrawdownSevere, Percentage: 0.60, MaxExposure: 0.40},
	DrawdownEmergency: {Action: DrawdownEmergency, Percentage: 0.90, MaxExposure: 0.10},
}

// NewDefaultDrawdownLevels creates drawdown levels from config
func NewDefaultDrawdownLevels(cfg map[string]config.DrawdownLevel) map[DrawdownAction]DrawdownLevel {
	levels := make(map[DrawdownAction]DrawdownLevel)
	if none, ok := cfg["none"]; ok {
		levels[DrawdownNone] = DrawdownLevel{Action: DrawdownNone, Percentage: none.Percentage, MaxExposure: none.MaxExposure}
	}
	if light, ok := cfg["light"]; ok {
		levels[DrawdownLight] = DrawdownLevel{Action: DrawdownLight, Percentage: light.Percentage, MaxExposure: light.MaxExposure}
	}
	if moderate, ok := cfg["moderate"]; ok {
		levels[DrawdownModerate] = DrawdownLevel{Action: DrawdownModerate, Percentage: moderate.Percentage, MaxExposure: moderate.MaxExposure}
	}
	if severe, ok := cfg["severe"]; ok {
		levels[DrawdownSevere] = DrawdownLevel{Action: DrawdownSevere, Percentage: severe.Percentage, MaxExposure: severe.MaxExposure}
	}
	if emergency, ok := cfg["emergency"]; ok {
		levels[DrawdownEmergency] = DrawdownLevel{Action: DrawdownEmergency, Percentage: emergency.Percentage, MaxExposure: emergency.MaxExposure}
	}
	return levels
}

// MacroAwareDrawdownDecision represents the final drawdown decision
type MacroAwareDrawdownDecision struct {
	Action             DrawdownAction `json:"action"`
	Percentage         float64        `json:"percentage"`
	MaxExposure        float64        `json:"max_exposure"`
	Rationale          string         `json:"rationale"`
	StructuralOverride bool           `json:"structural_override"`
	Timestamp          time.Time      `json:"timestamp"`
}

// MacroAwareDrawdownEngine integrates macro risk and structural trends for drawdown decisions
type MacroAwareDrawdownEngine struct {
	levels map[DrawdownAction]DrawdownLevel
	cfg    config.DrawdownConfig
}

// NewMacroAwareDrawdownEngine creates a new drawdown engine with default levels
func NewMacroAwareDrawdownEngine() *MacroAwareDrawdownEngine {
	return NewMacroAwareDrawdownEngineWithConfig(config.GetEngineConfig().Drawdown)
}

// NewMacroAwareDrawdownEngineWithConfig creates a new drawdown engine with custom config
func NewMacroAwareDrawdownEngineWithConfig(cfg config.DrawdownConfig) *MacroAwareDrawdownEngine {
	return &MacroAwareDrawdownEngine{
		levels: NewDefaultDrawdownLevels(cfg.Levels),
		cfg:    cfg,
	}
}

// Evaluate makes a drawdown decision based on macro risk and structural trends
func (e *MacroAwareDrawdownEngine) Evaluate(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
) *MacroAwareDrawdownDecision {
	decision := &MacroAwareDrawdownDecision{
		Timestamp: time.Now(),
	}

	// Check if structural trends can override macro risks
	canWithstand := e.canWithstandMacroRisk(macroAssessment.Level, structuralAssessment)
	decision.StructuralOverride = canWithstand && structuralAssessment.ShouldOverrideRisk

	// Determine drawdown action
	decision.Action = e.determineAction(macroAssessment, canWithstand)

	// Get drawdown level details
	level := e.levels[decision.Action]
	decision.Percentage = level.Percentage
	decision.MaxExposure = level.MaxExposure

	// Build rationale
	decision.Rationale = e.buildRationale(macroAssessment, structuralAssessment, decision)

	return decision
}

func (e *MacroAwareDrawdownEngine) canWithstandMacroRisk(
	level narrative.MacroRiskLevel,
	structural *narrative.StructuralTrendAssessment,
) bool {
	// Green/Yellow: always withstand
	if level <= narrative.MacroRiskYellow {
		return true
	}

	// No structural assessment: follow macro risk
	if structural == nil || structural.DominantTrend == nil {
		return false
	}

	// Orange risk: withstand if strong structural trend
	if level == narrative.MacroRiskOrange {
		return structural.OverrideScore >= e.cfg.OrangeOverrideMinScore
	}

	// Red risk: only withstand if very strong structural trend
	if level == narrative.MacroRiskRed {
		return structural.OverrideScore >= e.cfg.RedOverrideMinScore
	}

	return false
}

func (e *MacroAwareDrawdownEngine) determineAction(
	macro *narrative.MacroRiskAssessment,
	canWithstand bool,
) DrawdownAction {
	switch macro.Level {
	case narrative.MacroRiskGreen:
		return DrawdownNone

	case narrative.MacroRiskYellow:
		// Light drawdown for elevated risk
		return DrawdownLight

	case narrative.MacroRiskOrange:
		if canWithstand {
			// Reduced drawdown if structural trends are strong
			return DrawdownLight
		}
		return DrawdownModerate

	case narrative.MacroRiskRed:
		if canWithstand {
			// Moderate drawdown even in red if structural trends are exceptional
			return DrawdownModerate
		}
		// Severe drawdown for critical macro risk
		return DrawdownSevere

	default:
		return DrawdownLight
	}
}

func (e *MacroAwareDrawdownEngine) buildRationale(
	macro *narrative.MacroRiskAssessment,
	structural *narrative.StructuralTrendAssessment,
	decision *MacroAwareDrawdownDecision,
) string {
	rationale := fmt.Sprintf(
		"Macro risk level: %s. Foreign outflow probability: %.1f%%. ",
		macro.Level.String(),
		macro.ForeignOutflowProb,
	)

	if decision.StructuralOverride && structural != nil && structural.DominantTrend != nil {
		rationale += fmt.Sprintf(
			"Structural trend '%s' (score: %.2f) overrides macro risk. ",
			structural.DominantTrend.Name,
			structural.OverrideScore,
		)
	}

	rationale += fmt.Sprintf(
		"Drawdown action: %s (%.0f%% reduction, max exposure %.0f%%).",
		decision.Action.String(),
		decision.Percentage*100,
		decision.MaxExposure*100,
	)

	return rationale
}

// GetPositionSizeAdjustment returns the position size multiplier based on drawdown decision
func (e *MacroAwareDrawdownEngine) GetPositionSizeAdjustment(decision *MacroAwareDrawdownDecision) float64 {
	return decision.MaxExposure
}

// ShouldHaltTrading determines if trading should be halted based on the decision
func (e *MacroAwareDrawdownEngine) ShouldHaltTrading(decision *MacroAwareDrawdownDecision) bool {
	return decision.Action >= DrawdownSevere
}

// GetSectorConstraints returns sector-specific constraints based on macro assessment
func (e *MacroAwareDrawdownEngine) GetSectorConstraints(
	macro *narrative.MacroRiskAssessment,
) map[string]float64 {
	constraints := make(map[string]float64)

	switch macro.PrimaryFlow {
	case "risk_off":
		maps.Copy(constraints, e.cfg.SectorConstraintsRiskOff)

	case "carry_trade_unwind":
		maps.Copy(constraints, e.cfg.SectorConstraintsCarryTradeUnwind)

	case "sector_rotation":
		maps.Copy(constraints, e.cfg.SectorConstraintsSectorRotation)

	default:
	}

	return constraints
}

// CalculatePortfolioAdjustment calculates the target portfolio adjustment
func (e *MacroAwareDrawdownEngine) CalculatePortfolioAdjustment(
	currentExposure float64,
	decision *MacroAwareDrawdownDecision,
) (targetExposure float64, adjustment float64) {
	targetExposure = decision.MaxExposure
	adjustment = targetExposure - currentExposure
	return
}
