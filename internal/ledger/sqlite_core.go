// Package ledger provides SQLite-backed persistence for Atlas trading outcomes,
// experiments, sessions, human interventions, and market quotes.
package ledger

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenSQLiteDB opens or creates a SQLite database at the given path
// with WAL mode, a 5-second busy timeout, and foreign keys enabled.
// Pragmas are passed via DSN so they apply to every pooled connection;
// per-connection db.Exec would only reach one of them.
func OpenSQLiteDB(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	return db, nil
}

// InitSchema creates all ledger tables if they do not already exist,
// along with the indexes required for efficient querying.
func InitSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS outcomes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		symbol TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		action TEXT,
		weight REAL,
		target_price REAL,
		stop_loss REAL,
		conviction REAL,
		regime TEXT,
		timestamp TEXT,
		passed_guards INTEGER,
		guard_reason TEXT,
		factor_scores_json TEXT,
		conviction_breakdown_json TEXT
	);

	CREATE TABLE IF NOT EXISTS screening_rejects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		symbol TEXT NOT NULL,
		reason TEXT,
		timestamp TEXT,
		factor_scores_json TEXT
	);

	CREATE TABLE IF NOT EXISTS experiments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		experiment_id TEXT UNIQUE NOT NULL,
		session_id TEXT,
		mutation_brief_json TEXT,
		result_json TEXT,
		accepted INTEGER,
		timestamp TEXT
	);

	CREATE TABLE IF NOT EXISTS session_summaries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT UNIQUE NOT NULL,
		total_recs INTEGER,
		passed_guards INTEGER,
		rejected INTEGER,
		timestamp TEXT
	);

	CREATE TABLE IF NOT EXISTS human_interventions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		symbol TEXT NOT NULL,
		agent_id TEXT,
		action TEXT,
		timestamp TEXT
	);

	CREATE TABLE IF NOT EXISTS quotes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol TEXT NOT NULL,
		name TEXT,
		date TEXT NOT NULL,
		open REAL,
		high REAL,
		low REAL,
		close REAL,
		volume INTEGER,
		source TEXT,
		UNIQUE(symbol, date)
	);

	CREATE TABLE IF NOT EXISTS trades (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		trade_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		symbol TEXT NOT NULL,
		side TEXT NOT NULL,
		quantity INTEGER NOT NULL,
		price REAL NOT NULL,
		amount REAL NOT NULL,
		reason TEXT,
		timestamp TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS spawn_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		data_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS prompt_experiment_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		experiment_id TEXT UNIQUE NOT NULL,
		data_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS window_summaries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		window_id TEXT UNIQUE NOT NULL,
		data_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS mutation_briefs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		window_id TEXT NOT NULL,
		data_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	-- Stage 4 PR#2 — historical backfill tables. Each row carries is_synthetic
	-- (always 1 for these tables because they are populated from the
	-- Stage 4 CLI's staging JSONLs, NOT from live runtime emitters).
	-- PR #1247 (manifest 2026-07-21-regime-history-source-and-vocab-normalize.md D01):
	-- added 'source' column. Existing rows from older binaries have 'source'
	-- backfilled to 'synthetic' via the migration below; new rows written by
	-- the live ingest pipeline populate source='macro_ingest'.
	CREATE TABLE IF NOT EXISTS regime_history (
		date TEXT PRIMARY KEY,
		regime TEXT NOT NULL,
		source_session_id TEXT,
		recorded_at TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL,
		source TEXT NOT NULL DEFAULT 'synthetic'
	);

	CREATE TABLE IF NOT EXISTS stress_index_history (
		date TEXT PRIMARY KEY,
		score REAL NOT NULL,
		regime TEXT,
		components_json TEXT,
		source TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL
	);

	-- period_history stores the daily seven-period classification from
	-- PeriodDetector.DetectPeriod(). One row per date; UPSERT by date.
	-- Written by the live ingest pipeline (applyMacroUpdate).
	CREATE TABLE IF NOT EXISTS period_history (
		date TEXT PRIMARY KEY,
		period TEXT NOT NULL,
		recorded_at TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL,
		source TEXT NOT NULL DEFAULT 'macro_ingest'
	);


	CREATE TABLE IF NOT EXISTS geopolitical_history (
		date TEXT PRIMARY KEY,
		intensity REAL NOT NULL,
		sources_json TEXT,
		source TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS event_calendar_history (
		date TEXT NOT NULL,
		event_id TEXT NOT NULL,
		active_theme TEXT,
		source TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL,
		PRIMARY KEY (date, event_id)
	);

	CREATE TABLE IF NOT EXISTS prediction_backtest (
		date TEXT PRIMARY KEY,
		predicted_direction TEXT,
		predicted_confidence REAL,
		actual_direction TEXT,
		actual_capital_flow_change REAL,
		hit INTEGER,
		model_version TEXT,
		captured_at TEXT NOT NULL,
		is_synthetic INTEGER NOT NULL
	);

	-- Stage 5 PR#4: template trigger detector scan results
	-- One row per DetectionResult from narrative.DetectorRegistry.RunAll().
	-- scan_batch_id groups all rows from a single RunAll call (UUID per scan).
	CREATE TABLE IF NOT EXISTS detector_scan_log (
		scan_id INTEGER PRIMARY KEY AUTOINCREMENT,
		scan_batch_id TEXT NOT NULL,
		theme TEXT NOT NULL,
		severity TEXT NOT NULL,
		confidence REAL NOT NULL,
		detected_at TEXT NOT NULL,
		source TEXT NOT NULL,
		metadata_json TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_outcomes_session_id ON outcomes(session_id);
	CREATE INDEX IF NOT EXISTS idx_outcomes_symbol ON outcomes(symbol);
	CREATE INDEX IF NOT EXISTS idx_screening_rejects_session_id ON screening_rejects(session_id);
	CREATE INDEX IF NOT EXISTS idx_experiments_session_id ON experiments(session_id);
	CREATE INDEX IF NOT EXISTS idx_human_interventions_session_id ON human_interventions(session_id);
	CREATE INDEX IF NOT EXISTS idx_trades_session_id ON trades(session_id);
	CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol);
	CREATE INDEX IF NOT EXISTS idx_quotes_symbol_date ON quotes(symbol, date);
	CREATE INDEX IF NOT EXISTS idx_prompt_experiment_results_experiment_id ON prompt_experiment_results(experiment_id);
	CREATE INDEX IF NOT EXISTS idx_window_summaries_window_id ON window_summaries(window_id);
	CREATE INDEX IF NOT EXISTS idx_mutation_briefs_window_id ON mutation_briefs(window_id);
	CREATE INDEX IF NOT EXISTS idx_regime_history_captured_at ON regime_history(captured_at);
	CREATE INDEX IF NOT EXISTS idx_stress_index_history_captured_at ON stress_index_history(captured_at);
	CREATE INDEX IF NOT EXISTS idx_period_history_captured_at ON period_history(captured_at);
	CREATE INDEX IF NOT EXISTS idx_geopolitical_history_captured_at ON geopolitical_history(captured_at);
	CREATE INDEX IF NOT EXISTS idx_event_calendar_history_date ON event_calendar_history(date);
	CREATE INDEX IF NOT EXISTS idx_prediction_backtest_captured_at ON prediction_backtest(captured_at);
	CREATE INDEX IF NOT EXISTS idx_detector_scan_log_batch ON detector_scan_log(scan_batch_id);
	CREATE INDEX IF NOT EXISTS idx_detector_scan_log_theme_time ON detector_scan_log(theme, detected_at);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	// Apply additive column migrations that predate CREATE TABLE IF NOT EXISTS
	// supporting newer columns. Each migration is idempotent: PRAGMA table_info
	// gates the ALTER so re-running InitSchema on a fresh DB (or on an
	// already-migrated DB) is a no-op. Each migration's intent is captured
	// here so future operators reading the file can trace the schema delta.
	additiveMigrations := []func(*sql.DB) error{
		addRegimeHistorySourceColumn, // PR #1247 (D01)
		addOutcomesEvaluationColumns, // BL-06 (outcomes 評估欄位持久化)
	}
	for _, m := range additiveMigrations {
		if err := m(db); err != nil {
			return fmt.Errorf("apply additive migration: %w", err)
		}
	}

	return nil
}

// addRegimeHistorySourceColumn adds `source TEXT NOT NULL DEFAULT 'synthetic'`
// to regime_history if the column is missing. Existing rows inherit the
// DEFAULT value (backfill to 'synthetic'); new writes from live ingest
// (cmd/atlas/operations_tasks.go::macro_ingest → DashboardAPI.IngestAndUpdateMacro
// → persistRegime if present, or the StressRow-equivalent for regime) populate
// `source='macro_ingest'`. This migration is idempotent because ALTER TABLE
// ADD COLUMN with a DEFAULT is a no-op when the column already exists (we
// gate it explicitly with PRAGMA table_info for clarity + testability).
func addRegimeHistorySourceColumn(db *sql.DB) error {
	const table = "regime_history"
	const column = "source"

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma row: %w", err)
		}
		if name == column {
			return nil
		}
	}

	if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` TEXT NOT NULL DEFAULT 'synthetic'`); err != nil {
		return fmt.Errorf("alter %s add %s: %w", table, column, err)
	}
	return nil
}

// addColumnIfMissing adds a column to a table if it is not already present.
// Idempotent: PRAGMA table_info gates the ALTER, so re-running on a fresh or
// already-migrated DB is a no-op.
func addColumnIfMissing(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma row: %w", err)
		}
		if name == column {
			return nil
		}
	}
	if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + ddl); err != nil {
		return fmt.Errorf("alter %s add %s: %w", table, column, err)
	}
	return nil
}

// addOutcomesEvaluationColumns adds the evaluation fields that the SQLite
// outcomes table previously dropped on write (BL-06). The legacy `regime`
// column was misused to store AgentLayer (see RecordSessionOutcomes), so a
// separate `true_regime` column holds the real regime and `layer` the real
// layer. These columns let the performance report see genuine forward
// returns / regime / window for StoreBackend=sqlite instead of zeros.
func addOutcomesEvaluationColumns(db *sql.DB) error {
	const table = "outcomes"
	for col, ddl := range map[string]string{
		"layer":           "TEXT",
		"forward_return":  "REAL",
		"window":          "TEXT",
		"hit":             "INTEGER",
		"benchmark_delta": "REAL",
		"is_synthetic":    "INTEGER",
		"true_regime":     "TEXT",
	} {
		if err := addColumnIfMissing(db, table, col, ddl); err != nil {
			return fmt.Errorf("migrate %s.%s: %w", table, col, err)
		}
	}
	return nil
}
