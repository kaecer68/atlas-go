---
name: narrative
description: "Skill for the Narrative area of atlas. 148 symbols across 35 files."
---

# Narrative

148 symbols | 35 files | Cohesion: 74%

## When to Use

- Working with code in `internal/`
- Understanding how QuotesToNarrativeData, TestNarrativeEngineMatchChains, TestNarrativeEngineActiveModels work
- Modifying narrative-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/narrative/knowledge_base.go` | NewNarrativeEngine, MatchChains, ActiveModels, DetectEvents, detectUSRatesEvent (+17) |
| `internal/narrative/ingestor.go` | hitRateForTheme, detectEventsFromSnapshot, detectRetailDivergenceEventFromSnapshot, detectUSRatesEventFromSnapshot, detectJPYCarryUnwindEventFromSnapshot (+11) |
| `internal/narrative/report_generator_test.go` | TestGenerateDailySummary_DateMatches, TestGenerateDailySummary_SectionsNonEmpty, TestGenerateDailySummary_NarrativeEventsMentioned, TestGenerateDailySummary_TopPicks, TestGenerateDailySummary_TopPicksCapped (+8) |
| `internal/narrative/macro_assessment.go` | String, Assess, evaluateRiskFactors, calculateOutflowProbability, determinePrimaryFlow (+6) |
| `internal/monitoring/api/narrative/handlers.go` | writeJSON, writeJSONError, parseFloatQuery, HandleNarrativeEvents, HandleNarrativeChains (+3) |
| `internal/narrative/narrative_test.go` | TestNarrativeEngineMatchChains, TestNarrativeEngineActiveModels, TestNarrativeEngineDetectEvents, TestDetectEventsNoTrigger, TestKnowledgeBaseDefaults (+2) |
| `internal/narrative/divergence.go` | Update, RetailDivergenceAndMarginZScore, NewDivergenceDetector, mean, stddev |
| `internal/narrative/taiwan_geopolitical_provider_test.go` | TestTaiwanRSSGeopoliticalProvider_Name, TestTaiwanRSSGeopoliticalProvider_Feeds, TestTaiwanRSSGeopoliticalProvider_Keywords, TestCompositeTaiwanGeopoliticalProvider_Name, TestCompositeTaiwanGeopoliticalProvider_FetchScore |
| `internal/narrative/taiwan_geopolitical_provider.go` | NewTaiwanRSSGeopoliticalProvider, Name, NewCompositeTaiwanGeopoliticalProvider, Name, FetchScore |
| `cmd/experimental/validate-narrative-shock/main.go` | main, quotesToData, attachNarrative, printRun |

## Entry Points

Start here when exploring this area:

- **`QuotesToNarrativeData`** (Function) — `internal/orchestrator/system.go:459`
- **`TestNarrativeEngineMatchChains`** (Function) — `internal/narrative/narrative_test.go:96`
- **`TestNarrativeEngineActiveModels`** (Function) — `internal/narrative/narrative_test.go:108`
- **`NewNarrativeEngine`** (Function) — `internal/narrative/knowledge_base.go:105`
- **`TestNarrativeEngineDetectEvents`** (Function) — `internal/narrative/narrative_test.go:65`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `QuotesToNarrativeData` | Function | `internal/orchestrator/system.go` | 459 |
| `TestNarrativeEngineMatchChains` | Function | `internal/narrative/narrative_test.go` | 96 |
| `TestNarrativeEngineActiveModels` | Function | `internal/narrative/narrative_test.go` | 108 |
| `NewNarrativeEngine` | Function | `internal/narrative/knowledge_base.go` | 105 |
| `TestNarrativeEngineDetectEvents` | Function | `internal/narrative/narrative_test.go` | 65 |
| `TestDetectEventsNoTrigger` | Function | `internal/narrative/narrative_test.go` | 141 |
| `TestDashboardMacroRoutes` | Function | `internal/monitoring/dashboard_api_test.go` | 462 |
| `NewYahooFinanceMacroProvider` | Function | `internal/marketdata/yahoo_macro_provider.go` | 22 |
| `NewCompositeMacroProvider` | Function | `internal/marketdata/macro_provider.go` | 45 |
| `TestMacroIngestorDetectsUSRatesEvent` | Function | `internal/narrative/ingestor_test.go` | 11 |
| `TestMacroIngestorDetectsJPYCarryUnwind` | Function | `internal/narrative/ingestor_test.go` | 45 |
| `TestMacroIngestorNoTriggerOnCalmData` | Function | `internal/narrative/ingestor_test.go` | 70 |
| `NewMacroIngestor` | Function | `internal/narrative/ingestor.go` | 22 |
| `NewDivergenceDetector` | Function | `internal/narrative/divergence.go` | 11 |
| `TestTaiwanStressCalculator_Calculate` | Function | `internal/narrative/taiwan_stress_index_test.go` | 10 |
| `TestTaiwanStressCalculator_CalculateFromSnapshot_CachesResult` | Function | `internal/narrative/taiwan_stress_index_test.go` | 54 |
| `TestTaiwanStressCalculator_RegimeThresholds` | Function | `internal/narrative/taiwan_stress_index_test.go` | 91 |
| `NewTaiwanStressCalculator` | Function | `internal/narrative/taiwan_stress_index.go` | 30 |
| `NewRSSGeopoliticalProvider` | Function | `internal/narrative/geopolitical_provider.go` | 42 |
| `TestTSMCRevenueProvider_Name` | Function | `internal/marketdata/tsmc_revenue_provider_test.go` | 10 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Main → CompositeMacroProvider` | cross_community | 5 |
| `Main → YahooFinanceMacroProvider` | cross_community | 5 |
| `NewReportService → KnowledgeBase` | cross_community | 5 |
| `NewReportService → DefaultTemplates` | cross_community | 5 |
| `Main → RSSGeopoliticalProvider` | intra_community | 4 |
| `RunDailySimulation → QuotesToNarrativeData` | cross_community | 4 |
| `EvaluateModels → NextDate` | cross_community | 4 |
| `NewReportService → NarrativeEngine` | cross_community | 4 |
| `Main → Dataset` | cross_community | 3 |
| `Main → Value` | cross_community | 3 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Marketdata | 10 calls |
| Industry | 6 calls |
| Orchestrator | 6 calls |
| Monitoring | 5 calls |
| Replay | 5 calls |
| Baseline | 3 calls |
| Portfolio | 2 calls |
| Sim | 2 calls |

## How to Explore

1. `gitnexus_context({name: "QuotesToNarrativeData"})` — see callers and callees
2. `gitnexus_query({query: "narrative"})` — find related execution flows
3. Read key files listed above for implementation details
