---
name: narrative
description: "Skill for the Narrative area of atlas. 111 symbols across 21 files."
---

# Narrative

111 symbols | 21 files | Cohesion: 76%

## When to Use

- Working with code in `internal/`
- Understanding how TestNarrativeEngineDetectEvents, TestNarrativeEngineActiveModels, TestDetectEventsNoTrigger work
- Modifying narrative-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/narrative/knowledge_base.go` | NewNarrativeEngine, DetectEvents, ActiveModels, detectUSRatesEvent, detectAICapexEvent (+15) |
| `internal/narrative/report_generator_test.go` | TestGenerateDailySummary_DateMatches, TestGenerateDailySummary_SectionsNonEmpty, TestGenerateDailySummary_NarrativeEventsMentioned, TestGenerateDailySummary_TopPicks, TestGenerateDailySummary_TopPicksCapped (+8) |
| `internal/narrative/ingestor.go` | hitRateForTheme, detectEventsFromSnapshot, detectUSRatesEventFromSnapshot, detectJPYCarryUnwindEventFromSnapshot, detectGeopoliticalRiskEventFromSnapshot (+5) |
| `internal/narrative/macro_assessment.go` | Assess, evaluateRiskFactors, calculateOutflowProbability, determinePrimaryFlow, determineSectorRotation (+4) |
| `internal/narrative/narrative_test.go` | TestNarrativeEngineDetectEvents, TestNarrativeEngineActiveModels, TestDetectEventsNoTrigger, TestKnowledgeBaseDefaults, TestKnowledgeBaseRegisterAndMatch (+1) |
| `internal/monitoring/dashboard_api.go` | handleNarrativeEvents, handleNarrativeChains, handleNarrativeModels, parseFloatQuery, handleNarrativeTemplates (+1) |
| `internal/narrative/taiwan_geopolitical_provider_test.go` | TestTaiwanRSSGeopoliticalProvider_Name, TestTaiwanRSSGeopoliticalProvider_Feeds, TestTaiwanRSSGeopoliticalProvider_Keywords, TestTaiwanRSSGeopoliticalProvider_FetchScore, TestCompositeTaiwanGeopoliticalProvider_Name (+1) |
| `internal/narrative/taiwan_geopolitical_provider.go` | NewTaiwanRSSGeopoliticalProvider, Name, FetchScore, NewCompositeTaiwanGeopoliticalProvider, Name (+1) |
| `internal/narrative/taiwan_stress_index.go` | NewTaiwanStressCalculator, Calculate, CalculateFromSnapshot, CalculateFromSnapshotWithStore |
| `internal/narrative/geopolitical_provider.go` | NewRSSGeopoliticalProvider, NewGDELTGeopoliticalProvider, NewCompositeGeopoliticalProvider, FetchScore |

## Entry Points

Start here when exploring this area:

- **`TestNarrativeEngineDetectEvents`** (Function) — `internal/narrative/narrative_test.go:65`
- **`TestNarrativeEngineActiveModels`** (Function) — `internal/narrative/narrative_test.go:108`
- **`TestDetectEventsNoTrigger`** (Function) — `internal/narrative/narrative_test.go:141`
- **`NewNarrativeEngine`** (Function) — `internal/narrative/knowledge_base.go:105`
- **`TestTaiwanStressCalculator_Calculate`** (Function) — `internal/narrative/taiwan_stress_index_test.go:10`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestNarrativeEngineDetectEvents` | Function | `internal/narrative/narrative_test.go` | 65 |
| `TestNarrativeEngineActiveModels` | Function | `internal/narrative/narrative_test.go` | 108 |
| `TestDetectEventsNoTrigger` | Function | `internal/narrative/narrative_test.go` | 141 |
| `NewNarrativeEngine` | Function | `internal/narrative/knowledge_base.go` | 105 |
| `TestTaiwanStressCalculator_Calculate` | Function | `internal/narrative/taiwan_stress_index_test.go` | 10 |
| `TestTaiwanStressCalculator_CalculateFromSnapshot_CachesResult` | Function | `internal/narrative/taiwan_stress_index_test.go` | 54 |
| `TestTaiwanStressCalculator_RegimeThresholds` | Function | `internal/narrative/taiwan_stress_index_test.go` | 91 |
| `NewTaiwanStressCalculator` | Function | `internal/narrative/taiwan_stress_index.go` | 30 |
| `NewRSSGeopoliticalProvider` | Function | `internal/narrative/geopolitical_provider.go` | 40 |
| `DefaultTemplates` | Function | `internal/narrative/templates.go` | 3 |
| `TestKnowledgeBaseDefaults` | Function | `internal/narrative/narrative_test.go` | 7 |
| `TestKnowledgeBaseRegisterAndMatch` | Function | `internal/narrative/narrative_test.go` | 23 |
| `NewKnowledgeBase` | Function | `internal/narrative/knowledge_base.go` | 19 |
| `TestGenerateDailySummary_DateMatches` | Function | `internal/narrative/report_generator_test.go` | 9 |
| `TestGenerateDailySummary_SectionsNonEmpty` | Function | `internal/narrative/report_generator_test.go` | 20 |
| `TestGenerateDailySummary_NarrativeEventsMentioned` | Function | `internal/narrative/report_generator_test.go` | 35 |
| `TestGenerateDailySummary_TopPicks` | Function | `internal/narrative/report_generator_test.go` | 78 |
| `TestGenerateDailySummary_TopPicksCapped` | Function | `internal/narrative/report_generator_test.go` | 96 |
| `TestGenerateDailySummary_RiskLevel` | Function | `internal/narrative/report_generator_test.go` | 115 |
| `NewReportGenerator` | Function | `internal/narrative/report_generator.go` | 17 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `HandleChannelsIngest → NarrativeEvent` | cross_community | 5 |
| `HandleChannelsIngest → HitRateForTheme` | cross_community | 5 |
| `HandleChannelsIngest → BoolToFloat` | cross_community | 5 |
| `HandleMacroIngest → NarrativeEvent` | cross_community | 5 |
| `HandleMacroIngest → HitRateForTheme` | cross_community | 5 |
| `HandleMacroIngest → BoolToFloat` | cross_community | 5 |
| `HandleTaiwanStressIndex → NarrativeEvent` | cross_community | 5 |
| `HandleTaiwanStressIndex → HitRateForTheme` | cross_community | 5 |
| `HandleTaiwanStressIndex → BoolToFloat` | cross_community | 5 |
| `RunReplaySimulation → NarrativeEvent` | cross_community | 5 |

## Connected Areas

| Area | Connections |
|------|-------------|
| Monitoring | 11 calls |
| Swarm | 5 calls |
| Config | 1 calls |
| Db | 1 calls |
| Live | 1 calls |
| Risk | 1 calls |
| Replay | 1 calls |

## How to Explore

1. `gitnexus_context({name: "TestNarrativeEngineDetectEvents"})` — see callers and callees
2. `gitnexus_query({query: "narrative"})` — find related execution flows
3. Read key files listed above for implementation details
