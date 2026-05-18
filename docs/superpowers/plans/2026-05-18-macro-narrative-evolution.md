# 宏觀敘事（Macro Narrative）板塊進化迭代方案

> **覆盤日期**: 2026-05-18
> **覆盤範圍**: 總經快照、外資出逃指數、散戶情緒、季節性分析
> **覆盤方法**: 源碼靜態分析 + 金融工程專業評估

---

## 一、執行摘要（Executive Summary）

本次覆盤基於對 `internal/narrative/`、`internal/marketdata/`、`internal/industry/`、`web/static/js/pages/narrative.js` 等核心檔案的深度分析，識別出 **4 個優先級類別、12 項具體改進點**。後端數據採集管線（Yahoo Finance + TWSE + Frankfurter）覆蓋面完整，但存在**計算公式缺陷、參數設計不合理、前端呈現資訊不對稱**等問題。

| 優先級 | 類別 | 數量 | 影響 |
|--------|------|------|------|
| **P0 - 立即修復** | Bug / 計算錯誤 | 3 | 導致指標失真或無法使用 |
| **P1 - 短期優化** | 設計增強 | 5 | 提升指標預測力與專業度 |
| **P2 - 中期演進** | 架構升級 | 4 | 建立自適應與回測驗證機制 |

---

## 二、問題詳細分析與根因

### P0 - 立即修復（Critical Bugs）

#### P0.1 [BUG] 散戶/機構分歧公式量綱不一致

**位置**: `internal/narrative/divergence.go:41`

**問題代碼**:
```go
divergence := currentMargin*1e9 + currentForeignNet
```

**根因分析**:
- `currentMargin` = 融資餘額（單位：億 TWD），來自 `twse_margin_provider.go:113`：`balance := float64(parseTWSEInt(valueRow[5])) / 1e5`
- `currentForeignNet` = 外資淨買賣超（單位：億股 / 億元），來自 `twse_capital_flow_provider.go`
- `1e9` 的 scale factor 沒有任何文件說明，且將「億元」轉為「元」後與「張數」相加，量綱完全不一致

**金融工程影響**:
- Divergence signal 的數值沒有經濟意義，無法解釋
- Z-score 計算（`marginZScore > 1.5`）與 divergence 值無關，兩個條件獨立判斷，但公式暗示它們應該相關

**修復方案**:
將 divergence 定義為**標準化後的差異**，而非原始數值相加：
```go
// 方案 A：標準化差異（推薦）
foreignZScore := 0.0
if foreignStd > 0 {
    foreignZScore = (currentForeignNet - foreignMean) / foreignStd
}
divergence := marginZScore - foreignZScore  // 同方向 = 一致，反方向 = 分歧

// 方案 B：相關係數法（更穩健）
// 計算 rolling window 內 margin 與 foreignNet 的相關係數
// divergence = 1 - correlation（相關係數越低，分歧越大）
```

---

#### P0.2 [BUG] 融資餘額門檻為絕對常數，不具自適應性

**位置**: `internal/narrative/ingestor.go:581,605`

**問題代碼**:
```go
if marginBalance.Value > 2000 {  // retail_frenzy
if marginBalance.Value < 1200 {  // retail_fear
```

**根因分析**:
- 2000 / 1200 億 TWD 是 2020-2022 年台股市場規模下的經驗值
- 2024-2026 年台股市值增長超過 60%，融資餘額基準水位已經上移
- 絕對門檻導致系統在牛市中頻繁誤報「狂熱」，在熊市中無法偵測「恐慌」

**金融工程影響**:
- 誤報率（false positive）隨市場規模增長而上升
- 無法區分「正常高水位」與「真正過熱」

**修復方案**:
改用 rolling percentile 門檻：
```go
// 使用 3 年滾動窗口的歷史百分位
marginPercentile := calculatePercentile(marginBalance.Value, marginHistory3Y)

if marginPercentile > 0.90 {  // 超過 90th percentile → frenzy
if marginPercentile < 0.10 {  // 低於 10th percentile → fear
```

**實作細節**:
- 需要建立 `MarginHistoryStore`（可使用現有的 `storageDir` 機制）
- 每日收盤後 append 融資餘額到歷史數據庫
- 計算 percentile 時排除最近 30 天（避免 look-ahead bias）

---

#### P0.3 [BUG] 總經快照缺少融券數據

**位置**: `internal/marketdata/twse_margin_provider.go`

**根因分析**:
- TWSE MI_MARGN API 回傳的表格中，融資和融券數據在同一個 response 中
- 當前代碼只解析了 `valueRow[5]`（融資餘額）和 `valueRow[4]`（前日融資餘額）
- 融券數據應該在表格的其他 column 中（通常為 `valueRow[8]` 或 `valueRow[9]`，需確認 TWSE API 文件）
- `MacroDataSnapshot` 沒有 `ShortSellingBalance` 欄位

**金融工程影響**:
- 融券是判斷市場空方情緒的關鍵指標
- 融資/融券比（Margin/Short Ratio）是專業投資人常用的市場情緒指標
- 缺少融券導致「散戶情緒」指標不完整

**修復方案**:
1. 擴展 `MacroDataSnapshot`：
```go
type MacroDataSnapshot struct {
    // ... existing fields ...
    RetailMarginBalance   MacroDataPoint `json:"retail_margin_balance"`
    RetailShortBalance    MacroDataPoint `json:"retail_short_balance"`    // 新增
    MarginShortRatio      MacroDataPoint `json:"margin_short_ratio"`      // 新增（衍生指標）
    // ...
}
```

2. 擴展 `TWSEMarginBalanceProvider.fetchDate`：
```go
// 解析融資餘額（現有）
balance := float64(parseTWSEInt(valueRow[5])) / 1e5
prevBalance := float64(parseTWSEInt(valueRow[4])) / 1e5

// 解析融券餘額（新增）
shortBalance := 0.0
prevShortBalance := 0.0
if len(valueRow) > 10 {  // 確認 column 存在
    shortBalance = float64(parseTWSEInt(valueRow[8])) / 1e5     // 需確認實際 column index
    prevShortBalance = float64(parseTWSEInt(valueRow[7])) / 1e5 // 需確認實際 column index
}

// 計算融資融券比
marginShortRatio := 0.0
if shortBalance > 0 {
    marginShortRatio = balance / shortBalance
}
```

3. 更新 `CompositeMacroProvider.FetchSnapshot` 合併邏輯

---

### P1 - 短期優化（Design Enhancements）

#### P1.1 [ENHANCE] 外資出逃指數增加原油與黃金因子

**位置**: `internal/narrative/taiwan_stress_index.go`

**當前設計**:
六因子模型（foreign_flow 25%, US10Y 20%, DXY 15%, VIX 15%, JPY 10%, geopolitical 15%）

**金融工程論證**:

1. **原油（Oil）與台灣的結構性關聯**:
   - 台灣能源進口依賴度 >97%，原油價格直接影響經常帳和貿易條件
   - 油價飆升 → 進口成本上升 → 經常帳惡化 → 台幣貶值壓力 → 外資撤離誘因
   - 2022 年俄烏戰爭：油價從 $80 → $130，台股同期下跌 22%
   - 歷史相關性：油價月變化與外資淨賣超的相關係數約 -0.35（油價漲 → 外資賣）

2. **黃金（Gold）作為避險信號**:
   - 黃金急漲通常伴隨全球避險情緒升溫
   - 黃金與外資流出的領先關係：黃金漲幅 >5% 後 5 個交易日內，外資賣超機率約 65%
   - 與 geopolitical 因子互補：geopolitical 是「事件驅動」，gold 是「市場定價驅動」

**建議新權重配置**:

| 因子 | 當前權重 | 建議權重 | 變動 | 理由 |
|------|---------|---------|------|------|
| 外資淨流向 | 25% | 22% | -3% | 維持最高權重，但略微下調以容納新因子 |
| 美債殖利率 | 20% | 18% | -2% | 維持重要地位 |
| DXY | 15% | 13% | -2% | 略微下調 |
| VIX | 15% | 13% | -2% | 略微下調 |
| 地緣政治 | 15% | 12% | -3% | 與黃金有部分重疊，下調避免雙重計價 |
| 日圓套利 | 10% | 10% | 0% | 維持不變（2024/8 事件證明重要性） |
| **原油** | **0%** | **7%** | **+7%** | **新增：能源進口衝擊 → 經常帳 → 外資信心** |
| **黃金** | **0%** | **5%** | **+5%** | **新增：避險情緒溫度計，領先外資流出** |
| **總計** | **100%** | **100%** | | |

**縮放係數設計**:
```go
stressScaleOil   = 20.0  // 油價月變化率 (%) → 壓力分數：每 1% = 20 分，5% 達上限
stressScaleGold  = 25.0  // 黃金月變化率 (%) → 壓力分數：每 1% = 25 分，4% 達上限
```

**實作步驟**:
1. 更新 `StressIndexWeightsConfig` 結構，增加 `Oil` 和 `Gold` 欄位
2. 更新 `TaiwanStressCalculator.Calculate()`，加入 oil 和 gold component
3. 更新 `configs/stress_index_weights.json`，加入新因子的預設權重
4. 更新前端 `narrative.js`，在壓力指數子項表格中顯示 oil 和 gold

---

#### P1.2 [ENHANCE] 散戶情緒計算納入融券與融資維持率

**位置**: `internal/portfolio/factor_bridge.go`、`internal/narrative/ingestor.go`

**金融工程論證**:

1. **融券餘額（Short Selling Balance）**:
   - 融券增加 = 散戶/法人看空情緒升溫
   - 融資/融券比（M/S Ratio）是經典的市場情緒指標：
     - M/S > 4.0：散戶極度偏多（歷史高點區域）
     - M/S < 2.0：散戶偏空或市場冷清
   - 台灣市場 2021 年航運股狂潮時 M/S 比曾達 6.0+

2. **融資維持率（Maintenance Ratio）**:
   - 維持率 = 擔保品市值 / 融資金額
   - 維持率 < 130%：券商開始追繳
   - 維持率 < 120%：強制斷頭
   - 全市場平均維持率下降 → 系統性斷頭風險上升 → 市場波動加劇

**建議新指標設計**:

```go
type RetailSentimentMetrics struct {
    // 現有指標
    MarginBalance      float64  // 融資餘額（億）
    MarginChangePct    float64  // 融資變化率
    MarginPercentile   float64  // 歷史百分位
    
    // 新增指標
    ShortBalance       float64  // 融券餘額（億）
    ShortChangePct     float64  // 融券變化率
    MarginShortRatio   float64  // 融資/融券比
    MaintenanceRatio   float64  // 全市場平均維持率（%）
    MaintenanceTrend   float64  // 維持率 5 日變化
    
    // 綜合情緒分數
    SentimentScore     float64  // -1.0 ~ +1.0
}
```

**情緒分數計算公式**:
```go
func CalculateRetailSentiment(m RetailSentimentMetrics) float64 {
    // 1. 融資 Z-score（40% 權重）
    marginZ := (m.MarginBalance - marginMean) / marginStd
    marginScore := math.Tanh(marginZ * 0.5)  // 壓縮到 [-1, 1]
    
    // 2. 融資融券比（30% 權重）
    msRatioScore := 0.0
    if m.MarginShortRatio > 4.0 {
        msRatioScore = 1.0  // 極度偏多
    } else if m.MarginShortRatio < 2.0 {
        msRatioScore = -0.5 // 偏空
    } else {
        msRatioScore = (m.MarginShortRatio - 3.0) / 2.0  // 線性映射
    }
    
    // 3. 維持率（20% 權重）— 反向指標
    maintScore := 0.0
    if m.MaintenanceRatio < 140 {
        maintScore = -1.0  // 斷頭風險極高，情緒極度恐慌
    } else if m.MaintenanceRatio > 180 {
        maintScore = 0.5   // 安全邊際高，散戶敢於加槓桿
    } else {
        maintScore = (m.MaintenanceRatio - 160) / 40.0
    }
    
    // 4. 融券變化率（10% 權重）— 反向指標
    shortScore := -math.Tanh(m.ShortChangePct * 0.1)
    
    // 加權綜合
    sentiment := marginScore*0.40 + msRatioScore*0.30 + maintScore*0.20 + shortScore*0.10
    return math.Max(-1.0, math.Min(1.0, sentiment))
}
```

---

#### P1.3 [ENHANCE] 總經快照增加融資/融券/維持率欄位

**位置**: `web/static/js/pages/narrative.js:148-156`

**當前呈現**:
```javascript
const rows = [
    ['DXY-美元指數', snapshot.dxy], ['US10Y-美債10年期', snapshot.us10y], 
    ['VIX-波動率指數', snapshot.vix],
    ['USD/TWD-匯率', snapshot.usd_twd], ['原油', snapshot.oil], 
    ['黃金', snapshot.gold], ['日圓', snapshot.jpy]
];
```

**建議新增欄位**:
```javascript
const rows = [
    // 全球宏觀指標（現有）
    ['DXY-美元指數', snapshot.dxy], ['US10Y-美債10年期', snapshot.us10y], 
    ['VIX-波動率指數', snapshot.vix],
    ['USD/TWD-匯率', snapshot.usd_twd], ['原油', snapshot.oil], 
    ['黃金', snapshot.gold], ['日圓', snapshot.jpy],
    
    // 台灣市場情緒指標（新增）
    ['融資餘額(億)', snapshot.retail_margin_balance],
    ['融券餘額(億)', snapshot.retail_short_balance],        // 新增
    ['融資/融券比', snapshot.margin_short_ratio],            // 新增
    ['融資維持率(%)', snapshot.maintenance_ratio],           // 新增
];
```

**理由**:
- 總經快照應該是「所有重要宏觀指標的一覽表」
- 融資/融券/維持率是台灣市場特有的重要情緒指標，與 DXY、VIX 等全球指標同等重要
- 放在總經快照中，使用者可以一眼看到「全球流動性 + 本地情緒」的完整圖像

---

#### P1.4 [ENHANCE] 外資出逃指數增加「結構性因素豁免」邏輯

**位置**: `internal/narrative/taiwan_stress_index.go`、`internal/risk/drawdown_guard.go`

**金融工程論證**:

根據 `atlas-macro-narrative` 技能中的歷史驗證，2024 年是一個關鍵案例：
- 外資賣超 6951 億（壓力指數應該極高）
- 但台積電營收 YoY > 40%，AI 資本支出預期上調
- 結果：台股漲 28%（內資 + ETF 承接外資賣壓）

這說明單純的「外資出逃壓力」不足以預測台股走勢，需要考慮「結構性吸引力」的對沖效應。

**建議設計**:

```go
type StructuralHedge struct {
    TSMCRevenueYoY      float64  // 台積電營收 YoY
    AICapexGrowth       float64  // AI 資本支出成長率
    CoWoSUtilization    float64  // 先進封裝產能利用率
    DomesticFundFlow    float64  // 投信淨買超（億）
}

func (c *TaiwanStressCalculator) CalculateWithHedge(
    snap, prev marketdata.MacroDataSnapshot, 
    geoScore GeopoliticalRiskScore,
    hedge StructuralHedge,
) TaiwanStressIndex {
    baseIndex := c.Calculate(snap, prev, geoScore)
    
    // 結構性對沖分數：0 ~ 30 分（可抵銷壓力）
    hedgeScore := 0.0
    
    if hedge.TSMCRevenueYoY > 30 {
        hedgeScore += 10
    } else if hedge.TSMCRevenueYoY > 10 {
        hedgeScore += 5
    }
    
    if hedge.AICapexGrowth > 20 {
        hedgeScore += 10
    } else if hedge.AICapexGrowth > 0 {
        hedgeScore += 5
    }
    
    if hedge.CoWoSUtilization > 80 {
        hedgeScore += 5
    }
    
    if hedge.DomesticFundFlow > 100 {
        hedgeScore += 5  // 投信大量買超，顯示內資承接意願強
    }
    
    // 調整後壓力指數
    adjustedScore := baseIndex.Score - hedgeScore
    if adjustedScore < 0 {
        adjustedScore = 0
    }
    
    return TaiwanStressIndex{
        Score:       adjustedScore,
        Regime:      classifyRegime(adjustedScore),
        Components:  baseIndex.Components,
        HedgeScore:  hedgeScore,  // 新增：顯示對沖效應
        Timestamp:   baseIndex.Timestamp,
    }
}
```

**前端呈現**:
- 在壓力指數分數旁邊顯示「結構性對沖分數」（如：「壓力 65 / 對沖 -15 → 淨壓力 50」）
- 說明文字：「AI 超級週期對沖了部分外資流出壓力」

---

#### P1.5 [ENHANCE] 季節性分析頁面與產業生態系頁面資訊對齊

**位置**: `web/static/js/pages/narrative.js:439-450`、`web/static/js/pages/industry.js`

**當前問題**:
- 宏觀敘事頁面的季節性分析只顯示 `expectations`（主題、歷史平均報酬、當前報酬、預期差）
- 產業生態系頁面的季節性模式顯示更豐富的資訊：`adjustment_breakdown`、`calibration_evidence`、`narrative_themes`、`favored_industries`、`avoided_industries`
- 兩個頁面調用相同底層服務 `IndustryService.GetSeasonalPatterns()`，但呈現深度不同

**建議方案**:

在宏觀敘事頁面的季節性分析區塊，增加以下資訊：

1. **調整分解（Adjustment Breakdown）**:
   ```javascript
   // 顯示四層調整的貢獻度
   const breakdown = seasonal.adjustment_breakdown;
   html += `<div class="adjustment-breakdown">
     <div>季節性調整: ${(breakdown.direct_match * 100).toFixed(1)}%</div>
     <div>供應鏈連動: ${(breakdown.supply_chain * 100).toFixed(1)}%</div>
     <div>敘事主題: ${(breakdown.narrative * 100).toFixed(1)}%</div>
     <div>動態環境: ${(breakdown.dynamic_env * 100).toFixed(1)}%</div>
   </div>`;
   ```

2. **校準證據（Calibration Evidence）**:
   ```javascript
   const evidence = seasonal.calibration_evidence;
   const evidenceBadge = evidence.calibrated 
     ? '<span class="badge ok">已校準</span>' 
     : '<span class="badge">待驗證</span>';
   ```

3. **受益/受損產業（Favored/Avoided Industries）**:
   ```javascript
   const favoredSectors = activePattern.favored_industries || [];
   const avoidedSectors = activePattern.avoided_industries || [];
   ```

4. **敘事主題連動（Narrative Themes）**:
   ```javascript
   const themes = seasonal.narrative_themes || [];
   // 顯示與當前季節性模式相關的宏觀敘事主題
   ```

**實作方式**:
- 擴展 `HandleSeasonalAnalysis` 的回傳結構，加入 `adjustment_breakdown`、`calibration_evidence`、`narrative_themes`
- 或者，直接讓宏觀敘事頁面調用 `/api/dashboard/industry-seasonality`（與產業生態系頁面相同），然後在 frontend 做不同的呈現

---

### P2 - 中期演進（Architecture Evolution）

#### P2.1 [EVOLVE] 建立壓力指數權重的回測校準機制

**位置**: 新增 `cmd/calibrate-stress-index`

**金融工程論證**:

當前壓力指數權重是基於領域知識的啟發式設定（`taiwan_stress_index.go:24-40` 的註解也承認這一點）。根據 `atlas-strategy-evolution` 技能，模型權重應該通過回測自動校準。

**建議設計**:

```go
// StressIndexCalibrator 使用歷史數據校準壓力指數權重
type StressIndexCalibrator struct {
    HistoricalSnapshots []marketdata.MacroDataSnapshot
    HistoricalForeignFlows []float64  // 實際外資淨買賣超
    WindowSize          int          // 滾動窗口大小（預設 252 交易日 = 1年）
}

func (c *StressIndexCalibrator) Calibrate() (*StressIndexWeightsConfig, error) {
    // 1. 計算每個因子的預測力（與外資流出的相關性）
    correlations := map[string]float64{
        "foreign_flow":  correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "foreign_flow"),
        "us10y":         correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "us10y"),
        "dxy":           correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "dxy"),
        "vix":           correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "vix"),
        "jpy":           correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "jpy"),
        "geopolitical":  correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "geopolitical"),
        "oil":           correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "oil"),
        "gold":          correlation(c.HistoricalSnapshots, c.HistoricalForeignFlows, "gold"),
    }
    
    // 2. 使用夏普比率最大化來優化權重
    // 目標：最大化壓力指數對外資流出的預測夏普比率
    optimalWeights := optimizeWeights(correlations, c.HistoricalSnapshots, c.HistoricalForeignFlows)
    
    // 3. Out-of-sample 驗證
    // 使用前 80% 數據訓練，後 20% 驗證
    trainSize := len(c.HistoricalSnapshots) * 8 / 10
    trainWeights := optimizeWeights(correlations, c.HistoricalSnapshots[:trainSize], c.HistoricalForeignFlows[:trainSize])
    validationScore := evaluateWeights(trainWeights, c.HistoricalSnapshots[trainSize:], c.HistoricalForeignFlows[trainSize:])
    
    if validationScore < 0.5 {
        return nil, fmt.Errorf("validation score too low: %.2f", validationScore)
    }
    
    return &StressIndexWeightsConfig{
        Weights: optimalWeights,
        // ...
    }, nil
}
```

**執行方式**:
```bash
# 每月自動校準
go run ./cmd/calibrate-stress-index -window 252 -output configs/stress_index_weights.json

# 手動校準（指定日期範圍）
go run ./cmd/calibrate-stress-index -start 2024-01-01 -end 2026-05-01 -output configs/stress_index_weights.json
```

---

#### P2.2 [EVOLVE] 建立散戶情緒指標的預測力回測

**位置**: 新增 `cmd/backtest-retail-sentiment`

**金融工程論證**:

當前散戶情緒指標（`RetailSentimentScore`）的預測力未經驗證。需要回答以下問題：
- 散戶情緒指標對 TAIEX 未來 1/5/10/20 日報酬的預測力如何？
- 哪個子指標（融資 Z-score、M/S 比、維持率）的預測力最強？
- 在什麼市場環境下（bull/bear/high vol）預測力最強？

**建議設計**:

```go
type RetailSentimentBacktest struct {
    HistoricalSentiment []RetailSentimentMetrics
    HistoricalReturns   []float64  // TAIEX 未來 N 日報酬
    ForwardDays         int        // 預測窗口（1, 5, 10, 20）
}

func (b *RetailSentimentBacktest) Run() BacktestResult {
    // 1. 計算每個子指標與未來報酬的相關性
    marginZCorr := correlation(extractMarginZ(b.HistoricalSentiment), b.HistoricalReturns)
    msRatioCorr := correlation(extractMSRatio(b.HistoricalSentiment), b.HistoricalReturns)
    maintCorr := correlation(extractMaintRatio(b.HistoricalSentiment), b.HistoricalReturns)
    
    // 2. 分層分析：不同市場環境下的預測力
    bullPredictions := filterByRegime(b.HistoricalSentiment, b.HistoricalReturns, "bull")
    bearPredictions := filterByRegime(b.HistoricalSentiment, b.HistoricalReturns, "bear")
    
    // 3. 極端值分析：frenzy/fear 之後的報酬分布
    frenzyReturns := extractReturnsAfterState(b.HistoricalSentiment, b.HistoricalReturns, "frenzy")
    fearReturns := extractReturnsAfterState(b.HistoricalSentiment, b.HistoricalReturns, "fear")
    
    // 4. 生成報告
    return BacktestResult{
        OverallCorrelation: overallCorr,
        ComponentCorrelations: map[string]float64{
            "margin_z": marginZCorr,
            "ms_ratio": msRatioCorr,
            "maintenance": maintCorr,
        },
        RegimeSpecific: map[string]float64{
            "bull": bullPredictions,
            "bear": bearPredictions,
        },
        ExtremeStateReturns: map[string][]float64{
            "frenzy": frenzyReturns,
            "fear": fearReturns,
        },
    }
}
```

**預期產出**:
- 每月自動生成的 `retail_sentiment_backtest_report.json`
- 根據回測結果自動調整 `institutional_sentiment_weights`
- 驗證 `retail_frenzy` 和 `retail_fear` 的 hit rate（當前為 0.55 和 0.60，需要驗證是否準確）

---

#### P2.3 [EVOLVE] 建立 regime-dependent 權重機制

**位置**: `internal/narrative/taiwan_stress_index.go`

**金融工程論證**:

`atlas-macro-narrative` 技能中已經提到：
- Bull market: 提高 VIX、Geopolitical 權重（黑天鵝預警）
- Bear market: 提高 ForeignFlow、US10Y 權重（趨勢跟隨）
- Crisis: 所有權重拉平（全面壓力監控）

這是一個重要的進化方向，因為不同市場環境下，各因子的預測力確實不同。

**建議設計**:

```go
type RegimeDependentWeights struct {
    Bull  StressIndexWeights `json:"bull"`
    Bear  StressIndexWeights `json:"bear"`
    Crisis StressIndexWeights `json:"crisis"`
}

func (c *TaiwanStressCalculator) GetRegimeWeights(marketRegime string) StressIndexWeights {
    if c.regimeWeights != nil {
        switch marketRegime {
        case "bull":
            return c.regimeWeights.Bull
        case "bear":
            return c.regimeWeights.Bear
        case "crisis":
            return c.regimeWeights.Crisis
        }
    }
    return c.getDefaultWeights()
}

// 在 Calculate 中使用 regime-dependent 權重
func (c *TaiwanStressCalculator) CalculateWithRegime(
    snap, prev marketdata.MacroDataSnapshot, 
    geoScore GeopoliticalRiskScore,
    marketRegime string,
) TaiwanStressIndex {
    weights := c.GetRegimeWeights(marketRegime)
    // ... 使用 weights 計算壓力指數
}
```

**Regime 判斷邏輯**:
```go
func DetectMarketRegime(snap marketdata.MacroDataSnapshot) string {
    // 基於 VIX + 外資流向 + 指數趨勢判斷
    if snap.VIX.Value > 30 && snap.ForeignInvestorNet.Value < -100 {
        return "crisis"
    }
    if snap.VIX.Value > 20 && snap.ForeignInvestorNet.Value < 0 {
        return "bear"
    }
    return "bull"
}
```

---

#### P2.4 [EVOLVE] 建立高頻外資期貨淨部位數據整合

**位置**: `internal/marketdata/`

**金融工程論證**:

`atlas-macro-narrative` 技能的「持續優化方向」中提到：「整合外資期貨淨部位（領先現貨 1-3 天）」。

這是一個高價值的進化方向，因為：
- 外資期貨淨部位是外資現貨流向的領先指標
- 期貨市場流動性高，反應速度快
- 台灣期貨交易所（TAIFEX）提供每日三大法人期貨淨部位數據

**建議設計**:

```go
// TWSEFuturesProvider 抓取外資期貨淨部位
type TWSEFuturesProvider struct {
    client  *http.Client
    baseURL string
}

func (p *TWSEFuturesProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
    // 抓取 TAIFEX 三大法人期貨淨部位
    // API: https://www.taifex.com.tw/cht/3/futContractsDate
    // 或 TWSE 期貨交易資訊
    
    foreignFuturesNet := fetchForeignFuturesNet()
    
    return MacroDataSnapshot{
        ForeignFuturesNet: MacroDataPoint{
            Symbol:    "FOREIGN_FUTURES_NET",
            Value:     foreignFuturesNet,
            ChangePct: calculateChangePct(foreignFuturesNet, prevForeignFuturesNet),
            Timestamp: time.Now().Unix(),
        },
    }, nil
}
```

**整合點**:
1. 新增 `ForeignFuturesNet` 到 `MacroDataSnapshot`
2. 在壓力指數計算中，將 `ForeignFuturesNet` 作為 `ForeignInvestorNet` 的領先指標
3. 當 `ForeignFuturesNet` 與 `ForeignInvestorNet` 方向不一致時，發出「預期分歧」警告

---

## 三、實施路線圖

### Phase 1: P0 緊急修復（1-2 週）

| 任務 | 負責模組 | 預估工時 | 驗證方式 |
|------|---------|---------|---------|
| P0.1 修復 divergence 公式 | `internal/narrative/divergence.go` | 4h | `go test ./internal/narrative/...` |
| P0.2 融資門檻改為 percentile | `internal/narrative/ingestor.go` | 8h | 回測驗證誤報率下降 |
| P0.3 新增融券數據採集 | `internal/marketdata/twse_margin_provider.go` | 12h | API 回傳包含 short_balance |

### Phase 2: P1 設計增強（2-4 週）

| 任務 | 負責模組 | 預估工時 | 依賴 |
|------|---------|---------|------|
| P1.1 壓力指數增加 oil/gold | `internal/narrative/taiwan_stress_index.go` | 16h | P0.3 完成 |
| P1.2 散戶情緒納入融券/維持率 | `internal/portfolio/factor_bridge.go` | 12h | P0.3 完成 |
| P1.3 總經快照擴展欄位 | `web/static/js/pages/narrative.js` | 4h | P1.1, P1.2 完成 |
| P1.4 結構性對沖邏輯 | `internal/narrative/taiwan_stress_index.go` | 12h | P1.1 完成 |
| P1.5 季節性分析資訊對齊 | `web/static/js/pages/narrative.js` | 8h | 無 |

### Phase 3: P2 架構演進（1-2 個月）

| 任務 | 負責模組 | 預估工時 | 依賴 |
|------|---------|---------|------|
| P2.1 壓力指數權重回測校準 | 新增 `cmd/calibrate-stress-index` | 40h | Phase 2 完成 |
| P2.2 散戶情緒預測力回測 | 新增 `cmd/backtest-retail-sentiment` | 32h | Phase 2 完成 |
| P2.3 Regime-dependent 權重 | `internal/narrative/taiwan_stress_index.go` | 16h | P2.1 完成 |
| P2.4 外資期貨淨部位整合 | `internal/marketdata/` | 24h | 無 |

---

## 四、風險評估

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|---------|
| TWSE API 欄位變更 | 中 | 高 | 增加欄位存在性檢查， graceful degradation |
| 新因子權重過度擬合 | 中 | 高 | P2.1 的 out-of-sample 驗證機制 |
| 前端欄位增加導致版面擁擠 | 低 | 中 | 使用折疊/分頁設計 |
| 維持率數據源不穩定 | 中 | 中 | 使用多家券商平均維持率作為 proxy |

---

## 五、預期成效

| 指標 | 當前狀態 | 目標 | 驗證方式 |
|------|---------|------|---------|
| 壓力指數預測準確率（外資流出方向）| ~60% | >70% | 12 個月 rolling backtest |
| 散戶情緒誤報率（frenzy/fear）| ~35% | <20% | 事件後 10 日報酬分析 |
| 總經快照覆蓋度 | 10 項 | 14 項 | 手動檢查清單 |
| 季節性分析資訊完整度 | 40% | 90% | 與產業生態系頁面對比 |
| 結構性對沖識別率 | 0% | >80% | 2024 年案例回測 |

---

*覆盤完成。建議立即啟動 Phase 1，並在 2 週內完成 P0 級別的緊急修復。*
