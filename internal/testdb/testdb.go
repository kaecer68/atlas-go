// Package testdb centralizes one DATABASE_URL policy for PostgreSQL
// integration tests so tests across packages stop hardcoding DSNs (M6).
//
// Pool/Connect/URL/Require read DATABASE_URL only: missing DATABASE_URL or an
// unreachable database fails loudly in CI (os.Getenv("CI") set) and skips
// locally, so a broken CI postgres service can no longer produce a fake
// green light.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/db"
)

// URL returns DATABASE_URL, failing in CI and skipping locally when unset.
func URL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL not set but CI=true; integration tests require a PostgreSQL instance")
		}
		t.Skip("DATABASE_URL not set, skipping PG integration test")
	}
	return dsn
}

// Require fails in CI or skips locally when PostgreSQL cannot be used.
func Require(t *testing.T, err error) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatalf("PostgreSQL required for integration test: %v", err)
	}
	t.Skipf("PostgreSQL unavailable (%v); skipping PG integration test", err)
}

// Connect opens a raw connection pool (no migrations) with the CI/local skip
// policy. The caller owns the pool and must close it.
func Connect(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		Require(t, err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		Require(t, err)
	}
	return pool
}

// UnconnectedPool returns a real *pgxpool.Pool that never dials: ParseConfig
// only parses the DSN and MinConns=0 keeps NewWithConfig from opening a
// connection. Unit tests use it to verify pool wiring (open this var's seam →
// inject into the ledger factory → build the store) without a PostgreSQL
// server, so the pin "declared postgres must not fall back to sqlite" can be
// checked in `go test` (no build tag). It is the same trick as
// cmd/backfill-futures-bars' newUnconnectedPool, shared here instead of being
// copy-pasted per command package.
func UnconnectedPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse fake dsn %q: %v", dsn, err)
	}
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new unconnected pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Pool connects to PostgreSQL, applies migrations, and returns the pool. It
// reads DATABASE_URL only (no hardcoded DSN). The pool is closed via
// t.Cleanup.
func Pool(t *testing.T, migrationsPath string) *pgxpool.Pool {
	t.Helper()
	dsn := URL(t)
	pool, err := db.Init(context.Background(), dsn, migrationsPath)
	if err != nil {
		Require(t, err)
	}
	t.Cleanup(pool.Close)
	return pool
}
