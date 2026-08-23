package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SQLiteSessionStore struct {
	db *sql.DB
}

func NewSQLiteSessionStore(db *sql.DB) *SQLiteSessionStore {
	return &SQLiteSessionStore{db: db}
}

var _ SessionStore = (*SQLiteSessionStore)(nil)

func (s *SQLiteSessionStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	// SSoT write guard (2026-08-23): strict validation on the real-time write
	// path — a corrupted summary is rejected before it can pollute the
	// performance report later.
	if err := summary.Validate(); err != nil {
		return fmt.Errorf("record session summary: rejected corrupted summary: %w", err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal session summary: %w", err)
	}

	_, _ = s.db.Exec(`ALTER TABLE session_summaries ADD COLUMN summary_json TEXT`) //nolint:errcheck

	_, err = s.db.Exec(`
		INSERT INTO session_summaries (session_id, summary_json, timestamp)
		VALUES ($1, $2, $3)
		ON CONFLICT(session_id) DO UPDATE SET summary_json = $2, timestamp = $3
	`, summary.SessionID, string(summaryJSON), summary.RecordedAt)
	if err != nil {
		return fmt.Errorf("record session summary: %w", err)
	}

	return nil
}

func (s *SQLiteSessionStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	// Ensure the summary_json column exists (idempotent; in-memory test schemas
	// may not have it until RecordSessionSummary runs its own ALTER).
	_, _ = s.db.Exec(`ALTER TABLE session_summaries ADD COLUMN summary_json TEXT`) //nolint:errcheck
	rows, err := s.db.Query(`
		SELECT summary_json FROM session_summaries ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("load session summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []domain.SessionSummary
	for rows.Next() {
		var summaryJSON string
		if err := rows.Scan(&summaryJSON); err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}

		var summary domain.SessionSummary
		if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
			return nil, fmt.Errorf("unmarshal session summary: %w", err)
		}
		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return summaries, nil
}

func (s *SQLiteSessionStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}
