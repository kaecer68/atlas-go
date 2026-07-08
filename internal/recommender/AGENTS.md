# AGENTS.md — internal/recommender

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 為 `/api/recommendations` 提供 tier-aware 投資建議（Free / Registered / Premium）

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `TierRecommendation` | handler.go | 對外 response：`Tier`/`Regime`/`StressIndex`/`CapitalFlow`/`EventsToday`/`Strategies`/`Warning` |
| `StrategyRecommendation` | handler.go | 結構化推薦：`Active`/`Available`/`Ranked`/`EntrySignal`/`StopLoss`/`TakeProfit` |
| `MarketLight` | handler.go | 市場速覽：regime + 壓力指數 + 資金流 + 事件。`CapitalFlow` 為字串摘要(向後相容);`CapitalFlowDetail` 為結構化欄位(PR #1004 新增,來源 capitalflow.SummaryReport,新消費者優先用此欄位) |
| `Handler` | handler.go | 主 handler：`subStore`, `jwtMgr`, `narrative`, `capitalFlow`, `eventPredictor`, `strategyComp`, `regimeListener`, `lastSeenRegime`, `devMode` |
| `NewHandler` / `NewHandlerWithServices` | handler.go | old/new constructor (後者帶 4 服務 deps) |
| `HandlerDeps` | handler.go | deps grouping struct (4 optional services) |
| 4 個 consumer interfaces | deps.go | `NarrativeProvider` / `CapitalFlowProvider` / `EventPredictor` / `ComparisonEngine` |
| `RegimeChangeListener` | deps.go | func type fired on regime transitions |

## 認證鏈

```
HTTP request
  → ExtractToken (subscription/auth.go)
    → Verify (subscription/auth.go) → JWT valid
      → EffectiveTier (subscription/types.go) → premium/registered/free
  → Alt: X-User-Email header (ONLY when Handler.devMode=true; 401 otherwise)
```

`devMode` 由 `main.go` 透過 `RegisterRoutesWithDeps(..., devMode bool)` 注入。production 預設 `devMode=false`，僅本地/測試設 `ATLAS_DEV_MODE=true` 才開放 X-User-Email fallback。

## 資料流

```
GET /api/recommendations
  → RegisterRoutesWithDeps-mounted handler.HandleRecommendations
    → ExtractTier (JWT primary; X-User-Email 僅 dev mode)
      → 走 switch tier block
        → build handler 用 4 個 NIL-safe helpers:
            * stressIndexFromNarrative(p) / regimeFromNarrative(p)
            * capitalFlowFromCapitalFlow(p)
            * eventsFromPredictor(p)
            * signalsFromComparisonEngine(p)
          全部 graceful — nil deps 回 stub / fallback 值
      → TierRecommendation {Market, Strategies, Warning}
```

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **Hardcoded fallback** | NIL deps → handlers 回 hardcoded 安全值 (`Regime=NEUTRAL`, `Score=0`, `CapitalFlow="資金流向均衡"`, `EventsToday=[]`)，不會 panic。Q3 graceful degradation 經 7 個 adapter tests 驗證。 |
| **X-User-Email tier 偽造** | Production 預設 401 (`devMode=false`)。dev mode 需顯式設定 `ATLAS_DEV_MODE=true` 透過 `config.GetSecret(...)`，**不可直接呼叫 `os.Getenv`** (per `apigateway/CONSTITUTION.md` Art.1)。 |
| **`RegimeChangeListener` race** | `lastSeenRegime` 讀寫無 mutex — 並發請求可能丟/多觸發。Race tolerable by design (idempotent hook), 但若需要嚴格順序需加 sync。 |
| **StrategyScoreInfo 暫為 floating-point only** | `ComparisonEngine.GetScore(s)` 只回 float。`EntrySignal` 由 handler hardcoded 模板 `"Score=%.2f — 等回測支撐區間"` 衍生；`StopLoss` 寫死 `-5%`。完整 StrategyScoreInfo 整合由 RiskGate 提供 (待後續 PR)。 |

## 整合契約（adapters.go — 真實 producer wrapping）

```
                                  HandlerDeps{Narrative, CapitalFlow, EventPredictor, Strategy}
                                       ↑ main.go L593 RegisterRoutesWithDeps(deps)
                                       │    + config.GetSecret("ATLAS_DEV_MODE")=="true" → devMode
internal/recommender/handler.go
                                       ↑ adapter method (narrativeAdapter 等)
                                       │    4 struct adapters wrap 真實 producer
internal/recommender/adapters.go ────────┐
                                         │
   ┌─────────────┬──────────────┬─────────┐
   │narrative    │capitalflow   │eventdriven  strategy
   │adapter      │adapter       │adapter     adapter
   ↓             ↓              ↓          ↓
*narrative      capitalflow.   *eventdriven *strategy
.Narrative      Service         .Predictor   .Comparison
Service         (commit         (read        Engine
(read 661)        #1)           adapter)    (read adapter)
```

真實 producer signature 見 `.omo/research/2026-07-08-recommender-wiring-gaps.md`。

## 測試

- `internal/recommender/handler_test.go` — 13 tests (Tier gates, JWT, X-User-Email fallback, 4 service mocks 對齊新 signature, regime-change listener, e2e-equivalent with mocked services)
- `internal/recommender/e2e_test.go` — T13 E2E (mock + httptest.Server)
- `internal/recommender/adapters_test.go` — 7 tests (graceful nil-safety for 4 adapters + 2 pass-through)
- 全部 `go test ./internal/recommender/` 全綠
