//go:build integration

package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// TestMigrationsUpDownUp applies every migration (up to latest), rolls all
// the way back down to 0, then back up to latest against a real PostgreSQL.
// This is the PG migration gate: DDL that is valid on SQLite but invalid on
// PostgreSQL (e.g. an unquoted `window` column, SQLSTATE 42601) must fail
// here before it reaches production.
//
// It runs inside a throwaway database derived from DATABASE_URL so the
// down/up cycle cannot race with, or wipe, other integration tests sharing
// the CI postgres service.
func TestMigrationsUpDownUp(t *testing.T) {
	baseURL := testDatabaseURL(t)

	dbName := fmt.Sprintf("atlas_migration_gate_%d", time.Now().UnixNano())
	testURL, err := withDatabaseName(baseURL, dbName)
	if err != nil {
		t.Fatalf("derive migration test database URL: %v", err)
	}

	admin := connectPool(t, baseURL)
	t.Cleanup(admin.Close)

	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+quoteIdent(dbName)); err != nil {
		t.Fatalf("create migration test database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(dbName)); err != nil {
			t.Logf("drop migration test database %s: %v", dbName, err)
		}
	})

	m := newMigrationGateInstance(t, testURL)
	t.Cleanup(func() { _, _ = m.Close() })

	// Up to latest.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	// Down all the way to 0.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate down to 0: %v", err)
	}
	// Back up to latest.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up after down: %v", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("read schema_migrations version: %v", err)
	}
	if dirty {
		t.Fatalf("schema_migrations is dirty (version %d) after up/down/up", version)
	}
	if version == 0 {
		t.Fatal("schema_migrations version is 0 after up/down/up; expected latest")
	}

	assertTablesExist(t, testURL, "stock_signal_outcomes", "stock_win_rate")
}

// newMigrationGateInstance builds a golang-migrate instance for the migration
// gate test. The postgres:// scheme is rewritten to pgx5:// to match Init().
func newMigrationGateInstance(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()
	driverURL := strings.Replace(dsn, "postgres://", "pgx5://", 1)
	m, err := migrate.New("file://../../sql/migrations", driverURL)
	if err != nil {
		t.Fatalf("create migrate instance: %v", err)
	}
	return m
}

// assertTablesExist verifies the given public tables exist after the
// up/down/up cycle (i.e. migrations 000018 and 000019 both applied).
func assertTablesExist(t *testing.T, dsn string, tables ...string) {
	t.Helper()
	pool := connectPool(t, dsn)
	defer pool.Close()

	ctx := context.Background()
	for _, table := range tables {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q does not exist after up/down/up", table)
		}
	}
}

// withDatabaseName replaces the database name in a postgres URL.
func withDatabaseName(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

// quoteIdent double-quotes a SQL identifier, escaping embedded quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
