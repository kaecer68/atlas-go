// Package acceptance provides a pluggable framework for experiment acceptance
// gates. Gates are Evaluators that can be registered with a Pipeline; the
// Pipeline runs them in order and short-circuits on first failure.
//
// The framework is the bridge target for migrating the hard-coded switch
// in experiment/judge.go passesAcceptance into composable, testable
// units. The existing judge logic is preserved via a feature flag
// (UseAcceptancePipeline) so that old and new paths can be A/B tested
// during the migration window.
//
// Maturity: evolving
package acceptance
