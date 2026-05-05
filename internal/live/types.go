package live

import (
	"github.com/kaecer68/atlas-go/internal/domain"
)

type ExecutionInput struct {
	Regime               domain.Regime
	RawRecommendations   []domain.Recommendation
	FinalRecommendations []domain.Recommendation
	GuardOutcomes        []domain.GuardOutcome
	DeterminedBy         string
}