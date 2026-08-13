-- ============================================
-- 000011: historical tables (regime/stress/period/geopolitical/event_calendar/prediction_backtest)
-- PostgreSQL mirror of SQLiteHistoricalStore tables (Stage 4 PR#2 contract).
-- Rationale: multi-container writers share atlas.db (SQLite WAL); the
-- cross-container overlayfs WAL coordination intermittently fails with
-- SQLITE_IOERR(522). Postgres is the existing multi-process-friendly DB.
-- ============================================

-- Regime history: one row per date.
CREATE TABLE IF NOT EXISTS regime_history (
    date TEXT PRIMARY KEY,
    regime TEXT NOT NULL,
    source_session_id TEXT,
    recorded_at TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT 'synthetic'
);

-- Stress index history: one row per date.
CREATE TABLE IF NOT EXISTS stress_index_history (
    date TEXT PRIMARY KEY,
    score DOUBLE PRECISION NOT NULL,
    regime TEXT,
    components_json TEXT,
    source TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL
);

-- Period history: seven-period market cycle classification, one row per date.
CREATE TABLE IF NOT EXISTS period_history (
    date TEXT PRIMARY KEY,
    period TEXT NOT NULL,
    recorded_at TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT 'macro_ingest'
);

-- Geopolitical history: one row per date.
CREATE TABLE IF NOT EXISTS geopolitical_history (
    date TEXT PRIMARY KEY,
    intensity DOUBLE PRECISION NOT NULL,
    sources_json TEXT,
    source TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL
);

-- Event calendar history: (date, event_id) composite key.
CREATE TABLE IF NOT EXISTS event_calendar_history (
    date TEXT NOT NULL,
    event_id TEXT NOT NULL,
    active_theme TEXT,
    source TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL,
    PRIMARY KEY (date, event_id)
);

-- Prediction backtest: one row per date.
CREATE TABLE IF NOT EXISTS prediction_backtest (
    date TEXT PRIMARY KEY,
    predicted_direction TEXT,
    predicted_confidence DOUBLE PRECISION,
    actual_direction TEXT,
    actual_capital_flow_change DOUBLE PRECISION,
    hit INTEGER,
    model_version TEXT,
    captured_at TEXT NOT NULL,
    is_synthetic INTEGER NOT NULL
);

-- Indexes mirroring SQLite (idx_regime_history_captured_at etc.).
CREATE INDEX IF NOT EXISTS idx_regime_history_captured_at ON regime_history(captured_at);
CREATE INDEX IF NOT EXISTS idx_stress_history_captured_at ON stress_index_history(captured_at);
CREATE INDEX IF NOT EXISTS idx_period_history_captured_at ON period_history(captured_at);
CREATE INDEX IF NOT EXISTS idx_geopolitical_history_captured_at ON geopolitical_history(captured_at);
CREATE INDEX IF NOT EXISTS idx_event_calendar_history_captured_at ON event_calendar_history(captured_at);
CREATE INDEX IF NOT EXISTS idx_prediction_backtest_captured_at ON prediction_backtest(captured_at);
