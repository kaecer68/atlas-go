package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// newNilPGDualWrite creates a DualWriteRepository with nil PostgreSQL backend.
// This verifies that all methods handle nil PG gracefully and fall back to JSONL stores.
func newNilPGDualWrite() *DualWriteRepository {
	return &DualWriteRepository{
		pg: nil,
		jsonl: &JSONLRepository{
			alertStore:             &mockAlertStore{},
			metricsStore:           &mockMetricsStore{},
			outcomeStore:           &mockOutcomeStore{},
			screeningRejectStore:   &mockScreeningRejectStore{},
			sessionSummaryStore:    &mockSessionSummaryStore{},
			humanInterventionStore: &mockHumanInterventionStore{},
		},
	}
}

func TestDualWriteNilPG_Metrics_NoPanic(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	// Record — writes to JSONL even with nil PG
	if err := repo.Record(ctx, "test_metric", 42.0, map[string]string{"agent_id": "a1"}); err != nil {
		t.Errorf("Record with nil PG: %v", err)
	}

	// QueryRange — returns nil with nil PG
	points, err := repo.QueryRange(ctx, "test_metric", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Errorf("QueryRange with nil PG: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("Expected empty QueryRange result, got %d", len(points))
	}

	// QueryLatest — returns nil with nil PG
	latest, err := repo.QueryLatest(ctx, "test_metric", nil)
	if err != nil {
		t.Errorf("QueryLatest with nil PG: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil from QueryLatest with nil PG")
	}

	// Aggregate — returns 0 with nil PG
	val, err := repo.Aggregate(ctx, "test_metric", time.Now().Add(-time.Hour), time.Now(), "sum")
	if err != nil {
		t.Errorf("Aggregate with nil PG: %v", err)
	}
	if val != 0 {
		t.Errorf("Expected 0 from Aggregate, got %f", val)
	}
}

func TestDualWriteNilPG_Metrics_SaveSnapshotLoadToday(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	snap := &MetricsSnapshot{
		ScreeningTotal:  200,
		ScreeningPassed: 150,
		ScreeningRate:   0.75,
		Timestamp:       time.Now(),
	}

	if err := repo.SaveSnapshot(ctx, snap); err != nil {
		t.Errorf("SaveSnapshot with nil PG: %v", err)
	}

	// LoadToday — should return from JSONL fallback
	loaded, err := repo.LoadToday(ctx)
	if err != nil {
		t.Errorf("LoadToday with nil PG: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil from LoadToday (JSONL fallback)")
	}
	if loaded.ScreeningTotal != 200 {
		t.Errorf("Expected ScreeningTotal 200, got %d", loaded.ScreeningTotal)
	}
}

func TestDualWriteNilPG_Metrics_LoadRecent(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	for i := range 3 {
		_ = repo.SaveSnapshot(ctx, &MetricsSnapshot{
			ScreeningTotal: int64(i * 100),
			Timestamp:      time.Now(),
		})
	}

	recent, err := repo.LoadRecent(ctx, 2)
	if err != nil {
		t.Errorf("LoadRecent with nil PG: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("Expected 2 recent snapshots, got %d", len(recent))
	}
}

func TestDualWriteNilPG_Alerts_NoPanic(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	alert := domain.AlertRecord{
		ID: "jsonl-alert-1", Timestamp: time.Now(), Rule: "r1", Severity: "warning", Message: "test",
	}

	if err := repo.SaveAlert(ctx, alert); err != nil {
		t.Errorf("SaveAlert with nil PG: %v", err)
	}

	// LoadAllAlerts — JSONL fallback should work
	alerts, err := repo.LoadAllAlerts(ctx, 10)
	if err != nil {
		t.Errorf("LoadAllAlerts with nil PG: %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert from JSONL fallback, got %d", len(alerts))
	}

	// AcknowledgeAlert — works via JSONL
	if err := repo.AcknowledgeAlert(ctx, "jsonl-alert-1", "tester"); err != nil {
		t.Errorf("AcknowledgeAlert with nil PG: %v", err)
	}
}

func TestDualWriteNilPG_Alerts_LoadUnacknowledged(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	_ = repo.SaveAlert(ctx, domain.AlertRecord{
		ID: "ack-1", Timestamp: time.Now(), Rule: "r1", Severity: "info", Message: "unacked",
	})
	_ = repo.SaveAlert(ctx, domain.AlertRecord{
		ID: "ack-2", Timestamp: time.Now(), Rule: "r2", Severity: "info", Message: "will ack",
	})

	// Acknowledge one
	_ = repo.AcknowledgeAlert(ctx, "ack-2", "tester")

	unacked, err := repo.LoadUnacknowledgedAlerts(ctx)
	if err != nil {
		t.Errorf("LoadUnacknowledgedAlerts with nil PG: %v", err)
	}
	if len(unacked) != 1 {
		t.Errorf("Expected 1 unacknowledged alert, got %d", len(unacked))
	}
	if unacked[0].ID != "ack-1" {
		t.Errorf("Expected unacked alert 'ack-1', got %q", unacked[0].ID)
	}
}

func TestDualWriteNilPG_Alerts_PGOnlyReturnsNil(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	// These methods rely entirely on PG and should return nil/nil with nil PG
	bySeverity, err := repo.LoadAlertsBySeverity(ctx, "critical", 10)
	if err != nil {
		t.Errorf("LoadAlertsBySeverity with nil PG: %v", err)
	}
	if bySeverity != nil {
		t.Error("Expected nil from LoadAlertsBySeverity with nil PG")
	}

	byTime, err := repo.LoadAlertsByTimeRange(ctx, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Errorf("LoadAlertsByTimeRange with nil PG: %v", err)
	}
	if byTime != nil {
		t.Error("Expected nil from LoadAlertsByTimeRange with nil PG")
	}

	sessions, err := repo.QuerySessions(ctx)
	if err != nil {
		t.Errorf("QuerySessions with nil PG: %v", err)
	}
	if sessions != nil {
		t.Error("Expected nil from QuerySessions with nil PG")
	}
}

func TestDualWriteNilPG_Outcomes_RecordAndQuery(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "jsonl-sess-1", Symbol: "2330.TW", AgentID: "a1"},
		{Window: "jsonl-sess-1", Symbol: "2317.TW", AgentID: "a1"},
	}

	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Errorf("RecordOutcomes with nil PG: %v", err)
	}

	// QueryOutcomesBySession — falls back to JSONL
	loaded, err := repo.QueryOutcomesBySession(ctx, "jsonl-sess-1")
	if err != nil {
		t.Errorf("QueryOutcomesBySession with nil PG: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("Expected 2 outcomes from JSONL fallback, got %d", len(loaded))
	}

	// QueryAllOutcomes — always JSONL
	all, err := repo.QueryAllOutcomes(ctx)
	if err != nil {
		t.Errorf("QueryAllOutcomes with nil PG: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("Expected 2 outcomes from QueryAllOutcomes, got %d", len(all))
	}
}

func TestDualWriteNilPG_Outcomes_PGOnlyReturnsZero(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	bySymbol, err := repo.QueryOutcomesBySymbol(ctx, "2330.TW", start, end)
	if err != nil {
		t.Errorf("QueryOutcomesBySymbol with nil PG: %v", err)
	}
	if bySymbol != nil {
		t.Error("Expected nil from QueryOutcomesBySymbol with nil PG")
	}

	byAgent, err := repo.QueryOutcomesByAgent(ctx, "a1", start, end)
	if err != nil {
		t.Errorf("QueryOutcomesByAgent with nil PG: %v", err)
	}
	if byAgent != nil {
		t.Error("Expected nil from QueryOutcomesByAgent with nil PG")
	}

	rate, err := repo.QueryPassRate(ctx, "a1", time.Hour)
	if err != nil {
		t.Errorf("QueryPassRate with nil PG: %v", err)
	}
	if rate != 0 {
		t.Errorf("Expected 0 from QueryPassRate, got %f", rate)
	}

	top, err := repo.QueryTopSymbols(ctx, 5, start, end)
	if err != nil {
		t.Errorf("QueryTopSymbols with nil PG: %v", err)
	}
	if top != nil {
		t.Error("Expected nil from QueryTopSymbols with nil PG")
	}
}

func TestDualWriteNilPG_Outcomes_SessionFlows(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	session := domain.ReplaySession{ID: "jsonl-sess-2"}
	outcomes := []domain.RecommendationOutcome{
		{Window: "jsonl-sess-2", Symbol: "2330.TW", AgentID: "a1"},
	}

	// RecordSessionOutcomes — JSONL only
	if err := repo.RecordSessionOutcomes(ctx, session, outcomes); err != nil {
		t.Errorf("RecordSessionOutcomes with nil PG: %v", err)
	}

	// RecordExperiment — JSONL only
	if err := repo.RecordExperiment(ctx, domain.ExperimentRecord{
		ID: "exp-jsonl-1", TargetAgentID: "a1",
	}); err != nil {
		t.Errorf("RecordExperiment with nil PG: %v", err)
	}

	// RecordSessionExperiment — JSONL only
	if err := repo.RecordSessionExperiment(ctx, session, domain.ExperimentRecord{
		ID: "exp-jsonl-1", TargetAgentID: "a1",
	}); err != nil {
		t.Errorf("RecordSessionExperiment with nil PG: %v", err)
	}
}

func TestDualWriteNilPG_CapitalFlow_NoPanic(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	// RecordCapitalFlow — returns nil (PG-only)
	if err := repo.RecordCapitalFlow(ctx, "foreign", 100, 500, 400); err != nil {
		t.Errorf("RecordCapitalFlow with nil PG: %v", err)
	}

	latest, err := repo.QueryLatestCapitalFlow(ctx, "foreign")
	if err != nil {
		t.Errorf("QueryLatestCapitalFlow with nil PG: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil from QueryLatestCapitalFlow with nil PG")
	}

	rangeRecs, err := repo.QueryCapitalFlowRange(ctx, "foreign", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Errorf("QueryCapitalFlowRange with nil PG: %v", err)
	}
	if rangeRecs != nil {
		t.Error("Expected nil from QueryCapitalFlowRange with nil PG")
	}
}

func TestDualWriteNilPG_ExportStats_NoPanic(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	if err := repo.SaveExportStats(ctx, 114, 5, 1000, 800, 200); err != nil {
		t.Errorf("SaveExportStats with nil PG: %v", err)
	}

	latest, err := repo.QueryLatestExportStats(ctx)
	if err != nil {
		t.Errorf("QueryLatestExportStats with nil PG: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil from QueryLatestExportStats with nil PG")
	}

	rec, err := repo.QueryExportStatsByYearMonth(ctx, 114, 5)
	if err != nil {
		t.Errorf("QueryExportStatsByYearMonth with nil PG: %v", err)
	}
	if rec != nil {
		t.Error("Expected nil from QueryExportStatsByYearMonth with nil PG")
	}
}

func TestDualWriteNilPG_Audit_NoPanic(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	// ScreeningRejects
	rejects := []domain.ScreeningReject{
		{SessionID: "jsonl-sess-3", Symbol: "2330.TW", AgentID: "a1", Criterion: "PE too high", RecordedAt: time.Now()},
	}
	if err := repo.RecordScreeningRejects(ctx, "jsonl-sess-3", rejects); err != nil {
		t.Errorf("RecordScreeningRejects with nil PG: %v", err)
	}

	loadedRejects, err := repo.QueryScreeningRejectsBySession(ctx, "jsonl-sess-3")
	if err != nil {
		t.Errorf("QueryScreeningRejectsBySession with nil PG: %v", err)
	}
	// mockScreeningRejectStore returns nil/nil, so length should be 0
	if len(loadedRejects) != 0 {
		t.Errorf("Expected 0 screening rejects from mock, got %d", len(loadedRejects))
	}

	// SessionSummaries
	summary := domain.SessionSummary{
		SessionID: "jsonl-sess-4", OrderCount: 30, PositionCount: 10, EndingCash: 10000.0, PortfolioValue: 50000.0, RecordedAt: time.Now(),
	}
	if err := repo.SaveSessionSummary(ctx, summary); err != nil {
		t.Errorf("SaveSessionSummary with nil PG: %v", err)
	}

	loadedSummary, err := repo.LoadSessionSummary(ctx, "jsonl-sess-4")
	if err != nil {
		t.Errorf("LoadSessionSummary with nil PG: %v", err)
	}
	// JSONL fallback returns the recorded summary (PG was nil, so we read from JSONL).
	if loadedSummary == nil {
		t.Fatal("Expected non-nil from LoadSessionSummary with nil PG (JSONL fallback)")
	}
	if loadedSummary.SessionID != "jsonl-sess-4" {
		t.Errorf("SessionID = %q, want %q", loadedSummary.SessionID, "jsonl-sess-4")
	}
	if loadedSummary.OrderCount != 30 {
		t.Errorf("OrderCount = %d, want 30 (from JSONL record)", loadedSummary.OrderCount)
	}

	allSummaries, err := repo.LoadAllSessionSummaries(ctx)
	if err != nil {
		t.Errorf("LoadAllSessionSummaries with nil PG: %v", err)
	}
	if len(allSummaries) != 1 {
		t.Fatalf("Expected 1 summary from JSONL fallback, got %d", len(allSummaries))
	}
	if allSummaries[0].SessionID != "jsonl-sess-4" {
		t.Errorf("allSummaries[0].SessionID = %q, want %q", allSummaries[0].SessionID, "jsonl-sess-4")
	}

	// HumanInterventions
	if err := repo.RecordHumanIntervention(ctx, domain.HumanIntervention{
		ID: "hint-jsonl-1", Type: "approve", Reason: "manual override", Operator: "tester", SessionID: "jsonl-sess-5", RecordedAt: time.Now(),
	}); err != nil {
		t.Errorf("RecordHumanIntervention with nil PG: %v", err)
	}

	interventions, err := repo.LoadHumanInterventions(ctx)
	if err != nil {
		t.Errorf("LoadHumanInterventions with nil PG: %v", err)
	}
	// mockHumanInterventionStore returns nil/nil
	if len(interventions) != 0 {
		t.Errorf("Expected 0 interventions from mock, got %d", len(interventions))
	}
}

func TestDualWriteNilPG_RecordSessionSummary(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	session := domain.ReplaySession{ID: "jsonl-rs-1"}
	summary := domain.SessionSummary{
		SessionID: "jsonl-rs-1", Regime: domain.RegimeRiskOn, OrderCount: 50, PositionCount: 15, EndingCash: 20000.0, PortfolioValue: 100000.0, RecordedAt: time.Now(),
	}

	// RecordSessionSummary — writes to JSONL, skips PG
	if err := repo.RecordSessionSummary(ctx, session, summary); err != nil {
		t.Errorf("RecordSessionSummary with nil PG: %v", err)
	}
}

func TestDualWriteNilPG_QueryAllSessionScorecards(t *testing.T) {
	repo := newNilPGDualWrite()
	ctx := context.Background()

	// QueryAllSessionScorecards — always JSONL
	scorecards, outcomes, err := repo.QueryAllSessionScorecards(ctx)
	if err != nil {
		t.Errorf("QueryAllSessionScorecards with nil PG: %v", err)
	}
	if scorecards != nil {
		t.Error("Expected nil scorecards from mock")
	}
	if outcomes != nil {
		t.Error("Expected nil outcomes from mock")
	}
}
