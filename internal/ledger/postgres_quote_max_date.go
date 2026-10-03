package ledger

import (
	"context"
	"fmt"
	"time"
)

// MaxQuoteDate implements QuoteMaxDater for the PostgreSQL backend.
// date 是 TEXT（YYYY-MM-DD，見 sql/migrations/000012_quotes.up.sql），
// string 的 MAX 與日期序一致。

func (s *PostgresQuoteStore) MaxQuoteDate(ctx context.Context) (time.Time, error) {
	var maxDate *string
	err := s.pool.QueryRow(ctx, `SELECT MAX(date) FROM quotes`).Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("max quote date: %w", err)
	}
	if maxDate == nil {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02", *maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse max quote date %q: %w", *maxDate, err)
	}
	return t, nil
}
