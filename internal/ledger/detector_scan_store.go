// Package ledger — Stage 5 PR#4 detector_scan_log SQLite store
//
// Persists narrative.DetectionResult rows produced by the
// template_detector_scan BackgroundTask (see internal/scheduler/
// template_detector_scan.go) into the shared atlas.db SQLite database.
//
// Contract (pre-committed in ../../docs/archive/2026-07-14-atlas-stage5-detector-plan.md §PR#4):
//   - File:    internal/ledger/detector_scan_store.go
//   - Type:    DetectorScanStore (interface)
//   - Table:   detector_scan_log (schema in internal/ledger/sqlite_core.go)
//   - Columns: scan_id, scan_batch_id, theme, severity, confidence,
//     detected_at, source, metadata_json
//   - Method:  AppendScan(ctx, results []narrative.DetectionResult) (string, error)
//   - Method:  LoadRecentScans(ctx, limit int) ([]ScanResultRow, error)
//   - SQLite-only (no JSONL fallback; matches "每 1h 寫到 SQLite" requirement)
//
// scan_batch_id groups all rows from one RunAll() invocation (UUID), letting
// MCP / HTTP clients reconstruct a single scan's results and correlate with
// scheduler metrics.
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

// ScanResultRow is one row from detector_scan_log, serialized to JSON for
// HTTP / MCP API output. All narrative.* types are aliased (not redefined)
// so consumers can compare against the canonical types directly.
type ScanResultRow struct {
	ScanID      int64              `json:"scan_id"`
	ScanBatchID string             `json:"scan_batch_id"`
	Theme       string             `json:"theme"`
	Severity    narrative.Severity `json:"severity"`
	Confidence  float64            `json:"confidence"`
	DetectedAt  time.Time          `json:"detected_at"`
	Source      narrative.Source   `json:"source"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
}

// DetectorScanStore persists detector scan results for later query.
//
// Implementations:
//   - *SQLiteDetectorScanStore (production; uses shared atlas.db)
//   - mockDetectorScanStore (tests in scheduler/template_detector_scan_test.go)
//
// AppendScan returns the scan_batch_id (UUID) assigned to this batch; an
// empty results slice returns ("", nil) without inserting any rows.
type DetectorScanStore interface {
	AppendScan(ctx context.Context, results []narrative.DetectionResult) (batchID string, err error)
	LoadRecentScans(ctx context.Context, limit int) ([]ScanResultRow, error)
}

// SQLiteDetectorScanStore is the production SQLite-backed implementation.
// It uses the shared *sql.DB opened by getSharedSQLiteDB() so multiple
// stores share one connection pool and one file handle.
type SQLiteDetectorScanStore struct {
	db *sql.DB
}

// NewSQLiteDetectorScanStore wraps an open *sql.DB. The caller is
// responsible for ensuring InitSchema(db) has been called (done in
// cmd/atlas startup before NewDetectorScanStore is invoked).
func NewSQLiteDetectorScanStore(db *sql.DB) *SQLiteDetectorScanStore {
	return &SQLiteDetectorScanStore{db: db}
}

// AppendScan inserts all results as one transactional batch sharing
// the same scan_batch_id. Empty results is a no-op returning ("", nil).
//
// Per-row metadata is JSON-marshaled; nil/empty metadata maps to NULL.
func (s *SQLiteDetectorScanStore) AppendScan(ctx context.Context, results []narrative.DetectionResult) (string, error) {
	if len(results) == 0 {
		return "", nil
	}

	batchID := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("detector_scan: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO detector_scan_log
			(scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return "", fmt.Errorf("detector_scan: prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, r := range results {
		if r.Theme == "" {
			err = fmt.Errorf("detector_scan: result has empty theme (severity=%s)", r.Severity)
			return "", err
		}

		var metaJSON sql.NullString
		if len(r.Metadata) > 0 {
			b, mErr := json.Marshal(r.Metadata)
			if mErr != nil {
				err = fmt.Errorf("detector_scan: marshal metadata theme=%s: %w", r.Theme, mErr)
				return "", err
			}
			metaJSON = sql.NullString{String: string(b), Valid: true}
		}

		if _, execErr := stmt.ExecContext(
			ctx,
			batchID,
			r.Theme,
			string(r.Severity),
			r.Confidence,
			r.DetectedAt.UTC().Format(time.RFC3339),
			string(r.Source),
			metaJSON,
		); execErr != nil {
			err = fmt.Errorf("detector_scan: insert theme=%s: %w", r.Theme, execErr)
			return "", err
		}
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("detector_scan: commit: %w", err)
	}
	return batchID, nil
}

// LoadRecentScans returns up to limit most recent scan results, newest
// first (ORDER BY scan_id DESC). limit <= 0 falls back to 100.
//
// Results span multiple batches; clients that need per-batch grouping
// should group by ScanBatchID post-query.
func (s *SQLiteDetectorScanStore) LoadRecentScans(ctx context.Context, limit int) ([]ScanResultRow, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT scan_id, scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json
		FROM detector_scan_log
		ORDER BY scan_id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("detector_scan: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ScanResultRow, 0, limit)
	for rows.Next() {
		var (
			row          ScanResultRow
			severity     string
			source       string
			detectedAt   string
			metaJSONNull sql.NullString
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

		if metaJSONNull.Valid && metaJSONNull.String != "" {
			var meta map[string]any
			if jerr := json.Unmarshal([]byte(metaJSONNull.String), &meta); jerr != nil {
				return nil, fmt.Errorf("detector_scan: unmarshal metadata scan_id=%d: %w", row.ScanID, jerr)
			}
			row.Metadata = meta
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("detector_scan: rows iteration: %w", err)
	}
	return out, nil
}
