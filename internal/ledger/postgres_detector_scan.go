// Package ledger — PostgresDetectorScanStore
//
// PostgreSQL mirror of SQLiteDetectorScanStore (detector_scan_store.go).
// Detector scan results live in the multi-process-friendly PostgreSQL
// database so the MCP `template_detector_status` tool can query scan
// history with efficient LIMIT + ORDER BY across all containers
// (000013 migration).
//
// detected_at is stored as TEXT (RFC3339) matching the SQLite format.
package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

// PostgresDetectorScanStore is the PostgreSQL-backed DetectorScanStore.
type PostgresDetectorScanStore struct {
	pool *pgxpool.Pool
}

// NewPostgresDetectorScanStore binds the store to an already-opened pgxpool.
func NewPostgresDetectorScanStore(pool *pgxpool.Pool) *PostgresDetectorScanStore {
	return &PostgresDetectorScanStore{pool: pool}
}

// Compile-time assertion: PostgresDetectorScanStore implements DetectorScanStore.
var _ DetectorScanStore = (*PostgresDetectorScanStore)(nil)

// AppendScan inserts all results as one transactional batch sharing
// the same scan_batch_id (UUID). Empty results is a no-op returning ("", nil).
func (s *PostgresDetectorScanStore) AppendScan(ctx context.Context, results []narrative.DetectionResult) (string, error) {
	if len(results) == 0 {
		return "", nil
	}

	batchID := uuid.NewString()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("detector_scan: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, r := range results {
		if r.Theme == "" {
			return "", fmt.Errorf("detector_scan: result has empty theme (severity=%s)", r.Severity)
		}

		var metaJSON any
		if len(r.Metadata) > 0 {
			b, mErr := json.Marshal(r.Metadata)
			if mErr != nil {
				return "", fmt.Errorf("detector_scan: marshal metadata theme=%s: %w", r.Theme, mErr)
			}
			metaJSON = string(b)
		}

		if _, execErr := tx.Exec(ctx, `
			INSERT INTO detector_scan_log
				(scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, batchID, r.Theme, string(r.Severity), r.Confidence,
			r.DetectedAt.UTC().Format(time.RFC3339), string(r.Source), metaJSON,
		); execErr != nil {
			return "", fmt.Errorf("detector_scan: insert theme=%s: %w", r.Theme, execErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("detector_scan: commit: %w", err)
	}
	return batchID, nil
}

// LoadRecentScans returns up to limit most recent scan results, newest
// first (ORDER BY scan_id DESC). limit <= 0 falls back to 100.
func (s *PostgresDetectorScanStore) LoadRecentScans(ctx context.Context, limit int) ([]ScanResultRow, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT scan_id, scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json
		FROM detector_scan_log
		ORDER BY scan_id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("detector_scan: query: %w", err)
	}
	defer rows.Close()

	out := make([]ScanResultRow, 0, limit)
	for rows.Next() {
		var (
			row          ScanResultRow
			severity     string
			source       string
			detectedAt   string
			metaJSONNull *string
		)
		if scanErr := rows.Scan(
			&row.ScanID, &row.ScanBatchID, &row.Theme, &severity,
			&row.Confidence, &detectedAt, &source, &metaJSONNull,
		); scanErr != nil {
			return nil, fmt.Errorf("detector_scan: scan row: %w", scanErr)
		}
		row.Severity = narrative.Severity(severity)
		row.Source = narrative.Source(source)

		parsed, perr := time.Parse(time.RFC3339, detectedAt)
		if perr != nil {
			return nil, fmt.Errorf("detector_scan: parse detected_at %q (scan_id=%d): %w", detectedAt, row.ScanID, perr)
		}
		row.DetectedAt = parsed

		if metaJSONNull != nil && *metaJSONNull != "" {
			var meta map[string]any
			if jerr := json.Unmarshal([]byte(*metaJSONNull), &meta); jerr != nil {
				return nil, fmt.Errorf("detector_scan: unmarshal metadata scan_id=%d: %w", row.ScanID, jerr)
			}
			row.Metadata = meta
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("detector_scan: rows iteration: %w", rows.Err())
	}
	return out, nil
}
