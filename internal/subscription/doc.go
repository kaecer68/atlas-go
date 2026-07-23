// Package subscription provides a lightweight user subscription system
// with tier-based access control (free / registered / premium).
//
// Design principles:
//   - SQLite-backed user store (modernc.org/sqlite, already a dependency)
//   - HMAC-JWT for session tokens (stdlib only, no external JWT dependency)
//   - Middleware-pattern tier validation: ValidateTier(minTier)
//   - 7-day premium trial for new registered users
//
// Maturity: evolving
package subscription
