package ledger

import "github.com/kaecer68/atlas-go/internal/domain"

// SessionStore covers session-centric persistence.
// Extracted from OutcomeStore per Interface Segregation Principle.
type SessionStore interface {
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
	LoadSessionSummaries() ([]domain.SessionSummary, error)
	LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error)
}
