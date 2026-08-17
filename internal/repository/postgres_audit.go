package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func (r *PostgresRepository) RecordScreeningRejects(ctx context.Context, sessionID string, rejects []domain.ScreeningReject) error {
	if len(rejects) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, sr := range rejects {
		factorScores, _ := json.Marshal(sr.FactorScores)
		batch.Queue(`
			INSERT INTO screening_rejects (time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, sr.RecordedAt, sr.SessionID, sr.Symbol, sr.AgentID, sr.Skill,
			sr.Criterion, sr.CriterionLabel, sr.Threshold, sr.ActualValue, factorScores)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	_, err := br.Exec()
	if err != nil {
		return fmt.Errorf("insert screening rejects: %w", err)
	}
	return nil
}

func (r *PostgresRepository) QueryScreeningRejectsBySession(ctx context.Context, sessionID string) ([]domain.ScreeningReject, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores
		FROM screening_rejects
		WHERE session_id = $1
		ORDER BY time DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query screening rejects by session: %w", err)
	}
	defer rows.Close()

	return scanScreeningRejects(rows)
}

func scanScreeningRejects(rows pgx.Rows) ([]domain.ScreeningReject, error) {
	var rejects []domain.ScreeningReject
	for rows.Next() {
		var sr domain.ScreeningReject
		var skill, criterionLabel, threshold, actualValue sql.NullString
		var factorScores []byte
		err := rows.Scan(
			&sr.RecordedAt, &sr.SessionID, &sr.Symbol, &sr.AgentID, &skill,
			&sr.Criterion, &criterionLabel, &threshold, &actualValue, &factorScores,
		)
		if err != nil {
			continue
		}
		sr.Skill = skill.String
		sr.CriterionLabel = criterionLabel.String
		sr.Threshold = threshold.String
		sr.ActualValue = actualValue.String
		if len(factorScores) > 0 {
			if err := json.Unmarshal(factorScores, &sr.FactorScores); err != nil {
				continue
			}
		}
		rejects = append(rejects, sr)
	}
	return rejects, rows.Err()
}

func (r *PostgresRepository) SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error {
	brokerRuntime, _ := json.Marshal(summary.BrokerRuntime)
	guardOutcomes, _ := json.Marshal(summary.GuardOutcomes)
	taxSnapshots, _ := json.Marshal(summary.TaxSnapshots)

	_, err := r.pool.Exec(ctx, `
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

func (r *PostgresRepository) LoadSessionSummary(ctx context.Context, sessionID string) (*domain.SessionSummary, error) {
	var summary domain.SessionSummary
	var regime sql.NullString
	var nextExperimentAgentID, proposalID, commitID, approvalID, riskCommentary, parametersVersion sql.NullString
	var brokerRuntime, guardOutcomes, taxSnapshots []byte

	err := r.pool.QueryRow(ctx, `
		SELECT time, session_id, regime, order_count, position_count, ending_cash, portfolio_value, outcome_count, broker_runtime, next_experiment_agent_id, proposal_id, commit_id, approval_id, guard_outcomes, risk_commentary, tax_snapshots, after_tax_pnl, total_tax_paid, parameters_version
		FROM session_summaries
		WHERE session_id = $1
	`, sessionID).Scan(
		&summary.RecordedAt, &summary.SessionID, &regime, &summary.OrderCount,
		&summary.PositionCount, &summary.EndingCash, &summary.PortfolioValue, &summary.OutcomeCount,
		&brokerRuntime, &nextExperimentAgentID, &proposalID, &commitID,
		&approvalID, &guardOutcomes, &riskCommentary, &taxSnapshots, &summary.AfterTaxPnL, &summary.TotalTaxPaid,
		&parametersVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load session summary: %w", err)
	}

	summary.Regime = domain.Regime(regime.String)
	summary.NextExperimentAgentID = nextExperimentAgentID.String
	summary.ProposalID = proposalID.String
	summary.CommitID = commitID.String
	summary.ApprovalID = approvalID.String
	summary.RiskCommentary = riskCommentary.String
	summary.ParametersVersion = parametersVersion.String
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

func (r *PostgresRepository) LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error) {
	rows, err := r.pool.Query(ctx, `
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
		var summary domain.SessionSummary
		var regime sql.NullString
		var nextExperimentAgentID, proposalID, commitID, approvalID, riskCommentary, parametersVersion sql.NullString
		var brokerRuntime, guardOutcomes, taxSnapshots []byte

		err := rows.Scan(
			&summary.RecordedAt, &summary.SessionID, &regime, &summary.OrderCount,
			&summary.PositionCount, &summary.EndingCash, &summary.PortfolioValue, &summary.OutcomeCount,
			&brokerRuntime, &nextExperimentAgentID, &proposalID, &commitID,
			&approvalID, &guardOutcomes, &riskCommentary, &taxSnapshots, &summary.AfterTaxPnL, &summary.TotalTaxPaid,
			&parametersVersion,
		)
		if err != nil {
			continue
		}
		summary.Regime = domain.Regime(regime.String)
		summary.NextExperimentAgentID = nextExperimentAgentID.String
		summary.ProposalID = proposalID.String
		summary.CommitID = commitID.String
		summary.ApprovalID = approvalID.String
		summary.RiskCommentary = riskCommentary.String
		summary.ParametersVersion = parametersVersion.String
		if len(brokerRuntime) > 0 {
			if err := json.Unmarshal(brokerRuntime, &summary.BrokerRuntime); err != nil {
				continue
			}
		}
		if len(guardOutcomes) > 0 {
			if err := json.Unmarshal(guardOutcomes, &summary.GuardOutcomes); err != nil {
				continue
			}
		}
		if len(taxSnapshots) > 0 {
			if err := json.Unmarshal(taxSnapshots, &summary.TaxSnapshots); err != nil {
				continue
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (r *PostgresRepository) RecordHumanIntervention(ctx context.Context, intervention domain.HumanIntervention) error {
	_, err := r.pool.Exec(ctx, `
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

func (r *PostgresRepository) LoadHumanInterventions(ctx context.Context) ([]domain.HumanIntervention, error) {
	rows, err := r.pool.Query(ctx, `
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
		var targetAgentID, targetModelID, targetSector, targetSymbol, reason, operator, sessionID sql.NullString
		err := rows.Scan(
			&hi.RecordedAt, &hi.ID, &hi.Type, &targetAgentID, &targetModelID,
			&targetSector, &targetSymbol, &hi.Value, &reason, &operator, &sessionID,
		)
		if err != nil {
			continue
		}
		hi.TargetAgentID = targetAgentID.String
		hi.TargetModelID = targetModelID.String
		hi.TargetSector = targetSector.String
		hi.TargetSymbol = targetSymbol.String
		hi.Reason = reason.String
		hi.Operator = operator.String
		hi.SessionID = sessionID.String
		interventions = append(interventions, hi)
	}
	return interventions, rows.Err()
}

func (r *PostgresRepository) QueryAllSessionScorecards(ctx context.Context) ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	outcomes, err := r.QueryAllOutcomes(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("query all outcomes: %w", err)
	}
	scorecards := ledger.BuildScorecards(outcomes)
	return scorecards, outcomes, nil
}
