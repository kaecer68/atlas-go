package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite" // driver for the read-only stockpicker SQLite handle

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// win-rate read backends (C6). The two win-rate tools read exactly one of
// these per process: PostgreSQL when the deployment declares the postgres
// store backend and a DSN is available, otherwise the read-only SQLite ledger
// (the pre-C6 behavior, kept for backward compatibility).
const (
	WinRateBackendSQLite   = "sqlite"
	WinRateBackendPostgres = "postgres"
)

// ResolveWinRateBackend normalizes a declared store backend value
// (ATLAS_STORE_BACKEND) into the backend the win-rate read path uses.
//
// It delegates to the repository's single normalization rule,
// ledger.ResolveStoreBackend, so the read path can never disagree with the
// writers about what "the declared backend" means; an unknown value is an
// error rather than a silent fallback. Anything that is not postgres —
// including the legacy jsonl default and an unset variable — maps to sqlite,
// because jsonl has no win-rate read implementation and the SQLite ledger is
// the compatibility fallback.
func ResolveWinRateBackend(declared string) (string, error) {
	backend, err := ledger.ResolveStoreBackend(declared)
	if err != nil {
		return "", fmt.Errorf("stockpicker win-rate read backend: %w", err)
	}
	if backend == WinRateBackendPostgres {
		return WinRateBackendPostgres, nil
	}
	return WinRateBackendSQLite, nil
}

// OpenReadOnlyWinRatePGDB derives the read-only PostgreSQL handle the win-rate
// tools use when the postgres backend is declared and DATABASE_URL is set.
//
// Read-only is structural, the PostgreSQL twin of the SQLite handle's
// `mode=ro`: the connection starts with default_transaction_read_only=on, so
// an accidental INSERT/UPDATE on this handle fails inside the server
// (SQLSTATE 25006) instead of mutating the ledger. The no-write contract of
// the win-rate tools therefore does not depend on the queries staying SELECTs.
//
// The handle dials lazily: no connection is opened here, so a PostgreSQL that
// is unreachable at MCP startup is handled by the per-call probe plus SQLite
// fallback in winRateHandle, never by refusing to start the server. The caller
// owns the returned handle and closes it.
func OpenReadOnlyWinRatePGDB(dsn string) (*sql.DB, error) {
	connConfig, err := readOnlyWinRateConnConfig(dsn)
	if err != nil {
		return nil, err
	}

	db := stdlib.OpenDB(*connConfig)
	// The win-rate tools issue a handful of small SELECTs per call. A two
	// connection ceiling keeps the read path from competing with the main
	// pool, which is the one that writes.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	return db, nil
}

// readOnlyWinRateConnConfig parses dsn and forces the connection into a
// read-only session. Split out from OpenReadOnlyWinRatePGDB so the read-only
// setting can be asserted without a PostgreSQL server.
func readOnlyWinRateConnConfig(dsn string) (*pgx.ConnConfig, error) {
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("stockpicker win-rate: parse postgres DSN: %w", err)
	}
	if connConfig.RuntimeParams == nil {
		connConfig.RuntimeParams = map[string]string{}
	}
	connConfig.RuntimeParams["default_transaction_read_only"] = "on"
	return connConfig, nil
}

// winRateReadHandle is the resolved read source for the two win-rate tools:
// exactly one backend, plus the backend label used in log lines.
//
// Placeholders: every win-rate query is written with PostgreSQL-style $N
// placeholders and executed on both backends. That is not a wording trick —
// internal/stockpicker's store layer (SaveWinRate/LoadWinRate,
// RecordOutcomes/LoadOutcomes) already speaks $N and runs on the SQLite driver
// in production, because modernc.org/sqlite binds positional arguments to
// named parameters in order of appearance. Keeping one SQL text for both
// backends is what makes "the PG path and the SQLite path return the same
// rows" a checkable claim rather than an assumption.
type winRateReadHandle struct {
	db      *sql.DB
	backend string
}

// winRateHandle resolves the read handle for the win-rate tools.
//
// Order (C6):
//  1. PostgreSQL, when main wired Config.WinRatePGDB and the handle answers a
//     probe. A reachable PostgreSQL is authoritative even when it holds no
//     rows: an empty result is data, not an outage (the same stance as
//     ledger.PGFirstOutcomeStore), so an empty PG never falls back to SQLite.
//  2. The read-only SQLite ledger (Config.StockpickerDBPath), whenever PG is
//     not wired or not reachable.
//
// Only a successful PostgreSQL resolution is cached. While PG stays
// unreachable every call re-probes it, so a PostgreSQL that comes back after a
// restart is picked up without restarting the MCP server; the SQLite handle is
// cached separately by stockpickerSQLiteDB, so the fallback path opens the
// file once.
func (s *server) winRateHandle(ctx context.Context) (winRateReadHandle, error) {
	s.winRateMu.Lock()
	cached := s.winRateResolved
	s.winRateMu.Unlock()
	if cached != nil {
		return *cached, nil
	}

	var pgErr error
	if pg := s.cfg.WinRatePGDB; pg != nil {
		if err := pg.PingContext(ctx); err == nil {
			h := winRateReadHandle{db: pg, backend: WinRateBackendPostgres}
			s.winRateMu.Lock()
			s.winRateResolved = &h
			s.winRateMu.Unlock()
			return h, nil
		} else {
			pgErr = err
			fmt.Fprintf(os.Stderr,
				"atlas-mcp: stockpicker win-rate: postgres handle unusable (%v); reading the SQLite ledger instead\n", err)
		}
	}

	sqliteDB, sqliteErr := s.stockpickerSQLiteDB(ctx)
	switch {
	case sqliteErr == nil:
		return winRateReadHandle{db: sqliteDB, backend: WinRateBackendSQLite}, nil
	case pgErr != nil:
		// Both backends are unusable. Report both, so an operator who has a
		// PostgreSQL outage is not told to configure the SQLite ledger.
		return winRateReadHandle{}, fmt.Errorf("postgres win-rate ledger unusable (%v) and no SQLite fallback: %w", pgErr, sqliteErr)
	default:
		return winRateReadHandle{}, sqliteErr
	}
}

// stockpickerSQLiteDB returns the cached read-only SQLite handle backing the
// stockpicker win-rate stores, opening it lazily from
// Config.StockpickerDBPath on first use. Read-only mode (mode=ro) makes the
// no-write contract structural: any accidental write fails at the driver
// level. A nil/empty path or a missing file is reported as a descriptive
// error so the handler can answer "no data" instead of panicking.
func (s *server) stockpickerSQLiteDB(ctx context.Context) (*sql.DB, error) {
	s.winRateMu.Lock()
	defer s.winRateMu.Unlock()
	if s.winRateSQLiteDB != nil {
		return s.winRateSQLiteDB, nil
	}
	path := s.cfg.StockpickerDBPath
	if path == "" {
		return nil, fmt.Errorf("stockpicker DB not configured (set ATLAS_MCP_STOCKPICKER_DB)")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("stockpicker DB path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("stockpicker DB %s: %w", abs, err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open stockpicker DB %s: %w", abs, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open stockpicker DB %s: %w", abs, err)
	}
	s.winRateSQLiteDB = db
	return db, nil
}
