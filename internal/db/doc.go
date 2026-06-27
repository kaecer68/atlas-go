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
// Maturity: evolving
package db
