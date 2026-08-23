// Package taiwanholidays is the single source of truth for Taiwan trading-day
// classification (P1-8). It merges the previously duplicated lunar/statutory
// holiday tables from internal/marketdata and internal/industry into one
// package that both import, eliminating drift.
//
// Coverage: 2023-2040 (statutory fixed dates + lunar holidays + weekends).
// Beyond 2040 it falls back to weekend-only classification with a warning log.
//
// Maturity: utility
package taiwanholidays
