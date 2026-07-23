// Package strategy_ranker provides strategy ranking and tier assignment
// based on backtest performance metrics (Sharpe ratio, max drawdown, win rate).
//
// It consumes StrategyReport from internal/strategy_validator and produces
// ranked, tier-tagged output suitable for the recommendation engine.
//
// Maturity: evolving
package strategy_ranker
