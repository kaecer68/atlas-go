# PRISM Multi-Cohort Training 規格

> **文件角色**：atlas-go PRISM (Parallel Regime-Specific Independent Systems for Multi-cohort training) 多 cohort 訓練規格。
> **取代對象**：原 internal/prism/AGENTS.md（已遷移至此）。

PRISM 管理 5 個獨立 regime 訓練佇列，為 JANUS meta-layer 提供 cohort 績效數據。

---

## 核心職責

- **5-Regime 佇列管理**：Risk-On / Risk-Off / High-Vol / Low-Vol / Transition，各有獨立 TrainingQueue
- **排程與優先權**：`ScheduleTraining()` 依 agent 層級、Darwinian 權重計算優先級
- **訓練執行**：透過 `TrainingExecutor` 介面委派實際回測；未附加 executor 時，結果會標記 `Synthetic: true`
- **結果緩衝**：`GetCompletedResults()` 提供 JANUS 消費的訓練結果視窗

---

## 資料流

```
Orchestrator (regime detection) → prismPlugin.PostSimulation()
  → ScheduleTraining(agent, TrainingWindow{Regime, RegimeSet:true})
    → classifyRegime() — 尊重 RegimeSet flag，無覆寫時預設 Transition
      → TrainingQueue.Enqueue() → worker() → executeTraining()
        → TrainingExecutor.Run() (real backtest) 或 Synthetic result
          → recordCompletedResult() → JANUS 消費
```

---

## 關鍵設計決策

### classifyRegime 不使用時間推測

`classifyRegime()` 僅尊重 TrainingWindow 的 `RegimeSet` flag：regime 由 orchestrator 顯式設定時具有權威性，未設定時預設為 `RegimeTransition`。不採用 `time.Now().Month()` 等日曆/季節性啟發。

### 無 executor 時標記 Synthetic

當 `TrainingExecutor` 未附加時，`executeTraining()` 返回 `TrainingResult{Synthetic: true, Error: "no training executor configured"}`，而非靜默返回假資料。消費者（JANUS、worker loop）可透過 `Synthetic` flag 區分真實結果與佔位結果。

---

## 參數

PRISM 參數透過 `internal/config/parameters.go` 管理：

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `prism_boost_multiplier` | 1.0 | 新 agent 優先級加成倍率 |
| `prism_boost_min` | 0.0 | 最低優先級 |
| `prism_boost_max` | 100 | 最高優先級 |

---

## 測試覆蓋

| 類別 | 狀態 |
|------|------|
| Queue CRUD (Enqueue/Dequeue/Clear/Len) | ✅ 完整 |
| Manager lifecycle (Start/Stop/ScheduleTraining) | ✅ 完整 |
| classifyRegime (explicit override + default) | ✅ |
| executeTraining (executor/synthetic/error) | ✅ |
| CompletedResults buffer | ✅ |
| WithExecutor attachment | ✅ |
| 並行安全性 (worker pool) | ⚠️ 間接覆蓋（Start/Stop） |

---

## 陷阱

- **RegimeSet flag**：TrainingWindow 的 `Regime` 欄位只有在 `RegimeSet: true` 時才會被 `classifyRegime` 採用。忘記設定會導致任務路由至 Transition queue
- **Synthetic 結果**：未附加 executor 的 PRISMManager 會產生 Synthetic 結果。JANUS 消費者應檢查此 flag
- **autoBalancer ticker**：PRISM 內部使用獨立的 `time.Ticker`（非 BackgroundTaskManager），因其為內部維護循環，非外部排程任務
