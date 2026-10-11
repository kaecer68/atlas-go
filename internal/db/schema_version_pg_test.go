//go:build integration

package db_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// TestVerifySchemaVersion_NoDriftAfterMigrations is the end-to-end PG gate for
// the drift guardrail: after db.Init has applied every migration, the check must
// stay silent. A false positive here would spam WARN + atlas_schema_version_drift_total
// on every healthy startup.
func TestVerifySchemaVersion_NoDriftAfterMigrations(t *testing.T) {
	testdb.Pool(t, "../../sql/migrations") // applies migrations; fails/skips per testdb policy

	dsn := testdb.URL(t)
	if err := db.VerifySchemaVersion(dsn, "../../sql/migrations"); err != nil {
		t.Fatalf("expected no drift after migrations were applied, got: %v", err)
	}
}

// TestVerifySchemaVersion_DetectsLaggingSchema proves the guardrail fires when
// the binary ships a migration the database has not applied: the same live
// database, compared against a copy of sql/migrations that additionally holds a
// future migration file, must report drift.
func TestVerifySchemaVersion_DetectsLaggingSchema(t *testing.T) {
	testdb.Pool(t, "../../sql/migrations")

	dsn := testdb.URL(t)
	applied, err := db.LatestMigrationVersion("../../sql/migrations")
	if err != nil {
		t.Fatalf("read latest migration on disk: %v", err)
	}

	future := t.TempDir()
	copyMigrations(t, "../../sql/migrations", future)
	const futureVersion = 999999
	if err := os.WriteFile(
		filepath.Join(future, "999999_future_schema.up.sql"),
		[]byte("-- drift gate fixture: never applied\n"),
		0o644,
	); err != nil {
		t.Fatalf("write future migration: %v", err)
	}

	err = db.VerifySchemaVersion(dsn, future)
	if err == nil {
		t.Fatal("expected drift when the migration files are ahead of schema_migrations")
	}
	if !db.IsSchemaVersionDrift(err) {
		t.Fatalf("expected schema drift, got %T (%v)", err, err)
	}
	var drift *db.SchemaDrift
	if !errors.As(err, &drift) {
		t.Fatalf("errors.As(*db.SchemaDrift) failed for %v", err)
	}
	if drift.Applied != applied || drift.Latest != futureVersion {
		t.Errorf("drift = %+v, want applied=%d latest=%d", drift, applied, futureVersion)
	}
	if drift.Reason() != "lagging" {
		t.Errorf("Reason() = %q, want %q", drift.Reason(), "lagging")
	}
	if !strings.Contains(err.Error(), "999999") {
		t.Errorf("drift error %q should mention the future version 999999", err)
	}
}

// copyMigrations copies every file (not directories) of src into dst.
func copyMigrations(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", entry.Name(), err)
		}
	}
}
