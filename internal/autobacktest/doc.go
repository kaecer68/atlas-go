// Package autobacktest provides scheduled background backtest tasks for
// continuous model validation.
//
// autobacktest runs Runner.Run on a schedule (cron-style interval from
// ParametersConfig.Autobacktest) and pushes results to the ledger for
// Scorecard aggregation. It is the production path for catching silent
// model drift between major releases.
//
// Scheduled flow:
//
//	tick (interval)
//	  → fetch latest replay window
//	  → Runner.Run
//	  → ledger.BuildScorecards()
//	  → diff vs previous scorecard
//	  → emit EventDriftDetected if Sharpe delta > threshold
//
// Drift alerts are consumed by monitoring.DriftDetector v2
// (see docs/specs/sim-engine-spec.md for the integration contract).
//
// Maturity: evolving
package autobacktest
