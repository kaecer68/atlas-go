package domain

import (
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// ---- shared kernel ----

type Regime = shared.Regime

const (
	RegimeRiskOn  = shared.RegimeRiskOn
	RegimeRiskOff = shared.RegimeRiskOff
	RegimeNeutral = shared.RegimeNeutral
)

type MarketPeriod = shared.MarketPeriod

const (
	PeriodDownturn       = shared.PeriodDownturn
	PeriodTurnaroundUp   = shared.PeriodTurnaroundUp
	PeriodBull           = shared.PeriodBull
	PeriodPlateau        = shared.PeriodPlateau
	PeriodConsolidation  = shared.PeriodConsolidation
	PeriodTurnaroundDown = shared.PeriodTurnaroundDown
	PeriodBlackSwan      = shared.PeriodBlackSwan
)

type Side = shared.Side

const (
	SideBuy    = shared.SideBuy
	SideSell   = shared.SideSell
	SideReduce = shared.SideReduce
)

type (
	Quote                    = shared.Quote
	FactorScoreItem          = shared.FactorScoreItem
	FactorScoreBreakdown     = shared.FactorScoreBreakdown
	FactorScores             = shared.FactorScores
	NarrativeFactorScore     = shared.NarrativeFactorScore
	IndustryCycleFactorScore = shared.IndustryCycleFactorScore
	LinkageFactorScore       = shared.LinkageFactorScore
	ConvictionStep           = shared.ConvictionStep
	ConvictionBreakdown      = shared.ConvictionBreakdown
	AgentLayer               = shared.AgentLayer
)

const (
	LayerContext       = shared.LayerContext
	LayerMacro         = shared.LayerMacro
	LayerSector        = shared.LayerSector
	LayerStyle         = shared.LayerStyle
	LayerSuperinvestor = shared.LayerSuperinvestor
	LayerControl       = shared.LayerControl
)

type FlexTime = shared.FlexTime

// ---- recommendation context ----

type (
	Recommendation        = recommendation.Recommendation
	AgentSpec             = recommendation.AgentSpec
	AgentRegistry         = recommendation.AgentRegistry
	RecommendationOutcome = recommendation.RecommendationOutcome
	HumanIntervention     = recommendation.HumanIntervention
	Scorecard             = recommendation.Scorecard
	ScreeningCriteria     = recommendation.ScreeningCriteria
	RangeFilter           = recommendation.RangeFilter
	MinFilter             = recommendation.MinFilter
	ScreeningReject       = recommendation.ScreeningReject
	GuardSeverity         = recommendation.GuardSeverity
	RegimeBreakdown       = recommendation.RegimeBreakdown
	RegimePerformance     = recommendation.RegimePerformance
)

const (
	GuardSeveritySoft = recommendation.GuardSeveritySoft
	GuardSeverityHard = recommendation.GuardSeverityHard
)

type (
	GuardOutcome   = recommendation.GuardOutcome
	ExecutionInput = recommendation.ExecutionInput
	PromptControl  = recommendation.PromptControl
)

// ---- experiment context ----

const MutationBriefContractVersion = experiment.MutationBriefContractVersion

type ExperimentStatus = experiment.ExperimentStatus

const (
	ExperimentPlanned  = experiment.ExperimentPlanned
	ExperimentRunning  = experiment.ExperimentRunning
	ExperimentAccepted = experiment.ExperimentAccepted
	ExperimentRejected = experiment.ExperimentRejected
	ExperimentExpired  = experiment.ExperimentExpired
)

type (
	ExperimentRecord       = experiment.ExperimentRecord
	MutationBrief          = experiment.MutationBrief
	OOSResult              = experiment.OOSResult
	PromptExperimentResult = experiment.PromptExperimentResult
	ReplayDataMetadata     = experiment.ReplayDataMetadata
)

// Function wrappers — Go does not support function aliases, so we delegate.
var ControlBlockRe = recommendation.ControlBlockRe

func ExtractPromptControl(prompt string) (PromptControl, bool) {
	return recommendation.ExtractPromptControl(prompt)
}

func RenderPromptControl(ctrl PromptControl) string {
	return recommendation.RenderPromptControl(ctrl)
}

func CanTransitionExperimentStatus(from, to ExperimentStatus) bool {
	return experiment.CanTransitionExperimentStatus(from, to)
}

func TransitionExperimentStatus(record *ExperimentRecord, next ExperimentStatus) error {
	return experiment.TransitionExperimentStatus(record, next)
}
