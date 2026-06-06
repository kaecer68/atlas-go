# Atlas 投資人 NLG — 推薦解釋層

**版本**: 1.0  
**日期**: 2026-06-02  
**成熟度**: X（experimental）  
**父技能**: `atlas-investor-ui`

---

## 一、描述

本技能定義如何將 Atlas 的 Audit Trail 數據（FactorScoreBreakdown、ConvictionBreakdown、NarrativeEvent）轉化為**繁體中文的投資人語言**。目標是讓投資人看到的不再是 JSON，而是「為什麼推薦這檔股票」的自然語言解釋。

---

## 二、現狀：有數據，沒有語言

Atlas 已有最強的 Audit Trail：

| 數據層 | 格式 | 問題 |
|--------|------|------|
| `FactorScoreBreakdown` | JSON：每因子含 `Score`、`Weight`、`Formula`、`RawInputs`、`IsFallback` | 投資人看不懂 `momentum(20d)=5.2%, weight=0.25` |
| `ConvictionBreakdown` | JSON：`Steps[]` 含 `Rule`、`Delta`、`Reason` | 投資人無法從「Delta +0.2, Reason: sector_momentum_positive」理解含義 |
| `NarrativeEvent` | JSON：含 `ConfidenceSource`、`HitRate` | 命中率 81% 沒有被轉化為「此事件過去預測準確率 81%」 |

---

## 三、目標

將上述三層數據 → 繁體中文投資建議，例如：

> 「台積電（2330.TW）目前綜合評分 82/100。  
> 主要優勢來自：技術面動能強勁（20 日漲幅 5.2%）、估值具吸引力（本益比 15.3 vs 5 年平均 18.7）、外資持續買超。  
> 主要風險：半導體產業處於景氣高點，若 AI 資本支出降溫可能回調。  
> 過去 5 次類似市場環境下，台積電 10 日後的平均報酬為 +2.1%。」

---

## 四、實作基礎

### 已有基礎設施（可直接擴展）

| 檔案 | 現有功能 | 需擴展 |
|------|----------|--------|
| `narrative/nlg_templates.go` | NLG 模板系統 | 加入投資人語言模板（繁體中文） |
| `narrative/report_generator.go` | 報告生成器 | 不加新模組，只擴展模板 |

### 不新建模組

不新增 `internal/nlg/` 或 `internal/investor-nlg/`。直接在現有 `narrative/` 包中擴展。

---

## 五、模板設計

### 核心規則

| 條件 | 模板方向 |
|------|---------|
| `Score > 0.5` | 正面描述（「動能強勁」、「估值具吸引力」） |
| `Score < -0.3` | 負面描述（「動能偏弱」、「估值偏高」） |
| 其他 | 中性描述（「處於合理範圍」） |
| `IsFallback == true` | 加註 `*數據有限，此項評估僅供參考` |

### 模板範例（Go pseudo-code）

```go
// GenerateRecommendationExplanation 產生推薦的自然語言解釋
func GenerateRecommendationExplanation(symbol string, breakdown *FactorScoreBreakdown,
    conviction *ConvictionBreakdown, events []NarrativeEvent) string

// GenerateMarketSummary 產生市場狀態的自然語言摘要
func GenerateMarketSummary(regime Regime, riskLevel string, events []NarrativeEvent) string

// GenerateRiskExplanation 產生風險狀態的自然語言解釋
func GenerateRiskExplanation(status RiskGateStatus, calib *CalibrationReport) string
```

### 因子的投資人語言對照表

| 因子（內部名） | 投資人語言 |
|--------------|-----------|
| `momentum` | 技術面動能 |
| `value` | 估值水準 |
| `quality` | 財務品質 |
| `volatility` | 價格穩定性 |
| `growth` | 成長潛力 |
| `sentiment` | 市場情緒 |
| `size` | 市值規模 |
| `seasonal` | 季節性效應 |
| `precious_metals` | 貴金屬關聯 |
| `etf` | ETF 特性 |
| `linkage` | 產業供應鏈 |

---

## 六、Fallback 處理

若任何因子使用 fallback 值（`IsFallback == true`），必須在 NLG 輸出中標記：

```
「*技術面動能評估：數據有限，此項僅供參考」
```

不可靜默使用 fallback 數據而不告知投資人。

---

## 七、輸出層級

| 層級 | 內容 | 用途 |
|------|------|------|
| **一句話摘要** | ~20 字 | 儀表板推薦清單 |
| **段落解釋** | 3-5 句 | 推薦詳情頁 |
| **完整報告** | 多段，含歷史情境 | 深度研究 |

---

## 八、與其他技能關聯

| 技能 | 關聯 |
|------|------|
| `atlas-investor-pages` | 推薦詳情頁使用 NLG 輸出 |
| `atlas-investor-ui` | 設計原則「投資人語言優先」的直接實作 |
| `atlas-macro-narrative` | NLG 模板取用 narrative events、命中率 |
| `atlas-investor-roadmap` | Phase A P1 項目 |

---

## 九、驗收標準

- [ ] NLG 輸出為繁體中文，不含任何技術術語
- [ ] fallback 數據有標記
- [ ] 一句話摘要 < 30 字
- [ ] 段落解釋 3-5 句，邏輯連貫
- [ ] 不新建模組（僅擴展 `narrative/` 包）
