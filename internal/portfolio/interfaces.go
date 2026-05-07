package portfolio

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FactorEngineInterface is the contract for multi-factor scoring engines.
// Concrete implementation: FactorEngine.
type FactorEngineInterface interface {
	CalculateAllScores(
		symbol string,
		quotes map[string]domain.Quote,
		agentRecs []domain.Recommendation,
		agentWeights map[string]float64,
		factorWeights map[FactorType]float64,
		bridgeInputs ...FactorBridgeInput,
	) map[FactorType]float64

	CalculateAllScoresWithBreakdown(
		symbol string,
		quotes map[string]domain.Quote,
		agentRecs []domain.Recommendation,
		agentWeights map[string]float64,
		factorWeights map[FactorType]float64,
		bridgeInputs ...FactorBridgeInput,
	) (*domain.FactorScoreBreakdown, map[FactorType]float64)

	CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64
	CalculateValueScore(symbol string, quotes map[string]domain.Quote) float64
	CalculateQualityScore(symbol string, quotes map[string]domain.Quote) float64
}

// OptimizerInterface is the contract for portfolio optimization engines.
// Concrete implementation: Optimizer.
type OptimizerInterface interface {
	Optimize(
		ctx context.Context,
		recommendations []domain.Recommendation,
		quotes map[string]domain.Quote,
		totalCapital float64,
	) ([]OptimizedPosition, error)

	SetConstraints(c Constraints)
	SetAgentWeights(weights map[string]float64)
	SetFactorWeights(weights map[FactorType]float64)
	OptimizeToOrders(
		ctx context.Context,
		recommendations []domain.Recommendation,
		quotes map[string]domain.Quote,
		totalCapital float64,
	) ([]domain.Order, error)
}

// DarwinianWeightManagerInterface is the contract for Darwinian weight management.
// Concrete implementation: DarwinianWeightManager.
type DarwinianWeightManagerInterface interface {
	ApplyDarwinianWeightsWithEvents(
		recommendations []domain.Recommendation,
	) ([]domain.Recommendation, []ConvictionClampingEvent)

	GetWeight(agentID string) float64
	GetAllWeights() map[string]float64
	RecordOutcome(agentID string, dailyReturn float64, hit bool)
	PerformDailyAdjustment() (map[string]float64, []ClampingEvent)
	InitializeFromRegistry(registry domain.AgentRegistry)
	GetAllAgentWeightData() []*DarwinianAgentWeight
	Save() error
	AppendSnapshot() error
	Load() error
}

// ConvictionNormalizerInterface is the contract for belief score normalization.
// Concrete implementation: ConvictionNormalizer.
type ConvictionNormalizerInterface interface {
	Normalize(agentID string, conviction int, method NormalizationMethod) float64
	RecordConviction(agentID string, conviction int)
	GetStats(agentID string) (count int, mean, stdDev, min, max float64)
}

// Compile-time checks that concrete types satisfy interfaces.
var (
	_ FactorEngineInterface          = (*FactorEngine)(nil)
	_ OptimizerInterface             = (*Optimizer)(nil)
	_ DarwinianWeightManagerInterface = (*DarwinianWeightManager)(nil)
	_ ConvictionNormalizerInterface  = (*ConvictionNormalizer)(nil)
)
