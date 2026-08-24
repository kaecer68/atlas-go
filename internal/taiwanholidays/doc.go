// Package taiwanholidays is the single source of truth for Taiwan public
// holidays and trading-day classification (P1-8). It merges the previously
// duplicated lunar/statutory holiday tables from internal/marketdata and
// internal/industry into one package that both import, eliminating drift.
//
// Coverage: 2023-2040 (fixed statutory dates + verified lunar dates).
// Beyond 2040 it falls back to conventional lunar dates with a one-time
// warning log per year, so callers degrade gracefully instead of panicking.
//
// Maturity: utility
package taiwanholidays
