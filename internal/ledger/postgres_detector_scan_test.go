package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

// cleanupDetectorScanLog removes all test rows (scan_batch_id prefix
// "pgsqltest-") so tests stay isolated from migrated production data.
func cleanupDetectorScanLog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM detector_scan_log WHERE scan_batch_id LIKE 'pgsqltest-%'")
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM detector_scan_log WHERE scan_batch_id LIKE 'pgsqltest-%'")
	})
}

func TestPostgresDetectorScanStore_AppendAndLoad(t *testing.T) {
	pool := connectTestPG(t)
	cleanupDetectorScanLog(t, pool)
	store := NewPostgresDetectorScanStore(pool)
	ctx := context.Background()

	// First batch: 2 results sharing one batch ID.
	batch1 := []narrative.DetectionResult{
		{
			Theme: "pgsqltest-theme-a", Severity: narrative.SeverityHigh, Confidence: 0.9,
			DetectedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Source: narrative.SourceKB, Metadata: map[string]any{"k": "v"},
		},
		{
			Theme: "pgsqltest-theme-b", Severity: narrative.SeverityMedium, Confidence: 0.5,
			DetectedAt: time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC), Source: narrative.SourceKB,
		},
	}
	// Force the batch ID into the test namespace so cleanup works.
	// (AppendScan generates its own UUID; we overwrite via UPDATE afterwards.)
	batchID1, err := store.AppendScan(ctx, batch1)
	if err != nil {
		t.Fatalf("AppendScan batch1: %v", err)
	}
	if batchID1 == "" {
		t.Fatalf("expected non-empty batch ID")
	}
	if _, err := pool.Exec(ctx, "UPDATE detector_scan_log SET scan_batch_id = 'pgsqltest-' || $1 WHERE scan_batch_id = $1", batchID1); err != nil {
		t.Fatalf("namespace batch1: %v", err)
	}

	// Second batch: 1 result, later scan_id (newer).
	batch2 := []narrative.DetectionResult{
		{
			Theme: "pgsqltest-theme-c", Severity: narrative.SeverityCritical, Confidence: 0.99,
			DetectedAt: time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC), Source: narrative.SourceKB,
		},
	}
	batchID2, err := store.AppendScan(ctx, batch2)
	if err != nil {
		t.Fatalf("AppendScan batch2: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE detector_scan_log SET scan_batch_id = 'pgsqltest-' || $1 WHERE scan_batch_id = $1", batchID2); err != nil {
		t.Fatalf("namespace batch2: %v", err)
	}

	// LoadRecentScans returns newest first, limit respected.
	scans, err := store.LoadRecentScans(ctx, 2)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(scans) != 2 {
		t.Fatalf("expected 2 scans (limit), got %d", len(scans))
	}
	// Newest first: theme-c from batch2 must be first.
	if scans[0].Theme != "pgsqltest-theme-c" {
		t.Fatalf("expected newest first, got %+v", scans[0])
	}
	if scans[0].Severity != narrative.SeverityCritical || scans[0].Confidence != 0.99 {
		t.Fatalf("scan fields mismatch: %+v", scans[0])
	}

	// Metadata round-trip from batch1's row (present in the limit-2 window? no —
	// query all to check metadata unmarshal).
	all, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans(all): %v", err)
	}
	foundMeta := false
	for _, s := range all {
		if s.Theme == "pgsqltest-theme-a" {
			foundMeta = true
			if s.Metadata["k"] != "v" {
				t.Fatalf("metadata mismatch: %+v", s.Metadata)
			}
		}
	}
	if !foundMeta {
		t.Fatalf("theme-a row not found in all scans")
	}
}

func TestPostgresDetectorScanStore_EmptyNoOp(t *testing.T) {
	pool := connectTestPG(t)
	cleanupDetectorScanLog(t, pool)
	store := NewPostgresDetectorScanStore(pool)
	ctx := context.Background()

	batchID, err := store.AppendScan(ctx, nil)
	if err != nil {
		t.Fatalf("AppendScan(nil) should no-op: %v", err)
	}
	if batchID != "" {
		t.Fatalf("expected empty batch ID for no-op, got %q", batchID)
	}
	batchID, err = store.AppendScan(ctx, []narrative.DetectionResult{})
	if err != nil {
		t.Fatalf("AppendScan(empty) should no-op: %v", err)
	}
	if batchID != "" {
		t.Fatalf("expected empty batch ID for no-op, got %q", batchID)
	}

	scans, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(scans) != 0 {
		t.Fatalf("expected 0 scans after no-op, got %d", len(scans))
	}
}
