# AGENTS.md — internal/forecast

個股方向性預測引擎。Phase 3.5 M4 已 shipped（2026-07-02）。

> **分類語意權威**：`docs/specs/foreign-flow-forecast-spec.md`。

---

## 公開 API

### 1. ForecastEngine（通用個股方向預測）

```go
// engine.go
type ForecastEngine struct{ … }

func NewForecastEngine() *ForecastEngine
func (e *ForecastEngine) Predict(symbol string, horizon time.Duration) (ForecastResult, error)
```

**當前實作為 stub**：永遠回傳 `DirectionHold`、`Conviction=50`。真實方向性邏輯由 `ForeignForecast.Score` 提供。

### 2. ForecastResult

```go
// types.go
type Direction string

const (
    DirectionBullish Direction = "bullish"
    DirectionBearish Direction = "bearish"
    DirectionHold    Direction = "hold"
)

type ForecastResult struct {
    Symbol      string    `json:"symbol"`
    Conviction  int       `json:"conviction"` // 0-100
    Direction   Direction `json:"direction"`
    Horizon     string    `json:"horizon"` // "7d", "30d"
    Scenarios   []string  `json:"scenarios,omitempty"`
    GeneratedAt time.Time `json:"generated_at"`
}
```

### 3. TradeSignal（forecast → trade 橋接資料結構）

```go
// types.go
type TradeSignal struct {
    Symbol           string    `json:"symbol"`
    Action           string    `json:"action"`            // "buy", "sell", "hold"
    WeightMultiplier float64   `json:"weight_multiplier"` // 0.0 - 2.0
    Rationale        string    `json:"rationale"`
    SourceForecastID string    `json:"source_forecast_id"`
    GeneratedAt      time.Time `json:"generated_at"`
}
```

`TradeSignal` 由 `forecast_bridge` 的 `Adapter.ToTradeSignal` 從 `ForecastResult` 轉換而來，**不是由 forecast 模組直接產生**。

---

## ForeignForecast（規則型外資流向預測）

### Scorecard

```go
// foreign_forecast.go
type Input struct {
    ForeignFuturesOIZ  float64 // TAIFEX 外資期貨 OI 淨額 60日 Z-score
    ForeignSpot5DSlope float64 // 外資現貨 5 日線性回歸斜率（億/日）
    TSMADRChangePct    float64
    SPXChangePct       float64
    NDXChangePct       float64
    USDTWDChangePct    float64 // 正 = 台幣升值（符號反转计分）
    VIX                float64
}

type Result struct {
    Date        string
    Direction   ForeignDirection // "bullish" | "bearish" | "neutral"
    Probability float64          // 0..1
    Score       float64          // raw [-1, 1]
}

func Score(date string, in Input) Result
```

**權重配置**（`foreign_forecast.go` §4）：
| 特徵 | 權重 | 飽和 scale |
|------|------|-----------|
| ForeignFuturesOIZ | 0.30 | ±1.0 |
| ForeignSpot5DSlope | 0.20 | ±15.0（億/日） |
| TSMADRChangePct | 0.15 | ±2.0% |
| SPXChangePct | 0.15 | ±1.5% |
| NDXChangePct | 0.10 | ±2.0% |
| USDTWDChangePct | 0.10 | ±0.5%（符號反轉） |
| VIX > 25 | −0.10 | 遞減至 −0.10 |

方向判定：`Probability ≥ 0.60` → bullish，`≤ 0.40` → bearish，否則 neutral。

### 常數

```go
const (
    SignificanceThresholdTWD  int64 = 3_000_000_000 // 30 億台幣，非中性門檻
    MinSamplesForCalibration  = 90                  // 校準最小樣本數
    MinHitRateForCalibration   = 0.55               // 校準最低命中率
)
```

### Ledger（預測記錄）

```go
type Record struct {
    Date           string           `json:"date"`
    Direction      ForeignDirection `json:"predicted_direction"`
    Probability    float64          `json:"probability"`
    Score          float64          `json:"score"`
    ActualOutcome  ForeignDirection `json:"actual_outcome,omitempty"` // T+1 填入
    ActualNet      int64            `json:"actual_net,omitempty"`     // T+1 填入
    Correct        *bool            `json:"correct,omitempty"`       // nil=pending
}

type Ledger struct{ … }

func NewLedger(dir string) *Ledger
func (l *Ledger) Write(r Record) error
func (l *Ledger) Load(date string) (Record, error)
func (l *Ledger) List(n int) ([]Record, error) // 最近 n 筆記錄，oldest-first；格式錯誤的檔案會 skip
```

**注意**：`List` 遇到格式錯誤的 JSON 檔案會 `continue`（不回錯誤）。`Load` 失敗則整體失敗。

### Calibration gate

```go
type CalibrationStatus struct {
    Calibrated bool
    Samples    int
    HitRate    float64
    Reason     string
}

func Calibrate(records []Record) CalibrationStatus
func Judge(prev Record, actualNetTWD int64) Record
func TodayDate() string
```

**Calibrate 邏輯**：
1. 樣本數 < 90 → 未校準（"校準中"）
2. 命中率 < 55% → 未校準（"校準中"）
3. 否則 → `Calibrated: true`

---

## 關鍵陷阱

| 陷阱 | 說明 |
|------|------|
| **ForecastEngine 是 stub** | `Predict` 目前永遠回 `DirectionHold/Conviction=50`；真實邏輯在 `ForeignForecast.Score`。 |
| **上下游關係：forecast → forecast_bridge** | `forecast` 輸出 `ForecastResult`/`TradeSignal`；`forecast_bridge` 的 `Adapter` 負責依 conviction thresholds（70/30）轉換為可下單的 `TradeSignal.Action`。兩者必須同步變更。 |
| **ForeignForecast.Score 是封閉式規則** | 所有係數和 scale 均 hardcoded 在 `Score` 函式內（見 `foreign_forecast.go`），**不從參數系統讀取**。調整權重需改 code 並重新校準。 |
| **USD/TWD 符號反轉** | `Score` 內 `-in.USDTWDChangePct`（line 70）；台幣升值（正）為多頭訊號。 |
| **VIX 恐慌閾值** | VIX > 25 才觸發 penalty；≤ 25 不加分也不扣分。 |
| **Calibration 需要 90 天 warm-up** | Ledger 累積 < 90 樣本前，`Calibrate` 永遠回 `Calibrated: false`。 |
| **ForecastEngine 無 circuit breaker** | 目前 `ForecastEngine` 本身沒有熔斷機制；如需熔斷，應由 `forecast_bridge` 或 caller 實作。 |
| **SignificanceThresholdTWD 只影響 Judge** | 30 億台幣門檻定義在 `SignificanceThresholdTWD`，但 `Score` 本身不檢查這個值；`Judge` 填入 `ActualOutcome` 時才使用。 |

---

## 依賴關係

```
forecast
├── forecast_bridge   // 消費 ForecastResult → TradeSignal
│   └── adapter.go: Adapter.ToTradeSignal
├── orchestrator      // ForecastBridgeStrategy
│   └── executor_strategies.go: PredictAll([]forecast.TradeSignal, error)
└── strategy
    └── directional_trade_layer.go: ApplySignal(forecast.TradeSignal)
```

無外部第三方依賴（pure stdlib）。

---

## 測試

```bash
go test ./internal/forecast/...

# 覆蓋範圍：
# - ForecastEngine.Predict：symbol validation、horizon validation、result fields
# - Score：bullish/bearish/neutral 方向判定、probability bounded [0,1]
# - Judge：T+1 outcome 填入、correct 布林更新
# - Calibrate：90 樣本 warm-up、55% 命中率 gate
# - Ledger：Write/Load/List round-trip、empty dir
```
