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
//	screened_symbols.jsonl        ← REJECTS ONLY (see below)
//	trades.jsonl
//	summary.json
//	experiments.jsonl
//
// The `screened_symbols.jsonl` NAME IS HISTORICAL AND MISLEADING: the file holds
// screening REJECTS only — it is written from []domain.ScreeningReject and a
// symbol that PASSED screening is never listed. "Not in the file" therefore means
// "not rejected", NOT "not screened". Renaming it was considered and rejected:
// historical session directories (100+ locally, more in production) already carry
// the old name, so a rename would need a compatibility reader, and until then it
// would silently make past audits unreadable. The semantics are pinned by
// TestScreenedSymbolsArtifact_IsRejectsOnly.
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
