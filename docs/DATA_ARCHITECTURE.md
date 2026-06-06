# Data Architecture — Atlas-Go 資料架構

**用途**：AI 代理與開發者在編程時，需要知道自己需要什麼數據、這些數據存在哪裡、以及如何正確地獲取它們。本文檔是 **權威來源**（authoritative source），所有資料相關的實現必須與本文描述一致。

**最近更新**：2026-06-02（擴展至 39 個資料資產，原版僅覆蓋 ~13%）  
**相關修復**：PR #237 (校準資料來源)、PR #239 (移除 XOR 寫入模式)  
**資料治理**：`docs/DATA_NAMING_CONVENTION.md` · `docs/DATA_DIRECTORY_STANDARD.md` · `docs/DATA_CATALOG.md`

---

## 核心原則

1. **雙寫（dual-write）**：Outcome 資料同時寫入 PostgreSQL 和檔案系統。不存在"二選一"（XOR）模式。
2. **Session 目錄為持久層**：每個 session 的完整記錄保存在 `data/state/sessions/session-YYYYMMDD-daily/`。
3. **全域檔案為聚合層**：`data/state/recommendation_outcomes.jsonl` 以 O_APPEND 累積所有 session。
4. **AI 代理優先讀 Session 目錄**：Session 目錄有最豐富、最完整的 outcome 數據（含 per-agent forward return）。
5. **資料治理規範**：所有資料資產遵循 `docs/DATA_NAMING_CONVENTION.md`（命名）、`docs/DATA_DIRECTORY_STANDARD.md`（目錄結構）、`docs/DATA_MATURITY_STANDARD.md`（成熟度標記）。

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
| **成熟度** | S (stable) |

### 層級 2：全域 Outcome 檔案（聚合層）

```
data/state/recommendation_outcomes.jsonl
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次模擬結束後（`RecordOutcomes`，O_APPEND） |
| **寫入方式** | `os.OpenFile(..., O_APPEND|O_CREATE|O_WRONLY, ...)` |
| **保留策略** | 累積所有 session（不覆蓋） |
| **如何讀取** | `ledger.NewStore("data/state").LoadOutcomes()` |
| **典型用途** | 快速查詢最新一筆 outcome、dashboard 顯示 |
| **Schema** | `schemas/recommendation_outcomes.schema.json` |
| **⚠️ 已知問題** | 全域檔案 ~14.7x 大於 per-session 組合（P0.1 C.3），可能有重複資料 |
| **成熟度** | S (stable) |

### 層級 3：PostgreSQL 資料庫（查詢層）

| 資料表 | 說明 | 型態 |
|--------|------|------|
| `recommendation_outcomes` | 每筆推薦的結果（per-agent, per-symbol） | TimescaleDB hypertable |
| `metrics` | 系統運行指標 | TimescaleDB hypertable |
| `capital_flow` | 資金流向數據 | TimescaleDB hypertable |
| `export_statistics` | 匯出統計 | TimescaleDB hypertable |
| `alerts` | 系統告警 | 標準表 |
| `sessions` / `session_summaries` | Session 摘要 | 標準表 |
| `screening_rejects` | 篩選器排除記錄 | 標準表 |
| `human_interventions` | 人工干預記錄 | 標準表 |
| `users` / `workspaces` | 使用者與工作區 | 標準表 |

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次模擬結束後（`DualWriteRepository.RecordOutcomes`） |
| **寫入方式** | PostgreSQL INSERT（與檔案雙寫） |
| **壓縮策略** | TimescaleDB compression（7 天後觸發） |
| **保留策略** | 90 天 retention policy |
| **如何讀取** | `repo.QueryOutcomesBySession()` / `repo.QueryOutcomesByAgent()` |
| **Migration** | `internal/db/migrations/`（5 組 up/down SQL） |
| **成熟度** | S (stable) |

### 層級 4：Darwinian 權重（狀態層）

```
data/state/darwinian_weights.json       ← 當前權重（覆蓋寫入）
data/state/darwinian_history.jsonl      ← 歷史快照（O_APPEND）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 啟動時（`Save` after `InitializeFromRegistry`）+ 每次 session 後 |
| **如何讀取** | `PipelineService.LoadDarwinianStatus()` 從磁碟讀取 |
| **權重範圍** | [0.3, 2.5]，超界靜默正規化（⚠️ AGENTS.md 高危陷阱 #2） |
| **注意** | 權重檔案在啟動時才同步到磁碟。重啟後新 agent 才出現。 |
| **成熟度** | S (stable) |

---

### 層級 5：總經數據（Macro Layer）

```
data/state/macro/
  2026-04-01.json
  2026-04-02.json
  ...（38 個每日檔案）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每日（`globalmarket` 自動排程或手動觸發） |
| **生產者** | `internal/globalmarket/`（全球總經資料管理器） |
| **消費者** | `internal/narrative/`（宏觀敘事事件偵測：`US_rates_up`, `oil_price_shock`, `geopolitical_risk`） |
| **消費者** | `internal/industry/dynamic_env.go`（動態環境調變器：週期評分計算） |
| **消費者** | `internal/industry/cycle.go`（商業週期偵測：expansion/recovery/mature/recession） |
| **資料內容** | 原油價格、BDI 波羅的海指數、DXY 美元指數、台幣匯率、台灣加權指數、VIX |
| **格式** | `YYYY-MM-DD.json`（每日獨立檔案） |
| **成熟度** | E (evolving) — 指標仍在迭代新增 |
| **Schema** | 無（欄位隨來源 API 變化） |

### 層級 6：融資融券數據（Margin Layer）

```
data/state/margin/
  20260401_margin.json
  20260402_margin.json
  ...（34 個每日檔案）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每日（透過 TWSE 融資融券 API） |
| **生產者** | `internal/marketdata/`（TWSE margin API） |
| **消費者** | `internal/industry/`（產業輪動分析 — 市場情緒維度） |
| **消費者** | `internal/portfolio/`（市場情緒因子輸入） |
| **格式** | `YYYYMMDD_margin.json` |
| **成熟度** | E (evolving) |

### 層級 7：資金流向數據（Capital Flow Layer）

```
data/state/capital_flow/
  20250601_capital_flow.json
  20250602_capital_flow.json
  ...（82 個每日檔案）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每日（透過 TWSE 主力/法人資金流向 API） |
| **生產者** | `internal/marketdata/twse_capital_flow_provider.go`（P1.1 命名修正後） |
| **消費者** | `internal/industry/`（產業資金流向分析 — 行業輪動） |
| **格式** | `YYYYMMDD_capital_flow.json`（P1.1 修正：原為 `YYYYMMDD.json`） |
| **雙儲存** | 同時寫入 PostgreSQL `capital_flow` hypertable（⚠️ P0.1 A.1） |
| **成熟度** | E (evolving) |

---

### 層級 8：實驗與演化（Experiment & Evolution Layer）

```
data/state/experiments.jsonl              ← 實驗記錄（append-only）
data/state/experiments/                   ← 實驗目錄（183 JSON + archive/）
data/state/mutation-briefs/               ← 突變提案（132 個檔案）
data/state/constraint-mutations/          ← 限制條件突變（1 YAML）
```

| 檔案 | 說明 |
|------|------|
| **experiments.jsonl** | 實驗執行結果（mutation → execute → judge 完整生命週期），由 `internal/experiment/executor.go` 寫入，`internal/experiment/judge.go` 讀取 |
| **experiments/** | 每個實驗的詳細數據（JSON 格式），含 archive/ 歷史歸檔 |
| **mutation-briefs/** | 演化突變提案（`internal/evolution/` 建構），每個檔案描述一個 agent 參數調整/策略變更方案 |
| **constraint-mutations/** | 限制條件突變配置（YAML 格式），由演化引擎使用 |

| 成熟度 | experiments = S (stable), mutation-briefs = E (evolving), constraint-mutations = U (utility) |

---

### 層級 9：人工干預（Human Intervention Layer）

```
data/state/human_interventions.jsonl      ← 干預記錄（append-only）
data/state/approvals/                     ← 核准決策目錄（74 筆）
```

| 檔案 | 說明 |
|------|------|
| **human_interventions.jsonl** | 所有人工干預記錄（放行/否決/補追），由 Dashboard API 寫入，orchestrator 控制層讀取 |
| **approvals/** | 人工核准決策的個別檔案，每個決策對應一筆 `(symbol, agent_id)` 組合。三種按鈕：放行（確保不被濾除）、否決（強制排除）、補追（針對已被擋下的項目重新放行） |

| **成熟度** | S (stable) — 稽核軌跡核心 |

---

### 層級 10：Swarm 模擬（Swarm Layer）

```
data/state/swarm_latest.json              ← 最新 swarm 結果
data/state/swarm_training/                ← 訓練過程記錄（5 JSONL）
```

| 檔案 | 說明 |
|------|------|
| **swarm_latest.json** | 最新一次 MiroFish swarm 模擬結果（覆蓋寫入），由 `internal/swarm/` 寫入 |
| **swarm_training/** | 訓練過程的逐步記錄（JSONL 格式），用於回放分析 |

| **成熟度** | E (evolving) — swarm 訓練仍在迭代 |

---

### 層級 11：執行追蹤與事件（Trace & Event Layer）

```
data/state/traces/                        ← 執行追蹤（27 個 session JSONL）
data/state/eventlogic/                    ← 事件邏輯狀態（2 個檔案）
```

| 目錄 | 說明 |
|------|------|
| **traces/** | 執行追蹤記錄（決策鏈每一步的詳細資訊），由 `internal/orchestrator/` 寫入，用於診斷與除錯 |
| **eventlogic/** | 事件邏輯處理狀態，由 `internal/eventlogic/` 管理，`internal/eventbus/` 使用 |

| **成熟度** | traces = U (utility), eventlogic = X (experimental) |

---

### 層級 12：回測視窗（Backtest Window Layer）

```
data/state/windows/                       ← 回測視窗記錄（95 個）
```

| 欄位 | 說明 |
|------|------|
| **寫入時機** | 每次 `Window.Run()` 完成後 |
| **生產者** | `internal/backtest/window.go` |
| **消費者** | `cmd/backtest-window/` CLI |
| **成熟度** | X (experimental) |

---

### 層級 13：ML 模型（ML Model Layer）

```
data/state/ml_models/                     ← ML 模型檔案（4 個）
```

| 欄位 | 說明 |
|------|------|
| **內容** | 序列化模型權重/參數檔案 |
| **生產者** | `internal/metalearning/`、實驗性 CLI |
| **消費者** | `internal/metalearning/`（MetaLearner 策略選擇優化） |
| **成熟度** | X (experimental) — 實驗階段 |

---

### 層級 14：系統狀態（System State Layer）

```
data/state/simulation_state.json          ← 模擬引擎狀態
data/state/metalearner_state.json         ← 元學習狀態
data/state/maturity_tracker.json          ← 成熟度追蹤
data/state/metrics.jsonl                  ← 系統指標（append-only）
data/state/phase3_metrics.json            ← Phase 3 專用指標
data/state/clamping_events.jsonl          ← 權重夾制事件（append-only）
data/state/channel_health.json            ← 資訊通道健康狀態
data/state/alerts/                        ← 系統告警
data/state/branch-protection-snapshots/   ← 分支保護快照（4 個）
data/state/autobacktest/                  ← 自動回測記錄
```

| 檔案 | 生產者 | 消費者 | 成熟度 |
|------|--------|--------|--------|
| `simulation_state.json` | `internal/sim/` | orchestrator（啟動恢復） | U |
| `metalearner_state.json` | `internal/metalearning/` | metalearning | E |
| `maturity_tracker.json` | `cmd/check-maturity/` | monitoring | U |
| `metrics.jsonl` | `internal/monitoring/` | Dashboard API | E |
| `phase3_metrics.json` | `internal/monitoring/phase3_metrics.go` | Dashboard API | E |
| `clamping_events.jsonl` | `internal/portfolio/darwinian_weights.go` | Dashboard API | E |
| `channel_health.json` | `internal/apigateway/` | Dashboard API (`/api/dashboard/data-channels`) | U |
| `alerts/` | monitoring 告警引擎 | Dashboard API | U |
| `branch-protection-snapshots/` | CI 工具 | CI/CD pipeline | U |
| `autobacktest/` | 自動回測工具 | 回測分析 | U |

---

### 層級 15：外部資料快取（Provider Cache Layer）

```
data/state/finmind/                       ← FinMind API 快取
data/state/fubon/                         ← Fubon API 快取
data/state/fugle/                         ← Fugle API 快取
data/replay/                              ← 歷史回放數據（CSV/JSONL）
data/cache/dividends/                     ← 股息快取
```

| 目錄 | 說明 | 成熟度 |
|------|------|--------|
| **finmind/** | FinMind API 回應快取（免費，600 req/min），HybridProvider 回退鏈第 2 順位 | U |
| **fubon/** | Fubon API 回應快取（免費但需帳戶，300 req/min），回退鏈第 3 順位（⚠️ Go SDK 不支援行情 API，需 Python 微服務） | U |
| **fugle/** | Fugle API 回應快取（付費，50 req/min，Circuit Breaker 保護），回退鏈最後手段 | U |
| **replay/** | 歷史市場回放數據（`tw_extended_90days.csv` + `*.jsonl`），由 `cmd/import-replay/` 從 CSV 轉換 | U |
| **cache/dividends/** | 股息數據快取，可重新生成 | U |

**混合提供者回退順序**：TWSE OpenAPI（免費首選）→ FinMind → Fubon → Fugle（付費最後手段，Circuit Breaker 保護）

---

### 層級 16：參考數據（Reference Data Layer）

```
data/sector_data/sector_data.json         ← 產業分類數據
data/fundamentals.json                    ← 基本面參考數據（84KB）
data/test_returns.json                    ← 測試用報酬數據
```

| 檔案 | 說明 | 消費者 |
|------|------|--------|
| `sector_data.json` | 產業分類映射（股票代碼 → 產業代碼），手動維護 | `internal/industry/`、`internal/screener/` |
| `fundamentals.json` | 基本面數據（P/E、P/B、股息率），用於 screener 過濾 | `internal/screener/` |
| `test_returns.json` | 測試用報酬數據 | 測試套件 |

---

### 層級 17：參考與特殊數據

```
data/state/geopolitical/                  ← 地緣政治事件數據（2 個檔案）
data/state/tsmc_revenue/                  ← 台積電月營收數據（2 個檔案）
data/state/export/                        ← 匯出資料（3 個檔案）
data/state/parameter-snapshots/           ← 參數配置快照（17 個）
data/state/live/state/                    ← 即時交易狀態（內部結構）
```

| 目錄 | 說明 | 成熟度 |
|------|------|--------|
| **geopolitical/** | 地緣政治事件數據，由 `internal/narrative/` 偵測，用於宏觀敘事因果鏈 | X |
| **tsmc_revenue/** | 台積電月營收數據，用於 `AI_capex_surge` 敘事偵測 | X |
| **export/** | 匯出工具輸出（格式依匯出類型） | U |
| **parameter-snapshots/** | `internal/config/parameters.go:Save()` 的歷史快照，追蹤參數演變 | U |
| **live/state/** | 即時交易狀態，由 live 模組自行管理內部結構 | E |

---

### 層級 18：SQLite 資料庫（⚠️ 待處理）

```
data/state/atlas.db                       ← SQLite 3.x, 172KB, 6 tables, 52 rows
```

| 欄位 | 說明 |
|------|------|
| **Tables** | `outcomes`, `screening_rejects`, `experiments`, `session_summaries`, `human_interventions`, `quotes` |
| **生產者** | `cmd/migrate-jsonl-to-sqlite/main.go`（遷移工具） |
| **消費者** | `internal/config/config.go`（路徑參考，無主動讀取） |
| **⚠️ 問題** | 文件化不足：無 schema doc、無 AGENTS.md 提及、無 migration 記錄。與 PostgreSQL 內容重疊。 |
| **處理** | 見 P2.1 處理方案 — 決定保留/遷移/移除 |
| **成熟度** | X (experimental) |

---

### 層級 19：歷史歸檔與備份（Archive Layer）

```
data/state-archive/                       ← 歷史歸檔（7 個 timestamp 子目錄，⚠️ 全為空）
data/state/recommendation_outcomes.jsonl.backup.20260414062052  ← 手動備份（⚠️ orphaned）
```

| 檔案 | 狀態 | 處理 |
|------|------|------|
| `state-archive/` | 目錄存在但所有子目錄為空 — archiving 流程可能損壞 | P2.2 調查與修復 |
| `recommendation_outcomes.jsonl.backup.*` | Orphaned 手動備份，零 Go 程式碼參考 | P2.2 移除 |

---

## 資料流（Data Flow）

### 寫入路徑（完整）

```
RunDailySimulation()
  │
  ├── marketdata.GetQuotes()  → TWSE → FinMind → Fubon → Fugle
  ├── screener.ApplyFilters() → screening_rejects (篩選排除)
  ├── orchestrator.Execute()
  │     ├── collectRecommendations()
  │     │     └── factor_engine.CalculateAllScoresWithBreakdown()
  │     └── conviction_builder.Build()
  │
  ├── sim.Run() → simulation_state.json
  │
  ├── repo.RecordOutcomes()    → PostgreSQL (dual-write)
  ├── ledger.RecordOutcomes()  → data/state/recommendation_outcomes.jsonl (O_APPEND)
  └── ledger.RecordSessionOutcomes() → data/state/sessions/<session-id>/ (原子寫入)
```

**每日排程任務**（BackgroundTaskManager）：
```
macro data: globalmarket.FetchDailyMacro() → data/state/macro/YYYY-MM-DD.json
margin data: marketdata.FetchMargin() → data/state/margin/YYYYMMDD_margin.json
capital_flow: twse_capital_flow_provider.Fetch() → data/state/capital_flow/YYYYMMDD_capital_flow.json
channel health: apigateway.HealthCheck() → data/state/channel_health.json
```

**實驗生命週期**：
```
evolution.BuildMutationBrief() → data/state/mutation-briefs/<brief>.json
experiment.Executor.Run() → data/state/experiments.jsonl + data/state/experiments/
experiment.Judge.Evaluate() → 讀取 experiments.jsonl → judge result
baseline.Promote() / baseline.Revert() → data/state/baseline_policy.json
```

### 讀取路徑

```
校準工具 (calibrateDarwinian / calibrateRiskGate):
  → loadOutcomesFromSessions()           ← 讀取所有 session 目錄
  → 遍歷 data/state/sessions/*/
      → 讀取 recommendation_outcomes.jsonl
  → RiskGate.SelfCalibrate()              ← 校準風險參數（最近 30 session）

Dashboard API:
  → PipelineService.LoadRecommendationPipeline()
  → 讀取 session 目錄的最新 session
  → GET /api/dashboard/data-channels     ← channel_health.json
  → GET /api/dashboard/risk-calibration   ← calibration reports

Agent 績效:
  → DarwinianWeightManager.GetAllAgentWeightData()
  → 從記憶體讀取（在啟動時從 darwinian_weights.json 載入）

產業分析:
  → SeasonalEngine.GetAdjustmentBreakdown()
  → 讀取 macro/、margin/、capital_flow/（每日數據）
  → SupplyChainGraph.PropagateShock()
  → 讀取 supply_chain_graph.json（configs/） + sector_data.json

回測:
  → backtest.Window.Run()
  → 讀取 replay/（歷史數據）+ sessions/（outcome 資料）
```

---

## AI 代理常見錯誤與解決方案

| 錯誤 | 發生原因 | 正確做法 |
|------|---------|---------|
| 資料不足（"insufficient data"） | 讀取 `data/state/recommendation_outcomes.jsonl`（全域檔案可能不完整） | 改為讀取 session 目錄（有 per-agent forward return） |
| 找不到 forward return | 只查了 summary.json（沒有 forward return 欄位） | 讀取 session 目錄的 `recommendation_outcomes.jsonl` |
| 校準結果全是 0 或 1 | 用了合成數據（沒有足夠真實數據） | 等待系統跑更多 session 累積真實 outcome；檢查 `is_synthetic` 欄位 |
| Darwinian 權重不變 | 讀取了舊的 `darwinian_weights.json` | 等待啟動時的 `Save()` 或手動觸發 session |
| 找不到總經數據 | 不知道 macro/ 目錄位置或用錯日期格式 | 使用 `YYYY-MM-DD.json` 格式讀取 `data/state/macro/` |
| 混淆 JSONL vs CSV | 用 CSV parser 讀 JSONL 檔案 | JSONL = 每行獨立 JSON 物件，非 CSV |
| 找不到 Schema | 之前完全沒有 JSON Schema | 現在有 `schemas/recommendation_outcomes.schema.json`，見 `docs/JSON_SCHEMA_STANDARD.md` |
| 資料路徑寫死 | 硬編碼 `data/state/` 路徑 | 使用 `internal/config/config.go` 中的配置變數（`ATLAS_REPLAY_DATA_PATH` 等） |

---

## 如何新增資料消費者

1. 確認你需要的資料類型（outcome? session? agent weight? macro?）
2. 查看 `docs/DATA_CATALOG.md` 找到對應的資料資產及路徑
3. 讀取 `_metadata.json`（若有）了解成熟度與使用限制
4. 查看本文「讀取路徑」找到對應的讀取方法
5. 參考現有消費者代碼：
   - 校準工具：`cmd/calibrate-parameters/main.go` → `loadOutcomesFromSessions()`
   - Dashboard API：`internal/monitoring/service/pipeline.go`
   - Darwinian 權重：`internal/portfolio/darwinian_weights.go`
   - 總經分析：`internal/narrative/ingestor.go`、`internal/industry/dynamic_env.go`
6. **不要**直接 `os.Open` —— 使用 `ledger.NewStore()` 或 repository 提供的查詢介面
7. **不要**假設資料只在一個地方 —— 優先使用最完整的來源（Session 目錄 > 全域 JSONL > PostgreSQL）

---

## 如何新增資料生產者

1. 確認資料分類：replay / cache / reference / state？
2. 若為 state/，遵循 `docs/DATA_DIRECTORY_STANDARD.md` — 建立子目錄，不放平面檔案
3. 遵循 `docs/DATA_NAMING_CONVENTION.md` — 使用標準命名格式
4. 建立 `_metadata.json`（遵循 `docs/DATA_MATURITY_STANDARD.md`）
5. 如有 JSON/JSONL 資料，建立 JSON Schema（遵循 `docs/JSON_SCHEMA_STANDARD.md`）
6. 更新 `docs/DATA_CATALOG.md`（CI 會檢查新鮮度）
7. 若需要定時寫入，必須透過 `BackgroundTaskManager` 註冊（遵循 `internal/apigateway/CONSTITUTION.md` 第四條）
8. 若需外部資料抓取，必須透過已註冊的 `marketdata.Provider`（遵循 憲法第一條）

---

## 相關文件

- `docs/DATA_CATALOG.md` — 完整資料資產目錄（39 個資產）
- `docs/DATA_NAMING_CONVENTION.md` — 檔案命名規範（R1-R10）
- `docs/DATA_DIRECTORY_STANDARD.md` — 目錄結構規範與遷移路徑
- `docs/DATA_MATURITY_STANDARD.md` — 資料成熟度標記標準
- `docs/JSON_SCHEMA_STANDARD.md` — JSON Schema 定義標準
- `internal/ledger/ledger.go` — 檔案型 ledger 實現
- `internal/repository/dual_write.go` — PostgreSQL + 檔案雙寫
- `internal/orchestrator/system.go` — 資料流主控
- `internal/portfolio/darwinian_weights.go` — Darwinian 權重管理
- `internal/marketdata/provider.go` — 資料提供者抽象（TWSE/FinMind/Fubon/Fugle）
- `internal/apigateway/CONSTITUTION.md` — API Gateway 憲法（通道管理、背景任務、參數管理）
- `cmd/calibrate-parameters/main.go` — 校準工具
- `docs/PARAMETER_SYSTEM.md` — 參數系統文件
- `docs/audit/P0.1_verification_report.md` — 資料審計驗證報告（2026-06-02）
- `docs/audit/P0.2_root_cause_analysis.md` — 根因分析（FG-1/FG-2/FG-3）
