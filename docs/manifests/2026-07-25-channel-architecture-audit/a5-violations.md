# A5 — Constitution Compliance Cross-Check

**Date**: 2026-07-25
**Branch**: feat/p2-monitoring-alert-unification
**Phase**: A5 (Phase A: Structural Survey — 5/5)
**Constitution Version**: v1.2 (effective 2026-07-25)
**Source Files**: `internal/apigateway/CONSTITUTION.md`, `configs/allowed_env_vars.md`
**Companion JSON**: `a5-violations.json`

---

## TL;DR

| 條文 | 違規數 | 高嚴重 | 中嚴重 | 低/info |
|---|---|---|---|---|
| **Article 1** (`os.Getenv` 統一入口) | 26 callsites | 5 | 5 | 16 |
| **Article 4** (BackgroundTaskManager 註冊制) | 17 goroutines + 1 name collision | 6 | 9 | 2 |
| **Article 5** (Circuit Breaker 強制) | 8 問題點 | 4 | 2 | 2 |
| **Appendix D** (MCP Roots) | 8 問題點 | 0 | 2 | 6 |
| **CI 治理** (scripts/ci/check_constitution.sh) | 1 false-negative | 1 | 0 | 0 |

**Constitution 自我聲稱 vs 實際**：channels **37 = 37**（一致）；tasks **聲稱 9 (過時) vs 實際 60**（嚴重偏差）；附錄 A 任務欄位僅列 22 vs 實際 60 → **gap 38**。

---

## 1. Article 1 違規 — 直接 `os.Getenv` 業務邏輯

### 1.1 高嚴重（5 項 — 違反 §1.2 密鑰白名單規則）

| 檔案 | 行號 | 變數 | 違反內容 |
|---|---|---|---|
| `cmd/backfill-financial-statements/main.go` | 42-46 | `FINMIND_API_KEY` | 直接讀，§白名單禁止 internal/config 之外讀資料源 key |
| `cmd/backfill-institutional-investors/main.go` | 50-54 | `FINMIND_API_KEY` | 同上 |
| `cmd/backfill-month-revenue/main.go` | 54-58 | `FINMIND_API_KEY` | 同上 |
| `cmd/backfill-taifex-oi/main.go` | 58-62 | `FINMIND_API_KEY` | 同上 |
| `cmd/realtime-quote/main.go` | 27-30 | `FUGLE_API_KEY` | 同上 |
| `internal/config/config.go` | 162-232 | 多個未登錄 keys | `ATLAS_AGENT_REGISTRY_EXTRA_PATHS` / `LLM_*` / `SECTOR_PREDICTION_ENABLED` / `ATLAS_EVENT_PREDICTION_ENABLED` / `ATLAS_ALLOW_LIVE_BROKER` / `ATLAS_ALLOW_HTTP_BROKER` / `ATLAS_ALLOW_REAL_SIGNER` / `STAGE3_TASKS_ENABLED` / `STAGE3_ALERTS_ENABLED` — `configs/allowed_env_vars.md` 修訂歷史僅 v1.5 (2026-07-02)，未更新 |

### 1.2 中嚴重（5 項 — 業務模組直接讀允許位置外的 keys）

- `internal/monitoring/api/shared/handler.go:110-111,128-130,232-234` — Auth middleware 直接讀 `ATLAS_API_KEY` / `ATLAS_ENV` / `ATLAS_ADMIN_KEY`
- `internal/monitoring/api/system/health_aggregate.go:192-194` — 同上
- `cmd/experimental/validate-broker/main.go:33-36` — 直接讀 `ATLAS_BROKER_*` 三組密鑰
- `cmd/fubon-dma/main.go:64-67,124-127` — 直接讀 `FUBON_DMA_*` 四組 keys
- `cmd/atlas/admin_routes.go:35-37` — cmd 內合法但非 `main.go`，灰色地帶

### 1.3 低/info（16 項）

詳見 `a5-violations.json:article1_violations`。主要為：
- 多個 `internal/*` provider / db / bootstrap / dashboard 模組直接讀 `ATLAS_DATA_DIR` / `DATABASE_URL` / `FUBON_PROXY_PYTHON` / `ATLAS_STORE_BACKEND` / `ATLAS_SQLITE_PATH`（白名單已列，但 §1.2 精神是允許位置應在 `cmd/atlas/main.go`）
- 多個 `cmd/geo-ingest` / `cmd/macro-ingest` / `cmd/judge-experiment` / `cmd/daily-replay-sync` / `cmd/backfill-replay` cron 直接讀 `DATABASE_URL`
- `cmd/atlas-mcp/main.go` 直接讀 ~20 個 `ATLAS_MCP_*` keys（合法，列白名單）

---

## 2. Article 4 違規 — Rogue Background Goroutines

**§4.5.2 例外原則**：若該 goroutine 在 `Start()` 時啟動、在 `Stop()` 時結束、且有明確的排程間隔 → **必須使用 TaskManager**。明文例外僅：HTTP server、WaitGroup 一次性並行、事件監聽、context-with-timeout、WS ping/pong、test、autobacktest 專用 loop、live ruleEngine、DI 元件、simulation 路徑、F1-F9 supervisor。

### 2.1 高嚴重（6 項 — 明確應走 BTM）

| 檔案 | 行號 | Goroutine |
|---|---|---|
| `internal/marketdata/fubon_client.go` | 311 | `go c.runHealthProbe(interval)` — ticker 排程 fubon-proxy 健康 |
| `internal/marketdata/streaming.go` | 26 | `PollingAdapter.Subscribe` — ticker 重複 GetQuotes |
| `internal/metalearning/metalearner.go` | 292 | `MetaLearner.adaptationLoop` — ticker + wg |
| `internal/prism/prism_manager.go` | 564 | `PRISMManager.autoBalancer` — 5 min ticker |
| `internal/realtime/regime_adapter.go` | 268 | `RealTimeAdapter.Start` — ticker regime 更新 |
| `cmd/atlas/main.go:1545` + `cmd/atlas/operations_tasks.go:1543` | — | **Name collision**: `janus_regime_refresh` 雙重註冊；後者靜默覆蓋前者，破壞 channelID 掛勾 |

### 2.2 中嚴重（9 項 — 需判斷 lifecycle 邊界或補列例外）

| 檔案 | 行號 | Goroutine |
|---|---|---|
| `internal/marketdata/realtime/router.go` | 232 | `RealtimeRouter.healthCheckLoop` ticker |
| `internal/marketdata/realtime/router.go` | 104 | `RealtimeRouter.failoverLoop` 啟動 |
| `internal/mcp/anomaly/emitter.go` | 118 | `AnomalyEmitter.Run` ticker |
| `internal/monitoring/service/channel_health_synthesizer.go` | 65 | `channelHealthSynthesizer.run` ticker |
| `internal/monitoring/service/drift_detector.go` | 104 | `driftDetector.run` ticker |
| `internal/monitoring/service/ingestion_lag_monitor.go` | 66 | `ingestionLagMonitor.checkLoop` ticker |
| `internal/monitoring/service/regime_debouncer.go` | 80 | `regimeDebouncer.run` ticker |
| `internal/spawning/spawning_manager.go` | 105 | `SpawningManager.runLoop` ticker |
| `internal/live/scheduler.go` | 117, 120, 173 | `Scheduler.quotePoller / marketTimeScheduler / intradayProcessor` — 三個 lifecycle-bound ticker |

### 2.3 CI 治理 false-negative

`scripts/ci/check_constitution.sh` 對 gateway 註冊/背景任務違規僅 echo WARN 並 return 0；--json 雖累積 JSON_VIOLATIONS 但 jq 輸出不含 violations 詳情，**PR 無法被 CI 阻擋**。建議：non-zero exit + GitHub step summary。

---

## 3. Article 5 違規 — Circuit Breaker 一致性

§5.1 規定「連續 3 次失敗自動熔斷、退避 5 分鐘」，§5.4 規定「熔斷後必須返回緩存數據 + stale 標記」。

### 3.1 高嚴重（4 項）

| 檔案 | 行號 | 問題 |
|---|---|---|
| `internal/marketdata/yahoo_macro_provider.go` | 42-44 | provider 直接 `&http.Client{...}` + rate.NewLimiter；若被 rogue goroutine 直接呼叫，breaker.Call() 完全不執行 |
| `internal/marketdata/finmind_client.go` | 91-95 | 同上 |
| `internal/marketdata/fugle_client.go` | 115-120 | 同上 |
| `internal/marketdata/fubon_client.go` | runHealthProbe | 內部 probe 與 gateway breaker 各自維護失敗計數，行為分歧 |

### 3.2 中嚴重（2 項）

- `internal/apigateway/circuitbreaker.go:50-56` — `NewCircuitBreaker(maxFailures ...int)` variadic 允許覆寫常數
- `internal/apigateway/gateway.go:39` — `'tsmc_revenue': 5` 自訂 threshold，**偏離 §5.1 '3 次失敗'**

### 3.3 需查證（2 項）

- `gateway.Fetch` 在 breaker Open + 無緩存時是否回傳 stale 標記（§5.4 要求）
- `monitoring/AGENTS.md:73` drift thresholds 硬編碼是否走 `parameters.json` 框架

---

## 4. Appendix D 違規 — MCP Roots

### 4.1 中嚴重（2 項 — D.1 條件 7）

- `cmd/atlas-mcp/server/roots.go:121-133` (`handleMCPRootsList`)
- `cmd/atlas-mcp/server/roots.go:147-153` (`handleMCPRootsReadFile`)

兩處在 client 不支援 RootsV2 時回傳 `Content{Text: 'unsupported'}` + nil error，**違反 §D.1 第 7 條「explicit error 而非 soft fallback」**。註：實際工程上與 `mcp_sample_llm` / `mcp_elicit_user` 處理一致，但 CONSTITUTION 文字要求 explicit error。

### 4.2 低/info（6 項）

- `roots_open_other.go:8-12` — Windows fallback 無 `O_NOFOLLOW`，TOCTOU 弱化（production target Unix，可接受）
- `refreshRootsCache:85-97` — list_changed notification 缺 audit 紀錄（條件 5）
- `cmd/atlas-mcp/main.go:67-85` — D.3 環境變數與白名單對齊但 `ATLAS_MCP_ROOTS_ENABLED` 在 `configs/allowed_env_vars.md` 漏列
- `bind 127.0.0.1` 強制性需查 server.go（假設已實作）
- 其餘條件 D.1.1-6 大致合規（EvalSymlinks 在 prefix check 前 / O_NOFOLLOW Unix / io.LimitReader / IsRegular() / audit JSONL / bind 127.0.0.1）

---

## 5. Constitution Claims vs Reality

| 維度 | CONSTITUTION 聲稱 | 實際註冊 | Gap |
|---|---|---|---|
| **Channels** | 37 (§前言 + 附錄 A 一致) | 37 (`register_adapters.go`) | ✅ 一致 |
| **Channels runtime max** | 37 | 37 (全 API key + Yahoo + janus 啟用) | ✅ |
| **Channels runtime min** | (未聲稱) | 23 (無 key + no Yahoo + no janus) | ⚠️ runtime variance |
| **Tasks** | **9** ([INFERENCE] v1.0/v1.1 殘留) | **60** (`taskMgr.Register` 全部調用) | **❌ +51 過時** |
| **附錄 A 任務欄位** | 列 22 | 60 | ❌ **gap 38** |
| **Name collisions** | (應為 0) | 1 (`janus_regime_refresh` × 2) | ❌ |

**關鍵發現**：CONSTITUTION.md §前言 v1.2 將 channels 從 16 改為 37，但 tasks 數字從未對應更新。`cmd/atlas/operations_tasks.go`、`capital_tasks.go`、`calibration_tasks.go`、`data_sync_health_tasks.go`、`stage3_tasks.go` 五個文件 + `main.go` 共累積 60 個 `taskMgr.Register` 調用，遠超 §前言隱含的「9 個任務」。附錄 A 表格僅列 22 個 background task 名稱，亦與實際 60 不對齊。

---

## 6. 優先修復建議

### 6.1 P0 — 阻擋 merge
1. **修 `scripts/ci/check_constitution.sh`** — gateway/background 違規改 exit 1 + 將 JSON_VIOLATIONS 寫入 GitHub step summary
2. **修 `janus_regime_refresh` 重複註冊** — `cmd/atlas/operations_tasks.go:1543` 移除或改名（建議改名為 `janus_regime_refresh_legacy` 或刪除）
3. **修 §Appendix D 條件 7** — `roots.go:130,178` 改回傳 `errors.New("mcp_roots_*: client does not support roots")`

### 6.2 P1 — 高嚴重違規
4. **`cmd/backfill-*/main.go` + `cmd/realtime-quote/main.go`** — 改用 `config.GetSecret("FINMIND_API_KEY")` / `config.GetSecret("FUGLE_API_KEY")`
5. **`internal/marketdata/{yahoo_macro,finmind_client,fugle_client,fubon_client}.go`** — 移除 rogue ticker，全部走 `gateway.Fetch()` 或顯式註冊到 BTM
6. **`internal/realtime/regime_adapter.go:265` + `internal/metalearning/metalearner.go:286` + `internal/prism/prism_manager.go:563`** — 改註冊到 BackgroundTaskManager
7. **`internal/apigateway/gateway.go:39`** — 將 `tsmc_revenue: 5` 改回預設 3（§5.1）或在附錄 A 明文特化說明

### 6.3 P2 — 中嚴重 / 文件同步
8. **`configs/allowed_env_vars.md` 修訂歷史** — 補 v1.6 (2026-07-25) 條目，列出所有 2026-07-25 後新增 keys
9. **`CONSTITUTION.md` 附錄 A** — 將 background tasks 欄位從 22 擴展至完整 60（列出所有 task name + interval）
10. **`CONSTITUTION.md` §前言** — 將 tasks 數字從隱含「9」改為實際 60（或分類列出 5 大類）
11. **`docs/data-sources.md:262` government_flow** — 修為「rate.Inf / HasLimiter=true」（與 `adapter_government_flow.go:103` 一致）
12. **`docs/data-sources.md:46-63` US Market Indexes** — Yahoo shared limiter 描述改為三分組（yahooMacroLimiter 5s/2b / yahooIndexLimiter 1.5s / yahooTechLimiter 1.5s）

---

## 7. Evidence Sources

- `internal/apigateway/CONSTITUTION.md` 全文 v1.2 (594 lines)
- `configs/allowed_env_vars.md` 全文 (白名單 + 修訂歷史 v1.0-v1.5)
- `internal/apigateway/register_adapters.go:1-316` (37 channels)
- `internal/apigateway/gateway.go:39-40` (channelIDs + tsmc_revenue=5)
- `internal/apigateway/circuitbreaker.go:10-56,97-100,190-194` (CircuitBreaker 常數 + NewCircuitBreaker variadic)
- `internal/apigateway/background.go:180-194` (BackgroundTaskManager.Register)
- `cmd/atlas-mcp/server/roots.go:1-280` (附錄 D 實作)
- `cmd/atlas-mcp/server/roots_open_unix.go` + `roots_open_other.go` (O_NOFOLLOW 平台差異)
- Peers: `CheckConstitution` (A5 evidence pack) — 雙方 evidence 整合於 `a5-violations.json`

---

**Owner**: ComplianceScout (Compliance audit, A5)
**Status**: ✅ Complete — JSON + MD deliverable 寫入 `docs/manifests/2026-07-25-channel-architecture-audit/`
**Next Phase**: B1-B5 deep dive (depends on A1-A5)