// Package db provides PostgreSQL connection management and schema migrations.
//
// Init() is the single entry point for establishing a pgxpool and applying
// pending migrations via golang-migrate. On migration failure the pool is
// closed before returning to prevent connection leaks; migrate.ErrNoChange is
// treated as success.
//
// Configuration:
//   - DATABASE_URL env var is used as fallback when the Init() argument is empty
//   - Migration source path must be a file:// absolute path (golang-migrate
//     driver-specific convention)
//   - The driver URL prefix postgres:// is rewritten to pgx5:// for golang-migrate
//
// Schema version drift guardrail (schema_version.go):
//   - VerifySchemaVersion() compares the version golang-migrate recorded in
//     schema_migrations with the newest migration file on disk and returns a
//     *SchemaDrift (see IsSchemaVersionDrift) when the database lags the code or
//     schema_migrations is dirty.
//   - The check is advisory: callers own the policy. bootstrap logs WARN and
//     records atlas_schema_version_drift_total but never fails startup.
//   - LatestMigrationVersion()/CheckSchemaVersion() are the pure, testable
//     halves; verifySchemaVersion() takes an injected applied-state reader.
//
// Maturity: evolving
package db
