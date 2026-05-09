package ledger

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SQLiteQuoteStore implements QuoteStore backed by SQLite.
type SQLiteQuoteStore struct {
	db *sql.DB
}

// NewSQLiteQuoteStore creates a new SQLite-backed QuoteStore.
func NewSQLiteQuoteStore(db *sql.DB) *SQLiteQuoteStore {
	return &SQLiteQuoteStore{db: db}
}

// Compile-time assertion: SQLiteQuoteStore implements QuoteStore.
var _ QuoteStore = (*SQLiteQuoteStore)(nil)

// RecordQuotes persists a batch of daily bars using INSERT OR REPLACE.
func (s *SQLiteQuoteStore) RecordQuotes(quotes []domain.DailyBar) error {
	if len(quotes) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO quotes (symbol, name, date, open, high, low, close, volume, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, q := range quotes {
		dateStr := q.Date.Format("2006-01-02")
		_, err = stmt.Exec(q.Symbol, q.Name, dateStr, q.Open, q.High, q.Low, q.Close, q.Volume, q.Source)
		if err != nil {
			return fmt.Errorf("insert quote: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// LoadQuotes retrieves daily bars for a symbol within a time window.
func (s *SQLiteQuoteStore) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")

	rows, err := s.db.Query(`
		SELECT symbol, name, date, open, high, low, close, volume, source
		FROM quotes
		WHERE symbol = ? AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, symbol, startStr, endStr)
	if err != nil {
		return nil, fmt.Errorf("query quotes: %w", err)
	}
	defer rows.Close()

	var quotes []domain.DailyBar
	for rows.Next() {
		var q domain.DailyBar
		var dateStr string
		err := rows.Scan(&q.Symbol, &q.Name, &dateStr, &q.Open, &q.High, &q.Low, &q.Close, &q.Volume, &q.Source)
		if err != nil {
			return nil, fmt.Errorf("scan quote row: %w", err)
		}
		q.Date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse date: %w", err)
		}
		quotes = append(quotes, q)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return quotes, nil
}

// LoadLatestQuotes retrieves the most recent bar for each symbol.
func (s *SQLiteQuoteStore) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	result := make(map[string]domain.DailyBar)

	if len(symbols) == 0 {
		return result, nil
	}

	// Use a subquery to get the latest date per symbol.
	// #nosec G202 -- placeholders() generates only '?' characters, no user input.
	query := `
		SELECT q.symbol, q.name, q.date, q.open, q.high, q.low, q.close, q.volume, q.source
		FROM quotes q
		INNER JOIN (
			SELECT symbol, MAX(date) as max_date
			FROM quotes
			WHERE symbol IN (` + placeholders(len(symbols)) + `)
			GROUP BY symbol
		) latest ON q.symbol = latest.symbol AND q.date = latest.max_date
	`

	args := make([]interface{}, len(symbols))
	for i, sym := range symbols {
		args[i] = sym
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query latest quotes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var q domain.DailyBar
		var dateStr string
		err := rows.Scan(&q.Symbol, &q.Name, &dateStr, &q.Open, &q.High, &q.Low, &q.Close, &q.Volume, &q.Source)
		if err != nil {
			return nil, fmt.Errorf("scan latest quote row: %w", err)
		}
		q.Date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse date: %w", err)
		}
		result[q.Symbol] = q
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return result, nil
}

// placeholders generates a comma-separated list of '?' placeholders.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	result := "?"
	for i := 1; i < n; i++ {
		result += ",?"
	}
	return result
}
