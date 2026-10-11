package db

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
)

// Schema version drift guardrail.
//
// Motivation (2026-10-11): db.Init() applies migrations and then reports
// success, but nothing ever compared "what the database has" with "what this
// binary ships". A binary whose schema_migrations lags the migration files on
// disk keeps starting up and then fails (or silently degrades) on a missing
// column. This file adds that comparison as a cheap, reusable check; the caller
// (bootstrap) decides the policy — log WARN + metric, never fail startup.

// migrationUpFilePattern matches the golang-migrate up-migration file naming
// convention used by sql/migrations (NNNNNN_name.up.sql).
var migrationUpFilePattern = regexp.MustCompile(`^(\d+)_.+\.up\.sql$`)

// LatestMigrationVersion returns the highest version encoded in the migration
// file names inside migrationsPath.
//
// An unreadable directory, or one that holds no NNNNNN_*.up.sql file, is
// reported as an error instead of 0: "no migration files found" comparing equal
// to "nothing applied" is itself a silent failure.
func LatestMigrationVersion(migrationsPath string) (uint, error) {
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return 0, fmt.Errorf("read migrations directory %s: %w", migrationsPath, err)
	}

	var latest uint
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := migrationUpFilePattern.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		version, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		if !found || uint(version) > latest {
			latest = uint(version)
			found = true
		}
	}
	if !found {
		return 0, fmt.Errorf("no NNNNNN_*.up.sql migration files found in %s", migrationsPath)
	}
	return latest, nil
}

// SchemaState is the pair golang-migrate records in schema_migrations: the
// applied version and the dirty flag (a migration that started but did not
// complete).
type SchemaState struct {
	Version uint
	Dirty   bool
}

// SchemaDrift reports that the schema in the database is not the schema the
// running binary expects: the applied version lags the newest migration file on
// disk, and/or schema_migrations is dirty (half-applied migration).
type SchemaDrift struct {
	Applied uint
	Latest  uint
	Dirty   bool
}

func (e *SchemaDrift) Error() string {
	switch {
	case e.Dirty && e.Applied < e.Latest:
		return fmt.Sprintf("schema version drift: applied=%d (dirty) latest=%d", e.Applied, e.Latest)
	case e.Dirty:
		return fmt.Sprintf("schema_migrations is dirty at version %d (latest on disk=%d)", e.Applied, e.Latest)
	default:
		return fmt.Sprintf("schema version lag: applied=%d latest=%d", e.Applied, e.Latest)
	}
}

// Reason returns the bounded metric label for this drift, one of
// "lagging", "dirty" or "lagging_dirty" (bounded value set keeps Prometheus
// label cardinality fixed).
func (e *SchemaDrift) Reason() string {
	switch {
	case e.Dirty && e.Applied < e.Latest:
		return "lagging_dirty"
	case e.Dirty:
		return "dirty"
	default:
		return "lagging"
	}
}

// IsSchemaVersionDrift reports whether err is a *SchemaDrift. Callers use it to
// tell "the schema lags the code" apart from "the check itself could not run"
// (unreachable database, unreadable migrations directory).
func IsSchemaVersionDrift(err error) bool {
	var drift *SchemaDrift
	return errors.As(err, &drift)
}

// CheckSchemaVersion compares the applied schema state with the newest
// migration version available on disk.
//
// A lagging version or a dirty schema_migrations row is returned as
// *SchemaDrift. A schema at or ahead of the files on disk is not an error: a
// binary rollback legitimately leaves the database ahead of the code, and that
// is not a persistent schema mismatch.
func CheckSchemaVersion(state SchemaState, latest uint) error {
	if !state.Dirty && state.Version >= latest {
		return nil
	}
	return &SchemaDrift{Applied: state.Version, Latest: latest, Dirty: state.Dirty}
}

// appliedSchemaStateReader reports the schema state recorded in
// schema_migrations. It is the injection seam that lets unit tests exercise
// verifySchemaVersion without a live PostgreSQL.
type appliedSchemaStateReader func() (SchemaState, error)

// ReadAppliedSchemaState reads the version golang-migrate recorded in
// schema_migrations by opening a short-lived migrate instance against
// databaseURL. The instance is closed before returning, so the caller's pool is
// unaffected. A database that was never migrated (golang-migrate reports
// ErrNilVersion) is reported as version 0 with a nil error.
func ReadAppliedSchemaState(databaseURL, migrationsPath string) (SchemaState, error) {
	m, err := newMigrateInstance(databaseURL, migrationsPath)
	if err != nil {
		return SchemaState{}, err
	}
	defer func() { _, _ = m.Close() }()

	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return SchemaState{}, nil
		}
		return SchemaState{}, fmt.Errorf("read schema_migrations version: %w", err)
	}
	return SchemaState{Version: version, Dirty: dirty}, nil
}

// VerifySchemaVersion compares the schema_migrations state of databaseURL with
// the newest migration file inside migrationsPath. It returns a *SchemaDrift
// (test with IsSchemaVersionDrift) when the database lags or is dirty.
//
// Callers own the policy: bootstrap logs WARN + records a metric and keeps
// starting, because a lagging schema must not take the process down.
func VerifySchemaVersion(databaseURL, migrationsPath string) error {
	return verifySchemaVersion(migrationsPath, func() (SchemaState, error) {
		return ReadAppliedSchemaState(databaseURL, migrationsPath)
	})
}

// verifySchemaVersion is the testable core of VerifySchemaVersion: the newest
// version is read from disk, the applied state from the injected reader.
func verifySchemaVersion(migrationsPath string, readApplied appliedSchemaStateReader) error {
	latest, err := LatestMigrationVersion(migrationsPath)
	if err != nil {
		return err
	}
	state, err := readApplied()
	if err != nil {
		return err
	}
	return CheckSchemaVersion(state, latest)
}
