// Package ledger — PostgresQuoteStore
//
// PostgreSQL mirror of SQLiteQuoteStore (quote_store_sqlite.go). Quotes
// live in the multi-process-friendly PostgreSQL database so all containers
// can share them without SQLite WAL contention (SQLITE_IOERR(522)).
//
// date is stored as TEXT (YYYY-MM-DD) matching the SQLite format; string
// comparison is equivalent for date range queries.
package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PostgresQuoteStore implements QuoteStore backed by PostgreSQL.
type PostgresQuoteStore struct {
	pool *pgxpool.Pool
}

// NewPostgresQuoteStore binds the store to an already-opened pgxpool.
func NewPostgresQuoteStore(pool *pgxpool.Pool) *PostgresQuoteStore {
	return &PostgresQuoteStore{pool: pool}
}

// Compile-time assertion: PostgresQuoteStore implements QuoteStore.
var _ QuoteStore = (*PostgresQuoteStore)(nil)

// RecordQuotes persists a batch of daily bars using ON CONFLICT upsert
// (equivalent to SQLite INSERT OR REPLACE on (symbol, date)).
func (s *PostgresQuoteStore) RecordQuotes(quotes []domain.DailyBar) error {
	if len(quotes) == 0 {
		return nil
	}

	ctx := context.Background()
	batch := &pgx.Batch{}
	for _, q := range quotes {
		batch.Queue(`
			INSERT INTO quotes (symbol, name, date, open, high, low, close, volume, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (symbol, date) DO UPDATE SET
				name = excluded.name,
				open = excluded.open,
				high = excluded.high,
				low = excluded.low,
				close = excluded.close,
				volume = excluded.volume,
				source = excluded.source
		`, q.Symbol, q.Name, q.Date.UTC().Format("2006-01-02"),
			q.Open, q.High, q.Low, q.Close, q.Volume, q.Source)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	if _, err := br.Exec(); err != nil {
		return fmt.Errorf("insert quotes: %w", err)
	}
	return nil
}

// LoadQuotes retrieves daily bars for a symbol within a time window.
func (s *PostgresQuoteStore) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	startStr := start.UTC().Format("2006-01-02")
	endStr := end.UTC().Format("2006-01-02")

	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT symbol, name, date, open, high, low, close, volume, source
		FROM quotes
		WHERE symbol = $1 AND date >= $2 AND date <= $3
		ORDER BY date ASC
	`, symbol, startStr, endStr)
	if err != nil {
		return nil, fmt.Errorf("query quotes: %w", err)
	}
	defer rows.Close()

	return scanQuoteRows(rows)
}

// LoadLatestQuotes retrieves the most recent bar for each symbol.
func (s *PostgresQuoteStore) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	result := make(map[string]domain.DailyBar)

	if len(symbols) == 0 {
		return result, nil
	}

	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (symbol) symbol, name, date, open, high, low, close, volume, source
		FROM quotes
		WHERE symbol = ANY($1)
		ORDER BY symbol, date DESC
	`, symbols)
	if err != nil {
		return nil, fmt.Errorf("query latest quotes: %w", err)
	}
	defer rows.Close()

	quotes, err := scanQuoteRows(rows)
	if err != nil {
		return nil, err
	}
	for _, q := range quotes {
		result[q.Symbol] = q
	}
	return result, nil
}

// scanQuoteRows scans quote rows into DailyBar structs (shared by
// LoadQuotes and LoadLatestQuotes).
func scanQuoteRows(rows pgx.Rows) ([]domain.DailyBar, error) {
	var quotes []domain.DailyBar
	for rows.Next() {
		var q domain.DailyBar
		var dateStr string
		var name *string
		var source *string
		if err := rows.Scan(&q.Symbol, &name, &dateStr, &q.Open, &q.High, &q.Low, &q.Close, &q.Volume, &source); err != nil {
			return nil, fmt.Errorf("scan quote row: %w", err)
		}
		if name != nil {
			q.Name = *name
		}
		if source != nil {
			q.Source = *source
		}
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse date %q: %w", dateStr, err)
		}
		q.Date = parsed
		quotes = append(quotes, q)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows iteration: %w", rows.Err())
	}
	return quotes, nil
}
