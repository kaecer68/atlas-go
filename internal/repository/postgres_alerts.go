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

const alertColumns = `id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by, status, dedup_key, count, first_seen, last_seen, resolved_at, resolved_by, silenced_until`

func (r *PostgresRepository) SaveAlert(ctx context.Context, alert domain.AlertRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO alerts (`+alertColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (id) DO UPDATE SET
			acknowledged = EXCLUDED.acknowledged,
			acknowledged_at = EXCLUDED.acknowledged_at,
			acknowledged_by = EXCLUDED.acknowledged_by,
			status = EXCLUDED.status,
			count = EXCLUDED.count,
			last_seen = EXCLUDED.last_seen,
			resolved_at = EXCLUDED.resolved_at,
			resolved_by = EXCLUDED.resolved_by,
			silenced_until = EXCLUDED.silenced_until
	`, alert.ID, alert.Timestamp, alert.Rule, alert.Severity, alert.Message,
		alert.Value, alert.Threshold, alert.Acknowledged, alert.AcknowledgedAt, alert.AcknowledgedBy,
		alert.Status, alert.DedupKey, alert.Count, alert.FirstSeen, alert.LastSeen,
		alert.ResolvedAt, alert.ResolvedBy, alert.SilencedUntil)
	if err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	return nil
}

func (r *PostgresRepository) LoadAllAlerts(ctx context.Context, limit int) ([]domain.AlertRecord, error) {
	query := `
		SELECT ` + alertColumns + `
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
		SELECT `+alertColumns+`
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

func (r *PostgresRepository) ResolveAlert(ctx context.Context, alertID string, user string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE alerts
		SET status = 'resolved', resolved_at = NOW(), resolved_by = $2
		WHERE id = $1
	`, alertID, user)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert %q not found", alertID)
	}

	return nil
}

func (r *PostgresRepository) LoadAlertsBySeverity(ctx context.Context, severity string, limit int) ([]domain.AlertRecord, error) {
	query := `
		SELECT ` + alertColumns + `
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
		SELECT `+alertColumns+`
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

func (r *PostgresRepository) FindAlertByDedupKey(ctx context.Context, dedupKey string) (*domain.AlertRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+alertColumns+`
		FROM alerts
		WHERE dedup_key = $1
		LIMIT 1
	`, dedupKey)

	var rec domain.AlertRecord
	err := scanAlertRecord(row, &rec)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find alert by dedup key: %w", err)
	}
	return &rec, nil
}

func (r *PostgresRepository) UpdateAlertByID(ctx context.Context, id string, fn func(*domain.AlertRecord)) error {
	row := r.pool.QueryRow(ctx, `
		SELECT `+alertColumns+`
		FROM alerts
		WHERE id = $1
	`, id)

	var rec domain.AlertRecord
	if err := scanAlertRecord(row, &rec); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("alert %q not found", id)
		}
		return fmt.Errorf("update alert load: %w", err)
	}

	fn(&rec)

	_, err := r.pool.Exec(ctx, `
		UPDATE alerts
		SET timestamp = $2, rule = $3, severity = $4, message = $5, value = $6, threshold = $7,
		    acknowledged = $8, acknowledged_at = $9, acknowledged_by = $10,
		    status = $11, dedup_key = $12, count = $13, first_seen = $14, last_seen = $15,
		    resolved_at = $16, resolved_by = $17, silenced_until = $18
		WHERE id = $1
	`, rec.ID, rec.Timestamp, rec.Rule, rec.Severity, rec.Message,
		rec.Value, rec.Threshold, rec.Acknowledged, rec.AcknowledgedAt, rec.AcknowledgedBy,
		rec.Status, rec.DedupKey, rec.Count, rec.FirstSeen, rec.LastSeen,
		rec.ResolvedAt, rec.ResolvedBy, rec.SilencedUntil)
	if err != nil {
		return fmt.Errorf("update alert save: %w", err)
	}
	return nil
}

func scanAlertRecords(rows pgx.Rows) ([]domain.AlertRecord, error) {
	var records []domain.AlertRecord
	for rows.Next() {
		var rec domain.AlertRecord
		if err := scanAlertRecordFromRows(rows, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func scanAlertRecord(row pgx.Row, rec *domain.AlertRecord) error {
	var ackAt *time.Time
	var firstSeen *time.Time
	var lastSeen *time.Time
	var resolvedAt *time.Time
	var silencedUntil *time.Time

	err := row.Scan(
		&rec.ID, &rec.Timestamp, &rec.Rule, &rec.Severity, &rec.Message,
		&rec.Value, &rec.Threshold, &rec.Acknowledged, &ackAt, &rec.AcknowledgedBy,
		&rec.Status, &rec.DedupKey, &rec.Count, &firstSeen, &lastSeen,
		&resolvedAt, &rec.ResolvedBy, &silencedUntil,
	)
	if err != nil {
		return err
	}
	rec.AcknowledgedAt = ackAt
	rec.FirstSeen = firstSeen
	rec.LastSeen = lastSeen
	rec.ResolvedAt = resolvedAt
	rec.SilencedUntil = silencedUntil
	return nil
}

func scanAlertRecordFromRows(rows pgx.Rows, rec *domain.AlertRecord) error {
	var ackAt *time.Time
	var firstSeen *time.Time
	var lastSeen *time.Time
	var resolvedAt *time.Time
	var silencedUntil *time.Time

	err := rows.Scan(
		&rec.ID, &rec.Timestamp, &rec.Rule, &rec.Severity, &rec.Message,
		&rec.Value, &rec.Threshold, &rec.Acknowledged, &ackAt, &rec.AcknowledgedBy,
		&rec.Status, &rec.DedupKey, &rec.Count, &firstSeen, &lastSeen,
		&resolvedAt, &rec.ResolvedBy, &silencedUntil,
	)
	if err != nil {
		return err
	}
	rec.AcknowledgedAt = ackAt
	rec.FirstSeen = firstSeen
	rec.LastSeen = lastSeen
	rec.ResolvedAt = resolvedAt
	rec.SilencedUntil = silencedUntil
	return nil
}
