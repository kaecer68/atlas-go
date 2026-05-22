# AGENTS.md — internal/risk

`internal/risk` 向系統提供風險評估：VaR、回撤、資本階段、投資組合集中度、產業週期風險。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`escalateAction()` 靜默升級** | `MacroAwareDrawdownEngine.escalateAction()` 在多重風險面向一致時會將 `monitor` 靜默升級為 `halt`。日誌中看起來像正常的 `monitor`。 |
| **VaR 假設常態分佈** | 預設 VaR/CVaR 計算假設常態分佈報酬。在厚尾的台灣市場（極端事件）會低估尾部風險。可透過 `HistoricalVaR` 模式取得非參數 VaR。 |
| **Nil Provider 回傳零值評估** | `PortfolioRiskProvider` 若為 nil 會回傳所有欄位皆為 0 的 `PortfolioRiskAssessment`，不報錯。可能被誤解為「無風險」。 |
| **資本階段持久化可能過時** | `CapitalPhaseController` 使用檔案持久化（`PersistedState`）。若兩個 controller 操作同一檔案會發生 last-write-wins，導致狀態損毀。 |
| **產業權重不一致** | `CycleTrackerRiskProvider` 從 `map[string]float64` 取得產業權重；若此 map 與實際投資組合權重不同，評估會有偏差。 |
| **熔斷器未連結** | risk 的回撤邏輯與 `internal/apigateway` 的 `CircuitBreaker` 沒有直接關聯。降低風險的決策**不會**被動觸發 API 層熔斷。 |

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `VaRCalculator` | `var_calculator.go` | 計算 VaR/CVaR/最大回撤 |
| `MacroAwareDrawdownEngine` | `macro_aware_drawdown.go` | 多層回撤決策（`none→monitor→reduce→halt`） |
| `CapitalPhaseController` | `capital_controller.go` | 資本階段管理（`capital_accumulation→base→growth→harvest`） |
| `PortfolioRiskProvider` | `portfolio_risk.go` | 投資組合集中/曝險評估介面 |
| `IndustryRiskProvider` | `industry_risk.go` | 產業週期風險評估介面 |
| `ApprovalWorkflow` | `approval_workflow.go` | 人為介入風險核准工作流程 |

---

## 常見執行流程

```
MacroAwareDrawdownEngine.Evaluate(riskSnapshot, regime, narrativeEvents)
  → DrawdownDecision (action, rationale, positionScale)
  → GetPositionSizeAdjustment(decision) → float64
  → ShouldHaltTrading(decision) → bool
```

**注意**：`positionScale` 為乘法因子。`0.5` 表示「將所有倉位減半」— 不包含現金。若需限制總曝險的因子，請改用 `CapitalPhaseController.GetCapitalLimit()`。
