// Package ledger — Stage 5 PR#4 detector_scan_log store tests.
package ledger

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// mustOpenSQLiteDBForScan opens a temp-dir SQLite DB and runs InitSchema.
func mustOpenSQLiteDBForScan(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "scan_test.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sampleResult(theme string, severity narrative.Severity, source narrative.Source, confidence float64, meta map[string]any) narrative.DetectionResult {
	return narrative.DetectionResult{
		Theme:      theme,
		Severity:   severity,
		Confidence: confidence,
		DetectedAt: time.Now().UTC(),
		Source:     source,
		Metadata:   meta,
	}
}

func TestSQLiteDetectorScanStore_AppendAndLoad(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	results := []narrative.DetectionResult{
		sampleResult("US_rates_up", narrative.SeverityHigh, narrative.SourceKB, 0.85, map[string]any{"us10y_bps": 25.0}),
		sampleResult("tariff_shock", narrative.SeverityCritical, narrative.SourceIngestor, 0.95, nil),
	}

	batchID, err := store.AppendScan(ctx, results)
	if err != nil {
		t.Fatalf("AppendScan: %v", err)
	}
	if batchID == "" {
		t.Fatal("AppendScan returned empty batchID for non-empty results")
	}

	rows, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("LoadRecentScans returned %d rows, want 2", len(rows))
	}

	// Newest first (highest scan_id is the last-inserted tariff_shock)
	if rows[0].Theme != "tariff_shock" {
		t.Errorf("rows[0].Theme = %q, want tariff_shock (newest first)", rows[0].Theme)
	}
	if rows[0].Severity != narrative.SeverityCritical {
		t.Errorf("rows[0].Severity = %q, want critical", rows[0].Severity)
	}
	if rows[0].Confidence != 0.95 {
		t.Errorf("rows[0].Confidence = %f, want 0.95", rows[0].Confidence)
	}
	if rows[0].Source != narrative.SourceIngestor {
		t.Errorf("rows[0].Source = %q, want snapshot_ingestor", rows[0].Source)
	}
	if rows[0].Metadata != nil {
		t.Errorf("rows[0].Metadata = %+v, want nil (nil input → NULL)", rows[0].Metadata)
	}

	if rows[1].Theme != "US_rates_up" {
		t.Errorf("rows[1].Theme = %q, want US_rates_up", rows[1].Theme)
	}
	if v, ok := rows[1].Metadata["us10y_bps"]; !ok || v != 25.0 {
		t.Errorf("rows[1].Metadata[us10y_bps] = %v, want 25.0", v)
	}

	// All rows in one batch share scan_batch_id.
	if rows[0].ScanBatchID != batchID {
		t.Errorf("rows[0].ScanBatchID = %q, want %q", rows[0].ScanBatchID, batchID)
	}
	if rows[0].ScanBatchID != rows[1].ScanBatchID {
		t.Errorf("batch_id mismatch: %q vs %q", rows[0].ScanBatchID, rows[1].ScanBatchID)
	}
}

func TestSQLiteDetectorScanStore_AppendEmpty_NoOp(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	batchID, err := store.AppendScan(ctx, nil)
	if err != nil {
		t.Fatalf("AppendScan(nil): %v", err)
	}
	if batchID != "" {
		t.Errorf("AppendScan(nil) batchID = %q, want empty string", batchID)
	}

	rows, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows after empty AppendScan, got %d", len(rows))
	}
}

func TestSQLiteDetectorScanStore_MultipleAppends(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	if _, err := store.AppendScan(ctx, []narrative.DetectionResult{
		sampleResult("A", narrative.SeverityLow, narrative.SourceKB, 0.5, nil),
	}); err != nil {
		t.Fatalf("first AppendScan: %v", err)
	}
	if _, err := store.AppendScan(ctx, []narrative.DetectionResult{
		sampleResult("B", narrative.SeverityHigh, narrative.SourceKB, 0.8, nil),
	}); err != nil {
		t.Fatalf("second AppendScan: %v", err)
	}

	rows, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Theme != "B" {
		t.Errorf("rows[0].Theme = %q, want B (newest first)", rows[0].Theme)
	}
	if rows[0].ScanBatchID == rows[1].ScanBatchID {
		t.Error("expected different batch IDs across AppendScan calls")
	}
}

func TestSQLiteDetectorScanStore_LimitRespected(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	var batch []narrative.DetectionResult
	for i := range 5 {
		batch = append(batch, sampleResult("test", narrative.SeverityMedium, narrative.SourceKB, float64(i)/10, nil))
	}
	if _, err := store.AppendScan(ctx, batch); err != nil {
		t.Fatalf("AppendScan: %v", err)
	}

	rows, err := store.LoadRecentScans(ctx, 3)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows (limit=3), got %d", len(rows))
	}
}

func TestSQLiteDetectorScanStore_DefaultLimitOnZero(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	var batch []narrative.DetectionResult
	for range 3 {
		batch = append(batch, sampleResult("t", narrative.SeverityLow, narrative.SourceKB, 0.5, nil))
	}
	if _, err := store.AppendScan(ctx, batch); err != nil {
		t.Fatalf("AppendScan: %v", err)
	}

	rows, err := store.LoadRecentScans(ctx, 0)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows (default limit), got %d", len(rows))
	}
}

func TestSQLiteDetectorScanStore_NewestFirst(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	t1 := time.Now().Add(-2 * time.Hour).UTC()
	t2 := time.Now().Add(-1 * time.Hour).UTC()
	t3 := time.Now().UTC()

	// Insert in chronological order
	for _, ts := range []time.Time{t1, t2, t3} {
		var theme string
		switch ts {
		case t1:
			theme = "oldest"
		case t2:
			theme = "middle"
		default:
			theme = "newest"
		}
		if _, err := store.AppendScan(ctx, []narrative.DetectionResult{{
			Theme: theme, Severity: narrative.SeverityLow, Source: narrative.SourceKB, Confidence: 0.1, DetectedAt: ts,
		}}); err != nil {
			t.Fatalf("AppendScan: %v", err)
		}
	}

	rows, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Theme != "newest" {
		t.Errorf("rows[0].Theme = %q, want newest", rows[0].Theme)
	}
	if rows[2].Theme != "oldest" {
		t.Errorf("rows[2].Theme = %q, want oldest", rows[2].Theme)
	}
}

func TestSQLiteDetectorScanStore_EmptyThemeRejected(t *testing.T) {
	store := NewSQLiteDetectorScanStore(mustOpenSQLiteDBForScan(t))
	ctx := context.Background()

	_, err := store.AppendScan(ctx, []narrative.DetectionResult{
		sampleResult("", narrative.SeverityLow, narrative.SourceKB, 0.5, nil),
	})
	if err == nil {
		t.Error("AppendScan accepted result with empty theme; want error")
	}
}

func TestNewDetectorScanStore_SQLiteBackend(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		StoreBackend: "sqlite",
		LedgerDir:    dir,
		SQLitePath:   filepath.Join(dir, "factory_test.db"),
	}
	store, err := NewDetectorScanStore(cfg)
	if err != nil {
		t.Fatalf("NewDetectorScanStore: %v", err)
	}
	if store == nil {
		t.Fatal("NewDetectorScanStore returned nil")
	}

	ctx := context.Background()
	batchID, err := store.AppendScan(ctx, []narrative.DetectionResult{
		sampleResult("factory_test", narrative.SeverityLow, narrative.SourceKB, 0.1, nil),
	})
	if err != nil {
		t.Fatalf("AppendScan via factory: %v", err)
	}
	if batchID == "" {
		t.Error("expected non-empty batchID")
	}
	rows, err := store.LoadRecentScans(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecentScans via factory: %v", err)
	}
	if len(rows) != 1 || rows[0].Theme != "factory_test" {
		t.Errorf("expected 1 row with theme factory_test, got %d rows", len(rows))
	}
}

func TestNewDetectorScanStore_NonSQLiteBackend_Rejected(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		StoreBackend: "jsonl",
		LedgerDir:    dir,
		SQLitePath:   filepath.Join(dir, "should_not_be_used.db"),
	}
	store, err := NewDetectorScanStore(cfg)
	if err == nil {
		t.Fatal("NewDetectorScanStore with jsonl backend should return error")
	}
	if store != nil {
		t.Errorf("expected nil store on error, got %T", store)
	}
}
