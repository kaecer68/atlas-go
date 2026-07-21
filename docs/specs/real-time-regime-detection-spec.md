# Real-Time Regime Detection & Agent Adaptation — Architecture Specification

## 1. Overview

本規格定義 `atlas-go` 即時市場狀態偵測（regime detection）與代理權重動態調整（agent weight adaptation）模組的設計目標、介面契約與實作約束。

**模組位置**: `internal/realtime/`

**核心能力**:
- 亞秒級市場狀態分類（7 種 regime type）
- 基於即時數據窗口的自動 regime 偵測
- 偵測到 regime 變化時動態調整 agent 權重
- 可配置的靈敏度與權重變動限制

---

## 2. Regime 分類體系

### 2.1 RegimeType

```go
type RegimeType string

const (
    RegimeCalm         RegimeType = "calm"          // 平穩盤
    RegimeVolatile     RegimeType = "volatile"      // 高波動
    RegimeTrendingUp   RegimeType = "trending_up"   // 上升趨勢
    RegimeTrendingDown RegimeType = "trending_down" // 下降趨勢
    RegimeReversing    RegimeType = "reversing"     // 反轉訊號
    RegimeBreakout     RegimeType = "breakout"      // 突破（價量齊揚）
    RegimeBreakdown    RegimeType = "breakdown"     // 破底（價量齊跌）
)
```

### 2.2 偵測邏輯（優先序）

| 優先序 | 條件 | Regime |
|--------|------|--------|
| 1 | 偵測到反轉型態 | `reversing` |
| 2 | 成交量飆升 + 價格變動 > 2× 門檻 | `breakout` / `breakdown` |
| 3 | 波動率 > 門檻 | `volatile` |
| 4 | 價格趨勢 > 門檻 | `trending_up` / `trending_down` |
| 5 | 以上皆非 | `calm` |

---

## 3. 核心元件

### 3.1 MarketDataPoint

```go
type MarketDataPoint struct {
    Symbol    string    `json:"symbol"`
    Price     float64   `json:"price"`
    Volume    float64   `json:"volume"`
    Bid       float64   `json:"bid"`
    Ask       float64   `json:"ask"`
    Spread    float64   `json:"spread"`
    Timestamp time.Time `json:"timestamp"`
}
```

### 3.2 RegimeDetector

```go
type RegimeDetector struct {
    windowSize           int
    volatilityThreshold  float64
    volumeSpikeThreshold float64
    priceChangeThreshold float64
}

func NewRegimeDetector(params *config.RealtimeParameters) *RegimeDetector
func (rd *RegimeDetector) DetectRegime(data []MarketDataPoint) RegimeType
```

偵測演算法:
1. `calculateVolatility()` — 報酬率標準差
2. `detectVolumeSpike()` — 最新成交量 vs 前 N-1 筆平均
3. `calculatePriceTrend()` — 價格動能
4. `detectReversal()` — 反轉型態辨識

### 3.3 RealTimeAdapter

```go
type RealTimeAdapter struct {
    detector     *RegimeDetector
    dataWindows  map[string][]MarketDataPoint
    agentWeights map[string]map[string]float64
    config       *RealTimeConfig
    mu           sync.RWMutex
}

func NewRealTimeAdapter(params *config.RealtimeParameters) *RealTimeAdapter
func (rta *RealTimeAdapter) IngestData(point MarketDataPoint)
func (rta *RealTimeAdapter) RegisterAgent(agentID string, symbols []string, initialWeight float64)
func (rta *RealTimeAdapter) GetAgentWeight(agentID, symbol string) float64
func (rta *RealTimeAdapter) Start(ctx context.Context) error
func (rta *RealTimeAdapter) Stop()
```

**資料流**:
```
MarketDataPoint → IngestData() → dataWindows[symbol]
    → 每 N ms 檢查: DetectRegime(window) → regime change?
    → 若 regime 變化: AdjustWeights(regime) → agentWeights 更新
    → 發送 RegimeChangeEvent 至 eventbus
```

---

## 4. 參數配置

所有參數由 `internal/config/parameters.go` 的 `RealtimeParameters` 管理：

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `VolatilityThreshold` | 0.02 | 波動率門檻（2%） |
| `VolumeSpikeThreshold` | 2.0 | 成交量飆升倍數 |
| `PriceChangeThreshold` | 0.01 | 價格變動門檻（1%） |
| `MinConfidence` | 0.7 | 最小信心度（70%） |
| `WeightAdjustmentRate` | 0.10 | 權重調整速率 |
| `MaxWeightChange` | 0.50 | 單次最大權重變動 |
| `MinWeight` | 0.10 | 最低權重下限 |
| `UpdateIntervalMs` | 100 | 檢查間隔（毫秒） |

---

## 5. 與 Orchestrator 整合

`RealTimeAdapter` 透過以下方式與系統其他模組互動：

1. **EventBus**: regime 變化時發布 `RegimeChangeEvent`
2. **JANUS**: 接收 regime 變化通知，用於跨 cohort regime 偵測
3. **PRISM**: regime-specific 訓練佇列可根據即時 regime 調整優先序
4. **Portfolio**: agent 權重由 `GetAgentWeight()` 提供給 Darwinian 權重管理

```
RealtimeAdapter
    ├─ IngestData()        ← marketdata.Provider
    ├─ DetectRegime()      → EventBus (RegimeChangeEvent)
    │   ├─ → JANUS 跨 cohort 偵測
    │   └─ → PRISM 訓練佇列調整
    └─ GetAgentWeight()    → Portfolio Darwinian 權重
```

---

## 6. 測試策略

### 6.1 單元測試（`realtime_test.go`）

- `NewRealTimeAdapter` — 初始化驗證
- `DefaultConfig` — 參數預設值驗證
- `IngestData` — 數據注入與窗口管理
- `RegisterAgent` / `GetAgentWeight` — 權重註冊與查詢
- `DetectRegime` — 各 regime 型態偵測正確性

### 6.2 整合測試

- 多 symbol 併發 `IngestData` → regime 偵測正確性
- `Start()` / `Stop()` 生命週期管理
- Regime 變化 → EventBus 事件發布驗證

---

## 7. 檔案結構

```
internal/realtime/
├─ regime_adapter.go    # RealTimeAdapter + RegimeDetector 實作
└─ realtime_test.go     # 單元測試
```

---

## 8. 未來擴展

| 項目 | 狀態 | 說明 |
|------|------|------|
| WebSocket 即時報價 provider | 未實作 | 見 `internal/marketdata/` 現有 provider 架構 |
| 多 symbol 批次 regime 偵測 | 已實作 | `dataWindows` 支援多 symbol |
| 歷史 regime 回測驗證 | 規劃中 | 需與 JANUS 歷史 cohort 數據整合 |
| Fugle WebSocket 串接 | 未實作 | Fugle 為付費 API，circuit breaker 保護 |

---

## 9. 約束

1. **不修改現有 Provider**: realtime 模組獨立於 `marketdata.Provider` 體系
2. **不引入全域狀態**: `RealTimeAdapter` 由 caller 持有實例，依賴注入
3. **符合 Go 慣例**: 介面小而聚焦，錯誤包裝，`gofmt` 格式化
4. **支援 context.Context**: `Start()` / `Stop()` 支援 graceful shutdown

---

## 10. Status

**IMPLEMENTED** — `internal/realtime/` 已實作核心 regime 偵測與權重調整邏輯。

- ✅ RegimeDetector（7 種 regime 分類）
- ✅ RealTimeAdapter（數據窗口 + agent 權重管理）
- ✅ 參數化配置（經由 `config.RealtimeParameters`）
- ✅ 單元測試覆蓋
- 🔄 EventBus 整合（部分完成）
- 🔄 Orchestrator pipeline 整合（進行中）
