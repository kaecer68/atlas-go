# AGENTS.md — internal/risk

`internal/risk` 向系統提供風險評估：VaR、回撤、資本階段、投資組合集中度、產業週期風險。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`escalateAction()` 靜默升級** | `MacroAwareDrawdownEngine.escalateAction()` 在多重風險面向一致時會將 `monitor` 靜默升級為 `halt`。 |
| **VaR 假設常態分佈** | 預設 VaR/CVaR 假設常態分佈報酬，在厚尾台灣市場會低估尾部風險。可透過 `HistoricalVaR` 取得非參數 VaR。 |
| **Nil Provider 回傳零值評估** | `PortfolioRiskProvider` 若為 nil 會回傳所有欄位皆為 0 的 `PortfolioRiskAssessment`，不報錯。 |
| **資本階段持久化可能過時** | `CapitalPhaseController` 使用檔案持久化。若兩個 controller 操作同一檔案會發生 last-write-wins。 |
| **產業權重不一致** | `CycleTrackerRiskProvider` 從 `map[string]float64` 取得產業權重；若與實際投資組合權重不同，評估會有偏差。 |
| **熔斷器未連結** | risk 的回撤邏輯與 `internal/apigateway` 的 `CircuitBreaker` 沒有直接關聯。 |

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `VaRCalculator` | `var_calculator.go` | 計算 VaR/CVaR/最大回撤 |
| `MacroAwareDrawdownEngine` | `macro_aware_drawdown.go` | 多層回撤決策（`none→monitor→reduce→halt`） |
| `CapitalPhaseController` | `capital_controller.go` | 資本階段管理 |
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

**注意**：`positionScale` 為乘法因子。`0.5` 表示「將所有倉位減半」— 不包含現金。若需限制總曝險，請改用 `CapitalPhaseController.GetCapitalLimit()`。

---

## 組態設定

**`internal/risk` 統一使用全域 `config.GetParametersConfig()`，禁止 per-module 組態檔。**歷史背景與清理紀錄見 `docs/audit/2026-06-20-risk-orphan-config.md`。

### 正確做法
1. 擴充 `ParametersConfig.Risk` 或 `ParametersConfig.RiskGate` 結構於 `internal/config/parameters.go`(兩者皆為 top-level field;`Risk` 是全域風險參數、`RiskGate` 是 trade gate 門檻,兩個獨立 config block 各自有預設值於 `internal/config/parameters_defaults.go`)
2. 為新欄位加入 `ParameterMetadata[T]` 包裝（含 `Rationale` / `Source` / `Todo`）
3. 預設值置於 `internal/config/parameters_defaults.go`
4. 透過 `LockedSaveWithRollback` 寫回 `configs/parameters.json`
5. 校準管線自動讀取/寫回，無需手動同步

### 為什麼禁止 per-module loader
- 全域 loader 是中央化單例 + 鎖定回寫模式，確保生產環境單一 source of truth。
- 單一檔案 = 單一時戳 = 單一校準事件 = 稽核軌跡完整。
- `LockedSaveWithRollback` 保證原子寫入；拆檔會造成 split-brain，live trading 下可能產生 silent corruption。
- 若未來需要獨立迭代，應在 `ParametersConfig` 內做更細子分組（`Risk.VaR` / `Risk.Drawdown` / `Risk.Capital`），而非拆檔案。
