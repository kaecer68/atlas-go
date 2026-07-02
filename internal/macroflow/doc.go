// Package macroflow computes macro regime-based factor weight adjustments.
// The Engine takes a MacroDataSnapshot and a RiskLevel, evaluates 6 rules
// (Yellow/Orange/Red × Calm/Stress), and returns combined percentage deltas
// for Defensive, Aggressive, and Cash allocation tiers.
//
// Consumed by internal/orchestrator as the 7th pipeline strategy step
// (after WeightApplication, before ControlLayer). API may adjust as the
// macro→portfolio wiring matures.

// Maturity: evolving

package macroflow
