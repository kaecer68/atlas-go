-- ============================================
-- 000015: experiment/backtest/spawn tables (PostgreSQL mirrors of SQLite).
-- PostgreSQL mirrors of SQLiteStore tables (sqlite_core.go DDL):
-- experiments, prompt_experiment_results, window_summaries,
-- mutation_briefs, spawn_records. JSON blobs stored as TEXT; created_at
-- TEXT (RFC3339) matching the SQLite format.
-- ============================================

CREATE TABLE IF NOT EXISTS experiments (
    id BIGSERIAL PRIMARY KEY,
    experiment_id TEXT UNIQUE NOT NULL,
    session_id TEXT,
    mutation_brief_json TEXT,
    result_json TEXT,
    accepted INTEGER,
    timestamp TEXT
);
CREATE INDEX IF NOT EXISTS idx_experiments_session_id ON experiments (session_id);

CREATE TABLE IF NOT EXISTS prompt_experiment_results (
    id BIGSERIAL PRIMARY KEY,
    experiment_id TEXT UNIQUE NOT NULL,
    data_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_prompt_experiment_results_experiment_id ON prompt_experiment_results (experiment_id);

CREATE TABLE IF NOT EXISTS window_summaries (
    id BIGSERIAL PRIMARY KEY,
    window_id TEXT UNIQUE NOT NULL,
    data_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_window_summaries_window_id ON window_summaries (window_id);

CREATE TABLE IF NOT EXISTS mutation_briefs (
    id BIGSERIAL PRIMARY KEY,
    window_id TEXT NOT NULL,
    data_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mutation_briefs_window_id ON mutation_briefs (window_id);

CREATE TABLE IF NOT EXISTS spawn_records (
    id BIGSERIAL PRIMARY KEY,
    data_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
