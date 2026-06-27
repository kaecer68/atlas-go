package feature

// Maturity: evolving
//
// Package feature provides named feature extraction from market data bars.
// It is shared between cmd/backtest-pipeline (CLI) and internal/experiment
// (Judge evaluation — PermutationImportance).
//
// Pitfall: MakeExtractor is a public factory. Changing its signature requires
// synchronizing both consumers (cmd/backtest-pipeline and experiment.Judge).
