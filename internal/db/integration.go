//go:build integration

// Integration-test helpers for package db.
//
// These helpers centralize one DATABASE_URL policy so integration tests
// across packages stop hardcoding DSNs (M6). They are compiled only under
// -tags=integration, which is used exclusively by `go test`.
package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDatabaseURL returns DATABASE_URL, failing in CI and skipping locally
// when it is unset.
func testDatabaseURL(t *testing.T) string {
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

// requireTestDB fails in CI or skips locally when PostgreSQL cannot be used.
// This keeps integration tests from silently skipping (a fake green light)
// when CI's postgres service is misconfigured (M6).
func requireTestDB(t *testing.T, err error) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatalf("PostgreSQL required for integration test: %v", err)
	}
	t.Skipf("PostgreSQL unavailable (%v); skipping PG integration test", err)
}

// connectPool opens a raw connection pool (no migrations) with the CI/local
// skip policy. The caller owns the pool and must close it.
func connectPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		requireTestDB(t, err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		requireTestDB(t, err)
	}
	return pool
}

// TestPool connects to PostgreSQL, applies migrations, and returns the pool.
// It reads DATABASE_URL only (no hardcoded DSN). The pool is closed via
// t.Cleanup.
func TestPool(t *testing.T, migrationsPath string) *pgxpool.Pool {
	t.Helper()
	dsn := testDatabaseURL(t)
	pool, err := Init(context.Background(), dsn, migrationsPath)
	if err != nil {
		requireTestDB(t, err)
	}
	t.Cleanup(pool.Close)
	return pool
}
