---
name: marketdata
description: "Skill for the Marketdata area of atlas. 60 symbols across 15 files."
---

# Marketdata

60 symbols | 15 files | Cohesion: 80%

## When to Use

- Working with code in `internal/`
- Understanding how TestHybridProvider_NoAPIKey, TestHybridProvider_WithAPIKey, TestHybridProvider_Reset work
- Modifying marketdata-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/marketdata/hybrid_provider.go` | NewHybridProvider, Name, Reset, UseTWSE, UseFugle (+6) |
| `internal/marketdata/marketdata_test.go` | TestHybridProvider_NoAPIKey, TestHybridProvider_WithAPIKey, TestHybridProvider_Reset, TestHybridProvider_UseTWSE_UseFugle, TestHybridProvider_hasInvalidQuotes (+5) |
| `internal/marketdata/fugle_client.go` | GetClient, NewFugleClient, GetQuote, GetQuotes, NewFugleProviderWithAPIKey (+3) |
| `internal/marketdata/twse_openapi.go` | NewTWSEClient, GetQuotes, GetQuote, GetQuotesBySymbols, CheckMarketStatus (+1) |
| `internal/marketdata/export_provider.go` | NewExportStatisticsProvider, Name, FetchSnapshot, mockSnapshot |
| `internal/marketdata/twse_capital_flow_provider.go` | NewTWSECapitalFlowProvider, Name, FetchSnapshot, parseTWDVolume |
| `internal/marketdata/fugle_client_test.go` | TestFugleClient_GetQuote_Success, TestFugleClient_GetQuote_NonOK, TestFugleClient_GetQuotes, TestNewFugleProviderWithAPIKey |
| `internal/marketdata/export_provider_test.go` | TestExportStatisticsProvider_Name, TestExportStatisticsProvider_FetchSnapshot, TestExportStatisticsProvider_MockSnapshot |
| `internal/marketdata/twse.go` | NewTWSEProvider, Name, GetQuotes |
| `internal/marketdata/twse_capital_flow_provider_test.go` | TestNewTWSECapitalFlowProvider, TestParseTWDVolume |

## Entry Points

Start here when exploring this area:

- **`TestHybridProvider_NoAPIKey`** (Function) — `internal/marketdata/marketdata_test.go:44`
- **`TestHybridProvider_WithAPIKey`** (Function) — `internal/marketdata/marketdata_test.go:54`
- **`TestHybridProvider_Reset`** (Function) — `internal/marketdata/marketdata_test.go:64`
- **`TestHybridProvider_UseTWSE_UseFugle`** (Function) — `internal/marketdata/marketdata_test.go:89`
- **`NewHybridProvider`** (Function) — `internal/marketdata/hybrid_provider.go:20`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestHybridProvider_NoAPIKey` | Function | `internal/marketdata/marketdata_test.go` | 44 |
| `TestHybridProvider_WithAPIKey` | Function | `internal/marketdata/marketdata_test.go` | 54 |
| `TestHybridProvider_Reset` | Function | `internal/marketdata/marketdata_test.go` | 64 |
| `TestHybridProvider_UseTWSE_UseFugle` | Function | `internal/marketdata/marketdata_test.go` | 89 |
| `NewHybridProvider` | Function | `internal/marketdata/hybrid_provider.go` | 20 |
| `NewTWSEClient` | Function | `internal/marketdata/twse_openapi.go` | 44 |
| `TestHybridProvider_hasInvalidQuotes` | Function | `internal/marketdata/marketdata_test.go` | 103 |
| `TestTWSEClient_GetQuotes_Success` | Function | `internal/marketdata/marketdata_test.go` | 128 |
| `TestTWSEClient_GetQuotes_NonOKStatus` | Function | `internal/marketdata/marketdata_test.go` | 173 |
| `TestTWSEClient_GetQuotesBySymbols` | Function | `internal/marketdata/marketdata_test.go` | 188 |
| `TestExportStatisticsProvider_Name` | Function | `internal/marketdata/export_provider_test.go` | 8 |
| `TestExportStatisticsProvider_FetchSnapshot` | Function | `internal/marketdata/export_provider_test.go` | 15 |
| `TestExportStatisticsProvider_MockSnapshot` | Function | `internal/marketdata/export_provider_test.go` | 39 |
| `NewExportStatisticsProvider` | Function | `internal/marketdata/export_provider.go` | 18 |
| `TestNewTWSECapitalFlowProvider` | Function | `internal/marketdata/twse_capital_flow_provider_test.go` | 24 |
| `NewTWSECapitalFlowProvider` | Function | `internal/marketdata/twse_capital_flow_provider.go` | 32 |
| `NewTWSEProvider` | Function | `internal/marketdata/twse.go` | 11 |
| `TestTWSEProvider_Name` | Function | `internal/marketdata/marketdata_test.go` | 15 |
| `TestTWSEProvider_GetQuotes` | Function | `internal/marketdata/marketdata_test.go` | 22 |
| `TestFugleClient_GetQuote_Success` | Function | `internal/marketdata/fugle_client_test.go` | 10 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → ConvertToQuote` | cross_community | 6 |
| `Main → GetQuote` | cross_community | 6 |
| `Main → FugleClient` | cross_community | 5 |
| `Main → TWSECapitalFlowProvider` | cross_community | 5 |
| `Main → Close` | cross_community | 4 |
| `Main → FugleProvider` | cross_community | 4 |
| `Main → TWSEClient` | cross_community | 4 |
| `Main → TWSEClient` | cross_community | 4 |
| `Main → GetQuote` | cross_community | 4 |
| `Main → GetQuotesBySymbols` | cross_community | 4 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Live | 3 calls |
| Config | 1 calls |
| Daily-replay-sync | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestHybridProvider_NoAPIKey"})` — see callers and callees
2. `gitnexus_query({query: "marketdata"})` — find related execution flows
3. Read key files listed above for implementation details
