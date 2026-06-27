// Package calibration provides pure logic for inferring and calibrating
// algorithmic parameters from historical returns and agent outcomes.
//
// This package is intentionally pure (no I/O, no global state) so it can
// be tested exhaustively and called from anywhere in the pipeline. It is
// the engine behind cmd/calibrate-parameters which produces the parameters.json
// recommendations that humans then review.
//
// Calibration entry points:
//
//	InferFromReturns(returns)    → Sharpe, max drawdown, volatility
//	InferFromOutcomes(outcomes)   → hit rate, avg conviction, win/loss ratio
//	RecommendUpdates(current, observations) → ParameterMetadata diff
//
// All recommendations include Rationale and Source fields per the
// ParametersConfig contract (see docs/PARAMETER_SYSTEM.md).
//
// Maturity: utility
package calibration
