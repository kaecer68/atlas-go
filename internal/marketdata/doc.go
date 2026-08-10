// Package marketdata provides market data provider abstraction for Taiwan
// equities, with hybrid failover and aggregate corporate action reconciliation.
//
// Core interfaces:
//
//	Provider                — Single-stock quotes via GetQuotes
//	MacroDataProvider       — Macro indicators via FetchSnapshot
//	CorporateActionProvider — Dividend / split events via GetCorporateActions
//
// Production providers:
//
//	FugleProvider            — Real-time quotes (30 req/min — conservative free-tier limit, API key required)
//	TWSEOpenAPIProvider       — Daily quotes (no key, rate-limited)
//	HybridProvider            — Fugle primary, TWSE fallback on price=0 (default path)
//	TWSECapitalFlowProvider   — Foreign investor net buy/sell from T86 report
//	YahooMacroProvider        — US bonds, DXY, VIX
//	CompositeMacroProvider    — Last-write-wins merge of multiple macro sources
//	BDIProvider               — Baltic Dry Index via CNBC (5s rate limit)
//
// AggregatedCorporateActionProvider is the preferred CorporateActionProvider:
// TWSE is primary (includes ex-dividend reference price), FinMind is fallback.
// Dedup rule: same (symbol, ex_date) prefers TWSE; if TWSE is missing a field
// that FinMind has, fill it from FinMind. Partial failure is OK — only
// "both failed" returns an error. Output is sorted by ExDate ascending.
//
// ETF NAV has no realtime channel. TWSEETFNAVScraper uses a tiered strategy:
// Tier 1 (TWSE scrape) is a stub; Tier 2 (close price proxy) is the only
// working path. Tracking error typically < 0.5%.
//
// Conventions:
//   - All external requests must use x/time/rate client-side rate limiting
//   - Errors wrap HTTP status + endpoint context for diagnostics
//   - Taiwan data parsing aligns to CST (UTC+8)
//
// providerBreaker provides per-provider circuit breaking. Adding a new
// breaker requires (1) construct providerBreaker, (2) register in
// HybridProvider.breakers, (3) call shouldTry() + recordSuccess/recordFailure
// in the corresponding GetQuotes path.
//
// Maturity: stable
package marketdata
