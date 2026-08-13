-- ============================================
-- 000013: detector_scan_log table (PostgreSQL mirror of SQLite detector_scan_log).
-- PostgreSQL mirror of SQLiteDetectorScanStore (sqlite_core.go detector_scan_log
-- DDL + detector_scan_store.go contract). scan_id is BIGSERIAL preserving
-- ORDER BY scan_id DESC semantics of LoadRecentScans; detected_at is TEXT
-- (RFC3339) matching the SQLite string format.
-- ============================================

CREATE TABLE IF NOT EXISTS detector_scan_log (
    scan_id BIGSERIAL PRIMARY KEY,
    scan_batch_id TEXT NOT NULL,
    theme TEXT NOT NULL,
    severity TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    detected_at TEXT NOT NULL,
    source TEXT NOT NULL,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_detector_scan_log_batch ON detector_scan_log (scan_batch_id);
CREATE INDEX IF NOT EXISTS idx_detector_scan_log_theme_time ON detector_scan_log (theme, detected_at);
