# Data Architecture — Atlas-Go 資料架構

**用途**：AI 代理與開發者在編程時，需要知道自己需要什麼數據、這些數據存在哪裡、以及如何正確地獲取它們。本文檔是 **權威來源**（authoritative source），所有資料相關的實現必須與本文描述一致。

**最近更新**：2026-05-29（新增）
**相關修復**：PR #237 (校準資料來源)、PR #239 (移除 XOR 寫入模式)

---

## 核心原則

1. **雙寫（dual-write）**：Outcome 資料同時寫入 PostgreSQL 和檔案系統。不存在"二選一"（XOR）模式。
2. **Session 目錄為持久層**：每個 session 的完整記錄保存在 `data/state/sessions/session-YYYYMMDD-daily/`。
3. **全域檔案為聚合層**：`data/state/recommendation_outcomes.jsonl` 以 O_APPEND 累積所有 session。
4. **AI 代理優先讀 Session 目錄**：Session 目錄有最豐富、最完整的 outcome 數據（含 per-agent forward return）。

---

## 資料儲存層（Data Stores）

### 層級 1：Session 目錄（持久層 · 最完整）

```
data/state/sessions/
  session-20260101-daily/
    summary.json                       ← session 摘要
    recommendation_outcomes.jsonl      ← per-agent, per-symbol forward returns
    screened_symbols.jsonl             ← 被篩選器排除的股票
    positions.json                     ← 持倉狀態
    experiments.jsonl                  ← 實驗記錄
  session-20260102-daily/
    ...
  session-20260528-daily/
    ...
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次模擬結束後（`RecordSessionOutcomes`） |
| **寫入方式** | 原子寫入（temp file + rename） |
| **保留策略** | 永久保留，不刪除 |
| **如何讀取** | 遍歷目錄 → 讀取每個 session 的 `recommendation_outcomes.jsonl` |
| **典型用途** | 校準、回測、agent 績效分析 |

### 層級 2：全域 Outcome 檔案（聚合層）

```
data/state/recommendation_outcomes.jsonl
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次模擬結束後（`RecordOutcomes`，O_APPEND） |
| **保留策略** | 累積所有 session（不覆蓋） |
| **如何讀取** | `ledger.NewStore("data/state").LoadOutcomes()` |
| **典型用途** | 快速查詢最新一筆 outcome、dashboard 顯示 |

### 層級 3：PostgreSQL 資料庫（查詢層）

| 資料表 | 說明 |
|--------|------|
| `recommendation_outcomes` | 每筆推薦的結果（per-agent, per-symbol） |
| `sessions` | Session 記錄 |
| `screening_rejects` | 篩選器排除記錄 |
| `darwinian_snapshots` | Darwinian 權重快照 |

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次模擬結束後（`DualWriteRepository.RecordOutcomes`） |
| **如何讀取** | `repo.QueryOutcomesBySession()` / `repo.QueryOutcomesByAgent()` |
| **典型用途** | 跨 session 查詢、儀表板 API |

### 層級 4：Darwinian 權重（狀態層）

```
data/state/darwinian_weights.json       ← 當前權重（覆蓋寫入）
data/state/darwinian_history.jsonl      ← 歷史快照（O_APPEND）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 啟動時（Save after InitializeFromRegistry）+ 每次 session 後 |
| **如何讀取** | `PipelineService.LoadDarwinianStatus()` 從磁碟讀取 |
| **注意** | 權重檔案在啟動時才同步到磁碟。重啟後新 agent 才出現。|

---

## 資料流（Data Flow）

### 寫入路徑

```
RunDailySimulation()
  │
  ├── buildSyntheticOutcomes() → 產生 outcomes
  │
  ├── repo.RecordOutcomes()    → PostgreSQL + DualWrite JSONL (若 DB 可用)
  ├── ledger.RecordOutcomes()  → data/state/recommendation_outcomes.jsonl (O_APPEND)
  └── ledger.RecordSessionOutcomes() → data/state/sessions/<session-id>/ (原子寫入)
```

**關鍵規則**：三者同時寫入，絕不使用 XOR 模式。

### 讀取路徑

```
校準工具 (calibrateDarwinian):
  → loadOutcomesFromSessions()           ← 讀取所有 session 目錄
  → 遍歷 data/state/sessions/*/
      → 讀取 recommendation_outcomes.jsonl

Dashboard API:
  → PipelineService.LoadRecommendationPipeline()
  → 讀取 session 目錄的最新 session

Agent 績效:
  → DarwinianWeightManager.GetAllAgentWeightData()
  → 從記憶體讀取（在啟動時從 darwinian_weights.json 載入）
```

---

## AI 代理常見錯誤與解決方案

| 錯誤 | 發生原因 | 正確做法 |
|------|---------|---------|
| 資料不足（"insufficient data"） | 讀取 `data/state/recommendation_outcomes.jsonl`（只有 28 筆） | 改為讀取 session 目錄（1,841 筆） |
| 找不到 forward return | 只查了 summary.json（沒有 forward return 欄位） | 讀取 `recommendation_outcomes.jsonl`（有 `forward_return` 欄位） |
| 校準結果全是 0 或 1 | 用了合成數據（沒有足夠真實數據） | 等待系統跑更多 session 累積真實 outcome |
| Darwinian 權重不變 | 讀取了舊的 `darwinian_weights.json` | 等待啟動時的 `Save()` 或手動觸發 session |

---

## 如何新增資料消費者

1. 確認你需要的資料類型（outcome? session? agent weight?）
2. 查看上方「讀取路徑」找到對應的資料來源
3. 參考現有消費者代碼：
   - 校準工具：`cmd/calibrate-parameters/main.go` → `loadOutcomesFromSessions()`
   - Dashboard API：`internal/monitoring/service/pipeline.go`
   - Darwinian 權重：`internal/portfolio/darwinian_weights.go`
4. **不要**直接 `os.Open` —— 使用 `ledger.NewStore()` 或 repository 提供的查詢介面
5. **不要**假設資料只在一個地方 —— 優先使用最完整的來源

---

## 相關文件

- `internal/ledger/ledger.go` — 檔案型 ledger 實現
- `internal/repository/dual_write.go` — PostgreSQL + 檔案雙寫
- `internal/orchestrator/system.go` — 資料流主控
- `internal/portfolio/darwinian_weights.go` — Darwinian 權重管理
- `cmd/calibrate-parameters/main.go` — 校準工具
- `docs/PARAMETER_SYSTEM.md` — 參數系統文件
