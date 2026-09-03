package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ============================================
// Outcome Repository Implementation
// ============================================

// nullableText returns nil for empty input so empty market_period /
// market_period_source values persist as SQL NULL (mirror of the ledger
// package helper of the same shape).
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *PostgresRepository) RecordOutcomes(ctx context.Context, outcomes []domain.RecommendationOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		// A01: session_id must be '' for the global aggregate (mirror of
		// SQLiteOutcomeStore.RecordOutcomes). Storing o.Window (a date) here
		// made LoadSessionOutcomes(session-XXX) return 0 — the
		// performance-report trades=0 root cause.
		batch.Queue(`
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata, market_period, market_period_source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, time.Now(), "", o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata,
			nullableText(o.MarketPeriod), nullableText(o.MarketPeriodSource))
	}

	br := r.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	_, err := br.Exec()
	if err != nil {
		return fmt.Errorf("insert outcomes: %w", err)
	}
	return nil
}

func (r *PostgresRepository) QueryOutcomesBySession(ctx context.Context, sessionID string) ([]domain.RecommendationOutcome, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata,
			market_period, market_period_source
		FROM recommendation_outcomes
		WHERE session_id = $1
		ORDER BY time DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query outcomes by session: %w", err)
	}
	defer rows.Close()

	return scanRecommendationOutcomes(rows)
}

func (r *PostgresRepository) QueryOutcomesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata,
			market_period, market_period_source
		FROM recommendation_outcomes
		WHERE symbol = $1 AND time >= $2 AND time <= $3
		ORDER BY time DESC
	`, symbol, start, end)
	if err != nil {
		return nil, fmt.Errorf("query outcomes by symbol: %w", err)
	}
	defer rows.Close()

	return scanRecommendationOutcomes(rows)
}

func (r *PostgresRepository) QueryOutcomesByAgent(ctx context.Context, agentID string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata,
			market_period, market_period_source
		FROM recommendation_outcomes
		WHERE agent_id = $1 AND time >= $2 AND time <= $3
		ORDER BY time DESC
	`, agentID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query outcomes by agent: %w", err)
	}
	defer rows.Close()

	return scanRecommendationOutcomes(rows)
}

func (r *PostgresRepository) QueryAllOutcomes(ctx context.Context) ([]domain.RecommendationOutcome, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata,
			market_period, market_period_source
		FROM recommendation_outcomes
		ORDER BY time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query all outcomes: %w", err)
	}
	defer rows.Close()

	return scanRecommendationOutcomes(rows)
}

func (r *PostgresRepository) QueryPassRate(ctx context.Context, agentID string, window time.Duration) (float64, error) {
	start := time.Now().Add(-window)

	var total, passed int64
	err := r.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE passed_guards = TRUE) as passed
		FROM recommendation_outcomes
		WHERE agent_id = $1 AND time >= $2
	`, agentID, start).Scan(&total, &passed)
	if err != nil {
		return 0, fmt.Errorf("query pass rate: %w", err)
	}

	if total == 0 {
		return 0, nil
	}

	return float64(passed) / float64(total), nil
}

func (r *PostgresRepository) QueryTopSymbols(ctx context.Context, limit int, start, end time.Time) ([]SymbolCount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT symbol, COUNT(*) as count
		FROM recommendation_outcomes
		WHERE time >= $1 AND time <= $2
		GROUP BY symbol
		ORDER BY count DESC
		LIMIT $3
	`, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("query top symbols: %w", err)
	}
	defer rows.Close()

	var results []SymbolCount
	for rows.Next() {
		var sc SymbolCount
		if err := rows.Scan(&sc.Symbol, &sc.Count); err != nil {
			continue
		}
		results = append(results, sc)
	}

	return results, rows.Err()
}

type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	RecordedAt   time.Time `json:"recorded_at"`
	OutcomeCount int       `json:"outcome_count"`
}

func (r *PostgresRepository) QuerySessions(ctx context.Context) ([]SessionInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT session_id, MAX(time) as recorded_at, COUNT(*) as outcome_count
		FROM recommendation_outcomes
		GROUP BY session_id
		ORDER BY recorded_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.SessionID, &s.RecordedAt, &s.OutcomeCount); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func scanRecommendationOutcomes(rows pgx.Rows) ([]domain.RecommendationOutcome, error) {
	var outcomes []domain.RecommendationOutcome
	for rows.Next() {
		var o domain.RecommendationOutcome
		var t time.Time
		var sessionID, agentLayer string
		var metadata []byte
		var marketPeriod, marketPeriodSource *string
		err := rows.Scan(
			&t, &sessionID, &o.Symbol, &o.AgentID, &agentLayer,
			&o.Conviction, &o.PassedGuards, &o.GuardReason, &o.Price,
			&metadata, &marketPeriod, &marketPeriodSource,
		)
		if err != nil {
			continue
		}
		o.Window = sessionID
		o.Layer = domain.AgentLayer(agentLayer)
		if marketPeriod != nil {
			o.MarketPeriod = *marketPeriod
		}
		if marketPeriodSource != nil {
			o.MarketPeriodSource = *marketPeriodSource
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &o)
		}
		outcomes = append(outcomes, o)
	}
	return outcomes, rows.Err()
}
