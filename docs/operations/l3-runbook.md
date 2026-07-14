# L3 Stage 3 Scheduling + Alerting — Operations Runbook

> **Status**: Stage 3 shipped via PR #1127 (merge `a535a8c8`, 2026-07-12). Hotfix PR #1128 (`feat/stage-3.1-hotfix`) open with oncestamp persistence + Prometheus metrics + opt-out flags.
> **對象**: ops / on-call engineering
> **範圍**: 5 個排程任務 (sync-events/macro/capital/regime + recalibrate-templates) 與 5 條 alert rule (data-staleness-{warning,critical} / event-calendar-sparse / model-confidence-degraded / prediction-drift)
> **Metric 權威**: `/metrics` endpoint on the daemon → Prometheus 直連
> **Log 來源**: `internal/monitoring` 的 alert history (`monitor.history` ring buffer, `maxHistory=1000`) + `data/ledger/stage3_oncestamps.json` + `data/ledger/event_flow_predictions.jsonl`

## 1. Pre-flight Checklist

部署 Stage 3 前(或 merge 後第一次重啟 daemon 時)逐項確認:

- [ ] **預設 flag 已啟用**: `STAGE3_TASKS_ENABLED` 與 `STAGE3_ALERTS_ENABLED` 未顯式設為 `false`(預設 `true`,見 `internal/config/config.go:138-139`)。replay / sim 專用 deploy 才需要 disable。
- [ ] **Ledger 目錄可寫**: `cfg.LedgerDir`(預設 `data/ledger`)存在且 daemon 有寫權限。`NewEventFlowPredictionStore` + `NewFileOncestampStore` 在 boot 階段不驗證(懶創建),但第一個 task 跑時若寫失敗會 log error。
- [ ] **第一次 syn* 任務 fire**: boot 1 分鐘內,`sync-events-daily` / `sync-macro-daily` / `sync-capital-daily` 應該看到 log output(`[Gateway] registered ... background task` 與首次執行的 `stage3_task_audit task_executed` event)。若 missing → check `apigateway.ScheduledTask.Enabled` 是否被某處覆寫為 false。
- [ ] **/metrics 端點含 stage3 metric**: `curl /metrics | grep atlas_stage3` 應該看到 `atlas_stage3_task_runs_total` / `atlas_stage3_alerts_fired_total` 的 HELP 行(就算 0 個 sample 也應出現)。
- [ ] **OncestampStore 跨重啟存活**: 重啟 daemon 後 `data/ledger/stage3_oncestamps.json` 應仍存在;若 missing → 該週期內的 task 會 double-fire 一次但不致命(下次會去重)。

## 2. Daily Check-in 流程

### 2.1 Prometheus 指標(快速)

```bash
# 過去 10 分鐘的 5 個 task 跑率(預期:每個 task 每 run 各自 1 次)
curl -s http://localhost:18080/metrics | grep '^atlas_stage3_task_runs_total'

# 過去 24 小時的 alert fired 分佈(預期:event-calendar-sparse 在 228 / 連假會偶發)
curl -s http://localhost:18080/metrics | grep '^atlas_stage3_alerts_fired_total'
```

### 2.2 Alert rate 健康基線

| Alert rule | 預期頻率 | 異常訊號 |
|-----------|----------|----------|
| `data-staleness-warning` | 偶發(個別 channel 抓資料慢) | > 2/day 持續 1 週 → 該 channel 有結構性問題 |
| `data-staleness-critical` | 極少(>6 小時無資料) | > 1/week 持續 → channel health check 失效 |
| `event-calendar-sparse` | 228 / 春節 / 國慶 *前後* 不應 fire(已 typeFilter 修) | 真的 fire 了 → `internal/industry/event_calendar.go:895` 的 typeFilter 重新驗證 |
| `model-confidence-degraded` | 0/run(ledger 有資料 → 不應 fire) | 持續 fire → `LatestCapitalFlowPrediction` 永遠回中性值,predictor 有 bug |
| `prediction-drift` | 0/run(暖機期間) | 暖機結束後突然 fire → 檢查 actual vs prediction 的數值差距 |
| `prediction-drift-insufficient-history` | boot 後前 5 天 | 6 天後仍 fire → `predictionLedger.Len()` 沒在增長,predictor 沒被叫 |

### 2.3 一行任務健康檢查

```bash
# 最近 5 個 task 執行的 audit log
grep "stage3_task_audit" data/logs/atlas.log | tail -5

# 重啟後 oncestamp 是否保留
ls -la data/ledger/stage3_oncestamps.json
jq . data/ledger/stage3_oncestamps.json   # 應有 stage3.daily / .weekly / .monthly claim

# Ledger 筆數增長(gauge proxy)
wc -l data/ledger/event_flow_predictions.jsonl
```

## 3. Acceptance Criteria

不像 L2.4 有固定 14 天觀察期,Stage 3 的 acceptance 是 **持續 operational**:

### 持續 throughput(per day)

| 指標 | 預期下界 | 說明 |
|------|---------|------|
| `atlas_stage3_task_runs_total{task="sync-events-daily",result="success"}` | 1/day | 06:00 fire 一次 |
| `atlas_stage3_task_runs_total{task="sync-macro-daily",result="success"}` | 1/day | 06:00 fire 一次 |
| `atlas_stage3_task_runs_total{task="sync-capital-daily",result="success"}` | 1/day | 13:30 fire 一次(只有 trading day) |
| `atlas_stage3_task_runs_total{task="sync-regime-weekly",result="success"}` | 1/week | 週一 08:00 fire 一次 |
| `atlas_stage3_task_runs_total{task="recalibrate-templates-monthly",result="success"}` | 1/month | 1 號 08:00 fire 一次 |

任何 task 連續 2 個應 fire 的週期都沒出現在 metric → §4 排查。

### Ledger 增長速率

每天 06:00 + 13:45 append 應該各 +1 筆,**真正 trading day** 也約 +2/day。

`event_flow_predictions.jsonl` 的 `wc -l` 應該隨日線性增長(被 cap 1000 後保持恆定)。逆向變化(突然縮短)→ 檔案被破壞或誤刪,§4 流程修復。

## 4. Failure Modes & 處置

| 故障模式 | 偵測 | 處置 |
|---------|------|------|
| **task 完全不再 fire** | `atlas_stage3_task_runs_total{task=X}` 連續 2 週期 = 0 | (1) 檢查 `STAGE3_TASKS_ENABLED` 是否被改成 false;(2) 檢查 BTM `RegisteredTasks` 名單;(3) 重啟 daemon 觀察首次 fire |
| **task 持續 fail** | `result="failed"` 速率 / `result="success"` 速率 > 5% | audit log 看 `stage3_task_audit` 的 error 訊息:(a) "RefreshEventCalendar dependency is nil" → main.go wiring missing;(b) gateway 5xx → upstream issue;(c) data corruption → check JQ mtime |
| **alert 從未 fire** | `atlas_stage3_alerts_fired_total{rule=X}` 完全 0 且預期應有資料 | (1) 確認 metric 名稱無 typo;(2) check `monitor.handlers` 是否僅註冊 console,而非外部 SIEM;(3) `Rule=stage3_*` 對應的 deps callback 是不是 nil |
| **oncestamp JSON 損壞** | 啟動時 `[Stage3] oncestamp store unavailable` log 出現,daemon 退回 in-memory | 直接 `rm data/ledger/stage3_oncestamps.json`(daemon 會 lazy-recreate);本週可能會 double-fire 一次 |
| **event_flow_predictions JSONL 損壞** | `AppendPrediction` return error,audit log 印 "write failure" | (a) `ls -la data/ledger/event_flow_predictions.jsonl` 看檔案大小是否正常;(b) `jq . data/ledger/event_flow_predictions.jsonl` 找 corrupted line;(c) 必要時 truncate,history 從 0 開始重建 |
| **flag flip 沒生效** | `STAGE3_TASKS_ENABLED=false` 但 task 仍 fire | flag 在 boot 時讀,沒有 hot reload。**必須重啟** daemon 才生效 |
| **prediction-drift 暖機信號卡住** | boot 6 天後仍 emit `prediction-drift-insufficient-history` | (1) `wc -l data/ledger/event_flow_predictions.jsonl` 應 ≥ 5;(2) 若仍 0 筆 → `LatestCapitalFlowPrediction` 沒成功寫入 ledger,檢查 closure wiring;(3) 若有 > 5 筆但 alert 還在 → `RecentEventFlowPredictionsActualCount` 沒正確回報,fallback 邏輯有 bug |
| **Prometheus metric 為空** | `/metrics` 完全看不到 `atlas_stage3_*` | (1) helper `nil` collector 會 silently skip,確認 `stage3Deps.metricsCollector` 是否 wire;(2) inspect `RegisterHandler` 路徑是否走 `system.WithMetricsCollector(collector)` |

### 降階選項(permanent 修復前臨時)

| 想暫停的部分 | 設定 | 何時還原 |
|------------|------|---------|
| 5 個排程任務 | `STAGE3_TASKS_ENABLED=false` + restart | debugging 期間 |
| 3 個 alert evaluator | `STAGE3_ALERTS_ENABLED=false` + restart | alert storm 期間 |
| 全部 Stage 3 | 兩個 flag 同時 false + restart | hard rollback |

## 5. References

- Stage 3 設計: PR #1127 commit `b4dddbc5`,merge `a535a8c8`
- Stage 3 hotfix: PR #1128 (`feat/stage-3.1-hotfix`) — Items 1-6
- Stage 3 followup: PR `feat/stage-3.1-followup` — Items 7-9 (本 runbook + TRAPS + tests)
- 模組入口:
  - `internal/scheduler/stage3_tasks.go` — 5 個 task wrapper + 3 個 once-guard
  - `internal/scheduler/stage3_oncestamps.go` — `OncestampStore` interface + `FileOncestampStore`
  - `internal/monitoring/stage3_rules.go` — 5 條 alert rule 評估器
  - `internal/monitoring/startup_metrics.go` — `atlas_stage3_*` metric constants + helpers
  - `internal/ledger/event_flow_prediction_store.go` — ledger (Len() / Size() 用於暖機)
  - `cmd/atlas/stage3_tasks.go` — main.go wiring
- Config flags: `internal/config/config.go` 的 `Stage3TasksEnabled` / `Stage3AlertsEnabled`
- Wave 9 命名規約: [`../REFERENCE/TRAPS.md`](../REFERENCE/TRAPS.md) § Prometheus Metric 命名空間
- Oncall 通訊: PR #1128 留言 thread + Slack `#atlas-ops`(緊急 alert storm / panic)
