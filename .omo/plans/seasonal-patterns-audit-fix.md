# 實作計劃：產業生態系【季節性模式】審計修復

**建立日期**: 2026-05-15
**審計報告**: 季節性模式板塊存在參數無實證、供應鏈隔離、宏觀敘事脫鉤等結構性問題
**範圍**: P0 + P1 項目（前端修復 + 供應鏈整合 + 敘事整合 + 接線）

---

## Phase 1: P0 前端修復

### 1.1 補齊 `renderSeasonalityCalendar()` 函數

**檔案**: `web/static/js/pages/industry.js`
**現狀**: 行 201 呼叫 `renderSeasonalityCalendar(data)` 但函數從未定義 → ReferenceError
**實作**: 新增 `renderSeasonalityCalendar(data)` 函數，基於 API 回傳的 `calendar` 資料渲染 12 個月視圖：
- 從 `data.calendar.months[]` 讀取每月 pattern 列表
- 每個月份顯示活躍的季節性模式名稱、期間、準確度
- 以格線布局呈現（3x4 或 4x3）

**驗證**: DevTools console 不應再有 `renderSeasonalityCalendar is not defined` 錯誤

### 1.2 證據品質視覺提示

**檔案**: `web/static/js/pages/industry.js`
**實作**: 在 `renderSeasonalityList()` 和 `renderSeasonalityTab()` 中，當 `evidence_quality` 為 `"low"` 時顯示視覺提示：
- 在「歷史準確度」數值旁附加 `⚠️ 待驗證` 標籤
- 在表格頂部加入免責說明：「以下數值基於經驗法則，尚未經過回測校準」

**驗證**: 前端渲染後應可見免責說明與警告標籤

---

## Phase 2: P1 供應鏈整合

### 2.1 修改 SeasonalEngine 接受 SupplyChainGraph

**檔案**: `internal/industry/seasonality.go`
**變更**:
1. `SeasonalEngine` struct 新增欄位 `linkageGraph *SupplyChainGraph`
2. 新增方法 `SetLinkageGraph(graph *SupplyChainGraph)` 
3. 新增建構子或修改既有建構子以接受可選的 graph 參數

### 2.2 重構 GetPatternAdjustment() 遍歷供應鏈

**當前邏輯**（靜態字串比對）:
```go
if slices.Contains(p.FavoredIndustries, industryID) { adjustment *= p.AdjustmentFactor }
if slices.Contains(p.AvoidedIndustries, industryID) { adjustment *= (1.0 / p.AdjustmentFactor) }
```

**新邏輯**（供應鏈傳導 + 靜態比對並存）:
```go
func (se *SeasonalEngine) GetPatternAdjustment(industryID string, t time.Time) float64 {
    patterns := se.DetectCurrentPatterns(t)
    adjustment := 1.0
    
    for _, p := range patterns {
        // 1. Direct match (existing behavior)
        if slices.Contains(p.FavoredIndustries, industryID) {
            adjustment *= p.AdjustmentFactor
        }
        if slices.Contains(p.AvoidedIndustries, industryID) {
            adjustment *= (1.0 / p.AdjustmentFactor)
        }
        
        // 2. Supply chain propagation (new)
        if se.linkageGraph != nil {
            for _, favoredID := range p.FavoredIndustries {
                if industryID == favoredID { continue } // already handled
                // Check if our industry is upstream/downstream of a favored industry
                upstream := se.linkageGraph.GetUpstreamChain(favoredID, 2)
                downstream := se.linkageGraph.GetDownstreamChain(favoredID, 2)
                // Apply partial adjustment with decay
                decay := 0.3 // secondary effect decay
                for _, id := range upstream {
                    if id == industryID {
                        adjustment *= (1.0 + (p.AdjustmentFactor-1.0)*decay)
                    }
                }
                for _, id := range downstream {
                    if id == industryID {
                        adjustment *= (1.0 + (p.AdjustmentFactor-1.0)*decay)
                    }
                }
            }
        }
    }
    return adjustment
}
```

**decay 參數**: 從 `config.GetParametersConfig().Industry.LinkageParams` 讀取（reuse 既有 `DownstreamDecayFactor`/`UpstreamDecayFactor`）

### 2.3 更新 IndustryService 接線

**檔案**: `internal/monitoring/service/industry.go`
**變更**: `NewIndustryService()` 中呼叫 `seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())`

**檔案**: 找到 `NewIndustryService` 的所有呼叫點並確認 linkageAnalyzer 傳入正確

---

## Phase 3: P1 宏觀敘事整合

### 3.1 定義 NarrativeSeasonalProvider 介面

**檔案**: `internal/industry/seasonality.go`
```go
// NarrativeSeasonalProvider supplies active narrative events that affect seasonal adjustments.
type NarrativeSeasonalProvider interface {
    ActiveSeasonalThemes() []string // e.g., ["oil_price_shock", "AI_capex_surge"]
    ThemeMultiplier(theme string) float64
}
```

### 3.2 在 SeasonalEngine 中整合敘事事件

**檔案**: `internal/industry/seasonality.go`
**變更**:
1. `SeasonalEngine` struct 新增 `narrativeProvider NarrativeSeasonalProvider`
2. 新增 `SetNarrativeProvider(provider NarrativeSeasonalProvider)`
3. 在 `GetPatternAdjustment()` 最後疊加敘事調整：
```go
// 3. Narrative event overlay (new)
if se.narrativeProvider != nil {
    for _, theme := range se.narrativeProvider.ActiveSeasonalThemes() {
        multiplier := se.narrativeProvider.ThemeMultiplier(theme)
        // narrativeMultiplier → 1.0 = no effect, >1.0 = amplify, <1.0 = dampen
        adjustment *= multiplier
    }
}
```

### 3.3 實作 NarrativeSeasonalProvider

**檔案**: `internal/narrative/seasonal_bridge.go`（新建）
```go
package narrative

import "github.com/kaecer68/atlas-go/internal/industry"

type SeasonalBridge struct {
    engine *NarrativeEngine
}

func NewSeasonalBridge(engine *NarrativeEngine) *SeasonalBridge {
    return &SeasonalBridge{engine: engine}
}

func (sb *SeasonalBridge) ActiveSeasonalThemes() []string {
    // Get active narrative events
    events := sb.engine.DetectEvents(/* snapshot */)
    var themes []string
    for _, e := range events {
        if e.IsSeasonallyRelevant() {
            themes = append(themes, string(e.Theme))
        }
    }
    return themes
}

func (sb *SeasonalBridge) ThemeMultiplier(theme string) float64 {
    // Map narrative themes to seasonal adjustment multipliers
    // oil_price_shock → amplifies summer_electricity energy favor
    // AI_capex_surge → amplifies tech_peak_season
    // ...
}
```

---

## Phase 4: P1 接線 — CycleTracker.SetNarrativeProvider

### 4.1 找到 bootstrap 初始化點

**檔案**: 搜尋 `NewCycleTracker()` 和 `NewIndustryService()` 的呼叫點
**預期位置**: `internal/monitoring/` 或 `cmd/atlas/` 或 `internal/bootstrap/`

### 4.2 接線

```go
// 在初始化 IndustryService 之後
cycleTracker.SetNarrativeProvider(func(industryID string) float64 {
    // Return narrative hit rate for this industry from the narrative engine
    return narrativeBridge.IndustryHitRate(industryID)
})
seasonalEngine.SetNarrativeProvider(narrativeBridge)
seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())
```

---

## Phase 5: 驗證

```bash
go build ./...
go test ./internal/industry/...
go test ./internal/narrative/...
go vet ./...
```

---

## 變更影響範圍

| 模組 | 檔案 | 變更類型 |
|------|------|----------|
| 前端 | `web/static/js/pages/industry.js` | 新增函數 + UI 修改 |
| 產業 | `internal/industry/seasonality.go` | 擴充 struct + 重構方法 |
| 服務 | `internal/monitoring/service/industry.go` | 接線修改 |
| 敘事 | `internal/narrative/seasonal_bridge.go` | 新建檔案 |
| 初始化 | `internal/bootstrap/` 或 `cmd/atlas/` | 接線修改 |

## 向後相容性

- 所有既有 API 簽名保持不變
- `SeasonalEngine` 的新欄位透過 Setter 注入（nil-safe）
- 前端資料格式不變，僅新增 visual indicators
- `DefaultSeasonalPatterns()` 標記為 deprecated 但保留以維持向後相容

## 風險

- **低風險**: 前端修復（純 UI，不影響資料流）
- **中風險**: 供應鏈整合（計算結果會改變，需驗證 adjustment value 範圍合理）
- **中風險**: 敘事整合（需確保 narrative provider 在 bootstrap 階段可用）
