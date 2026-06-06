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
	return NewMacroAwareDrawdownEngineWithConfig(config.GetParametersConfig().Engine.Drawdown.ToConfig())
}

// NewMacroAwareDrawdownEngineWithConfig creates a new drawdown engine with custom config
func NewMacroAwareDrawdownEngineWithConfig(cfg config.DrawdownConfig) *MacroAwareDrawdownEngine {
	return &MacroAwareDrawdownEngine{
		levels: NewDefaultDrawdownLevels(cfg.Levels),
		cfg:    cfg,
	}
}

// NewMacroAwareDrawdownEngineFromParameters creates a new drawdown engine
// using the unified ParametersConfig.Drawdown settings. This is the preferred
// constructor when the unified parameter system is available.
func NewMacroAwareDrawdownEngineFromParameters() *MacroAwareDrawdownEngine {
	p := config.GetParametersConfig().Drawdown
	return NewMacroAwareDrawdownEngineWithConfig(config.DrawdownConfig{
		Levels: map[string]config.DrawdownLevel{
			"none":      {Percentage: p.NonePercentage.Value, MaxExposure: p.NoneMaxExposure.Value},
			"light":     {Percentage: p.LightPercentage.Value, MaxExposure: p.LightMaxExposure.Value},
			"moderate":  {Percentage: p.ModeratePercentage.Value, MaxExposure: p.ModerateMaxExposure.Value},
			"severe":    {Percentage: p.SeverePercentage.Value, MaxExposure: p.SevereMaxExposure.Value},
			"emergency": {Percentage: p.EmergencyPercentage.Value, MaxExposure: p.EmergencyMaxExposure.Value},
		},
		OrangeOverrideMinScore:            p.OrangeOverrideMinScore.Value,
		RedOverrideMinScore:               p.RedOverrideMinScore.Value,
		SectorConstraintsRiskOff:          p.SectorConstraintsRiskOff.Value,
		SectorConstraintsCarryTradeUnwind: p.SectorConstraintsCarryTradeUnwind.Value,
		SectorConstraintsSectorRotation:   p.SectorConstraintsSectorRotation.Value,
	})
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

// DrawdownBreakdownStep represents one step in the drawdown decision process.
type DrawdownBreakdownStep struct {
	Source     string  `json:"source"`
	Rule       string  `json:"rule"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
}

// DrawdownBreakdown documents the full decision chain for a drawdown evaluation.
type DrawdownBreakdown struct {
	Steps            []DrawdownBreakdownStep `json:"steps"`
	FinalAction      DrawdownAction          `json:"final_action"`
	FinalPercentage  float64                 `json:"final_percentage"`
	FinalMaxExposure float64                 `json:"final_max_exposure"`
	Timestamp        time.Time               `json:"timestamp"`
}

// EvaluateWithIndustry extends Evaluate with industry cycle risk analysis.
// The industry assessment may escalate the drawdown action when many industries are in recession.
func (e *MacroAwareDrawdownEngine) EvaluateWithIndustry(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
	industryAssessment *IndustryRiskAssessment,
) (*MacroAwareDrawdownDecision, *DrawdownBreakdown) {
	decision := e.Evaluate(macroAssessment, structuralAssessment)

	breakdown := &DrawdownBreakdown{
		Timestamp: time.Now(),
	}

	// Step 1: macro risk baseline
	breakdown.Steps = append(breakdown.Steps, DrawdownBreakdownStep{
		Source:     "macro",
		Rule:       fmt.Sprintf("MacroRiskLevel=%s, OutflowProb=%.1f%%", macroAssessment.Level.String(), macroAssessment.ForeignOutflowProb),
		Action:     decision.Action.String(),
		Confidence: 1.0,
	})

	// Step 2: structural override
	if decision.StructuralOverride && structuralAssessment != nil {
		breakdown.Steps = append(breakdown.Steps, DrawdownBreakdownStep{
			Source:     "structural",
			Rule:       fmt.Sprintf("OverrideScore=%.2f >= threshold, DominantTrend=%s", structuralAssessment.OverrideScore, structuralAssessment.DominantTrend.Name),
			Action:     "override_applied",
			Confidence: structuralAssessment.OverrideScore,
		})
	} else {
		breakdown.Steps = append(breakdown.Steps, DrawdownBreakdownStep{
			Source:     "structural",
			Rule:       "no structural override",
			Action:     "none",
			Confidence: 0.0,
		})
	}

	// Step 3: industry cycle risk
	if industryAssessment != nil && industryAssessment.TotalIndustryCount > 0 {
		recessionRatio := float64(industryAssessment.RecessionIndustryCount) / float64(industryAssessment.TotalIndustryCount)
		industryStep := e.buildIndustryCycleStep(industryAssessment, recessionRatio)

		if recessionRatio >= 0.25 {
			originalAction := decision.Action
			decision.Action = e.escalateAction(decision.Action)
			level := e.levels[decision.Action]
			decision.Percentage = level.Percentage
			decision.MaxExposure = level.MaxExposure
			decision.Rationale = decision.Rationale + fmt.Sprintf(
				" Industry cycle escalated from %s to %s (%.0f%% industries in recession, %d/%d).",
				originalAction.String(), decision.Action.String(),
				recessionRatio*100,
				industryAssessment.RecessionIndustryCount,
				industryAssessment.TotalIndustryCount,
			)
		}

		breakdown.Steps = append(breakdown.Steps, industryStep)
	}

	breakdown.FinalAction = decision.Action
	breakdown.FinalPercentage = decision.Percentage
	breakdown.FinalMaxExposure = decision.MaxExposure

	return decision, breakdown
}

func (e *MacroAwareDrawdownEngine) buildIndustryCycleStep(assessment *IndustryRiskAssessment, recessionRatio float64) DrawdownBreakdownStep {
	recessionPct := recessionRatio * 100
	expansionPct := float64(assessment.ExpansionIndustryCount) / float64(assessment.TotalIndustryCount) * 100

	step := DrawdownBreakdownStep{
		Source:     "industry_cycle",
		Confidence: 1.0 - recessionRatio,
	}

	switch {
	case recessionRatio >= 0.25:
		step.Rule = fmt.Sprintf(
			"%.0f%% industries in recession (%d/%d), %.0f%% expansion → escalate drawdown",
			recessionPct, assessment.RecessionIndustryCount, assessment.TotalIndustryCount, expansionPct,
		)
		step.Action = "escalate"
	case recessionRatio == 0:
		step.Rule = fmt.Sprintf(
			"0%% recession, %.0f%% expansion (%d/%d) → no industry cycle risk",
			expansionPct, assessment.ExpansionIndustryCount, assessment.TotalIndustryCount,
		)
		step.Action = "no_change"
	default:
		step.Rule = fmt.Sprintf(
			"%.0f%% industries in recession (%d/%d), %.0f%% expansion → below escalation threshold",
			recessionPct, assessment.RecessionIndustryCount, assessment.TotalIndustryCount, expansionPct,
		)
		step.Action = "no_change"
	}

	return step
}

// EvaluateWithPortfolio extends EvaluateWithIndustry with portfolio-level risk analysis.
// The portfolio risk assessment may escalate the drawdown action when concentration,
// sector exposure, or factor exposure risks are excessive.
func (e *MacroAwareDrawdownEngine) EvaluateWithPortfolio(
	macroAssessment *narrative.MacroRiskAssessment,
	structuralAssessment *narrative.StructuralTrendAssessment,
	industryAssessment *IndustryRiskAssessment,
	portfolioProvider PortfolioRiskProvider,
) (*MacroAwareDrawdownDecision, *DrawdownBreakdown) {
	decision, breakdown := e.EvaluateWithIndustry(macroAssessment, structuralAssessment, industryAssessment)

	if portfolioProvider != nil {
		portfolioRisk := portfolioProvider.Assess()
		if portfolioRisk != nil {
			portfolioStep := e.buildPortfolioRiskStep(portfolioRisk)
			breakdown.Steps = append(breakdown.Steps, portfolioStep)

			if portfolioRisk.TotalRiskScore >= 0.8 {
				originalAction := decision.Action
				decision.Action = e.escalateAction(decision.Action)
				level := e.levels[decision.Action]
				decision.Percentage = level.Percentage
				decision.MaxExposure = level.MaxExposure
				decision.Rationale = decision.Rationale + fmt.Sprintf(
					" Portfolio risk score %.2f escalated from %s to %s.",
					portfolioRisk.TotalRiskScore,
					originalAction.String(), decision.Action.String(),
				)
			}

			breakdown.FinalAction = decision.Action
			breakdown.FinalPercentage = decision.Percentage
			breakdown.FinalMaxExposure = decision.MaxExposure
		}
	}

	return decision, breakdown
}

// buildPortfolioRiskStep creates a breakdown step based on portfolio risk assessment.
func (e *MacroAwareDrawdownEngine) buildPortfolioRiskStep(assessment *PortfolioRiskAssessment) DrawdownBreakdownStep {
	step := DrawdownBreakdownStep{
		Source: "portfolio_risk",
	}

	switch {
	case assessment.TotalRiskScore >= 0.8:
		step.Rule = fmt.Sprintf(
			"High portfolio risk: concentration=%.2f, sector exposure=%d, factor exposure=%d → escalate drawdown",
			assessment.ConcentrationScore, len(assessment.SectorExposure), len(assessment.FactorExposure),
		)
		step.Action = "escalate"
		step.Confidence = assessment.TotalRiskScore
	case assessment.TotalRiskScore >= 0.5:
		step.Rule = fmt.Sprintf(
			"Moderate portfolio risk: concentration=%.2f, sector exposure=%d, factor exposure=%d → no escalation",
			assessment.ConcentrationScore, len(assessment.SectorExposure), len(assessment.FactorExposure),
		)
		step.Action = "no_change"
		step.Confidence = assessment.TotalRiskScore
	default:
		step.Rule = fmt.Sprintf(
			"Low portfolio risk: concentration=%.2f, sector exposure=%d, factor exposure=%d → no change",
			assessment.ConcentrationScore, len(assessment.SectorExposure), len(assessment.FactorExposure),
		)
		step.Action = "no_change"
		step.Confidence = 1.0 - assessment.TotalRiskScore
	}

	return step
}

func (e *MacroAwareDrawdownEngine) escalateAction(action DrawdownAction) DrawdownAction {
	switch action {
	case DrawdownNone:
		return DrawdownLight
	case DrawdownLight:
		return DrawdownModerate
	case DrawdownModerate:
		return DrawdownSevere
	case DrawdownSevere:
		return DrawdownEmergency
	default:
		return DrawdownEmergency
	}
}
