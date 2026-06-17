# 宏觀敘事 → 投資模型 功能審計報告

**分支**: `audit/macro-narrative-to-investment-model`  
**審計日期**: 2026-06-08  
**審計範圍**: 前端「宏觀敘事」頁面對「投資模型」板塊的參數引用、消費引用、覆蓋率、功能完整性  
**審計方法**: 靜態代碼分析 + 前後端數據流追蹤 + 參數配置一致性檢查

---

## 一、執行摘要

| 類別 | 嚴重缺陷 | 中等缺陷 | 輕微缺陷 | 合計 |
|------|---------|---------|---------|------|
| 數量 | 3 | 3 | 3 | 9 |
| 狀態 | 待修復 | 待修復 | 待修復 | — |

**結論**: 投資模型的核心評估功能存在結構性缺陷，導致 `RecentError`、`HitRate`、`Weight` 等關鍵指標**無法正確計算**，前端呈現的數據**不具實際參考價值**。建議優先修復 P0 級問題。

---

## 二、P0 嚴重缺陷（阻塞性）

### 🔴 P0-1: 板塊名稱鍵值不匹配導致模型評估完全失效

**位置**: `internal/narrative/knowledge_base.go:557-578` (`avgSectorReturn`)  
**影響**: 所有投資模型的 `RecentError`、`HitRate`、`Weight` 均為無效值

**問題描述**:

因果模板 (`templates.go`) 的 `Steps[].Affected` 使用**中文板塊名稱**（如 `"金融"`、`"高股息"`、`"AI供應鏈"`），而模型定義 (`knowledge_base.go:155-169`) 的 `FavoredSectors`/`AvoidedSectors` 使用**英文板塊鍵**（如 `"financials"`、`"high_dividend"`、`"ai_supply_chain"`）。

`EvaluateModels` 函數通過 `avgSectorReturn` 查找板塊對應的股票代碼：

```go
func (ne *NarrativeEngine) avgSectorReturn(ds *replay.Dataset, date time.Time, window int, sectors []string) float64 {
    for _, sector := range sectors {
        symbols, ok := sectorSymbolMap[sector]  // ← 使用英文鍵查找
        if !ok { continue }                      // ← 中文鍵永遠不匹配，直接跳過
        ...
    }
    if count == 0 { return math.NaN() }
}
```

結果：
- 所有板塊報酬返回 `NaN`
- `correct = 0`, `total = 0`
- `RecentError` 被強制設為 `0.5`（fallback 值）
- `HitRate = 0.5`，`Weight` 基於無效誤差計算

**修復方向**:
1. 統一模板 `Affected` 陣列使用英文板塊鍵（與 `sectorSymbolMap` 一致）
2. 或在前端/後端增加板塊名稱翻譯層

---

### 🔴 P0-2: `recent_prediction` 欄位從未被賦值

**位置**: `internal/narrative/types.go:64` + `knowledge_base.go:200-296`  
**影響**: 前端類型聲明包含此欄位，但後端始終為 `0`

**問題描述**:

`InvestmentModel` 結構定義了 `RecentPrediction float64`，但：
- 9 個模型初始化時均未設置此值（默認為 `0`）
- `EvaluateModels` 未計算或更新此欄位
- 前端 `renderNarrativePage` 雖在類型定義中聲明，但渲染時**未使用**此欄位

從金融工程角度，`RecentPrediction` 應代表模型對下一期大盤或板塊的預測方向/幅度。此欄位完全未實作。

**修復方向**:
1. 明確 `RecentPrediction` 的語義（預測什麼？如何計算？）
2. 在 `EvaluateModels` 中計算並更新，或從類型中移除

---

### 🔴 P0-3: 前端 `MODEL_NAME_MAP` 覆蓋率僅 33%

**位置**: `/api/dashboard/agent-names`（取代 `web/static/js/names.js`，2026-06 — 改由後端讀取 `configs/agents.json` 提供單一權威來源）  
**影響**: 6 個投資模型名稱無法被前端正確顯示

**問題描述**:

後端定義了 **9 個投資模型**：

| # | 模型 ID | 名稱 | 前端映射 |
|---|---------|------|---------|
| 1 | `hawkish_fed_model` | 鷹派聯準會模型 | ✅ 有 |
| 2 | `ai_supercycle_model` | AI 超級週期模型 | ✅ 有 |
| 3 | `geopolitical_hedge_model` | 地緣政治避險模型 | ✅ 有 |
| 4 | `taiwan_political_risk_model` | 台灣地緣風險模型 | ❌ **缺失** |
| 5 | `semiconductor_cycle_model` | 半導體週期模型 | ❌ **缺失** |
| 6 | `seasonal_model` | 季節性輪動模型 | ❌ **缺失** |
| 7 | `election_model` | 選舉週期模型 | ❌ **缺失** |
| 8 | `retail_divergence_model` | 散戶與法人背離 | ❌ **缺失** |
| 9 | `earnings_surprise_model` | 財報驚喜驅動 | ❌ **缺失** |

`modelName()` 函數對缺失的模型名稱會原樣返回，但用戶體驗受損（無本地化/無別名）。

**修復方向**: 補齊 `MODEL_NAME_MAP` 中缺失的 6 個模型名稱映射。

---

## 三、P1 中等缺陷

### 🟡 P1-1: `EvaluateModels` 硬編碼常量未使用參數配置

**位置**: `internal/narrative/knowledge_base.go:456-463`  
**影響**: 參數系統的 `ModelLookbackDays` 和 `ModelHoldWindowDays` 成為死配置

**問題描述**:

```go
const lookback = 30       // ← 硬編碼
const holdWindow = 5      // ← 硬編碼
```

但參數系統已定義了：
- `NarrativeParameters.ModelLookbackDays`（默認值：`10`）
- `NarrativeParameters.ModelHoldWindowDays`（默認值：`5`）

**矛盾點**: `lookback = 30` 與參數默認值 `10` 不一致。運營人員調整參數不會產生實際效果。

**修復方向**: 改為從 `config.GetParametersConfig().Narrative` 讀取。

---

### 🟡 P1-2: 因果鏈的 `favored_sectors` / `avoided_sectors` 未被前端消費

**位置**: `web/static/js/pages/narrative.js:317-343`  
**影響**: 用戶無法直觀看到因果鏈聚合後的板塊結論

**問題描述**:

後端 `CausalChain` 類型包含：
```go
FavoredSectors  []string `json:"favored_sectors"`
AvoidedSectors  []string `json:"avoided_sectors"`
```

但前端渲染因果鏈時，只顯示了 `steps`（每個步驟的 `affected` 板塊），沒有顯示頂層聚合的 `favored_sectors` 和 `avoided_sectors`。

**修復方向**: 在因果鏈卡片頂部增加「看多板塊 / 看空板塊」摘要區域。

---

### 🟡 P1-3: `sectorSymbolMap` 缺少 `technology` 和 `traditional` 鍵

**位置**: `internal/narrative/knowledge_base.go:155-169`  
**影響**: `retail_divergence_model` 和 `earnings_surprise_model` 的板塊評估失敗

**問題描述**:

```go
// retail_divergence_model:
AvoidedSectors: []string{"technology", "semiconductor"},
// earnings_surprise_model:
AvoidedSectors: []string{"traditional"},
```

但 `sectorSymbolMap` 中沒有 `"technology"` 和 `"traditional"` 的定義。

**修復方向**: 為缺失的板塊鍵補充對應的股票代碼列表，或統一板塊命名規範。

---

## 四、P2 輕微缺陷

### 🟢 P2-1: 前端模型渲染未顯示 `hit_rate`

**位置**: `web/static/js/pages/narrative.js:351-384`  
**影響**: 用戶只能看到 `recent_error`，無法直接看到命中率

雖然 `hit_rate = 1 - recent_error`，但從用戶體驗角度，直接顯示命中率更直觀。

---

### 🟢 P2-2: `GetActiveModels` 每次 API 請求都重新評估模型

**位置**: `internal/monitoring/service/narrative.go:76-82`  
**影響**: 性能開銷大，IO 密集型操作不應在請求路徑上執行

```go
func (s *NarrativeService) GetActiveModels(themes []string) []narrative.InvestmentModel {
    replayPath := config.GetReplayDataPath(s.WorkDir)
    if err := s.NarrativeEngine.EvaluateModels(replayPath); err != nil {
        logging.Warn("narrative_service", "evaluate_models_warning", logging.Err(err))
    }
    return s.NarrativeEngine.ActiveModels(themes)
}
```

`EvaluateModels` 需要讀取回測 CSV 並計算板塊報酬，每次 API 請求都執行會導致：
- 不必要的磁盤 IO
- 重複的數值計算
- 請求延遲增加

**修復方向**: 將模型評估移至定時任務（cron/scheduler），API 只讀取已計算的結果。

---

### 🟢 P2-3: `templateHitRates` 使用模板命中率而非模型命中率

**位置**: `internal/narrative/ingestor.go:357-370`  
**影響**: `hitRateForTheme` 返回的是模板歷史命中率，而非模型實際命中率

在 `NarrativeEvent` 創建時，`HitRate` 被設為 `hitRateForTheme(theme)`，這是從 `DefaultTemplates()` 的 `HistoricalHitRate` 獲取的**靜態模板命中率**，而非 `InvestmentModel.HitRate`（後者基於實際回測數據動態計算）。

這在設計上可能有意（模板命中率作為先驗信心），但需要文檔說明兩者的區別。

---

## 五、參數引用規範檢查

### 5.1 參數配置 → 事件檢測器映射

| 參數 | 定義位置 | 消費位置 | 狀態 |
|------|---------|---------|------|
| `US10YChangeBpsThreshold` | `parameters.go:273` | `knowledge_base.go:675` | ✅ 正確引用 |
| `DXYChangePctThreshold` | `parameters.go:274` | `knowledge_base.go:675` | ✅ 正確引用 |
| `GeopoliticalGPRThreshold` | `parameters.go:275` | `knowledge_base.go:827` | ✅ 正確引用 |
| `OilChangePctThreshold` | `parameters.go:276` | `knowledge_base.go:865` | ✅ 正確引用 |
| `JPYChangePctThreshold` | `parameters.go:277` | `knowledge_base.go:899` | ✅ 正確引用 |
| `VIXLevelThreshold` | `parameters.go:278` | `knowledge_base.go:899` | ✅ 正確引用 |
| `AICapexSentimentThreshold` | `parameters.go:285` | `knowledge_base.go:752` | ✅ 正確引用 |
| `GoldChangePctThreshold` | `parameters.go:281` | `knowledge_base.go:827` | ✅ 正確引用 |
| `USDTWDChangePctThreshold` | `parameters.go:282` | `knowledge_base.go:980` | ✅ 正確引用 |
| `RetailMarginZScoreThreshold` | `parameters.go:284` | `knowledge_base.go:1260` | ✅ 正確引用 |
| `RetailInstitutionalDivergenceThreshold` | `parameters.go:288` | `knowledge_base.go:1260` | ✅ 正確引用 |
| `AICapexNegativeSentimentThreshold` | `parameters.go:289` | `knowledge_base.go:1054` | ✅ 正確引用 |
| `ModelLookbackDays` | `parameters.go:341` | — | ❌ **未使用**（硬編碼 30） |
| `ModelHoldWindowDays` | `parameters.go:342` | — | ❌ **未使用**（硬編碼 5） |
| `TaiwanStressDXYWeight` | `parameters.go:308` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressForeignWeight` | `parameters.go:310` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressVIXWeight` | `parameters.go:311` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressJPYWeight` | `parameters.go:312` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressGeoWeight` | `parameters.go:313` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressOilWeight` | `parameters.go:314` | `stress_calculator.go` | ✅ 正確引用 |
| `TaiwanStressGoldWeight` | `parameters.go:315` | `stress_calculator.go` | ✅ 正確引用 |

### 5.2 參數驗證覆蓋率

`ParametersConfig.Validate()` 方法（`parameters.go:1413+`）驗證了大部分參數區塊，但**未驗證 NarrativeParameters**：

- ❌ 未驗證 `ModelLookbackDays > 0`
- ❌ 未驗證 `ModelHoldWindowDays > 0`
- ❌ 未驗證 `TaiwanStress*Weight` 權重總和為 1
- ❌ 未驗證 `ConfidenceDeviationCeiling` 在合理範圍

---

## 六、功能完整性評估

### 6.1 已實作功能

| 功能 | 狀態 | 備註 |
|------|------|------|
| 事件檢測（20+ 種宏觀事件） | ✅ | `DetectEvents` 覆蓋全面 |
| 因果鏈匹配 | ✅ | `MatchChains` 正確實作 |
| 活躍模型篩選 | ✅ | `ActiveModels` 正確實作 |
| 模型權重自動調整 | ⚠️ | 算法正確，但輸入數據無效 |
| 模板命中率更新 | ⚠️ | 算法正確，但依賴無效誤差 |
| 壓力指數計算 | ✅ | `TaiwanStressCalculator` 完整 |
| 前端渲染 | ⚠️ | 部分欄位未消費 |

### 6.2 未實作/不完整功能

| 功能 | 缺失說明 | 優先級 |
|------|---------|--------|
| `RecentPrediction` 計算 | 欄位存在但從未賦值 | P0 |
| 板塊名稱中英映射 | 模板與 `sectorSymbolMap` 鍵不匹配 | P0 |
| 模型評估參數化 | `lookback`/`holdWindow` 硬編碼 | P1 |
| 因果鏈板塊摘要 | `favored_sectors`/`avoided_sectors` 未渲染 | P1 |
| 命中率前端展示 | `hit_rate` 未顯示 | P2 |
| 模型評估緩存 | 每次請求重新計算 | P2 |

---

## 七、修復建議（按優先級排序）

### 第一輪：P0 阻塞性修復

1. **統一板塊命名規範**
   - 方案 A：將模板 `Steps[].Affected` 改為英文鍵（推薦）
   - 方案 B：在 `sectorSymbolMap` 中增加中文鍵映射
   - 成本：方案 A 影響面小，方案 B 更符合前端顯示需求

2. **實作 `RecentPrediction`**
   - 定義語義：模型對下一期 favored_sectors 相對於 avoided_sectors 的超額報酬預測
   - 在 `EvaluateModels` 中計算：使用最後一期數據的板塊報酬差作為預測值

3. **補齊前端 `MODEL_NAME_MAP`**
   - 增加 6 個缺失的模型名稱映射

### 第二輪：P1 重要修復

4. **參數化模型評估窗口**
   ```go
   params := config.GetParametersConfig().Narrative
   lookback := params.ModelLookbackDays.Value
   holdWindow := params.ModelHoldWindowDays.Value
   ```

5. **前端因果鏈增加板塊摘要**
   - 在因果鏈卡片頂部增加 `favored_sectors` / `avoided_sectors` 的 badge 顯示

6. **補充 `sectorSymbolMap` 缺失鍵**
   - 為 `technology` 和 `traditional` 板塊定義對應股票代碼

### 第三輪：P2 優化修復

7. **增加 `hit_rate` 前端展示**
8. **將 `EvaluateModels` 移至定時任務**
9. **補充 `ParametersConfig.Validate()` 對 NarrativeParameters 的驗證**

---

## 八、附錄

### A. 投資模型定義（後端）

```go
type InvestmentModel struct {
    ID               string   `json:"id"`
    Name             string   `json:"name"`
    Description      string   `json:"description"`
    Rationale        string   `json:"rationale"`
    ActiveThemes     []string `json:"active_themes"`
    FavoredSectors   []string `json:"favored_sectors"`
    AvoidedSectors   []string `json:"avoided_sectors"`
    RecentPrediction float64  `json:"recent_prediction"`  // ← 從未賦值
    RecentError      float64  `json:"recent_error"`
    HitRate          float64  `json:"hit_rate"`
    Weight           float64  `json:"weight"`
}
```

### B. 板塊映射對照表

| 英文鍵 | 中文名稱 | `sectorSymbolMap` | 模板中使用 |
|--------|---------|-------------------|-----------|
| `financials` | 金融 | ✅ | ✅ |
| `high_dividend` | 高股息 | ✅ | ✅（中文） |
| `etf_rotation` | ETF 輪動 | ✅ | ✅（中文） |
| `ai_supply_chain` | AI 供應鏈 | ✅ | ✅（中文） |
| `semiconductor` | 半導體 | ✅ | ✅ |
| `pcb` | PCB | ✅ | ✅ |
| `thermal` | 散熱 | ✅ | ✅ |
| `shipping` | 航運 | ✅ | ✅ |
| `small_cap` | 小型股 | ✅ | ✅（中文） |
| `consumer` | 消費 | ✅ | ✅ |
| `defensive` | 防禦性板塊 | ✅ | ✅ |
| `technology` | 科技板塊 | ❌ **缺失** | ✅ |
| `traditional` | 傳產板塊 | ❌ **缺失** | ✅ |

### C. 相關檔案清單

- `internal/narrative/types.go` — 類型定義
- `internal/narrative/knowledge_base.go` — 模型引擎核心
- `internal/narrative/templates.go` — 因果模板定義
- `internal/narrative/ingestor.go` — 事件攝取器
- `internal/monitoring/api/narrative/handlers.go` — API 路由
- `internal/monitoring/service/narrative.go` — Service 層
- `internal/config/parameters.go` — 參數定義
- `internal/config/parameters_defaults.go` — 參數默認值
- `web/static/js/pages/narrative.js` — 前端渲染
- `web/static/js/shared/field_types.ts` — 前端類型
- `/api/dashboard/agent-names` — 名稱映射（取代 `web/static/js/names.js`，2026-06）
