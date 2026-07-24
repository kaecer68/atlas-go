// Package strategy_ranker provides strategy validation, ranking and tier
// assignment based on backtest performance metrics (Sharpe ratio, max drawdown,
// win rate, TAIEX correlation).
//
// It consumes raw backtest results and produces StrategyReport values, then
// ranks and tags them with tiers (free / registered / premium) suitable for
// the recommendation engine.
//
// Maturity: evolving
package strategy_ranker
