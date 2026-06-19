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

---

## 組態設定（2026-06-20 裁定）

**`internal/risk` 模組統一使用全域 `config.GetParametersConfig()`，未採用 per-module 組態檔。**

### 歷史上曾存在的 `internal/risk/configs/parameters.json`

- **狀態**：未追蹤孤兒（建立於 2026-06-20 00:56:18，從未進入 git），已於 commit `chore(risk): remove orphan per-module config (use global ParametersConfig)` 刪除
- **體積**：190,833 bytes（與全域 `configs/parameters.json` 198,595 bytes 高度重疊，研判為 dump 殘留）
- **引用數**：0 個 `.go` 檔案以字串引用、0 個 `fsnotify`/watcher、0 個排程任務、0 個寫入器、0 個 lockfile 創建者
- **違反規範**：`docs/TRAPS.md`「繞過 ParametersConfig 硬編碼參數」明確禁止此模式；權威計畫 `docs/superpowers/plans/2026-05-21-risk-module-fixes.md` 6 個 Phase 全部走全域 ParametersConfig

### 新增 / 修改風險參數的正確做法

1. 擴充 `ParametersConfig.Risk` 結構於 `internal/config/parameters.go`（見 `RiskParameters` 類型，line 1241 附近）
2. 為新欄位加入 `ParameterMetadata[T]` 包裝（含 `Rationale` / `Source` / `Todo`）
3. 預設值置於 `internal/config/parameters_defaults.go`
4. 透過 `LockedSaveWithRollback` 寫回 `configs/parameters.json`（根目錄，預設路徑；可用 `ATLAS_PARAMETERS_CONFIG` 或 `ATLAS_PARAMETERS_CONFIG_PATH` 環境變數覆寫）
5. 校準管線（Bayesian optimizer / calibrator）會自動讀取/寫回，無需手動同步

### 為什麼不做 per-module 風險 loader

- 全域 loader 是中央化單例 + 鎖定回寫模式（`internal/config/parameters.go:2276` 起）
- 風險子結構已內嵌於全域 schema，`self_calibrate.go:191` 明確寫回全域路徑 — 拆檔會造成 split-brain
- 唯一 per-module loader 先例 `internal/risktest/config.go` 使用 CLI flag，無 env 變數、無 lock，與本模組需求不符
- 詳見 `.opencode/handoffs/risk-config-loader.md` 完整 Q1-Q4 調查記錄

### 金融工程權衡分析（per-module vs global，2026-06-20 補）

**本節以金融工程視角補完上述拒絕 per-module 的理由，避免未來僅看經驗性證據時再度發起討論。**

| 維度 | Per-module（orphan 暗示方向） | Global（現狀） |
|------|-------------------------------|---------------|
| 參數迭代速度 | 各子模組可獨立調參 | 須走完整 calibration pipeline |
| 生產環境一致性 | 兩份 source of truth 必然漂移 | 單一檔案 = 單一時戳 = 單一校準事件 |
| 校準原子性 | 拆檔後 `LockedSaveWithRollback` 無法保證跨檔原子寫入 | 單檔 lock + write + rollback（`parameters.go:2715-2722`） |
| 熱重載一致性 | 需 N 個熱重載路徑，每個獨立 lock | 單例 `ReloadParametersConfig`（`parameters.go:2576`） |
| 稽核軌跡 | N 個時間戳須 join 才能還原 | 單一時戳即完整快照 |
| 失效情境 | 校準崩潰時 runtime 可能讀到「昨天的 factor + 今天的 risk」拼接 | 寫失敗 = 完全沒寫，runtime 續用上一個完整快照 |

### 失效情境具體化（live trading 視角）

atlas-go 是 **live trading 系統**（`internal/portfolio/risk_manager.go:95-97` 即時消費 `params.Risk.*`），不是研究工具。Per-module 在此情境下的具體失效案例：

1. T0 — calibrator 開始 Bayesian 校準（讀全域 → 計算新 VaR/Drawdown 參數）
2. T1 — 計算完成，準備寫回
3. T2 — 部分寫入成功（factor 寫到全域，risk 寫到 per-module）
4. T3 — 程序崩潰 / disk full / 網路斷線
5. T4 — 重啟後 runtime 讀全域 → 拿到新 factor + 舊 risk

**對 live trading 的後果**：
- 新 factor 認為「成長股環境」→ 部位放大 1.5x
- 舊 risk 不知道 → Drawdown 熔斷閾值仍為前一代
- -3% 正常下跌可能錯誤觸發 halt，或更糟 — 應該觸發時未觸發
- 結算損失難以解釋（稽核記錄顯示參數「看起來對」，實際跨檔不一致）

這是 **silent corruption** — 系統不報錯，行為已錯。金融系統最致命的失效模式。

**額外強證據**：`internal/risk/self_calibrate.go:187-189` 顯示 calibrator 處理的 risk 欄位（`risk_max_position_size`、`risk_max_daily_loss_pct`）正是 orphan 那 190KB JSON 的核心內容。**orphan 就是 calibrator 寫入鏈的「鏡像 dump」被放在錯誤路徑上**，進一步證明拆檔會直接造成 split-brain。

### 替代方案（如未來真有 per-module 業務需求）

若業務需求演進到「某 risk 子模組需要獨立迭代」，正確做法是：
- 在 `ParametersConfig` 內做更細的子分組（`Risk.VaR` / `Risk.Drawdown` / `Risk.Capital`），不要拆檔案，而是在單一檔案內做邏輯分區
- 必要時用 `ReloadParametersConfig` 支援子集熱重載（hot-reload 已在 `parameters.go:2576` 預留 hook）

這能在保持單一 source of truth + 原子寫入的前提下，給出 per-module 帶來的 velocity 紅利。
