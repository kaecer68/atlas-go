# Dashboard 前端同步計劃：Screening Layer 可視化

## 背景

後端已完成 `internal/screener/` 與 `ScreeningCriteria` 的整合，但 Dashboard（`web/static/index.html`）尚未反映這些改變。操作者無法在網頁上看到：
- 各 agent 設定了哪些篩選條件
- 哪些標的被篩選層攔截、原因是什麼
- 某檔標的「完全沒有推薦」是否因為篩選條件過嚴

## 目標

讓 Dashboard 能夠即時、透明地呈現 Screening Layer 的運作狀態，且不影響既有效能。

---

## 設計原則

1. **不更動既有 Pipeline 表格的預設行為**：投資管線頁面預設仍只顯示「通過推薦」，但提供「查看被篩選標的」開關。
2. **向後相容**：舊 session 沒有 screening log 時，前端應優雅降級（隱藏篩選相關 UI）。
3. **單一 truth source**：後端 API 直接從 session 持久化檔案讀取 screening log，不做即時重算。
4. **最小侵入性**：儘量復用現有表格樣式與 modal 組件。

---

## 任務拆分

### Wave 1：後端持久化 Screening Log

**問題**：目前 `recommendation_outcomes.jsonl` 只記錄最終通過的推薦，沒有 screening 階段的 reject 紀錄。若要在 Dashboard 顯示「被篩掉的標的」，必須在模擬執行時一併寫入。

**方案**：在 `internal/orchestrator/executors.go` 的 `collectRecommendations` 中，當 `plugins.Screen()` 返回 `false` 時，將該 `(symbol, agent_id, criteria, reason)` 寫入 session-level 的 `screened_symbols.jsonl`。

**新增檔案/修改**：
- `internal/domain/screening.go`：新增 `ScreenedSymbolRecord` struct
- `internal/orchestrator/executors.go`：在 `collectRecommendations` 的 screen reject 分支中，累積 `[]ScreenedSymbolRecord`
- `internal/ledger/store.go`（或 `internal/orchestrator/system.go`）：在 `RecordSessionOutcomes` 時，同時將 screened records 寫入 `data/ledger/sessions/{sessionID}/screened_symbols.jsonl`

**資料結構範例**：
```json
{"symbol":"LOW_VOL.TW","agent_id":"semi-desk-01","agent_name":"半導體產業桌","criteria":"volume_intraday","threshold":1000000,"actual":100000,"reason":"volume below threshold","recorded_at":"2026-04-16T09:30:00Z"}
{"symbol":"EXPENSIVE.TW","agent_id":"value-yield-01","agent_name":"價值股息","criteria":"pe","threshold":18,"actual":25,"reason":"pe above max","recorded_at":"2026-04-16T09:30:00Z"}
```

**驗證**：
- 執行一次回測後，`screened_symbols.jsonl` 應出現對應記錄
- 舊 session 缺少此檔案時，Dashboard API 應返回空陣列而不報錯

---

### Wave 2：後端 API 擴充

#### 2.1 `handleAgentObservatory` 加入 Screening Criteria

**修改檔案**：`internal/monitoring/dashboard_api.go`

在 `AgentObservatoryResponse` 中新增欄位：
```go
type AgentObservatoryResponse struct {
    // ... existing fields ...
    Agents []AgentScreeningView `json:"agents"`
}

type AgentScreeningView struct {
    AgentID           string                `json:"agent_id"`
    Name              string                `json:"name"`
    Skill             string                `json:"skill"`
    Layer             string                `json:"layer"`
    ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
    Universe          []string              `json:"universe"`
}
```

`handleAgentObservatory` 內部讀取 `configs/agents.json`，將每個 enabled agent 的 `ScreeningCriteria` 一併返回。

#### 2.2 新增 `/api/dashboard/screening-log`

**修改檔案**：`internal/monitoring/dashboard_api.go`

新增 handler：
```go
func (a *DashboardAPI) handleScreeningLog(w http.ResponseWriter, r *http.Request)
```

Query params：`session_id`（可選，預設最新 session）

返回結構：
```go
type ScreeningLogResponse struct {
    SessionID       string                  `json:"session_id"`
    TotalScreened   int                     `json:"total_screened"`
    ByAgent         []AgentScreeningSummary `json:"by_agent"`
    Items           []domain.ScreenedSymbolRecord `json:"items"`
    RecordedAt      time.Time               `json:"recorded_at"`
}

type AgentScreeningSummary struct {
    AgentID         string `json:"agent_id"`
    ScreenedCount   int    `json:"screened_count"`
    TopCriteria     string `json:"top_criteria"`
}
```

讀取路徑：`data/ledger/sessions/{sessionID}/screened_symbols.jsonl`

#### 2.3 在 `handleRecommendationPipeline` 中嵌入 screening summary

**修改檔案**：`internal/monitoring/dashboard_api.go`

在 `RecommendationPipelineResponse` 中新增：
```go
type RecommendationPipelineResponse struct {
    // ... existing fields ...
    ScreeningSummary *ScreeningSummary `json:"screening_summary,omitempty"`
}

type ScreeningSummary struct {
    ScreenedCount int `json:"screened_count"`
    ByAgent       []AgentScreeningSummary `json:"by_agent"`
}
```

這樣 Pipeline 頁面可以直接在頂端 banner 顯示「本場次共篩選 N 筆標的」而不需要額外打 API。

---

### Wave 3：前端 UI 更新

#### 3.1 Agent Observatory 頁面新增「篩選條件」欄位

**修改檔案**：`web/static/index.html`

在 renderAgentObservatory 的表格中，新增一列「篩選條件」：
- 若 `screening_criteria.has_filters` 為 false，顯示 `-`
- 若有條件，顯示為緊湊 badge 列表：
  - `P/E ≤ 18`
  - `成交量 ≥ 100萬`
  - `動能 ≥ 0`
  - `股息率 ≥ 2%`
- badge 使用 `.badge.info` 樣式
- 滑鼠 hover 時顯示 tooltip：「該 agent 在生成推薦前會先用這些條件過濾標的池」

#### 3.2 Pipeline 頁面新增「顯示被篩選標的」開關

**修改檔案**：`web/static/index.html`

在 `#pipelineTable` 上方控制列，新增 checkbox：
```html
<input type="checkbox" id="pipelineShowScreened" onchange="togglePipelineShowScreened(this)">
<label for="pipelineShowScreened">顯示被篩選標的</label>
```

行為：
- 預設 **不勾選**，表格只顯示現有的 `finalOutputs`（既有行為）
- 勾選後，表格下方追加一個區塊（或同一表格的新 section）標題為「被篩選標的」，列出 `screening_summary.items`
- 每列顯示：`標的 | 公司名 | Agent | 觸發條件 | 實際值 | 門檻 | 原因`
- 列的文字顏色使用 `var(--muted)`，與通過推薦區隔

#### 3.3 Pipeline 頁面頂端 Banner 新增篩選統計

在現有的「原始輸入 N → 最終輸出 M」workflow 步驟之間，插入「篩選層」步驟：

```text
原始輸入 23 → 篩選層 攔截 8 → 最終輸出 15 → 風控長 → 投資長
```

若 `screening_summary` 不存在（舊 session），則隱藏「篩選層」步驟，維持既有外觀。

#### 3.4 總覽頁 KPI 卡片微調（可選）

可在「擁擠標的」卡片旁新增「篩選紀錄」快速入口 KPI 卡片：
- 標題：篩選紀錄
- 數值：顯示最新 session 的 `screened_count` 筆
- 點擊後直接切換到 Pipeline 頁面並自動勾選「顯示被篩選標的」

或者，不新增卡片，僅在「投資管線」KPI 區域增加一條 hint：「今日有 N 檔標的被篩選條件攔截」——較簡潔。

**建議**：先做「不新增卡片」的輕量版本，降低視覺負擔。

#### 3.5 控制與稽核頁面（可選）

在「控制與稽核」頁面的 agent 暫停/恢復列表旁，為每個 agent 顯示其 screening criteria 小字摘要，幫助操作者理解「為何暫停某 agent 可能影響篩選邏輯」。

**建議**：此項列為 Nice-to-have，優先完成 3.1~3.3。

---

## 資料流總結

```
Simulation Run
  → collectRecommendations (screening happens)
  → write screened_symbols.jsonl alongside recommendation_outcomes.jsonl
  → Dashboard API reads both files
  → index.html renders pipeline + screened log
```

---

## 驗收標準

- [ ] 執行回測後，`data/ledger/sessions/{sessionID}/screened_symbols.jsonl` 存在且格式正確
- [ ] `/api/dashboard/agent-observatory` 返回每個 agent 的 `screening_criteria`
- [ ] `/api/dashboard/screening-log` 能正確讀取並返回按 agent 分組的篩選紀錄
- [ ] Pipeline 頁面預設不變，勾選「顯示被篩選標的」後能看到被拒絕的標的及原因
- [ ] 舊 session（無 screening log）載入時，前端無錯誤，篩選相關 UI 自動隱藏
- [ ] `go test ./...`、`go vet ./...`、`staticcheck ./...` 全綠
- [ ] 總覆蓋率維持 ≥ 40%

---

## 時程估算

| Wave | 預估時間 |
|---|---|
| Wave 1（持久化 log）| 2-3 小時 |
| Wave 2（API 擴充）| 2-3 小時 |
| Wave 3（前端 UI）| 3-4 小時 |
| **Total** | **7-10 小時** |

---

## 風險與注意事項

1. **Disk I/O 增量**：每場次多寫一個 `screened_symbols.jsonl`，但內容僅為 reject 紀錄，通常數百行以內，影響極小。
2. **隱私/敏感性**：篩選紀錄僅含公開市場數據（股價、成交量、PE），無個資疑慮。
3. **向後相容**：舊 session 缺少 log 檔案時，API 與前端都必須優雅處理，不可報錯或空白。

---

## 下一步

待使用者確認本計劃後，即可按 Wave 1 → Wave 2 → Wave 3 的順序執行。
