package db

import (
	"context"
	"strings"
	"testing"
)

func TestInitRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Init(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestInit_EmptyURL_EnvFallbackToInvalid(t *testing.T) {
	t.Setenv("DATABASE_URL", "not-a-valid-url")
	_, err := Init(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error when DATABASE_URL env is invalid")
	}
	if !strings.Contains(err.Error(), "create connection pool") {
		t.Fatalf("expected connection pool error, got: %v", err)
	}
}

func TestInit_UnreachableURL_FailsAtPing(t *testing.T) {
	// Use a URL that pgxpool.New can parse but Ping will fail on.
	// Port 1 is reserved and should refuse connections immediately.
	_, err := Init(context.Background(), "postgres://localhost:1/db?sslmode=disable&connect_timeout=1", "")
	if err == nil {
		t.Fatal("expected ping error for unreachable database")
	}
	if !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("expected ping error, got: %v", err)
	}
}

func TestRunMigrations_InvalidSourcePath(t *testing.T) {
	err := runMigrations("postgres://localhost:5432/db", "/nonexistent/migration/path")
	if err == nil {
		t.Fatal("expected error for invalid migration source path")
	}
	if !strings.Contains(err.Error(), "create migrate instance") {
		t.Fatalf("expected migrate instance error, got: %v", err)
	}
}

func TestRunMigrations_UpError(t *testing.T) {
	// Empty temp dir as migration source, unreachable DB URL.
	// migrate.New validates the DB URL immediately and returns an error.
	dir := t.TempDir()
	err := runMigrations("postgres://localhost:9999/db?sslmode=disable&connect_timeout=1", dir)
	if err == nil {
		t.Fatal("expected error for unreachable database")
	}
	// migrate.New connects to validate the driver URL,
	// so the error comes from "create migrate instance" not "migrate up".
	if !strings.Contains(err.Error(), "create migrate instance") {
		t.Fatalf("expected migrate instance error, got: %v", err)
	}
}
