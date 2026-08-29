package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// recordSummaryStore is the minimal contract needed to retry summary writes.
// It is satisfied by *ledger.Store (and any other store with the same signature),
// which keeps the helper testable without spinning up a full ledger fixture.
type recordSummaryStore interface {
	RecordSessionSummary(domain.ReplaySession, domain.SessionSummary) error
}

// recordSummaryWithRetry calls RecordSessionSummary up to maxAttempts times
// with linear backoff (backoff * attemptNumber). It is the single chokepoint
// for every production session summary write (cmd/atlas/main.go:547/1306/2175
// all funnel through System.RecordSessionSummary → this helper → ledger).
//
// Rationale: a single fsync race or temp-file rename collision during
// summary.json write would otherwise leave an orphan session directory
// (recommendation_outcomes.jsonl present, summary.json missing) and break
// the pipeline page's "0 筆 AI 推薦, 0 筆放行" trust contract.
//
// Returns nil on first success; on persistent failure returns the last error
// so the caller can decide whether to abort the run. We never swallow the
// error — silent failure is the exact bug class we are guarding against.
func recordSummaryWithRetry(
	store recordSummaryStore,
	session domain.ReplaySession,
	summary domain.SessionSummary,
	backoff time.Duration,
) error {
	const maxAttempts = 3

	var lastErr error
	for attempt := range maxAttempts {
		if err := store.RecordSessionSummary(session, summary); err == nil {
			return nil
		} else {
			lastErr = err
		}
		// Sleep between attempts only — never after the final attempt,
		// otherwise the caller observes latency with no recovery chance.
		if attempt < maxAttempts-1 {
			time.Sleep(backoff * time.Duration(attempt+1))
		}
	}
	return lastErr
}
