# AGENTS.md — internal/baseline

本檔定義 `internal/baseline` 套件的政策生命週期與版本控制規範。

---

## OVERVIEW

`internal/baseline` 負責管理系統的「基準政策」（Baseline Policy），包含提示詞覆寫（Prompt Overrides）與模擬約束（Simulation Constraints）。系統透過實驗（Experiment）驗證後，可將優異的候選方案「晉升」（Promote）為新的基準，或在必要時進行「回滾」（Revert）。

---

## POLICY LIFECYCLE

1.  **載入 (Load)**：由 `data/state/baseline_policy.json` 載入。若檔案不存在，則回傳 `DefaultPolicy()`。
2.  **晉升 (Promotion)**：
    *   僅限 `domain.ExperimentAccepted` 狀態的實驗結果可進行晉升。
    *   晉升時會自動遞增 `Version`。
    *   支援 `risk_rule_change`、`portfolio_constraint_revision`（更新 Constraints）與一般提示詞更新（更新 PromptOverrides）。
    *   晉升歷程記錄於 `Policy.Promotions`。
3.  **回滾 (Reversion)**：
    *   支援回滾至「上一版」(Last)、「指定版本」(ToVersion) 或「特定實驗之前」(ToExperiment)。
    *   回滾會重建該版本的政策狀態，並記錄於 `Policy.RevertHistory`。
4.  **持久化 (Save)**：每次異動均會更新 `LastUpdatedAt` 並寫回 JSON。

---

## 核心檔案職責

| 檔案 | 職責 |
|------|------|
| `policy.go` | 定義 `Policy` 結構、預設值、載入/儲存與晉升邏輯（`Promote`）。 |
| `manager.go` | 提供 `Manager` 封裝，協調實驗結果檔案載入與政策更新。 |
| `rollback.go` | 實作回滾邏輯（`Revert`）、版本解析與政策重建（`reconstructPolicyAtVersion`）。 |

---

## ANTI-PATTERNS

- **手動修改 JSON**：嚴禁直接編輯 `data/state/baseline_policy.json`，應透過 `cmd/promote-baseline` 或 `cmd/revert-baseline` 進行。
- **忽視版本一致性**：`Policy.Version` 是唯一的真理來源，重建政策時必須確保與 `Promotions` 歷史對齊。
- **未載入 Baseline 執行實驗**：實驗引擎必須先呼叫 `baseline.Load()`。若未載入，系統將使用預設約束，可能導致實驗結果與現況脫節。
- **靜默失敗**：晉升與回滾均需確保檔案寫入成功，不可忽略 `Save()` 的回傳錯誤。

---

## 執行期政策強制

`internal/baseline/trigger.go` 提供執行期政策強制元件 `Trigger`，訂閱 `EventPositionUpdate` 並依現行 baseline policy 的 simulation constraints 評估每個部位。

### 核心型別

| 型別 | 位置 | 說明 |
|------|------|------|
| `Trigger` | `trigger.go` | 訂閱事件匯流排、評估 policy violation 的執行期守門員 |
| `Violation` | `trigger.go` | 單一違規記錄：symbol / field / actual / limit / severity / message |

### 運作方式

1. `NewTrigger(manager, bus)` 建立實例；`Start(ctx)` 訂閱 `EventPositionUpdate`。
2. `onPositionUpdate` 收到 `PositionEventPayload` 後載入 `manager.path` 的現行 policy。
3. `evaluate()` 對每個部位執行三項檢查，回傳 `[]Violation` 並寫入 structured log。

### 評估規則

| 規則 | 欄位 | 觸發條件 | Severity |
|------|------|---------|----------|
| 停損 | `Constraints.StopLossPct` | `pctChange <= -StopLossPct` | error |
| 停利 | `Constraints.TakeProfitPct` | `pctChange >= TakeProfitPct` | warn |
| 最大持有天數 | `Constraints.MaxHoldingDays` | `holdingDays >= MaxHoldingDays` | warn |

- `pctChange = (CurrentPrice - AverageCost) / AverageCost`；與 `internal/sim/engine.go` 使用相同正數幅度語意。
- `AverageCost <= 0` 時直接忽略，避免除以零。
- 違規僅產生 log，不會自動下單平倉；由下游 `internal/live` 或人工流程決定處置。

### 與 live orchestrator 的關係

`internal/live/orchestrator.go` 在 `EventMarketSnapshot` 的 critical handler 內對持有部位呼叫 `PublishPositionUpdate(..., "updated")`，使 `Trigger` 能持續追蹤停損/停利/持有天數。這是 Wave 9 將 baseline policy 從「回測約束」延伸到「執行期監控」的關鍵接點。
