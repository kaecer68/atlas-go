package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSQLiteOutcomeStoreRecordAndLoadOutcomes(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-1",
			Skill:         "sector-tech",
			Layer:         domain.LayerSector,
			Symbol:        "2330",
			Side:          domain.SideBuy,
			Conviction:    80,
			TargetPrice:   1100,
			StopLossPrice: 1000,
			Window:        "2026-01",
			ForwardReturn: 0.05,
			Hit:           true,
			Reason:        "strong momentum",
			Price:         1050,
			PassedGuards:  true,
			GuardReason:   "",
			RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			FactorScores: domain.FactorScores{
				Total: 75,
			},
		},
		{
			AgentID:       "agent-2",
			Skill:         "style-growth",
			Layer:         domain.LayerStyle,
			Symbol:        "2454",
			Side:          domain.SideBuy,
			Conviction:    60,
			TargetPrice:   900,
			StopLossPrice: 800,
			Window:        "2026-01",
			ForwardReturn: -0.02,
			Hit:           false,
			Reason:        "weak demand",
			Price:         850,
			PassedGuards:  true,
			GuardReason:   "",
			RecordedAt:    time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(loaded))
	}
	if loaded[0].Symbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", loaded[0].Symbol)
	}
	if loaded[0].AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", loaded[0].AgentID)
	}
}

func TestSQLiteOutcomeStoreRecordSessionOutcomes(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	session := domain.ReplaySession{
		ID:          "session-20260115",
		Mode:        "backtest",
		Market:      "TWSE",
		SessionDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
	}

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-1",
			Symbol:        "2330",
			Side:          domain.SideBuy,
			Conviction:    80,
			TargetPrice:   1100,
			StopLossPrice: 1000,
			Window:        "2026-01",
			ForwardReturn: 0.05,
			Hit:           true,
			RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := store.RecordSessionOutcomes(session, outcomes); err != nil {
		t.Fatalf("RecordSessionOutcomes failed: %v", err)
	}

	loaded, err := store.LoadSessionOutcomes("session-20260115")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}
	if loaded[0].Symbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", loaded[0].Symbol)
	}
}

func TestSQLiteOutcomeStoreLoadOutcomesFromSessions(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	globalOutcomes := []domain.RecommendationOutcome{
		{
			AgentID: "global-agent", Symbol: "9999", Side: domain.SideBuy, Conviction: 50,
			RecordedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		},
	}
	if err := store.RecordOutcomes(globalOutcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	sessionA := domain.ReplaySession{ID: "session-A", Mode: "backtest", Market: "TWSE", SessionDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)}
	sessionB := domain.ReplaySession{ID: "session-B", Mode: "backtest", Market: "TWSE", SessionDate: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)}

	if err := store.RecordSessionOutcomes(sessionA, []domain.RecommendationOutcome{
		{AgentID: "agent-a1", Symbol: "2330", Side: domain.SideBuy, Conviction: 80, RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{AgentID: "agent-a2", Symbol: "2454", Side: domain.SideBuy, Conviction: 70, RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("RecordSessionOutcomes A failed: %v", err)
	}
	if err := store.RecordSessionOutcomes(sessionB, []domain.RecommendationOutcome{
		{AgentID: "agent-b1", Symbol: "2317", Side: domain.SideBuy, Conviction: 60, RecordedAt: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("RecordSessionOutcomes B failed: %v", err)
	}

	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 session outcomes (excluding global sparse), got %d", len(loaded))
	}
	symbols := map[string]bool{}
	for _, o := range loaded {
		symbols[o.Symbol] = true
		if o.AgentID == "global-agent" {
			t.Errorf("global sparse outcome leaked into FromSessions: %+v", o)
		}
	}
	for _, want := range []string{"2330", "2454", "2317"} {
		if !symbols[want] {
			t.Errorf("expected symbol %s in FromSessions result, missing", want)
		}
	}
}

func TestSQLiteOutcomeStoreLoadSessionOutcomesEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	loaded, err := store.LoadSessionOutcomes("nonexistent-session")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for nonexistent session, got %v", loaded)
	}
}

func TestSQLiteOutcomeStoreRecordScreeningRejects(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	rejects := []domain.ScreeningReject{
		{
			SessionID:      "session-20260115",
			Symbol:         "2330",
			AgentID:        "agent-1",
			Skill:          "sector-tech",
			Criterion:      "PE",
			CriterionLabel: "PE > 30",
			Threshold:      "30",
			ActualValue:    "35",
			FactorScores:   domain.FactorScores{Total: 40},
			RecordedAt:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := store.RecordSessionScreeningRejects("session-20260115", rejects); err != nil {
		t.Fatalf("RecordSessionScreeningRejects failed: %v", err)
	}

	loaded, err := store.LoadSessionScreeningRejects("session-20260115")
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 reject, got %d", len(loaded))
	}
	if loaded[0].Symbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", loaded[0].Symbol)
	}
}

func TestSQLiteOutcomeStoreLoadSessionScreeningRejectsEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	loaded, err := store.LoadSessionScreeningRejects("nonexistent-session")
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for nonexistent session, got %v", loaded)
	}
}

func TestSQLiteOutcomeStoreRecordAndLoadHumanInterventions(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	intervention := domain.HumanIntervention{
		ID:            "hi-001",
		Type:          "approve_rec",
		TargetAgentID: "agent-1",
		TargetSymbol:  "2330",
		Reason:        "manual override for earnings beat",
		Operator:      "operator-1",
		SessionID:     "session-20260115",
		RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordHumanIntervention(intervention); err != nil {
		t.Fatalf("RecordHumanIntervention failed: %v", err)
	}

	loaded, err := store.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 intervention, got %d", len(loaded))
	}
	if loaded[0].Type != "approve_rec" {
		t.Errorf("expected type approve_rec, got %s", loaded[0].Type)
	}
	if loaded[0].TargetSymbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", loaded[0].TargetSymbol)
	}
}

func TestSQLiteOutcomeStoreRecordExperiment(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	record := domain.ExperimentRecord{
		ID:                "exp-001",
		ProposalID:        "prop-001",
		CommitID:          "abc123",
		ApprovalID:        "approval-001",
		TargetAgentID:     "agent-1",
		Skill:             "sector-tech",
		Hypothesis:        "increase conviction threshold",
		PromptVersionFrom: "v1",
		PromptVersionTo:   "v2",
		MutationType:      "constraint",
		AcceptanceGates:   []string{"sharpe > 1.0"},
		WindowStart:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:         time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		AcceptanceMetric:  "sharpe",
		BaselineValue:     0.8,
		CandidateValue:    1.2,
		Status:            domain.ExperimentAccepted,
	}

	if err := store.RecordExperiment(record); err != nil {
		t.Fatalf("RecordExperiment failed: %v", err)
	}
}

func TestSQLiteOutcomeStoreRecordSessionExperiment(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	session := domain.ReplaySession{
		ID:          "session-20260115",
		Mode:        "backtest",
		Market:      "TWSE",
		SessionDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
	}

	record := domain.ExperimentRecord{
		ID:            "exp-002",
		TargetAgentID: "agent-1",
		Status:        domain.ExperimentRunning,
		WindowStart:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordSessionExperiment(session, record); err != nil {
		t.Fatalf("RecordSessionExperiment failed: %v", err)
	}
}

func TestSQLiteOutcomeStoreRecordAndLoadSessionSummary(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	session := domain.ReplaySession{
		ID:          "session-20260115",
		Mode:        "backtest",
		Market:      "TWSE",
		SessionDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
	}

	summary := domain.SessionSummary{
		SessionID:      "session-20260115",
		Regime:         domain.RegimeRiskOn,
		OrderCount:     5,
		PositionCount:  3,
		EndingCash:     500000,
		PortfolioValue: 1500000,
		OutcomeCount:   10,
		RecordedAt:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		GuardOutcomes: []domain.GuardOutcome{
			{GuardID: "guard-1", Passed: true},
			{GuardID: "guard-2", Passed: false},
		},
	}

	if err := store.RecordSessionSummary(session, summary); err != nil {
		t.Fatalf("RecordSessionSummary failed: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].OutcomeCount != 10 {
		t.Errorf("expected outcome count 10, got %d", summaries[0].OutcomeCount)
	}
}

func TestSQLiteOutcomeStoreLoadAllSessionScorecards(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	scorecards, outcomes, err := store.LoadAllSessionScorecards()
	if err != nil {
		t.Fatalf("LoadAllSessionScorecards failed: %v", err)
	}
	if len(scorecards) != 0 {
		t.Fatalf("expected empty scorecards slice, got %v", scorecards)
	}
	if outcomes != nil {
		t.Fatalf("expected nil outcomes for empty store, got %v", outcomes)
	}
}

func TestSQLiteOutcomeStoreLoadHumanInterventionsEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	loaded, err := store.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for empty interventions, got %v", loaded)
	}
}

func TestSQLiteOutcomeStoreLoadSessionSummariesEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries failed: %v", err)
	}
	if summaries != nil {
		t.Fatalf("expected nil for empty summaries, got %v", summaries)
	}
}

func TestSQLiteOutcomeStoreOutcomeStoreInterface(t *testing.T) {
	var _ OutcomeStore = (*SQLiteOutcomeStore)(nil)
}

func TestSQLiteOutcomeStoreWithFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.db"

	db, err := OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	outcome := domain.RecommendationOutcome{
		AgentID:     "agent-1",
		Symbol:      "2330",
		Side:        domain.SideBuy,
		Conviction:  80,
		TargetPrice: 1100,
		Window:      "2026-01",
		Hit:         true,
		RecordedAt:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}

	db.Close()

	db2, err := OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("OpenSQLiteDB second open failed: %v", err)
	}
	defer db2.Close()

	store2 := NewSQLiteOutcomeStore(db2)
	loaded2, err := store2.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes from reopened db failed: %v", err)
	}
	if len(loaded2) != 1 {
		t.Fatalf("expected 1 outcome after reopen, got %d", len(loaded2))
	}
}

func TestSQLiteOutcomeStorePreservesFactorScores(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	outcome := domain.RecommendationOutcome{
		AgentID:    "agent-1",
		Symbol:     "2330",
		Side:       domain.SideBuy,
		Conviction: 80,
		Window:     "2026-01",
		Hit:        true,
		RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		FactorScores: domain.FactorScores{
			Total:    75,
			Momentum: 80,
			Value:    70,
			Quality:  75,
		},
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}
	if loaded[0].FactorScores.Total != 75 {
		t.Errorf("expected factor score total 75, got %f", loaded[0].FactorScores.Total)
	}
}

func TestSQLiteOutcomeStorePreservesConvictionBreakdown(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	outcome := domain.RecommendationOutcome{
		AgentID:    "agent-1",
		Symbol:     "2330",
		Side:       domain.SideBuy,
		Conviction: 80,
		Window:     "2026-01",
		Hit:        true,
		RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		ConvictionBreakdown: &domain.ConvictionBreakdown{
			Base:  60,
			Floor: 40,
			Final: 80,
			Steps: []domain.ConvictionStep{
				{Rule: "momentum_boost", Delta: 10, Reason: "strong momentum"},
				{Rule: "sector_boost", Delta: 10, Reason: "tech sector outperformance"},
			},
		},
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}
	if loaded[0].ConvictionBreakdown == nil {
		t.Fatalf("expected conviction breakdown to be preserved, got nil")
	}
	if loaded[0].ConvictionBreakdown.Final != 80 {
		t.Errorf("expected final conviction 80, got %d", loaded[0].ConvictionBreakdown.Final)
	}
	if len(loaded[0].ConvictionBreakdown.Steps) != 2 {
		t.Errorf("expected 2 conviction steps, got %d", len(loaded[0].ConvictionBreakdown.Steps))
	}
}

func TestSQLiteOutcomeStoreUpdateSessionSummary(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	session := domain.ReplaySession{
		ID:          "session-20260115",
		Mode:        "backtest",
		Market:      "TWSE",
		SessionDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
	}

	summary1 := domain.SessionSummary{
		SessionID:    "session-20260115",
		OutcomeCount: 5,
		RecordedAt:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordSessionSummary(session, summary1); err != nil {
		t.Fatalf("RecordSessionSummary failed: %v", err)
	}

	summary2 := domain.SessionSummary{
		SessionID:    "session-20260115",
		OutcomeCount: 10,
		RecordedAt:   time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordSessionSummary(session, summary2); err != nil {
		t.Fatalf("RecordSessionSummary update failed: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary after upsert, got %d", len(summaries))
	}
	if summaries[0].OutcomeCount != 10 {
		t.Errorf("expected updated outcome count 10, got %d", summaries[0].OutcomeCount)
	}
}

func TestSQLiteOutcomeStoreLoadOutcomesEmpty(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for empty outcomes, got %v", loaded)
	}
}

func TestSQLiteOutcomeStoreRoundTrip(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	store := NewSQLiteOutcomeStore(db)

	session := domain.ReplaySession{
		ID:          "session-roundtrip",
		Mode:        "backtest",
		Market:      "TWSE",
		SessionDate: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC),
	}

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:             "agent-tech",
			Skill:               "sector-semiconductor",
			Layer:               domain.LayerSector,
			Symbol:              "2330",
			Side:                domain.SideBuy,
			Conviction:          90,
			TargetPrice:         1200,
			StopLossPrice:       1050,
			Window:              "2026-03",
			ForwardReturn:       0.08,
			BenchmarkDelta:      0.03,
			Hit:                 true,
			Reason:              "earnings beat",
			Price:               1100,
			PassedGuards:        true,
			GuardReason:         "",
			RecordedAt:          time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			FactorScores:        domain.FactorScores{Total: 85},
			ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 70, Floor: 50, Final: 90},
		},
		{
			AgentID:       "agent-finance",
			Skill:         "sector-financials",
			Layer:         domain.LayerSector,
			Symbol:        "2884",
			Side:          domain.SideBuy,
			Conviction:    65,
			TargetPrice:   600,
			Window:        "2026-03",
			ForwardReturn: -0.01,
			Hit:           false,
			Reason:        "NPL increase",
			Price:         580,
			PassedGuards:  true,
			RecordedAt:    time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := store.RecordSessionOutcomes(session, outcomes); err != nil {
		t.Fatalf("RecordSessionOutcomes failed: %v", err)
	}

	loaded, err := store.LoadSessionOutcomes("session-roundtrip")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(loaded))
	}

	scorecards, outcomes2, err := store.LoadAllSessionScorecards()
	if err != nil {
		t.Fatalf("LoadAllSessionScorecards failed: %v", err)
	}
	if len(scorecards) != 0 {
		t.Fatalf("expected empty scorecards, got %v", scorecards)
	}
	if len(outcomes2) != 2 {
		t.Fatalf("expected 2 outcomes in scorecards query, got %d", len(outcomes2))
	}

	_ = os.Stdout
}

// writeSessionOutcomeJSONL writes a rich per-session outcome JSONL file with
// full evaluation fields (Hit/ForwardReturn/Window) under baseDir, mirroring
// what ledger.Store produces.
func writeSessionOutcomeJSONL(t *testing.T, baseDir, sessionID string, outcomes []domain.RecommendationOutcome) {
	t.Helper()
	dir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create jsonl: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, o := range outcomes {
		if err := enc.Encode(o); err != nil {
			t.Fatalf("encode outcome: %v", err)
		}
	}
}

func TestSQLiteOutcomeStoreLoadOutcomesFromSessions_RichJSONLDelegation(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	baseDir := t.TempDir()
	rich := []domain.RecommendationOutcome{
		{
			AgentID: "agent-1", Skill: "sector-tech", Layer: domain.LayerSector,
			Symbol: "2330", Side: domain.SideBuy, Conviction: 80, Window: "2026-01-01",
			ForwardReturn: 0.052, Hit: true, RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			AgentID: "agent-2", Skill: "style-growth", Layer: domain.LayerStyle,
			Symbol: "2454", Side: domain.SideBuy, Conviction: 60, Window: "2026-01-02",
			ForwardReturn: -0.021, Hit: false, RecordedAt: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
		},
	}
	writeSessionOutcomeJSONL(t, baseDir, "session-20260115-daily", rich)

	// Truncated SQLite rows exist too — delegation must prefer JSONL.
	store := NewSQLiteOutcomeStore(db).WithJSONLBaseDir(baseDir)
	if err := store.RecordSessionOutcomes(domain.ReplaySession{ID: "session-20260115-daily"}, []domain.RecommendationOutcome{
		{AgentID: "agent-1", Symbol: "2330", Side: domain.SideBuy, Conviction: 80, RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("RecordSessionOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rich outcomes from JSONL, got %d", len(loaded))
	}
	bySymbol := map[string]domain.RecommendationOutcome{}
	for _, o := range loaded {
		bySymbol[o.Symbol] = o
	}
	got := bySymbol["2330"]
	if !got.Hit || got.ForwardReturn != 0.052 || got.Window != "2026-01-01" || got.Skill != "sector-tech" {
		t.Errorf("2330 evaluation fields lost: Hit=%v ForwardReturn=%v Window=%q Skill=%q", got.Hit, got.ForwardReturn, got.Window, got.Skill)
	}
	got = bySymbol["2454"]
	if got.Hit || got.ForwardReturn != -0.021 || got.Window != "2026-01-02" {
		t.Errorf("2454 evaluation fields lost: Hit=%v ForwardReturn=%v Window=%q", got.Hit, got.ForwardReturn, got.Window)
	}
}

func TestSQLiteOutcomeStoreLoadSessionOutcomes_RichJSONLDelegation(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	baseDir := t.TempDir()
	writeSessionOutcomeJSONL(t, baseDir, "session-20260115-daily", []domain.RecommendationOutcome{
		{
			AgentID: "agent-1", Symbol: "2330", Side: domain.SideBuy, Conviction: 80,
			Window: "2026-01-01", ForwardReturn: 0.052, Hit: true,
			RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	})

	store := NewSQLiteOutcomeStore(db).WithJSONLBaseDir(baseDir)
	loaded, err := store.LoadSessionOutcomes("session-20260115-daily")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}
	if !loaded[0].Hit || loaded[0].ForwardReturn != 0.052 {
		t.Errorf("evaluation fields lost: Hit=%v ForwardReturn=%v", loaded[0].Hit, loaded[0].ForwardReturn)
	}
}

func TestSQLiteOutcomeStoreLoadOutcomesFromSessions_FallbackEmptyJSONL(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB failed: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// baseDir with no sessions/ dir → JSONL delegation yields nothing,
	// so SQLite rows must still be returned (truncated but present).
	store := NewSQLiteOutcomeStore(db).WithJSONLBaseDir(t.TempDir())
	if err := store.RecordSessionOutcomes(domain.ReplaySession{ID: "session-A"}, []domain.RecommendationOutcome{
		{AgentID: "agent-a1", Symbol: "2330", Side: domain.SideBuy, Conviction: 80, RecordedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("RecordSessionOutcomes failed: %v", err)
	}

	loaded, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Symbol != "2330" {
		t.Fatalf("expected fallback to SQLite rows, got %+v", loaded)
	}
}
