# FactorWeightRegression 事件

> **事件常數**：`EventFactorWeightRegression = "portfolio.factor.regression"`
> **對應 EventType 字串值**：`portfolio.factor.regression`
> **Severity**：info（PD-W9-1）
> **Schema Version**：1

## 觸發條件

FactorWeightRegressionDetector 訂閱 `EventRegimeChange`，當 regime 變化時：
1. 透過 constructor DI 注入的 `WeightProvider.GetWeights(newRegime)` 讀取當前因子權重
2. 與上次快取的權重比較，計算 regression score = `Σ|current - prev|`
3. 若 score ≥ 0.5（`FactorWeightRegressionThreshold`）→ emit `EventFactorWeightRegression`

**重要**：第一次 regime change（無 prev weights）不 emit（建立 baseline）。

## Payload Schema

```json
{
  "regime": "bear",
  "factor_diffs": {
    "momentum": 1.5,
    "value": -0.5
  },
  "regression_score": 2.0,
  "threshold": 0.5
}
```

| 欄位 | 型別 | 說明 |
|------|------|------|
| `regime` | string | 新的 regime（change 後） |
| `factor_diffs` | map[string]float64 | 每個因子的權重位移（new - prev），僅含非零值 |
| `regression_score` | float64 | 總 regression score = Σ\|new - prev\| |
| `threshold` | float64 | 觸發閾值（固定 0.5） |

## 對應 Source API

- `EventRegimeChange` payload（`eventbus.RegimeEventPayload.NewRegime`）
- `WeightProvider.GetWeights(regime string) map[string]float64`（抽象介面）
  - 實際實作：`*portfolio.FactorWeightEngine.GetWeights`（`internal/portfolio/factor_weight_engine.go:79`）
  - 透過 constructor DI 注入 `factorWeightRegressionDetector`，**monitoring/service 不 import portfolio**

## 對應 Alert Rule

- 對應 Prometheus rule 檔案已於 2026-07-25 移除：原 `monitoring/rules/disabled/wave9_factor_weight_regression.yml` 引用 `factor_weight_regression_score` 但該 metric 從未 emit；`EventFactorWeightRegression` 仍透過 eventbus 發布，未來若實作 metric emit 應連同 rule 重新建立。

## 對應 Service

- `internal/monitoring/service/factor_weight_regression.go`：`FactorWeightRegressionDetector` 介面 + 實作
- `internal/monitoring/service/weight_provider.go`：`WeightProvider` 介面定義
- 依賴注入：`NewFactorWeightRegressionDetector(bus eventbus.EventBus, provider WeightProvider)`
- 由 `internal/monitoring/dashboard_api.go` 或同等 wiring 程式注入 `*portfolio.FactorWeightEngine`

## 測試

- `internal/monitoring/service/factor_weight_regression_test.go`：
  - `TestFactorWeightRegression_NoPriorWeightsNoEmit`：首次 regime 不 emit（建立 baseline）
  - `TestFactorWeightRegression_SmallChangeNoEmit`：score < 0.5 不 emit
  - `TestFactorWeightRegression_LargeChangeEmit`：score ≥ 0.5 emit
  - `TestFactorWeightRegression_NilProviderNoEmitNoPanic`：provider nil 不 panic
  - `TestRegressionScore`：單元測試 regression score 計算（含完全取代 a→b 場景）

## Forward-Compat 驗證

- ✅ 只訂閱 EventRegimeChange（eventbus.go 不在 #611 list）
- ✅ 只透過 WeightProvider 介面讀 weights（不直接 import portfolio）
- ✅ factor_weight_regression.go 屬於 monitoring/service，零 #611 檔案依賴

## DI 風險（已記錄）

依 Wave 9 plan §7 Risks：
- `WeightProvider` 介面破壞（`GetWeights` 簽名變動）→ 機率低，影響中
- FactorWeightEngine DI 注入失敗（如 main.go 重構後忘了接線）→ 機率中，影響中
  - **緩解**：Wave 9.3 啟動時加 runtime assertion：若 `provider == nil` 不 emit，log warning 即可，不 panic

## 啟用情境（建議）

- **Risk management**：regime 切換時因子權重位移過大表示市場結構變化，建議人工 review
- **Factor drift 量化**：長期追蹤 regression_score 趨勢，識別因子失效
- **回測驗證**：Wave 9.6 整合測試時驗證 regression score 與回測 sharpe 比率相關性