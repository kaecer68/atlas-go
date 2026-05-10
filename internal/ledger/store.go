package ledger

import "github.com/kaecer68/atlas-go/internal/domain"

// OutcomeStore is the primary interface for the append-only recommendation ledger.
// It covers the core production flow: outcome recording, session summaries,
// screening rejects, experiment records, and human interventions.
//
// *Store automatically satisfies this interface. New consumers should depend on
// OutcomeStore rather than *Store to enable test mocking and backend swaps.
type OutcomeStore interface {
	// Outcome recording
	RecordOutcomes(outcomes []domain.RecommendationOutcome) error
	RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error
	LoadOutcomes() ([]domain.RecommendationOutcome, error)
	LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error)

	// Screening rejects
	RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error
	LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error)
	RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error
	LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error)
	LoadAllSessionTrades() ([]domain.TradeRecord, error)

	// Experiment lifecycle
	RecordExperiment(record domain.ExperimentRecord) error
	RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error

	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
	LoadSessionSummaries() ([]domain.SessionSummary, error)
	LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error)

	// Human-in-the-loop
	RecordHumanIntervention(intervention domain.HumanIntervention) error
	LoadHumanInterventions() ([]domain.HumanIntervention, error)
}

// ExperimentStore covers experiment detail persistence (prompt results, listing).
// Separate from OutcomeStore per Interface Segregation Principle.
type ExperimentStore interface {
	LoadExperiments() ([]domain.ExperimentRecord, error)
	RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error
	UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error
}

// BacktestStore covers backtest window and mutation persistence.
type BacktestStore interface {
	RecordWindowSummary(summary domain.BacktestWindowSummary) error
	RecordMutationBrief(windowID string, brief domain.MutationBrief) error
}

// FullStore combines all ledger interfaces for consumers that genuinely need
// the complete persistence surface.
type FullStore interface {
	OutcomeStore
	SessionStore
	ExperimentStore
	BacktestStore
	RecordSpawnRecord(record SpawnRecord) error
	LoadSpawnRecords() ([]SpawnRecord, error)
}
