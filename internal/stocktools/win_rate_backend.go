// win-rate read backend resolution (C6b).
//
// The four win-rate endpoints (GET /api/stock/win_rate,
// /api/stock/condition_winrate, /api/stock/industry_winrate,
// /api/stock/family-expectancy) read exactly one store per process:
// PostgreSQL when the deployment declares the postgres store backend and a DSN
// is available, otherwise the read-only SQLite ledger (the pre-C6b behavior,
// kept for backward compatibility). The design mirrors the MCP read path
// settled in C6 (cmd/atlas-mcp/server/stockpicker_read.go): one normalization
// rule, a structurally read-only PostgreSQL session, a per-call probe with
// SQLite fallback, and one SQL text that both backends execute.
package stocktools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// win-rate read backends (C6b). Same identifiers as the MCP read path (C6) so
// an operator's `ATLAS_STORE_BACKEND` means the same thing on both servers.
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
		return "", fmt.Errorf("stocktools win-rate read backend: %w", err)
	}
	if backend == WinRateBackendPostgres {
		return WinRateBackendPostgres, nil
	}
	return WinRateBackendSQLite, nil
}

// OpenReadOnlyWinRatePGDB derives the read-only PostgreSQL handle the four
// win-rate endpoints use when the postgres backend is declared and
// DATABASE_URL is set.
//
// Read-only is structural, the PostgreSQL twin of the SQLite handle's
// `mode=ro`: the connection starts with default_transaction_read_only=on, so
// an accidental INSERT/UPDATE on this handle fails inside the server
// (SQLSTATE 25006) instead of mutating the ledger. The no-write contract of
// the win-rate endpoints therefore does not depend on the queries staying
// SELECTs.
//
// The handle dials lazily: no connection is opened here, so a PostgreSQL that
// is unreachable at HTTP-server startup is handled by the per-call probe plus
// SQLite fallback in WinRateSource.Handle, never by refusing to start the
// server. The caller owns the returned handle and closes it.
func OpenReadOnlyWinRatePGDB(dsn string) (*sql.DB, error) {
	connConfig, err := readOnlyWinRateConnConfig(dsn)
	if err != nil {
		return nil, err
	}

	db := stdlib.OpenDB(*connConfig)
	// The win-rate endpoints issue a handful of small reads per request. A two
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
		return nil, fmt.Errorf("stocktools: parse postgres DSN: %w", err)
	}
	if connConfig.RuntimeParams == nil {
		connConfig.RuntimeParams = map[string]string{}
	}
	connConfig.RuntimeParams["default_transaction_read_only"] = "on"
	return connConfig, nil
}

// winRateReadHandle is the resolved read source for the win-rate endpoints:
// exactly one backend, plus the backend label used in log lines.
//
// Placeholders: every win-rate query is written with PostgreSQL-style $N
// placeholders and executed on both backends. That is not a wording trick —
// internal/stockpicker's store layer (LoadWinRate/LoadOutcomes) already speaks
// $N and runs on the SQLite driver in production, because modernc.org/sqlite
// binds positional arguments to named parameters in order of appearance.
// Keeping one SQL text for both backends is what makes "the PG path and the
// SQLite path return the same rows" a checkable claim rather than an
// assumption.
type winRateReadHandle struct {
	db      *sql.DB
	backend string
}

// WinRateSource resolves which store the four win-rate endpoints read.
//
// Order (C6b):
//  1. PostgreSQL, when the wiring handed it a read-only handle
//     (OpenReadOnlyWinRatePGDB) and the handle answers a probe. A reachable
//     PostgreSQL is authoritative even when it holds no rows: an empty result
//     is data, not an outage (the same stance as ledger.PGFirstOutcomeStore),
//     so an empty PG never falls back to SQLite.
//  2. The read-only SQLite ledger (OpenWinRateDB), whenever PostgreSQL is not
//     wired or not reachable.
//
// Only a successful PostgreSQL resolution is cached. While PostgreSQL stays
// unreachable every call re-probes it (and logs the fallback, so a read that
// silently changed store cannot go unnoticed), so a PostgreSQL that comes back
// after a restart is picked up without restarting the HTTP server; the SQLite
// handle is cached separately by sqliteHandle.
//
// A source with no PostgreSQL handle at all (the pre-C6b deployment shape)
// caches the SQLite decision on first use: there is nothing to re-probe.
type WinRateSource struct {
	pg         *sql.DB
	sqlitePath string
	// logf is the observability seam: the resolved backend must be recordable,
	// so tests inject a recorder instead of scraping the process log.
	logf func(format string, args ...any)

	mu       sync.Mutex
	resolved *winRateReadHandle
	sqlite   *sql.DB
}

// OpenWinRateSource is the production wiring entry point: it decides whether
// the endpoints get a PostgreSQL handle at all, and returns the source the
// provider resolves through.
//
// The decision is logged on every branch, because a deployment that believes it
// reads PostgreSQL while actually reading a stale SQLite file is the failure
// mode this change exists to remove; the alternative — silently leaving the
// PostgreSQL handle unwired — would reintroduce it in the opposite direction.
// Wiring never dials (see OpenReadOnlyWinRatePGDB), so an unreachable
// PostgreSQL at startup is reported by the first request, not by a crash.
func OpenWinRateSource(pgURL, declaredBackend, sqlitePath string) *WinRateSource {
	pg, reason := openWinRatePG(pgURL, declaredBackend)
	s := NewWinRateSource(pg, sqlitePath)
	if pg != nil {
		s.logf("stocktools: win-rate reads: PostgreSQL first (read-only session); SQLite fallback %s", sqlitePath)
	} else {
		s.logf("stocktools: win-rate reads: SQLite ledger %s (%s)", sqlitePath, reason)
	}
	return s
}

// openWinRatePG returns the read-only PostgreSQL handle for the declared
// backend, or nil plus the reason it was not wired. The reason is a fixed
// message (never an echo of the raw environment value) so it is safe to log.
func openWinRatePG(pgURL, declaredBackend string) (*sql.DB, string) {
	if pgURL == "" {
		return nil, "DATABASE_URL not set"
	}
	backend, err := ResolveWinRateBackend(declaredBackend)
	if err != nil {
		return nil, fmt.Sprintf("ATLAS_STORE_BACKEND=%q is not a known backend", loggableBackendValue(declaredBackend))
	}
	if backend != WinRateBackendPostgres {
		return nil, fmt.Sprintf("ATLAS_STORE_BACKEND=%q does not declare postgres", loggableBackendValue(declaredBackend))
	}
	handle, err := OpenReadOnlyWinRatePGDB(pgURL)
	if err != nil {
		return nil, fmt.Sprintf("postgres handle: %v", err)
	}
	return handle, ""
}

// winRateLogf is the default sink of the win-rate read path. It is a package
// variable so a test can capture the lines instead of scraping the process log
// — the resolved backend must be assertable, not merely "logged somewhere".
var winRateLogf = log.Printf

// NewWinRateSource builds a source over an already-decided PostgreSQL handle
// (nil = SQLite only) and a SQLite ledger path.
func NewWinRateSource(pg *sql.DB, sqlitePath string) *WinRateSource {
	return &WinRateSource{pg: pg, sqlitePath: sqlitePath, logf: winRateLogf}
}

// Handle resolves the store this call must read. The error reports both
// backends when neither is usable, so an operator with a PostgreSQL outage is
// not told to configure the SQLite ledger.
func (s *WinRateSource) Handle(ctx context.Context) (winRateReadHandle, error) {
	s.mu.Lock()
	cached := s.resolved
	s.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}

	var pgErr error
	if s.pg != nil {
		if err := s.pg.PingContext(ctx); err == nil {
			h := winRateReadHandle{db: s.pg, backend: WinRateBackendPostgres}
			s.mu.Lock()
			s.resolved = &h
			s.mu.Unlock()
			s.logf("stocktools: win-rate reads: PostgreSQL (read-only session)")
			return h, nil
		} else {
			pgErr = err
			// Logged per call on purpose: the fallback decision is the thing
			// that must not be silent, and it is not cached (the next call
			// probes PostgreSQL again).
			s.logf("stocktools: win-rate reads: SQLite ledger (postgres handle unusable: %v)", err)
		}
	}

	sqliteDB, sqliteErr := s.sqliteHandle()
	switch {
	case sqliteErr == nil:
		h := winRateReadHandle{db: sqliteDB, backend: WinRateBackendSQLite}
		if s.pg == nil {
			// Nothing to re-probe: this deployment is SQLite-only until it is
			// restarted, so cache the decision instead of reopening the file
			// (and re-logging) on every request.
			s.mu.Lock()
			s.resolved = &h
			s.mu.Unlock()
		}
		return h, nil
	case pgErr != nil:
		return winRateReadHandle{}, fmt.Errorf("postgres win-rate ledger unusable (%v) and no SQLite fallback: %w", pgErr, sqliteErr)
	default:
		return winRateReadHandle{}, sqliteErr
	}
}

// sqliteHandle returns the cached read-only SQLite handle, opening it lazily
// from sqlitePath on first use. Read-only mode (mode=ro) makes the no-write
// contract structural: any accidental write fails at the driver level. A
// nil/empty path or a missing file is reported as a descriptive error so the
// handler can answer 503 instead of panicking.
func (s *WinRateSource) sqliteHandle() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlite != nil {
		return s.sqlite, nil
	}
	path := strings.TrimSpace(s.sqlitePath)
	if path == "" {
		return nil, errors.New("stocktools: win-rate SQLite ledger not configured (set ATLAS_MCP_STOCKPICKER_DB)")
	}
	db, err := OpenWinRateDB(path)
	if err != nil {
		return nil, err
	}
	s.sqlite = db
	return db, nil
}

// Close releases the handles the source owns (PostgreSQL and SQLite). Both are
// opened by this type, so both are its to close; the source is not reusable
// afterwards.
func (s *WinRateSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, db := range []*sql.DB{s.pg, s.sqlite} {
		if db == nil {
			continue
		}
		if err := db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	s.pg, s.sqlite, s.resolved = nil, nil, nil
	return errors.Join(errs...)
}

// loggableBackendValue renders an operator-supplied ATLAS_STORE_BACKEND value
// for a log line. The value reaches the log, so it is reduced to the characters
// a store backend name can contain first: an environment variable must not be
// able to forge log structure (gosec G706, log injection). Anything outside
// that set is reported as a marker instead of being echoed.
func loggableBackendValue(v string) string {
	if v == "" {
		return "(unset)"
	}
	if len(v) > 32 {
		return "(too long)"
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return "(contains characters outside [A-Za-z0-9_.-])"
		}
	}
	return v
}
