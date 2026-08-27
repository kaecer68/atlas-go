package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

// connectTestPG connects to PostgreSQL (DATABASE_URL only, no hardcoded DSN),
// runs migrations, and returns the pool. See testdb.Pool for the CI/local
// skip policy.
func connectTestPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t, "../../sql/migrations")
}

// cleanupHistoricalTables removes all historical rows so tests are isolated.
func cleanupHistoricalTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{"prediction_backtest", "event_calendar_history", "geopolitical_history", "period_history", "stress_index_history", "regime_history"} {
		_, _ = pool.Exec(ctx, "DELETE FROM "+tbl)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, tbl := range []string{"prediction_backtest", "event_calendar_history", "geopolitical_history", "period_history", "stress_index_history", "regime_history"} {
			_, _ = pool.Exec(ctx, "DELETE FROM "+tbl)
		}
	})
}

func TestPostgresHistoricalStore_RegimeRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupHistoricalTables(t, pool)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	row := RegimeRow{
		Date:        "2026-08-13",
		Regime:      "RISK_ON",
		Source:      "macro_ingest",
		RecordedAt:  time.Now().UTC(),
		CapturedAt:  time.Now().UTC(),
		IsSynthetic: 0,
	}
	if err := store.UpsertRegime(ctx, row); err != nil {
		t.Fatalf("UpsertRegime: %v", err)
	}

	got, ok, err := store.LoadRegimeByDate(ctx, "2026-08-13")
	if err != nil || !ok {
		t.Fatalf("LoadRegimeByDate: ok=%v err=%v", ok, err)
	}
	if got.Regime != "RISK_ON" || got.Source != "macro_ingest" {
		t.Fatalf("regime mismatch: %+v", got)
	}

	hist, err := store.LoadRegimeHistory(ctx, 10)
	if err != nil || len(hist) != 1 {
		t.Fatalf("LoadRegimeHistory: len=%d err=%v", len(hist), err)
	}
}

func TestPostgresHistoricalStore_StressJSONRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupHistoricalTables(t, pool)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	row := StressRow{
		Date:       "2026-08-13",
		Score:      0.75,
		Regime:     "high",
		Components: map[string]interface{}{"dxy": 0.5, "vix": 30.0},
		Source:     "macro_ingest",
		CapturedAt: time.Now().UTC(),
	}
	if err := store.UpsertStress(ctx, row); err != nil {
		t.Fatalf("UpsertStress: %v", err)
	}

	got, ok, err := store.LoadStressByDate(ctx, "2026-08-13")
	if err != nil || !ok {
		t.Fatalf("LoadStressByDate: ok=%v err=%v", ok, err)
	}
	if got.Score != 0.75 || got.Components["dxy"] != 0.5 {
		t.Fatalf("stress mismatch: %+v", got)
	}
}

func TestPostgresHistoricalStore_GeopoliticalSourcesRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupHistoricalTables(t, pool)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	row := GeopoliticalRow{
		Date:       "2026-08-13",
		Intensity:  5.5,
		Sources:    []string{"GPR", "WUI"},
		Source:     "geo",
		CapturedAt: time.Now().UTC(),
	}
	if err := store.UpsertGeopolitical(ctx, row); err != nil {
		t.Fatalf("UpsertGeopolitical: %v", err)
	}

	got, ok, err := store.LoadGeopoliticalByDate(ctx, "2026-08-13")
	if err != nil || !ok {
		t.Fatalf("LoadGeopoliticalByDate: ok=%v err=%v", ok, err)
	}
	if len(got.Sources) != 2 || got.Sources[0] != "GPR" {
		t.Fatalf("geo sources mismatch: %+v", got.Sources)
	}
}

func TestPostgresHistoricalStore_PeriodRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupHistoricalTables(t, pool)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	row := PeriodRow{Date: "2026-08-13", Period: "bull", Source: "macro_ingest", CapturedAt: time.Now().UTC()}
	if err := store.UpsertPeriod(ctx, row); err != nil {
		t.Fatalf("UpsertPeriod: %v", err)
	}
	got, ok, err := store.LoadPeriodByDate(ctx, "2026-08-13")
	if err != nil || !ok || got.Period != "bull" {
		t.Fatalf("LoadPeriodByDate: ok=%v period=%q err=%v", ok, got.Period, err)
	}
}

func TestPostgresHistoricalStore_HasTables(t *testing.T) {
	pool := connectTestPG(t)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	tables, err := store.HasTables(ctx)
	if err != nil {
		t.Fatalf("HasTables: %v", err)
	}
	for _, want := range []string{"regime_history", "stress_index_history", "period_history", "geopolitical_history", "event_calendar_history", "prediction_backtest"} {
		if !tables[want] {
			t.Errorf("expected table %s to exist (migration applied)", want)
		}
	}
}

func TestPostgresHistoricalStore_CountSynthetic(t *testing.T) {
	pool := connectTestPG(t)
	cleanupHistoricalTables(t, pool)
	store := NewPostgresHistoricalStore(pool)
	ctx := context.Background()

	if err := store.UpsertRegime(ctx, RegimeRow{Date: "2026-08-13", Regime: "RISK_ON", CapturedAt: time.Now().UTC(), IsSynthetic: 1}); err != nil {
		t.Fatalf("UpsertRegime: %v", err)
	}
	counts, err := store.CountSynthetic(ctx)
	if err != nil {
		t.Fatalf("CountSynthetic: %v", err)
	}
	if counts["regime_history"] != 1 {
		t.Fatalf("expected 1 synthetic regime, got %d", counts["regime_history"])
	}
}
