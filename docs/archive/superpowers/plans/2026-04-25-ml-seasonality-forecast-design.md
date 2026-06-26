# ML 季節性預測 — 概念設計文件

**日期**: 2026-04-25
**狀態**: 概念設計
**觸發原因**: 季節性引擎 (`internal/industry/seasonality.go`) 已有規則型匹配，但缺乏基於歷史資料的預測能力

---

## 1. 現況分析

### 當前季節性引擎

```
internal/industry/seasonality.go
├── SeasonalPattern        # 規則定義（名稱、描述、日期範圍、影響產業）
├── DefaultSeasonalPatterns()  # 12 個硬編碼模式
├── GetActivePatterns()    # 依當前日期匹配活躍模式
├── GetPatternsForIndustry()   # 新增：依產業篩選
└── GetIndustryImpact()    # 新增：產業影響分析

internal/industry/seasonal_performance.go
├── SeasonalPerformance    # 表現追蹤（模式名稱、產業、統計數據）
├── TrackPerformance()     # 記錄實際表現
└── GetHistoricalStats()   # 查詢歷史統計
```

### 當前限制

1. **規則型匹配**：僅依賴預定義日期範圍（如 `01-15 ~ 04-15`），無法動態調整
2. **無預測能力**：只能回答「現在是什麼季節」，不能回答「下個月會是什麼季節」
3. **無信心度**：匹配結果是二元（是/否），沒有機率分佈
4. **無自適應**：氣候變遷、產業結構變化無法反映在規則中

---

## 2. 目標

### 短期目標（3 個月）

1. **歷史回測驗證**：基於已有 `SeasonalPerformance` 資料，驗證現有規則的準確率
2. **信心度評分**：為每個季節性匹配附加信心度（0-1）
3. **預測視窗**：支援查詢未來 30/60/90 天的季節性預測

### 中期目標（6 個月）

1. **輕量級模型**：實作基於統計的預測模型（無需外部 ML 框架）
2. **特徵工程**：自動從市場資料中提取季節性特徵
3. **模型評估**：自動化回測框架，比較規則型與統計型預測

### 長期目標（12 個月）

1. **整合外部資料**：氣象資料、節曆資料、產業週期指標
2. **線上學習**：模型根據新資料自動更新權重
3. **不確定性量化**：提供預測區間而非點估計

---

## 3. 技術方案

### 3.1 方案總覽

| 方案 | 複雜度 | 準確率預估 | 實作成本 | 適合階段 |
|------|--------|-----------|----------|----------|
| **加權移動平均** | 低 | 中 | 1-2 天 | 短期 |
| **馬可夫鏈** | 中 | 中高 | 3-5 天 | 短期-中期 |
| **Prophet (Facebook)** | 高 | 高 | 1-2 週 | 中期 |
| **LSTM 神經網路** | 很高 | 很高 | 2-4 週 | 長期 |

### 3.2 推薦方案：馬可夫鏈 + 加權移動平均（混合式）

**理由**：
1. 純 Go 實作，無需 Python/外部 ML 框架
2. 可解釋性高（投資領域需要可解釋性）
3. 資料需求低（3 個月資料即可開始）
4. 可漸進式升級

---

## 4. 架構設計

### 4.1 資料流

```
市場資料 (TWSE/券商)
    ↓
特徵提取器 (internal/industry/seasonal_features.go)
    ├── 價格動能 (20日/60日/120日報酬率)
    ├── 成交量變化 (月對月、年對年)
    ├── 產業輪動指標 (相對強度)
    ├── 外資買賣超 (月度累計)
    └── 匯率變動 (USD/TWD)
    ↓
馬可夫鏈模型 (internal/industry/markov_model.go)
    ├── 狀態定義 (bull/bear/sideways × 季節)
    ├── 轉移機率矩陣 (從歷史資料估計)
    └── 預測函式 (給定當前狀態，預測未來 N 天狀態分佈)
    ↓
預測結果 (internal/industry/seasonal_forecast.go)
    ├── 短期預測 (7-30 天)
    ├── 中期預測 (30-90 天)
    └── 信心度評分 (基於歷史準確率)
```

### 4.2 核心型別設計

```go
// internal/industry/markov_model.go

// TransitionMatrix 馬可夫鏈轉移機率矩陣
type TransitionMatrix struct {
    States []string                // 狀態列表
    Matrix [][]float64             // 轉移機率 [from][to]
    LastUpdated time.Time          // 最後更新時間
    SampleSize int                 // 樣本數
}

// MarkovModel 季節性馬可夫鏈模型
type MarkovModel struct {
    matrix *TransitionMatrix
    history []StateObservation    // 歷史狀態觀察
}

// StateObservation 狀態觀察記錄
type StateObservation struct {
    Date    time.Time
    State   string                // 當前狀態
    Features map[string]float64   // 當時特徵值
}

// Forecast 季節性預測結果
type Forecast struct {
    TargetDate time.Time
    StateProbabilities map[string]float64  // 狀態機率分佈
    Confidence float64                      // 整體信心度 (0-1)
    HistoricalAccuracy float64              // 歷史準確率
    TopFactors []FactorImportance           // 影響因子重要性
}

// FactorImportance 因子重要性
type FactorImportance struct {
    Name string
    Weight float64
    Direction string  // "positive" / "negative"
}
```

### 4.3 訓練流程

```go
// Train 從歷史資料訓練馬可夫鏈模型
func (m *MarkovModel) Train(observations []StateObservation) error {
    // 1. 計算狀態轉移次數
    // 2. 估計轉移機率矩陣（最大概似估計）
    // 3. 加入平滑化（避免零機率問題）
    // 4. 驗證矩陣收斂性
}

// Predict 預測未來 N 天的狀態分佈
func (m *MarkovModel) Predict(currentState string, days int) *Forecast {
    // 1. 從當前狀態開始
    // 2. 矩陣乘法計算 N 步轉移機率
    // 3. 計算信心度（基於樣本數與歷史準確率）
    // 4. 回傳預測結果
}
```

---

## 5. 特徵工程

### 5.1 季節性特徵列表

| 特徵 | 計算方式 | 預期季節性 | 資料來源 |
|------|----------|-----------|----------|
| 月效應 | 各月平均報酬率 | 1月/5月/10月較高 | TWSE 日行情 |
| 週效應 | 各週日平均報酬率 | 週一效應、週末效應 | TWSE 日行情 |
| 季效應 | 各季平均報酬率 | Q1/Q4 通常較強 | TWSE 日行情 |
| 成交量月效應 | 各月平均成交量 | 3月/9月/12月較高 | TWSE 成交量 |
| 外資買賣超月效應 | 各月累計買賣超 | 季底/年底較活躍 | 證交所外資資料 |
| 匯率月效應 | USD/TWD 月變動 | 報季效應 | 央行匯率資料 |

### 5.2 特徵計算實作

```go
// internal/industry/seasonal_features.go

// FeatureCalculator 季節性特徵計算器
type FeatureCalculator struct {
    priceData   []PricePoint
    volumeData  []VolumePoint
    foreignData []ForeignTradingPoint
}

// CalculateMonthlyEffect 計算月效應特徵
func (fc *FeatureCalculator) CalculateMonthlyEffect() map[int]float64 {
    // 回傳各月平均報酬率: {1: 0.023, 2: -0.011, ...}
}

// CalculateSeasonalMomentum 計算季節性動能
func (fc *FeatureCalculator) CalculateSeasonalMomentum() map[string]float64 {
    // 回傳各季動能指標: {"Q1": 0.045, "Q2": 0.012, ...}
}
```

---

## 6. 評估框架

### 6.1 回測方法

```
訓練集: 2020-01 ~ 2024-12 (5 年)
驗證集: 2025-01 ~ 2025-06 (6 個月)
測試集: 2025-07 ~ 2026-04 (目前)
```

### 6.2 評估指標

| 指標 | 計算方式 | 目標值 |
|------|----------|--------|
| 準確率 | 正確預測狀態數 / 總預測數 | > 60% |
| Brier Score | 預測機率與實際結果的平方差 | < 0.25 |
| Log Loss | 對數損失函數 | < 0.7 |
| 方向準確率 | 預測漲跌方向正確率 | > 55% |
| 夏普比率 | 預測策略的風險調整後報酬 | > 0.5 |

### 6.3 基準比較

| 模型 | 預期準確率 | 實作難度 |
|------|-----------|----------|
| 隨機猜測 | 33% (3 狀態) | 無 |
| 持久性模型（假設明天=今天） | 45-50% | 低 |
| 規則型季節性（現有） | 50-55% | 已完成 |
| 馬可夫鏈（目標） | 55-65% | 中 |
| Prophet（長期目標） | 60-70% | 高 |

---

## 7. 實作計畫

### Phase 1: 基礎建設（1-2 週）

- [ ] 建立 `internal/industry/seasonal_features.go`
- [ ] 實作特徵計算器（月效應、季效應、成交量效應）
- [ ] 建立 `internal/industry/markov_model.go`
- [ ] 實作馬可夫鏈訓練與預測函式
- [ ] 建立單元測試

### Phase 2: 整合與 API（1 週）

- [ ] 新增 `GET /api/industry/seasonal-forecast` API 端點
- [ ] 前端新增預測視覺化（機率分佈圖）
- [ ] 整合至產業詳細彈窗

### Phase 3: 評估與優化（1-2 週）

- [ ] 建立回測框架
- [ ] 跑歷史回測，計算評估指標
- [ ] 根據結果調整模型參數
- [ ] 文件與使用說明

---

## 8. 風險與限制

| 風險 | 影響 | 緩解措施 |
|------|------|----------|
| 資料不足（< 2 年） | 模型準確率低 | 使用規則型模型作為 fallback |
| 過擬合 | 歷史表現好但實戰差 | 嚴格訓練/驗證/測試分割 |
| 結構性斷點（如疫情） | 模型失效 | 加入斷點偵測，自動重新訓練 |
| 計算效能 | 即時預測延遲 | 預先計算，快取預測結果 |

---

## 9. 與現有系統的整合點

### 9.1 季節性引擎擴展

```
internal/industry/seasonality.go (現有)
    ↓ 擴展
internal/industry/seasonal_forecast.go (新增)
    ├── MarkovModel
    ├── FeatureCalculator
    └── ForecastEngine
```

### 9.2 API 擴展

```
GET /api/industry/seasonality        # 現有：當前活躍模式
GET /api/industry/seasonal-forecast  # 新增：未來預測
    ├── ?days=30                     # 預測天數
    ├── ?industry=半導體              # 特定產業
    └── ?confidence=true             # 包含信心度
```

### 9.3 前端擴展

- 產業詳細彈窗新增「季節性預測」分頁
- 顯示未來 30/60/90 天的狀態機率分佈
- 視覺化：堆疊長條圖或雷達圖

---

## 10. 結論

**推薦實作順序**：
1. 先完成特徵工程（1-2 天）
2. 實作馬可夫鏈基本模型（3-5 天）
3. 建立回測驗證框架（2-3 天）
4. 整合至 API 與前端（1 週）

**不建議**：
- 直接使用深度學習（資料不足、可解釋性差）
- 依賴外部 ML 服務（維運複雜、延遲高）
- 完全替換現有規則引擎（應採漸進式混合）

**成功關鍵**：
- 資料品質 > 模型複雜度
- 可解釋性 > 準確率（投資領域）
- 漸進式改進 > 一次性重構
