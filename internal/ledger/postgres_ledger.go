// Package ledger — PostgresLedgerStore
//
// PostgreSQL mirror of the SQLite/JSONL ledger stores (OutcomeStore,
// SessionStore; experiment/backtest/spawn surface added in a follow-up
// PR). All data lives in the multi-process-friendly PostgreSQL database so
// all containers share one consistent ledger.
//
// SQL mapping mirrors internal/repository/postgres_outcomes.go and
// postgres_audit.go (the authoritative repository-side mapping); ledger
// cannot import repository (repository imports ledger — cycle), so the SQL
// is duplicated here by design.
//
// The OutcomeStore/SessionStore interfaces carry no ctx — internal methods
// use context.Background().
package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PostgresLedgerStore implements OutcomeStore + SessionStore on PostgreSQL.
type PostgresLedgerStore struct {
	pool *pgxpool.Pool
}

// NewPostgresLedgerStore binds the store to an already-opened pgxpool.
func NewPostgresLedgerStore(pool *pgxpool.Pool) *PostgresLedgerStore {
	return &PostgresLedgerStore{pool: pool}
}

// Compile-time assertions.
var (
	_ OutcomeStore          = (*PostgresLedgerStore)(nil)
	_ SessionStore          = (*PostgresLedgerStore)(nil)
	_ ExperimentStore       = (*PostgresLedgerStore)(nil)
	_ BacktestStore         = (*PostgresLedgerStore)(nil)
	_ FullStore             = (*PostgresLedgerStore)(nil)
	_ ScorecardOutcomeStore = (*PostgresLedgerStore)(nil)
)

// ------------------------------------------------------------------
// Outcomes
// ------------------------------------------------------------------

// RecordOutcomes writes a batch of global outcomes. session_id is ” for the
// global aggregate (mirror of SQLiteOutcomeStore.RecordOutcomes); metadata
// stores the full object JSON (window survives in metadata), so reading back
// unmarshals metadata over the scanned columns. A01: previously session_id
// stored o.Window (a date) which broke LoadSessionOutcomes(session-XXX) — the
// performance-report trades=0 root cause.
func (s *PostgresLedgerStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}
	ctx := context.Background()

	batch := &pgx.Batch{}
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		ts := o.RecordedAt
		if ts.IsZero() {
			ts = time.Now()
		}
		batch.Queue(`
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, ts, "", o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return fmt.Errorf("insert outcomes: %w", err)
	}
	return nil
}

// RecordSessionOutcomes writes outcomes for a session. session_id stores the
// session ID (session-YYYYMMDD-daily) so LoadSessionOutcomes(sessionID) finds
// them — mirror of SQLiteOutcomeStore.RecordSessionOutcomes. A zero
// RecordedAt falls back to the session date.
func (s *PostgresLedgerStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}
	ctx := context.Background()

	batch := &pgx.Batch{}
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		ts := o.RecordedAt
		if ts.IsZero() {
			ts = session.SessionDate
		}
		if ts.IsZero() {
			ts = time.Now()
		}
		batch.Queue(`
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, ts, session.ID, o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return fmt.Errorf("insert session outcomes: %w", err)
	}
	return nil
}

// LoadOutcomes reads all outcomes, newest first.
func (s *PostgresLedgerStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return s.loadOutcomes("", "")
}

// LoadSessionOutcomes reads outcomes for one session.
func (s *PostgresLedgerStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return s.loadOutcomes("WHERE session_id = $1", sessionID)
}

// LoadOutcomesFromSessions reads all outcomes — PostgreSQL is the complete
// source, no JSONL delegation.
func (s *PostgresLedgerStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return s.loadOutcomes("", "")
}

// loadOutcomes runs the outcome SELECT with an optional WHERE clause.
func (s *PostgresLedgerStore) loadOutcomes(where string, arg string) ([]domain.RecommendationOutcome, error) {
	ctx := context.Background()
	query := `
		SELECT time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata
		FROM recommendation_outcomes `
	if where != "" {
		query += where + " "
	}
	query += "ORDER BY time DESC"

	var rows pgx.Rows
	var err error
	if where != "" {
		rows, err = s.pool.Query(ctx, query, arg)
	} else {
		rows, err = s.pool.Query(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("query outcomes: %w", err)
	}
	defer rows.Close()

	return scanPGOutcomes(rows)
}

// scanPGOutcomes rebuilds RecommendationOutcome rows, unmarshalling the
// metadata JSON over the scanned columns (mirror of
// repository.scanRecommendationOutcomes).
func scanPGOutcomes(rows pgx.Rows) ([]domain.RecommendationOutcome, error) {
	var outcomes []domain.RecommendationOutcome
	for rows.Next() {
		var o domain.RecommendationOutcome
		var t time.Time
		var sessionID, agentLayer string
		var metadata []byte
		err := rows.Scan(
			&t, &sessionID, &o.Symbol, &o.AgentID, &agentLayer,
			&o.Conviction, &o.PassedGuards, &o.GuardReason, &o.Price,
			&metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("scan outcome row: %w", err)
		}
		o.RecordedAt = t
		o.Window = sessionID
		o.Layer = domain.AgentLayer(agentLayer)
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &o); err != nil {
				return nil, fmt.Errorf("unmarshal outcome metadata: %w", err)
			}
		}
		outcomes = append(outcomes, o)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("outcome rows: %w", rows.Err())
	}
	return outcomes, nil
}

// ScorecardOutcomeStore is the optional narrow interface for stores that can
// serve the observatory scorecard slim projection: only the 8 scalar fields
// consumed by BuildScorecards/computeAgentRegimeBreakdown
// (AgentID/Skill/Layer/Window/ForwardReturn/Hit/RecordedAt/Regime), with the
// heavy metadata fields (factor_scores, conviction_breakdown,
// supporting_events, parameter_snapshot) never unmarshalled (#1780 Phase 1).
//
// It is deliberately NOT part of OutcomeStore: jsonl/sqlite backends and
// existing mocks keep compiling and automatically fall back to the full
// LoadOutcomesFromSessions read (each fallback layer warns + counts, see B1
// in the #1780 Phase 1 review).
type ScorecardOutcomeStore interface {
	LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error)
}

// LoadScorecardOutcomes reads only the 8 scalar fields the observatory
// scorecard pipeline consumes, via a slim JSONB projection instead of the
// full-table + full-metadata load (LoadOutcomesFromSessions → scanPGOutcomes,
// which unmarshals every row's complete metadata JSON ~1.69GB live heap at
// 63k rows — the #1780 OOM root cause). Semantic equivalence with the full
// read, per the #1780 Phase 1 review:
//
//   - field values come from metadata (the JSONB source of truth), exactly
//     like scanPGOutcomes' column-scan-then-unmarshal final result;
//   - COALESCE defaults reproduce encoding/json zero-value behavior for
//     absent/null keys, and guard the metadata DEFAULT '{}' rows from NULL
//     scans into float64/bool (B2);
//   - recorded_at is returned as TEXT (RFC3339Nano preserved) and parsed in
//     Go, so sub-microsecond ordering is identical to the full read instead
//     of being rounded away by a timestamptz cast (B3).
func (s *PostgresLedgerStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	ctx := context.Background()
	const query = `
		SELECT agent_id,
		       COALESCE(metadata->>'skill', ''),
		       COALESCE(metadata->>'layer', ''),
		       COALESCE(metadata->>'window', ''),
		       COALESCE((metadata->>'forward_return')::float8, 0),
		       COALESCE((metadata->>'hit')::boolean, false),
		       COALESCE(metadata->>'regime', ''),
		       COALESCE(metadata->>'recorded_at', ''),
		       time
		FROM recommendation_outcomes
		ORDER BY time DESC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query scorecard outcomes: %w", err)
	}
	defer rows.Close()

	var outcomes []domain.RecommendationOutcome
	for rows.Next() {
		var o domain.RecommendationOutcome
		var layer, recordedAtText string
		var colTime time.Time
		if err := rows.Scan(
			&o.AgentID, &o.Skill, &layer, &o.Window,
			&o.ForwardReturn, &o.Hit, &o.Regime, &recordedAtText, &colTime,
		); err != nil {
			return nil, fmt.Errorf("scan scorecard outcome row: %w", err)
		}
		o.Layer = domain.AgentLayer(layer)
		if recordedAtText == "" {
			// Metadata has no recorded_at key — mirror the full read, which
			// keeps the scanned time column value in that case.
			o.RecordedAt = colTime
		} else {
			parsed, err := time.Parse(time.RFC3339Nano, recordedAtText)
			if err != nil {
				return nil, fmt.Errorf("parse scorecard outcome recorded_at %q: %w", recordedAtText, err)
			}
			o.RecordedAt = parsed
		}
		outcomes = append(outcomes, o)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("scorecard outcome rows: %w", rows.Err())
	}
	return outcomes, nil
}

// LoadAllSessionScorecards aggregates all outcomes into scorecards
// (PostgreSQL is the complete source).
func (s *PostgresLedgerStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	outcomes, err := s.LoadOutcomesFromSessions()
	if err != nil {
		return nil, nil, err
	}
	return BuildScorecards(outcomes), outcomes, nil
}

// ------------------------------------------------------------------
// Screening rejects
// ------------------------------------------------------------------

// RecordSessionScreeningRejects persists screening rejects for a session.
func (s *PostgresLedgerStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	if len(rejects) == 0 {
		return nil
	}
	ctx := context.Background()

	batch := &pgx.Batch{}
	for _, sr := range rejects {
		factorScores, _ := json.Marshal(sr.FactorScores)
		batch.Queue(`
			INSERT INTO screening_rejects (time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, sr.RecordedAt, sr.SessionID, sr.Symbol, sr.AgentID, sr.Skill,
			sr.Criterion, sr.CriterionLabel, sr.Threshold, sr.ActualValue, factorScores)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return fmt.Errorf("insert screening rejects: %w", err)
	}
	return nil
}

// LoadSessionScreeningRejects reads screening rejects for a session,
// ordered by time then id for stable ordering.
func (s *PostgresLedgerStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores
		FROM screening_rejects
		WHERE session_id = $1
		ORDER BY time, id
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query screening rejects by session: %w", err)
	}
	defer rows.Close()

	var rejects []domain.ScreeningReject
	for rows.Next() {
		var sr domain.ScreeningReject
		var factorScores []byte
		if err := rows.Scan(
			&sr.RecordedAt, &sr.SessionID, &sr.Symbol, &sr.AgentID, &sr.Skill,
			&sr.Criterion, &sr.CriterionLabel, &sr.Threshold, &sr.ActualValue, &factorScores,
		); err != nil {
			return nil, fmt.Errorf("scan screening reject: %w", err)
		}
		if len(factorScores) > 0 {
			if err := json.Unmarshal(factorScores, &sr.FactorScores); err != nil {
				return nil, fmt.Errorf("unmarshal factor_scores: %w", err)
			}
		}
		rejects = append(rejects, sr)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("screening reject rows: %w", rows.Err())
	}
	return rejects, nil
}

// ------------------------------------------------------------------
// Session summaries
// ------------------------------------------------------------------

// RecordSessionSummary upserts a session summary (mirror of
// repository.postgres_audit.go SaveSessionSummary).
func (s *PostgresLedgerStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	// SSoT write guard (2026-08-23): strict validation on the real-time write
	// path — a corrupted summary is rejected before it can pollute the
	// performance report later.
	if err := summary.Validate(); err != nil {
		return fmt.Errorf("record session summary: rejected corrupted summary: %w", err)
	}
	ctx := context.Background()

	brokerRuntime, _ := json.Marshal(summary.BrokerRuntime)
	guardOutcomes, _ := json.Marshal(summary.GuardOutcomes)
	taxSnapshots, _ := json.Marshal(summary.TaxSnapshots)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO session_summaries (time, session_id, regime, order_count, position_count, ending_cash, portfolio_value, outcome_count, broker_runtime, next_experiment_agent_id, proposal_id, commit_id, approval_id, guard_outcomes, risk_commentary, tax_snapshots, after_tax_pnl, total_tax_paid, parameters_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (session_id) DO UPDATE SET
			time = EXCLUDED.time,
			regime = EXCLUDED.regime,
			order_count = EXCLUDED.order_count,
			position_count = EXCLUDED.position_count,
			ending_cash = EXCLUDED.ending_cash,
			portfolio_value = EXCLUDED.portfolio_value,
			outcome_count = EXCLUDED.outcome_count,
			broker_runtime = EXCLUDED.broker_runtime,
			next_experiment_agent_id = EXCLUDED.next_experiment_agent_id,
			proposal_id = EXCLUDED.proposal_id,
			commit_id = EXCLUDED.commit_id,
			approval_id = EXCLUDED.approval_id,
			guard_outcomes = EXCLUDED.guard_outcomes,
			risk_commentary = EXCLUDED.risk_commentary,
			tax_snapshots = EXCLUDED.tax_snapshots,
			after_tax_pnl = EXCLUDED.after_tax_pnl,
			total_tax_paid = EXCLUDED.total_tax_paid,
			parameters_version = EXCLUDED.parameters_version
	`, summary.RecordedAt, summary.SessionID, string(summary.Regime), summary.OrderCount,
		summary.PositionCount, summary.EndingCash, summary.PortfolioValue, summary.OutcomeCount,
		brokerRuntime, summary.NextExperimentAgentID, summary.ProposalID, summary.CommitID,
		summary.ApprovalID, guardOutcomes, summary.RiskCommentary, taxSnapshots, summary.AfterTaxPnL, summary.TotalTaxPaid,
		summary.ParametersVersion)
	if err != nil {
		return fmt.Errorf("save session summary: %w", err)
	}
	return nil
}

// LoadSessionSummaries reads all session summaries, newest first.
func (s *PostgresLedgerStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT time, session_id, regime, order_count, position_count, ending_cash, portfolio_value, outcome_count, broker_runtime, next_experiment_agent_id, proposal_id, commit_id, approval_id, guard_outcomes, risk_commentary, tax_snapshots, after_tax_pnl, total_tax_paid, parameters_version
		FROM session_summaries
		ORDER BY time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("load all session summaries: %w", err)
	}
	defer rows.Close()

	var summaries []domain.SessionSummary
	for rows.Next() {
		summary, err := scanPGSessionSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, *summary)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("session summary rows: %w", rows.Err())
	}
	return summaries, nil
}

// scanPGSessionSummary scans one session_summaries row (mirror of
// repository.postgres_audit.go LoadSessionSummary scan). Columns added by
// later migrations (risk_commentary, tax_snapshots, after_tax_pnl,
// total_tax_paid, parameters_version) may be NULL on rows written before
// those columns existed — scan with pointer types and normalize.
func scanPGSessionSummary(rows pgx.Rows) (*domain.SessionSummary, error) {
	var summary domain.SessionSummary
	var regime string
	var brokerRuntime, guardOutcomes, taxSnapshots []byte
	var riskCommentary, parametersVersion *string
	var afterTaxPnL, totalTaxPaid *float64

	err := rows.Scan(
		&summary.RecordedAt, &summary.SessionID, &regime, &summary.OrderCount,
		&summary.PositionCount, &summary.EndingCash, &summary.PortfolioValue, &summary.OutcomeCount,
		&brokerRuntime, &summary.NextExperimentAgentID, &summary.ProposalID, &summary.CommitID,
		&summary.ApprovalID, &guardOutcomes, &riskCommentary, &taxSnapshots, &afterTaxPnL, &totalTaxPaid,
		&parametersVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("scan session summary: %w", err)
	}

	summary.Regime = domain.Regime(regime)
	if riskCommentary != nil {
		summary.RiskCommentary = *riskCommentary
	}
	if parametersVersion != nil {
		summary.ParametersVersion = *parametersVersion
	}
	if afterTaxPnL != nil {
		summary.AfterTaxPnL = *afterTaxPnL
	}
	if totalTaxPaid != nil {
		summary.TotalTaxPaid = *totalTaxPaid
	}
	if len(brokerRuntime) > 0 {
		if err := json.Unmarshal(brokerRuntime, &summary.BrokerRuntime); err != nil {
			return nil, fmt.Errorf("unmarshal broker_runtime: %w", err)
		}
	}
	if len(guardOutcomes) > 0 {
		if err := json.Unmarshal(guardOutcomes, &summary.GuardOutcomes); err != nil {
			return nil, fmt.Errorf("unmarshal guard_outcomes: %w", err)
		}
	}
	if len(taxSnapshots) > 0 {
		if err := json.Unmarshal(taxSnapshots, &summary.TaxSnapshots); err != nil {
			return nil, fmt.Errorf("unmarshal tax_snapshots: %w", err)
		}
	}
	return &summary, nil
}

// ------------------------------------------------------------------
// Trades
// ------------------------------------------------------------------

// RecordSessionTrades persists trades for a session. timestamp is stored
// as RFC3339 matching the SQLite format.
func (s *PostgresLedgerStore) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	if len(trades) == 0 {
		return nil
	}
	ctx := context.Background()

	batch := &pgx.Batch{}
	for _, trade := range trades {
		batch.Queue(`
			INSERT INTO trades (trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, trade.TradeID, sessionID, trade.Symbol,
			string(trade.Side), trade.Quantity, trade.Price, trade.Amount,
			trade.Reason, trade.Timestamp.UTC().Format(time.RFC3339))
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return fmt.Errorf("insert session trades: %w", err)
	}
	return nil
}

// LoadSessionTrades reads trades for a session, oldest first.
func (s *PostgresLedgerStore) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp
		FROM trades WHERE session_id = $1 ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query session trades: %w", err)
	}
	defer rows.Close()
	return scanPGTrades(rows)
}

// LoadAllSessionTrades reads all trades, newest first.
func (s *PostgresLedgerStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp
		FROM trades ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query all trades: %w", err)
	}
	defer rows.Close()
	return scanPGTrades(rows)
}

// scanPGTrades rebuilds TradeRecord rows (RFC3339 timestamps).
func scanPGTrades(rows pgx.Rows) ([]domain.TradeRecord, error) {
	trades := make([]domain.TradeRecord, 0)
	for rows.Next() {
		var rec domain.TradeRecord
		var side, ts string
		if err := rows.Scan(&rec.TradeID, &rec.SessionID, &rec.Symbol, &side,
			&rec.Quantity, &rec.Price, &rec.Amount, &rec.Reason, &ts); err != nil {
			return nil, fmt.Errorf("scan trade row: %w", err)
		}
		rec.Side = domain.Side(side)
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("parse trade timestamp %q: %w", ts, err)
		}
		rec.Timestamp = parsed
		trades = append(trades, rec)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("trade rows: %w", rows.Err())
	}
	return trades, nil
}

// ------------------------------------------------------------------
// Experiment records (part of OutcomeStore; full ExperimentStore surface
// lands in the follow-up PR with the experiments migration)
// ------------------------------------------------------------------

// RecordExperiment writes an experiment record to the global experiments table.
func (s *PostgresLedgerStore) RecordExperiment(record domain.ExperimentRecord) error {
	briefJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}
	ctx := context.Background()
	// #1780: ON CONFLICT DO NOTHING — the auto-experiment pipeline retries the
	// same experiment_id across status transitions and (before #1774 was
	// fixed) across task failures; a duplicate key error here surfaced as a
	// PG ERROR every ~2min during the restart stampede. First write wins;
	// later transitions are append-only via RecordPromptExperimentResult.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO experiments (experiment_id, mutation_brief_json, accepted, timestamp)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (experiment_id) DO NOTHING
	`, record.ID, string(briefJSON), boolToInt(record.Status == domain.ExperimentAccepted),
		record.WindowStart.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return fmt.Errorf("insert experiment: %w", err)
	}
	return nil
}

// RecordSessionExperiment writes an experiment record for a specific session.
func (s *PostgresLedgerStore) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	briefJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO experiments (experiment_id, session_id, mutation_brief_json, accepted, timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`, record.ID, session.ID, string(briefJSON), boolToInt(record.Status == domain.ExperimentAccepted),
		record.WindowStart.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return fmt.Errorf("insert session experiment: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------
// Human interventions
// ------------------------------------------------------------------

// RecordHumanIntervention persists a human intervention (mirror of
// repository.postgres_audit.go RecordHumanIntervention).
func (s *PostgresLedgerStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO human_interventions (time, intervention_id, type, target_agent_id, target_model_id, target_sector, target_symbol, value, reason, operator, session_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (intervention_id) DO NOTHING
	`, intervention.RecordedAt, intervention.ID, intervention.Type, intervention.TargetAgentID,
		intervention.TargetModelID, intervention.TargetSector, intervention.TargetSymbol,
		intervention.Value, intervention.Reason, intervention.Operator, intervention.SessionID)
	if err != nil {
		return fmt.Errorf("record human intervention: %w", err)
	}
	return nil
}

// LoadHumanInterventions reads all human interventions, newest first.
func (s *PostgresLedgerStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT time, intervention_id, type, target_agent_id, target_model_id, target_sector, target_symbol, value, reason, operator, session_id
		FROM human_interventions
		ORDER BY time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("load human interventions: %w", err)
	}
	defer rows.Close()

	var interventions []domain.HumanIntervention
	for rows.Next() {
		var hi domain.HumanIntervention
		if err := rows.Scan(
			&hi.RecordedAt, &hi.ID, &hi.Type, &hi.TargetAgentID, &hi.TargetModelID,
			&hi.TargetSector, &hi.TargetSymbol, &hi.Value, &hi.Reason, &hi.Operator, &hi.SessionID,
		); err != nil {
			return nil, fmt.Errorf("scan human intervention: %w", err)
		}
		interventions = append(interventions, hi)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("human intervention rows: %w", rows.Err())
	}
	return interventions, nil
}

// ------------------------------------------------------------------
// Experiment store surface
// ------------------------------------------------------------------

// LoadExperiments reads all experiment records, newest first (mirror of
// SQLiteStore.LoadExperiments).
func (s *PostgresLedgerStore) LoadExperiments() ([]domain.ExperimentRecord, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `SELECT mutation_brief_json FROM experiments ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query experiments: %w", err)
	}
	defer rows.Close()

	var records []domain.ExperimentRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan experiment: %w", err)
		}
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			return nil, fmt.Errorf("unmarshal experiment: %w", err)
		}
		records = append(records, rec)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("experiment rows: %w", rows.Err())
	}
	return records, nil
}

// RecordPromptExperimentResult persists a prompt experiment result as a JSON blob.
func (s *PostgresLedgerStore) RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal prompt experiment result: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO prompt_experiment_results (experiment_id, data_json, created_at)
		VALUES ($1, $2, $3)
	`, experimentID, string(data), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert prompt experiment result: %w", err)
	}
	return nil
}

// UpdatePromptExperimentResult replaces an existing prompt experiment result by experiment_id.
func (s *PostgresLedgerStore) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal prompt experiment result: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO prompt_experiment_results (experiment_id, data_json, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (experiment_id) DO UPDATE SET
			data_json = excluded.data_json,
			created_at = excluded.created_at
	`, experimentID, string(data), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert prompt experiment result: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------
// Backtest store surface
// ------------------------------------------------------------------

// RecordWindowSummary persists a backtest window summary as a JSON blob.
func (s *PostgresLedgerStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal window summary: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO window_summaries (window_id, data_json, created_at)
		VALUES ($1, $2, $3)
	`, summary.WindowID, string(data), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert window summary: %w", err)
	}
	return nil
}

// RecordMutationBrief persists a mutation brief as a JSON blob.
func (s *PostgresLedgerStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	data, err := json.Marshal(brief)
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ($1, $2, $3)
	`, windowID, string(data), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert mutation brief: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------
// Spawn records
// ------------------------------------------------------------------

// RecordSpawnRecord persists a spawn record as a JSON blob.
func (s *PostgresLedgerStore) RecordSpawnRecord(record SpawnRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal spawn record: %w", err)
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO spawn_records (data_json, created_at)
		VALUES ($1, $2)
	`, string(data), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert spawn record: %w", err)
	}
	return nil
}

// LoadSpawnRecords reads all spawn records, most recent first.
func (s *PostgresLedgerStore) LoadSpawnRecords() ([]SpawnRecord, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `SELECT data_json FROM spawn_records ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query spawn records: %w", err)
	}
	defer rows.Close()

	var records []SpawnRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan spawn record: %w", err)
		}
		var rec SpawnRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			return nil, fmt.Errorf("unmarshal spawn record: %w", err)
		}
		records = append(records, rec)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("spawn record rows: %w", rows.Err())
	}
	return records, nil
}
