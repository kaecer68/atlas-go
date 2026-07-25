# Production Rollout Runbook — Day 8 起

> **觸發時機**：staging 7-day soak 全綠後
> **Owner**：release captain + on-call SRE + 1 reviewer
> **rollback SLA**：30 分鐘內可回滾
> **本文檔對應**：Issue #1187 §5 成功後動作

## 0. 前置檢查（Day 7 結束後跑）

- [ ] `~/logs/atlas-soak/2026-07-15..2026-07-21.json` 7 天檔全 `overall: "pass"`
- [ ] `launchctl list | grep com.atlas.soak-check` 仍 loaded
- [ ] staging atlas-go container 仍在 18080 healthy
- [ ] Wave 1-4 PR 全部 merged
- [ ] CI on main 全綠（最後 commit）

> **註**：Issue #1187 已於 2026-07-15 標記為 CLOSED（含 6-check + G-08 + Stage 6 + 本 runbook 5 個 comment 完整更新）。本 runbook 是其 §5「成功後動作」的擴充 SOP，執行 trigger 由 soak 完成驅動，不由 issue 重新打開驅動。

## 1. Pre-flight（merge staging → main 前 1 小時）

```bash
# 確認 staging 為最後 green commit
cd /path/to/atlas-go && git fetch origin
git log -1 --oneline origin/main
# 應為 9a7da23f（含 G-08 fix）或更新版本（任何 7-day 期間 fix 都要先在 staging 跑過）

# 確認 Issue #1187 為 closed + comment thread 完整
gh issue view 1187 --json state --jq '.state'  # 應為 "CLOSED"
gh issue view 1187 --json comments  # 應有完整 5 個 comment（6-check + G-08 + Stage 6 + Runbook + 7-day pass）
```

## 2. 部署程序

```bash
# Step 1: tag the green commit
git tag -a v0.0.0.33-staging-green -m "7-day soak passed: $(date -u +%Y-%m-%d)"
git push origin v0.0.0.33-staging-green

# Step 2: trigger production image build
gh workflow run release.yml --ref main  # or appropriate workflow

# Step 3: wait for build + push to ghcr.io
gh run list --workflow release.yml --limit 3 --json status,conclusion,name
# 預期：success within 15-30 min

# Step 4: production deploy
# (依 project 既有 production deploy 流程)

# Step 5: smoke test
curl -fsS https://atlas.example.com/health
curl -fsS https://atlas.example.com/api/llm/health
```

## 3. Post-deploy 監控（前 24 小時）

- [ ] `atlas-go` container health 持續 `healthy`
- [ ] `/api/llm/health` 3 providers all healthy
- [ ] 觀察 alert volume（前 1 小時可能 false positive 多，視 noise level 決定是否調整閾值）
- [ ] 用 `cmd/atlas-stage4-loader --drop-synthetic`（新版本）驗證 production 路徑仍能寫入

## 4. 7-day Soak 收尾（Day 7 結束後 24h 內）

### 4.1 文件歸檔

> 檔案已於 PR #1359 移至 `docs/archive/`：
> - `2026-07-15-staging-soak-test.md` → `docs/archive/`
> - `soak-day-counter.md` → `docs/archive/`
>
> 此節保留為歷史參考，operator 不需再手動搬移。

```bash
# (已自動完成 — PR #1359)
# mkdir -p docs/operations/archive/2026-07-15/
# mv docs/operations/2026-07-15-staging-soak-test.md ...
```

### 4.2 Script 改名 + 推廣

```bash
# 改名：staging-specific → 通用 staging health check
git mv scripts/staging-soak-check.sh scripts/staging-deployment-health-check.sh
# 改內容：移除"5-check audit follow-up specific" comment，改通用 "5 個 staging 必活 endpoint"
# 更新 docs reference

# install script 也改名
git mv scripts/install-soak-automation.sh scripts/install-staging-health-check.sh
```

### 4.3 Cron 降頻

```bash
# 把 daily 改成 weekly（保留檢查但不要每天 spam）
# macOS: launchd Weekday=1 (Monday)
sed -i '' 's|Hour>6</Hour>|Weekday>1</Weekday>|' ~/Library/LaunchAgents/com.atlas.soak-check.plist
launchctl bootout gui/$(id -u)/com.atlas.soak-check
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.atlas.soak-check.plist
```

### 4.4 .omo plans 移除

```bash
# 計劃完成，gitignored 工作目錄可刪除
rm -rf .omo/plans/2026-07-15-capital-flow-audit-followup/
# 注意：這是 gitignored，刪除不影響 git
```

## 5. Issue #1187 收尾

```bash
# 標記為 closed
gh issue close 1187 --comment "Production rollout complete. 7-day soak passed. 5-check staging health check continues weekly via launchd."
```

## 6. 變更控制（change control）

| 動作 | Owner | 批准 | 記錄位置 |
|------|-------|------|----------|
| Production deploy | on-call SRE | release captain | Issue #1187 comment + Slack #atlas-trading |
| Soak test archive | release captain | self | git commit message |
| Script rename | release captain | self | git commit message |
| LaunchAgent cadence | release captain | self | commit + Slack |
| Issue close | release captain | self | gh issue close 1187 |

## 7. 緊急 rollback（30 min SLA）

```bash
# 1. revert main to last green tag
git revert <last-green-commit-sha>
# or
git reset --hard v0.0.0.33-staging-green
git push --force-with-lease

# 2. redeploy previous version
gh workflow run release.yml --ref <previous-tag>

# 3. notify
# - Slack #atlas-trading: "Production rollback to v0.0.0.33-staging-green"
# - reopen Issue #1187 if closed
# - post-mortem within 24h
```

## 8. 監控保留

- **LaunchAgent 永久保留**（降頻為 weekly Monday 06:00 UTC）—— 任何 regression 5 個 endpoint 之一死亡都會 fail
- **log retention**：`~/logs/atlas-soak/*.json` 保留 90 天（手動 `find -mtime +90 -delete`）
- **launchd log**：`launchd.out.log` / `launchd.err.log` 同樣保留 90 天

## 9. Success criteria（Day 8 結束）

- [ ] Production `atlas-go` container 跑 v0.0.0.33-staging-green（或更新版）
- [ ] 24h 無 critical alert
- [ ] `/api/events/prediction` 每天都有 5 個 prediction
- [ ] Document archive 完成
- [x] Issue #1187 closed（已於 2026-07-15 因 Day 1 6/6 pass + Stage 6 完成 + Runbook 上線完成）
- [ ] Slack #atlas-trading 收到 "Production rollout complete"

> **歷史紀錄**：Issue #1187 早於本 runbook 結束前 closed，是因為 follow-up PR 全部合併後 main 已穩定。closed 不代表已完成所有 production rollout 動作；只是 plan stage 結束。第 7 日 6/6 全綠後仍需執行本 runbook §2-§8 才能實際 deploy production image。
