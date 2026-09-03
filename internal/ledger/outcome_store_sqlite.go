package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SQLiteOutcomeStore implements OutcomeStore backed by SQLite.
//
// The outcomes table stores recommendation records only — evaluation fields
// (Hit / ForwardReturn / Window / Skill / BenchmarkDelta) are NOT persisted
// in the schema. WithJSONLBaseDir enables rich per-session JSONL reads so
// Load* methods return full evaluation data (see ledger.Store), falling back
// to the truncated SQLite rows when no JSONL data exists for a session.
type SQLiteOutcomeStore struct {
	db    *sql.DB
	jsonl OutcomeStore // rich JSONL store (nil = SQLite-only reads)
}

var _ OutcomeStore = (*SQLiteOutcomeStore)(nil)

// NewSQLiteOutcomeStore creates a new SQLite-backed outcome store.
func NewSQLiteOutcomeStore(db *sql.DB) *SQLiteOutcomeStore {
	return &SQLiteOutcomeStore{db: db}
}

// WithJSONLBaseDir enables rich per-session JSONL reads rooted at baseDir
// (typically cfg.LedgerDir). Without it, Load* methods read the truncated
// SQLite table where evaluation fields are always zero.
func (s *SQLiteOutcomeStore) WithJSONLBaseDir(baseDir string) *SQLiteOutcomeStore {
	if baseDir != "" {
		s.jsonl = NewStore(baseDir)
	}
	return s
}

// RecordOutcomes writes a batch of recommendation outcomes to the global outcomes table.
func (s *SQLiteOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, weight, target_price,
			stop_loss, conviction, regime, timestamp, passed_guards, guard_reason,
			factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, outcome := range outcomes {
		action := string(outcome.Side)
		ts := outcome.RecordedAt.Format("2006-01-02T15:04:05Z07:00")

		factorJSON, err := json.Marshal(outcome.FactorScores)
		if err != nil {
			return fmt.Errorf("marshal factor_scores: %w", err)
		}
		var convictionJSON []byte
		if outcome.ConvictionBreakdown != nil {
			convictionJSON, err = json.Marshal(outcome.ConvictionBreakdown)
			if err != nil {
				return fmt.Errorf("marshal conviction_breakdown: %w", err)
			}
		}

		_, err = stmt.Exec(
			"", // session_id - global outcomes have empty session_id
			outcome.Symbol,
			outcome.AgentID,
			action,
			nil, // weight - not in outcome type
			outcome.TargetPrice,
			outcome.StopLossPrice,
			outcome.Conviction,
			outcome.Regime,
			ts,
			outcome.PassedGuards,
			outcome.GuardReason,
			string(factorJSON),
			string(convictionJSON),
			string(outcome.Layer),
			outcome.ForwardReturn,
			outcome.Window,
			boolToInt(outcome.Hit),
			outcome.BenchmarkDelta,
			boolToInt(outcome.IsSynthetic),
			outcome.Regime,
			nullableOutcomeText(outcome.MarketPeriod),
			nullableOutcomeText(outcome.MarketPeriodSource),
		)
		if err != nil {
			return fmt.Errorf("insert outcome: %w", err)
		}
	}

	return tx.Commit()
}

// RecordSessionOutcomes writes outcomes for a specific session.
func (s *SQLiteOutcomeStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, weight, target_price,
			stop_loss, conviction, regime, timestamp, passed_guards, guard_reason,
			factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, outcome := range outcomes {
		action := string(outcome.Side)
		ts := outcome.RecordedAt.Format("2006-01-02T15:04:05Z07:00")

		factorJSON, err := json.Marshal(outcome.FactorScores)
		if err != nil {
			return fmt.Errorf("marshal factor_scores: %w", err)
		}
		var convictionJSON []byte
		if outcome.ConvictionBreakdown != nil {
			convictionJSON, err = json.Marshal(outcome.ConvictionBreakdown)
			if err != nil {
				return fmt.Errorf("marshal conviction_breakdown: %w", err)
			}
		}

		_, err = stmt.Exec(
			session.ID,
			outcome.Symbol,
			outcome.AgentID,
			action,
			nil,
			outcome.TargetPrice,
			outcome.StopLossPrice,
			outcome.Conviction,
			outcome.Regime,
			ts,
			outcome.PassedGuards,
			outcome.GuardReason,
			string(factorJSON),
			string(convictionJSON),
			string(outcome.Layer),
			outcome.ForwardReturn,
			outcome.Window,
			boolToInt(outcome.Hit),
			outcome.BenchmarkDelta,
			boolToInt(outcome.IsSynthetic),
			outcome.Regime,
			nullableOutcomeText(outcome.MarketPeriod),
			nullableOutcomeText(outcome.MarketPeriodSource),
		)
		if err != nil {
			return fmt.Errorf("insert outcome: %w", err)
		}
	}

	return tx.Commit()
}

// LoadOutcomesFromSessions 聚合所有 session 的 outcomes，包含 session_id != ” 的列。
// 與 LoadOutcomes()（僅 session_id = ”，稀疏的全域檔）不同，
// 此方法符合 internal/ledger/AGENTS.md 規範：「richest data source with
// per-agent, per-symbol forward returns」。
//
// 當 WithJSONLBaseDir 已啟用時，優先委派 per-session JSONL（唯一保存
// Hit/ForwardReturn/Window 的 rich source）；JSONL 無資料時 fallback 到
// SQLite 表（僅 recommendation 記錄，評估欄位為零）。
func (s *SQLiteOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	if s.jsonl != nil {
		if outcomes, err := s.jsonl.LoadOutcomesFromSessions(); err == nil && len(outcomes) > 0 {
			return outcomes, nil
		}
	}
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source
		FROM outcomes WHERE session_id != '' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query outcomes from sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanOutcomes(rows)
}

// LoadOutcomes 讀取全域 sparse outcomes（session_id = ”），對應
// baseDir/recommendation_outcomes.jsonl。Per-AGENTS.md 規範，請勿用此方法
// 計算單場 OutcomeCount。
func (s *SQLiteOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source
		FROM outcomes WHERE session_id = '' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanOutcomes(rows)
}

// LoadSessionOutcomes reads outcomes for a specific session.
// With WithJSONLBaseDir enabled, reads the rich per-session JSONL first;
// falls back to the truncated SQLite rows when no JSONL data exists.
func (s *SQLiteOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	if s.jsonl != nil {
		if outcomes, err := s.jsonl.LoadSessionOutcomes(sessionID); err == nil && len(outcomes) > 0 {
			return outcomes, nil
		}
	}
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source
		FROM outcomes WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query session outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanOutcomes(rows)
}

// RecordSessionScreeningRejects persists screening rejects for a session.
func (s *SQLiteOutcomeStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO screening_rejects (session_id, symbol, reason, timestamp, factor_scores_json)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, reject := range rejects {
		factorJSON, err := json.Marshal(reject.FactorScores)
		if err != nil {
			return fmt.Errorf("marshal factor_scores: %w", err)
		}
		ts := reject.RecordedAt.Format("2006-01-02T15:04:05Z07:00")

		_, err = stmt.Exec(
			sessionID,
			reject.Symbol,
			reject.CriterionLabel,
			ts,
			string(factorJSON),
		)
		if err != nil {
			return fmt.Errorf("insert screening reject: %w", err)
		}
	}

	return tx.Commit()
}

// LoadSessionScreeningRejects reads screening rejects for a session.
func (s *SQLiteOutcomeStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	rows, err := s.db.Query(`
		SELECT symbol, reason, timestamp, factor_scores_json
		FROM screening_rejects WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query screening rejects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rejects []domain.ScreeningReject
	for rows.Next() {
		var sym, reason, ts, factorJSON string
		if err := rows.Scan(&sym, &reason, &ts, &factorJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		var fs domain.FactorScores
		if factorJSON != "" {
			if err := json.Unmarshal([]byte(factorJSON), &fs); err != nil {
				return nil, fmt.Errorf("unmarshal factor_scores: %w", err)
			}
		}

		rejects = append(rejects, domain.ScreeningReject{
			SessionID:      sessionID,
			Symbol:         sym,
			Criterion:      "",
			CriterionLabel: reason,
			FactorScores:   fs,
			RecordedAt:     parseTimestamp(ts),
		})
	}

	return rejects, rows.Err()
}

func (s *SQLiteOutcomeStore) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	if len(trades) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO trades (trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, trade := range trades {
		ts := trade.Timestamp.Format("2006-01-02T15:04:05Z07:00")
		if _, err := stmt.Exec(trade.TradeID, sessionID, trade.Symbol,
			string(trade.Side), trade.Quantity, trade.Price, trade.Amount,
			trade.Reason, ts); err != nil {
			return fmt.Errorf("insert trade %s: %w", trade.TradeID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *SQLiteOutcomeStore) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	rows, err := s.db.Query(`
		SELECT trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp
		FROM trades WHERE session_id = ? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query session trades: %w", err)
	}
	defer func() { _ = rows.Close() }()

	trades := make([]domain.TradeRecord, 0)
	for rows.Next() {
		var rec domain.TradeRecord
		var side, ts string
		if err := rows.Scan(&rec.TradeID, &rec.SessionID, &rec.Symbol, &side,
			&rec.Quantity, &rec.Price, &rec.Amount, &rec.Reason, &ts); err != nil {
			return nil, fmt.Errorf("scan trade row: %w", err)
		}
		rec.Side = domain.Side(side)
		rec.Timestamp, _ = time.Parse("2006-01-02T15:04:05Z07:00", ts)
		trades = append(trades, rec)
	}
	return trades, rows.Err()
}

func (s *SQLiteOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	rows, err := s.db.Query(`
		SELECT trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp
		FROM trades ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("query all trades: %w", err)
	}
	defer func() { _ = rows.Close() }()

	trades := make([]domain.TradeRecord, 0)
	for rows.Next() {
		var rec domain.TradeRecord
		var side, ts string
		if err := rows.Scan(&rec.TradeID, &rec.SessionID, &rec.Symbol, &side,
			&rec.Quantity, &rec.Price, &rec.Amount, &rec.Reason, &ts); err != nil {
			return nil, fmt.Errorf("scan trade row: %w", err)
		}
		rec.Side = domain.Side(side)
		rec.Timestamp, _ = time.Parse("2006-01-02T15:04:05Z07:00", ts)
		trades = append(trades, rec)
	}
	return trades, rows.Err()
}

// RecordExperiment writes an experiment record to the global experiments table.
func (s *SQLiteOutcomeStore) RecordExperiment(record domain.ExperimentRecord) error {
	briefJSON, err := json.Marshal(record) // mutation brief fields embedded in record
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}

	_, err = s.db.Exec(
		`
		INSERT INTO experiments (experiment_id, mutation_brief_json, accepted, timestamp)
		VALUES (?, ?, ?, ?)`,
		record.ID,
		string(briefJSON),
		record.Status == domain.ExperimentAccepted,
		record.WindowStart.Format("2006-01-02T15:04:05Z07:00"),
	)
	if err != nil {
		return fmt.Errorf("insert experiment: %w", err)
	}

	return nil
}

// RecordSessionExperiment writes an experiment record for a specific session.
func (s *SQLiteOutcomeStore) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	briefJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}

	_, err = s.db.Exec(
		`
		INSERT INTO experiments (experiment_id, session_id, mutation_brief_json, accepted, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		record.ID,
		session.ID,
		string(briefJSON),
		record.Status == domain.ExperimentAccepted,
		record.WindowStart.Format("2006-01-02T15:04:05Z07:00"),
	)
	if err != nil {
		return fmt.Errorf("insert session experiment: %w", err)
	}

	return nil
}

// RecordSessionSummary persists a session summary.
//
// BL-01/BL-04 fix (perf-report-zero audit): this store is the sim's outcome
// store when StoreBackend=sqlite, so the summary it writes must carry the full
// SessionSummary (including PortfolioValue / PositionCount / OrderCount) that
// the performance report consumes. Prior to the fix it wrote only the 5-column
// projection (total_recs/passed_guards/rejected/timestamp), leaving
// PortfolioValue zero and starving the report of equity data for 7/21+ sessions.
// We now also persist summary_json (mirroring SQLiteSessionStore) and read it
// back in LoadSessionSummaries so the report and the sim agree on one source.
func (s *SQLiteOutcomeStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
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

	_, err = s.db.Exec(
		`
		INSERT INTO session_summaries (session_id, total_recs, passed_guards, rejected, timestamp, summary_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			total_recs = excluded.total_recs,
			passed_guards = excluded.passed_guards,
			rejected = excluded.rejected,
			timestamp = excluded.timestamp,
			summary_json = excluded.summary_json`,
		session.ID,
		summary.OutcomeCount,
		countPassedGuards(summary.GuardOutcomes),
		len(summary.GuardOutcomes)-countPassedGuards(summary.GuardOutcomes),
		summary.RecordedAt.Format("2006-01-02T15:04:05Z07:00"),
		string(summaryJSON),
	)
	if err != nil {
		return fmt.Errorf("insert session summary: %w", err)
	}

	return nil
}

// LoadSessionSummaries reads all session summaries.
//
// BL-01 fix: prefer the full summary_json (written by RecordSessionSummary)
// which carries PortfolioValue/PositionCount/OrderCount; fall back to the
// legacy 5-column projection for rows written before the fix. This is what
// lets the performance report see real equity data for StoreBackend=sqlite.
func (s *SQLiteOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	// Ensure the summary_json column exists (idempotent; production DBs get it
	// via ALTER in RecordSessionSummary, but an in-memory test schema may not).
	_, _ = s.db.Exec(`ALTER TABLE session_summaries ADD COLUMN summary_json TEXT`) //nolint:errcheck
	rows, err := s.db.Query(`
		SELECT session_id, total_recs, passed_guards, rejected, timestamp, COALESCE(summary_json, '')
		FROM session_summaries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query session summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []domain.SessionSummary
	for rows.Next() {
		var sid string
		var total, passed, rejected int
		var ts, summaryJSON string
		if err := rows.Scan(&sid, &total, &passed, &rejected, &ts, &summaryJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if summaryJSON != "" {
			var summary domain.SessionSummary
			if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
				return nil, fmt.Errorf("unmarshal session summary %s: %w", sid, err)
			}
			summaries = append(summaries, summary)
			continue
		}

		summaries = append(summaries, domain.SessionSummary{
			SessionID:    sid,
			OutcomeCount: total,
			RecordedAt:   parseTimestamp(ts),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}

	return summaries, nil
}

// LoadAllSessionScorecards aggregates outcomes into scorecards.
// Returns empty scorecards slice — not yet implemented in SQLite.
func (s *SQLiteOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json,
			layer, forward_return, window, hit, benchmark_delta, is_synthetic, true_regime,
			market_period, market_period_source
		FROM outcomes WHERE session_id != '' ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("query outcomes for scorecards: %w", err)
	}
	defer func() { _ = rows.Close() }()

	outcomes, err := scanOutcomes(rows)
	if err != nil {
		return nil, nil, err
	}

	return []domain.Scorecard{}, outcomes, nil
}

// RecordHumanIntervention persists a human intervention record.
func (s *SQLiteOutcomeStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	ts := intervention.RecordedAt.Format("2006-01-02T15:04:05Z07:00")
	_, err := s.db.Exec(
		`
		INSERT INTO human_interventions (session_id, symbol, agent_id, action, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		intervention.SessionID,
		intervention.TargetSymbol,
		intervention.TargetAgentID,
		intervention.Type,
		ts,
	)
	if err != nil {
		return fmt.Errorf("insert human intervention: %w", err)
	}

	return nil
}

// LoadHumanInterventions reads all human intervention records.
func (s *SQLiteOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	rows, err := s.db.Query(`
		SELECT session_id, symbol, agent_id, action, timestamp
		FROM human_interventions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query human interventions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var interventions []domain.HumanIntervention
	for rows.Next() {
		var sid, sym, agentID, action, ts string
		if err := rows.Scan(&sid, &sym, &agentID, &action, &ts); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		interventions = append(interventions, domain.HumanIntervention{
			SessionID:     sid,
			TargetSymbol:  sym,
			TargetAgentID: agentID,
			Type:          action,
			RecordedAt:    parseTimestamp(ts),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}

	return interventions, nil
}

// scanOutcomes scans outcome rows into RecommendationOutcome structs.
// Reads the evaluation columns (layer/forward_return/window/hit/benchmark_delta/
// is_synthetic/true_regime) added by BL-06. For rows written before the
// migration, the new columns are NULL → zero values (Layer falls back to the
// legacy regime column which historically stored the layer).
func scanOutcomes(rows *sql.Rows) ([]domain.RecommendationOutcome, error) {
	var outcomes []domain.RecommendationOutcome
	for rows.Next() {
		var sym, agentID string
		var action, regime, ts, guardReason, factorJSON, convictionJSON sql.NullString
		var passedGuards bool
		// Optional columns may be NULL (partial writes or legacy rows) →
		// scan with Null types and treat NULL as zero/empty. Includes the BL-06
		// evaluation columns added by migration.
		var targetPrice, stopLoss, forwardReturn, benchmarkDelta sql.NullFloat64
		var conviction, hit, isSynthetic sql.NullInt64
		var layer, window, trueRegime, marketPeriod, marketPeriodSource sql.NullString

		if err := rows.Scan(&sym, &agentID, &action, &targetPrice, &stopLoss, &conviction,
			&regime, &ts, &passedGuards, &guardReason, &factorJSON, &convictionJSON,
			&layer, &forwardReturn, &window, &hit, &benchmarkDelta, &isSynthetic, &trueRegime,
			&marketPeriod, &marketPeriodSource); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		var fs domain.FactorScores
		if factorJSON.String != "" {
			if err := json.Unmarshal([]byte(factorJSON.String), &fs); err != nil {
				return nil, fmt.Errorf("unmarshal factor_scores: %w", err)
			}
		}

		var cb *domain.ConvictionBreakdown
		if convictionJSON.String != "" {
			var breakdown domain.ConvictionBreakdown
			if err := json.Unmarshal([]byte(convictionJSON.String), &breakdown); err != nil {
				return nil, fmt.Errorf("unmarshal conviction_breakdown: %w", err)
			}
			cb = &breakdown
		}

		// BL-06: prefer the dedicated layer column; fall back to the legacy
		// regime column (which historically stored the layer) for pre-migration rows.
		effectiveLayer := layer.String
		if effectiveLayer == "" {
			effectiveLayer = regime.String
		}

		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:             agentID,
			Symbol:              sym,
			Side:                domain.Side(action.String),
			TargetPrice:         targetPrice.Float64,
			StopLossPrice:       stopLoss.Float64,
			Conviction:          int(conviction.Int64),
			Layer:               domain.AgentLayer(effectiveLayer),
			Regime:              trueRegime.String,
			Window:              window.String,
			MarketPeriod:        marketPeriod.String,
			MarketPeriodSource:  marketPeriodSource.String,
			ForwardReturn:       forwardReturn.Float64,
			BenchmarkDelta:      benchmarkDelta.Float64,
			Hit:                 hit.Int64 == 1,
			IsSynthetic:         isSynthetic.Int64 == 1,
			RecordedAt:          parseTimestamp(ts.String),
			PassedGuards:        passedGuards,
			GuardReason:         guardReason.String,
			FactorScores:        fs,
			ConvictionBreakdown: cb,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}

	return outcomes, nil
}

// boolToInt converts a bool to 0/1 for SQLite INTEGER columns.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableOutcomeText returns nil for an empty string so empty
// market_period / market_period_source values persist as SQL NULL (legacy
// rows written before the Phase 2 PR-2a columns read back as empty strings).
func nullableOutcomeText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// countPassedGuards counts guard outcomes that passed.
func countPassedGuards(outcomes []domain.GuardOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Passed {
			n++
		}
	}
	return n
}

// parseTimestamp parses ISO8601 timestamps from SQLite.
func parseTimestamp(s string) (t time.Time) {
	t, _ = time.Parse("2006-01-02T15:04:05Z07:00", s) //nolint:errcheck
	return t
}
