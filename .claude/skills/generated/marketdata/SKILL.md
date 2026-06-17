---
name: marketdata
description: "Skill for the Marketdata area of atlas. 109 symbols across 27 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Marketdata

109 symbols | 27 files | Cohesion: 72%

## When to Use

- Working with code in `internal/`
- Understanding how TestTEJClient_GetStockPriceDaily_Success, TestTEJClient_GetStockPriceDaily_APIError, TestTEJClient_GetStockPriceDaily_EmptyResponse work
- Modifying marketdata-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/marketdata/hybrid_provider.go` | defaultCircuitBreakerConfig, NewHybridProvider, Name, UseTWSE, UseFugle (+13) |
| `internal/marketdata/marketdata_test.go` | TestHybridProvider_NoAPIKey, TestHybridProvider_WithAPIKey, TestHybridProvider_UseTWSE_UseFugle, TestTWSEClient_GetQuotes_Success, TestTWSEClient_GetQuotes_NonOKStatus (+6) |
| `internal/marketdata/tej_provider_test.go` | TestTEJClient_GetStockPriceDaily_Success, TestTEJClient_GetStockPriceDaily_APIError, TestTEJClient_GetStockPriceDaily_EmptyResponse, TestTEJClient_GetFinancialStatements, TestTEJClient_RateLimiter (+5) |
| `internal/marketdata/fugle_client.go` | GetClient, NewFugleClient, GetQuote, GetQuotes, NewFugleProviderWithAPIKey (+3) |
| `internal/marketdata/tej_provider.go` | GetStockPriceDaily, NewTEJClient, Ping, GetFinancialStatements, toFloat64 (+2) |
| `internal/marketdata/twse_capital_flow_provider.go` | fetchLatestTradingDay, fetchDate, parseTWDVolume, NewTWSECapitalFlowProvider, Name (+2) |
| `internal/marketdata/twse_openapi.go` | NewTWSEClient, GetQuotes, GetQuote, GetQuotesBySymbols, CheckMarketStatus (+1) |
| `internal/marketdata/tsmc_revenue_provider.go` | fetchLatestMonth, fetchMonth, FetchSnapshot, saveRevenue, TSMCRevenueProviderWithClient |
| `internal/marketdata/twse.go` | NewMockProvider, Name, GetQuotes, IsMock |
| `internal/marketdata/export_provider.go` | NewExportStatisticsProvider, Name, FetchSnapshot, ExportStatisticsProviderWithClient |

## Entry Points

Start here when exploring this area:

- **`TestTEJClient_GetStockPriceDaily_Success`** (Function) — `internal/marketdata/tej_provider_test.go:11`
- **`TestTEJClient_GetStockPriceDaily_APIError`** (Function) — `internal/marketdata/tej_provider_test.go:50`
- **`TestTEJClient_GetStockPriceDaily_EmptyResponse`** (Function) — `internal/marketdata/tej_provider_test.go:65`
- **`TestRedisNonceReplayStoreRejectsReplayAcrossInstances`** (Function) — `internal/live/nonce_store_test.go:96`
- **`TestRedisNonceReplayStoreAllowsReuseAfterTTL`** (Function) — `internal/live/nonce_store_test.go:124`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestTEJClient_GetStockPriceDaily_Success` | Function | `internal/marketdata/tej_provider_test.go` | 11 |
| `TestTEJClient_GetStockPriceDaily_APIError` | Function | `internal/marketdata/tej_provider_test.go` | 50 |
| `TestTEJClient_GetStockPriceDaily_EmptyResponse` | Function | `internal/marketdata/tej_provider_test.go` | 65 |
| `TestRedisNonceReplayStoreRejectsReplayAcrossInstances` | Function | `internal/live/nonce_store_test.go` | 96 |
| `TestRedisNonceReplayStoreAllowsReuseAfterTTL` | Function | `internal/live/nonce_store_test.go` | 124 |
| `NewRedisNonceReplayStore` | Function | `internal/live/nonce_store.go` | 116 |
| `TestHybridProvider_NoAPIKey` | Function | `internal/marketdata/marketdata_test.go` | 49 |
| `TestHybridProvider_WithAPIKey` | Function | `internal/marketdata/marketdata_test.go` | 59 |
| `TestHybridProvider_UseTWSE_UseFugle` | Function | `internal/marketdata/marketdata_test.go` | 94 |
| `NewHybridProvider` | Function | `internal/marketdata/hybrid_provider.go` | 59 |
| `NewTWSEClient` | Function | `internal/marketdata/twse_openapi.go` | 44 |
| `TestTWSEClient_GetQuotes_Success` | Function | `internal/marketdata/marketdata_test.go` | 142 |
| `TestTWSEClient_GetQuotes_NonOKStatus` | Function | `internal/marketdata/marketdata_test.go` | 187 |
| `TestTWSEClient_GetQuotesBySymbols` | Function | `internal/marketdata/marketdata_test.go` | 202 |
| `TestParseTWDVolume` | Function | `internal/marketdata/twse_capital_flow_provider_test.go` | 6 |
| `NewMockProvider` | Function | `internal/marketdata/twse.go` | 11 |
| `TestMockProvider_Name` | Function | `internal/marketdata/marketdata_test.go` | 13 |
| `TestMockProvider_GetQuotes` | Function | `internal/marketdata/marketdata_test.go` | 20 |
| `TestMockProvider_IsMock` | Function | `internal/marketdata/marketdata_test.go` | 40 |
| `TestTEJClient_GetFinancialStatements` | Function | `internal/marketdata/tej_provider_test.go` | 125 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleRecommendationPipeline → Close` | cross_community | 5 |
| `Main → TWSECapitalFlow` | cross_community | 5 |
| `Main → Close` | cross_community | 5 |
| `Main → ParseTWDVolume` | cross_community | 5 |
| `RunAutoCapitalFlowFetchOnStartup → TWSECapitalFlow` | cross_community | 5 |
| `RunAutoCapitalFlowFetchOnStartup → Close` | cross_community | 5 |
| `RunAutoCapitalFlowFetchOnStartup → ParseTWDVolume` | cross_community | 5 |
| `Main → Close` | cross_community | 4 |
| `Main → Close` | cross_community | 4 |
| `Main → Close` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 7 calls |
| Industry | 4 calls |
| Config | 1 calls |
| Daily-replay-sync | 1 calls |
| Swarm | 1 calls |
| Prism | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestTEJClient_GetStockPriceDaily_Success"})` — see callers and callees
2. `gitnexus_query({query: "marketdata"})` — find related execution flows
3. Read key files listed above for implementation details
