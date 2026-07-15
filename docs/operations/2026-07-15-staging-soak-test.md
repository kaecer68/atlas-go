# Staging 7-Day Soak Test — Capital Flow Audit Follow-up

> **啟動日期**：2026-07-15（所有 Wave PR 合併後啟動）
> **結束日期**：啟動後 7 天
> **目的**：驗證 14 個 commit（10 個原始 PR + 3 個 follow-up + 1 個 docs update）合併後在 staging 環境不破壞既有功能
> **Owner**：release captain + on-call SRE

## 環境

- Staging 部署：Docker Compose（同 production schema）
- 資料來源：staging JSONL fixtures + 沙盒 API keys
- 觀察工具：Prometheus + 內建 `/api/llm/health` 端點

## 每日檢查清單

每 24 小時跑一次以下命令並截圖到 `docs/operations/soak-logs/YYYY-MM-DD/`：

### 自動化 runner（推薦）

`scripts/staging-soak-check.sh` 把 6 個檢查包成單一 script，輸出 JSON 到 `/var/log/atlas-soak/$DATE.json`，exit code 反映結果：
- 0：all pass 或 warn（資料缺但系統健康）
- 1：任一 hard fail（資料完整性破壞）
- 2：script 連不上 staging

安裝方式見 `scripts/staging-soak-check.cron`（crontab template）。建議：

```bash
# 部署
sudo cp scripts/staging-soak-check.sh /usr/local/bin/
sudo chmod 755 /usr/local/bin/staging-soak-check.sh
sudo cp scripts/staging-soak-check.cron /etc/cron.d/atlas-soak
sudo chmod 644 /etc/cron.d/atlas-soak

# 每天 06:00 UTC 自動跑（stage3 sync-events-daily 之後、sync-capital-daily 13:30 之前）
# 結果寫到 /var/log/atlas-soak/$DATE.json
# (Optional) Slack/PagerDuty hook 接 non-zero exit code
```

Release captain 每天 5 分鐘 review `cat /var/log/atlas-soak/$(date -u +%Y-%m-%d).json | jq` 即可。

### 手動指令（debug 用）

如果 script 報 fail 但需要 detail，用以下指令單獨跑每個 check：

#### 1. Stress Index 健康度

```bash
curl -s http://localhost:18080/api/llm/stress_index/current | jq '.components.geopolitical, .score'
```

**預期**：`geopolitical > 0`（staging 有 RSS feed 連線），`score` 跟 `regime` 邏輯一致。

#### 2. Event Flow Prediction 正常性

```bash
curl -s http://localhost:18080/api/events/prediction | jq '.predictions | map(.direction) | unique'
```

**預期**：`["inflow", "neutral", "outflow"]` 至少 2 個值（不是全部 neutral）。

#### 3. Historical Store 無測試污染

```bash
sqlite3 data/state/atlas.db "SELECT recorded_at, COUNT(*) FROM regime_history GROUP BY recorded_at HAVING COUNT(*) > 1"
```

**預期**：0 筆結果（修前會有 5 秒級重複）。

#### 4. Prediction Backtest 有資料

```bash
sqlite3 data/state/atlas.db "SELECT COUNT(*) FROM prediction_backtest WHERE is_synthetic = 0"
```

**預期**：每天至少 1 筆新資料（scheduler 每日 06:00 觸發 `recalibrate-templates-monthly`）。

#### 5. Stage 3 排程全部觸發

```bash
curl -s http://localhost:18080/api/scheduler/status | jq '.tasks | map(.name) | sort'
```

**預期**：`["recalibrate-templates-monthly", "sync-capital-daily", "sync-events-daily", "sync-macro-daily", "sync-regime-weekly", "template-detector-scan"]` 6 個 task 都在。

#### 6. Alert Rules 4 條未誤報

```bash
curl -s http://localhost:18080/api/alert/list?since=24h | jq '.alerts | length, .by_severity'
```

**預期**：過去 24 小時 alert 數量 < 10 條；無 critical severity 連續 2 天。

## Wave 驗證里程碑

| Day | Wave 必須驗證 | 失敗時動作 |
|-----|--------------|----------|
| Day 1 | Wave 1 PRs（PR-FIX-02/03/06/08/10 + F-02/F-03）運行無誤 | rollback Wave 1 |
| Day 3 | Wave 2 PRs（PR-FIX-05/07 + F-01）運行無誤 | rollback Wave 2 |
| Day 5 | Wave 3 PRs（PR-FIX-01/04）運行無誤 | rollback Wave 3 |
| Day 7 | Wave 4 PR（PR-FIX-09）test coverage 正常 | rollback Wave 4 |

## 退出條件（任一未達即失敗）

1. ✅ 7 個連續 24 小時區間無 critical alert
2. ✅ `event_flow_prediction` 至少 5 天有非 neutral 輸出
3. ✅ `prediction_backtest` 表每天至少 1 筆新資料
4. ✅ `regime_history` 無秒級重複（同上檢查 3）
5. ✅ 4 條 alert 規則（staleness / sparse / confidence / drift）皆有被觸發紀錄

## Rollback 程序

若任一 Day 失敗：
```bash
# 1. 停 staging container
docker compose down

# 2. 從最後一個 green tag 還原 image
docker pull ghcr.io/kaecer68/atlas-go:v0.0.0.31-staging

# 3. 重新啟動
docker compose up -d

# 4. 記錄失敗到 docs/operations/soak-logs/YYYY-MM-DD/incident.md
```

## 成功後動作

7-day soak 全綠後：
1. Merge staging → main（觸發 production deploy）
2. 在 `docs/audit/2026-07-15-capital-flow-audit-followup.md` 標記「verified in staging」
3. 通知 `atlas-trading` channel 開始 production rollout
4. 關閉本 follow-up 計畫（移除 `.omo/plans/2026-07-15-capital-flow-audit-followup/`）

## 觀察窗口 vs Soak 差異

- **Wave 11 L2.4 observation window**：22 天（L2.4 sector agents 觀察）
- **本 7-day soak**：純 PR 合併後的 baseline 穩定性驗證
- 兩者並行不衝突，可同時進行
