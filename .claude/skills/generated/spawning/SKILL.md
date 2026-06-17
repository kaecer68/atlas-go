---
name: spawning
description: "Skill for the Spawning area of atlas. 27 symbols across 3 files."
auto_generated: true
load_policy: "manual_only"
---
> ⚠️ **AUTO-GENERATED — 不應自動載入**
> 此技能由程式碼符號索引工具自動生成，僅供程式碼導航參考。
> **不包含領域知識、金融工程見解或使用情境**。AI Coding 時請勿將此技能載入 context。
> 需要模組領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。


# Spawning

27 symbols | 3 files | Cohesion: 80%

## When to Use

- Working with code in `internal/`
- Understanding how TestGapDetector, NewGapDetector, TestAgentFactory work
- Modifying spawning-related functionality

## Key Files

| File | Symbols |
|------|---------|
| `internal/spawning/agent_factory.go` | CreateAgentForGap, generateAgentID, determineLayer, generateSkill, generateName (+9) |
| `internal/spawning/gap_detector.go` | NewGapDetector, DetectGaps, detectSectorGaps, detectStyleGaps, detectMarketCapGaps (+6) |
| `internal/spawning/spawning_test.go` | TestGapDetector, TestAgentFactory |

## Entry Points

Start here when exploring this area:

- **`TestGapDetector`** (Function) — `internal/spawning/spawning_test.go:9`
- **`NewGapDetector`** (Function) — `internal/spawning/gap_detector.go:109`
- **`TestAgentFactory`** (Function) — `internal/spawning/spawning_test.go:86`
- **`NewAgentFactory`** (Function) — `internal/spawning/agent_factory.go:19`
- **`DetectGaps`** (Method) — `internal/spawning/gap_detector.go:120`

## Key Symbols

| Symbol | Type | File | Line |
|--------|------|------|------|
| `TestGapDetector` | Function | `internal/spawning/spawning_test.go` | 9 |
| `NewGapDetector` | Function | `internal/spawning/gap_detector.go` | 109 |
| `TestAgentFactory` | Function | `internal/spawning/spawning_test.go` | 86 |
| `NewAgentFactory` | Function | `internal/spawning/agent_factory.go` | 19 |
| `DetectGaps` | Method | `internal/spawning/gap_detector.go` | 120 |
| `GetOpenGaps` | Method | `internal/spawning/gap_detector.go` | 376 |
| `CreateAgentForGap` | Method | `internal/spawning/agent_factory.go` | 27 |
| `CloneAgentWithVariation` | Method | `internal/spawning/agent_factory.go` | 361 |
| `inferSectorFromAgent` | Function | `internal/spawning/gap_detector.go` | 414 |
| `inferStyleFromAgent` | Function | `internal/spawning/gap_detector.go` | 446 |
| `average` | Function | `internal/spawning/gap_detector.go` | 470 |
| `defaultPromptTemplate` | Function | `internal/spawning/agent_factory.go` | 321 |
| `detectSectorGaps` | Method | `internal/spawning/gap_detector.go` | 161 |
| `detectStyleGaps` | Method | `internal/spawning/gap_detector.go` | 244 |
| `detectMarketCapGaps` | Method | `internal/spawning/gap_detector.go` | 290 |
| `detectRegimeGaps` | Method | `internal/spawning/gap_detector.go` | 303 |
| `detectCorrelationGaps` | Method | `internal/spawning/gap_detector.go` | 333 |
| `generateAgentID` | Method | `internal/spawning/agent_factory.go` | 75 |
| `determineLayer` | Method | `internal/spawning/agent_factory.go` | 93 |
| `generateSkill` | Method | `internal/spawning/agent_factory.go` | 108 |

## Execution Flows

| Flow | Type | Steps |
|------|------|-------|
| `Start → InferSectorFromAgent` | cross_community | 6 |
| `Start → KnowledgeGap` | cross_community | 6 |
| `Start → Average` | cross_community | 6 |
| `Start → InferStyleFromAgent` | cross_community | 6 |
| `Start → DetectMarketCapGaps` | cross_community | 5 |
| `PostSimulation → DetectMarketCapGaps` | cross_community | 4 |

## How to Explore

1. `gitnexus_context({name: "TestGapDetector"})` — see callers and callees
2. `gitnexus_query({query: "spawning"})` — find related execution flows
3. Read key files listed above for implementation details
