package ledger

import (
	"database/sql"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func NewFullStore(cfg config.Config) (FullStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		return newSQLiteFullStore(cfg.SQLitePath)
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func newStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func newSQLiteFullStore(path string) (*SQLiteStore, error) {
	db, err := OpenSQLiteDB(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
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

func (s *SQLiteStore) RecordSpawnRecord(record SpawnRecord) error {
	return fmt.Errorf("not implemented: %s", "RecordSpawnRecord")
}

func (s *SQLiteStore) LoadSpawnRecords() ([]SpawnRecord, error) {
	return nil, fmt.Errorf("not implemented: %s", "LoadSpawnRecords")
}

func (s *SQLiteStore) LoadExperiments() ([]domain.ExperimentRecord, error) {
	return nil, fmt.Errorf("not implemented: %s", "LoadExperiments")
}

func (s *SQLiteStore) RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	return fmt.Errorf("not implemented: %s", "RecordPromptExperimentResult")
}

func (s *SQLiteStore) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	return fmt.Errorf("not implemented: %s", "UpdatePromptExperimentResult")
}

func (s *SQLiteStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	return fmt.Errorf("not implemented: %s", "RecordWindowSummary")
}

func (s *SQLiteStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	return fmt.Errorf("not implemented: %s", "RecordMutationBrief")
}

func NewOutcomeStore(cfg config.Config) (OutcomeStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := OpenSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite db: %w", err)
		}
		if err := InitSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("init schema: %w", err)
		}
		return NewSQLiteOutcomeStore(db), nil
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func NewSessionStore(cfg config.Config) (SessionStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := OpenSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite db: %w", err)
		}
		if err := InitSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("init schema: %w", err)
		}
		return NewSQLiteSessionStore(db), nil
	default:
		return newStore(cfg.LedgerDir), nil
	}
}

func NewQuoteStore(cfg config.Config) (QuoteStore, error) {
	switch cfg.StoreBackend {
	case "sqlite":
		db, err := OpenSQLiteDB(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite db: %w", err)
		}
		if err := InitSchema(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("init schema: %w", err)
		}
		return NewSQLiteQuoteStore(db), nil
	default:
		return NewJSONLQuoteStore(cfg.LedgerDir), nil
	}
}
