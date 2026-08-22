// Package baseline provides version-controlled baseline policy management
// for atlas-go: load, promote, revert, save, and runtime enforcement.
//
// Policy lifecycle:
//
//	Load     — Read data/state/baseline_policy.json (fall back to DefaultPolicy)
//	Promote  — Only domain.ExperimentAccepted results; auto-increment Version;
//	           supports risk_rule_change, portfolio_constraint_revision, and
//	           prompt overrides; history recorded in Policy.Promotions
//	Revert   — To last version, ToVersion, or ToExperiment; history recorded
//	           in Policy.RevertHistory
//	Save     — Updates LastUpdatedAt and writes back to JSON
//
// Core files:
//
//	policy.go    — Policy struct, defaults, load/save, Promote()
//	manager.go   — Manager facade coordinating experiment results and policy updates
//	rollback.go  — Revert(), version resolution, reconstructPolicyAtVersion
//	writeback.go — Charter A/B delta writeback (Phase C4 evolution loop):
//	               significant_enable → constraints, directional_watch →
//	               evidence-only watch promotion, inert/degenerate → finding
//
// Runtime enforcement: Trigger (trigger.go) subscribes to EventPositionUpdate
// and evaluates each position against current policy's simulation constraints.
// NewTrigger(manager, bus) + Start(ctx) wires it up. evaluate() performs three
// checks per position (stop-loss / take-profit / max holding days) and emits
// []Violation records. Violations are logged only — they never auto-close
// positions; downstream internal/live or human workflow decides the action.
//
// Trigger position source: internal/live/orchestrator.go publishes
// PublishPositionUpdate(..., "updated") from the critical handler in
// EventMarketSnapshot. This is the bridge between policy-as-backtest-constraint
// and policy-as-live-monitoring.
//
// Invariants:
//   - data/state/baseline_policy.json MUST NOT be edited directly; use
//     cmd/promote-baseline or cmd/revert-baseline
//   - Policy.Version is the single source of truth; Promotions and
//     RevertHistory must align with it
//   - experiment engines MUST call baseline.Load() first, otherwise the system
//     uses default constraints and results diverge from current state
//   - Promote and Revert MUST NOT ignore Save() errors (silent corruption)
//
// Maturity: stable
package baseline
