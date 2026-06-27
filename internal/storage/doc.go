// Package storage provides file lifecycle management with retention-based auto-cleanup.
// Supports dry-run mode and multi-policy aggregation.
//
// Key contracts:
//   - filepath.Match uses glob patterns (not regex); complex patterns need pre-testing
//   - Exclusion list is exact string match (not pattern-based)
//   - First policy error halts Run(); subsequent policies will NOT execute
//   - Deletion errors are logged to stderr but do NOT fail the whole run
//   - Missing directories return 0 files (graceful, not an error)
//   - LastReport() returns any (requires type assertion to CleanupReport)
//   - Atomic writes are handled by ledger.SessionWriter; this module only cleans up
//
// Maturity: stable
package storage
