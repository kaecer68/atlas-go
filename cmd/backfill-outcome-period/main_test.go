package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func ptr(s string) *string { return &s }

// openTempDB opens a fresh SQLite file DB (file-backed so UPDATE persists).
func openTempDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backfill-test.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func insertOutcomeRow(t *testing.T, db *sql.DB, ts string, mp any, mps any) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, conviction, regime, timestamp, passed_guards,
			layer, forward_return, window, hit, is_synthetic, true_regime, market_period, market_period_source)
		VALUES ('', '2330', 'agent-a', 'BUY', 80, 'RISK_ON', ?, 1, 'sector', 0.02, ?, 1, 0, 'RISK_ON', ?, ?)`,
		ts, ts[:10], mp, mps); err != nil {
		t.Fatalf("insert outcome %s: %v", ts, err)
	}
}

func seedOutcomePeriodFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	hist := ledger.NewSQLiteHistoricalStore(db)
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	for _, row := range []ledger.PeriodRow{
		{Date: "2026-04-01", Period: "bull", IsSynthetic: 0, Source: "macro_ingest", CapturedAt: now},
		{Date: "2020-06-15", Period: "black_swan", IsSynthetic: 1, Source: "period_history_range_backfill_ohlcv", CapturedAt: now},
	} {
		if err := hist.UpsertPeriod(ctx, row); err != nil {
			t.Fatalf("upsert period %s: %v", row.Date, err)
		}
	}
	insertOutcomeRow(t, db, "2026-04-01T00:00:00Z", nil, nil)          // matches live period
	insertOutcomeRow(t, db, "2020-06-15T00:00:00Z", nil, nil)          // matches synthetic period
	insertOutcomeRow(t, db, "2019-01-02T00:00:00Z", nil, nil)          // no period row → unmatched
	insertOutcomeRow(t, db, "2026-04-01T10:00:00Z", "plateau", "live") // already set → skip
}

func assertPeriod(t *testing.T, db *sql.DB, ts, wantPeriod, wantSource string) {
	t.Helper()
	var p, s sql.NullString
	if err := db.QueryRow(`SELECT market_period, market_period_source FROM outcomes WHERE timestamp = ?`, ts).Scan(&p, &s); err != nil {
		t.Fatalf("query %s: %v", ts, err)
	}
	if p.String != wantPeriod || s.String != wantSource {
		t.Errorf("row %s: got period=%q source=%q, want %q/%q", ts, p.String, s.String, wantPeriod, wantSource)
	}
}

func TestBackfillSQLiteDB(t *testing.T) {
	ctx := context.Background()
	db, _ := openTempDB(t)
	seedOutcomePeriodFixture(t, db)

	// Dry-run reports candidates without writing.
	res, err := backfillSQLiteDB(ctx, db, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Total != 3 || res.Matched != 2 || res.Unmatched != 1 {
		t.Fatalf("dry run result = %+v, want total=3 matched=2 unmatched=1", res)
	}
	assertPeriod(t, db, "2026-04-01T00:00:00Z", "", "")

	// Real run fills matching rows, leaves unmatched untouched, skips set rows.
	res, err = backfillSQLiteDB(ctx, db, false)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Total != 3 || res.Matched != 2 {
		t.Fatalf("run result = %+v, want total=3 matched=2", res)
	}
	assertPeriod(t, db, "2026-04-01T00:00:00Z", "bull", "live")
	assertPeriod(t, db, "2020-06-15T00:00:00Z", "black_swan", "synthetic")
	assertPeriod(t, db, "2019-01-02T00:00:00Z", "", "")
	assertPeriod(t, db, "2026-04-01T10:00:00Z", "plateau", "live")

	// Second run only sees the permanently-unmatched row (idempotent: the
	// two matched rows are now set, nothing new to fill).
	res, err = backfillSQLiteDB(ctx, db, false)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Total != 1 || res.Matched != 0 || res.Unmatched != 1 {
		t.Fatalf("second run result = %+v, want total=1 matched=0 unmatched=1", res)
	}
}

func TestTradingDateOf(t *testing.T) {
	o := domain.RecommendationOutcome{Window: "2026-04-01", RecordedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)}
	if got := tradingDateOf(o); got != "2026-04-01" {
		t.Errorf("date-shaped Window should win, got %q", got)
	}
	o = domain.RecommendationOutcome{Window: "2026-04", RecordedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)}
	if got := tradingDateOf(o); got != "2026-04-02" {
		t.Errorf("RecordedAt fallback = %q, want 2026-04-02", got)
	}
	o = domain.RecommendationOutcome{}
	if got := tradingDateOf(o); got != "" {
		t.Errorf("empty outcome trading date = %q, want empty", got)
	}
}

func jsonLine(o domain.RecommendationOutcome) string {
	b, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func readOutcomeFromJSONL(t *testing.T, path, agentID string) domain.RecommendationOutcome {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var o domain.RecommendationOutcome
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if o.AgentID == agentID {
			return o
		}
	}
	t.Fatalf("agent %s not found in %s", agentID, path)
	return domain.RecommendationOutcome{}
}

func TestBackfillJSONLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommendation_outcomes.jsonl")
	rows := []domain.RecommendationOutcome{
		{AgentID: "a", Symbol: "2330", Window: "2026-04-01", RecordedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{AgentID: "b", Symbol: "2317", Window: "2019-01-02", RecordedAt: time.Date(2019, 1, 2, 0, 0, 0, 0, time.UTC)},
		{AgentID: "c", Symbol: "0050", Window: "2026-04-01", RecordedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), MarketPeriod: "plateau", MarketPeriodSource: "live"},
	}
	raw := ""
	for _, o := range rows {
		raw += jsonLine(o) + "\n"
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	periodFor := func(date string) (string, string, bool) {
		if date == "2026-04-01" {
			return "bull", "live", true
		}
		return "", "", false
	}

	// Dry run: counts only, file unchanged.
	examined, filled, err := backfillJSONLFile(path, periodFor, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if examined != 3 || filled != 1 {
		t.Fatalf("dry run examined=%d filled=%d, want 3/1", examined, filled)
	}
	if got := readOutcomeFromJSONL(t, path, "a"); got.MarketPeriod != "" {
		t.Errorf("dry run must not write; agent a period = %q", got.MarketPeriod)
	}

	// Real run rewrites the file.
	examined, filled, err = backfillJSONLFile(path, periodFor, false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if examined != 3 || filled != 1 {
		t.Fatalf("run examined=%d filled=%d, want 3/1", examined, filled)
	}
	a := readOutcomeFromJSONL(t, path, "a")
	if a.MarketPeriod != "bull" || a.MarketPeriodSource != "live" {
		t.Errorf("agent a: got %q/%q, want bull/live", a.MarketPeriod, a.MarketPeriodSource)
	}
	if b := readOutcomeFromJSONL(t, path, "b"); b.MarketPeriod != "" || b.MarketPeriodSource != "" {
		t.Errorf("agent b unmatched day must stay empty, got %q/%q", b.MarketPeriod, b.MarketPeriodSource)
	}
	if c := readOutcomeFromJSONL(t, path, "c"); c.MarketPeriod != "plateau" || c.MarketPeriodSource != "live" {
		t.Errorf("agent c pre-set row must be untouched, got %q/%q", c.MarketPeriod, c.MarketPeriodSource)
	}

	// Second run: nothing to fill.
	if examined, filled, err = backfillJSONLFile(path, periodFor, false); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if filled != 0 {
		t.Errorf("second run filled=%d, want 0 (idempotent)", filled)
	}
}
