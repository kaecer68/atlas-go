//go:build integration

package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// cleanupQuotesTable removes test-prefixed quote rows so tests stay isolated
// from migrated production data.
func cleanupQuotesTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'pgsqltest-%'")
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'pgsqltest-%'")
	})
}

func TestPostgresQuoteStore_RoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupQuotesTable(t, pool)
	store := NewPostgresQuoteStore(pool)

	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	quotes := []domain.DailyBar{
		{Symbol: "pgsqltest-2330", Name: "台積電", Date: base.AddDate(0, 0, 0), Open: 900, High: 910, Low: 890, Close: 905, Volume: 1000, Source: "fugle"},
		{Symbol: "pgsqltest-2330", Name: "台積電", Date: base.AddDate(0, 0, 1), Open: 910, High: 920, Low: 905, Close: 915, Volume: 1100, Source: "fugle"},
	}
	if err := store.RecordQuotes(quotes); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	// Window read: only the first day falls in [base, base].
	got, err := store.LoadQuotes("pgsqltest-2330", base, base.Add(time.Hour*23))
	if err != nil {
		t.Fatalf("LoadQuotes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 quote in window, got %d", len(got))
	}
	if got[0].Close != 905 || got[0].Volume != 1000 {
		t.Fatalf("quote mismatch: %+v", got[0])
	}
	if got[0].Date.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("date mismatch: %v", got[0].Date)
	}

	// Latest quote returns the most recent bar.
	latest, err := store.LoadLatestQuotes([]string{"pgsqltest-2330"})
	if err != nil {
		t.Fatalf("LoadLatestQuotes: %v", err)
	}
	if len(latest) != 1 {
		t.Fatalf("expected 1 latest quote, got %d", len(latest))
	}
	if latest["pgsqltest-2330"].Close != 915 {
		t.Fatalf("latest close mismatch: %+v", latest["pgsqltest-2330"])
	}
}

func TestPostgresQuoteStore_OverwriteSameKey(t *testing.T) {
	pool := connectTestPG(t)
	cleanupQuotesTable(t, pool)
	store := NewPostgresQuoteStore(pool)

	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "pgsqltest-2454", Date: day, Close: 800, Volume: 500},
	}); err != nil {
		t.Fatalf("RecordQuotes v1: %v", err)
	}
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "pgsqltest-2454", Date: day, Close: 820, Volume: 600},
	}); err != nil {
		t.Fatalf("RecordQuotes v2: %v", err)
	}

	got, err := store.LoadQuotes("pgsqltest-2454", day, day.Add(time.Hour*23))
	if err != nil {
		t.Fatalf("LoadQuotes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row after overwrite, got %d", len(got))
	}
	if got[0].Close != 820 || got[0].Volume != 600 {
		t.Fatalf("ON CONFLICT update failed: %+v", got[0])
	}
}

func TestPostgresQuoteStore_EmptyInputs(t *testing.T) {
	pool := connectTestPG(t)
	store := NewPostgresQuoteStore(pool)

	if err := store.RecordQuotes(nil); err != nil {
		t.Fatalf("RecordQuotes(nil) should no-op: %v", err)
	}
	if err := store.RecordQuotes([]domain.DailyBar{}); err != nil {
		t.Fatalf("RecordQuotes(empty) should no-op: %v", err)
	}

	latest, err := store.LoadLatestQuotes(nil)
	if err != nil {
		t.Fatalf("LoadLatestQuotes(nil) should no-op: %v", err)
	}
	if len(latest) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(latest))
	}
}
