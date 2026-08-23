package ledger

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestStoreImplementsSessionStore(t *testing.T) {
	var store *Store
	var _ SessionStore = store
}

func TestStoreSessionStoreMethods(t *testing.T) {
	baseDir := t.TempDir()
	store := NewStore(baseDir)

	session := domain.ReplaySession{ID: "session-test"}
	summary := domain.SessionSummary{
		SessionID:      session.ID,
		Regime:         domain.RegimeNeutral,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
	}

	if err := store.RecordSessionSummary(session, summary); err != nil {
		t.Fatalf("RecordSessionSummary: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	scorecards, outcomes, err := store.LoadAllSessionScorecards()
	if err != nil {
		t.Fatalf("LoadAllSessionScorecards: %v", err)
	}
	if scorecards == nil {
		t.Fatal("scorecards should not be nil")
	}
	if outcomes == nil {
		t.Fatal("outcomes should not be nil")
	}
}
