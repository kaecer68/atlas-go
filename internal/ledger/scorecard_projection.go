package ledger

import "github.com/kaecer68/atlas-go/internal/domain"

// LoadScorecardProjection returns the outcomes needed to build scorecards or
// per-agent statistics, preferring the slim projection when the store provides
// it (ScorecardOutcomeStore) and falling back to the full per-session read
// otherwise.
//
// Every consumer in this file's callers used to read the whole outcomes table
// including the metadata JSONB — 271k rows / 644 MB in production, which
// expanded to ~1.7-2.9 GB of live heap per call and OOM-killed the atlas
// container on a ~5 minute loop (2026-09-17). The slim read returns the same
// scalar fields without ever transferring the blob.
//
// A store whose slim read errors is not fatal: the full read still works, so
// callers keep serving data.
func LoadScorecardProjection(store OutcomeStore) ([]domain.RecommendationOutcome, error) {
	if slim, ok := store.(ScorecardOutcomeStore); ok {
		outcomes, err := slim.LoadScorecardOutcomes()
		if err == nil && len(outcomes) > 0 {
			return outcomes, nil
		}
	}
	outcomes, err := store.LoadOutcomesFromSessions()
	if err == nil && len(outcomes) > 0 {
		return outcomes, nil
	}
	// JSONL/SQLite ledgers may keep outcomes only in the flat file (no sessions
	// directory); an empty per-session read must fall back rather than yield
	// nothing, and a missing ledger must not become an error.
	if flat, ferr := store.LoadOutcomes(); ferr == nil {
		return flat, nil
	}
	return nil, err
}
