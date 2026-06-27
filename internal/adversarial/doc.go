// Package adversarial provides adversarial training: AdversarialTrainer,
// BattleResult, and stress tests for hardening agent robustness.
//
// AdversarialTrainer runs two agents in opposition: one tries to maximize
// returns under perturbations, the other tries to find scenarios that break
// the first agent. BattleResult captures the win/loss outcome.
//
// Use cases:
//   - Hardening prompt-based agents against adversarial market scenarios
//   - Generating edge cases for backtest validation
//   - Evaluating agent robustness to distribution shift
//
// Maturity: experimental
// X-tier — do not depend on this from stable/evolving modules.
package adversarial
