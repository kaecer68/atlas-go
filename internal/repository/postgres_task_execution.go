package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type TaskExecutionStore struct {
	pg *PostgresRepository
}

func NewTaskExecutionStore(pg *PostgresRepository) *TaskExecutionStore {
	return &TaskExecutionStore{pg: pg}
}

func (s *TaskExecutionStore) CreateExecution(ctx context.Context, exec domain.TaskExecution) error {
	_, err := s.pg.pool.Exec(ctx, `
		INSERT INTO task_executions (
			id, task_type, command_name, command_args, request_payload, status, actor, actor_source,
			idempotency_key, retry_of, parent_execution_id, experiment_id, result_path,
			baseline_version_before, baseline_version_after, requires_confirmation,
			confirmed_at, cancel_requested_at, submitted_at, started_at, finished_at,
			exit_code, error_message, summary
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
	`, exec.ID, exec.TaskType, exec.CommandName, mustJSON(exec.CommandArgs), exec.RequestPayload, exec.Status, exec.Actor, exec.ActorSource,
		exec.IdempotencyKey, exec.RetryOf, exec.ParentExecutionID, exec.ExperimentID, exec.ResultPath,
		exec.BaselineVersionBefore, exec.BaselineVersionAfter, exec.RequiresConfirmation,
		exec.ConfirmedAt, exec.CancelRequestedAt, exec.SubmittedAt, exec.StartedAt, exec.FinishedAt,
		exec.ExitCode, exec.ErrorMessage, exec.Summary)
	if err != nil {
		return fmt.Errorf("insert task execution: %w", err)
	}
	return nil
}

func (s *TaskExecutionStore) UpdateExecution(ctx context.Context, exec domain.TaskExecution) error {
	_, err := s.pg.pool.Exec(ctx, `
		UPDATE task_executions SET
			status = $2, actor = $3, actor_source = $4, experiment_id = $5, result_path = $6,
			baseline_version_before = $7, baseline_version_after = $8, requires_confirmation = $9,
			confirmed_at = $10, cancel_requested_at = $11, started_at = $12, finished_at = $13,
			exit_code = $14, error_message = $15, summary = $16, updated_at = NOW()
		WHERE id = $1
	`, exec.ID, exec.Status, exec.Actor, exec.ActorSource, exec.ExperimentID, exec.ResultPath,
		exec.BaselineVersionBefore, exec.BaselineVersionAfter, exec.RequiresConfirmation,
		exec.ConfirmedAt, exec.CancelRequestedAt, exec.StartedAt, exec.FinishedAt,
		exec.ExitCode, exec.ErrorMessage, exec.Summary)
	if err != nil {
		return fmt.Errorf("update task execution: %w", err)
	}
	return nil
}

func (s *TaskExecutionStore) GetExecution(ctx context.Context, id string) (*domain.TaskExecution, error) {
	row := s.pg.pool.QueryRow(ctx, `
		SELECT id, task_type, command_name, command_args, request_payload, status, actor, actor_source,
			idempotency_key, retry_of, parent_execution_id, experiment_id, result_path,
			baseline_version_before, baseline_version_after, requires_confirmation,
			confirmed_at, cancel_requested_at, submitted_at, started_at, finished_at,
			exit_code, error_message, summary, created_at, updated_at
		FROM task_executions WHERE id = $1
	`, id)

	var exec domain.TaskExecution
	var argsJSON []byte
	err := row.Scan(
		&exec.ID, &exec.TaskType, &exec.CommandName, &argsJSON, &exec.RequestPayload, &exec.Status, &exec.Actor, &exec.ActorSource,
		&exec.IdempotencyKey, &exec.RetryOf, &exec.ParentExecutionID, &exec.ExperimentID, &exec.ResultPath,
		&exec.BaselineVersionBefore, &exec.BaselineVersionAfter, &exec.RequiresConfirmation,
		&exec.ConfirmedAt, &exec.CancelRequestedAt, &exec.SubmittedAt, &exec.StartedAt, &exec.FinishedAt,
		&exec.ExitCode, &exec.ErrorMessage, &exec.Summary, &exec.CreatedAt, &exec.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}
	if len(argsJSON) > 0 {
		_ = json.Unmarshal(argsJSON, &exec.CommandArgs)
	}
	return &exec, nil
}

func (s *TaskExecutionStore) ListExecutions(ctx context.Context, filter domain.ExecutionFilter) ([]domain.TaskExecution, error) {
	query := `
		SELECT id, task_type, command_name, command_args, request_payload, status, actor, actor_source,
			idempotency_key, retry_of, parent_execution_id, experiment_id, result_path,
			baseline_version_before, baseline_version_after, requires_confirmation,
			confirmed_at, cancel_requested_at, submitted_at, started_at, finished_at,
			exit_code, error_message, summary, created_at, updated_at
		FROM task_executions WHERE 1=1
	`
	args := []any{}
	argIdx := 1

	if filter.TaskType != "" {
		query += fmt.Sprintf(" AND task_type = $%d", argIdx)
		args = append(args, filter.TaskType)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.ExperimentID != "" {
		query += fmt.Sprintf(" AND experiment_id = $%d", argIdx)
		args = append(args, filter.ExperimentID)
		argIdx++
	}
	if filter.Actor != "" {
		query += fmt.Sprintf(" AND actor = $%d", argIdx)
		args = append(args, filter.Actor)
		argIdx++
	}
	if filter.Since != nil {
		query += fmt.Sprintf(" AND submitted_at >= $%d", argIdx)
		args = append(args, *filter.Since)
		argIdx++
	}

	query += " ORDER BY submitted_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
	}

	rows, err := s.pg.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var results []domain.TaskExecution
	for rows.Next() {
		var exec domain.TaskExecution
		var argsJSON []byte
		err := rows.Scan(
			&exec.ID, &exec.TaskType, &exec.CommandName, &argsJSON, &exec.RequestPayload, &exec.Status, &exec.Actor, &exec.ActorSource,
			&exec.IdempotencyKey, &exec.RetryOf, &exec.ParentExecutionID, &exec.ExperimentID, &exec.ResultPath,
			&exec.BaselineVersionBefore, &exec.BaselineVersionAfter, &exec.RequiresConfirmation,
			&exec.ConfirmedAt, &exec.CancelRequestedAt, &exec.SubmittedAt, &exec.StartedAt, &exec.FinishedAt,
			&exec.ExitCode, &exec.ErrorMessage, &exec.Summary, &exec.CreatedAt, &exec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if len(argsJSON) > 0 {
			_ = json.Unmarshal(argsJSON, &exec.CommandArgs)
		}
		results = append(results, exec)
	}
	return results, rows.Err()
}

func (s *TaskExecutionStore) AppendEvent(ctx context.Context, event domain.TaskExecutionEvent) error {
	_, err := s.pg.pool.Exec(ctx, `
		INSERT INTO task_execution_events (execution_id, seq, event_type, stream, level, message, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, event.ExecutionID, event.Sequence, event.EventType, event.Stream, event.Level, event.Message, event.Payload, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert task execution event: %w", err)
	}
	return nil
}

func (s *TaskExecutionStore) ListEventsAfter(ctx context.Context, executionID string, afterSeq int64) ([]domain.TaskExecutionEvent, error) {
	rows, err := s.pg.pool.Query(ctx, `
		SELECT execution_id, seq, event_type, stream, level, message, payload, created_at
		FROM task_execution_events
		WHERE execution_id = $1 AND seq > $2
		ORDER BY seq ASC
	`, executionID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var results []domain.TaskExecutionEvent
	for rows.Next() {
		var ev domain.TaskExecutionEvent
		err := rows.Scan(&ev.ExecutionID, &ev.Sequence, &ev.EventType, &ev.Stream, &ev.Level, &ev.Message, &ev.Payload, &ev.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		results = append(results, ev)
	}
	return results, rows.Err()
}

func (s *TaskExecutionStore) UpsertLineage(ctx context.Context, lineage domain.ExperimentLineageRecord) error {
	_, err := s.pg.pool.Exec(ctx, `
		INSERT INTO experiment_lineage (
			experiment_id, execution_id, parent_experiment_id, root_experiment_id, lineage_depth,
			target_agent_id, target_skill, mutation_type, brief_path, candidate_path, result_path,
			status, git_commit_id, params_snapshot, result_snapshot, baseline_value, candidate_value, improvement_value,
			recorded_at, judged_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (experiment_id) DO UPDATE SET
			status = EXCLUDED.status,
			result_path = EXCLUDED.result_path,
			result_snapshot = EXCLUDED.result_snapshot,
			baseline_value = EXCLUDED.baseline_value,
			candidate_value = EXCLUDED.candidate_value,
			improvement_value = EXCLUDED.improvement_value,
			judged_at = EXCLUDED.judged_at
	`, lineage.ExperimentID, lineage.ExecutionID, lineage.ParentExperimentID, lineage.RootExperimentID, lineage.LineageDepth,
		lineage.TargetAgentID, lineage.TargetSkill, lineage.MutationType, lineage.BriefPath, lineage.CandidatePath, lineage.ResultPath,
		lineage.Status, lineage.GitCommitID, lineage.ParamsSnapshot, lineage.ResultSnapshot,
		lineage.BaselineValue, lineage.CandidateValue, lineage.ImprovementValue, lineage.RecordedAt, lineage.JudgedAt)
	if err != nil {
		return fmt.Errorf("upsert lineage: %w", err)
	}
	return nil
}

func (s *TaskExecutionStore) GetLineage(ctx context.Context, experimentID string) (*domain.ExperimentLineageRecord, error) {
	row := s.pg.pool.QueryRow(ctx, `
		SELECT experiment_id, execution_id, parent_experiment_id, root_experiment_id, lineage_depth,
			target_agent_id, target_skill, mutation_type, brief_path, candidate_path, result_path,
			status, git_commit_id, params_snapshot, result_snapshot, baseline_value, candidate_value, improvement_value,
			recorded_at, judged_at
		FROM experiment_lineage WHERE experiment_id = $1
	`, experimentID)

	var rec domain.ExperimentLineageRecord
	err := row.Scan(
		&rec.ExperimentID, &rec.ExecutionID, &rec.ParentExperimentID, &rec.RootExperimentID, &rec.LineageDepth,
		&rec.TargetAgentID, &rec.TargetSkill, &rec.MutationType, &rec.BriefPath, &rec.CandidatePath, &rec.ResultPath,
		&rec.Status, &rec.GitCommitID, &rec.ParamsSnapshot, &rec.ResultSnapshot,
		&rec.BaselineValue, &rec.CandidateValue, &rec.ImprovementValue, &rec.RecordedAt, &rec.JudgedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get lineage: %w", err)
	}
	return &rec, nil
}

func (s *TaskExecutionStore) GetLineageChildren(ctx context.Context, parentExperimentID string) ([]domain.ExperimentLineageRecord, error) {
	rows, err := s.pg.pool.Query(ctx, `
		SELECT experiment_id, execution_id, parent_experiment_id, root_experiment_id, lineage_depth,
			target_agent_id, target_skill, mutation_type, brief_path, candidate_path, result_path,
			status, git_commit_id, params_snapshot, result_snapshot, baseline_value, candidate_value, improvement_value,
			recorded_at, judged_at
		FROM experiment_lineage WHERE parent_experiment_id = $1
		ORDER BY recorded_at ASC
	`, parentExperimentID)
	if err != nil {
		return nil, fmt.Errorf("list lineage children: %w", err)
	}
	defer rows.Close()

	var results []domain.ExperimentLineageRecord
	for rows.Next() {
		var rec domain.ExperimentLineageRecord
		err := rows.Scan(
			&rec.ExperimentID, &rec.ExecutionID, &rec.ParentExperimentID, &rec.RootExperimentID, &rec.LineageDepth,
			&rec.TargetAgentID, &rec.TargetSkill, &rec.MutationType, &rec.BriefPath, &rec.CandidatePath, &rec.ResultPath,
			&rec.Status, &rec.GitCommitID, &rec.ParamsSnapshot, &rec.ResultSnapshot,
			&rec.BaselineValue, &rec.CandidateValue, &rec.ImprovementValue, &rec.RecordedAt, &rec.JudgedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan lineage: %w", err)
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (s *TaskExecutionStore) InsertBaselineHistory(ctx context.Context, record domain.BaselineHistoryRecord) error {
	_, err := s.pg.pool.Exec(ctx, `
		INSERT INTO baseline_history (execution_id, experiment_id, version_before, version_after, promoted_by, promoted_at, baseline_path, diff_summary, diff_patch, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, record.ExecutionID, record.ExperimentID, record.VersionBefore, record.VersionAfter, record.PromotedBy, record.PromotedAt,
		record.BaselinePath, record.DiffSummary, record.DiffPatch, record.Metadata)
	if err != nil {
		return fmt.Errorf("insert baseline history: %w", err)
	}
	return nil
}

func (s *TaskExecutionStore) ListBaselineHistory(ctx context.Context, limit int) ([]domain.BaselineHistoryRecord, error) {
	rows, err := s.pg.pool.Query(ctx, `
		SELECT id, execution_id, experiment_id, version_before, version_after, promoted_by, promoted_at, baseline_path, diff_summary, diff_patch, metadata
		FROM baseline_history ORDER BY promoted_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list baseline history: %w", err)
	}
	defer rows.Close()

	var results []domain.BaselineHistoryRecord
	for rows.Next() {
		var rec domain.BaselineHistoryRecord
		err := rows.Scan(&rec.ID, &rec.ExecutionID, &rec.ExperimentID, &rec.VersionBefore, &rec.VersionAfter,
			&rec.PromotedBy, &rec.PromotedAt, &rec.BaselinePath, &rec.DiffSummary, &rec.DiffPatch, &rec.Metadata)
		if err != nil {
			return nil, fmt.Errorf("scan baseline history: %w", err)
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (s *TaskExecutionStore) InsertMetricPoints(ctx context.Context, points []domain.MetricTrendPoint) error {
	batch := &pgx.Batch{}
	for _, p := range points {
		batch.Queue(`
			INSERT INTO metric_trends (execution_id, experiment_id, series_key, metric_name, metric_scope, metric_value, baseline_value, delta_value, sampled_at, tags)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, p.ExecutionID, p.ExperimentID, p.SeriesKey, p.MetricName, p.MetricScope, p.MetricValue, p.BaselineValue, p.DeltaValue, p.SampledAt, p.Tags)
	}
	br := s.pg.pool.SendBatch(ctx, batch)
	return br.Close()
}

func (s *TaskExecutionStore) QueryMetricTrends(ctx context.Context, filter domain.MetricTrendFilter) ([]domain.MetricTrendPoint, error) {
	query := `
		SELECT id, execution_id, experiment_id, series_key, metric_name, metric_scope, metric_value, baseline_value, delta_value, sampled_at, tags
		FROM metric_trends WHERE 1=1
	`
	args := []any{}
	argIdx := 1

	if filter.ExperimentID != "" {
		query += fmt.Sprintf(" AND experiment_id = $%d", argIdx)
		args = append(args, filter.ExperimentID)
		argIdx++
	}
	if filter.SeriesKey != "" {
		query += fmt.Sprintf(" AND series_key = $%d", argIdx)
		args = append(args, filter.SeriesKey)
		argIdx++
	}
	if filter.MetricName != "" {
		query += fmt.Sprintf(" AND metric_name = $%d", argIdx)
		args = append(args, filter.MetricName)
		argIdx++
	}
	if !filter.Start.IsZero() {
		query += fmt.Sprintf(" AND sampled_at >= $%d", argIdx)
		args = append(args, filter.Start)
		argIdx++
	}
	if !filter.End.IsZero() {
		query += fmt.Sprintf(" AND sampled_at <= $%d", argIdx)
		args = append(args, filter.End)
		argIdx++
	}

	query += " ORDER BY sampled_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
	}

	rows, err := s.pg.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metric trends: %w", err)
	}
	defer rows.Close()

	var results []domain.MetricTrendPoint
	for rows.Next() {
		var p domain.MetricTrendPoint
		err := rows.Scan(
			&p.ID, &p.ExecutionID, &p.ExperimentID, &p.SeriesKey, &p.MetricName, &p.MetricScope,
			&p.MetricValue, &p.BaselineValue, &p.DeltaValue, &p.SampledAt, &p.Tags,
		)
		if err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
