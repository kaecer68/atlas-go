package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SQLiteOutcomeStore implements OutcomeStore backed by SQLite.
type SQLiteOutcomeStore struct {
	db *sql.DB
}

var _ OutcomeStore = (*SQLiteOutcomeStore)(nil)

// NewSQLiteOutcomeStore creates a new SQLite-backed outcome store.
func NewSQLiteOutcomeStore(db *sql.DB) *SQLiteOutcomeStore {
	return &SQLiteOutcomeStore{db: db}
}

// RecordOutcomes writes a batch of recommendation outcomes to the global outcomes table.
func (s *SQLiteOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, weight, target_price,
			stop_loss, conviction, regime, timestamp, passed_guards, guard_reason,
			factor_scores_json, conviction_breakdown_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, outcome := range outcomes {
		action := string(outcome.Side)
		regime := string(outcome.Layer)
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
			regime,
			ts,
			outcome.PassedGuards,
			outcome.GuardReason,
			string(factorJSON),
			string(convictionJSON),
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
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO outcomes (session_id, symbol, agent_id, action, weight, target_price,
			stop_loss, conviction, regime, timestamp, passed_guards, guard_reason,
			factor_scores_json, conviction_breakdown_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, outcome := range outcomes {
		action := string(outcome.Side)
		regime := string(outcome.Layer)
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
			regime,
			ts,
			outcome.PassedGuards,
			outcome.GuardReason,
			string(factorJSON),
			string(convictionJSON),
		)
		if err != nil {
			return fmt.Errorf("insert outcome: %w", err)
		}
	}

	return tx.Commit()
}

// LoadOutcomes reads all global outcomes (session_id = "").
func (s *SQLiteOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json
		FROM outcomes WHERE session_id = '' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query outcomes: %w", err)
	}
	defer rows.Close()

	return scanOutcomes(rows)
}

// LoadSessionOutcomes reads outcomes for a specific session.
func (s *SQLiteOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	rows, err := s.db.Query(`
		SELECT symbol, agent_id, action, target_price, stop_loss, conviction,
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json
		FROM outcomes WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query session outcomes: %w", err)
	}
	defer rows.Close()

	return scanOutcomes(rows)
}

// RecordSessionScreeningRejects persists screening rejects for a session.
func (s *SQLiteOutcomeStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO screening_rejects (session_id, symbol, reason, timestamp, factor_scores_json)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

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
	defer rows.Close()

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

// RecordExperiment writes an experiment record to the global experiments table.
func (s *SQLiteOutcomeStore) RecordExperiment(record domain.ExperimentRecord) error {
	briefJSON, err := json.Marshal(record) // mutation brief fields embedded in record
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}

	_, err = s.db.Exec(`
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

	_, err = s.db.Exec(`
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
func (s *SQLiteOutcomeStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	_, err := s.db.Exec(`
		INSERT INTO session_summaries (session_id, total_recs, passed_guards, rejected, timestamp)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			total_recs = excluded.total_recs,
			passed_guards = excluded.passed_guards,
			rejected = excluded.rejected,
			timestamp = excluded.timestamp`,
		session.ID,
		summary.OutcomeCount,
		countPassedGuards(summary.GuardOutcomes),
		len(summary.GuardOutcomes)-countPassedGuards(summary.GuardOutcomes),
		summary.RecordedAt.Format("2006-01-02T15:04:05Z07:00"),
	)
	if err != nil {
		return fmt.Errorf("insert session summary: %w", err)
	}

	return nil
}

// LoadSessionSummaries reads all session summaries.
func (s *SQLiteOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	rows, err := s.db.Query(`
		SELECT session_id, total_recs, passed_guards, rejected, timestamp
		FROM session_summaries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query session summaries: %w", err)
	}
	defer rows.Close()

	var summaries []domain.SessionSummary
	for rows.Next() {
		var sid string
		var total, passed, rejected int
		var ts string
		if err := rows.Scan(&sid, &total, &passed, &rejected, &ts); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
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
			regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json
		FROM outcomes WHERE session_id != '' ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("query outcomes for scorecards: %w", err)
	}
	defer rows.Close()

	outcomes, err := scanOutcomes(rows)
	if err != nil {
		return nil, nil, err
	}

	return []domain.Scorecard{}, outcomes, nil
}

// RecordHumanIntervention persists a human intervention record.
func (s *SQLiteOutcomeStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	ts := intervention.RecordedAt.Format("2006-01-02T15:04:05Z07:00")
	_, err := s.db.Exec(`
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
	defer rows.Close()

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
func scanOutcomes(rows *sql.Rows) ([]domain.RecommendationOutcome, error) {
	var outcomes []domain.RecommendationOutcome
	for rows.Next() {
		var sym, agentID, action, regime, ts, guardReason, factorJSON, convictionJSON string
		var targetPrice, stopLoss float64
		var conviction int
		var passedGuards bool

		if err := rows.Scan(&sym, &agentID, &action, &targetPrice, &stopLoss, &conviction,
			&regime, &ts, &passedGuards, &guardReason, &factorJSON, &convictionJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		var fs domain.FactorScores
		if factorJSON != "" {
			if err := json.Unmarshal([]byte(factorJSON), &fs); err != nil {
				return nil, fmt.Errorf("unmarshal factor_scores: %w", err)
			}
		}

		var cb *domain.ConvictionBreakdown
		if convictionJSON != "" {
			var breakdown domain.ConvictionBreakdown
			if err := json.Unmarshal([]byte(convictionJSON), &breakdown); err != nil {
				return nil, fmt.Errorf("unmarshal conviction_breakdown: %w", err)
			}
			cb = &breakdown
		}

		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:             agentID,
			Symbol:              sym,
			Side:                domain.Side(action),
			TargetPrice:         targetPrice,
			StopLossPrice:       stopLoss,
			Conviction:          conviction,
			Layer:               domain.AgentLayer(regime),
			RecordedAt:          parseTimestamp(ts),
			PassedGuards:        passedGuards,
			GuardReason:         guardReason,
			FactorScores:        fs,
			ConvictionBreakdown: cb,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}

	return outcomes, nil
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
	t, _ = time.Parse("2006-01-02T15:04:05Z07:00", s)
	return t
}
