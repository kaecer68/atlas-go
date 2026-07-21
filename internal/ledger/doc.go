// Package ledger provides dual-backend (JSONL + SQLite) append-only
// persistence for trade outcomes, experiments, human interventions, and
// scorecard lifecycle.
//
// Core types:
//
//	Store           — JSONL default backend, append-only
//	OutcomeStore    — Unified interface for outcomes / experiments / interventions
//	FullStore       — Factory combining all ledger interfaces
//	SQLiteStore     — SQLite backend (WAL mode, foreign keys)
//	SessionWriter   — Atomic write (temp-dir → rename)
//	Archiver        — Auto gzip archival with expiration cleanup
//	SpawnRecord     — Agent spawn audit trail
//	WindowSplitter  — OOS validation: IS/OOS split + Sharpe trend
//
// Per-session layout (under sessions/<sessionID>/):
//
//	recommendation_outcomes.jsonl
//	screened_symbols.jsonl
//	trades.jsonl
//	summary.json
//	experiments.jsonl
//
// Plus global files at baseDir: recommendation_outcomes.jsonl,
// experiments.jsonl, human_interventions.jsonl.
//
// BuildScorecards() runs OOS validation when aggregating scorecards:
//
//	SortOutcomesByTime() → Split() into IS (first 2/3) and OOS (last 1/3)
//	→ compute IS/OOS Sharpe → ratio oos_sharpe / is_sharpe (< 1 = degradation)
//	→ rolling Sharpe linear regression (up/down/flat)
//	→ IsOOSDivergent() decides overfit_warning and oos_sample_warning
//
// When extending Scorecard fields, all four linked points must update
// together: domain.Scorecard, BuildScorecards(), window_splitter.go,
// sharpeTrendSlope(). The OOS contract is defined in
// docs/specs/domain-types-spec.md §4.
//
// Critical invariants:
//   - JSONL is one JSON object per line — never a JSON array
//   - append-only: never mutate existing records; readers dedupe
//   - RecordedAt is the calculation-completion timestamp, not the trading day
//     (parse SessionID for trading day, e.g. session-20260413-daily)
//   - LoadOutcomes() reads the global sparse file and MUST NOT be used to
//     compute a single session's OutcomeCount (that value comes from
//     GuardOutcomes in the current session)
//   - RecordSessionTrades silently skips empty slices (returns nil, no file)
//   - SQLiteSessionStore.LoadAllSessionScorecards is unimplemented
//     (returns nil, nil, nil — not for production queries)
//
// Maturity: stable
package ledger
