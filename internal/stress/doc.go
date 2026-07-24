// Package stress provides stress test scenarios with RunScenario for
// scenario-based risk evaluation.
//
// RunScenario applies a synthetic market perturbation to a baseline
// portfolio snapshot and returns the resulting portfolio state for
// downstream risk evaluation. Scenarios are configurable via the
// StressScenario struct (magnitude, duration, recovery shape).
//
// Differences from cmd/stress-test/internal/risktest:
//   - risktest: multi-scenario batch execution via cmd/stress-test
//   - stress: single-scenario runtime API used by live risk evaluation
//
// Maturity: evolving
package stress
