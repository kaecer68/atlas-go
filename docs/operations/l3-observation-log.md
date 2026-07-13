# L3 Stage 3 Scheduling + Alerting — Observation Log

> **對應 runbook**：[`docs/operations/l3-runbook.md`](./l3-runbook.md)
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

啟用 Stage 3 排程 + 報警前,逐項確認:

- [ ] **環境選擇**: production `/admin/#page-system` → 確認 `atlas` 容器 up,`/health` 200
- [ ] **`STAGE3_TASKS_ENABLED` / `STAGE3_ALERTS_ENABLED`** env vars 預設 true(已合併 PR #1128 commit `a76f7250` 行為)
- [ ] **`/metrics` 端點可達**:`curl -fsS http://localhost:18080/metrics` 回包含 `atlas_stage3_task_runs_total` 與 `atlas_stage3_alerts_fired_total`(就算還沒 fire,Gauge 已 register)
- [ ] **`data/ledger/` 可寫入**:`touch data/ledger/.test && rm data/ledger/.test`(container 內 user 必須有 rw 權限)。若失敗 → oncestamp fallback in-memory(看 runbook §1)
- [ ] **第一次 06:00 Asia/Taipei 觀察**:`docker exec atlas ls -la data/ledger/stage3_oncestamps.json` 確認檔案被建立
- [ ] **第一輪 daily fire 後檢查 Prometheus**:`atlas_stage3_task_runs_total{task="sync-events-daily"} == 1`(代表 06:00 真的有跑,不是 closure closure closure 一直是 0)

### 啟動日填寫欄

```
- 啟動日期 (UTC +8):
- 重啟前最後一次合併到 develop 的 commit SHA:<head SHA>
- 啟動時的 image tag:`ghcr.io/kaecer68/atlas-go@sha256:...`
- 啟動時的 atlas version(`atlas --version`):
- 啟動當下的 regime:`RiskOn | Neutral | RiskOff`(影響 `event-calendar-sparse` 判讀)
- 第一輪 sync-events-daily 06:00 完成時間:
- 第一輪 sync-events-daily 結果(success/failed):
- 第一輪 oncestamp.json size:
```

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

- Runbook: [`docs/operations/l3-runbook.md`](./l3-runbook.md)
- Wave 11 scheduling 設計: PR #1127 (merge commit `a535a8c8`)
- Stage 3.1 hotfix: PR #1128 (merge commit `30f0df82`) — Items 1-6 + CI flakes (analogue→analog + WaitGroup)
- Stage 3.1 follow-up: PR #1129 (merge commit `54af456c`) — L3 runbook + TRAPS rows + race/retry/BTM tests
- L2.4 predecessor observation-log 範本(只看結構,不複製 LLM-driven metric query): [`docs/operations/l2-4-observation-log.md`](./l2-4-observation-log.md)
- Wave 9 metric 命名規約來源: PR #926 + Issue #927(`docs/REFERENCE/TRAPS.md` §Prometheus)
- 通訊管道: Slack `#atlas-ops`(緊急 alert storm / panic / 大規模 task 失敗)
