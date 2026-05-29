# AGENTS.md — internal/strategy

**成熟度**: evolving
**模組職責**: 策略註冊、選擇與比較，依據盤勢與績效動態切換投資策略。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Registry` | `registry.go` | 策略註冊表（內建 5 種預設策略：all_weather、growth、value、defensive、momentum） |
| `Selector` | `selector.go` | 依據盤勢與 ComparisonEngine 分數選擇最佳策略 |
| `ComparisonEngine` | `comparison.go` | 追蹤各策略交易紀錄，計算 Sharpe、最大回撤、勝率 |
| `StrategyAllocator` | `allocator.go` | 風險平價資金配置：依波動率倒數分配策略權重 |
| `Strategy` | `types.go` | 策略定義（ID、Agent 列表、風險偏好、盤勢偏好） |
| `StrategyMix` | `allocator.go` | 策略權重映射（Σw = 1），含 Validate() 檢查 |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **策略切換有冷卻期** | `Selector.shouldSwitch()` 檢查 `MinSwitchInterval`（預設來自參數配置），短時間內不會反覆切換。 |
| **無候選時 fallback** | `Selector.Select()` 在無 regime 匹配策略時回傳 `all_weather`，若連 all_weather 都沒有則回傳 `fallback` 策略。 |
| **ComparisonEngine 分數計算** | `GetScore()` 使用 Sharpe*0.4 + DailyReturn*30*0.3 + WinRate*0.3，歷史不足 days 時回傳 0.5。 |
| **Allocator 波動率預設** | `estimateVolatility()` 在資料不足 5 筆時回傳 0.20（年化預設波動率）。 |
| **權重上限 50% 下限 5%** | `StrategyAllocator` 預設 `maxWeight=0.50`、`minWeight=0.05`，以迭代方式重新正規化。 |

---

## 測試

- `go test ./internal/strategy/...`
- 涵蓋 Registry CRUD、Selector 切換邏輯、Allocator 風險平價計算、ComparisonEngine 分數計算

(End of file - total 36 lines)
