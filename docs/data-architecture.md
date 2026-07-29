# Data Architecture — Atlas-Go 資料架構

**用途**：AI 與開發者的資料使用指南——需要什麼數據、存在哪裡、如何正確獲取。

**權威來源**:本文檔描述原則與流程,具體資產細節見 [`docs/data-catalog.md`](data-catalog.md)。

---

## L2.3 工具呼叫追蹤與 conviction 流程 (Wave 11, v0.0.0.21)

When `UseLLMSectorAgents=true`, the LLM-driven sector agent produces a structured data flow:

```
LLM PlanComplete (driver)
  ↓ []PlanStep
SectorAgentLLM.PlanStep → AgentLoop.AdvancePlan (Round += len(steps))
  ↓ for each tool step:
SectorAgentLLM.AdvanceToolCall (error if Phase != Plan)
  → SectorAgentLLM.RunToolCall (PR1 placeholder; "not yet implemented" — pending follow-up)
  ↓ tool results concatenated
SectorAgentLLM.AdvanceReflect (error if Phase != ToolCall)
  → SectorAgentLLM.Reflect → LLM ReflectComplete
  ↓ Reflection{Continue, FinalConviction, Reasoning}
  if Continue=true: loop back to PlanComplete (capped at MaxIter)
  if Continue=false: AdvanceFinal(FinalConviction) → PhaseFinal
  ↓
domain.Recommendation{Symbol, Agent, Conviction, Reason, ...}
  → orchestrator → downstream
```

**Data invariants**:
- `AgentLoop.Round` accumulates `len(PlanStep)` per `AdvancePlan` call (NOT +1 per call).
- `AgentLoop.Exhausted()` returns true when `Round >= MaxIter`. A one-time `slog.Warn` fires if the legacy `len(Steps) >= MaxIter` threshold is detected (divergence guard).
- `AgentLoop.AdvanceFinal(c)` clamps `c` to `[0,100]` with `slog.Warn` on clamp.
- `domain.Recommendation.Conviction` = `Reflection.FinalConviction` (LLM's final value).
- `domain.Recommendation.Reason` = `Reflection.Reasoning`.

**Tool call traces** (L2.4):
- Per-recommendation metrics logged: `agent_loop.round`, `agent_loop.plan_count`, `agent_loop.reflect_count`, `agent_loop.tool_count`, `llm.latency_ms.{plan,reflect}`, `reflect.continue`, `recommendation.{symbol,conviction}`.
- See [`docs/specs/l2-4-observation-spec.md`](specs/l2-4-observation-spec.md) §Metrics for the full metrics list（L2.4 observation schema，PR #821 / PR #824 永久化）。

---

## 核心原則

1. **雙寫（dual-write）**：Outcome 同時寫入 PostgreSQL 和檔案系統，不存在 XOR 模式
2. **Session 目錄為持久層**：`data/state/sessions/session-YYYYMMDD-daily/` 保存完整記錄
3. **全域檔案為聚合層**：`data/state/recommendation_outcomes.jsonl` 以 O_APPEND 累積
4. **AI 優先讀 Session 目錄**：最豐富的 outcome 數據（含 per-agent forward return）
5. **治理規範**：遵循 `data-naming-convention.md`、`data-directory-standard.md`、`data-maturity-standard.md`

---

## TAIEX 與 `historical_volatility` 韌性與失敗記錄語義

- **`taiex_index` fallback 鏈路**：
  - 主源為 Yahoo Finance `^TWII`；當 Yahoo 失敗（network、HTTP 非 2xx、空 chart result、無有效 TAIEX 列）時，自動 fallback 至 **TWSE OpenAPI** `https://www.twse.com.tw/exchangeReport/MI_INDEX?response=json&date=YYYYMMDD&type=IND`。
  - TWSE 回應的 `title` 日期必須與請求日期一致；週末/休市/前一日資料會被拒絕，避免寫入非當日官方值。
  - 若主源與備援皆失敗，`taiex` 鍵為零值，且 `taiex_index` 會出現在 `MacroDataSnapshot.FailedChannels` 中。

- **`historical_volatility` 陳舊快取誠實化**：
  - 該欄位由 `TaiwanVolatilityProvider` 從 `^TWII` 20 個交易日收盤價計算，並與 `taiex_index` 共用 `twiiCache`（60s TTL）。
  - 當 cache hit 時，provider 會檢查快取資料時間戳是否為**當前交易日**；若為前一交易日或更舊，回傳 error，拒絕以陳舊收盤價產出波動率。
  - 此設計確保不會再出現「snapshot 有 `historical_volatility` 但底層是昨日 TAIEX」的靜默錯誤。

- **snapshot 層級狀態欄位**：
  - `DataStatus`：`ok` / `degraded` / `stale`。
  - `FailedChannels`：hard-fail 的 channel ID 列表（如 `taiex_index`、`tw_vol`）。
  - `StaleChannels`：僅取得 stale/cached 資料的 channel ID 列表（circuit-breaker open 或 fallback 標記 stale）。
  - 這些欄位由 `macroDataGatewayAdapter.fetchFresh` 在合併所有 channel 後統一設定，並寫入 `data/state/macro/YYYY-MM-DD.json`（見 `macro_ingest` 排程）。

---

## 資料儲存層索引

| 層級 | 路徑 | 類型 | 成熟度 | 詳細說明 |
|------|------|------|--------|----------|
| 1 | `data/state/sessions/` | Session 持久層 | S | 每次模擬完整記錄，原子寫入 |
| 2 | `data/state/recommendation_outcomes.jsonl` | 全域聚合 | S | O_APPEND 累積，快速查詢 |
| 3 | PostgreSQL | 查詢層 | S | TimescaleDB hypertable，90 天 retention |
| 4 | `data/state/darwinian_weights.json` | 狀態層 | S | 權重範圍 [0.3, 2.5]，超界靜默正規化 |
| 5 | `data/state/macro/` | 總經數據 | E | 原油/BDI/DXY/台幣/加權/VIX。回補記錄見 `backfill_log.jsonl`（每行一筆，欄位：`date` / `field` / `value` / `change_pct` / `source_url` / `source_fetched_at` / `backfilled_at` / `baseline_date` / `baseline_value`）。`latest.json` / `previous.json` 不參與回補。 |
| 6 | `data/state/margin/` | 融資融券 | E | TWSE API，市場情緒維度 |
| 7 | `data/state/capital_flow/` | 資金流向 | E | 主力/法人流向，產業輪動 |
| 8 | `data/state/experiments*` | 實驗記錄 | S/E/U | experiments.jsonl + 實驗目錄 + 突變提案 |
| 9 | `data/state/human_interventions*` | 人工干預 | S | 稽核軌跡，放行/否決/補追 |
| 10 | `data/state/swarm*` | Swarm 狀態（模擬已降級） | E | 歷史 snapshot 與訓練記錄（不再產生新資料，PR #963） |
| 11 | `data/state/traces/`, `strategy_techniques/` | 追蹤與心法 | U/S | 決策鏈追蹤 + 投資心法狀態 |
| 12 | `data/state/windows/` | 回測視窗 | X | Window.Run() 輸出 |
| 13 | `data/state/ml_models/` | ML 模型 | X | 序列化權重檔案 |
| 14 | `data/state/*_state.json`, `metrics*` | 系統狀態 | U/E | 模擬/元學習/指標/告警狀態 |
| 15 | `data/state/{finmind,fubon,fugle}/` | API 快取 | U | 混合提供者回退鏈快取 |
| 16 | `data/sector_data/`, `fundamentals.json` | 參考數據 | S | 產業分類、基本面、測試數據 |
| 17 | `data/state/{geopolitical,tsmc_revenue,export,parameter-snapshots,live/state}/` | 特殊數據 | X/U/E | 地緣政治、台積電營收、參數快照、即時狀態 |
| 18 | `data/state/atlas.db` | SQLite（⚠️ 待處理） | X | 6 tables，與 PostgreSQL 重疊，見 P2.1 |
| 19 | `data/state-archive/` | 歸檔（⚠️ 空） | U | 所有子目錄為空，見 P2.2 |

> **每層詳細欄位、生產者、消費者、Schema** → 見 `docs/data-catalog.md`

---

## 資料流

### 寫入路徑

```
RunDailySimulation()
  ├── marketdata.GetQuotes()  → TWSE → FinMind → Fubon → Fugle
  ├── screener.ApplyFilters() → screening_rejects
  ├── orchestrator.Execute()  → recommendations
  ├── sim.Run()               → simulation_state.json
  ├── repo.RecordOutcomes()   → PostgreSQL (dual-write)
  ├── ledger.RecordOutcomes() → recommendation_outcomes.jsonl (O_APPEND)
  └── ledger.RecordSessionOutcomes() → sessions/<session-id>/ (原子寫入)
```

**每日排程**（BackgroundTaskManager）：
- 總經：`globalmarket.FetchDailyMacro()` → `macro/YYYY-MM-DD.json`
- 融資：`marketdata.FetchMargin()` → `margin/YYYYMMDD_margin.json`
- 流向：`twse_capital_flow_provider.Fetch()` → `capital_flow/YYYYMMDD_capital_flow.json`
- 健康：`apigateway.HealthCheck()` → `channel_health.json`

**實驗生命週期**：
`evolution.BuildMutationBrief()` → `experiment.Executor.Run()` → `experiment.Judge.Evaluate()` → `baseline.Promote()/Revert()`

### 讀取路徑

| 用途 | 讀取來源 | 方法 |
|------|---------|------|
| 校準 | `sessions/*/` | `loadOutcomesFromSessions()` |
| Dashboard | 最新 session | `PipelineService.LoadRecommendationPipeline()` |
| Agent 績效 | 記憶體（啟動載入） | `DarwinianWeightManager.GetAllAgentWeightData()` |
| 產業分析 | `macro/`, `margin/`, `capital_flow/` | `SeasonalEngine.GetAdjustmentBreakdown()` |
| 回測 | `replay/` + `sessions/` | `backtest.Window.Run()` |

---

## AI 代理常見錯誤

| 錯誤 | 原因 | 正確做法 |
|------|------|---------|
| 資料不足 | 讀取全域 `recommendation_outcomes.jsonl`（可能不完整） | 改讀 session 目錄的 `recommendation_outcomes.jsonl` |
| 找不到 forward return | 只查 `summary.json`（無此欄位） | 讀取 session 目錄的 `recommendation_outcomes.jsonl` |
| 校準結果全 0/1 | 使用合成數據 | 檢查 `is_synthetic` 欄位，累積真實 outcome |
| Darwinian 權重不變 | 讀取舊檔案 | 等待啟動 `Save()` 或觸發 session |
| 找不到總經數據 | 用錯日期格式 | 使用 `YYYY-MM-DD.json` 讀取 `data/state/macro/` |
| 混淆 JSONL vs CSV | 用 CSV parser 讀 JSONL | JSONL = 每行獨立 JSON |
| 資料路徑寫死 | 硬編碼 `data/state/` | 使用 `internal/config/config.go` 配置變數 |

---

## 如何新增資料消費者

1. 確認需要的資料類型 → 查 [`data-catalog.md`](data-catalog.md)
2. 讀取 `_metadata.json`（若有）了解成熟度限制
3. **不要**直接 `os.Open` — 使用 `ledger.NewStore()` 或 repository 介面
4. **不要**假設單一來源 — 優先順序：Session 目錄 > 全域 JSONL > PostgreSQL

## 如何新增資料生產者

1. 分類：replay / cache / reference / state？
2. `state/` 下建立子目錄（遵循 `data-directory-standard.md`）
3. 標準命名（遵循 `data-naming-convention.md`）
4. 建立 `_metadata.json`（遵循 `data-maturity-standard.md`）
5. JSON/JSONL 建立 Schema（遵循 `json-schema-standard.md`）
6. 更新 [`data-catalog.md`](data-catalog.md)（CI 檢查新鮮度）
7. 定時寫入透過 `BackgroundTaskManager`（`constitution.md` 第四條）
8. 外部資料透過已註冊 `marketdata.Provider`（`constitution.md` 第一條）

---

## 相關文件

- [`data-catalog.md`](data-catalog.md) — 完整資產目錄（39 個資產，含生產者/消費者/Schema）
- [`data-naming-convention.md`](data-naming-convention.md) — 命名規範
- [`data-directory-standard.md`](data-directory-standard.md) — 目錄結構
- [`data-maturity-standard.md`](data-maturity-standard.md) — 成熟度標記
- [`json-schema-standard.md`](json-schema-standard.md) — Schema 標準
- `internal/apigateway/CONSTITUTION.md` — 通道管理、背景任務、參數管理憲法
- `internal/ledger/ledger.go` — 檔案型 ledger
- `internal/repository/dual_write.go` — PostgreSQL + 檔案雙寫
- `cmd/calibrate-parameters/main.go` — 校準工具
