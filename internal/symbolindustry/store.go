// Package symbolindustry persists the per-stock industry field produced by
// the first-party `symbol_industry` data channel, so industry-level
// statistics can be computed over a real population instead of a small
// hard-coded representative list.
//
// Backend split mirrors the ledger/channelsecrets stores: Postgres is the
// production source of truth (backend=postgres + injected pool); everything
// else falls back to the job-local SQLite artifact at
// <workDir>/data/state/atlas.db for dev/CLI use.
package symbolindustry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Mapping status values for Entry.MappingStatus.
const (
	StatusMapped   = "mapped"
	StatusUnmapped = "unmapped"
	StatusUnknown  = "unknown"
)

// Entry is one symbol's industry row. Symbol is normalized (no `.TW`
// suffix). CanonicalL1 is the canonical L1 sector id; an empty value means
// unmapped/unknown.
type Entry struct {
	Symbol         string    `json:"symbol"`
	CompanyName    string    `json:"company_name,omitempty"`
	Market         string    `json:"market,omitempty"`
	IndustryCode   string    `json:"industry_code"`
	IndustryNameZH string    `json:"industry_name_zh,omitempty"`
	CanonicalL1    string    `json:"canonical_l1,omitempty"`
	MappingStatus  string    `json:"mapping_status"`
	MappingReason  string    `json:"mapping_reason,omitempty"`
	Source         string    `json:"source,omitempty"`
	AsOf           string    `json:"as_of,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitzero"`
}

// Store persists symbol industry rows.
type Store interface {
	// UpsertAll inserts or replaces every entry, keyed by Symbol. Returns the
	// number of rows written. An empty slice is a no-op returning 0.
	UpsertAll(ctx context.Context, entries []Entry) (int, error)
	// LoadAll returns every row ordered by symbol ascending.
	LoadAll(ctx context.Context) ([]Entry, error)
	// Lookup returns one row; ok=false when the symbol is absent.
	Lookup(ctx context.Context, symbol string) (Entry, bool, error)
	// Count returns the number of rows.
	Count(ctx context.Context) (int, error)
}

// NewStore returns the backend-appropriate Store. backend "postgres" with a
// non-nil pool uses Postgres (production SSoT); anything else falls back to
// the job-local SQLite artifact (dev/CLI).
func NewStore(ctx context.Context, backend string, pool *pgxpool.Pool, workDir string) (Store, error) {
	if backend == "postgres" {
		if pool == nil {
			return nil, errors.New("symbolindustry: postgres backend requires a non-nil pool")
		}
		return &postgresStore{pool: pool}, nil
	}
	if workDir == "" {
		return nil, errors.New("symbolindustry: workDir required for sqlite store")
	}
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("symbolindustry: mkdir %s: %w", filepath.Dir(dbPath), err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("symbolindustry: open sqlite %s: %w", dbPath, err)
	}
	s := &sqliteStore{db: db}
	if err := s.ensureTable(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// --- Postgres (production) ---

type postgresStore struct {
	pool *pgxpool.Pool
}

const postgresColumns = `symbol, company_name, market, industry_code,
	industry_name_zh, canonical_l1, mapping_status, mapping_reason,
	source, as_of, updated_at`

func (s *postgresStore) UpsertAll(ctx context.Context, entries []Entry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("symbolindustry: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	for _, e := range entries {
		updatedAt := e.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO symbol_industry (`+postgresColumns+`)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (symbol) DO UPDATE SET
			     company_name = EXCLUDED.company_name,
			     market = EXCLUDED.market,
			     industry_code = EXCLUDED.industry_code,
			     industry_name_zh = EXCLUDED.industry_name_zh,
			     canonical_l1 = EXCLUDED.canonical_l1,
			     mapping_status = EXCLUDED.mapping_status,
			     mapping_reason = EXCLUDED.mapping_reason,
			     source = EXCLUDED.source,
			     as_of = EXCLUDED.as_of,
			     updated_at = EXCLUDED.updated_at`,
			e.Symbol, e.CompanyName, e.Market, e.IndustryCode,
			e.IndustryNameZH, e.CanonicalL1, e.MappingStatus, e.MappingReason,
			e.Source, e.AsOf, updatedAt); err != nil {
			return 0, fmt.Errorf("symbolindustry: upsert %s: %w", e.Symbol, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("symbolindustry: commit: %w", err)
	}
	return len(entries), nil
}

func (s *postgresStore) LoadAll(ctx context.Context) ([]Entry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+postgresColumns+` FROM symbol_industry ORDER BY symbol ASC`)
	if err != nil {
		return nil, fmt.Errorf("symbolindustry: load all: %w", err)
	}
	defer rows.Close()
	out := make([]Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("symbolindustry: load all rows: %w", err)
	}
	return out, nil
}

func (s *postgresStore) Lookup(ctx context.Context, symbol string) (Entry, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+postgresColumns+` FROM symbol_industry WHERE symbol = $1`, symbol)
	e, err := scanEntry(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	return e, true, nil
}

func (s *postgresStore) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM symbol_industry`).Scan(&n); err != nil {
		return 0, fmt.Errorf("symbolindustry: count: %w", err)
	}
	return n, nil
}

// --- SQLite (dev/CLI fallback; job-local atlas.db) ---

type sqliteStore struct {
	db *sql.DB
}

const sqliteColumns = `symbol, company_name, market, industry_code,
	industry_name_zh, canonical_l1, mapping_status, mapping_reason,
	source, as_of, updated_at`

func (s *sqliteStore) ensureTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS symbol_industry (
			symbol           TEXT PRIMARY KEY,
			company_name     TEXT NOT NULL DEFAULT '',
			market           TEXT NOT NULL DEFAULT '',
			industry_code    TEXT NOT NULL DEFAULT '',
			industry_name_zh TEXT NOT NULL DEFAULT '',
			canonical_l1     TEXT NOT NULL DEFAULT '',
			mapping_status   TEXT NOT NULL DEFAULT '',
			mapping_reason   TEXT NOT NULL DEFAULT '',
			source           TEXT NOT NULL DEFAULT '',
			as_of            TEXT NOT NULL DEFAULT '',
			updated_at       TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("symbolindustry: ensure table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_symbol_industry_canonical_l1
		 ON symbol_industry(canonical_l1)`); err != nil {
		return fmt.Errorf("symbolindustry: ensure index: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpsertAll(ctx context.Context, entries []Entry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("symbolindustry: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for _, e := range entries {
		updatedAt := e.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO symbol_industry (`+sqliteColumns+`)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (symbol) DO UPDATE SET
			     company_name = excluded.company_name,
			     market = excluded.market,
			     industry_code = excluded.industry_code,
			     industry_name_zh = excluded.industry_name_zh,
			     canonical_l1 = excluded.canonical_l1,
			     mapping_status = excluded.mapping_status,
			     mapping_reason = excluded.mapping_reason,
			     source = excluded.source,
			     as_of = excluded.as_of,
			     updated_at = excluded.updated_at`,
			e.Symbol, e.CompanyName, e.Market, e.IndustryCode,
			e.IndustryNameZH, e.CanonicalL1, e.MappingStatus, e.MappingReason,
			e.Source, e.AsOf, updatedAt.UTC().Format(time.RFC3339)); err != nil {
			return 0, fmt.Errorf("symbolindustry: upsert %s: %w", e.Symbol, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("symbolindustry: commit: %w", err)
	}
	return len(entries), nil
}

func (s *sqliteStore) LoadAll(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sqliteColumns+` FROM symbol_industry ORDER BY symbol ASC`)
	if err != nil {
		return nil, fmt.Errorf("symbolindustry: load all: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("symbolindustry: load all rows: %w", err)
	}
	return out, nil
}

func (s *sqliteStore) Lookup(ctx context.Context, symbol string) (Entry, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sqliteColumns+` FROM symbol_industry WHERE symbol = ?`, symbol)
	e, err := scanEntry(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	return e, true, nil
}

func (s *sqliteStore) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM symbol_industry`).Scan(&n); err != nil {
		return 0, fmt.Errorf("symbolindustry: count: %w", err)
	}
	return n, nil
}

// scanFn adapts pgx/sql row scanning to a shared Entry decoder.
type scanFn func(dest ...any) error

// scanEntry decodes one row. The updated_at column is TIMESTAMPTZ on
// Postgres and RFC3339 TEXT on SQLite, so it is read as a raw value and
// normalized to a UTC time.Time.
func scanEntry(scan scanFn) (Entry, error) {
	var e Entry
	var updatedAt any
	err := scan(&e.Symbol, &e.CompanyName, &e.Market, &e.IndustryCode,
		&e.IndustryNameZH, &e.CanonicalL1, &e.MappingStatus, &e.MappingReason,
		&e.Source, &e.AsOf, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, sql.ErrNoRows
		}
		return Entry{}, fmt.Errorf("symbolindustry: scan: %w", err)
	}
	e.UpdatedAt = parseUpdatedAt(updatedAt)
	return e, nil
}

func parseUpdatedAt(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case string:
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts.UTC()
		}
	case []byte:
		if ts, err := time.Parse(time.RFC3339, string(t)); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}
