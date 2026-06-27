// Package logging provides a unified structured logging interface wrapping
// log/slog, with context propagation and field helper functions.
//
// Core symbols:
//
//	logger          — Package-level *slog.Logger (mu-protected)
//	ctxKey          — context.Context key for logger propagation
//	Component / Event / Symbol / SessionID / AgentID / DurationMs —
//	                 structured field helpers (return slog.Attr)
//	FStr / FInt / FFloat64 / FBool — generic slog field helpers
//	Err             — Wrap error as slog field; nil returns nil (no "error":"nil")
//
// Lifecycle:
//
//	init()          — text handler on stderr (Info level, default)
//	Init()          — Switch to JSON handler, adjust level
//	WithLogger(ctx, l) — Inject logger into context
//	FromContext(ctx)   — Extract logger; fall back to slog.Default() if absent
//
// Cautions:
//   - FromContext never returns nil (slog.Default fallback guarantees usability)
//   - Info() / Error() read the package-level logger, not context. For per-
//     request logging, use InfoContext() / ErrorContext()
//   - Critical uses a custom slog level 12 (above Error); external parsers
//     may not recognize it
//   - Err(nil) returns nil — no "error":"nil" string emitted
//   - SetLogContext is package-level (affects all callers globally). For
//     per-request isolation, use WithLogger
//   - LegacyLog is compatibility-only (outputs at info level with "component"
//   - "message" fields); new code should avoid it
//
// Maturity: stable
package logging
