package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

var (
	sharedSQLiteDBs = make(map[string]*sql.DB)
	sharedSQLiteMu  sync.Mutex
	// postgresPool is injected by the bootstrap/main wiring when the
	// historical store is backed by PostgreSQL (StoreBackend=postgres).
	postgresPool *pgxpool.Pool
)

// SetPostgresPool injects the shared pgxpool used by PostgresHistoricalStore.
// Called once at wiring time when StoreBackend=postgres; nil leaves the
// postgres historical backend unavailable.
func SetPostgresPool(pool *pgxpool.Pool) {
	postgresPool = pool
}

// getSharedSQLiteDB returns a shared *sql.DB for the given path.
// The DB is opened once per path, WAL mode is enabled, foreign keys
// are enforced, and the ledger schema is initialized on the first call.
func getSharedSQLiteDB(path string) (*sql.DB, error) {
	sharedSQLiteMu.Lock()
	defer sharedSQLiteMu.Unlock()

	if db, ok := sharedSQLiteDBs[path]; ok {
		return db, nil
	}

	db, err := OpenSQLiteDB(path)
	if err != nil {
		return nil, err
	}

	if err := InitSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	sharedSQLiteDBs[path] = db
	return db, nil
}

func NewFullStore(cfg config.Config) (FullStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		return newSQLiteFullStore(cfg.SQLitePath)
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func newStore(baseDir string) *Store {
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	return &Store{baseDir: baseDir}
}

func newSQLiteFullStore(path string) (*SQLiteStore, error) {
	db, err := getSharedSQLiteDB(path)
	if err != nil {
		return nil, fmt.Errorf("shared sqlite: %w", err)
	}

	return &SQLiteStore{
		db:                 db,
		SQLiteOutcomeStore: NewSQLiteOutcomeStore(db),
		SQLiteQuoteStore:   NewSQLiteQuoteStore(db),
	}, nil
}

type SQLiteStore struct {
	db *sql.DB
	*SQLiteOutcomeStore
	*SQLiteQuoteStore
}

var _ FullStore = (*SQLiteStore)(nil)

// RecordSpawnRecord persists a spawn record as a JSON blob.
func (s *SQLiteStore) RecordSpawnRecord(record SpawnRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal spawn record: %w", err)
	}
	_, err = s.db.Exec(
		`
		INSERT INTO spawn_records (data_json, created_at)
		VALUES (?, ?)`,
		string(data), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert spawn record: %w", err)
	}
	return nil
}

// LoadSpawnRecords reads all spawn records, most recent first.
func (s *SQLiteStore) LoadSpawnRecords() ([]SpawnRecord, error) {
	rows, err := s.db.Query(`SELECT data_json FROM spawn_records ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query spawn records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []SpawnRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan spawn record: %w", err)
		}
		var rec SpawnRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			return nil, fmt.Errorf("unmarshal spawn record: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// LoadExperiments reads all experiment records from the global experiments table.
func (s *SQLiteStore) LoadExperiments() ([]domain.ExperimentRecord, error) {
	rows, err := s.db.Query(`SELECT mutation_brief_json FROM experiments ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query experiments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []domain.ExperimentRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan experiment: %w", err)
		}
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			return nil, fmt.Errorf("unmarshal experiment: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// RecordPromptExperimentResult persists a prompt experiment result as a JSON blob.
func (s *SQLiteStore) RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal prompt experiment result: %w", err)
	}
	_, err = s.db.Exec(
		`
		INSERT INTO prompt_experiment_results (experiment_id, data_json, created_at)
		VALUES (?, ?, ?)`,
		experimentID, string(data), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert prompt experiment result: %w", err)
	}
	return nil
}

// UpdatePromptExperimentResult replaces an existing prompt experiment result by experiment_id.
func (s *SQLiteStore) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal prompt experiment result: %w", err)
	}
	_, err = s.db.Exec(
		`
		INSERT INTO prompt_experiment_results (experiment_id, data_json, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(experiment_id) DO UPDATE SET
			data_json = excluded.data_json,
			created_at = excluded.created_at`,
		experimentID, string(data), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert prompt experiment result: %w", err)
	}
	return nil
}

// RecordWindowSummary persists a backtest window summary as a JSON blob.
func (s *SQLiteStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal window summary: %w", err)
	}
	_, err = s.db.Exec(
		`
		INSERT INTO window_summaries (window_id, data_json, created_at)
		VALUES (?, ?, ?)`,
		summary.WindowID, string(data), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert window summary: %w", err)
	}
	return nil
}

// RecordMutationBrief persists a mutation brief as a JSON blob.
func (s *SQLiteStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	data, err := json.Marshal(brief)
	if err != nil {
		return fmt.Errorf("marshal mutation brief: %w", err)
	}
	_, err = s.db.Exec(
		`
		INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES (?, ?, ?)`,
		windowID, string(data), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert mutation brief: %w", err)
	}
	return nil
}

func NewOutcomeStore(cfg config.Config) (OutcomeStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := getSharedSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("shared sqlite: %w", err)
		}
		// WithJSONLBaseDir: the SQLite outcomes table drops evaluation fields
		// (Hit/ForwardReturn/Window) on write, so Load* delegates to the rich
		// per-session JSONL source (LedgerDir) when available.
		return NewSQLiteOutcomeStore(db).WithJSONLBaseDir(cfg.LedgerDir), nil
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func NewSessionStore(cfg config.Config) (SessionStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := getSharedSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("shared sqlite: %w", err)
		}
		return NewSQLiteSessionStore(db), nil
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func NewQuoteStore(cfg config.Config) (QuoteStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := getSharedSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("shared sqlite: %w", err)
		}
		return NewSQLiteQuoteStore(db), nil
	default:
		return NewJSONLQuoteStore(cfg.LedgerDir), nil
	}
}

// NewDetectorScanStore returns the SQLite-backed Stage 5 PR#4
// detector_scan_log store. Unlike other stores, DetectorScanStore has NO
// JSONL fallback — the plan contract (../../docs/archive/2026-07-14-atlas-stage5-detector-plan.md §PR#4)
// explicitly mandates SQLite so the MCP `template_detector_status` tool
// can query scan history with efficient LIMIT + ORDER BY.
func NewDetectorScanStore(cfg config.Config) (DetectorScanStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := getSharedSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("detector_scan: shared sqlite: %w", err)
		}
		return NewSQLiteDetectorScanStore(db), nil
	default:
		return nil, fmt.Errorf("detector_scan: backend %q not supported (sqlite-only per Stage 5 PR#4 contract)", cfg.StoreBackend)
	}
}

// NewHistoricalStore returns the SQLite-backed Stage 4 PR#2 historical store.
// Rows carry is_synthetic (1 for staging JSONL fixtures, 0 for real production
// data). The store is SQLite-only because the MCP history_* tools (history_regime,
// history_stress, history_event_calendar) run LIMIT + ORDER BY queries that
// need an indexed relational table.
func NewHistoricalStore(cfg config.Config) (HistoricalStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := getSharedSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("historical: shared sqlite: %w", err)
		}
		return NewSQLiteHistoricalStore(db), nil
	case "postgres":
		if postgresPool == nil {
			return nil, fmt.Errorf("historical: postgres backend requires SetPostgresPool before NewHistoricalStore")
		}
		return NewPostgresHistoricalStore(postgresPool), nil
	default:
		return nil, fmt.Errorf("historical: backend %q not supported (sqlite or postgres)", cfg.StoreBackend)
	}
}
