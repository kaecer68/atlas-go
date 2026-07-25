# A2 — Scheduled / Cron / Background Tasks Audit

**Audit date**: 2026-07-25
**Slice**: A2 — navigator of every scheduled/cron/background task that fetches or processes external data
**Scout**: SchedulerScout

---

## TL;DR

| 類別 | 數量 | 範例 |
|---|---|---|
| `BackgroundTaskManager.Register` 註冊 | **80** | `auto_capital_flow`, `auto_taiex_index`, `ml_retrain`, `prism_training`, `sync-events-daily` … |
| Scheduler inner task (成熟度閘門) | 1 | `narrative_weight_calibration` (在 `BackgroundCalibrationScheduler` 內,被 `calibration_cycle` 驅動) |
| Docker cron containers | 10 | `atlas-cron-geo-ingest`, `atlas-cron-replay-sync`, `atlas-cron-quote-backfill` … |
| Docker long-running worker | 1 | `atlas-prism-worker` (always-on daemon,非 cron) |
| Unregistered `time.NewTicker` 路徑 | 16 | `RealTimeAdapter`, `live.Scheduler`, `monitoring.RuleEngine`, `metaLearner` … |
| one-shot CLI (無 background schedule) | 75 | `backfill-*`, `calibrate-*`, `run-experiment`, `fubon-dma`, `atlas-mcp` … |
| **總 surface** | **≈ 174** | (註冊圖 +90 / CLI 工具 +75 / Docker cron +10 / long-running worker +1) |

---

## 1. `BackgroundTaskManager` 註冊總覽 (`internal/apigateway/background.go`)

`BackgroundTaskManager` 是一個集中式排程器,內含 `gateway.Fetch`、jitter、overlap 保護、半開熔斷探測,
與 Constitution Article 4 強制約束配套:
> 禁止裸 goroutine 定時任務;所有排程必須用 `BackgroundTaskManager`。

註冊 **80** 個任務 (依 module 分組):

| 模組 / 檔案 | 數量 | 範例任務 | 排程 |
|---|---|---|---|
| `cmd/atlas/operations_tasks.go` | 12 | `macro_ingest`, `realtime_feed`, `prism_training`, `government_flow_aggregate` | 30s / 60s / 5m / 6h / 10m / 12h / 24h / 28h |
| `cmd/atlas/data_sync_health_tasks.go` | 13 | `auto_taiex_index`, `channel_health_fugle`, `tsmc_revenue`, `health_check`, `us_market_refresh` | 30s / 1m / 5m / 6h / 1h / 24h |
| `cmd/atlas/capital_tasks.go` | 10 | `auto_capital_flow`, `auto_margin`, `auto_export`, `auto_taifex_institutional`, `auto_twse_sbl` | 30m / 1h / 6h / 12h / 24h |
| `cmd/atlas/calibration_tasks.go` | 17 | `calibration_cycle`, `auto_strategy_evolution`, `regime_calibrate`, `auto_calibrate`, `rsi_tw_calibrate` | 1h / 6h / 24h / 7d |
| `cmd/atlas/stage3_tasks.go` | 8 | `sync-events-daily`, `sync-macro-daily`, `sync-capital-daily`, `sync-regime-weekly`, `stage3-alert-*` | 1m (with TPE hour-gate) / 5m |
| `cmd/atlas/main.go` (residual) | 14 | `auto_daily_simulation`, `auto_experiment`, `ml_retrain`, `daily_report_generate`, `auto_universe_refresh`, `janus_regime_refresh`, `tej_refresh`, `autobacktest_daily`, `rule_engine_check` … | 1m / 1h / 24h / 7d / config |
| `internal/scheduler/template_detector_scan.go` | 1 | `template_detector_scan` | 1h |
| `internal/scheduler/narrative_update.go` | 1 | `narrative_weight_update` | 1h |

> 完整逐項含 `ChannelID` / `line_numbers` 對照表見 `a2-tasks.json` `registered_tasks.background_task_manager` 區段。

### 重要 Channel 對應

`ScheduledTask.ChannelID` 設定 → 強制走 `gateway.Fetch(ChannelID)` 路徑 (CONSTITUTION Art 4.4)。

| Channel | 任務 | 排程 |
|---|---|---|
| `twse_capital_flow` | `auto_capital_flow`, `sync-capital-daily` | 30m / 1m-13:30 |
| `twse_margin` | `auto_margin`, `margin_history_backfill` | 30m / 24h |
| `export_statistics` | `auto_export` | 12h |
| `taifex_institutional` | `auto_taifex_institutional` | 1h |
| `twse_sbl` | `auto_twse_sbl` (STUB G02) | 1h |
| `government_flow` | `auto_government_flow` | 1h |
| `geopolitical` | `auto_geopolitical` | 6h |
| `tej` | `tej_refresh` | 1h |
| `janus_regime` | `janus_regime_refresh` | 6h |
| `taiex_index` | `auto_taiex_index` | 1h |
| `exchange_rate` | `auto_exchange_rate` | 1h |
| `geopolitical_taiwan` | `auto_geopolitical_taiwan` | 6h |
| `taifex_daily` | `auto_taifex_daily` | 1h |
| `twse_replay` | `etf_nav_refresh`, `auto_backfill`, `channel_health_twse_replay` | 24h / 24h / 1h |
| `tsmc_revenue` | `tsmc_revenue` | 24h |
| `fugle` / `fubon` / `finmind` | `channel_health_*` | 1h each |

### 1m interval + TPE hour-gate 的小時級任務

`stage3_tasks.go` 與 `main.go` 用「每分鐘跑一次,內部 guard 判斷小時分鐘」的模式達成 06:00 / 06:30 / 13:30 / 13:45 / 14:00 / 08:00(Mon) / 1st 08:00 等精準時點觸發,且 1 分鐘 tick 上限 vs 1 小時 tick 是有意識的取捨 (overlap 保護允許 1 秒 tolerance,1m tick 不會被跳過)。

---

## 2. Scheduler inner task (1 個)

`internal/scheduler/auto_calibration.go` 提供了 `BackgroundCalibrationScheduler` (成熟度閘門排程器,
對應 `CalibrationTask` 切片)。**生產環境目前未直接使用**:
- `cmd/atlas/calibration_tasks.go:110` 創建了 `calScheduler` 但把 1 個 inner `narrative_weight_calibration` 註冊進去
- 然而 outer `calibration_cycle` 透過 `BackgroundTaskManager` 觸發後,呼叫的是 `calTask.Run(...)` (`cmd/atlas/calibration_tasks.go:120`)
- 換言之,`BackgroundCalibrationScheduler.Register` 介面已存在但 `RunDaily` 的 caller 鏈目前在 main.go 內直接 `enabled` 與否判斷;scheduler 物件本身只承載 inner task 註冊

> 註:`seasonal_calibrate` (PR10b 移除) 與 `NewSeasonalCalibrationTask` (deprecated since PR10b) 都不在 active set 內。

---

## 3. Docker cron containers (10 個)

共用同一個 `Dockerfile.cron` image(= 10 個 `cron-*:latest` 與 `atlas-cron-*:latest` tag),
執行 `scripts/cron-entrypoint.sh` (60s 輪詢自製 cron;取代 dcron 避免 macOS Docker Desktop seccomp setpgid 阻塞):

| Container | Schedule | CRON_COMMAND | 對應 cmd entrypoint |
|---|---|---|---|
| `atlas-cron-geo-ingest` | `0 8 * * *` | `/app/geo-ingest -work-dir /app` | `cmd/geo-ingest/main.go` |
| `atlas-cron-macro-ingest` | `0 8 * * *` | `/app/macro-ingest -work-dir /app` | `cmd/macro-ingest/main.go` |
| `atlas-cron-replay-sync` | `30 15 * * *` | `/app/daily-replay-sync -csv /app/data/replay/tw_extended_90days.csv` | `cmd/daily-replay-sync/main.go` |
| `atlas-cron-darwinian` | `0 9 * * *` | `/app/scripts/darwinian_adjust.sh --apply` | `scripts/darwinian_adjust.sh` |
| `atlas-cron-c07-collect` | `30 15 * * 1-5` | `/app/scripts/c07-collect.sh` | `scripts/c07-collect.sh` → `c07-obs-collector` |
| `atlas-cron-c07-evaluate` | `0 9 * * 1-5` | `/app/scripts/c07-evaluate.sh` | `scripts/c07-evaluate.sh` → `c07-day-evaluator` |
| `atlas-cron-backfill-month-revenue` | `0 0 1 * *` | `/app/backfill-month-revenue -start $(date -d '2 months ago' +%Y-%m-01) -end $(date -d 'last month' +%Y-%m-%d)` | `cmd/backfill-month-revenue/main.go` |
| `atlas-cron-backfill-financial-statements` | `0 0 1 * *` | `/app/backfill-financial-statements -start ...` | `cmd/backfill-financial-statements/main.go` |
| `atlas-cron-backfill-institutional-investors` | `0 2 * * *` | `/app/backfill-institutional-investors -date $(date -d 'yesterday' +%Y-%m-%d)` | `cmd/backfill-institutional-investors/main.go` |
| `atlas-cron-quote-backfill` | `0 3 * * *` | `/app/cron-quote-backfill` | `cmd/cron-quote-backfill/main.go` |

`scripts/cron-entrypoint.sh` 邏輯:`while true; do cron_match; sleep 60; done`,每分鐘比對 5 個 cron 欄位 (`min hour day month weekday`),支援 `*` / `1,3,5` / `1-5`。

### Dockerfile.cron 額外編譯但未接 cron 排程的 binary

`Dockerfile.cron` 還 build 了 `backfill-replay` 與 `c07-obs-collector` / `c07-day-evaluator`,
但目前 `docker-compose.yml` 沒有對應 `cron-backfill-replay` 容器 — 屬於歷史遺留 baseline,可考慮清理或在後續 PR 補上對應服務。

---

## 4. Docker long-running worker (1 個)

| Container | 入口 | 模式 |
|---|---|---|
| `atlas-prism-worker` | `atlas-go prism worker` | PRISM 訓練佇列 long-lived daemon;`cmd/atlas/main.go:337` 的 `runPrismWorker` 啟動後永久阻塞至 SIGINT |

`prism-worker` 不是 cron schedule,它是「always-on」背景 container。

---

## 5. Unregistered `time.NewTicker` 路徑 (16 個)

這 16 個直接 `time.NewTicker` 啟動的 goroutine 沒有走 `BackgroundTaskManager`,
但多半有正當理由 (BEFORE-pipeline 元件 / socket keep-alive / 監控內部)。
**它們不算「對外資料抓取排程」**,但若要做完整 audit 仍應登錄在案:

| 來源 | 用途 | 排程 |
|---|---|---|
| `internal/prism/prism_manager.go:563` | `autoBalancer` 5m 排程 | 5m |
| `internal/metalearning/metalearner.go:292` | `adaptationLoop` 策略族群演化 | configurable |
| `internal/realtime/regime_adapter.go:268` | `RealTimeAdapter.Start` 即時 regime 偵測 | `UpdateInterval` (sub-second) |
| `internal/live/scheduler.go:115` | `quotePoller` 即時報價輪詢 | `QuotePollInterval` |
| `internal/live/scheduler.go:170` | `intradayProcessor` 盤中循環 | `IntradayInterval` |
| `internal/monitoring/rules.go:55` | `RuleEngine.Start` 規則引擎 | `checkInterval` |
| `internal/monitoring/service/channel_health_synthesizer.go:65` | 通道健康聚合 | `interval` |
| `internal/monitoring/service/drift_detector.go:104` | 模型漂移偵測 | `DriftCheckInterval` |
| `internal/monitoring/service/ingestion_lag_monitor.go:66` | 通道延遲監控 | `interval` |
| `internal/monitoring/service/regime_debouncer.go:80` | regime 轉換 debouncer | `RegimeDebounceCheckInt` |
| `internal/mcp/anomaly/emitter.go:118` | anomaly emitter | `cfg.Interval` |
| `internal/marketdata/realtime/fugle_ws.go:409` | WebSocket ping keep-alive | `pingPeriod` |
| `internal/marketdata/realtime/router.go:232` | quote router 聚合 | `period` |
| `internal/marketdata/fubon_client.go:326` | fubon-proxy health probe | `interval` |
| `internal/marketdata/streaming.go:26` | quote polling adapter | `Interval` |
| `internal/fubonproxy/manager_test.go:1861` | 測試 race detector (test-only) | 2s `time.AfterFunc` |
| `internal/config/filelock.go:61` | file lock 短期 spinlock | 10ms |

> 註: `L2.4 auto-cron` (`internal/scheduler/l2_4_auto_cron.go`) 屬於刻意「不註冊」的 case,
> 程式碼區塊 1-30 自行說明:prereqs (followup.md §1) 達成後才會真正 `Register()`;`ShouldL24AutoCronFire()` 5-condition gate 必須全部通過才放行。

---

## 6. One-shot CLI 看起來像 cron 的工具 (75 個)

由於 `cmd/` 底下有極大量 one-shot CLI,即使功能上「適合週期跑」(如 `backfill-*`, `calibrate-*`),
它們**沒有 `BackgroundTaskManager` 註冊**,需要外部 cron / k8s CronJob / 手動觸發。

範例分組:

- **Backfill (12)**: `backfill-month-revenue`, `backfill-financial-statements`, `backfill-institutional-investors`, `backfill-quotes`, `backfill-var-returns`, `backfill-taifex-oi`, `backfill-summaries`, `backfill-fundamentals-ps-sector`, `backfill-industry-tree`, `backfill-replay`, `merge-replay`, `import-replay`, `fetch-historical`, `fetch-historical-capital-flow`, `extend-replay-etf`
- **Calibrate (8)**: `calibrate-baselines`, `calibrate-rsi-tw`, `calibrate-stress-index`, `calibrate-parameters`, `calibrate-seasonal` (BTM 之子行程), `calibrate-thresholds`, `calibration-validate`, `optimize-parameters`
- **Convert (4)**: `convert-baseline-policy`, `convert-experiment-results`, `convert-experiments-jsonl`, `convert-recommendation-outcomes`
- **Check / Cleanup (4)**: `check-data-health`, `check-maturity`, `check-persistence-format`, `cleanup-channel-health`
- **Migrate (2)**: `migrate-data`, `migrate-jsonl-to-sqlite`
- **Validate (5)**: `validate-parameters`, `validate-stress-index`, `validate-twse-capital-flow`, `validate-risk-gate`, `validate-broker`
- **Experiment (3)**: `run-experiment`, `judge-experiment`, `promote-baseline`, `revert-baseline`
- **Backtest (4)**: `backtest-event-flow`, `backtest-pipeline`, `backtest-window`, `c07-preflight`
- **Codegen (5)**: `gentags`, `mapgen`, `atlas-mcp-setup`, `atlas-mcp/descgen`, `lint-prompts`, `lint-pr`
- **Long-lived but not periodic (4)**: `atlas-mcp` (HTTP+stdio 伺服器), `realtime-quote` (WebSocket CLI), `fubon-dma` (券商互動), `stress-test`
- **Stage 4 (2)**: `atlas-stage4-loader`, `atlas-stage4-backfill`
- **Plugin / experimental (10)**: `plugin-poc`, `plugin-e2e`, `test-hybrid`, `janus-status`, `janus-backtest`, `c07-spot-check-recorder`, `sector-allocation-closure-preflight`, `staging-drill-strategy-techniques`, `experimental/stress-test`, `experimental/c07-obs-collector`, `experimental/c07-day-evaluator`
- **Other (3)**: `archive-state`, `parameter-health-check`, `atlas-mcp-setup`

> 完整 75 行列在 `a2-tasks.json` `one_shot_cli_lookalikes` 區段。

---

## 7. 群組總人數圖

```
[BackgroundTaskManager]
├─ operations_tasks.go          12
├─ data_sync_health_tasks.go    13
├─ capital_tasks.go             10
├─ calibration_tasks.go         17
├─ stage3_tasks.go               8
├─ main.go (residual)           14
├─ internal/scheduler/template_detector_scan.go  1
└─ internal/scheduler/narrative_update.go         1
                                                    ---> 76 tasks (cron-scheduled, jitter+overlap)
BackgroundCalibrationScheduler inner tasks:
└─ narrative_weight_calibration                   1
                                                    ---> 1 maturity-gated inner task

Docker cron containers (10) + prism-worker (1)      ---> 11 separate processes

Unregistered time.NewTicker (16)                   ---> ad-hoc goroutines (non-fetch mostly)

One-shot CLI under cmd/ = 75                       ---> manual / external cron candidates
```

---

## 8. Channel coverage 對齊 (`internal/apigateway/register_adapters.go`)

`register_adapters.go` 註冊約 38 個 channels (含 stub)。Channel 與 BTM 任務的對應:

| Channel | Adapter 已註冊 | BTM 任務 |
|---|---|---|
| `twse_replay` | ✅ | `etf_nav_refresh`, `auto_backfill`, `channel_health_twse_replay` (3) |
| `twse_capital_flow` | ✅ | `auto_capital_flow`, `sync-capital-daily` (2) |
| `twse_margin` | ✅ | `auto_margin`, `margin_history_backfill` (2) |
| `export_statistics` | ✅ | `auto_export` (1) |
| `taifex_daily` | ✅ | `auto_taifex_daily` (1) |
| `taifex_institutional` | ✅ | `auto_taifex_institutional` (1) |
| `tej` | ✅ | `tej_refresh` (1) |
| `tsmc_revenue` | ✅ | `tsmc_revenue` (1) |
| `geopolitical` | ✅ | `auto_geopolitical` (1) |
| `geopolitical_taiwan` | ✅ | `auto_geopolitical_taiwan` (1) |
| `fugle` / `fubon` / `finmind` | ✅ | `channel_health_fugle` / `fubon` / `finmind` (各 1) |
| `taiex_index` | ✅ | `auto_taiex_index` (1) |
| `exchange_rate` | ✅ | `auto_exchange_rate` (1) |
| `twse_sbl` | ✅ (STUB G02) | `auto_twse_sbl` (1, 持續 healthCheck=inactive) |
| `janus_regime` | ✅ (optional) | `janus_regime_refresh` (1) |
| `frankfurter_fx`, `us_yahoo`, `us_spx`, `us_ndx`, `us_dji`, `us_nvda`, `us_aapl`, `us_msft`, `tsm_adr`, `dram_spot_price`, `twse_sector_index`, `bdi`, `sox_index`, `tw_vol` | ✅ | **❌ 無 BTM 任務** — 僅在 `macro_ingest` 內部 closure 透過 `MarketDataProvider` 拉取 |
| `sector_data`, `day_trading`, `twse_oddlot`, `twse_etf` | ✅ (no-auto-fetch) | ❌ |
| `tdcc_equity_dispersion` | ✅ (STUB G01) | ❌ (auto-fetch not scheduled) |

> 結論:14 個 channels **僅在 `macro_ingest` / `realtime_feed` 內部 closure 觸發**,
> 沒有獨立的 BTM 任務。`fubon` / `finmind` / `fugle` 屬於付費通道,只用於健康檢查,
> 真正的下游資料是 `twse_replay` / `finmind` 提供的歷史回放。

---

## 9. 重要發現 / 觀察

1. **`BackgroundTaskManager.Start` 之後無 hot-reload**:task 註冊全部在 `main.go` 啟動期完成 (`taskMgr.Start(sysCtx)` at `cmd/atlas/main.go:1885`)。要新增任務需改程式碼重啟 atlas container。
2. **2 個 1m-tick + hour-gate 任務 (`sync-macro-daily` / `sync-capital-daily`) 都對 `twse_capital_flow` channel**:`sync-capital-daily` 透過 closure 內 `d.gateway.Fetch(ctx, "twse_capital_flow")` 走 Gateway,但 `ScheduledTask.ChannelID` 欄位留空 — 跳過 `Register()` 階段的 channel 存在性檢查 (CONSTITUTION 4.4 是契約式約束,不是建構式約束)。
3. **stage3 1m-tick 模式優缺**:overlap tolerance 允許 1s 內重疊,所以 1m 觸發與 1h 觸發相比不會被 over-skip;ticker 風暴但 closure 內 99% 時間只做 `now.Hour() == 6 && now.Minute() == 30` 之類的判斷,成本可忽略。
4. **`start.go` 的 `govFlow` / `margin` / `export` / `sbl` / `replay` / `capital_flow` health 修復任務存在** (`FixGovFlowHealth` / `FixTwseCapFlowHealth` / 等 sonic agents),但這些是 agent 層級的維護,不是 cron task。
5. **`monitoring.Monitor.AutoHandler` 失敗/恢復 handler** 在 `setupBackgroundTaskManager` (`cmd/atlas/background_tasks.go:19`) 注入,consecutiveFailures >= 3 觸發 alert flow — 這表示所有 BTM 任務失敗都會進 atlas 主監控。
6. **`scripts/darwinian_adjust.sh` 與 `c07-collect.sh` / `c07-evaluate.sh` 是 bash 腳本** (非 Go binary),但其內部呼叫對應的 Go binary (`c07-obs-collector` / `c07-day-evaluator`)。`darwinian_adjust.sh` 純 bash 處理 darwinian weights JSON。
7. **cron container 共用同一 image**: Dockerfile.cron 一次 build = 10 個 cron container 用同一個 image (差異在 `CRON_SCHEDULE` / `CRON_COMMAND` env)。
8. **Dockerfile.cron 多 build 出的 `backfill-replay` binary 目前無人呼叫**,對應容器服務沒在 docker-compose.yml 註冊 (`backfill-replay` 屬於歷史遺留,可考慮清理或補上對應 cron 容器)。

---

## 10. Audit 結論對下游切片啟示

- **A3 (channel health / monitoring path)**:76 個 BTM 任務中,**只有 12 個有 `ChannelID` 透過 Gateway 走完整 health path**,其餘 64 個 (含 Stage 3 / strategy / calibration) 走 closure 內部 fetch,**不享受 circuit breaker / rate-limit / 統一 FetchResult 結構**。
- **A4 (alert path)**:80 個 BTM 任務的 `consecutiveFailures` 都會被 `SetFailureHandler` 收集,但只有 14 個寫進 `data_health` 欄位 (`LastDataAsOf` / `LastNewSamples` / `NoProgressReason`) — `SetDataHealth` 是 optional,沒強制。
- **A5 (compliance)**:Constitution Art. 4 的 `Register` 規約要求 ChannelID 必填 + channel 必須存在;若 closure 內手動 `gateway.Fetch` 沒宣告 ChannelID,等於繞過契約 — 這是 64 個任務目前的實際狀態,合規風險。

---

## 11. 來源檔案清單

- `internal/apigateway/background.go` (BackgroundTaskManager type)
- `internal/apigateway/register_adapters.go` (Channel 註冊)
- `internal/apigateway/CONSTITUTION.md` (Art. 4 排程規約)
- `cmd/atlas/main.go` (main shell + 14 inline `Register` 殘餘)
- `cmd/atlas/operations_tasks.go` (12 tasks)
- `cmd/atlas/data_sync_health_tasks.go` (13 tasks)
- `cmd/atlas/capital_tasks.go` (10 tasks)
- `cmd/atlas/calibration_tasks.go` (17 tasks + 1 inner CalibrationTask)
- `cmd/atlas/stage3_tasks.go` (8 tasks)
- `cmd/atlas/background_tasks.go` (failure/recovery handler)
- `internal/scheduler/doc.go` (scheduler package 規約)
- `internal/scheduler/auto_calibration.go` (BackgroundCalibrationScheduler)
- `internal/scheduler/template_detector_scan.go` (1 task)
- `internal/scheduler/narrative_update.go` (1 task)
- `internal/scheduler/l2_4_auto_cron.go` (gate-only, deferred)
- `internal/scheduler/ml_retrain.go` (closed-over via main.go)
- `internal/scheduler/auto_rollback.go` (closed-over via main.go)
- `internal/scheduler/system_health.go` (RunDaily callable)
- `internal/scheduler/seasonal_task.go` (deprecated factory)
- `internal/scheduler/strategy_evolution.go` (factory for BTM)
- `internal/scheduler/stage3_tasks.go` (SyncXxxDailyTaskFunc factory)
- `internal/scheduler/stage3_oncestamps.go` (cross-restart dedup)
- `Dockerfile.cron` (10 cron binary builds)
- `docker-compose.yml` (10 cron services + 1 prism-worker)
- `scripts/cron-entrypoint.sh` (60s-loop cron field matcher)
- `scripts/darwinian_adjust.sh` (cron-darwinian 入口)
- `scripts/c07-collect.sh` / `scripts/c07-evaluate.sh` (cron c07 入口)
- 全部 `cmd/*/main.go` (one-shot CLI 75 個)

---

## 12. 計數總表

| 類別 | 數量 |
|---|---|
| `BackgroundTaskManager.Register` 註冊 | **80** |
| Scheduler inner tasks | 1 |
| Docker cron containers | 10 |
| Docker long-running worker | 1 |
| Unregistered `time.NewTicker` 路徑 | 16 |
| One-shot CLI 工具 (無 background schedule) | 75 |
| **總 surface** | **183** |

差異說明:registered_tasks 80 + scheduler_inner 1 + docker_cron 10 + docker_long_running 1 + unregistered 16 + one-shot 75 = 183。
前一節 TL;DR 表格寫 174 是早期未把 scheduler_inner 與 unregistered 全數列入的 quick estimate;以本表 183 為最終精確值。

---

**Audit 完成。對應 JSON:** `a2-tasks.json`
