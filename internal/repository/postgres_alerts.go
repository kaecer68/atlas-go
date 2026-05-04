package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// ============================================
// Alert Repository Implementation
// ============================================

func (r *PostgresRepository) SaveAlert(ctx context.Context, alert domain.AlertRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO alerts (id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			acknowledged = EXCLUDED.acknowledged,
			acknowledged_at = EXCLUDED.acknowledged_at,
			acknowledged_by = EXCLUDED.acknowledged_by
	`, alert.ID, alert.Timestamp, alert.Rule, alert.Severity, alert.Message,
		alert.Value, alert.Threshold, alert.Acknowledged, alert.AcknowledgedAt, alert.AcknowledgedBy)

	return err
}

func (r *PostgresRepository) LoadAllAlerts(ctx context.Context, limit int) ([]domain.AlertRecord, error) {
	query := `
		SELECT id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by
		FROM alerts
		ORDER BY timestamp DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("load all alerts: %w", err)
	}
	defer rows.Close()

	return scanAlertRecords(rows)
}

func (r *PostgresRepository) LoadUnacknowledgedAlerts(ctx context.Context) ([]domain.AlertRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by
		FROM alerts
		WHERE NOT acknowledged
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("load unacknowledged alerts: %w", err)
	}
	defer rows.Close()

	return scanAlertRecords(rows)
}

func (r *PostgresRepository) AcknowledgeAlert(ctx context.Context, alertID string, user string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE alerts
		SET acknowledged = TRUE, acknowledged_at = NOW(), acknowledged_by = $2
		WHERE id = $1 AND NOT acknowledged
	`, alertID, user)

	if err != nil {
		return fmt.Errorf("acknowledge alert: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert %q not found or already acknowledged", alertID)
	}

	return nil
}

func (r *PostgresRepository) LoadAlertsBySeverity(ctx context.Context, severity string, limit int) ([]domain.AlertRecord, error) {
	query := `
		SELECT id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by
		FROM alerts
		WHERE severity = $1
		ORDER BY timestamp DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.pool.Query(ctx, query, severity)
	if err != nil {
		return nil, fmt.Errorf("load alerts by severity: %w", err)
	}
	defer rows.Close()

	return scanAlertRecords(rows)
}

func (r *PostgresRepository) LoadAlertsByTimeRange(ctx context.Context, start, end time.Time) ([]domain.AlertRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by
		FROM alerts
		WHERE timestamp >= $1 AND timestamp <= $2
		ORDER BY timestamp DESC
	`, start, end)
	if err != nil {
		return nil, fmt.Errorf("load alerts by time range: %w", err)
	}
	defer rows.Close()

	return scanAlertRecords(rows)
}

func scanAlertRecords(rows pgx.Rows) ([]domain.AlertRecord, error) {
	var records []domain.AlertRecord
	for rows.Next() {
		var rec domain.AlertRecord
		var ackAt *time.Time
		err := rows.Scan(
			&rec.ID, &rec.Timestamp, &rec.Rule, &rec.Severity, &rec.Message,
			&rec.Value, &rec.Threshold, &rec.Acknowledged, &ackAt, &rec.AcknowledgedBy,
		)
		if err != nil {
			continue
		}
		rec.AcknowledgedAt = ackAt
		records = append(records, rec)
	}
	return records, rows.Err()
}
