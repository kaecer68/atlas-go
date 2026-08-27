package ledger

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// cleanupLedgerTestRows TRUNCATEs the ledger tables at the start of every
// test (M8). The previous combined WHERE mixed `agent_id` into tables that
// lack the column (trades, session_summaries, human_interventions,
// experiments); those DELETEs silently errored and leaked rows across runs
// ("expected 2 trades, got 4"). CI runs integration packages serially
// (-p 1), so a per-test TRUNCATE is a deterministic clean slate.
func cleanupLedgerTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{
		"recommendation_outcomes", "screening_rejects", "session_summaries",
		"trades", "human_interventions", "experiments",
	}
	run := func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+strings.Join(tables, ", ")); err != nil {
			t.Errorf("truncate ledger test tables: %v", err)
		}
	}
	run()
	t.Cleanup(run)
}

func TestPostgresLedgerStore_OutcomeRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupLedgerTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	o := domain.RecommendationOutcome{
		AgentID:       "pgsqltest-agent",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2330",
		Side:          domain.SideBuy,
		Conviction:    75,
		TargetPrice:   1100,
		StopLossPrice: 1000,
		Window:        "pgsqltest-global",
		ForwardReturn: 0.05,
		Hit:           true,
		Reason:        "test",
		Price:         1050,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	if err := store.RecordOutcomes([]domain.RecommendationOutcome{o}); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	found := false
	for _, got := range outcomes {
		if got.AgentID == "pgsqltest-agent" {
			found = true
			if got.Symbol != "2330" || !got.Hit || got.ForwardReturn != 0.05 || got.Layer != domain.LayerSector {
				t.Fatalf("outcome mismatch (metadata unmarshal failed): %+v", got)
			}
		}
	}
	if !found {
		t.Fatalf("pgsqltest-agent outcome not found in %d rows", len(outcomes))
	}

	// Session-scoped load.
	if err := store.RecordSessionOutcomes(domain.ReplaySession{ID: "pgsqltest-sess-1"}, []domain.RecommendationOutcome{
		{AgentID: "pgsqltest-agent2", Symbol: "2317", Side: domain.SideBuy, Window: "pgsqltest-sess-1", RecordedAt: time.Now()},
	}); err != nil {
		t.Fatalf("RecordSessionOutcomes: %v", err)
	}
	sessOutcomes, err := store.LoadSessionOutcomes("pgsqltest-sess-1")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(sessOutcomes) != 1 || sessOutcomes[0].Symbol != "2317" {
		t.Fatalf("session outcomes mismatch: %+v", sessOutcomes)
	}

	// Scorecards aggregate.
	scorecards, all, err := store.LoadAllSessionScorecards()
	if err != nil {
		t.Fatalf("LoadAllSessionScorecards: %v", err)
	}
	if len(all) == 0 || scorecards == nil {
		t.Fatalf("expected non-empty scorecard aggregation, got %d outcomes", len(all))
	}
}

func TestPostgresLedgerStore_ScreeningAndSummaryRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupLedgerTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	// Screening rejects.
	rejects := []domain.ScreeningReject{
		{
			SessionID: "pgsqltest-sess-2", Symbol: "0050", AgentID: "pgsqltest-ag", Skill: "sector-tech",
			Criterion: "vol", CriterionLabel: "high volatility", Threshold: "0.3", ActualValue: "0.5",
			RecordedAt: time.Now(), FactorScores: domain.FactorScores{},
		},
	}
	if err := store.RecordSessionScreeningRejects("pgsqltest-sess-2", rejects); err != nil {
		t.Fatalf("RecordSessionScreeningRejects: %v", err)
	}
	gotRejects, err := store.LoadSessionScreeningRejects("pgsqltest-sess-2")
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects: %v", err)
	}
	if len(gotRejects) != 1 || gotRejects[0].Symbol != "0050" {
		t.Fatalf("screening rejects mismatch: %+v", gotRejects)
	}

	// Session summary.
	summary := domain.SessionSummary{
		SessionID:      "pgsqltest-sess-2",
		Regime:         domain.RegimeRiskOn,
		OrderCount:     3,
		PositionCount:  2,
		EndingCash:     1000,
		PortfolioValue: 2000,
		OutcomeCount:   5,
		GuardOutcomes:  []domain.GuardOutcome{{GuardID: "g1", Passed: true}},
		RecordedAt:     time.Now(),
	}
	if err := store.RecordSessionSummary(domain.ReplaySession{ID: "pgsqltest-sess-2"}, summary); err != nil {
		t.Fatalf("RecordSessionSummary: %v", err)
	}
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	found := false
	for _, s := range summaries {
		if s.SessionID == "pgsqltest-sess-2" {
			found = true
			if s.PortfolioValue != 2000 || len(s.GuardOutcomes) != 1 || s.Regime != domain.RegimeRiskOn {
				t.Fatalf("summary mismatch: %+v", s)
			}
		}
	}
	if !found {
		t.Fatalf("pgsqltest-sess-2 summary not found")
	}
}

func TestPostgresLedgerStore_TradesRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupLedgerTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	ts := time.Date(2026, 8, 1, 0, 52, 7, 0, time.UTC)
	trades := []domain.TradeRecord{
		{
			TradeID: "pgsqltest-t1", SessionID: "pgsqltest-sess-3", Symbol: "00713.TW", Side: domain.SideBuy,
			Quantity: 100, Price: 60.7, Amount: 6070, Reason: "optimized_position|weight:95.00%", Timestamp: ts,
		},
		{
			TradeID: "pgsqltest-t2", SessionID: "pgsqltest-sess-3", Symbol: "0056.TW", Side: domain.SideSell,
			Quantity: 50, Price: 38.5, Amount: 1925, Reason: "rebalance", Timestamp: ts.Add(time.Hour),
		},
	}
	if err := store.RecordSessionTrades("pgsqltest-sess-3", trades); err != nil {
		t.Fatalf("RecordSessionTrades: %v", err)
	}
	got, err := store.LoadSessionTrades("pgsqltest-sess-3")
	if err != nil {
		t.Fatalf("LoadSessionTrades: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(got))
	}
	// ORDER BY timestamp ASC.
	if got[0].TradeID != "pgsqltest-t1" || got[1].TradeID != "pgsqltest-t2" {
		t.Fatalf("trade order mismatch: %+v", got)
	}
	if got[0].Side != domain.SideBuy || !got[0].Timestamp.Equal(ts) {
		t.Fatalf("trade field mismatch: %+v", got[0])
	}

	all, err := store.LoadAllSessionTrades()
	if err != nil {
		t.Fatalf("LoadAllSessionTrades: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected >=2 all trades, got %d", len(all))
	}

	// Empty no-op.
	if err := store.RecordSessionTrades("pgsqltest-sess-3", nil); err != nil {
		t.Fatalf("RecordSessionTrades(nil) should no-op: %v", err)
	}
}

func TestPostgresLedgerStore_HumanInterventionRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupLedgerTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	hi := domain.HumanIntervention{
		ID:            "pgsqltest-hi-1",
		Type:          "pause_agent",
		TargetAgentID: "pgsqltest-ag",
		Reason:        "test",
		Operator:      "tester",
		SessionID:     "pgsqltest-sess-4",
		RecordedAt:    time.Now(),
	}
	if err := store.RecordHumanIntervention(hi); err != nil {
		t.Fatalf("RecordHumanIntervention: %v", err)
	}
	all, err := store.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions: %v", err)
	}
	found := false
	for _, got := range all {
		if got.ID == "pgsqltest-hi-1" {
			found = true
			if got.Operator != "tester" || got.Type != "pause_agent" {
				t.Fatalf("intervention mismatch: %+v", got)
			}
		}
	}
	if !found {
		t.Fatalf("pgsqltest-hi-1 not found")
	}
}
