# AGENTS.md — internal/recommender

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 為 `/api/recommendations` 提供 tier-aware 投資建議（Free / Registered / Premium）

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `TierRecommendation` | `handler.go:11-17` | 對外 response：`Tier`/`Regime`/`StressIndex`/`CapitalFlow`/`EventsToday`/`Strategies` |
| `StrategyRecommendation` | `handler.go:19-27` | 結構化推薦：`Active`/`Available`/`Ranked`/`EntrySignal`/`StopLoss`/`TakeProfit` |
| `MarketLight` | `handler.go:29-36` | 市場速覽：regime + 壓力指數 + 資金流 + 事件 |
| `Handler` | `handler.go:38-42` | 主 handler：`subStore`, `jwtMgr` |
| `NewHandler` | `handler.go:44-48` | constructor |

## 認證鏈

```
HTTP request
  → ExtractToken (subscription/auth.go)
    → Verify (subscription/auth.go) → JWT valid
      → EffectiveTier (subscription/types.go) → premium/registered/free
  → Alt: X-User-Email header (ONLY when ATLAS_DEV_MODE=true; 401 otherwise)
```

**P0-2 修復**：production `ATLAS_DEV_MODE` 未設置時，`X-User-Email` header 一律 401。dev mode (`ATLAS_DEV_MODE=true`) 保留 legacy 行為給本地測試。

## 資料流

```
GET /api/recommendations
  → HandleRecommendations
    → ExtractTier (JWT or X-User-Email)
      → TierFree/Registered/Premium
        → switch tier → 寫死 MarketLight + StrategyRecommendation  ← P0-1 STUB
```

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **stub 寫死** | `HandleRecommendations` 內所有 `MarketLight`/`StrategyRecommendation` 欄位目前是 hardcoded constants（NEUTRAL/0.0/"資金流向均衡"）。**真實數據整合是 Sprint 2 T7-T12 工作**。 |
| **X-User-Email tier 偽造** | 即使 JWT verify 失敗，仍 fallback `X-User-Email` header → 已用 `ATLAS_DEV_MODE` ENV flag 控管（commit `7abe4c2f`）。 |
| **無 AGENTS.md 跨模組 contract** | 與 `narrative`/`capitalflow`/`eventdriven`/`strategy_ranker` 整合契約尚未正式化，T7 需定義。 |

## 整合點（Sprint 2 T7-T12 預備）

| 服務 | 用途 | 預期呼叫 |
|------|------|----------|
| `monitoring/service/narrative.go::GetCurrentStressIndex` | 即時 TaiwanStressIndex | reg-mode detection |
| `monitoring/service/narrative.go::BuildMarketNarrativeData` | 事件 + chain + templates | EventsToday 注入 |
| `capitalflow.Handler::HandleDaily` | 七大資金勢力 summary | CapitalFlow 注入 |
| `eventdriven/predictor.go::NewPredictor` | 5-day flow prediction | 短期 signals 注入 |
| `strategy.ComparisonEngine::GetScore` | 策略評分 | EntrySignal/StopLoss 計算（Q2） |
| `risk.RiskGate::Mode` | 即時 mode | 警示 banner 顯示 |

擴 RegisterRoutes signature 從 `(mux, *subStore, jwtMgr)` 改為 `(mux, *subStore, jwtMgr, narrativeService, capitalflowHandler, predictor, comparisonEngine)` — 由 main.go L593 改呼叫。

## 測試

- 4 個 tests in `handler_test.go`
- RED→GREEN TDD 紀律：每個 P 必先 failing test（見 T1 例子）
- Mock 真實 services 用 interface-only，或套件層 `package` 共用 mock
