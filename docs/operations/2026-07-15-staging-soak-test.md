# Staging 7-Day Soak Test — Capital Flow Audit Follow-up

> **啟動日期**：2026-07-15（14 個 commit 合併後當天）
> **結束日期**：啟動後 7 天
> **目的**：驗證 12 個 PR 合併後在 staging 環境不破壞既有功能
> **Owner**：release captain + on-call SRE

## 環境

- Staging 部署：Docker Compose（同 production schema）
- 資料來源：PostgreSQL ledger（per `DATABASE_URL`）+ 沙盒 API keys
- 觀察工具：Prometheus + 內建 `/api/llm/health` 端點

## 每日檢查

### 6 個可驗證的 HTTP endpoint（5 個 + 1 個 G-08 fix 揭漏後補的）

原計畫假設的 `regime_history` / `prediction_backtest` 表只在 PR-FIX-01 加了 SQLite schema，**實際部署用 PostgreSQL** 且表未 populated。`/api/llm/stress_index/current` 與 `/api/alert/list` 也只有 MCP tool wrapper，無 HTTP route。

可實際驗證的 6 個 endpoint（5 個原始 + 1 個 G-08 fix 揭漏後補的 detector_scan）：

| # | Check | Endpoint | 通過條件 |
|---|-------|----------|---------|
| 1 | health | `GET /health` | `status: "ok"` |
| 2 | llm_router | `GET /api/llm/health` | ≥ 1 healthy provider |
| 3 | capital_flow | `GET /api/capital-flow/summary` | 有 `resonance_dir`（驗證 G-12 ChangePct fix） |
| 4 | event_prediction | `GET /api/events/prediction` | ≥ 5 個 prediction（驗證 G-06 narrative tilt 5→24 themes） |
| 5 | scheduler | `GET /api/scheduler/status` | ≥ 30 tasks + `macro_ingest` + `auto_capital_flow` 都在 |
| 6 | detector_scan | `GET /api/detector/scan/status?limit=1` | 503 + "store unavailable" 視為 pass（route reachable but jsonl backend needs sqlite for data — see G-08 follow-up） |

### 6th check 的由來（G-08 真相揭露，2026-07-15 commit `79d87635`）

加了 detector_scan check 後，發現 G-08 仍未完全修復：
- ✅ MCP tool wrapper 已註冊（commit `f754d014`）
- ❌ HTTP route 帶 auth 回 404（route 沒註冊）

**修復**（commit `9a7da23f`）：移除 if-else gate，總是註冊路由 + nil store 容錯。詳見 `.omo/audit/2026-07-15-capital-flow-audit-followup.md`（內部） 「G-08 真相揭露 + 修復」段。

### 自動化 runner（macOS launchd 推薦）

`scripts/staging-soak-check.sh` 跑這 6 check，輸出 JSON 到 `~/logs/atlas-soak/$DATE.json`，exit code：
- 0：all pass 或 warn
- 1：任一 hard fail
- 2：連不上 staging

**macOS 安裝**（cron 在 macOS Catalina+ 不 work，用 LaunchAgent）：
```bash
./scripts/install-soak-automation.sh
```

這會：cp script 到 `~/bin/`、cp plist 到 `~/Library/LaunchAgents/`、建立 `~/logs/atlas-soak/`、用 `launchctl bootstrap` 載入。

**Linux 安裝**（用 cron 替代 launchd）：
```bash
sudo cp scripts/staging-soak-check.sh /usr/local/bin/
sudo chmod 755 /usr/local/bin/staging-soak-check.sh
echo "0 6 * * *  atlas  /usr/local/bin/staging-soak-check.sh >> /var/log/atlas-soak/cron.log 2>&1" | sudo tee /etc/cron.d/atlas-soak
```

**手動觸發**：
```bash
launchctl kickstart -k gui/$(id -u)/com.atlas.soak-check
```

### Release captain 每天 5 分鐘 review

```bash
cat ~/logs/atlas-soak/$(date -u +%Y-%m-%d).json | jq .
```

`overall: "pass"` 全綠；`"fail"` 需排查；`"warn"` 觀察。

## Wave 驗證里程碑

| Day | Wave 必須驗證 | 失敗時動作 |
|-----|--------------|----------|
| Day 1 | Wave 1 PRs（PR-FIX-02/03/06/08/10 + F-02/F-03） | rollback Wave 1 |
| Day 3 | Wave 2 PRs（PR-FIX-05/07 + F-01） | rollback Wave 2 |
| Day 5 | Wave 3 PRs（PR-FIX-01/04） | rollback Wave 3 |
| Day 7 | Wave 4 PR（PR-FIX-09）test coverage | rollback Wave 4 |

## 退出條件（任一未達即失敗）

1. ✅ 7 個連續 24h 區間 `overall: "pass"`
2. ✅ `event_prediction` 每天都有 5 個 prediction（任意 direction 皆可，包括全部 inflow）
3. ✅ `llm_router` 每天都有 ≥ 1 healthy provider
4. ✅ `capital_flow` `resonance_dir` 欄位有值（不是空）
5. ✅ `scheduler` 每天 52 個 tasks 都存在，沒有 task 連續 2 天 disabled

## Rollback 程序

若任一 Day 失敗：
```bash
# 1. 停 staging
cd /path/to/atlas-go && docker compose down

# 2. 還原到 merge 12 個 PR 之前的 main commit
git checkout <pre-audit-followup-commit-sha>
docker compose build atlas && docker compose up -d

# 3. 記錄失敗
mkdir -p ~/logs/atlas-soak/incidents/$(date -u +%Y-%m-%d)
echo "incident description" > ~/logs/atlas-soak/incidents/$(date -u +%Y-%m-%d)/incident.md
```

## 成功後動作

7-day soak 全綠後：
1. **歸檔**本文件到 archive 目錄（建立 `docs/operations/archive/2026-07-15/` 後把本檔移過去，加「Completed YYYY-MM-DD」標頭）
2. **改名推廣** `scripts/staging-soak-check.sh` → `scripts/staging-deployment-health-check.sh`，改為通用 6-check（任何 staging 都跑，含 G-08 detector_scan 第 6 check）
3. **保留但降頻 cron/launchd**：daily 06:00 → weekly Monday 06:00（cron 改 `0 6 * * 1`；launchd 改 Weekday=1）
4. **刪除** `.omo/plans/2026-07-15-capital-flow-audit-followup/`（gitignored，純 plan-only，任務已完成）
5. **Production rollout**：merge main → production deploy（per release process）
6. **更新** `.omo/audit/2026-07-15-capital-flow-audit-followup.md`（內部） 標記「verified in staging YYYY-MM-DD」

## 觀察窗口 vs Soak 差異

- **Wave 11 L2.4 observation window**：22 天（L2.4 sector agents 觀察）
- **本 7-day soak**：純 PR 合併後的 baseline 穩定性驗證
- 兩者並行不衝突，可同時進行
