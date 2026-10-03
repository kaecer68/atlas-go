package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MaxQuoteDate implements QuoteMaxDater for the SQLite backend.
// date 是 TEXT（YYYY-MM-DD，見 sqlite_core.go quotes DDL），
// string 的 MAX 與日期序一致。

func (s *SQLiteQuoteStore) MaxQuoteDate(ctx context.Context) (time.Time, error) {
	var maxDate sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT MAX(date) FROM quotes`).Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("max quote date: %w", err)
	}
	if !maxDate.Valid {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02", maxDate.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse max quote date %q: %w", maxDate.String, err)
	}
	return t, nil
}
