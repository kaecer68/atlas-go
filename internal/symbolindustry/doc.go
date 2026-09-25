// Package symbolindustry persists the per-stock industry field produced by the
// first-party symbol_industry data channel (issue #1943).
//
// Maturity: experimental
//
// The field is what lets industry-level statistics (sector exposure,
// SmartUniverse population, coverage audits) run on the whole listed market
// instead of the hard-coded ~27 representative stocks. Entries are keyed by
// normalized symbol (no ".TW" suffix) and carry the canonical L1 sector plus
// the upstream code disposition; rows whose upstream code has no defensible
// canonical target keep an empty CanonicalL1 and are reported rather than
// imputed.
//
// Backend split mirrors internal/channelsecrets: Postgres is the production
// source of truth (backend=postgres + injected pool), everything else uses the
// job-local SQLite artifact under <workDir>/data/state for dev/CLI.
//
// Promotion criteria (→ stable): one observation window with the gate on,
// zero invariant violations, and a schema that has not moved.
package symbolindustry
