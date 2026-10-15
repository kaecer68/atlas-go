package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// WindowSummaryReader is the READ half of the backtest-window surface whose
// write half is BacktestStore (window summary + mutation brief).
//
// It exists because the window family used to be file-only on BOTH sides: the
// JSONL *Store wrote <LedgerDir>/windows/<windowID>.json and every reader
// scanned that directory with os.ReadDir/os.ReadFile (internal/monitoring's
// report + system-health, internal/experiment's judge). Once the producers move
// to the backend-aware factory (ATLAS_STORE_BACKEND=postgres), a file-scanning
// reader silently answers with the pre-migration files — i.e. stale or empty,
// which is the same failure mode as the T1 signal-engine incident
// (scripts/ci/check_ledger_seam.py). Consumers therefore take this interface as
// an OPTIONAL seam: injected ⇒ read through the store; nil ⇒ keep the legacy
// file behavior.
//
// LoadWindowSummary returns ok=false (and a zero summary) when the window is
// absent — a missing window is a normal state (the producer has not run yet),
// not an error.
type WindowSummaryReader interface {
	LoadWindowSummaries() ([]domain.BacktestWindowSummary, error)
	LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error)
}

// WindowStore is the backend-aware backtest-window surface: both halves of the
// family, resolved by NewWindowStore from config.StoreBackend.
type WindowStore interface {
	BacktestStore
	WindowSummaryReader
}

// NewWindowStore returns the backend-aware window store for the configured
// backend, following the same shape as NewFullStore/NewOutcomeStore/
// NewReportOutcomeStore: jsonl and sqlite answer directly, postgres is wrapped
// in PGFirstOutcomeStore so a PostgreSQL outage degrades to the JSONL ledger
// (marked degraded) instead of failing the read outright.
//
// Producers must use this (or NewFullStore) instead of a raw ledger.NewStore
// directory: the raw JSONL store answers a missing file with an empty result,
// so a postgres deployment would write its window summaries where no reader
// looks.
func NewWindowStore(cfg config.Config) (WindowStore, error) {
	backend, err := ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, fmt.Errorf("window store: %w", err)
	}
	switch backend {
	case "jsonl":
		return newStore(cfg.LedgerDir), nil
	case "sqlite":
		return newSQLiteFullStore(cfg.SQLitePath)
	case "postgres":
		if postgresPool == nil {
			return nil, fmt.Errorf("window store: postgres backend requires SetPostgresPool before NewWindowStore")
		}
		return NewPGFirstOutcomeStore(NewPostgresLedgerStore(postgresPool), newStore(cfg.LedgerDir)), nil
	default:
		return nil, fmt.Errorf("window store: unexpected store backend %q", backend)
	}
}

// windowSummariesDir is the JSONL window directory. It holds both halves of
// the family: <windowID>.json (summaries) and <windowID>-mutation-brief.json
// (briefs).
func (s *Store) windowSummariesDir() string {
	return filepath.Join(s.baseDir, "windows")
}

// LoadWindowSummaries reads every window summary in the JSONL window
// directory, skipping mutation-brief files (they are a separate artifact and
// carry no window id field to filter on). A missing directory yields an empty
// result, matching the historical os.ReadDir behavior in
// internal/monitoring/service (report + system health).
func (s *Store) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	dir := s.windowSummariesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read windows dir %s: %w", dir, err)
	}

	var summaries []domain.BacktestWindowSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, mutationBriefSuffix) {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			// Deleted between ReadDir and ReadFile (a concurrent producer
			// rotating the directory): skip rather than fail the whole read.
			continue
		}
		var summary domain.BacktestWindowSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			// A single unreadable window must not blank the whole report —
			// same policy as the pre-migration reader.
			logging.Warn("ledger_window_store", "corrupted_window_summary_skipped",
				logging.FStr("path", path), logging.Err(err))
			continue
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// LoadWindowSummary reads the window summary for one window id from the JSONL
// window directory. ok=false means "no such window" (also for ids that cannot
// name a file, so a caller-supplied id can never escape the directory).
func (s *Store) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	if !validWindowID(windowID) {
		return domain.BacktestWindowSummary{}, false, nil
	}
	path := filepath.Join(s.windowSummariesDir(), windowID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.BacktestWindowSummary{}, false, nil
		}
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("read window summary %s: %w", path, err)
	}
	var summary domain.BacktestWindowSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("unmarshal window summary %s: %w", path, err)
	}
	return summary, true, nil
}

// mutationBriefSuffix is the JSONL filename suffix of the mutation-brief half
// of the window family (internal/ledger's *Store.RecordMutationBrief).
const mutationBriefSuffix = "-mutation-brief.json"

// validWindowID rejects ids that would not name a single file inside the
// window directory (path separators, traversal, empty).
func validWindowID(windowID string) bool {
	return windowID != "" && !strings.ContainsAny(windowID, `/\`)
}

// LoadWindowSummaries reads every persisted window summary, oldest window id
// first (the ids are window-YYYYMMDD-YYYYMMDD, so the order is chronological
// and matches the JSONL directory order).
func (s *SQLiteStore) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	rows, err := s.db.Query(`SELECT data_json FROM window_summaries ORDER BY window_id`)
	if err != nil {
		return nil, fmt.Errorf("query window summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWindowSummaries(rows)
}

// LoadWindowSummary reads one window summary by id.
func (s *SQLiteStore) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	return loadSQLWindowSummary(s.db.QueryRow(`SELECT data_json FROM window_summaries WHERE window_id = ?`, windowID))
}

// LoadWindowSummaries reads every persisted window summary, oldest window id
// first.
func (s *PostgresLedgerStore) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT data_json FROM window_summaries ORDER BY window_id`)
	if err != nil {
		return nil, fmt.Errorf("query window summaries: %w", err)
	}
	defer rows.Close()
	return scanWindowSummaries(rows)
}

// LoadWindowSummary reads one window summary by id.
func (s *PostgresLedgerStore) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	return loadSQLWindowSummary(s.pool.QueryRow(context.Background(), `SELECT data_json FROM window_summaries WHERE window_id = $1`, windowID))
}

// windowSummaryRows is the shared shape of the SQLite/PG row cursors.
type windowSummaryRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanWindowSummaries(rows windowSummaryRows) ([]domain.BacktestWindowSummary, error) {
	var summaries []domain.BacktestWindowSummary
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan window summary: %w", err)
		}
		var summary domain.BacktestWindowSummary
		if err := json.Unmarshal([]byte(data), &summary); err != nil {
			return nil, fmt.Errorf("unmarshal window summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("window summary rows: %w", err)
	}
	return summaries, nil
}

// windowSummaryRow is the shared shape of the SQLite/PG single-row readers.
type windowSummaryRow interface {
	Scan(dest ...any) error
}

// loadSQLWindowSummary turns a single-row read into the (summary, found, error)
// contract: sql.ErrNoRows and pgx.ErrNoRows both mean "no such window".
func loadSQLWindowSummary(row windowSummaryRow) (domain.BacktestWindowSummary, bool, error) {
	var data string
	err := row.Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			return domain.BacktestWindowSummary{}, false, nil
		}
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("scan window summary: %w", err)
	}
	var summary domain.BacktestWindowSummary
	if err := json.Unmarshal([]byte(data), &summary); err != nil {
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("unmarshal window summary: %w", err)
	}
	return summary, true, nil
}

var (
	_ WindowStore = (*Store)(nil)
	_ WindowStore = (*SQLiteStore)(nil)
	_ WindowStore = (*PostgresLedgerStore)(nil)
)
