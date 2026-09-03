//go:build integration

package ledger

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/portfolio"
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
		AgentID:            "pgsqltest-agent",
		Skill:              "sector-tech",
		Layer:              domain.LayerSector,
		Symbol:             "2330",
		Side:               domain.SideBuy,
		Conviction:         75,
		TargetPrice:        1100,
		StopLossPrice:      1000,
		Window:             "pgsqltest-global",
		ForwardReturn:      0.05,
		Hit:                true,
		Reason:             "test",
		Price:              1050,
		PassedGuards:       true,
		RecordedAt:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		MarketPeriod:       "plateau",
		MarketPeriodSource: "live",
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
			if got.MarketPeriod != "plateau" || got.MarketPeriodSource != "live" {
				t.Errorf("market_period columns lost: got %q/%q, want plateau/live",
					got.MarketPeriod, got.MarketPeriodSource)
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

// scorecardOutcomeTuple is the canonical 8-field projection the observatory
// consumes (AgentID/Skill/Layer/Window/ForwardReturn/Hit/RecordedAt/Regime).
// #1780 Phase 1 equivalence compares slim vs full reads on exactly these
// fields (k3 review B4: per-field comparison, not bit-for-bit).
type scorecardOutcomeTuple struct {
	agentID       string
	skill         string
	layer         string
	window        string
	forwardReturn float64
	hit           bool
	regime        string
	recordedAtNS  int64
}

func scorecardTuple(o domain.RecommendationOutcome) scorecardOutcomeTuple {
	return scorecardOutcomeTuple{
		agentID:       o.AgentID,
		skill:         o.Skill,
		layer:         string(o.Layer),
		window:        o.Window,
		forwardReturn: o.ForwardReturn,
		hit:           o.Hit,
		regime:        o.Regime,
		recordedAtNS:  o.RecordedAt.UnixNano(),
	}
}

// TestPostgresLedgerStore_ScorecardSlimEquivalence proves the #1780 Phase 1
// slim projection (LoadScorecardOutcomes) returns the same 8 scalar fields as
// the full metadata read (LoadOutcomesFromSessions → scanPGOutcomes) on
// production-shaped rows:
//   - metadata carries ns-precision recorded_at (B3: timestamptz rounds to
//     µs, so slim must parse the RFC3339Nano text, not the cast column);
//   - heavy metadata fields are present and must not leak/alter the 8 fields;
//   - regime is absent on some rows (omitempty at write time) and both paths
//     must yield the zero value.
func TestPostgresLedgerStore_ScorecardSlimEquivalence(t *testing.T) {
	pool := connectTestPG(t)
	cleanupLedgerTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	// ns-precision base so µs rounding WOULD change ordering if slim used the
	// timestamptz cast. Two rows per agent pair are < 1µs apart (B3 tie case).
	base := time.Date(2026, 6, 1, 0, 0, 0, 123456789, time.UTC)
	mkOutcome := func(agent, window string, i int, ret float64, hit bool, regime string) domain.RecommendationOutcome {
		return domain.RecommendationOutcome{
			AgentID:       agent,
			Skill:         "sector-tech",
			Layer:         domain.LayerSector,
			Symbol:        "2330.TW",
			Side:          domain.SideBuy,
			Conviction:    80,
			TargetPrice:   1100,
			StopLossPrice: 1000,
			Window:        window,
			ForwardReturn: ret,
			Hit:           hit,
			Regime:        regime,
			Reason:        "equivalence-fixture",
			Price:         1050,
			PassedGuards:  true,
			RecordedAt:    base.Add(time.Duration(i) * 200 * time.Nanosecond),
			// Heavy fields — must be ignored by both paths' scorecard math.
			FactorScores:      domain.FactorScores{Value: 0.7, Quality: 0.3},
			SupportingEvents:  []string{"evt-1", "evt-2"},
			ParameterSnapshot: &shared.ParameterSnapshot{FactorWeights: map[string]float64{"value": 0.6}, ConfigVersion: "v1"},
		}
	}
	var fixture []domain.RecommendationOutcome
	// agent-a: 3 windows × 3 outcomes; agent-b: 3 windows × 2 outcomes, no regime.
	for i := 0; i < 9; i++ {
		w := fmt.Sprintf("2026-06-%02d", i/3+1)
		fixture = append(fixture, mkOutcome("slim-equiv-a", w, i, 0.01+0.001*float64(i), i%2 == 0, "risk_on"))
	}
	for i := 0; i < 6; i++ {
		w := fmt.Sprintf("2026-06-%02d", i/2+1)
		fixture = append(fixture, mkOutcome("slim-equiv-b", w, i, -0.005+0.0005*float64(i), i%3 == 0, ""))
	}
	if err := store.RecordOutcomes(fixture); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	// Raw row whose metadata omits recorded_at (column-only fallback path) —
	// must still match the full read, which also keeps the column value when
	// the metadata key is absent.
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO recommendation_outcomes
		        (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
		VALUES ($1, '', '2317.TW', 'slim-equiv-raw', 'sector', 70, true, 'raw', 100,
		        '{"agent_id":"slim-equiv-raw","skill":"value","layer":"style","window":"2026-05-01",
		          "forward_return":0.02,"hit":true}')`,
		base.Add(50*time.Microsecond)); err != nil {
		t.Fatalf("insert metadata-without-recorded_at row: %v", err)
	}
	// Raw row with EMPTY metadata '{}' — B2 NULL-safety: the COALESCE'd slim
	// SQL must scan without error instead of crashing on NULL ::float8/::boolean.
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO recommendation_outcomes
		        (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
		VALUES ($1, 'sess-empty-meta', '0050.TW', 'slim-equiv-empty', 'style', 60, false, '', 100, '{}'::jsonb)`,
		base.Add(60*time.Microsecond)); err != nil {
		t.Fatalf("insert empty-metadata row: %v", err)
	}

	full, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions: %v", err)
	}
	slim, err := store.LoadScorecardOutcomes()
	if err != nil {
		t.Fatalf("LoadScorecardOutcomes (slim) must not error on NULL-able metadata: %v", err)
	}

	tupleLess := func(a, b scorecardOutcomeTuple) int {
		switch {
		case a.agentID < b.agentID:
			return -1
		case a.agentID > b.agentID:
			return 1
		case a.window < b.window:
			return -1
		case a.window > b.window:
			return 1
		case a.recordedAtNS < b.recordedAtNS:
			return -1
		case a.recordedAtNS > b.recordedAtNS:
			return 1
		case a.skill < b.skill:
			return -1
		case a.skill > b.skill:
			return 1
		default:
			return 0
		}
	}
	toTuples := func(outcomes []domain.RecommendationOutcome, skipAgent string) []scorecardOutcomeTuple {
		var out []scorecardOutcomeTuple
		for _, o := range outcomes {
			if o.AgentID == skipAgent {
				continue // empty-metadata row: layer/window semantics differ by design (see below)
			}
			out = append(out, scorecardTuple(o))
		}
		slices.SortFunc(out, tupleLess)
		return out
	}
	fullTuples := toTuples(full, "slim-equiv-empty")
	slimTuples := toTuples(slim, "slim-equiv-empty")
	if len(fullTuples) != len(slimTuples) {
		t.Fatalf("row count mismatch: full=%d slim=%d", len(fullTuples), len(slimTuples))
	}
	for i := range fullTuples {
		if fullTuples[i] != slimTuples[i] {
			t.Errorf("8-field mismatch at #%d:\n  full=%+v\n  slim=%+v", i, fullTuples[i], slimTuples[i])
		}
	}

	// B3: the sub-µs tie must keep ns precision on BOTH paths (slim parses the
	// RFC3339Nano text instead of a µs timestamptz cast), and SortOutcomesByTime
	// must therefore order the two rows 200ns apart identically.
	var fullA, slimA []domain.RecommendationOutcome
	for _, o := range full {
		if o.AgentID == "slim-equiv-a" {
			fullA = append(fullA, o)
		}
	}
	for _, o := range slim {
		if o.AgentID == "slim-equiv-a" {
			slimA = append(slimA, o)
		}
	}
	sortedFull := portfolio.SortOutcomesByTime(fullA)
	sortedSlim := portfolio.SortOutcomesByTime(slimA)
	for i := range sortedFull {
		if !sortedFull[i].RecordedAt.Equal(sortedSlim[i].RecordedAt) {
			t.Fatalf("sorted RecordedAt mismatch at #%d: full=%v slim=%v", i, sortedFull[i].RecordedAt, sortedSlim[i].RecordedAt)
		}
		if i > 0 && !sortedSlim[i-1].RecordedAt.Before(sortedSlim[i].RecordedAt) {
			t.Errorf("slim order not strictly chronological at #%d (ns lost?): %v vs %v",
				i, sortedSlim[i-1].RecordedAt, sortedSlim[i].RecordedAt)
		}
	}
	if !sortedSlim[0].RecordedAt.Equal(fixture[0].RecordedAt) {
		t.Errorf("slim lost ns precision: got %v want %v", sortedSlim[0].RecordedAt, fixture[0].RecordedAt)
	}

	// B4: after the deterministic-windows fix, both paths must yield identical
	// scorecards (compare all fields except LastUpdatedAt).
	fullSCs := BuildScorecards(full)
	slimSCs := BuildScorecards(slim)
	if len(fullSCs) != len(slimSCs) {
		t.Fatalf("scorecard count mismatch: full=%d slim=%d", len(fullSCs), len(slimSCs))
	}
	for i := range fullSCs {
		fullSCs[i].LastUpdatedAt = time.Time{}
		slimSCs[i].LastUpdatedAt = time.Time{}
	}
	// Scorecards come back sorted by SharpeLike asc (AgentID tiebreak) — match
	// by AgentID so ordering differences cannot hide field differences.
	fullByAgent := map[string]domain.Scorecard{}
	for _, sc := range fullSCs {
		fullByAgent[sc.AgentID] = sc
	}
	for _, sc := range slimSCs {
		if sc.AgentID == "slim-equiv-empty" {
			continue // documented divergence for the hand-written '{}' row, asserted below
		}
		want, ok := fullByAgent[sc.AgentID]
		if !ok {
			t.Fatalf("slim scorecard agent %q missing from full path", sc.AgentID)
		}
		if !reflect.DeepEqual(sc, want) {
			t.Errorf("scorecard mismatch for agent %q:\n  slim=%+v\n  full=%+v", sc.AgentID, sc, want)
		}
	}

	// B2 empty-metadata row: slim must load it (no NULL scan error). Layer and
	// window are metadata-only in the slim projection, so they read '' for a
	// '{}' row while the full read pre-fills them from the agent_layer /
	// session_id columns — production rows written by Record* always carry all
	// 8 keys in metadata, so this divergence only affects hand-written legacy
	// rows and is the documented "metadata is the source of truth" semantics.
	var emptyFull, emptySlim *domain.RecommendationOutcome
	for i := range full {
		if full[i].AgentID == "slim-equiv-empty" {
			emptyFull = &full[i]
			break
		}
	}
	for i := range slim {
		if slim[i].AgentID == "slim-equiv-empty" {
			emptySlim = &slim[i]
			break
		}
	}
	if emptyFull == nil || emptySlim == nil {
		t.Fatal("empty-metadata row missing from one of the paths")
	}
	if emptySlim.Skill != "" || emptySlim.Layer != "" || emptySlim.Window != "" ||
		emptySlim.ForwardReturn != 0 || emptySlim.Hit || emptySlim.Regime != "" {
		t.Errorf("slim empty-metadata row: expected zero scalar fields, got %+v", emptySlim)
	}
	if emptySlim.RecordedAt.UnixNano() != emptyFull.RecordedAt.UnixNano() {
		t.Errorf("slim empty-metadata row recorded_at should fall back to the time column (µs): slim=%v full=%v",
			emptySlim.RecordedAt, emptyFull.RecordedAt)
	}
}
