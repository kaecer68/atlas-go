# 已知限制深度評估報告

## 執行摘要

本次評估針對參數管理系統的三個已知限制，從**金融專業**與**軟件工程**雙重角度進行深度檢視。發現限制 2（P2 參數未遷移）存在**高嚴重性設計缺陷**，直接影響投資決策品質與模型可解釋性；限制 1 與限制 3 為**中等嚴重性技術債**，影響測試覆蓋率與運維效率。

---

## 限制 1：Portfolio 測試失敗 — `WithParameters` 方法

### 現況

`internal/portfolio/volatility_manager_test.go` 中 11 個測試案例呼叫 `vm.WithParameters(params)`，但原始 main 分支的 `VolatilityManager` 並未實作此方法。我們已在當前工作區補上：

```go
func (vm *VolatilityManager) WithParameters(p *RuntimeParameters) *VolatilityManager {
    vm.params = p
    return vm
}
```

### 金融專業角度評估

#### 🔴 嚴重問題：波動率模型無法適應不同市場環境

`VolatilityManager` 控制以下關鍵金融參數：

| 參數 | 預設值 | 金融意義 | 環境敏感性 |
|------|--------|----------|-----------|
| `GARCH.Omega` | 0.000001 | 長期波動率水平 | 結構性變化時需調整 |
| `GARCH.Alpha` | 0.1 | 新聞衝擊反應速度 | 危機時期應提高 |
| `GARCH.Beta` | 0.85 | 波動率持續性 | 高波動環境應降低 |
| `HighVolThreshold` | 1.5 | 高波警報觸發 | 應隨 VIX 水位動態調整 |
| `CorrelationMinDays` | 30 | 相關性計算最小天數 | 快速變化市場應縮短 |

**設計不合理之處：**

1. **靜態參數陷阱**：預設 `CorrelationMinDays = 30` 在台股約為 1.5 個月。2020 年 3 月疫情爆發時，市場相關性在 2 週內從 0.3 飆升至 0.8，30 天窗口無法及時捕捉這種變化。理想情況下應提供**多時間尺度相關性**（10/20/60 天）並動態加權。

2. **GARCH 參數過度簡化**：使用固定 `Alpha=0.1, Beta=0.85` 的 GARCH(1,1) 模型，但實證研究（Engle & Patton, 2001）顯示台股日報酬的適配參數應為 Alpha≈0.05-0.15, Beta≈0.80-0.95，且需隨時間滾動重新估計。當前設計將 MLE 估計值硬編碼，違反了時變波動率（time-varying volatility）的基本假設。

3. **閾值缺乏市場基準**：`HighVolThreshold = 1.5`（資產波動率 > 目標×1.5 時減碼）沒有考慮台股特有的波動結構。台股日均波動率約 1.2%，年化約 19%；但電子股在財報季可達 3-4%。統一閾值對不同產業不公平。

#### 🟡 次要問題：參數變更無法即時生效

即使補上了 `WithParameters`，變更後的參數只影響**後續**的 `UpdateReturns` 和 `GetVolatilityForecast` 呼叫。已經計算的歷史波動率點（`volatilityHistory`）不會重新計算。這在金融場景中是不可接受的：

- 場景：盤中發現波動率模型低估，需要立即調整 `HighVolThreshold`
- 結果：新閾值只影響未來計算，已經觸發的減碼建議不會重新評估
- 風險：可能導致過度減碼或減碼不足

### 軟件工程角度評估

#### 🔴 嚴重問題：API 設計不一致

```go
// 當前設計：混合初始化
func NewVolatilityManager(targetVol, maxVol float64) *VolatilityManager {
    return &VolatilityManager{
        targetVolatility:   targetVol,  // ← 建構子參數
        maxVolatility:      maxVol,     // ← 建構子參數
        smoothingFactor:    0.3,        // ← 硬編碼
        rebalanceThreshold: 0.05,       // ← 硬編碼
        params:             DefaultRuntimeParameters(), // ← 其他參數
    }
}
```

**違反的原則：**
1. **Single Responsibility Principle**：建構子同時處理「初始化」和「參數配置」
2. **Dependency Inversion Principle**：依賴具體的 `RuntimeParameters` struct，而非抽象 interface
3. **Open/Closed Principle**：擴展參數需要修改建構子簽名

#### 🟡 次要問題：缺乏參數驗證

```go
// 當前實作：無條件賦值
func (vm *VolatilityManager) WithParameters(p *RuntimeParameters) *VolatilityManager {
    vm.params = p  // 沒有驗證！
    return vm
}
```

如果呼叫者傳入 `GARCH.Alpha = 0.5, GARCH.Beta = 0.6`，總和 1.1 > 1，違反 GARCH 平穩性條件，但代碼不會報錯。這會導致波動率預測發散（forecast → ∞）。

#### 🟡 次要問題：並發安全

`WithParameters` 修改 `vm.params` 但沒有持有寫鎖。如果同時有 goroutine 執行 `GetVolatilityAdjustments()`（讀鎖），可能讀取到部分更新的指針。

### 建議修復方案

1. **引入參數 interface**：
```go
type VolatilityParameters interface {
    GetGARCH() GARCHParameters
    GetThresholds() ThresholdParameters
    Validate() error
}
```

2. **增加參數驗證**：
```go
func (vm *VolatilityManager) WithParameters(p *RuntimeParameters) error {
    if err := p.Validate(); err != nil {
        return fmt.Errorf("invalid parameters: %w", err)
    }
    vm.mu.Lock()
    defer vm.mu.Unlock()
    vm.params = p
    return nil
}
```

3. **提供重新計算方法**：
```go
func (vm *VolatilityManager) RecalculateAll() {
    // 使用新參數重新計算所有歷史波動率點
}
```

---

## 限制 2：P2 參數未遷移（Marketdata / Industry / Strategy）

### 現況

三個 package 中發現 **200+ 硬編碼參數**，涵蓋：

| Package | 參數類型 | 估計數量 | 影響範圍 |
|---------|----------|----------|----------|
| `marketdata` | API 速率限制、超時、重試閾值 | ~30 | 數據獲取穩定性 |
| `industry` | 產業權重、季節性因子、景氣循環閾值、風險評分 | ~150 | 投資組合配置、行業輪動 |
| `strategy` | 策略切換間隔、分數閾值、觀察窗口 | ~30 | 策略選擇與切換 |

### 金融專業角度評估

#### 🔴 極嚴重問題：產業權重缺乏理論基礎

`internal/industry/types.go` 定義了 9 個 Level-1 產業權重：

```go
// Level 1: Broad sectors
{ID: "semiconductor",        Weight: 0.25}  // 半導體 25%
{ID: "ai_supply_chain",      Weight: 0.20}  // AI 供應鏈 20%
{ID: "robotics",             Weight: 0.08}  // 機器人 8%
{ID: "financials",           Weight: 0.15}  // 金融 15%
{ID: "shipping",             Weight: 0.10}  // 航運 10%
{ID: "energy",               Weight: 0.05}  // 能源 5%
{ID: "electronics_components", Weight: 0.07} // 電子零組件 7%
{ID: "consumer",             Weight: 0.05}  // 消費 5%
{ID: "industrial",           Weight: 0.05}  // 工業 5%
```

**設計不合理之處：**

1. **與市場結構脫節**：
   - 台灣加權指數中，半導體權重約 60%（台積電單一股票即佔 25-30%）
   - 系統給半導體 25%，這是**主動減碼 35%**
   - 問題：這個減碼幅度是基於什麼？風險規避？還是單純為了分散化？
   - 如果是風險規避，應該由風險管理模組動態計算，而非靜態權重

2. **AI 供應鏈與半導體高度重疊**：
   - AI 供應鏈的代表股包括 `2382.TW`（廣達）、`2317.TW`（鴻海）
   - 但台積電（`2330.TW`）同時是半導體和 AI 供應鏈的核心
   - 兩個類別合計權重 45%，實際暴露可能高於 60%（因重疊）
   - 沒有處理類別相關性（correlation）的權重調整機制

3. **缺乏動態調整**：
   - 2024-2025 年 AI 熱潮中，AI 供應鏈的實際市值佔比從 10% 上升到 25%
   - 靜態權重 20% 意味著系統會**系統性低配**這個板塊
   - 這種結構性偏差（structural bias）會導致長期 underperformance

#### 🔴 嚴重問題：季節性因子缺乏實證支持

`internal/industry/seasonality.go`：

```go
// 春節行情
AdjustmentFactor:   1.15,  // +15%
HistoricalAccuracy: 0.70,  // 70% 準確率
AvgMarketReturn:    0.032, // 平均 3.2%

// 電子旺季（7-9 月）
AdjustmentFactor:   1.25,  // +25%
HistoricalAccuracy: 0.75,  // 75% 準確率
```

**設計不合理之處：**

1. **沒有誤差範圍**：`HistoricalAccuracy = 0.70` 是點估計，沒有信賴區間。樣本數多少？統計顯著性如何？在金融計量中，任何預測都應該附帶標準誤。

2. **未考慮結構性斷裂（structural break）**：
   - 2020 年前後，台股的季節性模式因 COVID-19 發生顯著變化
   - 遠端工作推升了電子股全年需求，淡化了傳統「電子旺季」
   - AI 大模型的出現可能改變了「春節行情」的資金流動模式
   - 系統沒有檢測結構性斷裂的機制，也沒有滾動更新這些因子的流程

3. **調整因子過大**：
   - 電子旺季 +25% 的調整因子意味著如果某股票原本權重 10%，季節性調整後變 12.5%
   - 但這個因子是乘在什麼上的？是信念分數？還是倉位規模？
   - `internal/orchestrator/executors.go` 中未找到 seasonality 的實際調用點，可能是**死代碼**

#### 🔴 嚴重問題：景氣循環閾值過於簡化

`internal/industry/cycle.go`：

```go
// 景氣階段判斷
case revenueGrowth > 0.20 && profitGrowth > 0.20:
    phase = CycleExpansion

case revenueGrowth > 0.05 && profitGrowth > 0.05:
    phase = CycleRecovery
```

**設計不合理之處：**

1. **跨產業統一閾值不合理**：
   - 半導體產業：營收成長 20% 是正常水平（台積電 2024 YoY +30%），不應視為「擴張期」
   - 金融業：營收成長 20% 是異常高（國泰金 2024 YoY +5%），應視為「過熱」
   - 航運業：營收成長波動極大（長榮 2021 YoY +200%，2023 YoY -60%），20% 閾值毫無意義
   - **正確做法**：每個產業應有自己的歷史分位數閾值（如 75th percentile）

2. **沒有考慮基期效應**：
   - `RevenueGrowthYoY` 是年增率，受基期影響極大
   - 2023 年基期低（半導體庫存調整），2024 年增長率高不代表真的「擴張」
   - 應該使用 2 年複合成長率（CAGR）或與產業中位數比較

3. **庫存週轉率的產業差異**：
   - `InventoryTurnover > 6.0` 對電子業是高效，但對汽車業是低效（汽車週轉率約 8-12）
   - 統一閾值無法反映產業特性

#### 🟡 中等问题：風險評分的線性假設

`internal/industry/risk.go`：

```go
// 客戶集中度風險
if topCustomerShare > 30 { riskScore += 0.4 }
if topCustomerShare > 50 { riskScore += 0.3 }
if usExposure > 50      { riskScore += 0.2 }
if usExposure > 70      { riskScore += 0.1 }
```

**設計不合理之處：**

1. **線性叠加不合理**：
   - 客戶佔比 50% + 美國暴露 70% = 風險分數 1.0（滿分）
   - 但這兩個風險因子高度相關（台灣電子業的大客戶通常就是美國公司）
   - 簡單相加會**重複計算**（double counting），實際風險應小於 1.0

2. **對台灣產業結構理解不足**：
   - 台積電的客戶集中度：蘋果約 25%、NVIDIA 約 15%、AMD 約 10%
   - 按照此模型，台積電的客戶集中度風險 = 0.4（top > 30）+ 0.2（US > 50）= 0.6
   - 但台積電的商業模式本質上就是「為少數大客戶提供先進製程」，這是**結構性特徵**而非風險
   - 將結構性特徵標記為「中高風險」會導致系統**錯誤減碼**優質資產

3. **沒有考慮客戶品質**：
   - 客戶是蘋果（AAA 信用）vs 客戶是某中國手機品牌，風險完全不同
   - 模型沒有區分客戶信用評級

#### 🟡 中等问题：API 速率限制缺乏彈性

`internal/marketdata/` 中各 provider 的速率限制：

| Provider | 限制 | 超時 |
|----------|------|------|
| TWSE | 0.6 req/s | 15s |
| Fubon | 300 req/min | 10s |
| TEJ | 5 req/s | 30s |
| Fugle | 60 req/min | 未明確 |

**設計不合理之處：**

1. **沒有降級策略**：當某個 API 到達速率限制時，系統會簡單等待（`rateLimiter.Wait`），但沒有優先級隊列。盤中即時報價（urgent）和歷史資料回填（non-urgent）使用同一個 limiter。

2. **超時值固定**：TWSE 15 秒超時在網路擁塞時可能不足，但無法動態調整。應該根據成功率自適應調整（指數退避）。

3. **沒有熔斷機制**：如果某個 provider 連續失敗，應自動切換到備援 provider，而非持續重試。

### 軟件工程角度評估

#### 🔴 極嚴重問題：參數分散且無版本控制

```
industry/
├── seasonality.go          # 季節性因子（AdjustmentFactor, HistoricalAccuracy）
├── cycle.go                # 景氣循環閾值（RevenueGrowth > 0.20）
├── risk.go                 # 風險評分規則（topCustomerShare > 30）
├── types.go                # 產業權重（Weight: 0.25）
├── linkage.go              # 供應鏈衝擊傳播（correlation * 0.8）
└── seasonal_performance.go  # 季節性績效（獨立檔案，重複邏輯？）
```

**問題：**
1. **同一概念散佈多處**：「產業配置權重」在 `types.go`（靜態權重）和 `sector_rotator.go`（動態調整）中重複定義
2. **沒有單一真相來源**：如果研究員想調整「半導體權重」，需要修改多個檔案
3. **沒有參數 snapshot**：無法追溯「某個實驗執行時的產業權重配置」

#### 🔴 嚴重問題：缺乏參數驗證

```go
// industry/seasonality.go
AdjustmentFactor: 1.15,  // 沒有檢查是否 > 0

// industry/cycle.go
revenueGrowth > 0.20  // 沒有檢查輸入數據是否為負

// industry/risk.go
topCustomerShare > 30  // 沒有檢查單位（是 % 還是 0-1？）
```

在 `risk.go` 中發現嚴重單位不一致：
- `RevenueSharePct` 欄位名暗示是百分比（0-100）
- 但 `topCustomerShare > 30` 的比較暗示可能是 0-100 的整數
- 然而 `RiskScore` 計算中 `riskScore += 0.4` 暗示最終分數是 0-1
- 如果傳入 `RevenueSharePct = 0.5`（意圖 50%），模型會視為 0.5 < 30，沒有風險 — **嚴重錯誤**

#### 🟡 次要問題：測試無法覆蓋參數邊界

測試檔案中的參數是硬編碼的：

```go
// industry/risk_test.go
func TestCustomerConcentrationRisk() {
    customers := []CustomerConcentration{
        {CustomerName: "Apple", RevenueSharePct: 50.0},  // 硬編碼
    }
    risk := rm.CalculateCustomerConcentrationRisk("2330.TW")
    // 測試只驗證固定輸入，無法測試閾值邊界
}
```

無法回答以下問題：
- 如果 `topCustomerShare = 29.9`，風險分數是否正確地從 0.4 降至 0？
- 如果 `usExposure = 70.1`，兩個條件（>50 和 >70）是否同時觸發？
- 如果所有客戶都在 US（100%），風險分數是否超過 1.0？

### 建議修復方案

1. **產業參數集中化管理**：
```go
// internal/config/parameters.go 新增
type IndustryParameters struct {
    SectorWeights          ParameterMetadata[map[string]float64]
    SeasonalPatterns       ParameterMetadata[[]SeasonalPattern]
    CycleThresholds        ParameterMetadata[map[string]CycleThreshold]
    ConcentrationRiskRules ParameterMetadata[[]RiskRule]
}
```

2. **每個產業獨立閾值**：
```go
type CycleThreshold struct {
    IndustryID           string
    ExpansionRevenuePct  float64  // 半導體: 0.30, 金融: 0.05
    ExpansionProfitPct   float64
    RecoveryRevenuePct   float64
    RecoveryProfitPct    float64
}
```

3. **風險評分非線性化**：
```go
// 使用 Copula 或簡單的乘法組合
riskScore := 1.0 - (1.0-customerRisk)*(1.0-geographicRisk)
```

4. **增加單位強制型別**：
```go
type Percentage float64  // 0-100
type Decimal float64      // 0-1
```

---

## 限制 3：Hot-reload 缺失

### 現況

參數變更後需要重啟服務才能生效。`internal/config/parameters.go` 使用單例模式：

```go
var (
    parametersConfig *ParametersConfig
    parametersPath   = envOr("ATLAS_PARAMETERS_CONFIG", "configs/parameters.json")
)

func GetParametersConfig() *ParametersConfig {
    if parametersConfig == nil {
        cfg, _ := LoadParametersConfig(parametersPath)
        parametersConfig = cfg
    }
    return parametersConfig
}
```

### 金融專業角度評估

#### 🔴 嚴重問題：無法應對市場突發事件

**場景：2024 年 8 月 5 日（日元套息交易平倉）**

1. **事件**：日圓單日升值 5%，VIX 飆升至 65
2. **系統反應**：Realtime 模組檢測到高波動，觸發風險警報
3. **操作員決策**：需要將 `RiskParameters.MaxDrawdownPct` 從 8% 調降至 5%（更嚴格的風控）
4. **當前限制**：必須重啟服務，期間：
   - 所有進行中的模擬實驗中斷
   - 盤中交易信號停止產生（如果 live trading 啟用）
   - 重啟期間（30-60 秒）無風險監控

**金融影響**：
- 30 秒的重啟窗口在極端市場中可能是災難性的
- 2020 年 3 月 12 日（黑色星期四），台股在開盤後 10 分鐘內下跌 7%
- 如果風險參數需要調整，重啟期間系統無法執行止損

#### 🟡 次要問題：實驗迭代效率低下

投資研究的核心流程是**假設 → 調參 → 回測 → 評估**：

```
傳統流程：
1. 修改參數（5 分鐘）
2. 重啟服務（1 分鐘）
3. 執行回測（10 分鐘）
4. 評估結果（5 分鐘）
→ 每次迭代 21 分鐘

理想流程：
1. 修改參數（5 分鐘）
2. Hot-reload（1 秒）
3. 執行回測（10 分鐘）
4. 評估結果（5 分鐘）
→ 每次迭代 20 分鐘
```

看起來差異不大，但如果一個研究員一天要測試 20 組參數組合：
- 傳統流程：20 × 21 = 420 分鐘（7 小時）
- 理想流程：20 × 20 = 400 分鐘（6.7 小時）
- 節省 20 分鐘，但更重要的是**不中斷流程**

#### 🟡 次要問題：A/B 測試困難

在機器學習驅動的投資策略中，經常需要並行運行多個參數配置進行 A/B 測試。當前設計無法實現：

- 配置 A：KellyFraction = 0.25（保守）
- 配置 B：KellyFraction = 0.50（積極）
- 並行運行兩個配置，比較 30 天後的夏普比率

當前只能序列執行：先跑 A（重啟），再跑 B（重啟）。

### 軟件工程角度評估

#### 🔴 嚴重問題：單例模式違反並發安全

```go
// 當前設計
type RealTimeAdapter struct {
    params *config.RealtimeParameters  // ← 指向單例的指針
}

// 問題：如果 hot-reload 時替換了 parametersConfig，
// 已經存在的 RealTimeAdapter 仍然指向舊的參數
```

**Race Condition 場景**：
1. Goroutine A 執行 `GetVolatilityAdjustments()`，讀取 `vm.params.GARCH.HighVolThreshold`
2. 同時，管理員呼叫 `RollbackToSnapshot()`，替換 `parametersConfig`
3. Goroutine A 讀取到 nil pointer，panic

#### 🔴 嚴重問題：沒有觀察者模式

```go
// 理想設計
type ParameterObserver interface {
    OnParametersChanged(category string, old, new *ParametersConfig)
}

type VolatilityManager struct {
    params *RuntimeParameters
}

func (vm *VolatilityManager) OnParametersChanged(category string, old, new *ParametersConfig) {
    if category == "garch" || category == "all" {
        vm.RecalculateAll()
    }
}
```

當前設計：
- 沒有註冊機制
- 沒有事件廣播
- 各模組不知道參數已變更

#### 🟡 次要問題：缺乏配置熱加載的基礎設施

即使實現了 hot-reload，還需要：
1. **配置驗證**：reload 前驗證新配置合法性
2. **灰度發布**：先對 10% 的流量使用新配置
3. **自動回滾**：如果新配置導致錯誤率上升，自動恢復舊配置
4. **版本控制**：保留最近 N 個配置版本

當前完全沒有這些機制。

### 建議修復方案

1. **使用原子指針替換單例**：
```go
var parametersConfig atomic.Pointer[ParametersConfig]

func GetParametersConfig() *ParametersConfig {
    return parametersConfig.Load()
}

func ReloadParameters() error {
    cfg, err := LoadParametersConfig(parametersPath)
    if err != nil {
        return err
    }
    parametersConfig.Store(cfg)
    notifyObservers("all", cfg)
    return nil
}
```

2. **引入觀察者模式**：
```go
type ParameterStore struct {
    config    atomic.Pointer[ParametersConfig]
    observers []ParameterObserver
    mu        sync.RWMutex
}

func (s *ParameterStore) RegisterObserver(o ParameterObserver) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.observers = append(s.observers, o)
}
```

3. **文件系統監聽（可選）**：
```go
func (s *ParameterStore) Watch(path string) {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(path)
    
    go func() {
        for event := range watcher.Events {
            if event.Op&fsnotify.Write == fsnotify.Write {
                s.Reload()
            }
        }
    }()
}
```

4. **API 觸發 reload**：
```go
// POST /api/parameters/reload
func (h *Handlers) HandleReload(w http.ResponseWriter, r *http.Request) {
    if err := config.ReloadParameters(); err != nil {
        writeJSONError(w, 400, err.Error())
        return
    }
    writeJSON(w, 200, map[string]string{"status": "reloaded"})
}
```

---

## 綜合評估與優先級建議

| 限制 | 金融嚴重度 | 工程嚴重度 | 綜合評級 | 建議優先級 |
|------|-----------|-----------|----------|-----------|
| **限制 2：P2 參數未遷移** | 🔴 極高 | 🔴 極高 | **P0** | 立即處理 |
| **限制 1：WithParameters** | 🟡 中 | 🔴 高 | **P1** | 下個迭代 |
| **限制 3：Hot-reload** | 🟡 中 | 🟡 中 | **P2** | 規劃階段 |

### 為什麼限制 2 是 P0？

從**投資組合理論**的角度：
- 產業權重直接影響投資組合的預期報酬和風險特徵
- 根據 Markowitz 現代投資組合理論，即使權重偏差 5%，在槓桿作用下可能導致夏普比率下降 10-20%
- 當前半導體權重 25% vs 市場權重 60%，這是一個巨大的結構性 underweight
- 如果這個權重設定是基於「風險規避」的直覺而非量化回測，那麼系統可能長期跑輸大盤

從**監管合規**的角度：
- 如果這個系統未來用於管理客戶資產，未經實證支持的參數設定可能構成**未盡注意義務**（failure of fiduciary duty）
- 美國 SEC 和台灣金管會都要求投資決策有明確的、可審計的依據
- 當前 150+ 硬編碼參數沒有來源說明、沒有校準記錄、沒有變更歷史

### 為什麼限制 1 是 P1？

雖然 `WithParameters` 已補上，但底層的**參數設計哲學**問題更根本：
- `VolatilityManager` 混合了建構子參數和 runtime 參數
- 這種不一致性會傳染給其他模組（如 `RiskManager`、`Sizer`）
- 建議在下個迭代中統一所有 portfolio 模組的參數注入模式

### 為什麼限制 3 是 P2？

Hot-reload 是**運維效率**問題，不是**投資品質**問題：
- 在模擬環境中，重啟的代價可以接受
- 在生產環境中，可以通過藍綠部署（blue-green deployment）暫時解決
- 但長期來看，隨著參數數量增加和實驗頻率提高，hot-reload 會變得越來越重要

---

## 附錄：P2 未遷移參數詳細清單

### Marketdata（~30 個）

```
twseRateLimit       = 0.6   // TWSE OpenAPI
twseRateBurst       = 3
twseTimeout         = 15 * time.Second

fubonIntradayRateLimit    = 300  // Fubon
fubonHistoricalRateLimit  = 60
fubonTimeout              = 10 * time.Second

tejCallsPerSecond   = 5    // TEJ
tejTimeout          = 30 * time.Second

fugleRateLimit      = 60   // Fugle（監控代碼中，非明確定義）
```

**金融影響**：API 速率限制影響數據即時性。盤中若因 rate limit 無法獲取報價，可能錯過交易機會或無法及時止損。

### Industry（~150 個）

```
// 產業權重（9 個 Level-1）
semiconductor: 0.25
ai_supply_chain: 0.20
robotics: 0.08
financials: 0.15
shipping: 0.10
energy: 0.05
electronics_components: 0.07
consumer: 0.05
industrial: 0.05

// 季節性因子（7 個模式 × 4 個參數 = 28 個）
spring_festival: AdjustmentFactor=1.15, HistoricalAccuracy=0.70, AvgMarketReturn=0.032
earnings_window: AdjustmentFactor=1.10, HistoricalAccuracy=0.55, AvgMarketReturn=0.015
// ... 等

// 景氣循環閾值（4 個指標 × 4 個階段 × 多個產業）
Expansion:  RevenueGrowthYoY > 0.20, ProfitGrowthYoY > 0.20
Recovery:   RevenueGrowthYoY > 0.05, ProfitGrowthYoY > 0.05
// ... 等

// 風險評分（客戶集中度）
topCustomerShare > 30: +0.4
topCustomerShare > 50: +0.3
usExposure > 50:       +0.2
usExposure > 70:       +0.1

// 新聞延遲風險
latencyGap / 24.0: 線性映射至 0-1

// 非對稱風險
dropPct > 0.10: 嚴重
dropPct > 0.07: 高度
dropPct > 0.05: 中度
```

### Strategy（~30 個）

```
MinSwitchInterval: 5 * 24 * time.Hour  // 策略切換冷卻期
SwitchThreshold:   0.10               // 分數差異閾值
GetScore window:   20                  // 觀察窗口
```

**金融影響**：策略切換冷卻期 5 天過長。在快速變化的市場中（如 2024 年 AI 主題輪動），5 天可能錯過整個行情。應根據市場波動率動態調整。

---

*報告生成時間：2026-05-04*  
*評估範圍：internal/config, internal/portfolio, internal/industry, internal/marketdata, internal/strategy*  
*參考標準：Markowitz Modern Portfolio Theory, Engle GARCH Model, SEC Fiduciary Duty Guidelines*