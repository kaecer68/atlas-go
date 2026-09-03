package ledger

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// newPeriodFixtureOutcomes builds two outcomes carrying the Phase 2 PR-2a
// market-period fields plus one legacy outcome without them.
func newPeriodFixtureOutcomes() []domain.RecommendationOutcome {
	recorded := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return []domain.RecommendationOutcome{
		{
			AgentID:            "agent-period-1",
			Skill:              "sector-tech",
			Layer:              domain.LayerSector,
			Symbol:             "2330",
			Side:               domain.SideBuy,
			Conviction:         80,
			Window:             "2026-04-01",
			ForwardReturn:      0.05,
			Hit:                true,
			RecordedAt:         recorded,
			IsSynthetic:        true,
			Regime:             "RISK_ON",
			MarketPeriod:       "bull",
			MarketPeriodSource: "live",
		},
		{
			AgentID:            "agent-period-2",
			Skill:              "style-growth",
			Layer:              domain.LayerStyle,
			Symbol:             "0050",
			Side:               domain.SideBuy,
			Conviction:         60,
			Window:             "2020-06-15",
			ForwardReturn:      -0.03,
			Hit:                false,
			RecordedAt:         time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC),
			IsSynthetic:        true,
			Regime:             "RISK_OFF",
			MarketPeriod:       "black_swan",
			MarketPeriodSource: "synthetic",
		},
	}
}

// TestInitSchemaAddsOutcomePeriodColumns verifies InitSchema provisions the
// Phase 2 PR-2a market_period columns on the outcomes table.
func TestInitSchemaAddsOutcomePeriodColumns(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	for _, col := range []string{"market_period", "market_period_source"} {
		if !columnExists(t, db, "outcomes", col) {
			t.Errorf("outcomes.%s column missing after InitSchema", col)
		}
	}
}

func columnExists(t *testing.T, db *sql.DB, table, col string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == col {
			return true
		}
	}
	return false
}

// TestSQLiteOutcomeStoreMarketPeriodRoundTrip verifies the two new columns
// survive RecordOutcomes/LoadOutcomes and RecordSessionOutcomes/
// LoadSessionOutcomes round trips (SQLite read path, no JSONL delegation).
func TestSQLiteOutcomeStoreMarketPeriodRoundTrip(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := NewSQLiteOutcomeStore(db)

	outcomes := newPeriodFixtureOutcomes()
	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("record global outcomes: %v", err)
	}
	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("load global outcomes: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded outcomes, got %d", len(loaded))
	}
	byAgent := map[string]domain.RecommendationOutcome{}
	for _, o := range loaded {
		byAgent[o.AgentID] = o
	}
	if got := byAgent["agent-period-1"]; got.MarketPeriod != "bull" || got.MarketPeriodSource != "live" {
		t.Errorf("agent-period-1: got period=%q source=%q, want bull/live", got.MarketPeriod, got.MarketPeriodSource)
	}
	if got := byAgent["agent-period-2"]; got.MarketPeriod != "black_swan" || got.MarketPeriodSource != "synthetic" {
		t.Errorf("agent-period-2: got period=%q source=%q, want black_swan/synthetic", got.MarketPeriod, got.MarketPeriodSource)
	}

	// Session-scoped round trip.
	session := domain.ReplaySession{ID: "session-20260401-daily", SessionDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.RecordSessionOutcomes(session, outcomes[:1]); err != nil {
		t.Fatalf("record session outcomes: %v", err)
	}
	sess, err := store.LoadSessionOutcomes(session.ID)
	if err != nil {
		t.Fatalf("load session outcomes: %v", err)
	}
	if len(sess) != 1 {
		t.Fatalf("expected 1 session outcome, got %d", len(sess))
	}
	if sess[0].MarketPeriod != "bull" || sess[0].MarketPeriodSource != "live" {
		t.Errorf("session outcome: got period=%q source=%q, want bull/live", sess[0].MarketPeriod, sess[0].MarketPeriodSource)
	}
}

// TestSQLiteOutcomeStoreLegacyRowReadsEmptyPeriod verifies rows written
// before PR-2a (no market_period columns) scan back with empty fields —
// they become "unknown" matrix cells, never garbage values.
func TestSQLiteOutcomeStoreLegacyRowReadsEmptyPeriod(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	// Simulate a legacy row: insert with the pre-PR-2a column list only.
	_, err = db.Exec(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, conviction, regime, timestamp, passed_guards,
			layer, forward_return, window, hit, is_synthetic, true_regime)
		VALUES ('', '2330', 'legacy-agent', 'BUY', 80, 'RISK_ON', '2026-01-15T00:00:00Z', 1,
			'style_growth', 0.02, '2026-01-15', 1, 0, 'RISK_ON')`)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	store := NewSQLiteOutcomeStore(db)
	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(loaded))
	}
	if loaded[0].MarketPeriod != "" || loaded[0].MarketPeriodSource != "" {
		t.Errorf("legacy row should read empty period fields, got %q/%q",
			loaded[0].MarketPeriod, loaded[0].MarketPeriodSource)
	}
	if loaded[0].ForwardReturn != 0.02 || !loaded[0].Hit {
		t.Errorf("legacy evaluation fields should still round-trip")
	}
}
