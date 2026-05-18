# 宏觀敘事（Macro Narrative）板塊進化迭代方案 v2.0

> **版本**: 2.0（基於專業金融工程審計修正）
> **審計日期**: 2026-05-18
> **前一版本問題**: v1.0 存在多重共線性處理不當、實施階段順序顛倒、缺少事件驗證框架、結構性對沖類別錯誤、忽略非正態分布特性等 8 個方法論缺陷
> **核心修正**: 重新定義客觀函數、重建實施階段、引入 PCA 降維、建立 5 事件驗證框架、改用累積流量標準化

---

## 零、第一步：定義客觀函數（Objective Function）

**在寫任何一行代碼之前，必須先回答這個問題：外資出逃指數到底在衡量什麼？**

### 當前定義（存在問題）
```
權重 = 因子對外資撤離的「直接性（DIRECTNESS）」
```
這不是一個可驗證的客觀函數。你如何量化「直接性」？你如何驗證「直接性」是否正確？

### 修正後的定義

**外資出逃指數衡量的是：在未來 5 個交易日內，外資出現異常淨賣超（定義為滾動 1 年窗口中 top 15% 的週累積淨賣超）的條件機率。**

這是一個**分類問題（classification problem）**，不是一個回歸問題：

```
目標變數 Y ∈ {0, 1}
Y = 1 if 未來 5 日累積外資淨賣超 ∈ top 15% of rolling 1-year distribution
Y = 0 otherwise
```

**為什麼這樣定義？**
- 「異常」是有統計意義的：top 15% 對應約 1.0σ（在正態分布假設下），是業界常用的異常門檻
- 「5 個交易日」：太短（1 日）噪音過大，太長（20 日）失去預警價值。5 日是一週的市場節奏
- 「分類而非回歸」：風險管理的核心問題是「會不會出事」，不是「出多少事」
- 這使得壓力指數可以被校準為**機率**（0-100 分 = 0%-100% 機率），對使用者有直觀意義

---

## 一、實施階段（修正順序）

### Phase 0: 修復關鍵 Bug + 建立 Baseline（1 週）

**目標**：確保現有系統沒有計算錯誤，建立可驗證的基準線

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P0.1** 修復 divergence 公式 | 改用 5 日累積流量 Z-score 差異 | 無 |
| **P0.2** 修復融資門檻 | 改用 rolling percentile + 加速度雙條件 | 無 |
| **P0.3** 補齊融券數據 | 擴展 TWSE API 解析、新增 MacroDataSnapshot 欄位 | 無 |
| **P0.4** 建立 event-based 驗證框架 | 定義 5 個歷史壓力事件的測試案例 | 無 |

#### P0.1 [FIX] Divergence 公式改用累積流量標準化

**原代碼問題**（`divergence.go:41`）：
```go
divergence := currentMargin*1e9 + currentForeignNet
// ─────┬─────        ────┬────
//  億元 TWD           張數（或億元）
//  量綱不一致，且 1e9 無文件說明
```

**修正方案**：改用 5 日累積流量的 Z-score 差異

```go
// DivergenceDetector 追蹤滾動累積流量
type DivergenceDetector struct {
    marginCum5  []float64  // margin 5日累積變化
    foreignCum5 []float64  // foreign 5日累積淨買賣超
    window      int        // 累積窗口（預設 5）
}

func (d *DivergenceDetector) Update(marginBalance, prevMargin, foreignNet, prevForeignNet float64) {
    // 計算單日變化
    marginDelta := marginBalance - prevMargin
    foreignDelta := foreignNet - prevForeignNet

    // 維護 5 日累積序列
    // marginCum5 和 foreignCum5 以 rolling sum 方式維護

    d.marginCum5 = append(d.marginCum5, computeRollingSum(d.marginDeltas, d.window))
    d.foreignCum5 = append(d.foreignCum5, computeRollingSum(d.foreignDeltas, d.window))
}

func (d *DivergenceDetector) RetailDivergenceAndMarginZScore() (float64, float64) {
    if len(d.marginCum5) < 60 {
        return 0, 0
    }
    // 對 5 日累積流量做 Z-score（比日度 Z-score 更穩定）
    marginZ := computeZScore(d.marginCum5[len(d.marginCum5)-1], d.marginCum5[:len(d.marginCum5)-1])
    foreignZ := computeZScore(d.foreignCum5[len(d.foreignCum5)-1], d.foreignCum5[:len(d.foreignCum5)-1])

    // 分歧 = 兩個 Z-score 方向相反
    // marginZ > 0 (散戶加槓桿) && foreignZ < 0 (外資流出) → 正分歧
    // marginZ < 0 (散戶去槓桿) && foreignZ > 0 (外資流入) → 負分歧
    divergence := 0.0
    if marginZ > 0 && foreignZ < 0 {
        divergence = min(marginZ, -foreignZ) // 分歧強度 = 較弱的那一邊
    } else if marginZ < 0 && foreignZ > 0 {
        divergence = -min(-marginZ, foreignZ)
    }

    return divergence, marginZ
}
```

**為什麼用 5 日累積而非日度？**
- 外資每日流向具有高波動性和自相關性（連續多日同向的機率 >50%）
- 日度 Z-score 假設獨立同分布（i.i.d.），但外資流向明顯違反此假設
- 5 日累積平滑了噪音，同時保持足夠的即時性

---

#### P0.2 [FIX] 融資門檻改用 percentile + 加速度雙條件

**原代碼問題**（`ingestor.go:581,605`）：
```go
if marginBalance.Value > 2000 { // 絕對常數，不隨市場規模調整
if marginBalance.Value < 1200 { // 2020 年的經驗值
```

**修正方案**：雙條件觸發

```go
func detectRetailFrenzyEventFromSnapshot(
    margin marketdata.MacroDataPoint,
    marginHistory []float64, // 3 年滾動歷史
    now time.Time,
) *NarrativeEvent {
    if margin.Symbol == "" {
        return nil
    }
    percentile := computePercentile(margin.Value, marginHistory)
    acceleration := computeAcceleration(margin.Value, marginHistory) // 5 日變化率的變化

    // 雙條件：高百分位 AND 加速度為正（融資還在加速增加）
    if percentile > 0.85 && acceleration > 0 {
        confidence := 0.55 + (percentile-0.85)*2.0 // 越高越有信心，上限 0.85
        if confidence > 0.85 {
            confidence = 0.85
        }
        return &NarrativeEvent{
            Theme:      "retail_frenzy",
            Sentiment:  1.0,
            Confidence: confidence,
            // ...
        }
    }
    return nil
}

func detectRetailFearEventFromSnapshot(
    margin marketdata.MacroDataPoint,
    marginHistory []float64,
    maintenanceRatio float64, // 全市場平均維持率
    now time.Time,
) *NarrativeEvent {
    if margin.Symbol == "" {
        return nil
    }
    percentile := computePercentile(margin.Value, marginHistory)
    acceleration := computeAcceleration(margin.Value, marginHistory)

    // 恐懼雙條件：低百分位 AND（加速下降 OR 維持率 < 140%）
    isFear := (percentile < 0.15 && acceleration < 0) || maintenanceRatio < 140

    if isFear {
        confidence := 0.60
        if maintenanceRatio < 130 {
            confidence = 0.85 // 斷頭風險極高，信心度大幅提升
        }
        return &NarrativeEvent{
            Theme:      "retail_fear",
            Sentiment:  -1.0,
            Confidence: confidence,
            // ...
        }
    }
    return nil
}
```

**為什麼用雙條件？**
- 單純的 percentile 在長期牛市中會失效：90th percentile 不斷上移，永遠觸發不到
- acceleration（加速度）捕捉的是「轉折點」而非「絕對水位」
- 維持率是防止系統性風險（margin call cascade）的獨特信號，不應與融資餘額混為一談

---

#### P0.3 [FIX] 補齊融券數據採集

**修正方案**：擴展 `TWSEMarginBalanceProvider` 以同時解析融資和融券

```go
// marginSnapshot 同時包含融資和融券數據
type marginSnapshot struct {
    MarginBalance      float64
    MarginChangePct    float64
    ShortBalance       float64
    ShortChangePct     float64
    MaintenanceRatio   float64  // 全市場平均維持率（若可取得）
}

func (t *TWSEMarginBalanceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
    now := time.Now().UTC()
    for i := range 5 { // 減少為 5 天回溯（原來 7 天）
        dateStr := now.AddDate(0, 0, -i).Format("20060102")
        snap, err := t.fetchDateExpanded(ctx, dateStr)
        if err == nil {
            ts := time.Now().Unix()
            return MacroDataSnapshot{
                RetailMarginBalance: MacroDataPoint{
                    Symbol:    "TAIWAN_MARGIN_BALANCE",
                    Value:     snap.MarginBalance,
                    ChangePct: snap.MarginChangePct,
                    Timestamp: ts,
                },
                RetailShortBalance: MacroDataPoint{
                    Symbol:    "TAIWAN_SHORT_BALANCE",
                    Value:     snap.ShortBalance,
                    ChangePct: snap.ShortChangePct,
                    Timestamp: ts,
                },
                // ... existing fields
            }, nil
        }
    }
    return MacroDataSnapshot{}, fmt.Errorf("no margin data available")
}
```

**TWSE MI_MARGN API 的實際結構**（需在實作時確認）：
- Table 0（信用交易統計）: Data[2] 為「合計」行
- Column 5: 融資買進, Column 6: 融資賣出, Column 7: 融資餘額
- Column 10: 融券買進, Column 11: 融券賣出, Column 12: 融券餘額
- 這些 column index 在實作前必須透過實際 API 回應確認

---

#### P0.4 [NEW] 建立事件驅動驗證框架

在開始任何模型改進之前，必須先定義檢驗標準。

```go
// StressEventTestCase 定義一個歷史壓力事件的驗證案例
type StressEventTestCase struct {
    Name        string
    Date        string   // 事件日期 (YYYY-MM-DD)
    Window      int      // 評估窗口（交易日）
    Factors     map[string]float64  // 各因子的預期表現
    ExpectedRegime string // 預期壓力等級
    Rationale   string   // 金融工程論證
}

// 五個驗證案例
var ValidationCases = []StressEventTestCase{
    {
        Name: "Russia-Ukraine Invasion",
        Date: "2022-02-24",
        Window: 10,
        Factors: map[string]float64{
            "oil":           15.0,  // 油價飆升 >10%
            "gold":          5.0,   // 黃金避險需求
            "geopolitical":  85.0,  // 地緣風險急升
            "vix":           30.0,  // VIX 飆升
            "foreign_flow": -300.0, // 外資大幅賣超（億）
        },
        ExpectedRegime: "crisis",
        Rationale: "戰爭引發的全面避險：油金齊漲、VIX 飆升、外資撤離新興市場。壓力指數應達 crisis 等級。",
    },
    {
        Name: "Taiwan Margin Liquidation Cascade",
        Date: "2022-09-28",
        Window: 15,
        Factors: map[string]float64{
            "foreign_flow": -200.0,
            "vix":           28.0,
            "dxy":           1.5,   // 美元走強
        },
        ExpectedRegime: "high",
        Rationale: "融資斷頭潮：維持率跌破 140%，強制平倉導致螺旋下跌。外資同時流出。",
    },
    {
        Name: "BOJ Carry Trade Unwind",
        Date: "2024-08-05",
        Window: 5,
        Factors: map[string]float64{
            "jpy":           5.0,   // 日圓急速升值
            "vix":           65.0,  // VIX 創 COVID 後新高
            "foreign_flow": -500.0, // 外資恐慌性賣超
        },
        ExpectedRegime: "crisis",
        Rationale: "Carry trade 強制平倉：日圓單日升 3%+、VIX 飆至 65、全球資金回流日本。這是壓力指數的理想測試案例。",
    },
    {
        Name: "Trump Tariff Shock",
        Date: "2025-04-07",
        Window: 10,
        Factors: map[string]float64{
            "geopolitical":  75.0,
            "vix":           35.0,
            "foreign_flow": -400.0,
            "dxy":           1.0,
        },
        ExpectedRegime: "crisis",
        Rationale: "貿易戰升級：關稅威脅導致全球風險溢酬飆升，台灣作為出口導向經濟體首當其衝。台股單日跌 9.7%。",
    },
    {
        Name: "US-Iran Tensions with Oil Spike",
        Date: "2026-03-15",
        Window: 10,
        Factors: map[string]float64{
            "oil":           12.0,
            "gold":          4.0,
            "geopolitical":  70.0,
            "foreign_flow": -150.0,
        },
        ExpectedRegime: "high",
        Rationale: "中東局勢緊張推升油價，但 AI 基本面支撐下台股衝擊有限。壓力指數應達 high 但非 crisis。",
    },
}

// ValidateAgainstCases 以歷史案例驗證壓力指數
func ValidateAgainstCases(calc *TaiwanStressCalculator, cases []StressEventTestCase) *ValidationReport {
    report := &ValidationReport{Results: make([]CaseValidationResult, len(cases))}
    for i, tc := range cases {
        snap := reconstructSnapshot(tc.Date, tc.Factors)
        idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, GeopoliticalRiskScore{})

        report.Results[i] = CaseValidationResult{
            Case:         tc.Name,
            ActualScore:  idx.Score,
            ActualRegime: idx.Regime,
            Passed:       idx.Regime == tc.ExpectedRegime,
        }
    }
    return report
}
```

**驗證標準**：
- 5 個案例中至少 4 個的 regime 分類正確（80% pass rate）
- crisis case 的 score 不得低於 50（不能在危機時顯示「安全」）
- low stress case 的 score 不得高於 70（不能在太平時顯示「危機」）

---

### Phase 1: 校準現有模型（2 週）

**目標**：使用定義好的客觀函數校準現有六因子模型的權重，建立 baseline 績效

**關鍵原則**：在校準完成之前，不新增任何因子。

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P1.1** 壓力指數權重校準 CLI | 新增 `cmd/calibrate-stress-index` | P0.4 |
| **P1.2** Baseline 績效報告 | 產出現有模型的準確率、精確率、召回率 | P1.1 |
| **P1.3** 前端壓力指數呈現優化 | 增加因子貢獻分解視覺化 | 無 |

#### P1.1 壓力指數權重校準（以分類問題為目標）

```go
// CalibrateWeightsAsClassifier 將壓力指數校準為外資出逃事件的機率模型
func CalibrateWeightsAsClassifier(
    historicalSnapshots []marketdata.MacroDataSnapshot,
    historicalFlows []float64, // 外資淨買賣超
    window int, // 預測窗口（5 日）
    percentile float64, // 異常門檻（0.15 = top 15%）
) (*StressIndexWeightsConfig, *CalibrationReport) {

    // Step 1: 定義目標變數
    Y := make([]int, len(historicalSnapshots))
    for i := 0; i < len(historicalSnapshots)-window; i++ {
        cumFlow := sum(historicalFlows[i+1 : i+1+window])
        threshold := rollingPercentile(cumFlow, historicalFlows, 252, percentile)
        if cumFlow < threshold { // 淨賣超超過門檻 = 異常
            Y[i] = 1
        }
    }

    // Step 2: 使用 Logistic Regression 校準（而非線性回歸）
    // log(P(Y=1) / P(Y=0)) = β₀ + β₁·DXY + β₂·US10Y + ... + βₙ·factorₙ
    // 這自動確保輸出在 [0, 1] 範圍內（可解釋為機率）
    features := extractFeatures(historicalSnapshots)
    model := trainLogisticRegression(features, Y)

    // Step 3: 將 logistic regression 係數轉換為權重
    // 權重 = |βᵢ| / Σ|βⱼ|
    absSum := sumAbs(model.Coefficients)
    weights := StressIndexWeights{
        DXY:          abs(model.Coefficients[0]) / absSum,
        US10Y:        abs(model.Coefficients[1]) / absSum,
        ForeignFlow:  abs(model.Coefficients[2]) / absSum,
        VIX:          abs(model.Coefficients[3]) / absSum,
        JPY:          abs(model.Coefficients[4]) / absSum,
        Geopolitical: abs(model.Coefficients[5]) / absSum,
    }

    // Step 4: Cross-validation (time-series aware, no shuffle)
    cvScore := timeSeriesCrossValidation(features, Y, 5) // 5-fold

    // Step 5: 產出校準報告
    report := &CalibrationReport{
        BaselineWeights: getDefaultWeights(),
        CalibratedWeights: weights,
        PreCalibrationAccuracy: evaluateAccuracy(features, Y, getDefaultWeights()),
        PostCalibrationAccuracy: cvScore,
        FeatureImportance: model.Coefficients,
        // 最重要的：因子是否顯著？
        PValues: model.PValues,
        // 如果某個因子的 p-value > 0.05，它不應該留在模型中
        SignificantFactors: filterSignificant(model.Coefficients, model.PValues, 0.05),
    }

    return &StressIndexWeightsConfig{Weights: weights}, report
}
```

**為什麼用 Logistic Regression 而非簡單相關性？**
- Logistic regression 直接對應我們的客觀函數（分類問題 Y ∈ {0,1}）
- 自動確保輸出在 [0,1]（可解釋為機率）
- 提供 p-values，可以判斷哪些因子顯著、哪些不顯著
- 如果某個因子的 p-value > 0.05，說明它對預測外資出逃沒有統計顯著貢獻——應該移除

**校準必須是 time-series aware 的**（不能 random shuffle）：
```go
func timeSeriesCrossValidation(features [][]float64, Y []int, folds int) float64 {
    // 使用 expanding window（不是 random k-fold）
    // 每次用前 80% 數據訓練，後 20% 驗證
    // 這防止了 look-ahead bias
    n := len(features)
    scores := make([]float64, folds)
    for f := 0; f < folds; f++ {
        trainEnd := n * (f + 1) * 4 / 5 / folds  // 80% for training
        testStart := trainEnd
        testEnd := n * (f + 1) / folds

        model := trainLogisticRegression(features[:trainEnd], Y[:trainEnd])
        scores[f] = evaluateAccuracy(features[testStart:testEnd], Y[testStart:testEnd], model)
    }
    return mean(scores)
}
```

---

#### P1.2 Baseline 績效報告

校準完成後，必須產出以下報告：

```
===== 外資出逃指數 Baseline 績效報告 =====
日期範圍: 2023-01-01 ~ 2026-05-18
預測窗口: 5 個交易日
異常定義: 5 日累積外資淨賣超 ∈ top 15% of rolling 1-year

[因子權重比較]
因子              原始權重    校準權重    變化    p-value    顯著性
─────────────────────────────────────────────────────────
DXY               15.0%       XX.X%      ±X.X   X.XXX      ✅/❌
US10Y             20.0%       XX.X%      ±X.X   X.XXX      ✅/❌
ForeignFlow       25.0%       XX.X%      ±X.X   X.XXX      ✅/❌
VIX               15.0%       XX.X%      ±X.X   X.XXX      ✅/❌
JPY               10.0%       XX.X%      ±X.X   X.XXX      ✅/❌
Geopolitical      15.0%       XX.X%      ±X.X   X.XXX      ✅/❌

[分類績效]
準確率 (Accuracy):   XX.X%
精確率 (Precision):  XX.X%  (預測出逃事件中，實際發生的事件比例)
召回率 (Recall):     XX.X%  (實際出逃事件中，被預測到的比例)
F1 Score:            XX.X%
AUC-ROC:             XX.X%

[歷史事件驗證]
事件                      預期 Regime    實際 Score    實際 Regime    通過?
─────────────────────────────────────────────────────────────────
2022-02 俄烏戰爭          crisis         XX.X          XXXX          ✅/❌
2022-09 融資斷頭潮        high           XX.X          XXXX          ✅/❌
2024-08 Carry Unwind      crisis         XX.X          XXXX          ✅/❌
2025-04 關稅衝擊          crisis         XX.X          XXXX          ✅/❌
2026-03 美伊緊張          high           XX.X          XXXX          ✅/❌

[不顯著的因子]
以下因子的 p-value > 0.05，對預測外資出逃沒有統計顯著貢獻：
 - [因子名] (p=X.XXX)
建議：在下一階段考慮移除或以不同形式重新引入。

[共線性警告]（Variance Inflation Factor > 5）
 - [因子 A] ↔ [因子 B]: VIF = X.X (存在中度共線性)
```

---

### Phase 2: 增量驗證——測試新因子的邊際貢獻（2 週）

**目標**：逐一測試原油、黃金、融資維持率等新因子是否顯著提升 out-of-sample 預測力

**關鍵原則**：新因子必須通過增量驗證才納入模型。不通過的因子應被拒絕。

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P2.1** Oil 因子增量驗證 | 加入 oil 後 AUC-ROC 是否顯著提升？ | P1.2 |
| **P2.2** Gold 因子增量驗證 | 加入 gold 後 AUC-ROC 是否顯著提升？ | P1.2 |
| **P2.3** Maintenance Ratio 因子驗證 | 加入維持率後 AUC-ROC 是否顯著提升？ | P1.2 |
| **P2.4** PCA 降維處理共線性 | 若多因子 VIF > 5，使用 PCA 提取正交成分 | P2.1, P2.2 |

#### P2.1-2.3 增量驗證協議

```go
type IncrementalValidationResult struct {
    Factor          string
    BaselineAUC     float64
    WithFactorAUC   float64
    DeltaAUC        float64  // 必須 > 0.02 才能被認為是「有意義的改進」
    DieboldMariano  float64  // DM test p-value（必須 < 0.05）
    Recommendation  string   // "ACCEPT" or "REJECT"
}

func validateFactorIncremental(
    factorName string,
    baselineFeatures [][]float64,
    newFactor []float64,
    Y []int,
) IncrementalValidationResult {
    // Diebold-Mariano test: 比較兩個模型的預測誤差是否有顯著差異
    baselinePreds := predict(baselineModel, baselineFeatures)
    augmentedPreds := predict(augmentedModel, appendFeatures(baselineFeatures, newFactor))

    dmStat, dmPValue := dieboldMarianoTest(baselinePreds, augmentedPreds, Y)

    baselineAUC := computeAUC(baselinePreds, Y)
    augmentedAUC := computeAUC(augmentedPreds, Y)

    deltaAUC := augmentedAUC - baselineAUC

    // 接受標準：
    // 1. DM test p-value < 0.05（新模型預測力顯著優於舊模型）
    // 2. AUC 提升 > 0.02（實際改進，不是噪音）
    if dmPValue < 0.05 && deltaAUC > 0.02 {
        return IncrementalValidationResult{
            Factor: factorName, DeltaAUC: deltaAUC,
            DieboldMariano: dmPValue, Recommendation: "ACCEPT",
        }
    }
    return IncrementalValidationResult{
        Factor: factorName, DeltaAUC: deltaAUC,
        DieboldMariano: dmPValue, Recommendation: "REJECT",
    }
}
```

**為什麼需要 Diebold-Mariano test？**
- 單純比較 AUC 可能被噪音驅動
- DM test 是預測模型比較的黃金標準，判斷兩個模型的預測誤差是否有統計顯著差異
- 防止將「隨機波動導致的 AUC 提升」誤判為「因子有效」

---

#### P2.4 PCA 降維處理共線性

如果 `P1.2` 的 Baseline 報告顯示多個因子的 VIF > 5，則必須先處理共線性再進行增量驗證。

```go
// PCAFactorTransformer 使用主成分分析提取正交風險維度
type PCAFactorTransformer struct {
    components  []PCAComponent
    explainedVar []float64
}

type PCAComponent struct {
    Loadings map[string]float64  // 各原始因子對該主成分的貢獻
    Label    string              // 可解釋的標籤（如 "Global Risk Aversion"）
}

func (p *PCAFactorTransformer) Transform(factors map[string]float64) map[string]float64 {
    // 將原始因子投影到正交主成分空間
    orthogonal := make(map[string]float64)
    for i, comp := range p.components {
        score := 0.0
        for name, loading := range comp.Loadings {
            score += loading * factors[name]
        }
        orthogonal[comp.Label] = score
    }
    return orthogonal
}
```

PCA 的優勢：
- 自動處理共線性（主成分之間完全正交）
- 減少維度（可以用 3-4 個主成分保留 90%+ 的解釋力）
- 每個主成分有經濟解釋（如 PC1 = "全球風險偏好"、PC2 = "利率環境"、PC3 = "地緣政治"）

---

### Phase 3: 新增已驗證的因子（1 週）

**目標**：只納入通過 Phase 2 增量驗證的因子

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P3.1** 更新壓力指數計算 | 加入通過驗證的因子（含 PCA 處理） | Phase 2 |
| **P3.2** 更新 config 與前端 | 更新 `stress_index_weights.json` + `narrative.js` | P3.1 |
| **P3.3** 最終驗證 | 對更新後的模型重新跑 P0.4 的 5 事件驗證 | P3.1 |

**注意**：如果 Phase 2 的結果是「所有新因子的增量驗證都未通過」（delta AUC < 0.02 或 DM p-value > 0.05），則 Phase 3 應為空操作——**這本身就是一個有價值的結論**，說明現有的六因子模型已經足夠，不需要增加複雜度。

---

### Phase 4: 非線性架構升級（2-3 週，選擇性）

**目標**：解決「結構性對沖」和 regime-dependent 權重的需求（但用專業方法，而非簡單相減）

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P4.1** 2×2 矩陣框架 | 壓力 × 吸引力矩陣，取代線性相減 | Phase 3 |
| **P4.2** Regime-switching 模型 | Markov-switching 或其他狀態轉換模型 | Phase 3 |
| **P4.3** 微結構指標整合 | Put/Call Ratio、價量背離、ETF 折溢價 | P4.2 |

#### P4.1 2×2 壓力 × 吸引力矩陣

修正原方案 P1.4 中「結構性對沖」的類別錯誤：

```go
type CapitalFlowMatrix struct {
    Pressure      float64  // 外資出逃壓力 (0-100)
    Attractiveness float64 // 結構性吸引力 (0-100)
}

func (m CapitalFlowMatrix) Quadrant() string {
    switch {
    case m.Pressure >= 50 && m.Attractiveness < 40:
        return "CRISIS"       // 高壓力 + 低吸引力 → 立即行動
    case m.Pressure >= 50 && m.Attractiveness >= 40:
        return "TENSION"      // 高壓力 + 高吸引力 → 監控，擇股操作
    case m.Pressure < 50 && m.Attractiveness < 40:
        return "STAGNATION"   // 低壓力 + 低吸引力 → 低機會
    default:
        return "OPPORTUNITY"  // 低壓力 + 高吸引力 → 正常操作
    }
}

// 結構性吸引力獨立計算（不與壓力指數混為一談）
func (c *AttractivenessCalculator) Calculate(snap marketdata.MacroDataSnapshot) float64 {
    // 獨立計算，不影響壓力指數
    score := 0.0
    if snap.TSMCRevenue.ChangePct > 30 { score += 30 }
    if snap.CapexGrowth.Value > 20 { score += 25 }
    if snap.CoWoSUtilization.Value > 80 { score += 20 }
    if snap.DomesticFundNet.Value > 50 { score += 15 }
    if snap.SOXIndex.ChangePct > 0 { score += 10 }
    return min(100, score)
}
```

**前端呈現**：顯示 2×2 象限圖，讓使用者同時看到壓力與吸引力兩個維度，而非一個被壓縮的單一數字。

#### P4.2 Regime-switching 模型

```go
// RegimeSwitchingModel 使用隱馬可夫模型（HMM）自動偵測市場體制
type RegimeSwitchingModel struct {
    states    int          // 體制數量（預設 3: bull/bear/crisis）
    transition [][]float64 // 狀態轉移矩陣
    emission  []Emission   // 各狀態下的因子分布
}

// 自動判斷當前 market regime 並使用對應權重
func (m *RegimeSwitchingModel) GetRegimeWeights(factors []float64) StressIndexWeights {
    regime := m.classify(factors)
    return m.regimeSpecificWeights[regime]
}
```

HMM 的優勢：
- 自動學習市場體制的轉換機率（bull → bear 的機率是多少？）
- 不需要手動定義閾值
- 每個體制有獨立的因子權重，自動捕捉 regime-dependent 的預測力變化

---

### Phase 5: 散戶情緒與微結構指標（1-2 週，選擇性）

| 任務 | 說明 | 依賴 |
|------|------|------|
| **P5.1** 散戶情緒預測力驗證 | 對 TAIEX 未來報酬的回測 | Phase 0 |
| **P5.2** 微結構指標整合（Put/Call、價量背離） | 短期風險預警 | 無 |
| **P5.3** 維持率監控告警 | 全市場維持率 < 140% 觸發警報 | Phase 0 |

#### P5.1 散戶情緒對 TAIEX 的預測力驗證

```go
type RetailSentimentBacktest struct {
    // 使用 Granger Causality test 而非簡單相關性
    // H₀: 散戶情緒的歷史值不包含對 TAIEX 未來報酬的額外預測資訊
}

func (b *RetailSentimentBacktest) GrangerCausalityTest(maxLag int) GrangerResult {
    for lag := 1; lag <= maxLag; lag++ {
        F, pValue := grangerCausality(b.sentiment, b.returns, lag)
        if pValue < 0.05 {
            // 散戶情緒在 lag 期之前 Granger-cause TAIEX 報酬
            return GrangerResult{Causal: true, OptimalLag: lag, PValue: pValue}
        }
    }
    return GrangerResult{Causal: false}
}
```

**Granger causality** 比簡單相關性更嚴謹，因為：
- 控制了 TAIEX 自身的自回歸效應
- 檢驗的是「增量預測力」而非「同期相關性」
- 是學術界判斷「X 是否預測 Y」的標準方法

#### P5.2 微結構指標

```go
type MicrostructureIndicators struct {
    PutCallRatio       float64  // 買賣權未平倉比
    PutCallChange      float64  // 5 日變化
    VolumeDivergence   float64  // 價量背離分數（-1=價漲量縮, +1=價跌量增）
    ETFDiscountPremium float64  // 台灣 50 ETF 折溢價
    BidAskSpread       float64  // 加權平均買賣價差
}

// 微結構壓力指數：獨立於宏觀壓力指數的短期風險指標
func (m *MicrostructureIndicators) ShortTermStressScore() float64 {
    score := 0.0
    // Put/Call > 1.5 → 極度偏空
    if m.PutCallRatio > 1.5 { score += 30 }
    // 價量背離 → 趨勢弱化
    score += abs(m.VolumeDivergence) * 25
    // ETF 折價 > 1% → 套利機制異常
    if m.ETFDiscountPremium < -1.0 { score += 25 }
    // 買賣價差擴大 → 流動性枯竭
    if m.BidAskSpread > 0.5 { score += 20 }
    return min(100, score)
}
```

---

## 二、修正後的技術路線圖總覽

```
Phase 0 (1週)           Phase 1 (2週)           Phase 2 (2週)           Phase 3 (1週)        Phase 4-5 (選擇性)
┌──────────────┐       ┌──────────────┐       ┌──────────────┐       ┌──────────────┐       ┌──────────────┐
│ P0.1 修復    │       │ P1.1 校準    │       │ P2.1 Oil     │       │ P3.1 更新    │       │ P4.1 2×2    │
│ divergence   │──→    │ Logistic     │──→    │ 增量驗證     │──→    │ 壓力指數     │──→    │ 矩陣框架     │
│              │       │ Regression   │       │              │       │              │       │              │
│ P0.2 修復    │       │              │       │ P2.2 Gold    │       │ P3.2 前端    │       │ P4.2 HMM     │
│ margin門檻   │       │ P1.2 Baseline│       │ 增量驗證     │       │ 更新         │       │ Regime模型   │
│              │       │ 績效報告     │       │              │       │              │       │              │
│ P0.3 補齊    │       │              │       │ P2.3 Maint   │       │ P3.3 最終    │       │ P5.1 散戶    │
│ 融券數據     │       │ P1.3 前端    │       │ 增量驗證     │       │ 驗證         │       │ 預測力驗證   │
│              │       │ 呈現優化     │       │              │       │              │       │              │
│ P0.4 驗證    │       │              │       │ P2.4 PCA     │       │              │       │ P5.2 微結構  │
│ 框架建立     │       │              │       │ 降維         │       │              │       │ 指標整合     │
└──────────────┘       └──────────────┘       └──────────────┘       └──────────────┘       └──────────────┘
     ↑                        ↑                      ↑
     │                        │                      │
  GATE: 5事件              GATE: 不顯著因子         GATE: delta AUC>0.02
  validation pass          p-value>0.05 的因子      AND DM p-value<0.05
  才能進入 Phase 1        應被標記/移除             否則 REJECT 該因子
```

---

## 三、前端疊代方案

### 3.1 壓力指數呈現（P1.3）

```javascript
// narrative.js - 修正後的壓力指數呈現
export function renderStressIndex(stress) {
    // 1. 主要分數：現在可解釋為「5 日內外資異常流出的機率」
    const score = stress.score.toFixed(0);
    const interpretation = score >= 70 ? '極高機率（立即警戒）' :
                           score >= 50 ? '較高機率（監控）' :
                           score >= 30 ? '中等機率（注意）' :
                           '低機率（正常）';

    // 2. 因子貢獻分解：使用 stacked bar 視覺化
    const componentBars = Object.entries(stress.components)
        .sort(([,a], [,b]) => b - a)
        .map(([name, value]) => {
            const pct = (value / stress.score * 100).toFixed(0);
            return `<div class="factor-bar" style="width:${pct}%">${factorLabel(name)} ${value.toFixed(1)}</div>`;
        });

    // 3. 顯著性標記（來自 P1.2 的 Baseline 報告）
    const significantFactors = stress.significant_factors || [];
    const insignificantNote = stress.insignificant_factors?.length
        ? `<div class="warning">以下因子在統計上不顯著(p>0.05)：${stress.insignificant_factors.join(', ')}</div>`
        : '';

    // 4. 校準資訊
    const calibrationNote = stress.calibration_date
        ? `<div class="text-muted text-xs">模型最後校準：${stress.calibration_date} | AUC: ${stress.auc_roc?.toFixed(2)}</div>`
        : '';
}
```

### 3.2 總經快照擴展（P3.2）

```javascript
// narrative.js - 修正後的總經快照
const rows = [
    // 全球宏觀（現有）
    ['DXY-美元指數', snapshot.dxy], ['US10Y-美債10年期', snapshot.us10y],
    ['VIX-波動率指數', snapshot.vix], ['USD/TWD-匯率', snapshot.usd_twd],
    ['原油', snapshot.oil], ['黃金', snapshot.gold], ['日圓', snapshot.jpy],
    // 台灣市場情緒（新增——Phase 0 後可用）
    ['融資餘額(億)', snapshot.retail_margin_balance],
    ['融券餘額(億)', snapshot.retail_short_balance],      // P0.3 後可用
];
```

### 3.3 2×2 象限圖（P4.1 後可用）

在宏觀敘事頁面新增一個象限圖，取代單一的壓力指數數字：

```html
<div id="capital-flow-matrix">
    <!-- 2×2 矩陣：壓力（Y軸）× 吸引力（X軸） -->
    <div class="quadrant CRISIS">CRISIS<br>立即減碼</div>
    <div class="quadrant TENSION">TENSION<br>擇股監控</div>
    <div class="quadrant STAGNATION">STAGNATION<br>等待機會</div>
    <div class="quadrant OPPORTUNITY">OPPORTUNITY<br>正常操作</div>
    <!-- 當前位置的點 -->
    <div class="current-dot" style="left:{attractiveness}%;bottom:{100-pressure}%"></div>
</div>
```

---

## 四、風險與緩解

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|---------|
| Logistic regression 校準需要足夠的歷史數據 | 中 | 高 | 最小需要 252 個交易日（1 年），若不夠則延後 Phase 1 |
| PCA 降維後經濟解釋性下降 | 中 | 中 | 每個主成分必須命名（如 PC1="Global Risk Aversion"），保留可解釋性 |
| 融券數據欄位與預期不符 | 中 | 中 | 實作時先 dump API 回應確認 column index |
| 新因子全部未通過增量驗證 | 中 | 低 | 這是正面結果——證明現有模型已足夠，不增加複雜度 |
| 維持率數據源不可靠 | 高 | 中 | 先使用 TWSE 公布的全市場數據，不依賴單一券商 |

---

## 五、與原方案 v1.0 的關鍵差異

| 維度 | v1.0（原方案） | v2.0（修正方案） |
|------|--------------|----------------|
| **客觀函數** | 未定義 | 明確定義為分類問題：預測 5 日內異常外資流出 |
| **因子新增方式** | 直接加入線性模型 | 先校準現有模型 → 增量驗證 → 僅納入通過 DM test 的因子 |
| **共線性處理** | 未處理 | VIF 檢測 → PCA 降維 |
| **結構性對沖** | 簡單相減（類別錯誤） | 獨立 2×2 矩陣框架 |
| **Divergence 修正** | 日度 Z-score（假設正態） | 5 日累積流量 Z-score（處理自相關） |
| **融資門檻** | 單純 percentile | percentile + 加速度 + 維持率三條件 |
| **實施順序** | 修 bug → 加功能 → 校準 | 修 bug → 校準現有 → 增量驗證 → 加入新因子 |
| **驗證框架** | 未定義具體案例 | 5 個歷史極端事件測試案例 + DM test |
| **校準方法** | 相關性分析 | Logistic regression（分類）+ expanding window cross-validation |
| **拒絕機制** | 無 | p-value > 0.05 的因子標記為不顯著；DM test 未通過的因子被拒絕 |
| **前端呈現** | 單一分數 | 分數 + 機率解釋 + 因子分解圖 + 顯著性標記 + 2×2 象限 |

---

## 六、預期成效

| 指標 | 當前 | Phase 0 後 | Phase 1 後 | Phase 3 後 |
|------|------|-----------|-----------|-----------|
| Divergence signal 經濟可解釋性 | ❌ 量綱不一致 | ✅ 標準化差異 | - | - |
| 融資門檻自適應性 | ❌ 絕對常數 | ✅ percentile+加速度 | - | - |
| 壓力指數 AUC-ROC | 未知 | 未知 | ≥ 0.70 | ≥ 0.75 |
| 壓力指數可解釋為機率 | ❌ | ❌ | ✅ Logistic | ✅ |
| 5 事件驗證通過率 | 待測 | 待測 | ≥ 80% | ≥ 80% |
| 不顯著因子已標記 | ❌ | ❌ | ✅ | ✅ |
| 新因子通過增量驗證 | N/A | N/A | N/A | 僅納入通過的 |

---

*報告版本: 2.0 | 審計日期: 2026-05-18 | 基於金融工程專業方法論修正*
