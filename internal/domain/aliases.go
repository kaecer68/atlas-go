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

type Side = shared.Side

const (
	SideBuy  = shared.SideBuy
	SideSell = shared.SideSell
)

type Quote = shared.Quote
type FactorScoreItem = shared.FactorScoreItem
type FactorScoreBreakdown = shared.FactorScoreBreakdown
type FactorScores = shared.FactorScores
type ConvictionStep = shared.ConvictionStep
type ConvictionBreakdown = shared.ConvictionBreakdown
type AgentLayer = shared.AgentLayer

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

type Recommendation = recommendation.Recommendation
type AgentSpec = recommendation.AgentSpec
type AgentRegistry = recommendation.AgentRegistry
type RecommendationOutcome = recommendation.RecommendationOutcome
type HumanIntervention = recommendation.HumanIntervention
type Scorecard = recommendation.Scorecard
type ScreeningCriteria = recommendation.ScreeningCriteria
type RangeFilter = recommendation.RangeFilter
type MinFilter = recommendation.MinFilter
type ScreeningReject = recommendation.ScreeningReject
type GuardSeverity = recommendation.GuardSeverity

const (
	GuardSeveritySoft = recommendation.GuardSeveritySoft
	GuardSeverityHard = recommendation.GuardSeverityHard
)

type GuardOutcome = recommendation.GuardOutcome
type ExecutionInput = recommendation.ExecutionInput
type PromptControl = recommendation.PromptControl

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

type ExperimentRecord = experiment.ExperimentRecord
type MutationBrief = experiment.MutationBrief
type OOSResult = experiment.OOSResult
type PromptExperimentResult = experiment.PromptExperimentResult
type ReplayDataMetadata = experiment.ReplayDataMetadata

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
