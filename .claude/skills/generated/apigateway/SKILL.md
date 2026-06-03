---
name: apigateway
description: "Skill for the Apigateway area of atlas. 194 symbols across 10 files."
---

# Apigateway

194 symbols | 10 files | Cohesion: 75%

## When to Use

- Understanding the unified data ingestion pipeline (Gateway, BackgroundTaskManager, ChannelAdapters)
- Adding a new data provider (must implement a ChannelAdapter)
- Registering a new scheduled background task
- Debugging rate limiting, circuit breakers, or channel health
- Working with the 14-channel unified data collection system

## Core Architecture

```
Market Data Source → ChannelAdapter (thin wrapper) → Gateway (rate limit + circuit breaker + cache)
                                                          ↓
                                              BackgroundTaskManager (scheduled fetch)
                                                          ↓
                                              Consumer (orchestrator/dashboard)
```

## Key Files

| File | Symbols |
|------|---------|
| `internal/apigateway/gateway.go` | NewGateway, RegisterChannel, Fetch, ChannelStatus, HasChannel, Start/Stop (+7) |
| `internal/apigateway/channel_adapters.go` | RegisterChannelAdapters, 14 ChannelAdapter types (Fugle, FinMind, TWSE, TEJ, etc.) |
| `internal/apigateway/background.go` | BackgroundTaskManager, ScheduledTask, RetryPolicy, MarketHoursOnly, WrapChannelTask, executeWithRetry (+10) |
| `internal/apigateway/limits.go` | RateLimitManager with hardcoded limits for all 14 channels |
| `internal/apigateway/cache.go` | CacheLayer for fetch response caching |
| `internal/apigateway/health.go` | UnifiedHealthStore for channel health tracking |

## Entry Points

Start here when exploring this area:

- **`NewGateway`** (Function) — `internal/apigateway/gateway.go:120` — creates the unified data gateway
- **`RegisterChannelAdapters`** (Function) — `internal/apigateway/channel_adapters.go:1123` — registers all 14 channel adapters
- **`NewBackgroundTaskManager`** (Function) — `internal/apigateway/background.go:156` — creates task manager for scheduled data collection
- **`WrapChannelTask`** (Function) — `internal/apigateway/background.go:17` — generic wrapper for channel-driven BTM tasks

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `NewGateway` | Function | `internal/apigateway/gateway.go` | 120 |
| `RegisterChannel` | Method | `internal/apigateway/gateway.go` | ~180 |
| `Fetch` | Method | `internal/apigateway/gateway.go` | ~220 |
| `ChannelStatus` | Method | `internal/apigateway/gateway.go` | ~260 |
| `HasChannel` | Method | `internal/apigateway/gateway.go` | ~290 |
| `Start` | Method | `internal/apigateway/gateway.go` | ~310 |
| `RegisterChannelAdapters` | Function | `internal/apigateway/channel_adapters.go` | 1123 |
| `NewBackgroundTaskManager` | Function | `internal/apigateway/background.go` | 156 |
| `Register` | Method | `internal/apigateway/background.go` | 163 |
| `Start` | Method | `internal/apigateway/background.go` | ~200 |
| `Status` | Method | `internal/apigateway/background.go` | ~250 |
| `WrapChannelTask` | Function | `internal/apigateway/background.go` | 17 |

## 14 ChannelAdapters

| Channel | Provider | Condition |
|---------|----------|-----------|
| `fugle` | NewFugleClient | FUGLE_API_KEY set |
| `fubon` | NewFubonClient | FUBON_API_KEY set |
| `finmind` | NewFinMindClient | FINMIND_API_KEY set |
| `twse_replay` | NewTWSEClient | always |
| `us_yahoo` | NewYahooFinanceMacroProvider | YAHOO_ENABLED |
| `twse_capital_flow` | NewTWSECapitalFlowProvider | always |
| `twse_margin` | NewTWSEMarginBalanceProvider | always |
| `export_statistics` | NewExportStatisticsProvider | always |
| `tej` | NewTEJClient | TEJ_API_KEY set |
| `geopolitical` | NewCompositeGeopoliticalProvider | always |
| `frankfurter_fx` | NewFrankfurterFXProvider | always |
| `tsmc_revenue` | NewTSMCRevenueProvider | always |
| `geopolitical_taiwan` | NewTaiwanRSSGeopoliticalProvider | always |
| `janus_regime` | janus.Engine | janusEngine != nil |

## BackgroundTaskManager Features

- **10+ scheduled tasks** registered in `cmd/atlas/main.go run()` (macro_ingest, metrics_snapshot, autobacktest, capital_flow, margin, export, geopolitical, etc.)
- **MarketHoursOnly** guard — tasks constrained to TWSE operating hours (09:00-13:30, Mon-Fri)
- **RetryPolicy** — exponential backoff with configurable MaxAttempts, InitialDelay, MaxDelay, Multiplier
- **WrapChannelTask[T]** — generic wrapper converting `<-chan T` to `BackgroundTaskFunc`
- **TaskFailureHandler** — per-task and global callbacks on consecutive failures

## Development Rules

1. **ALL new data sources** MUST go through Gateway. Implement a ChannelAdapter and register via `RegisterChannelAdapters()`.
2. **No direct `New*Provider()` calls** in new code. Use `gateway.Fetch(channelID)` instead.
3. **Background tasks** above 30-second intervals should use `taskMgr.Register()`, not raw goroutine+ticker.
4. **Rate limits** for each channel are defined in `internal/apigateway/limits.go`.
5. **Circuit breaker** thresholds are per-channel in `internal/apigateway/circuit_breaker.go`.

## Connected Areas

| Area | Connection |
|------|-----------|
| cmd/atlas/main.go | BTM task registration |
| internal/monitoring/ | Dashboard status display, channel health |
| internal/marketdata/ | Data provider implementations wrapped by adapters |
| internal/narrative/ | Geopolitical channel adapters |
| internal/janus/ | JANUS regime channel adapter |
