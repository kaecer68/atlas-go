package ledger

import (
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSQLiteSessionStore_RecordAndLoad(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	store := NewSQLiteSessionStore(db)

	session := domain.ReplaySession{
		ID:          "session-test-001",
		Mode:        "replay",
		Market:      "TWSE",
		SessionDate: time.Now(),
		DataSource:  "TWSE",
		StartedAt:   time.Now(),
	}
	summary := domain.SessionSummary{
		SessionID:      session.ID,
		Regime:         domain.RegimeNeutral,
		OrderCount:     5,
		PositionCount:  3,
		EndingCash:     100000.0,
		PortfolioValue: 150000.0,
		OutcomeCount:   10,
		RecordedAt:     time.Now(),
		AfterTaxPnL:    5000.0,
		TotalTaxPaid:   50.0,
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
	if summaries[0].SessionID != summary.SessionID {
		t.Errorf("SessionID: expected %s, got %s", summary.SessionID, summaries[0].SessionID)
	}
	if summaries[0].Regime != summary.Regime {
		t.Errorf("Regime: expected %s, got %s", summary.Regime, summaries[0].Regime)
	}
	if summaries[0].OrderCount != summary.OrderCount {
		t.Errorf("OrderCount: expected %d, got %d", summary.OrderCount, summaries[0].OrderCount)
	}
}

func TestSQLiteSessionStore_LoadAllSessionScorecards_ReturnsEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	store := NewSQLiteSessionStore(db)

	scorecards, outcomes, err := store.LoadAllSessionScorecards()
	if err != nil {
		t.Fatalf("LoadAllSessionScorecards: %v", err)
	}
	if scorecards != nil {
		t.Error("scorecards should be nil")
	}
	if outcomes != nil {
		t.Error("outcomes should be nil")
	}
}

func TestSQLiteSessionStore_UpdateExisting(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "atlas-sqlite-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	db, err := OpenSQLiteDB(tmpPath)
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	store := NewSQLiteSessionStore(db)

	session := domain.ReplaySession{ID: "session-update-test"}
	summary1 := domain.SessionSummary{
		SessionID:      session.ID,
		Regime:         domain.RegimeRiskOn,
		OrderCount:     1,
		EndingCash:     100000.0,
		PortfolioValue: 150000.0,
		RecordedAt:     time.Now(),
	}
	summary2 := domain.SessionSummary{
		SessionID:      session.ID,
		Regime:         domain.RegimeRiskOff,
		OrderCount:     2,
		EndingCash:     90000.0,
		PortfolioValue: 160000.0,
		RecordedAt:     time.Now(),
	}

	if err := store.RecordSessionSummary(session, summary1); err != nil {
		t.Fatalf("RecordSessionSummary (first): %v", err)
	}
	if err := store.RecordSessionSummary(session, summary2); err != nil {
		t.Fatalf("RecordSessionSummary (second): %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary after update, got %d", len(summaries))
	}
	if summaries[0].OrderCount != summary2.OrderCount {
		t.Errorf("OrderCount after update: expected %d, got %d", summary2.OrderCount, summaries[0].OrderCount)
	}
}

func TestSQLiteSessionStore_ImplementsSessionStore(t *testing.T) {
	var store *SQLiteSessionStore
	var _ SessionStore = store
}
