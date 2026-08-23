// Package dailyreport generates automated daily market reports.
//
// Reports are triggered at 14:30 (market close + 30 min) and include:
//   - Global capital flow overview (bond, USD, JPY, VIX)
//   - Taiwan capital flow decomposition (7 forces + resonance)
//   - Event calendar (tomorrow's events)
//   - Strategy signals (recommended strategy + entry conditions)
//   - Risk warnings (stress index, drawdown alert)
//
// Output formats: JSON (API/MCP), Markdown (web display), optional HTML email.
//
// # Stateful workflow (2026-08-23)
//
// Reports are no longer one-shot artifacts. Each report carries a workflow
// state machine (see workflow.go):
//
//		generated ──(auto-generate)──▶ needs_review ──(revise)──▶ corrected
//		                                    │                         │
//		                                    └─────(approve)───────────┴──▶ approved
//
//	  - Generate() stamps fresh reports with WorkflowNeedsReview.
//	  - POST /api/reports/{date}/revise applies a human correction (whitelisted
//	    Strategy/Period/Risk fields only), appends a RevisionEntry to
//	    RevisionHistory, sets WorkflowCorrected and overwrites the persisted
//	    YYYY-MM-DD.json (history stays inside the file). The corrected report
//	    is the version downstream consumers should use.
//	  - POST /api/reports/{date}/approve approves without revision
//	    (WorkflowApproved). Both endpoints require ATLAS_API_KEY admin auth
//	    (same guard as cmd/atlas wrapAdminAuth).
//
// All workflow fields are omitempty, so legacy persisted reports stay
// byte-compatible.
//
// # Cross-day claim tracking (2026-08-23)
//
// Each generated report emits up to three tracked claims — strategy
// recommendation, period call, risk warning — persisted to
// <ledgerDir>/report_tracked_claims.jsonl (see tracker.go). Claims carry an
// independent state machine:
//
//	tracking ──(verify after 5 trading days)──▶ verified | expired
//
// A background task (report_tracker_verify, 15:00 Taipei) evaluates due
// claims against realized data (recommendation outcomes or replay prices).
// GET /api/reports/tracked-claims lists tracking state.
//
// Maturity: evolving
package dailyreport
