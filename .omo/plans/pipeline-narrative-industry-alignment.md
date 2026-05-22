# Plan: 投資管線 — 敘事・產業數據對齊

**Branch**: `feat/pipeline-narrative-industry-alignment`
**Status**: 規劃中
**Created**: 2026-05-20

---

## 背景

宏觀敘事（`internal/narrative/`）與產業生態系（`internal/industry/`）已完成大量推算升級：
- NarrativeEvent 新增 ConfidenceSource, HitRate, Severity, Status, SourceData, Duration, ExpiresAt
- 產業生態系新增供應鏈連動、季節性模式四層分解、週期羅盤、動態環境調變
- 前端「決策鏈」頁面已獨立呼叫 `/api/narrative/*` 與 `/api/industry/*` 顯示數據

但**投資管線**（`/api/dashboard/recommendation-pipeline`）作為核心推薦展示 API，其回傳的 `PipelineItem` 結構**完全沒有**敘事與產業數據：
- NarrativeEvent 未進入 PipelineItem
- 產業生態系計算未回傳至管線
- Conviction 計算對敘事/產業數據是盲的
- FactorScores 缺少敘事/產業因子
- 決策鏈透明化第三階段（宏觀事件）與管線斷裂

本計畫旨在打通這些數據斷層，實現短期可交付、中期可擴展的完整對齊方案。

---

## Phase 0: 立即修復（低風險，1-2 天）

### P0.1 — 前端演算法說明文字更新

**問題**：`pipeline.js:106-108` 仍寫著「confidence 為經驗硬編碼常數（heuristic_fixed_v1）」

**修改**：
- 檔案：`web/static/js/pages/pipeline.js`
- 位置：`renderDecisionChain()` 中的 `macroContent`
- 將硬編碼說明更新為反映實際 ConfidenceSource（deviation_based_v1, margin_history_percentile, calendar_seasonal）
- 預計改動：~5 行

**驗證**：
- 前端載入決策鏈頁面，確認「宏觀算法說明」區塊顯示更新後的文字

---

### P0.2 — NarrativeEvent IDs 寫入 RecommendationOutcome

**問題**：`Recommendation` 有 `SupportingEvents []string` 欄位，但只有 context/superinvestor 層被填充，且 `RecommendationOutcome`（JSONL 持久化）**根本沒有**此欄位

**修改**：

1. **`internal/domain/recommendation/recommendation.go`** — `RecommendationOutcome` 新增欄位：
   ```go
   SupportingEvents    []string `json:"supporting_events,omitempty"`
   ```

2. **`internal/orchestrator/executors.go`** — `collectRecommendations()` 接收 NarrativeEvents：
   ```go
   func collectRecommendations(
       ctx context.Context,
       registry domain.AgentRegistry,
       quotes map[string]domain.Quote,
       plugins *PluginRegistry,
       overrides map[string]string,
       regime domain.Regime,
       sessionID string,
       scratchpad *Scratchpad,
       narrativeEvents []narrative.NarrativeEvent,  // 新增參數
   ) ([]domain.Recommendation, []domain.ScreeningReject)
   ```

3. **`internal/orchestrator/executors.go`** — `ExecuteWithContext()` 傳遞 NarrativeEvents 給 `collectRecommendations()`

4. **`internal/orchestrator/executors.go`** — 在 `collectRecommendations()` 尾部，對所有 layer 的推薦（不限 context/superinvestor）填充 `SupportingEvents`：
   ```go
   for i := range recs {
       eventIDs := make([]string, len(narrativeEvents))
       for j, e := range narrativeEvents {
           eventIDs[j] = e.ID
       }
       recs[i].SupportingEvents = eventIDs
   }
   ```

5. **`internal/orchestrator/system.go`** — `buildSyntheticOutcomes()` 將 `rec.SupportingEvents` 複製到 `outcome.SupportingEvents`

**預計改動**：~40 行（5 個檔案）

**驗證**：
- `go test ./internal/orchestrator/...`
- `go test ./internal/domain/...`
- 執行一次回測，檢查 `recommendation_outcomes.jsonl` 中是否出現 `"supporting_events"` 欄位

---

### P0.3 — PipelineItem 新增 narrative_event_ids 欄位

**問題**：`PipelineItem`（API 回應結構）沒有 narrative 相關欄位

**修改**：

1. **`internal/monitoring/api/pipeline/handlers.go`** — `PipelineItem` 新增欄位：
   ```go
   NarrativeEventIDs []string `json:"narrative_event_ids,omitempty"`
   ```

2. **`internal/monitoring/service/pipeline.go`** — `PipelineItemData` 新增欄位：
   ```go
   NarrativeEventIDs []string
   ```

3. **`internal/monitoring/service/pipeline.go`** — `loadSessionPipelineData()` 從 `outcome.SupportingEvents` 映射到 `PipelineItemData.NarrativeEventIDs`

4. **`internal/monitoring/api/pipeline/handlers.go`** — `HandleRecommendationPipeline()` 映射新欄位：
   ```go
   items[i] = PipelineItem{
       // ...existing fields...
       NarrativeEventIDs: item.NarrativeEventIDs,
   }
   ```

**預計改動**：~20 行（2 個檔案）

**驗證**：
- `curl 'http://localhost:8080/api/dashboard/recommendation-pipeline' | jq '.items[0].narrative_event_ids'`
- 確認前端能正常接收新欄位（不做渲染）
- `go test ./internal/monitoring/...`

---

## Phase 1: 短期增強（1-2 週）

### P1.1 — NarrativeConvictionModulator

**目標**：參考 `IndustryCycleModulator` 的模式，建立敘事信念度調節器

**新建檔案**：`internal/orchestrator/narrative_conviction_modulator.go`

**核心邏輯**：
```go
type NarrativeConvictionModulator struct {
    themeHitRates map[string]float64  // Theme → HitRate
    skillToTheme  map[string]string   // agent skill → 相關 NarrativeTheme
}

// skillToTheme 映射：
// "semiconductor_desk"   → "AI_capex_surge"
// "ai_supply_chain_desk" → "AI_capex_surge"
// "shipping_desk"        → "oil_price_shock"
// "financials_desk"      → "US_rates_up"
// "etf_rotation_desk"    → "JPY_carry_unwind"

func (m *NarrativeConvictionModulator) ModulateRecommendations(
    recs []domain.Recommendation,
    registry domain.AgentRegistry,
    events []narrative.NarrativeEvent,
) {
    for i := range recs {
        skill := skillLookup[recs[i].Agent]
        theme := m.skillToTheme[skill]
        for _, e := range events {
            if e.Theme == theme && e.Status == "active" {
                adj := int(math.Round(float64(10) * e.HitRate))
                recs[i].Conviction += adj
                step := domain.ConvictionStep{
                    Rule:   "narrative_boost",
                    Delta:  adj,
                    Reason: fmt.Sprintf("%s (hit_rate: %.0f%%, confidence: %.0f%%)",
                        e.Theme, e.HitRate*100, e.Confidence*100),
                }
                if recs[i].ConvictionBreakdown != nil {
                    recs[i].ConvictionBreakdown.Steps = append(
                        recs[i].ConvictionBreakdown.Steps, step)
                    recs[i].ConvictionBreakdown.Final = recs[i].Conviction
                }
            }
        }
    }
}
```

**註冊**：
- `internal/orchestrator/plugin_registry.go` — 新增 `narrativeModulator` 欄位與 setter
- `internal/orchestrator/executors.go` — 在 `collectRecommendations()` 中，於 cycle modulator 之後執行：
  ```go
  if plugins.narrativeModulator != nil {
      plugins.narrativeModulator.ModulateRecommendations(recs, registry, narrativeEvents)
  }
  ```

**預計改動**：~120 行（新建 1 檔案 + 修改 2 檔案）

**驗證**：
- `go test ./internal/orchestrator/... -run TestNarrativeModulator`
- 檢查有 active NarrativeEvent 時的 conviction breakdown steps 是否包含 `"narrative_boost"` 步驟

---

### P1.2 — PipelineItem 新增 narrative_context 與 industry_context

**目標**：讓前端管線頁面能直接展示敘事與產業上下文

**修改**：

1. **`internal/monitoring/api/pipeline/handlers.go`** — `PipelineItem` 新增：
   ```go
   NarrativeContext  *NarrativeContextItem  `json:"narrative_context,omitempty"`
   IndustryContext   *IndustryContextItem   `json:"industry_context,omitempty"`
   ```

2. 新增輔助結構（同檔案）：
   ```go
   type NarrativeContextItem struct {
       ActiveThemes   []string `json:"active_themes"`
       PrimaryTheme   string   `json:"primary_theme,omitempty"`
       PrimaryHitRate float64  `json:"primary_hit_rate,omitempty"`
       DirectionHint  string   `json:"direction_hint,omitempty"` // "positive" / "negative" / "neutral"
   }

   type IndustryContextItem struct {
       IndustryID         string  `json:"industry_id"`
       BusinessCycle      string  `json:"business_cycle"`
       CycleConfidence    float64 `json:"cycle_confidence"`
       SeasonalMultiplier float64 `json:"seasonal_multiplier"`
       SystemicImportance float64 `json:"systemic_importance"`
   }
   ```

3. **`internal/monitoring/service/pipeline.go`** — `PipelineService` 需要注入依賴或改為 enrichment 層
   - 短期方案：PipelineService 接收 `*narrative.NarrativeEngine` 和 `*industry.CycleTracker` 注入
   - 在 `loadSessionPipelineData()` 中對每筆 item 進行 narrative/industry context enrichment

**預計改動**：~100 行（新建結構 + 修改 2 檔案）

**驗證**：
- API 回應包含 `narrative_context` 與 `industry_context`
- 前端管線頁面顯示對應資訊（不破壞現有佈局）

---

### P1.3 — 前端管線頁面加入敘事影響列

**目標**：投資管線表格新增「敘事影響」列，顯示關聯的 NarrativeEvent 主題標籤

**修改**：`web/static/js/pages/pipeline.js`

1. 表格 `<thead>` 在「因子總分」之後新增 `<th>敘事影響</th>`
2. `buildTableRows()` 中，每列根據 `item.narrative_event_ids` 渲染主題標籤
3. 主題標籤點擊時，彈出 modal 顯示該 NarrativeEvent 的詳細資訊（Confidence, HitRate, ConfidenceSource, SourceData）

**預計改動**：~60 行

**驗證**：
- 前端管線頁面載入後，有 active NarrativeEvent 的推薦顯示主題標籤
- 點擊標籤彈出詳細資訊

---

## Phase 2: 中期重構（1-2 月）

### P2.1 — NarrativeFactor 與 IndustryCycleFactor

**目標**：讓 FactorScores 本質上反映宏觀與產業環境

**新建檔案**：`internal/portfolio/narrative_factor.go` + `internal/portfolio/industry_cycle_factor.go`

**計算邏輯**：

- **NarrativeFactor**（權重: 0.10）
  - 基於當前 active NarrativeEvent 的主題與該標的所屬產業的關聯性
  - 公式：`Σ(HitRate × Sentiment × ThemeMatch) / ActiveEventCount`
  - 例如：`AI_capex_surge` active 時，半導體產業標的獲得正向調整

- **IndustryCycleFactor**（權重: 0.10）
  - 基於 CycleTracker 中該產業的 ContinuousPhaseScore
  - 擴張期 → +0.5 ~ +1.0，衰退期 → -0.5 ~ -1.0
  - 信心度加權

**調整**：
- `internal/portfolio/factor_engine.go` — `FactorWeightConfig` 的 defaultWeights 更新
- `internal/domain/shared/shared.go` — `FactorScores` 與 `FactorScoreBreakdown` 新增欄位
- 總分公式變更：`動能×0.25 + 價值×0.20 + 品質×0.20 + Agent×0.15 + 敘事×0.10 + 產業週期×0.10`

**預計改動**：~200 行（新建 2 檔案 + 修改 4 檔案）

**驗證**：
- `go test ./internal/portfolio/...`
- 檢查 API 回應的 `factor_scores.breakdown` 包含新因子
- 前端因子明細自動顯示新因子（因 `renderFactorBreakdown()` 已支援動態渲染）

---

### P2.2 — PipelineService 升級為 Enrichment 層

**目標**：管線服務從純讀取層升級為數據補充層

**修改**：`internal/monitoring/service/pipeline.go`

1. `PipelineService` 新增依賴：
   ```go
   type PipelineService struct {
       // ...existing fields...
       narrativeEngine *narrative.NarrativeEngine
       cycleTracker    *industry.CycleTracker
       seasonalEngine  *industry.SeasonalEngine
       linkageGraph    *industry.SupplyChainGraph
   }
   ```

2. `LoadRecommendationPipeline()` 在載入 JSONL 後調用 `enrichPipelineItems()`：
   ```go
   func (s *PipelineService) enrichPipelineItems(items []PipelineItemData) {
       events := s.narrativeEngine.ActiveEvents()
       for i := range items {
           // 1. Narrative context
           enrichment := s.buildNarrativeContext(items[i], events)
           items[i].NarrativeContext = enrichment

           // 2. Industry context
           industryCtx := s.buildIndustryContext(items[i])
           items[i].IndustryContext = industryCtx
       }
   }
   ```

3. `buildNarrativeContext()` 根據 `SupportingEvents` 中的 EventID 查詢當前事件的狀態（可能已過期/已確認）

4. `buildIndustryContext()` 根據 `skillToIndustry` 映射查詢 CycleTracker、SeasonalEngine、LinkageGraph

**預計改動**：~150 行（修改 1-2 檔案）

**驗證**：
- API 回應的 `narrative_context` 與 `industry_context` 為即時數據（非 JSONL 持久化時的舊數據）
- 前端切換場次時，敘事與產業數據正確反映該場次時間點的狀態

---

### P2.3 — 前端「敘事影響」深度整合

**目標**：管線頁面與決策鏈頁面的敘事數據達到一致的深度

**修改**：`web/static/js/pages/pipeline.js`

1. 「敘事影響」列支援按主題分組折疊/展開
2. 每筆推薦的 factor breakdown 中顯示 narrative_factor 與 industry_cycle_factor
3. Conviction breakdown 中的 `narrative_boost` 步驟高亮顯示，並可點擊查看觸發事件
4. 管線頁面頂部新增「當前宏觀事件摘要」橫幅（從 `/api/narrative/events` 取得）

**預計改動**：~150 行

---

## Phase 3: 長期演進（2+ 月）

### P3.1 — 事件驅動因子權重引擎全面實作

**目標**：`FactorWeightEngine` 全面接收 NarrativeEvent，動態調整六因子權重

**新建檔案**：`internal/portfolio/event_driven_weights.go`

**邏輯**：
```go
// 當 AI_capex_surge 觸發時：
// Quality 權重 +0.05, Momentum 權重 +0.05, Value 權重 -0.05

// 當 US_rates_up 觸發時：
// Value 權重 +0.05, InstSentiment 權重 +0.05, Momentum 權重 -0.05

// 當 oil_price_shock 觸發時：
// Liquidity 權重 -0.05, Momentum 權重 -0.05
```

### P3.2 — 供應鏈連動分數納入因子計算

**目標**：`SupplyChainGraph.SystemicImportance` 作為 Quality 的子因子

### P3.3 — 季節性模式直接影響信念度

**目標**：`SeasonalEngine.GetPatternAdjustment()` 整合至 `NarrativeConvictionModulator`

### P3.4 — 回測驗證框架

**目標**：建立自動化驗證確保：
- 敘事事件對 conviction 的影響方向正確（回測命中率驗證）
- 產業週期調整的置信度校準
- 前端顯示與後端計算的一致性

---

## 檔案變更總覽

| Phase | 檔案 | 新建/修改 | 預計行數 |
|-------|------|----------|---------|
| P0.1 | `web/static/js/pages/pipeline.js` | 修改 | ~5 |
| P0.2 | `internal/domain/recommendation/recommendation.go` | 修改 | +2 |
| P0.2 | `internal/orchestrator/executors.go` | 修改 | ~15 |
| P0.2 | `internal/orchestrator/system.go` | 修改 | ~5 |
| P0.3 | `internal/monitoring/api/pipeline/handlers.go` | 修改 | +5 |
| P0.3 | `internal/monitoring/service/pipeline.go` | 修改 | ~15 |
| P1.1 | `internal/orchestrator/narrative_conviction_modulator.go` | **新建** | ~80 |
| P1.1 | `internal/orchestrator/plugin_registry.go` | 修改 | ~15 |
| P1.1 | `internal/orchestrator/executors.go` | 修改 | ~10 |
| P1.1 | `internal/orchestrator/narrative_conviction_modulator_test.go` | **新建** | ~60 |
| P1.2 | `internal/monitoring/api/pipeline/handlers.go` | 修改 | ~50 |
| P1.2 | `internal/monitoring/service/pipeline.go` | 修改 | ~50 |
| P1.3 | `web/static/js/pages/pipeline.js` | 修改 | ~60 |
| P2.1 | `internal/portfolio/narrative_factor.go` | **新建** | ~60 |
| P2.1 | `internal/portfolio/industry_cycle_factor.go` | **新建** | ~60 |
| P2.1 | `internal/portfolio/factor_engine.go` | 修改 | ~30 |
| P2.1 | `internal/domain/shared/shared.go` | 修改 | ~10 |
| P2.2 | `internal/monitoring/service/pipeline.go` | 修改 | ~150 |
| P2.3 | `web/static/js/pages/pipeline.js` | 修改 | ~150 |

---

## 風險評估

| 風險 | 可能性 | 影響 | 緩解措施 |
|------|--------|------|---------|
| JSONL 向後相容性破壞 | 低 | 高 | P0.2 使用 `omitempty`，舊 JSONL 無此欄位時預設為 nil |
| NarrativeEngine 未初始化 | 中 | 中 | 所有 modulator 實作 nil-safe；PipelineService 降級為原有行為 |
| 前端效能影響（25→更多 API 呼叫） | 低 | 低 | P0.3/P1.2 將敘事數據嵌入現有 API 回應，不增加呼叫次數 |
| Conviction 過度調整（多個 modulator 疊加） | 中 | 中 | 設定總調整上限（±30）；Phase 3 回測驗證調整幅度 |
| 因子權重變更影響現有回測結果 | 高（P2.1） | 中 | P2.1 為 opt-in（透過 ParametersConfig 開關）；預設關閉 |

---

## 相依性

```
P0.1 ──→ 獨立
P0.2 ──→ P0.3 依賴 P0.2（需要 SupportingEvents 寫入 JSONL）
P0.3 ──→ 獨立（但若無 P0.2，narrative_event_ids 永遠為空）
P1.1 ──→ 依賴 P0.2（需要 NarrativeEvents 傳入 collectRecommendations）
P1.2 ──→ 可獨立（短期用 JSONL 數據，長期依賴 P2.2）
P1.3 ──→ 依賴 P0.3 + P1.2
P2.1 ──→ 依賴 P1.1（需要 modulator 建立映射關係）
P2.2 ──→ 可獨立，但建議在 P1.2 之後
P2.3 ──→ 依賴 P1.3 + P2.1 + P2.2
```

**建議執行順序**：P0.1 → P0.2 → P0.3 → P1.1 + P1.2（並行）→ P1.3 → P2.1 + P2.2（並行）→ P2.3
