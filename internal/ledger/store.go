package ledger

import "github.com/kaecer68/atlas-go/internal/domain"

type OutcomeStore interface {
	RecordOutcomes(outcomes []domain.RecommendationOutcome) error
	LoadOutcomes() ([]domain.RecommendationOutcome, error)
	RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error
	LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error)
	RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error
	LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error)
	RecordExperiment(record domain.ExperimentRecord) error
	RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
	LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error)
	LoadHumanInterventions() ([]domain.HumanIntervention, error)
	RecordHumanIntervention(intervention domain.HumanIntervention) error
	RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error
	UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error
}
