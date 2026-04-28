# 事件驅動多策略系統實作計劃

**版本**：v1.0
**建立日期**：2026-04-28
**狀態**：已鎖定設計決策，等待實作

---

## 目標

建立「事件驅動、多策略、可自我迭代」的投資研究系統，將龐大的監測數據（monitoring/narrative）與推薦管線（FactorEngine）連接，形成真正的數據交叉點。

---

## 核心問題診斷

```
現有數據流：
Market Data → FactorEngine → 推薦管線（只有 4 因子：Momentum/Value/Quality/Agent）
                    ↑
              這裡是瓶頸

龐大但獨立的監測系統：
TWSEBalanceProvider（真實） ──┐
                               ├──→ monitoring Dashboard
TWSECapitalFlowProvider（真實）┘         │
                               └──→ narrative（StressIndex）
                                          │ ← 死亡交叉點

這些數據從未進入 FactorEngine。
```

---

## Phase 1：動態因子權重系統

### 1.1 擴展 NarrativeEvent 結構

**新增欄位**：

| 欄位 | 類型 | 說明 |
|------|------|------|
| `Duration` | `time.Duration` | 事件影響持續時間 |
| `ExpiresAt` | `*time.Time` | 事件過期時間點 |
| `Severity` | `string` (low/medium/high/critical) | 事件嚴重程度 |
| `Status` | `string` (active/confirmed/faded/expired) | 事件狀態 |

**內建事件持續時間**（從學術研究）：
| 事件類型 | 價格發現期 | 影響持續期 |
|----------|-----------|------------|
| Fed 利率決策 | 1-4 小時 | 3-7 日 |
| 企業財報 | 1-3 日 | 5-10 日 |
| 地緣政治危機 | 即時 | 7-30 日 |
| US_rates_up | 1-4 小時 | 3-7 日 |
| JPY_carry_unwind | 即時 | 7-14 日 |
| geopolitical_risk_spike | 即時 | 7-30 日 |
| oil_price_shock | 即時 | 5-15 日 |
| AI_capex_surge | 1-30 日 | 30-90 日 |

### 1.2 建立 FactorWeightEngine

**職責**：根據事件動態調整因子權重

**配置結構**：
```go
type FactorWeightConfig struct {
    BaseWeights      map[string]float64  // 基礎權重
    EventAdjustments map[string]EventWeightAdjustment
}

type EventWeightAdjustment struct {
    ThemePattern string
    WeightDelta  map[string]float64  // 因子 → 權重變化
    RegimeFilter string             // 僅在特定 regime 生效
}
```

**基礎權重配置**（6 因子）：
| 因子 | 基礎權重 | 說明 |
|------|----------|------|
| Momentum | 0.20 | 動能因子 |
| Value | 0.15 | 價值因子 |
| Quality | 0.15 | 品質因子 |
| Agent | 0.20 | 代理人因子 |
| InstitutionalSentiment | 0.15 | 機構情緒因子 |
| Liquidity | 0.15 | 流動性因子 |

### 1.3 建立 FactorBridge（宏觀數據橋接器）

**職責**：將 MacroDataSnapshot（monitoring 數據）轉換為可用於因子計算的輸入

**MacroDataSnapshot 來源**：
- TWSEBalanceProvider → 散戶資金流向
- TWSECapitalFlowProvider → 外資/法人數據
- TaiwanStressIndex → 市場壓力指數

**輸出**：
- `ForeignFlowScore`: 外資買賣超標準化分數 [-1, 1]
- `MarginBalanceScore`: 券資比標準化分數 [-1, 1]
- `RetailSentimentScore`: 散戶情緒分數 [-1, 1]
- `StressLevel`: 市場壓力等級 (0-100)

### 1.4 新增 InstitutionalSentiment 因子

**計算方式**：
```
InstitutionalSentiment = ForeignWeight × ForeignFlowScore
                      + DomesticWeight × DomesticFlowScore
                      + MarginWeight × MarginBalanceScore

where:
  ForeignWeight = 0.50
  DomesticWeight = 0.30
  MarginWeight = 0.20
```

**資料來源**：
- 外資：`TWSECapitalFlowProvider.GetForeignInvestment()`
- 法人：`TWSECapitalFlowProvider.GetDomesticInstitutional()`
- 券資比：`TWSEBalanceProvider.GetMarginBalance()`

### 1.5 新增 Liquidity 因子

**計算方式**（Amihud ILLIQ proxy）：
```
Liquidity = -log( |Return| / Volume )  // 標準化後
```

**輸入**：
- 日收益率
- 日成交量

**閾值**：
- ILLIQ > 1.0：低流動性（權重調降）
- ILLIQ < 0.1：高流動性（正常權重）

### 1.6 建立 RegimeChange 機制

**Regime 類型**：
| Regime | 判斷條件 |
|--------|----------|
| Bull | VIX < 15, TrendUp |
| Bear | VIX > 25, TrendDown |
| Neutral | VIX 15-25 |
| HighVol | VIX > 30 |

**Regime 變化時**：
1. 記錄 `PreviousRegime` / `CurrentRegime`
2. 計算 Regime 持續時間
3. 觸發 FactorWeightEngine 重新計算權重
4. 發送 `RegimeChangedEvent` 到因果鏈

---

## Phase 2：動態相關性閾值

### 2.1 建立 VIX × Regime 混合閾值引擎

**目標**：將靜態相關性閾值 0.7 改為動態計算

**動態閾值範圍**：`[0.40, 0.85]`

**計算公式**：
```
BaseThreshold = 0.70
VIXAdjustment = (VIX - 20) / 100  // VIX=20 時為 0
RegimeAdjustment = RegimeMultiplier[Regime]
DynamicThreshold = BaseThreshold + VIXAdjustment + RegimeAdjustment
DynamicThreshold = clamp(DynamicThreshold, 0.40, 0.85)
```

**Regime 倍數**：
| Regime | 倍數 |
|--------|------|
| Bull | -0.05 |
| Bear | +0.10 |
| Neutral | 0.00 |
| HighVol | +0.15 |

**範例**：
- VIX=15, Bull: 0.70 - 0.05 + (15-20)/100 = 0.60
- VIX=35, HighVol: 0.70 + 0.15 + (35-20)/100 = 0.90 → clamp to 0.85

### 2.2 閾值驗證頻率

- **每日快速更新**：根據收盤 VIX 重新計算
- **每 5 交易日完整驗證**：回測驗證閾值有效性

---

## Phase 3：多策略框架

### 3.1 Strategy 結構與 Registry

```go
type Strategy struct {
    ID          string
    Name        string
    Description string
    Enabled     bool
    Agents      []string  // 使用的 Agent ID 列表
    Filters     []string  // 使用的 Filter 列表
    Priority    int       // 執行優先順序
}

type StrategyRegistry struct {
    strategies map[string]*Strategy
}
```

### 3.2 內建策略

| 策略 ID | 名稱 | 說明 | Agent 子集 |
|---------|------|------|------------|
| `all_weather` | 全天候 | 所有 Agent，保守閾值 | 全部 |
| `growth` | 成長動能 | 動能 + AI supply chain | momentum, ai_supply_chain |
| `value` | 價值投資 | Value + Quality | value, quality |
| `defensive` | 防御型 | 高品質 + 低波動 | quality, low_volatility |
| `momentum` | 純動能 | 僅動能因子 | momentum |

### 3.3 策略每日比較系統

**比較維度**：
- 日報酬率
- 夏普比率（20日滾動）
- 最大回撤
- 勝率

**輸出**：
```go
type StrategyComparison struct {
    Date           time.Time
    StrategyID     string
    DailyReturn    float64
    SharpeRatio    float64
    MaxDrawdown    float64
    WinRate        float64
    Outperformance  float64  // vs benchmark
}
```

### 3.4 策略選擇引擎

**邏輯**：
1. 根據 Regime 初步篩選可用策略
2. 根據近 20 日表現給予權重
3. 選擇表現最佳的策略組合
4. 每日重新平衡

---

## NewsSentiment（平行軌跡）

### 4.1 Finnhub 限制發現

**重要限制**：
- Finnhub News Sentiment API **僅支援美股公司**（Premium 功能）
- 台股新聞情緒**無法使用 Finnhub**

### 4.2 替代方案評估

| 方案 | 台股覆蓋 | 情緒分析 | 成本 | 備註 |
|------|----------|----------|------|------|
| TWSE 開放資料 | 完整 | 無 | 免費 | 可取得新聞量/討論度 |
| TEJ（台灣經濟新報） | 完整 | 有 | 昂貴 | 台灣專業金融數據庫 |
| 自行建置 | 可擴展 | 可訓練 | 高 | 需要 NLP 模型 |
| Finage | 部分 | API 提供 | 中 | 需確認台股覆蓋 |

**建議**：
1. **短期**：使用 TWSE 開放資料作為代理指標（新聞發布量、討論度）
2. **中期**：評估 Finage 或自行建置 NLP 模型
3. **長期**：若預算允許，採用 TEJ

### 4.3 實作路徑（若採用 TWSE 開放資料）

```go
type NewsSentimentProxy struct {
    // 使用公開資訊觀測站的異常公告數量
    // 作為市場情緒的代理指標
    MOPSAnnouncementCount  int
    MOPSAnnouncementDelta  float64  // 相比前一週
    ForumBuzzScore         float64  // 假設從 forum aggregator 取得
}
```

---

## 歷史數據來源

### 5.1 TWSE 開放資料 API

**端點**：
- `https://openapi.twse.com.tw/v1/` - Swagger JSON
- 盤後資訊：上市個股日收盤價、月均價

**限制**：
- 即時性可能延遲
- 需要自行處理頻率轉換

### 5.2 TWSE Data E-Shop

**產品**：
- 歷史交易資訊：15+ 年
- 涵蓋所有市場產品

**適用場景**：
- 長期回測（15 年 backtest）
- 因子有效性驗證

### 5.3 EOD Historical Data API

**覆蓋**：
- 1314 檔台股 ETF/個股
- 25+ 年歷史數據（Standard 方案）

**優點**：
- 單一 API 涵蓋多市場
- 標準化 JSON 格式

---

## 實作順序

```
Week 1-2 ─────────────────────────────────────────────
├── Phase 1.1: 擴展 NarrativeEvent 結構
├── Phase 1.2: 建立 FactorWeightEngine 基礎
└── Phase 1.3: 建立 FactorBridge 雛形

Week 3-4 ─────────────────────────────────────────────
├── Phase 1.4: 實作 InstitutionalSentiment 因子
├── Phase 1.5: 實作 Liquidity 因子
└── Phase 1.6: 建立 RegimeChange 機制

Week 5-6 ─────────────────────────────────────────────
├── Phase 2.1: 建立動態閾值引擎
└── Phase 2.2: 閾值驗證框架

Week 7-8 ─────────────────────────────────────────────
├── Phase 3.1: Strategy 結構與 Registry
├── Phase 3.2: 內建策略實作
└── Phase 3.3: 策略比較系統

NewsSentiment（並行）─────────────────────────────────
├── 評估替代數據源
├── 實作代理指標（若採用 TWSE）
└── 建立情緒更新pipeline

歷史數據（必要前提）─────────────────────────────────
├── 串接 TWSE 開放資料 API
└── 驗證資料完整性與頻率
```

---

## 驗證標準

| 項目 | 驗證方式 | 成功標準 |
|------|----------|----------|
| NarrativeEvent 擴展 | `go test ./internal/narrative/...` | 50+ 測試通過 |
| FactorWeightEngine | 回測驗證 | 動態權重版本勝過靜態版本 |
| FactorBridge | Unit test | 正確轉換所有監測數據 |
| 動態閾值 | 回測驗證 | 報酬率改善 or 回撤減少 |
| 多策略框架 | `go test ./internal/orchestrator/...` | 策略切換正常運作 |
| NewsSentiment | 與 Finnhub/替代方案相關性驗證 | 相關性 > 0.5 |

---

## CI 要求

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

---

## 風險與緩解

| 風險 | 影響 | 緩解措施 |
|------|------|----------|
| 歷史數據取得困難 | 高 | 採用 TWSE 開放資料 + EOD API 雙備援 |
| Finnhub 台股情緒限制 | 高 | 評估替代方案，採用代理指標 |
| 動態權重過度擬合 | 中 | 限制最小調整幅度，建立回測驗證框架 |
| Regime 判斷不穩定 | 中 | 使用 5 日移動平均平滑 Regime 變化 |

---

## 依賴關係

```
Phase 1 (基礎)
  ├── NarrativeEvent 擴展
  ├── FactorWeightEngine
  ├── FactorBridge
  ├── InstitutionalSentiment 因子
  ├── Liquidity 因子
  └── RegimeChange 機制

Phase 2 (依賴 Phase 1)
  └── 動態閾值引擎

Phase 3 (依賴 Phase 1/2)
  ├── Strategy Registry
  ├── 內建策略
  └── 策略比較系統

NewsSentiment (並行)
  └── 獨立實作，追蹤 Phase 1 進度
```

---

## 備註

- **不使用 TDD**：使用者明確指示
- **向後相容**：`executeOrder` / `ExecuteOrder` 兩個方法都需要保留
- **覆蓋率門檻**：40%（總平均）
- **不引入全域可變狀態**：執行期協調使用 context
