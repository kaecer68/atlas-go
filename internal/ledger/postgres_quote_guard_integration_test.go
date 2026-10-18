//go:build integration

package ledger

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestPostgresQuoteStoreWriteGuard covers the three cases the write-path guard
// must distinguish on the production backend: a normal row still writes,
// close=0 is refused (rejected row absent from the table, WARN emitted, metric
// sink called), and NaN is refused. The pre-fix behavior was verified red with
// this same probe (close=0 row present; NaN stored as 'NaN'::float8).
func TestPostgresQuoteStoreWriteGuard(t *testing.T) {
	pool := connectTestPG(t)
	cleanupQuotesTable(t, pool)
	store := NewPostgresQuoteStore(pool)
	logs := quoteGuardCapture(t)

	day := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	batch := []domain.DailyBar{
		// 1. normal row: must be written.
		{Symbol: "pgsqltest-guard-valid", Name: "valid", Date: day, Open: 900, High: 910, Low: 890, Close: 905, Volume: 1000, Source: "fugle"},
		// 2. close=0: the documented bad value (FU-20260930-03) must be refused.
		{Symbol: "pgsqltest-guard-zero", Name: "zero", Date: day, Close: 0, Volume: 0, Source: "finmind"},
		// 3. NaN close: must be refused (pgx would store 'NaN'::float8).
		{Symbol: "pgsqltest-guard-nan", Name: "nan", Date: day, Close: math.NaN(), Volume: 500, Source: "fugle"},
	}

	if err := store.RecordQuotes(batch); err != nil {
		t.Fatalf("RecordQuotes with a partially invalid batch must not fail: %v", err)
	}

	// Positive half: the valid row is really there.
	valid, err := store.LoadQuotes("pgsqltest-guard-valid", day, day.Add(time.Hour*23))
	if err != nil {
		t.Fatalf("LoadQuotes(valid): %v", err)
	}
	if len(valid) != 1 || valid[0].Close != 905 {
		t.Fatalf("valid row missing or wrong: %+v", valid)
	}

	// Negative half: neither refused row reached the table (asserted in SQL so
	// a LoadQuotes-side filter cannot produce a false green).
	ctx := context.Background()
	var stored int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM quotes WHERE symbol LIKE 'pgsqltest-guard-%' AND symbol <> 'pgsqltest-guard-valid'",
	).Scan(&stored); err != nil {
		t.Fatalf("count refused rows: %v", err)
	}
	if stored != 0 {
		t.Fatalf("refused rows reached the table: %d", stored)
	}

	logged := logs.String()
	for _, want := range []string{
		"quote_write_rejected",
		"backend=postgres",
		"reason=non_positive",
		"symbol=pgsqltest-guard-zero",
		"reason=non_finite",
		"symbol=pgsqltest-guard-nan",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("WARN log missing %q: %q", want, logged)
		}
	}
}

// TestPostgresQuoteStoreAllRowsRejected pins the all-rejected contract on the
// production backend: the caller learns nothing was written instead of assuming
// a symbol was persisted.
func TestPostgresQuoteStoreAllRowsRejected(t *testing.T) {
	pool := connectTestPG(t)
	cleanupQuotesTable(t, pool)
	store := NewPostgresQuoteStore(pool)
	quoteGuardCapture(t)

	day := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "pgsqltest-guard-all", Date: day, Close: 0, Source: "finmind"},
	})
	if !errors.Is(err, ErrAllQuotesRejected) {
		t.Fatalf("expected ErrAllQuotesRejected, got %v", err)
	}

	var stored int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM quotes WHERE symbol = 'pgsqltest-guard-all'").Scan(&stored); err != nil {
		t.Fatalf("count: %v", err)
	}
	if stored != 0 {
		t.Fatalf("expected no stored row, got %d", stored)
	}
}
