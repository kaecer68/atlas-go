// Package experiment implements the mutation lifecycle for the Atlas-Go
// evolution loop: Propose → Execute → Judge → Promote/Revert.
//
// Key components:
//
//	Executor    — Runs a mutation brief against a replay window
//	Judge       — Evaluates experiment results vs baseline policy
//	Promote     — Accepts winning experiments into baseline
//
// The experiment flow is the core of the OpenClaw training loop:
//  1. Select weakest agent from scorecard
//  2. Propose mutation (prompt tightening, rule change, or constraint revision)
//  3. Execute on unseen replay window
//  4. Judge against baseline using Sharpe, drawdown, hit rate
//  5. Promote if accepted, revert if rejected
//
// Safety rails:
//   - Minimum observation threshold (n≥10) before promotion
//   - Baseline must be loaded before experiment
//   - Each experiment is isolated; no cross-session contamination
//
// Maturity: stable
package experiment
