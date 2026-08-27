// Package testdb centralizes one DATABASE_URL policy for PostgreSQL
// integration tests so packages stop hardcoding DSNs (k3 audit M6).
//
// URL/Require/Connect/Pool read DATABASE_URL only: a missing or unreachable
// database fails loudly in CI (os.Getenv("CI") set) and skips locally, so a
// broken CI postgres service can no longer produce a fake green light.
//
// Maturity: utility
package testdb
