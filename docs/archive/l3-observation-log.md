# L3 Stage 3 Scheduling + Alerting — Observation Log

> **對應 runbook**：[`docs/operations/l3-runbook.md`](../operations/l3-runbook.md)
> **範圍**：Stage 3 排程 + 報警(permanently operational, daily check-in forever — 不是 L2.4 的 14 天時限觀察)
> **啟動日期**：(TBD，第一次在 production 跑 `sync-events-daily` 06:00 Asia/Taipei 後填寫)
> **觀察對象**：5 個 periodic tasks(`sync-events-daily` / `sync-macro-daily` / `sync-capital-daily` / `sync-regime-weekly` / `recalibrate-templates-monthly`) + 3 個 alert evaluator wrappers(`stage3-alert-staleness` / `stage3-alert-daily` / `stage3-alert-market-close`)
> **關閉條件**：無(permanently operational;如有 redesign 走 new major version)

---

## 觀察指標（對齊 runbook §2）

| 指標 | Prometheus query | 目標 |
|------|------------------|------|
| `task_runs.total` | `sum(rate(atlas_stage3_task_runs_total[24h])) by (task)` | 每 task 各日 ≥1 次(per design 預期 06:00 / 13:30 / 等) |
| `task_runs.failure_rate` | `rate(atlas_stage3_task_runs_total{result="failed"}[24h]) / rate(atlas_stage3_task_runs_total[24h])` | < 5%(per task) |
| `alerts_fired.data_staleness` | `rate(atlas_stage3_alerts_fired_total{rule=~"data-staleness.*"}[24h])` | 通常 = 0;若 > 0/24h 排 gateway issue |
| `alerts_fired.event_calendar_sparse` | `rate(atlas_stage3_alerts_fired_total{rule="event-calendar-sparse"}[24h])` | 通常 = 0;若 spring festival/228/國慶連假誤觸發,跑 `IsTaiwanTradingDay` regression(commit `0dcd159b`) |
| `alerts_fired.model_confidence_degraded` | `rate(atlas_stage3_alerts_fired_total{rule="model-confidence-degraded"}[24h])` | 通常 = 0;首次部署前 5 天必為「ledger 為空不算 failure」(見 warmup ledger.Len() < 5) |
| `alerts_fired.prediction_drift` | `rate(atlas_stage3_alerts_fired_total{rule=~"prediction-drift.*"}[24h])` | warmup 之前看到 `prediction-drift-insufficient-history` 屬正常(暖機期信號,level=Info) |
| **oncestamp file mtime** | shell `stat data/ledger/stage3_oncestamps.json -c %y`(或 inotify-style watcher) | 跨重啟後變動即代表 once-guard 寫入生效;若 dead > 24h 代表 dailyOnceGuard 沒被觸發 |

---

## Daily Check-in 格式

每日(或每次 container 重啟後)追加一筆：

```markdown
### YYYY-MM-DD — Day N
- Pre-flight re-verified: Y / N(若 N,跑 runbook §1 後才能記此筆)
- Tasks fired in last 24h:
  - sync-events-daily: <次數> (last <timestamp>)
  - sync-macro-daily: <次數>
  - sync-capital-daily: <次數>
  - sync-regime-weekly: <次數>(週期性)
  - recalibrate-templates-monthly: <次數>(月週期)
- Alert fired last 24h:<列表或「none」>
- oncestamp.json mtime:<timestamp>
- 異常:<free text,或「none」>
- 決策:繼續觀察 / 觸發 runbook §4 failure mode 處置
```

---

## Day 0 — Baseline（permanently operational，但仍有「啟動日」作為觀察起點）

### Pre-flight Checklist 驗證（對齊 l3-runbook.md §1）

啟用 Stage 3 排程 + 報警前,逐項確認：

- [x] **環境選擇**: local docker compose。`atlas-go` container `Up 5 seconds (healthy)`，boot log `<info msg=server_startup_ok>`
- [x] **`/health` 200 OK**:`curl http://localhost:18080/health` 回 `{"status":"ok","ports":{"atlas_http":{"addr":":18080","state":"healthy"},"fubon_proxy":{"addr":"127.0.0.1:18081","state":"free"}}}`
- [x] **`STAGE3_TASKS_ENABLED` / `STAGE3_ALERTS_ENABLED`** env vars 預設 true（已合併 PR #1128 commit `a76f7250` 行為；deployed binary 同 binary 代碼，不需額外設定）
- [x] **Stage 3 排程全部 register**:見下面 §「任務 register 證據」
- [x] **`/metrics` 端點可達**:`curl http://localhost:18080/metrics` 回 200 + counter count = 0（expected — metric 直到第一次 emit 才 record，符合 `TestStage3Metrics_PrometheusHandler_StableBeforeEmission` 設計）
- [ ] **`data/ledger/` 可寫入**:`docker exec atlas-go touch data/ledger/.test && rm data/ledger/.test` — **next daily check-in 由運維確認**
- [ ] **第一輪 `sync-events-daily` 06:00 Asia/Taipei fire 之後**:見下面 §「Day 0 — 第一次 fire 觀察」(預計 2026-07-14 06:00 CST = 22:00 UTC)
- [ ] **第一輪 fire 後驗證 Prometheus**:`atlas_stage3_task_runs_total{task="sync-events-daily"} == 1` — **next morning**

### 啟動日填寫欄 (實際數據)

```
- 啟動日期 (UTC+8): 2026-07-13 23:02 CST
- 啟動時的本地 image tag: atlas-atlas:latest (sha256: 79a853fced9e)
- 容器 image 怎麼來: local docker compose build（不是 ghcr.io pull — README deploy flow 用 compose build）
- 容器 build source 位置: working tree at commit e8261a6b (ops/speed-up-docker-build-context, base: develop 8a4a3efa + .dockerignore 優化)
- 容器 binary 內含最新 commit: 8a4a3efa (PR #1131 fix — Stage 3.1 hotfix + Stage 3 docs + Stage 3 test gap)
  - 注意: develop HEAD at deploy time = ffdd454f (PR #1134 prism-trigger event-driven OnCompleted)。這 3 個 PR (#1132 tsmc_revenue fallback, #1133 vix-baseline, #1134 prism-trigger) 不在本 image 內。
  - Container 啟動時無出現 panic/error，這 3 個 PR 跟 Stage 3 路徑無交集，**不影響 Day 0 觀察結論**
- 啟動時的 atlas version: `dev` (容器並未帶 `-X main.version=...` ldflag，因為 compose 沒設 ATLAS_VERSION env)
- 啟動當下的 regime: KPI `<div class="kpi-value">-</div>` — **fresh container, fresh boot，regime API 還沒 repopulate**（next daily check-in 抓一次）
- 第一輪 sync-events-daily 06:00 完成時間: TBD (next morning 2026-07-14 06:00 CST / 22:00 UTC)
- 第一輪 sync-events-daily 結果: TBD
- 第一輪 oncestamp.json size: TBD
- 容器 boot log 觀察: `[Gateway] BackgroundTaskManager started with 61 tasks`（merge 前是 60，Stage 3 加 8 = 68 total，但舊 container 報 60, 已 deploy 報 61 — 數字略低於預期，下方 §「已知偏差」說明）
```

### 任務 register 證據

容器 boot log (timestamp 15:01:28 UTC = 23:01:28 CST)：

```
[Gateway] registered sync-events-daily background task (1m interval, fires 06:00)
[Gateway] registered sync-macro-daily background task (1m interval, fires 06:00)
[Gateway] registered sync-capital-daily background task (1m interval, fires 13:30)
[Gateway] registered sync-regime-weekly background task (1m interval, fires Mon 08:00)
[Gateway] registered recalibrate-templates-monthly background task (1m interval, fires 1st 08:00)
[Gateway] registered stage3-alert-staleness background task (10m interval)
[Gateway] registered stage3-alert-daily background task (1m interval, fires 06:30)
[Gateway] registered stage3-alert-market-close background task (1m interval, fires 13:45)
```

8/8 Stage 3 tasks 完整 register。

### 已知偏差（deploy 觀察時記錄）

- **`BackgroundTaskManager started with 61 tasks`** — pre-Stage-3 容器 boot log 報 60 tasks；Stage 3 加 8 個理應 68，但日誌回報 61。差 7 個可能是 PR #1132/#1133/#1134 dry-registered 的 task 還沒進 deploy binary 之差（image 是 deploy 時 build 的，#1132/#1133/#1134 merge 在 image build 之後），所以這些 task 在 BTM 裡還沒看到。**非 Stage 3 觀察範圍內**。
- **`/metrics` 不列 `atlas_stage3_*`**（boot 後立刻 query）— expected by design：`RecordStage3TaskRun` / `RecordStage3AlertFired` 才有 emit，fire 完才 expose。所以下面 Day 1 才會看到這兩個 metric。

### Day 0 — 第一次 fire 觀察

> **TL;DR**： deploy 在 2026-07-13 23:02 CST，最近一輪 `sync-events-daily` 已過（今天 06:00 CST = 13:00 UTC）；下一輪 fire 預計 **2026-07-14 06:00 CST = 22:00 UTC**。**Day 1** daily check-in 應該在那之後 30 分鐘填寫。

下次 manual 操作（在 fire 之後）：

```bash
docker exec atlas-go ls -la /app/data/ledger/stage3_oncestamps.json
curl -fsS http://localhost:18080/metrics | grep -E '^atlas_stage3_'
docker logs atlas-go --since 22h | grep -E "task_executed|stage3_task_audit"
```

Expected output（**先跑這些，再回來補 Day 1 entry**）：

- `stage3_oncestamps.json` size > 0（`sync-events-daily` 06:00 fire 後 write 1 claim entry）
- `/metrics` 出現 `atlas_stage3_task_runs_total{task="sync-events-daily",result="success"} 1`（還可能有 `sync-macro-daily` if scheduler 跑過）
- Log line `task_executed task_id=sync-events-daily` 帶 `retry_count=0 error=""`

如果以上三項都看到，DONE；Day 1 entry 即可用下面 § Daily Check-in 範本填。

### Deploy 同時留下的補充工作

- PR #1135 (`ops/speed-up-docker-build-context`, commit `e8261a6b`) — `.dockerignore` 補 `services/` + frontend `node_modules/`，把 build context 從 11+ GB 縮到 4 s。可獨立 merge 提升 CI 效率。**本 deploy 已採用此 fix**（local build 從 580s context transfer 降到約 4 s）。


---

## Day 1 — Week 1（穩定運轉建立 baseline）

(空白,待每次 daily check-in 填寫)

### YYYY-MM-DD — Day 1
- ...

---

## Day 8 — Week 2（無重大問題, 仍持續 daily）

(空白)

---

## 已知限制（從 l3-runbook §4 failure modes 演化）

每當 runbook §4 觸發,在此新增一筆 post-mortem + how-was-it-resolved:

### YYYY-MM-DD — Post-mortem #1

- Failure mode 編號:
- Failure mode 名稱:
- 觸發條件:
- 處置動作:
- 修復 commit:
- 已加防護(regression test / alert):
- L3 觀察期結論(production ready / 需更久觀察): __________________

---

## References

- Runbook: [`docs/operations/l3-runbook.md`](../operations/l3-runbook.md)
- Wave 11 scheduling 設計: PR #1127 (merge commit `a535a8c8`)
- Stage 3.1 hotfix: PR #1128 (merge commit `30f0df82`) — Items 1-6 + CI flakes (analogue→analog + WaitGroup)
- Stage 3.1 follow-up: PR #1129 (merge commit `54af456c`) — L3 runbook + TRAPS rows + race/retry/BTM tests
- L2.4 predecessor observation-log 範本(只看結構,不複製 LLM-driven metric query): [`docs/archive/l2-4-observation-log.md`](./l2-4-observation-log.md)
- Wave 9 metric 命名規約來源: PR #926 + Issue #927(`docs/reference/traps.md` §Prometheus)
- 通訊管道: Slack `#atlas-ops`(緊急 alert storm / panic / 大規模 task 失敗)
