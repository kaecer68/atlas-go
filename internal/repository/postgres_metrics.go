package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgPool abstracts the subset of *pgxpool.Pool methods used by PostgresRepository.
// It is satisfied by *pgxpool.Pool and enables test doubles for unit tests.
type pgPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PostgresRepository implements all repository interfaces using PostgreSQL/TimescaleDB.
type PostgresRepository struct {
	pool pgPool
}

// NewPostgresRepository creates a new PostgreSQL-backed repository.
// Returns nil when pool is nil so callers can use `r.pg != nil` as a single
// availability guard (DualWrite fallback logic depends on this).
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	if pool == nil {
		return nil
	}
	return &PostgresRepository{pool: pool}
}

// ============================================
// Metrics Repository Implementation
// ============================================

func (r *PostgresRepository) Record(ctx context.Context, metricName string, value float64, labels map[string]string) error {
	agentID := labels["agent_id"]
	sessionID := labels["session_id"]
	symbol := labels["symbol"]
	regime := labels["regime"]

	metadata, _ := json.Marshal(labels)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO metrics (time, metric_name, value, agent_id, session_id, symbol, regime, metadata)
		VALUES (NOW(), $1, $2, $3, $4, $5, $6, $7)
	`, metricName, value, agentID, sessionID, symbol, regime, metadata)
	if err != nil {
		return fmt.Errorf("record metric: %w", err)
	}
	return nil
}

func (r *PostgresRepository) QueryRange(ctx context.Context, metricName string, start, end time.Time) ([]MetricPoint, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, metric_name, value, agent_id, session_id, symbol, regime, metadata
		FROM metrics
		WHERE metric_name = $1 AND time >= $2 AND time <= $3
		ORDER BY time DESC
	`, metricName, start, end)
	if err != nil {
		return nil, fmt.Errorf("query metrics range: %w", err)
	}
	defer rows.Close()

	return scanMetricPoints(rows)
}

func (r *PostgresRepository) QueryLatest(ctx context.Context, metricName string, labels map[string]string) (*MetricPoint, error) {
	var query strings.Builder
	query.WriteString(`
		SELECT time, metric_name, value, agent_id, session_id, symbol, regime, metadata
		FROM metrics
		WHERE metric_name = $1
	`)
	args := []any{metricName}
	argIdx := 2

	for key, value := range labels {
		switch key {
		case "agent_id":
			fmt.Fprintf(&query, " AND agent_id = $%d", argIdx)
			args = append(args, value)
			argIdx++
		case "session_id":
			fmt.Fprintf(&query, " AND session_id = $%d", argIdx)
			args = append(args, value)
			argIdx++
		case "symbol":
			fmt.Fprintf(&query, " AND symbol = $%d", argIdx)
			args = append(args, value)
			argIdx++
		}
	}

	query.WriteString(" ORDER BY time DESC LIMIT 1")

	var point MetricPoint
	var metadata []byte
	err := r.pool.QueryRow(ctx, query.String(), args...).Scan(
		&point.Time, &point.Name, &point.Value,
		&point.AgentID, &point.SessionID, &point.Symbol, &point.Regime,
		&metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest metric: %w", err)
	}

	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &point.Metadata)
	}

	return &point, nil
}

func (r *PostgresRepository) Aggregate(ctx context.Context, metricName string, start, end time.Time, agg string) (float64, error) {
	var query string
	switch agg {
	case "avg", "AVG":
		query = "SELECT AVG(value) FROM metrics WHERE metric_name = $1 AND time >= $2 AND time <= $3"
	case "sum", "SUM":
		query = "SELECT SUM(value) FROM metrics WHERE metric_name = $1 AND time >= $2 AND time <= $3"
	case "count", "COUNT":
		query = "SELECT COUNT(*)::float8 FROM metrics WHERE metric_name = $1 AND time >= $2 AND time <= $3"
	case "max", "MAX":
		query = "SELECT MAX(value) FROM metrics WHERE metric_name = $1 AND time >= $2 AND time <= $3"
	case "min", "MIN":
		query = "SELECT MIN(value) FROM metrics WHERE metric_name = $1 AND time >= $2 AND time <= $3"
	default:
		return 0, fmt.Errorf("unsupported aggregation: %s", agg)
	}

	var result float64
	err := r.pool.QueryRow(ctx, query, metricName, start, end).Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("aggregate metrics: %w", err)
	}

	return result, nil
}

func (r *PostgresRepository) SaveSnapshot(ctx context.Context, snapshot *MetricsSnapshot) error {
	// Store each field as a separate metric point
	metrics := map[string]float64{
		"screening_total":     float64(snapshot.ScreeningTotal),
		"screening_passed":    float64(snapshot.ScreeningPassed),
		"screening_rate":      snapshot.ScreeningRate,
		"alerts_triggered":    float64(snapshot.AlertsTriggered),
		"alerts_acknowledged": float64(snapshot.AlertsAcknowledged),
	}

	for alertType, count := range snapshot.AlertsByType {
		metrics["alerts_"+alertType] = float64(count)
	}

	for name, value := range metrics {
		if err := r.Record(ctx, name, value, map[string]string{"type": "snapshot"}); err != nil {
			return fmt.Errorf("record snapshot metric %s: %w", name, err)
		}
	}

	return nil
}

func (r *PostgresRepository) LoadToday(ctx context.Context) (*MetricsSnapshot, error) {
	today := time.Now().Format("2006-01-02")
	start, _ := time.Parse("2006-01-02", today)
	end := start.Add(24 * time.Hour)

	points, err := r.QueryRange(ctx, "screening_total", start, end)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, nil
	}

	// Build snapshot from the latest data points
	snapshot := &MetricsSnapshot{Timestamp: points[0].Time}

	// Query all metrics for today (no metric_name filter) and populate snapshot
	rows, err := r.pool.Query(ctx, `
		SELECT time, metric_name, value, agent_id, session_id, symbol, regime, metadata
		FROM metrics
		WHERE time >= $1 AND time <= $2
		ORDER BY time DESC
	`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query all metrics range: %w", err)
	}
	defer rows.Close()

	metrics, err := scanMetricPoints(rows)
	if err != nil {
		return nil, err
	}

	for _, m := range metrics {
		switch m.Name {
		case "screening_total":
			snapshot.ScreeningTotal = int64(m.Value)
		case "screening_passed":
			snapshot.ScreeningPassed = int64(m.Value)
		case "screening_rate":
			snapshot.ScreeningRate = m.Value
		case "alerts_triggered":
			snapshot.AlertsTriggered = int64(m.Value)
		case "alerts_acknowledged":
			snapshot.AlertsAcknowledged = int64(m.Value)
		}
	}

	return snapshot, nil
}

func (r *PostgresRepository) LoadRecent(ctx context.Context, n int) ([]MetricsSnapshot, error) {
	// Get distinct timestamps for the last n snapshots
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT time_bucket('1 minute', time) as bucket
		FROM metrics
		WHERE metric_name LIKE 'screening_%'
		ORDER BY bucket DESC
		LIMIT $1
	`, n)
	if err != nil {
		return nil, fmt.Errorf("query recent snapshots: %w", err)
	}
	defer rows.Close()

	var buckets []time.Time
	for rows.Next() {
		var bucket time.Time
		if err := rows.Scan(&bucket); err != nil {
			continue
		}
		buckets = append(buckets, bucket)
	}

	var snapshots []MetricsSnapshot
	for _, bucket := range buckets {
		start := bucket
		end := bucket.Add(time.Minute)

		snap, err := r.loadSnapshotForRange(ctx, start, end)
		if err != nil {
			continue
		}
		if snap != nil {
			snapshots = append(snapshots, *snap)
		}
	}

	return snapshots, nil
}

func (r *PostgresRepository) loadSnapshotForRange(ctx context.Context, start, end time.Time) (*MetricsSnapshot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, metric_name, value, agent_id, session_id, symbol, regime, metadata
		FROM metrics
		WHERE time >= $1 AND time <= $2
		ORDER BY time DESC
	`, start, end)
	if err != nil {
		return nil, fmt.Errorf("query all metrics for range: %w", err)
	}
	defer rows.Close()

	points, err := scanMetricPoints(rows)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, nil
	}

	snapshot := &MetricsSnapshot{Timestamp: points[0].Time}
	for _, p := range points {
		switch p.Name {
		case "screening_total":
			snapshot.ScreeningTotal = int64(p.Value)
		case "screening_passed":
			snapshot.ScreeningPassed = int64(p.Value)
		case "screening_rate":
			snapshot.ScreeningRate = p.Value
		case "alerts_triggered":
			snapshot.AlertsTriggered = int64(p.Value)
		case "alerts_acknowledged":
			snapshot.AlertsAcknowledged = int64(p.Value)
		}
	}

	return snapshot, nil
}

func scanMetricPoints(rows pgx.Rows) ([]MetricPoint, error) {
	var points []MetricPoint
	for rows.Next() {
		var point MetricPoint
		var metadata []byte
		err := rows.Scan(
			&point.Time, &point.Name, &point.Value,
			&point.AgentID, &point.SessionID, &point.Symbol, &point.Regime,
			&metadata,
		)
		if err != nil {
			continue
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &point.Metadata)
		}
		points = append(points, point)
	}
	return points, rows.Err()
}
