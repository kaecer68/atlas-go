// Package reporting provides report generation: Markdown, ASCII charts,
// and agent performance tables for the dashboard and CLI outputs.
//
// Output formats:
//
//	Markdown     — Human-readable reports for docs/ and PR descriptions
//	ASCII chart  — Terminal-friendly sparkline/bar visualizations
//	HTML table   — For dashboard inline rendering
//	JSON         — Machine-readable for downstream automation
//
// Report types:
//
//	AgentPerformance    — Per-agent Sharpe, hit rate, drawdown
//	BacktestSummary     — IS/OOS comparison, factor contributions
//	DailyPnlReport      — Per-day PnL with attribution to factors/agents
//	RiskSnapshotReport  — VaR, drawdown, position concentration
//
// Reports are pure: they take structured data and return formatted strings,
// no I/O. The caller handles file writing.
//
// Maturity: evolving
package reporting
