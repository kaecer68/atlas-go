# Data Catalog — Atlas-Go 資料資產目錄

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威來源（authoritative）  
**自動生成**: `scripts/gen_data_catalog.sh`（P2.3 之後）  
**強制性**: CI 檢查新鮮度（`scripts/ci/check_data_catalog.sh`）  
**相關文檔**: `docs/data-catalog-template.md` · `docs/data-architecture.md` · `docs/data-maturity-standard.md`

---

## 總覽

| 統計 | 數值 |
|------|------|
| 總資料資產 | 39 |
| Stable (S) | 8 |
| Evolving (E) | 8 |
| Experimental (X) | 5 |
| Utility (U) | 12 |
| JSONL | 11 |
| JSON | 23 |
| CSV | 1 |
| SQLite | 1 |
| YAML | 1 |
| 有 Schema | 2 |

---

## S · Stable — 核心生產資料

### recommendation_outcomes `data/state/recommendation_outcomes.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **大小** | ~144MB, ~38,630 lines |
| **Schema** | `schemas/recommendation_outcomes.schema.json` |
| **生產者** | `internal/ledger/outcomes.go:RecordOutcomes()` |
| **消費者** | `internal/monitoring/`, `internal/risk/`, `cmd/backtest-window/`, `web/` |
| **描述** | 所有 session 推薦結果聚合檔，系統最重要輸出資料 |

### sessions `data/state/sessions/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（100 個 session 子目錄） |
| **格式** | `session-YYYYMMDD-daily/`，每目錄含 `summary.json`、`recommendation_outcomes.jsonl`、`screened_symbols.jsonl`、`positions.json`、`experiments.jsonl` |
| **生產者** | `internal/ledger/sessions.go:RecordSessionOutcomes()` |
| **消費者** | 校準工具、Dashboard API、回測視窗、RiskGate |
| **描述** | 最完整的 per-session 資料，包含 per-agent forward return（data-architecture.md 層級 1） |

### darwinian_weights `data/state/darwinian_weights.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **大小** | ~40KB, 728 lines |
| **生產者** | `internal/portfolio/darwinian_weights.go:Save()` |
| **消費者** | `PipelineService.LoadDarwinianStatus()` |
| **描述** | 所有 agent 的當前 Darwinian 權重（限制 [0.3, 2.5]） |

### darwinian_history `data/state/darwinian_history.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **生產者** | `internal/portfolio/darwinian_weights.go` |
| **消費者** | Dashboard API、權重演進分析 |
| **描述** | Darwinian 權重歷史快照，每次變更時 append 一行 |

### baseline_policy `data/state/baseline_policy.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `internal/baseline/` |
| **消費者** | `internal/orchestrator/system.go`（啟動時載入）、`cmd/promote-baseline/`、`cmd/revert-baseline/` |
| **描述** | 當前 baseline policy 版本，實驗執行前必須確認存在 |

### experiments `data/state/experiments.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **生產者** | `internal/experiment/executor.go` |
| **消費者** | `internal/experiment/judge.go`、Dashboard API |
| **描述** | 實驗執行記錄（mutation → execute → judge） |

### human_interventions `data/state/human_interventions.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **生產者** | `internal/monitoring/api/dashboard/`（人工核准/否決/補追） |
| **消費者** | `internal/orchestrator/`（控制層讀取干預記錄） |
| **描述** | 人工干預稽核軌跡，持久化至 `data/state/approvals/` |

### approvals `data/state/approvals/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（74 筆核准決策） |
| **生產者** | Dashboard API 人工干預流程 |
| **消費者** | Orchestrator 控制層（放行/否決/補追邏輯） |
| **描述** | 人工覆寫記錄，確保被核准的推薦不被控制層濾除 |

---

## E · Evolving — 演進中的資料

### macro `data/state/macro/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（38 個每日 JSON） |
| **格式** | `YYYY-MM-DD.json` |
| **生產者** | `internal/globalmarket/`（全球總經資料管理） |
| **消費者** | `internal/narrative/`（宏觀敘事事件偵測）、`internal/industry/cycle.go`（週期羅盤） |
| **描述** | 每日總經指標快照（原油、BDI、DXY、台幣匯率等），驅動 macro narrative 和 industry cycle |

### margin `data/state/margin/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（34 個每日 JSON） |
| **格式** | `YYYYMMDD_margin.json` |
| **生產者** | `internal/marketdata/`（TWSE 融資融券 API） |
| **消費者** | `internal/industry/`（產業輪動分析）、`internal/portfolio/`（市場情緒因子） |
| **描述** | 每日融資融券餘額數據，作為市場情緒指標 |

### capital_flow `data/state/capital_flow/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（82 個每日 JSON） |
| **格式** | `YYYYMMDD_capital_flow.json`（P1.1 已修正命名） |
| **生產者** | `internal/marketdata/twse_capital_flow_provider.go` |
| **消費者** | `internal/industry/`（產業資金流向分析） |
| **描述** | 每日主力/法人資金流向數據，驅動行業輪動分析 |

### metrics `data/state/metrics.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **生產者** | `internal/monitoring/`（系統指標收集） |
| **消費者** | Dashboard API（監控頁面） |
| **描述** | 系統運行指標（latency, 成功率, throughput 等） |

### phase3_metrics `data/state/phase3_metrics.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（讀寫模式） |
| **生產者** | `internal/monitoring/phase3_metrics.go` |
| **消費者** | Dashboard API（Phase 3 指標頁面） |
| **描述** | Phase 3 專用指標（2026-05 導入），有主動讀寫，非 orphaned |

### clamping_events `data/state/clamping_events.jsonl`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL (append-only) |
| **生產者** | `internal/portfolio/darwinian_weights.go`（權重夾制事件記錄） |
| **消費者** | Dashboard API（監控頁面） |
| **描述** | Darwinian 權重超出 [0.3, 2.5] 範圍時的夾制事件記錄 |

### mutation_briefs `data/state/mutation-briefs/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（132 個檔案） |
| **生產者** | `internal/evolution/`（突變提案建構） |
| **消費者** | `internal/experiment/executor.go`（實驗執行） |
| **描述** | 進化突變提案，每個檔案描述一個 mutation（agent 參數調整/策略變更） |

### swarm_latest `data/state/swarm_latest.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `internal/swarm/`（MiroFish swarm 模擬） |
| **消費者** | Dashboard API、`internal/metalearning/`（策略進化） |
| **描述** | 最新一次 swarm 模擬結果；透過 `ExportTrainingData()` 餵入 MetaLearner 進行策略演化 |

---

## X · Experimental — 實驗性資料

### ml_models `data/state/ml_models/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（4 個模型檔案） |
| **生產者** | `internal/metalearning/`、實驗性 CLI |
| **消費者** | `internal/metalearning/`（策略選擇優化） |
| **描述** | ML 模型檔案（序列化模型權重/參數），實驗階段 |

### geopolitical `data/state/geopolitical/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（2 個 JSON） |
| **生產者** | `internal/narrative/`（地緣政治事件偵測） |
| **消費者** | `internal/narrative/`（宏觀敘事因果鏈） |
| **描述** | 地緣政治事件數據，驅動 geopolitical_risk 敘事 |

### strategy_techniques `data/state/strategy_techniques/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（12 個 strategy frames） |
| **生產者** | `internal/strategy_techniques/` |
| **消費者** | `internal/orchestrator/`（plugin）+ `internal/monitoring/api/strategies/`（handler） |
| **描述** | 5 層框架投資心法庫，9 條 production seeds + 3 L4，已穩定生產 |

### tsmc_revenue `data/state/tsmc_revenue/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（2 個檔案） |
| **生產者** | `internal/marketdata/`（TWSE 月營收 API） |
| **消費者** | `internal/narrative/`（AI_capex_surge 敘事） |
| **描述** | 台積電月營收數據，用於 AI 資本支出敘事偵測 |

### windows `data/state/windows/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（95 個紀錄） |
| **生產者** | `internal/backtest/window.go:Run()` |
| **消費者** | `cmd/backtest-window/` |
| **描述** | 回測視窗紀錄，每個視窗包含策略績效評估 |

---

## U · Utility — 輔助工具資料

### channel_health `data/state/channel_health.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `internal/apigateway/`（背景健康檢查） |
| **消費者** | Dashboard API（`/api/dashboard/data-channels`） |
| **描述** | 各資訊通道健康狀態快照（TWSE/FinMind/Fubon/Fugle） |

### simulation_state `data/state/simulation_state.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `internal/sim/`（模擬引擎） |
| **消費者** | `internal/orchestrator/system.go`（啟動恢復） |
| **描述** | 模擬引擎狀態快照，用於重啟後恢復模擬進度 |

### metalearner_state `data/state/metalearner_state.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `internal/metalearning/` |
| **消費者** | `internal/metalearning/`（策略選擇優化） |
| **描述** | 元學習協調器狀態（策略選擇權重、歷史績效） |

### maturity_tracker `data/state/maturity_tracker.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON（覆蓋寫入） |
| **生產者** | `cmd/check-maturity/`（CI 成熟度檢查工具） |
| **消費者** | `internal/monitoring/`（Dashboard 成熟度頁面） |
| **描述** | 程式碼成熟度追蹤結果快照 |

### swarm_training `data/state/swarm_training/`

| 欄位 | 值 |
|------|-----|
| **狀態** | **Archived (deleted 2026-08-14)** |
| **說明** | 2026-08-14 刪除 31.36 GB。`internal/swarm/` 套件已於 commit `db9412c3` (P2 final cleanup, 2026-07-07) 從程式碼庫移除；production 程式碼 0 reader；API 端點全 404；generator 已刪無法 regenerate。同時刪除 `swarm_latest.json` + `metalearner_state.json` (deprecated, test-only)。詳見 `.omo/manifests/2026-08-13-sqlite-to-postgres-migration.md` §七-B。 |

### traces `data/state/traces/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（27 個 session JSONL） |
| **生產者** | `internal/orchestrator/`（執行追蹤） |
| **消費者** | 診斷與除錯 |
| **描述** | 執行追蹤記錄（決策鏈每一步的詳細資訊） |

### alerts `data/state/alerts/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個檔案） |
| **生產者** | `internal/monitoring/`（告警規則引擎） |
| **消費者** | Dashboard API |
| **描述** | 系統告警記錄 |

### autobacktest `data/state/autobacktest/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個檔案） |
| **生產者** | 自動回測工具 |
| **消費者** | 回測分析 |
| **描述** | 自動回測結果記錄 |

### branch-protection-snapshots `data/state/branch-protection-snapshots/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（4 個快照） |
| **生產者** | Branch protection 檢查工具 |
| **消費者** | CI/CD pipeline |
| **描述** | 分支保護狀態快照，非 runtime 資料 |

### constraint-mutations `data/state/constraint-mutations/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個 YAML） |
| **生產者** | `internal/evolution/` |
| **消費者** | 實驗生命週期 |
| **描述** | 限制條件突變配置 |

### export `data/state/export/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（3 個檔案） |
| **生產者** | 匯出工具 |
| **消費者** | 外部分析 |
| **描述** | 匯出資料（格式依匯出類型而定） |

### parameter-snapshots `data/state/parameter-snapshots/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（17 個快照） |
| **生產者** | `internal/config/parameters.go:Save()` |
| **消費者** | 參數校準分析 |
| **描述** | 參數配置歷史快照，追蹤參數演變 |

### finmind `data/state/finmind/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個快取） |
| **生產者** | `internal/marketdata/finmind_client.go` |
| **消費者** | HybridProvider（回退鏈） |
| **描述** | FinMind API 回應快取 |

### fubon `data/state/fubon/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個快取） |
| **生產者** | `internal/marketdata/fubon_client.go` |
| **消費者** | HybridProvider（回退鏈） |
| **描述** | Fubon API 回應快取 |

### fugle `data/state/fugle/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（1 個快取） |
| **生產者** | `internal/marketdata/fugle_client.go` |
| **消費者** | HybridProvider（回退鏈，Circuit Breaker 保護） |
| **描述** | Fugle API 回應快取（付費 API，最後手段） |

---

## Reference · 參考數據

### replay `data/replay/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（13 個 CSV/JSONL） |
| **格式** | `tw_extended_90days.csv` + `*.jsonl` |
| **生產者** | `cmd/import-replay/`（CSV → JSONL 匯入） |
| **消費者** | `internal/replay/`（回放數據載入）、`internal/sim/`（模擬引擎） |
| **描述** | 歷史市場數據，唯讀。由 TWSE CSV 匯入轉換 |

### fundamentals `data/fundamentals.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON |
| **大小** | ~84KB |
| **生產者** | 手動維護？ |
| **消費者** | `internal/screener/`（宣告式個股篩選） |
| **描述** | 基本面參考數據（P/E、P/B、股息率等） |

### sector_data `data/sector_data/sector_data.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON |
| **生產者** | 手動維護 |
| **消費者** | `internal/industry/`（產業分類）、`internal/screener/` |
| **描述** | 產業分類映射（股票代碼 → 產業代碼） |

### cache `data/cache/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory |
| **格式** | `dividends/` 子目錄 |
| **生產者** | 資料提供者快取機制 |
| **消費者** | 股息計算 |
| **描述** | 可重新生成的臨時快取資料 |

---

## Database · 資料庫

### atlas.db `data/state/atlas.db`

| 欄位 | 值 |
|------|-----|
| **類型** | SQLite 3.x |
| **大小** | 172KB, 6 tables, 52 rows |
| **Tables** | `outcomes`, `screening_rejects`, `experiments`, `session_summaries`, `human_interventions`, `quotes` |
| **生產者** | `cmd/migrate-jsonl-to-sqlite/main.go` |
| **消費者** | `internal/config/config.go`（路徑參考） |
| **狀態** | 本機 dev artifact；`ATLAS_STORE_BACKEND=sqlite` 時使用。prod 上 `ATLAS_STORE_BACKEND=postgres` 為空殼或不會被寫入。 |
| **注意** | 禁止把此路徑當成 production 資料來源；CLI/job 讀資料必須走 `store_factory` backend-aware 路徑。 |

### PostgreSQL `atlas` database

| 欄位 | 值 |
|------|-----|
| **類型** | PostgreSQL 15（TimescaleDB hypertable） |
| **Tables** | `metrics`, `recommendation_outcomes`, `capital_flow`, `export_statistics`, `alerts`, `users`, `workspaces`, `screening_rejects`, `session_summaries`, `human_interventions`, `quotes`, `detector_scan_log`, `task_liveness`, `experiment_lineage`, `baseline_history`, `metric_trends`, `stock_signal_outcomes`, `stock_win_rate`, `regime_history`, `stress_index_history`, `period_history`, `geopolitical_history`, `event_calendar_history`, `prediction_backtest` |
| **Migration** | `sql/migrations/`（19 組 up/down） |
| **壓縮** | TimescaleDB compression（7 天後） |
| **保留** | 90 天 retention policy |
| **生產者** | `internal/repository/dual_write.go`、`internal/ledger/postgres_*.go` |
| **消費者** | Dashboard API（`repo.Query*()` 方法） |
| **SSoT** | `quotes` 由 PostgreSQL 作為單一真相來源；SQLite `atlas.db.quotes` 僅為本機 dev artifact，prod 不可信。 |

---

## Archive · 歷史歸檔

### state-archive `data/state-archive/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（7 個 timestamp 子目錄） |
| **狀態** | ⚠️ 目錄為空 — archiving 流程可能損壞，見 P2.2 |
| **生產者** | 歸檔工具（未知） |
| **描述** | 歷史狀態歸檔（2026-05-06/07 timestamp 快照） |

### test_returns `data/test_returns.json`

| 欄位 | 值 |
|------|-----|
| **類型** | JSON |
| **生產者** | 手動維護（測試輔助） |
| **消費者** | 測試套件（`go test ./...`） |
| **描述** | 測試用報酬數據，用於模擬引擎和 factor engine 的單元測試 |
| **成熟度** | U (utility) |

### live `data/state/live/`

| 欄位 | 值 |
|------|-----|
| **類型** | Directory（含 state/ 子目錄） |
| **生產者** | `internal/live/`（即時交易模組） |
| **消費者** | live orchestrator（啟動恢復） |
| **描述** | 即時交易運行狀態，內部結構由 live 模組自行管理，不強制重整 |
| **成熟度** | E (evolving) |

### backup `data/state/recommendation_outcomes.jsonl.backup.20260414062052`

| 欄位 | 值 |
|------|-----|
| **類型** | JSONL（手動備份） |
| **狀態** | ⚠️ Orphaned — 見 P2.2 清理方案 |
| **描述** | 2026-04-14 的手動備份，無程式碼參考 |

---

## 相關文件

| 文檔 | 關係 |
|------|------|
| `docs/data-architecture.md` | 詳細讀寫路徑與資料流 |
| `docs/data-naming-convention.md` | 檔案命名規範（R1-R10） |
| `docs/data-directory-standard.md` | 目錄結構規範 |
| `docs/json-schema-standard.md` | JSON Schema 定義標準 |
| `docs/data-maturity-standard.md` | 資料成熟度標記標準 |
| `docs/data-catalog-template.md` | Catalog 範本格式定義 |
| `schemas/recommendation_outcomes.schema.json` | Outcomes JSON Schema |
| `schemas/data_metadata.schema.json` | _metadata.json Schema |
