# Warmup → Auto-Calibration → Auto-Evolution 修復進度

**分支**: `feat/warmup-auto-evolution`  
**最後更新**: 2026-06-01  
**WIP Commit**: `dd2052b` — "context overflow checkpoint - refactor in progress"

---

## 一、已完成項目 (Done)

### P0 — 統計閾值調整與 Maturity 基礎設施

| 項目 | 文件 | 狀態 |
|------|------|------|
| `SystemMaturity` 類型定義 + `MaturityTracker`（含持久化） | `internal/domain/maturity.go` | ✅ |
| `MaturityTracker` 單元測試 | `internal/domain/maturity_test.go` | ✅ 通過 |
| Sharpe 最小樣本：5 → 60 | `internal/config/parameters_defaults.go` | ✅ |
| VaR/CVaR/ComponentVaR 最小樣本：0/2 → 252 | `internal/risk/var_calculator.go` | ✅ |
| Welch t-test 最小樣本：2 → 63 | `internal/experiment/judge.go` | ✅ |
| Volatility 最小樣本：2 → 30 | `internal/experiment/judge.go` | ✅ |
| Bayesian Optimizer GP fit：2 → 10 | `internal/config/bayesian_optimizer.go` | ✅ |
| Health 最小樣本：10 → 30 | `internal/config/parameters_defaults.go` | ✅ |
| Correlation 最小樣本：15 → 30 | `internal/industry/linkage.go` | ✅ |
| Beta 最小樣本：2 → 60 | `internal/reporting/performance.go` | ✅ |
| Engine Sharpe 最小樣本：2 → 60 | `internal/sim/engine.go` | ✅ |

### P0 — Maturity Gating（各模組接入 Burn-in 閘門）

| 模組 | 文件 | Burn-in 行為 | 狀態 |
|------|------|-------------|------|
| Darwinian Weight Manager | `internal/portfolio/darwinian_weights.go` | 雙倍 cooldown、禁用 performance bonus、multiplier 開根號 (~50% 幅度) | ✅ |
| Pre-Trade Risk Gate (VaR) | `internal/risk/pre_trade.go` | 通過但記錄 warming 日誌，使用靜態閾值 | ✅ |
| Experiment Judge | `internal/experiment/judge.go` | `passesAcceptance` 一律拒絕 | ✅ |
| Calibration Engine | `internal/orchestrator/calibration_engine.go` | `CalibrateAll` 直接返回 nil | ✅ |

### P1 — 自動校準排程器

| 項目 | 文件 | 狀態 |
|------|------|------|
| `BackgroundCalibrationScheduler` | `internal/scheduler/auto_calibration.go` | ✅ |
| 單元測試 | `internal/scheduler/auto_calibration_test.go` | ✅ 通過 |

### P2 — 自動實驗提案器

| 項目 | 文件 | 狀態 |
|------|------|------|
| `AutoProposer` + 5 種觸發條件 | `internal/experiment/auto_proposer.go` | ✅ |
| 單元測試 | `internal/experiment/auto_proposer_test.go` | ✅ 通過 |

### P3 — 自動判斷/推廣

| 項目 | 文件 | 狀態 |
|------|------|------|
| `AutoJudgePromoter` + 冷卻/觀測門檻 | `internal/experiment/auto_judge_promoter.go` | ✅ |
| 單元測試 | `internal/experiment/auto_judge_promoter_test.go` | ✅ 通過 |

### P4 — 系統健康監控

| 項目 | 文件 | 狀態 |
|------|------|------|
| `SystemHealthMonitor` + 3 種檢查 | `internal/scheduler/system_health.go` | ✅ |

### P5 — 設計文檔

| 項目 | 文件 | 狀態 |
|------|------|------|
| 完整設計文檔 | `docs/warmup_auto_evolution_design.md` | ✅ |

---

## 二、待修復項目 (Fix Required)

### 🔴 高優先：測試失敗

#### `internal/scheduler/auto_rollback_test.go` — 3 個測試失敗

**失敗清單：**

| 測試 | 原因 | 修復方向 |
|------|------|----------|
| `TestAutoRollback_AgentDisable` | 測試直接設置 `agentFailCount=30`，但 `checkAgentFailures()` 會遍歷 agent 並檢查 `RollingSharpe < -1.0`。新 agent Sharpe=0，進入 else 分支將 count 重置為 0。 | 在測試中先通過 `dw.RecordOutcome()` 注入足夠多（≥60 個）負向回報，使 `RollingSharpe` 降到 -1.0 以下，再調用 `RunDaily()`。 |
| `TestAutoRollback_PromotionDegradation` | `pre_promotion_sharpe=0`（60 個 +2% 回報計算出的 Sharpe 仍為 0），導致後續即使性能惡化也無法觸發 20% 降幅閾值。 | 增加 seed 樣本數量（>60）或使用更極端的回報值（如 +0.05 / -0.05），確保 preSharpe > 0 且 degraded Sharpe < preSharpe * 0.8。 |
| `TestAutoRollback_History` | 與 AgentDisable 相同原因：`agentFailCount` 被重置，沒有觸發 disable，因此沒有 history 記錄。 | 同 AgentDisable 修復方案。 |

**參考：**
- `RecordOutcome(agentID, forwardReturn, isHit)` 在 `internal/portfolio/darwinian_weights.go:265`
- Sharpe 計算需要 ≥60 個樣本（`SharpeMinSampleSize=60`）
- RollingSharpe 更新邏輯在 `UpdateRollingSharpe()` 中

---

### 🟡 中優先：功能 Stub / TODO

#### 1. `AutoRollback.executeRollback()` — 2 個 action 未實現

位於 `internal/scheduler/auto_rollback.go:225-258`

```go
// TODO: baseline revert and calibration revert require backup infrastructure.
```

| Action | 狀態 | 依賴 |
|--------|------|------|
| `disable_agent` | ✅ 已實現（調用 `dwManager.RemoveAgent`） | 無 |
| `revert_baseline` | ❌ Stub — 返回 error | `baseline.Manager.Revert(snapshotPath)` |
| `revert_calibration` | ❌ Stub — 返回 error | `ParametersConfig.SaveWithRollback()` |

**修復路徑：**
- 在 `baseline.Manager` 中增加 `Revert(snapshotPath string) error` 方法（需要備份基線的機制）
- 在 `config.ParametersConfig` 或相關結構中增加參數快照/回滾機制
- 或者：暫時將這兩個 action 標記為 `ActionTypeAlertOnly`，只發警報不執行回滾，等基礎設施到位

#### 2. `AutoRollback.computeSystemSharpe()` / `computeSystemCompositeScore()` — 過度簡化

目前直接用所有 agent RollingSharpe 的平均值作為系統指標。設計文檔中提到的「system-wide composite score」可能需要更精確的定義（如加權平均、考慮資產配置等）。

**建議：** 與設計者確認 `compositeScore` 的精確公式，或暫時保持簡化實現並在文檔中註明。

---

### 🟡 中優先：設計文檔中規劃但未實現的組件

#### 3. `internal/orchestrator/maturity_plugin.go` — 缺失

設計文檔 §9.1 提到需要一個 orchestrator plugin 來將 maturity 狀態傳遞給各個 executor。目前 maturity gating 是直接在每個模組內部實現的，沒有統一的 plugin 架構。

**評估：** 當前每個模組直接 `WithMaturityTracker()` 的實現方式已經可用，plugin 是架構優化而非阻塞項。可以標記為 **P4（可選）**。

#### 4. API Endpoints — 缺失

設計文檔 §8.2 規劃的 endpoints：

| Endpoint | 用途 | 狀態 |
|----------|------|------|
| `GET /api/system/maturity` | 查詢 maturity 狀態 | ❌ 未實現 |
| `GET /api/system/calibration/status` | 校準任務運行狀態 | ❌ 未實現 |
| `GET /api/experiments/auto-proposals` | 待審批的自動提案 | ❌ 未實現 |
| `POST /api/experiments/auto-proposals/{id}/approve` | CALIBRATING 階段人工審批 | ❌ 未實現 |

**評估：** dashboard API 屬於 P4 滾出階段，非當前阻塞項。

#### 5. Event Bus 整合 — 缺失

設計文檔 §8.3 規劃的事件：

| 事件 | 狀態 |
|------|------|
| `MaturityChanged` | ❌ 未發布（`MaturityTracker.Refresh()` 有 callback 但未接入 event bus） |
| `AutoCalibrationCompleted` | ❌ 未發布 |
| `AutoExperimentProposed` | ❌ 未發布 |
| `AutoPromoted` / `AutoReverted` | ❌ 未發布 |

**修復路徑：** 為 `AutoJudgePromoter`、`AutoProposer`、`BackgroundCalibrationScheduler` 增加 `WithEventBus()` 方法，在關鍵節點發布事件。

---

### 🟢 低優先：組件連接（Wiring）

#### 6. `cmd/atlas/main.go` — 未接入 MaturityTracker

目前 main.go 沒有：
- 創建 `MaturityTracker`
- 將 tracker 注入到 `DarwinianWeightManager`、`PreTradeGate`、`Judge`、`CalibrationEngine` 等組件
- 註冊 background tasks（calibration scheduler、auto judge、auto rollback、health monitor）

**修復方向：** 在 bootstrap 流程中：
```go
// 1. 創建 tracker
maturityTracker, _ := domain.NewMaturityTracker("data/state/maturity_tracker.json")

// 2. 注入到各組件
dwManager.WithMaturityTracker(maturityTracker)
preTradeGate.WithMaturityTracker(maturityTracker)
judge.WithMaturityTracker(maturityTracker)
calibrationEngine.WithMaturityTracker(maturityTracker)

// 3. 註冊 background tasks
bgManager.Register(autoCalibrationTask)
bgManager.Register(autoJudgeTask)
bgManager.Register(autoRollbackTask)
bgManager.Register(healthMonitorTask)
```

#### 7. Backfill for Existing Deployments — 未實現

設計文檔 §11 Q3 決策：若系統已有歷史記錄，應根據 ledger 最早記錄決定初始 maturity phase，而非一律從 BURN_IN 開始。

**修復方向：** 在 `NewMaturityTracker` 中增加可選邏輯：若 `statePath` 不存在，查詢 ledger 最早交易日，計算 `days_earliest_to_now`，若 ≥60/≥252 則直接設為 CALIBRATING/FULL_AUTO。

---

## 三、下一步修復建議（優先序）

### 立即執行（讓測試全綠）

1. **修復 `auto_rollback_test.go`**
   - `TestAutoRollback_AgentDisable` / `TestAutoRollback_History`：使用 `RecordOutcome` 注入負向回報使 Sharpe < -1.0
   - `TestAutoRollback_PromotionDegradation`：增加樣本數並使用更極端的回報值確保 pre/post Sharpe 有顯著差異

2. **確認 `go test ./...` 全綠**
   - 當前編譯已通過，僅 scheduler 包有 3 個測試失敗

### 本 PR 內完成（功能閉環）

3. **暫時處理 `revert_baseline` / `revert_calibration` Stub**
   - 選項 A：實現 baseline/parameter 的備份與回滾基礎設施
   - 選項 B：將這兩個 action 改為僅發警報（不執行回滾），避免測試/運行時產生錯誤

4. **在 `cmd/atlas/main.go` 或 bootstrap 中接入 MaturityTracker**
   - 至少完成 tracker 創建和注入到核心組件，使功能真正可用

### 後續 PR（P4 / 可選）

5. Dashboard API Endpoints
6. Event Bus 整合
7. `maturity_plugin.go` 統一架構
8. Backfill for existing deployments

---

## 四、變更文件總覽

### 新增文件（12 個）

```
docs/warmup_auto_evolution_design.md
internal/domain/maturity.go
internal/domain/maturity_test.go
internal/experiment/auto_judge_promoter.go
internal/experiment/auto_judge_promoter_test.go
internal/experiment/auto_proposer.go
internal/experiment/auto_proposer_test.go
internal/scheduler/auto_calibration.go
internal/scheduler/auto_calibration_test.go
internal/scheduler/auto_rollback.go
internal/scheduler/auto_rollback_test.go
internal/scheduler/system_health.go
```

### 修改文件（17 個）

```
internal/config/bayesian_optimizer.go          # observations < 2 → < 10
internal/config/bayesian_optimizer_test.go      # 測試同步調整
internal/config/parameters_defaults.go          # 多個閾值調整
internal/experiment/judge.go                    # burn-in gate + t-test/vol 閾值
internal/experiment/judge_helpers_test.go       # 測試同步調整
internal/experiment/judge_test.go               # 測試同步調整
internal/experiment/sharpe_stability.go         # 閾值調整
internal/experiment/sharpe_stability_test.go    # 測試同步調整
internal/industry/linkage.go                    # correlation n < 15 → 30
internal/orchestrator/calibration_engine.go     # burn-in gate
internal/portfolio/darwinian_weights.go         # maturity gating + conservative mode
internal/portfolio/darwinian_weights_test.go    # 測試同步調整
internal/reporting/performance.go               # sharpe/beta 閾值
internal/risk/pre_trade.go                      # VaR warming gate
internal/risk/var_calculator.go                 # VaR/CVaR 閾值
internal/risk/var_calculator_test.go            # 測試同步調整
internal/sim/engine.go                          # sharpe 閾值
```

---

## 五、快速驗證命令

```bash
# 編譯
go build ./...

# 核心新模組測試
go test -v ./internal/domain/...
go test -v ./internal/experiment/...
go test -v ./internal/scheduler/...        # 當前有 3 個失敗

# 全量測試
go test ./...

# CI 檢查
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...
```
