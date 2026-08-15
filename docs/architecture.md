# Atlas 系統架構活地圖

> 最後更新：2026-07-29
> 維護紀律：每支 PR 完成後同步更新本圖涉及區域（見 `.omo/audit/ATLAS_SYSTEM_STATE.md` 尾行）。

---

## 1. 資料通道總覽（38 通道）

**註冊位置**：`internal/apigateway/gateway.go:channelIDs()`（靜態列舉） +
`internal/apigateway/register_adapters.go:RegisterChannelAdapters()`（運行時 instantiate）。

> 38 為靜態上限（含 2 個 inactive stub）。實際 runtime 註冊數 22–33 個，取決於 API key 配置與 `YahooEnabled` 旗標。

### 1.1 YH — Yahoo Finance（10 通道）

| channelID | 指標 | MacroDataSnapshot 欄位 | BTM 任務 |
|-----------|------|-----------------------|----------|
| `us_yahoo` | 9 合 1 批次：VIX/DXY/US10Y/USD_TWD/Oil/Gold/Silver/Copper/JPY | 多欄位（見 §1.6） | `macro_ingest` fan-out (5m)† |
| `us_spx` | S&P 500 指數 | `SPXIndex` | `us_market_refresh_us_spx` (5m) |
| `us_ndx` | Nasdaq Composite | `NDXIndex` | `us_market_refresh_us_ndx` (5m) |
| `us_dji` | Dow Jones Industrial | `DJIIndex` | `us_market_refresh_us_dji` (5m) |
| `us_nvda` | NVIDIA Corp | `NVDA` | `us_market_refresh_us_nvda` (5m) |
| `us_aapl` | Apple Inc | `AAPL` | `us_market_refresh_us_aapl` (5m) |
| `us_msft` | Microsoft Corp | `MSFT` | `us_market_refresh_us_msft` (5m) |
| `tsm_adr` | TSMC ADR (NYSE) | `TSMADR` | `us_market_refresh_tsm_adr` (5m) |
| `taiex_index` | 台灣加權指數（Yahoo `^TWII` → TWSE OpenAPI `MI_INDEX?type=IND` fallback） | `TAIEX` | `auto_taiex_index` (1h) |
| `tw_vol` | 20 日年化波動率（`^TWII` 快取；cache 陳舊時拒絕計算） | `HistoricalVolatility` | `macro_cache_tw_vol` (5m) |

**限流架構**：3 組 shared limiter —
- `yahooMacroLimiter`（5s/2b）：僅 `us_yahoo`
- `yahooIndexLimiter`（1.5s/1b）：`us_spx`, `us_ndx`, `us_dji`
- `yahooTechLimiter`（1.5s/1b）：`us_nvda`, `us_aapl`, `us_msft`, `tsm_adr`
- `taiexIndexLimiter`（5s/1b）：`taiex_index`
- `ExportStatisticsRate`（5s/2b）：`tw_vol`

> † `us_yahoo` 不在 `USMarketChannels()` 中（該函數僅含 8 個 index/tech channel），
> 實際由 `macro_ingest` 每 5 分鐘的 27-channel fan-out 間接 fetch。CONSTITUTION.md 附錄 A
> 列有 `us_market_refresh_us_yahoo` 實際上不存在。已知落差，勿新增同名 BTM 任務。

### 1.2 TW — TWSE / 台灣官方（11 通道）

| channelID | 資料內容 | Provider | 限流 | BTM 任務 |
|-----------|---------|----------|------|----------|
| `twse_replay` | 歷史 replay 資料（file-backed） | `GetSharedTWSEClient` | `rate.Inf` | `auto_backfill` (24h) |
| `twse_capital_flow` | 三大法人買賣超 | `TWSECapitalFlowProvider` | 5s/1b | `auto_capital_flow` (30m) |
| `twse_margin` | 融資融券餘額 | `TWSEMarginBalanceProvider` | 5s/1b | `auto_margin` (30m) |
| `twse_sector_index` | 台灣半導體指數（TAISEMI proxy） | `TWSESectorIndexProvider` | 5s/2b | `macro_cache_twse_sector_index` (15m) |
| `day_trading` | 當沖交易統計 | `DayTradingChannelAdapter` | 5s/1b | — |
| `market_volume` | 集中市場成交金額（億） | `MarketVolumeChannelAdapter` | 5s/1b | — |
| `twse_oddlot` | 零股交易 | `TWSEOddLotChannelAdapter` | 5s/2b | — |
| `twse_etf` | ETF 申購贖回淨額（⚠️ 資料源已移除，見 `internal/monitoring/known_issues.go`） | `TWSEETFChannelAdapter` | 1s/1b | — |
| `twse_insider` | 內部人持股轉讓 | `TWSEInsiderChannelAdapter` | 5s/1b | `auto_twse_insider` (1h) |
| `twse_sbl` | 借券賣出餘額（**STUB**，G02） | `TWSESBLChannelAdapter` | 2s/1b | `auto_twse_sbl` (1h, disabled) |
| `export_statistics` | 海關進出口統計 | `ExportStatisticsProvider` | 5s/2b | `auto_export` (12h) |

### 1.3 TW — 其他台灣資料源（8 通道）

| channelID | Provider | 平台 | 限流 | BTM 任務 |
|-----------|----------|------|------|----------|
| `fugle` | `GetSharedFugleClient` | 富果 API | 1s/1b | `channel_health_fugle` (1h) |
| `fubon` | `GetSharedFubonClient` | 富邦證券 proxy | 1s/1b | `channel_health_fubon` (1h) |
| `finmind` | `GetSharedFinMindClient` | FinMind | 6s/1b（免費） | `channel_health_finmind` (1h) |
| `tej` | `GetSharedTEJClient` | TEJ | 1s/1b | `tej_refresh` (1h) |
| `taifex_daily` | `TaifexChannelAdapter` | 期交所 | 5s/2b | `auto_taifex_daily` (1h) |
| `taifex_institutional` | `TaifexInstitutionalAdapter` | 期交所 法人 OI | 5s/2b | `auto_taifex_institutional` (1h) |
| `tdcc_equity_dispersion` | `TDCClientChannelAdapter`（**STUB**，G01） | 集保 | 5s/1b | — |
| `government_flow` | `GovernmentFlowProvider`（file-backed） | 官股行庫 | `rate.Inf` | `auto_government_flow` (1h) |

> **STUB 說明**：`twse_sbl` (G02) 和 `tdcc_equity_dispersion` (G01) 的 provider 尚未實作，僅註冊 channel 占位
> 供 dashboard 顯示「not yet live」。`Metadata().Stub=true`，`HealthCheck` 回傳 `inactive`，
> 因此 alerting path 不會觸發警報。詳見 `internal/apigateway/stub_adapter_test.go`。

### 1.4 GL — 全球/商品（6 通道）

| channelID | 指標 | Provider | 限流 | BTM 任務 |
|-----------|------|----------|------|----------|
| `sox_index` | SOX 費半指數 | `SOXIndexProvider` | 5s/2b | `us_market_refresh_sox_index` (5m) |
| `dram_spot_price` | DRAM 現貨價（MU 代理） | `DRAMSpotPriceProvider` | 5s/2b | `macro_cache_dram_spot_price` (5m) |
| `bdi` | 波羅的海乾散貨指數 | `BDIProvider` (CNBC) | 5s/2b | `macro_cache_bdi` (5m) |
| `frankfurter_fx` | USD/JPY 匯率 | `FrankfurterFXProvider` | 10s/1b | `macro_cache_frankfurter_fx` (10m) |
| `exchange_rate` | USD/TWD 等匯率 | `ExchangeRateProvider` | 5s/2b | `auto_exchange_rate` (1h) |
| `tsmc_revenue` | 台積電月營收 | `TSMCRevenueProvider` (FinMind) | 2m/1b | `tsmc_revenue` (24h) |

### 1.5 NT — 內部計算/文本（3 通道）

| channelID | 類型 | Provider | 限流 | BTM 任務 |
|-----------|------|----------|------|----------|
| `geopolitical` | GDELT + RSS 全球地緣風險 | `GeopoliticalChannelAdapter` | 1m/1b | `auto_geopolitical` (6h) |
| `geopolitical_taiwan` | CNA/自由/TVBS RSS | `TaiwanGeopoliticalChannelAdapter` | 1m/1b | `auto_geopolitical_taiwan` (6h) |
| `janus_regime` | JANUS 市場體制（內部計算） | `JANUSRegimeChannelAdapter` (Compute) | `rate.Inf` | `janus_regime_refresh` (6h) |

### 1.6 通道 → MacroDataSnapshot 欄位對照

`MacroDataSnapshot`（`internal/marketdata/macro_provider.go`）為系統核心資料結構，含 65 欄位。
主要通道對應：

| 通道 | 寫入快照欄位 |
|------|------------|
| `us_yahoo`（批次） | `US10Y`, `DXY`, `VIX`, `USD_TWD`, `Oil`, `Gold`, `Silver`, `Copper`, `JPY` |
| `us_spx` | `SPXIndex` |
| `us_ndx` | `NDXIndex` |
| `us_dji` | `DJIIndex` |
| `us_nvda` | `NVDA` |
| `us_aapl` | `AAPL` |
| `us_msft` | `MSFT` |
| `tsm_adr` | `TSMADR` |
| `taiex_index` | `TAIEX` |
| `tw_vol` | `HistoricalVolatility` |
| `sox_index` | `SOXIndex` |
| `dram_spot_price` | `DRAMSpotPrice` |
| `bdi` | `Bdi` |
| `exchange_rate` | `USD_TWD`（輔助） |
| `twse_capital_flow` | `ForeignInvestorNet`, `ForeignDealerNet`, `DomesticFundNet`, `DealerNet`, `DealerSelfNet`, `DealerHedgingNet` |
| `twse_margin` | `RetailMarginBalance`, `RetailShortBalance`, `MarginMaintenanceRatio` |
| `twse_etf` | `ETFNetSubscription`（⚠️ 資料源已移除，消費者 subC3 停用） |
| `twse_insider` | `InsiderNet` |
| `government_flow` | `GovernmentNet`, `InsuranceNet` |
| `export_statistics` | `ExportElectronics` |
| `market_volume` | `MarketVolume` |
| `day_trading` | `DayTradeRatio` |
| `taifex_daily` / `taifex_institutional` | `ForeignFuturesOINet` |
| `twse_sector_index` | `TaiwanSemiIndex` |
| `tsmc_revenue` | `TSMCRevenue` |

---

## 2. 資料流圖

```
┌─────────────────┐    gateway.Fetch()     ┌──────────────────┐
│  External APIs  │───────────────────────→│  Channel Adapter │
│  (Yahoo, TWSE,  │    HTTP / RSS / File   │  (DataProvider)  │
│   Fugle, etc.)   │                        └────────┬─────────┘
└─────────────────┘                                  │
                                                     ▼
                                         ┌───────────────────────┐
                                         │       Gateway          │
                                         │  · RateLimiter         │
                                         │  · CircuitBreaker      │
                                         │  · UnifiedHealthStore  │
                                         │  · CacheLayer (5m TTL) │  ← 記憶體快取
                                         └───────────┬───────────┘
                                                     │
                            ┌────────────────────────┼────────────────────┐
                            │                        │                    │
                            ▼                        ▼                    ▼
                  ┌──────────────────┐   ┌──────────────────┐   ┌────────────────┐
                  │ BackgroundTasks  │   │  CompositeMacro  │   │  Direct API    │
                  │ · macro_ingest   │   │    Provider      │   │  Consumers     │
                  │   (5m)           │   │ (mergeSnapshot)  │   │ · capital_flow │
                  │ · us_market      │   └────────┬─────────┘   │ · explain      │
                  │   _refresh_*     │            │             │ · stock_quote  │
                  │   (5m per ch)    │            ▼             └────────┬───────┘
                  │ · auto_* tasks   │   ┌──────────────────┐            │
                  └────────┬─────────┘   │ MacroDataSnapshot│            │
                           │             │  (65 fields)     │            │
                           ▼             └────────┬─────────┘            │
                  ┌──────────────────┐            │                      │
                  │ disk cache       │            │                      │
                  │ data/state/<ch>/ │            │                      │
                  │ latest.json      │            │                      │
                  └──────────────────┘            │                      │
                                                  ▼                      ▼
                                         ┌──────────────────────────────────┐
                                         │         HTTP API Layer           │
                                         │  GET /api/macro/snapshot/latest  │
                                         │  GET /api/capital-flow/daily     │
                                         │  GET /api/dashboard/us-indices   │
                                         │  GET /api/narrative/*            │
                                         │  GET /api/market/explain         │
                                         │  ... (50+ endpoints)             │
                                         └──────────────┬───────────────────┘
                                                        │
                                                        ▼
                                         ┌──────────────────────────────────┐
                                         │         Frontend SPA             │
                                         │  client_web / admin_web          │
                                         │  (page-shell pattern, 30s refresh)│
                                         └──────────────────────────────────┘
```

### 2.1 快取層次

| 層級 | 位置 | TTL | 機制 |
|------|------|-----|------|
| L1：Gateway in-memory | `CacheLayer`（`cache_registry.go`） | 5 min | `Fetch()` 自動讀寫 |
| L2：Provider local cache | 各 adapter 內部（如 `usCache`, `twiiCache`） | 隨 provider | 減少同 channel 重複 HTTP fetch |
| L3：Disk snapshot | `data/state/<channelID>/latest.json` | 永久（覆寫） | `saveSnapshot()` 於 adapter 寫入 |
| L4：DB history 表 | PostgreSQL（`regime_history`, `stress_index_history` 等） | 永久 | 歷史紀錄，見 §3 寫入路徑 |

### 2.3 TAIEX / `historical_volatility` 韌性語義

1. **`taiex_index` 雙源鏈路**：
   - 主源：Yahoo Finance `^TWII` chart（3 個月日線）。
   - 備援：TWSE OpenAPI `exchangeReport/MI_INDEX?type=IND`，僅在 Yahoo 失敗時啟動。
   - 備援回應須與請求交易日一致；週末/休市/日期不符者一律拒絕，不寫入陳舊值。
   - 兩路皆敗時，`TAIEX` 欄位為零值，`taiex_index` 進入 `FailedChannels`。

2. **`tw_vol` 陳舊快取守門**：
   - 與 `taiex_index` 共用 `twiiCache`（60s TTL）。
   - cache hit 時檢查 `^TWII` 資料時間戳是否為**當前交易日**；若為前一日或更舊，視為失敗，不會用舊收盤價計算 20 日年化波動率。

3. **snapshot 層級狀態**：
   - `MacroDataSnapshot.DataStatus` 由 `macroDataGatewayAdapter.fetchFresh` 綜合：
     - `ok`：全部 channel 成功。
     - `degraded`：有 channel hard-fail（`FailedChannels` 非空）。
     - `stale`：僅有 stale-only channel（`StaleChannels` 非空，無 hard-fail）。
   - 這些欄位會隨 snapshot 寫入 `latest.json`，API 與 dashboard 可見。

1. **macro_ingest**（5m）：`BackgroundTaskManager` → `DashboardAPI.IngestAndUpdateMacro()` →
   逐 channel 呼叫 `gateway.Fetch()` → merge 為 `MacroDataSnapshot` →
   更新 JANUS regime engine + 寫入 `regime_history`/`period_history` DB

2. **us_market_refresh_***（5m per ch，共 8 個 Yahoo）：獨立 BTM 任務，各自 `gateway.Fetch()` →
   provider cache populated → `macro_ingest` 取用時 cache hit

3. **macro_cache_***（5-15m，6 個）：為 `macro_ingest` 批次閉包中無獨立 BTM 的通道
   提供預先 cache warm，涵蓋 dram_spot_price、tw_vol、bdi、frankfurter_fx、
   sector_data、twse_sector_index

4. **auto_capital_flow / auto_margin**（30m）：交易時段內透過 `gateway.Fetch()` 更新
   資金流向與融資融券資料，寫入 `data/state/` disk cache

---

## 3. 寫入路徑清單

> 所有會寫入 DB / event store / disk state 的入口。此清單用於防止 warmup 類副作用事故。

### 3.1 持久化架構總覽

atlas-go 有三類持久化後端：

| 後端 | 位置 | 說明 |
|------|------|------|
| **PostgreSQL** | `internal/repository/`（`DualWriteRepository` 門面） | 生產 DB：alerts、metrics、outcomes、sessions、capital flow、task executions |
| **SQLite Ledger** | `internal/ledger/`（`getSharedSQLiteDB`） | 19 張表的本地 ledger：simulation/experiment 完整記錄 |
| **File Ledger / JSONL** | `{ledgerDir}/`、`data/state/`、`configs/` | 檔案型備援（JSONL session records、baseline JSON、parameter snapshots） |

**雙寫邊界**：`DualWriteRepository`（`internal/repository/dual_write.go`）先寫 JSONL/file store，
PG 可用時再寫 PG。**不完全雙寫**：`RecordSessionOutcomes`、`RecordExperiment`、
`RecordSessionExperiment` 目前只走 JSONL ledger，不寫 PG。

### 3.2 PostgreSQL 寫入（`internal/repository/`）

| 寫入路徑 | 檔案 | 表 | 操作 |
|---------|------|-----|------|
| Alert | `postgres_alerts.go` | `alerts` | UPSERT / UPDATE ack/resolve |
| Screening rejects | `postgres_audit.go` | `screening_rejects` | batch INSERT |
| Session summary | `postgres_audit.go` | `session_summaries` | UPSERT |
| Human intervention | `postgres_audit.go` | `human_interventions` | INSERT（衝突忽略） |
| Metrics | `postgres_metrics.go` | `metrics` | INSERT per indicator |
| Outcomes | `postgres_outcomes.go` | `recommendation_outcomes` | batch INSERT |
| Capital flow | `postgres_others.go` | `capital_flow` | INSERT |
| Export statistics | `postgres_others.go` | `export_statistics` | DELETE year/month + INSERT replacements（同一 transaction） |
| Task execution | `postgres_task_execution.go` | `task_executions`, `task_execution_events` | INSERT / UPDATE / batch metric_trends |
| Experiment lineage | `postgres_task_execution.go` | `experiment_lineage` | UPSERT |
| Baseline history | `postgres_task_execution.go` | `baseline_history` | INSERT |
| Channel health | `internal/apigateway/channel_health.go` | `channel_health` | UPSERT |

### 3.3 SQLite Ledger 寫入（`internal/ledger/`）

| 寫入路徑 | 檔案 | 表 | 操作 |
|---------|------|-----|------|
| Outcomes | `outcome_store_sqlite.go` | `outcomes` | transaction batch INSERT |
| Screening rejects | `outcome_store_sqlite.go` | `screening_rejects` | transaction batch INSERT |
| Trades | `outcome_store_sqlite.go` | `trades` | transaction batch INSERT |
| Experiments | `outcome_store_sqlite.go` | `experiments` | INSERT（global 或 session-scoped） |
| Session summaries | `outcome_store_sqlite.go` / `session_store_sqlite.go` | `session_summaries` | UPSERT |
| Human interventions | `outcome_store_sqlite.go` | `human_interventions` | INSERT |
| Quotes | `quote_store_sqlite.go` | `quotes` | `INSERT OR REPLACE` |
| Regime history | `historical_store.go` | `regime_history` | UPSERT（`ON CONFLICT(date) DO UPDATE`） |
| Period history | `historical_store.go` | `period_history` | UPSERT |
| Stress index history | `historical_store.go` | `stress_index_history` | UPSERT |
| Geopolitical history | `historical_store.go` | `geopolitical_history` | UPSERT |
| Event calendar history | `historical_store.go` | `event_calendar_history` | UPSERT（`ON CONFLICT(date, event_id)`） |
| Prediction backtest | `historical_store.go` | `prediction_backtest` | UPSERT |
| Detector scan log | `detector_scan_store.go` | `detector_scan_log` | transaction INSERT |
| Spawn records | `store_factory.go` | `spawn_records` | INSERT |
| Prompt experiment results | `store_factory.go` | `prompt_experiment_results` | INSERT / UPSERT |
| Window summaries | `store_factory.go` | `window_summaries` | INSERT |
| Mutation briefs | `store_factory.go` | `mutation_briefs` | INSERT |

### 3.4 事件持久化

系統中**沒有名為 `EventStore` 或 `event_store` 的 production 型別/資料表**。
事件持久化實作於兩處：

| 路徑 | 機制 | 格式 |
|------|------|------|
| `{ledgerDir}/events.jsonl` | Global event bus audit subscriber | JSONL append |
| PG `task_execution_events` | `TaskExecutionStore.AppendEvent` | INSERT per event |

`ChannelEventBus.Publish` 本身僅進記憶體 channel；只有啟用 audit subscriber 才落盤。

### 3.5 檔案型 State 寫入

| 入口 | 路徑 | 內容 |
|------|------|------|
| `saveSnapshot()` | `data/state/<channelID>/latest.json` | 各 channel 最新 fetch 結果 |
| `TWSECapitalFlowProvider` | `data/state/capital_flow/` | 資金流向 JSONL |
| `TWSEMarginBalanceProvider` | `data/state/margin/` | 融資融券 JSONL |
| `SectorDataProvider` | `data/state/sector_data/` | 產業分類 |
| `GovernmentFlowProvider` | `data/state/government_flow/` | 官股行庫 CSV |
| `ExportStatisticsProvider` | `data/state/export/` | 出口統計 |
| Replay data | `data/replay/` | 歷史 replay JSONL |
| Baseline policy | `configs/baseline_policy.json` | baseline 配置 atomic write（lock + rollback） |
| Darwinian weights | `data/state/darwinian_weights.json` | 演化權重 snapshot |
| Parameter snapshots | `configs/parameters_snapshots/` | 參數歷史快照 |

### 3.6 Experiment / Baseline / Simulation 寫入

| 寫入 | 檔案 | 說明 |
|------|------|------|
| Candidate prompt | `internal/experiment/executor.go` | 產生 experiment prompt 檔案 |
| Experiment record | `internal/experiment/executor.go` → ledger | 寫 `ExperimentRecord` + `PromptExperimentResult` |
| Auto brief | `internal/experiment/auto.go` | append-only JSON brief |
| TTL expired | `internal/experiment/ttl.go` | 直接覆寫過期 experiment result JSON |
| Judge/Promote | `internal/experiment/auto_judge_promoter.go` | 暫存 result JSON → promote 後移除 |
| Baseline policy | `internal/baseline/policy.go` → `SaveWithLock` | locked atomic write + rollback |
| Simulation traces | `internal/orchestrator/` | `Scratchpad.Record` WAL + `SimTraceWriter.ExportJSONL` |

### 3.7 Control Override 寫入（admin API）

Control override **不直接修改 mutable table**，而是 append-only human intervention：

1. 所有 control mutation（pause/resume/weight/ban/approve/reject）建立 `HumanIntervention`
2. 經 `ControlService.RecordIntervention` → `ledger.OutcomeStore.RecordHumanIntervention`
3. SQLite: INSERT `human_interventions`；JSONL: append intervention record
4. `GetActiveOverrides` 每次重播 intervention ledger，依 pause/resume、ban/unban、expiry 計算當前狀態

| Endpoint | 檔案 |
|---------|------|
| `POST /api/control/pause-agent` | `internal/monitoring/api/control/handlers.go` |
| `POST /api/control/resume-agent` | 同上 |
| `POST /api/control/set-model-weight` | 同上 |
| `POST /api/control/sector-ban` | 同上 |
| `POST /api/experiment/promote` | `internal/monitoring/api/experiment/handlers.go` |
| `POST /api/experiment/revert` | 同上 |
| `POST /api/experiment/judge` | 同上 |
| `POST /admin/trigger-simulation` | `cmd/atlas/main.go` |

### 3.8 Subscription Store（獨立 SQLite）

| 寫入 | 檔案 | 說明 |
|------|------|------|
| User register | `internal/subscription/store.go` | INSERT `users` + INSERT `subscription_events` (registered/trial) |
| Login | 同上 | INSERT `subscription_events` (login) |
| SetTier | 同上 | UPDATE `users` tier + INSERT `subscription_events` |

使用獨立 SQLite `users.db`（非 ledger DB）。`recordEvent` 刻意忽略寫入錯誤。

### 3.9 Bootstrap / Warmup 寫入

> 啟動時以下操作會寫入 DB/disk，非純讀：

| 步驟 | 寫入目標 | 說明 |
|------|---------|------|
| `bootstrap.InitDatabase` | PostgreSQL（全部 pending migration） | golang-migrate |
| LEDGER SQLite init | SQLite（`CREATE TABLE IF NOT EXISTS` + additive `ALTER TABLE`） | schema migration |
| Alert cleanup | File alert store | 清除舊 gateway heartbeat alerts |
| `dashboard.IngestAndUpdateMacro` | Ledger（regime_history, period_history 等）+ channel state | 首次 macro ingest |
| Darwinian weights init | `data/state/darwinian_weights.json` | `Save()` on init |
| Event bus bootstrap events | `events.jsonl`（若 audit subscriber 啟用） | 發布後落盤 |
---

## 4. 前端架構

### 4.1 三前端主從架構

```
client_web/                  admin_web/                  shared_web/
├── static/                  ├── static/                  └── static/
│   ├── index.html           │   ├── index.html               ├── js/
│   └── js/                  │   └── js/                      │   ├── pages/     ← 共 29 頁面
│       ├── main.js   ← SPA  │       └── main.js   ← SPA      │   ├── page-shells/ ← 10 shells
│       ├── page-shells/     │                                 │   ├── components/ ← 共用元件
│       │   ├── stock-quote.js    │                            │   ├── shared/     ← utils/tokens
│       │   ├── strategies.js     │                            │   └── modals/
│       │   └── ...               │                            └── css/
│       └── components/           │                                ├── base/
│           └── home-tier-sections.js│                            │   └── variables.css ← 設計 tokens
│                                  │                                ├── pages/
└── embed.go                      └── embed.go                     ├── components/
    (Go embed FS)                     (Go embed FS)                 └── layout/
                                                                       ├── page-shell.css
                                                                       ├── sidebar.css
                                                                       └── responsive.css
```

**原則**：
- `shared_web/` 為所有前端共享的頁面、元件、CSS 和工具函數。
- `client_web/` 為投資人端 SPA，含專屬 page-shell（stock-quote, strategies 等）。
- `admin_web/` 為管理端 SPA，僅含管理專屬頁面。
- 兩個端點各有專屬 `embed.go`，在 Go binary 中透過 `embed.FS` 靜態內嵌。

### 4.2 Page-Shell 模式

`client_web/static/js/main.js` 中的 `SHELL_LOADERS` 定義每個 page 的 shell loader：

```js
// client_web/static/js/main.js
const SHELL_LOADERS = {
  'home':             () => import('../../../shared_web/static/js/pages/home.js'),
  'strategies':       () => import('./page-shells/strategies.js'),
  'performance-report': () => import('./page-shells/performance-report.js'),
  'stock-quote':      () => import('./page-shells/stock-quote.js'),
  'methodology':      () => import('../../../shared_web/static/js/page-shells/methodology.js'),
  'narrative':        () => import('../../../shared_web/static/js/page-shells/narrative.js'),
  // ...
};
```

- **Page** = 共享頁面（`shared_web/static/js/pages/` 或 page-shells）
- **Shell** = 頁面骨架（`shared_web/static/js/page-shells/`），含 layout 與導航
- **component** = 可重用 UI 區塊（如 `seven-force-board.js`, `event-calendar.js`）
- **路由**：基於 `data-page` 屬性的 SPA 內部路由，無 URL hash 切換

`shared_web/static/css/layout/page-shell.css` 定義統一的 `.page` / `.page.active` 切換動畫。

### 4.3 設計 Tokens

- **位置**：`shared_web/static/css/base/variables.css`
- **內容**：CSS custom properties（`--text`, `--muted`, `--bg`, `--accent`, `--font-display`, `--space-lg` 等）
- **color-tokens.js**：JS 端 token 存取（`shared_web/static/js/shared/color-tokens.js`），供圖表使用

### 4.4 建置流程

- **Bundler**：`esbuild`（`client_web/esbuild.config.mjs`, `admin_web/esbuild.config.mjs`）
- **Shared plugin**：`shared_web/esbuild-shared-plugin.mjs` 處理跨 app 的 import paths
- **測試**：Playwright（`client_web/playwright.config.ts`, `admin_web/playwright.config.ts`）
- **嵌入**：`//go:embed static/*` 在 `client_web/embed.go` 和 `admin_web/embed.go`

### 4.5 自動刷新

`client_web/static/js/main.js` 每 30 秒呼叫 `loadAll()` 刷新首頁資料。
連續失敗 3 次後顯示 error banner；成功後自動恢復。

---

## 5. 架構化石區

> **勿踩**：以下區域為已知死碼/未接線引擎/stub——修改前需確認不影響現有行為。

| 化石 | 位置 | 說明 | 風險 |
|------|------|------|------|
| `twse_sbl` (G02) | `internal/marketdata/twse_sbl_provider.go` | STUB provider，回傳「endpoint not yet confirmed」 | 移除會讓 dashboard 消失此 channel |
| `tdcc_equity_dispersion` (G01) | `internal/marketdata/tdcc_provider.go` | STUB provider，回傳「API access not yet configured」 | 同上 |
| Unused parameters | `internal/config/defaults_portfolio.go` | `BetaRangeMin`, `BetaRangeMax`, `MinTradeSize`, `MinPositionSize`, `TargetBeta` 標示 unused | 可能未來實作 beta constraint |
| `boolParamAccessor` | `internal/config/param_table.go` | 未使用的 scaffolding，預留 bool 參數支援 | 無消費者 |
| Legacy health paths | `internal/monitoring/` vs `internal/apigateway/health.go` | 兩套健康檢查共存（UnifiedHealthStore + monitoring 自製 store） | 合併時注意兩者消費者 |
| `adversarial.GetVulnerabilities` | `internal/adversarial/AGENTS.md` | 明確標示「simplified stub」，不走真實 replay | 勿依賴其結果 |
| Deprecated `POST /api/macro/ingest` | `internal/monitoring/api/macro/handlers.go` | 手動 ingest trigger，已被 BTM 取代 | 僅保留相容性 |
| Orphan summaries | `internal/backfill/summaries.go` | 無 `summary.json` 的 session 重建工具 | 離線使用，非 hot path |
| `SignalEngine` (F08) | `internal/autobacktest/loop.go` | 原為 orphan，現由 autobacktest 消費 | 勿刪 |
| `NarrativeEngine` x2 | `cmd/atlas/main.go` vs `internal/monitoring/dashboard_api.go` | Dashboard 與 eventdriven detector 使用兩個不同的 engine instance | 模型/權重/即時 macro state 可能分岔 |

### 5.1 未接線通道（無獨立 BTM 排程）

以下通道無自動排程任務，資料更新依賴外部系統或手動觸發：

| channelID | 目前觸發方式 |
|-----------|------------|
| `day_trading` | 無 BTM 任務，可能依賴 `macro_ingest` 內部批次 |
| `market_volume` | 同 `day_trading` |
| `twse_oddlot` | 無 BTM 任務 |
| `twse_etf` | 無 BTM 任務（⚠️ 資料源已移除，2026-08-10 實測 TWT44U → 404） |
| `tdcc_equity_dispersion` | STUB，無排程 |
| `sector_data` | `macro_cache_sector_data` (15m) |

---

## 6. API 路由全覽

### 6.1 公開端點（無需認證）

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/health` | 系統健康檢查（含 port probe） |
| `GET` | `/api/health` | → 308 redirect to `/health` |
| `GET` | `/api/routes` | API 路由清單（curated） |
| `GET` | `/api/capital-flow/daily` | 七維錢潮雷達 daily |
| `GET` | `/api/capital-flow/summary` | 錢潮摘要 |
| `GET` | `/api/capital-flow/history` | 錢潮歷史 |
| `GET` | `/api/capital-flow/historical-snapshot/{date}` | 指定日期快照 |
| `GET` | `/api/market/explain` | 今日台股解說（需 auth） |
| `GET` | `/api/macro/snapshot/latest` | 最新宏觀快照 |
| `GET` | `/api/macro/snapshot/timeline` | 歷史宏觀快照 |
| `GET` | `/api/narrative/*` | 敘事事件/鏈/模型 |
| `GET` | `/api/dashboard/us-indices` | 美股即時指數 |
| `GET` | `/api/dashboard/data-channels` | 資料通道狀態 |
| `GET` | `/api/dashboard/system-health` | 系統健康 |
| `GET` | `/api/dashboard/risk-exposure` | 風險暴露 |
| `GET` | `/api/dashboard/performance-report` | 績效報告 |
| `GET` | `/api/dashboard/calendar-events` | → 308 redirect to `/api/events/calendar` |
| `GET` | `/api/events/calendar` | 事件日曆 |
| `GET` | `/api/events/predictions` | 未來 5 日資金預測（home 頁面用） |
| `GET` | `/api/regime/history` | 市場體制歷史（含 `market_period`） |
| `GET` | `/api/taiwan/stress-index` | TRJ 壓力指數 |
| `GET` | `/api/janus/regime-score` | JANUS 體制評分 |
| `GET` | `/api/strategies/{id}` | 策略詳情 |
| `GET` | `/api/strategies/{id}/attribution` | 策略歸因 |
| `GET` | `/api/strategies/{id}/summary` | 策略摘要 |
| `GET` | `/api/reports/latest` | 最新每日報告 |
| `GET` | `/api/stock/quote` | 個股報價（需 Fugle key） |
| `GET` | `/api/stock/technical` | 技術指標 |
| `GET` | `/api/stock/chips` | 籌碼資料 |
| `GET` | `/api/stock/fundamentals` | 基本面 |

### 6.2 管理端點（需 admin auth）

| Method | Path | 說明 |
|--------|------|------|
| `POST` | `/api/backtest/run` | 觸發回測 |
| `POST` | `/api/control/pause-agent` | 暫停 agent |
| `POST` | `/api/control/resume-agent` | 恢復 agent |
| `POST` | `/api/control/set-model-weight` | 調整策略權重 |
| `POST` | `/api/control/sector-ban` | 禁止產業 |
| `POST` | `/api/experiment/promote` | 提升實驗 |
| `POST` | `/api/experiment/revert` | 回退實驗 |
| `POST` | `/api/experiment/judge` | 評分實驗 |
| `POST` | `/api/dashboard/channels/` | 通道管理 |
| `POST` | `/api/dashboard/api-keys/update` | API key 更新 |
| `POST` | `/api/macro/ingest` | 手動觸發宏觀攝取（deprecated） |

### 6.3 MCP Server（atlas-mcp, stdio mode）

`cmd/atlas-mcp/` 透過 MCP protocol 暴露 80+ tools 給外部 AI agent，
涵蓋市場資料、策略、風險、實驗、控制、事件等全量功能。

---

## 7. 背景任務全覽

> 所有透過 `BackgroundTaskManager.Register()` 註冊的任務，含 channel 綁定與排程間隔。

完整任務清單分散於 `cmd/atlas/*_tasks.go`。主要類別：

| 類別 | 任務範例 | 間隔 |
|------|---------|------|
| **資料擷取** | `us_market_refresh_*`（8 個）、`auto_capital_flow`、`auto_margin` | 5m / 30m |
| **快取預熱** | `macro_cache_*`（6 個） | 5-15m |
| **宏觀攝取** | `macro_ingest` | 5m |
| **通道健康** | `channel_health_sync`、`channel_health_*`（6 個） | 5m / 1h |
| **校準** | `risk_gate_calibrate`、`factor_weight_calibrate`、`auto_calibrate` 等 | 24h / 7d |
| **模擬** | `auto_daily_simulation`、`auto_experiment`、`auto_propose` | 24h / 7d |
| **回測** | `window_backtest`、`autobacktest_daily` | 7d / 1h |
| **維護** | `storage_cleanup`、`fundamentals_staleness_check` | 24h |
| **事件** | `auto_calendar_refresh`、`sync-events-daily` | 24h / 1m |
| **即時** | `realtime_feed` | 30s |
| **進化** | `auto_strategy_evolution`、`auto_judge_promoter` | 24h |

---

## 8. 快速參考

- **新增資料通道 SOP**：`internal/apigateway/CONSTITUTION.md`（6 條憲法）
- **通道註冊兩處**：`limits.go`（限流）+ `gateway.go:channelIDs()`（列舉）
- **Adapter 模式**：`DataProvider` interface → HTTP / File / Compute 三種實作
- **熔斷參數**：3 次連續失敗 → 5 分鐘 Open → 2 次 HalfOpen 探測 → Close
- **快取 TTL**：Gateway in-memory 5 min（`CacheLayer`）
- **API 架構**：`internal/monitoring/api/<domain>/handlers.go` 各子包 RegisterRoutes 模式
- **前端路由**：`data-page` attribute → `switchPage()` → 動態 import shell loader
