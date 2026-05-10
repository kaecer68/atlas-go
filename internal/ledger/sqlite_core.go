// Package ledger provides SQLite-backed persistence for Atlas trading outcomes,
// experiments, sessions, human interventions, and market quotes.
package ledger

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenSQLiteDB opens or creates a SQLite database at the given path,
// enables WAL mode, and enforces foreign keys.
func OpenSQLiteDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable wal mode: %w", err)
	}

	// Enable foreign key constraints.
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
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

	CREATE INDEX IF NOT EXISTS idx_outcomes_session_id ON outcomes(session_id);
	CREATE INDEX IF NOT EXISTS idx_outcomes_symbol ON outcomes(symbol);
	CREATE INDEX IF NOT EXISTS idx_screening_rejects_session_id ON screening_rejects(session_id);
	CREATE INDEX IF NOT EXISTS idx_experiments_session_id ON experiments(session_id);
	CREATE INDEX IF NOT EXISTS idx_human_interventions_session_id ON human_interventions(session_id);
	CREATE INDEX IF NOT EXISTS idx_trades_session_id ON trades(session_id);
	CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol);
	CREATE INDEX IF NOT EXISTS idx_quotes_symbol_date ON quotes(symbol, date);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	return nil
}
