# Staging 7-Day Soak Day Counter

> **啟動**：2026-07-15（commit `007a5056` 之後）
> **結束**：2026-07-22（Day 7 06:00 UTC 之後）
> **自動化**：`com.atlas.soak-check` LaunchAgent 每日 06:00 UTC 跑
> **Log 路徑**：`~/logs/atlas-soak/YYYY-MM-DD.json`

## Day-by-day 進度

| Day | 日期 | 狀態 | Wave milestone | overall |
|-----|------|------|----------------|---------|
| Day 1 | 2026-07-15 | ✅ done | Wave 1 驗證 | `pass` |
| Day 2 | 2026-07-16 | ⏳ pending | — | — |
| Day 3 | 2026-07-17 | ⏳ pending | Wave 2 驗證 | — |
| Day 4 | 2026-07-18 | ⏳ pending | — | — |
| Day 5 | 2026-07-19 | ⏳ pending | Wave 3 驗證 | — |
| Day 6 | 2026-07-20 | ⏳ pending | — | — |
| Day 7 | 2026-07-21 | ⏳ pending | Wave 4 驗證 | — |

## 每日 review checklist（5 分鐘）

1. 讀今天的 log：
   ```bash
   cat ~/logs/atlas-soak/$(date -u +%Y-%m-%d).json | jq .
   ```
2. 確認 `overall: "pass"`（不是 fail）
3. 確認 5 個 check 都有具體 reason（不是空字串）
4. 記錄到本文件 Day 行（`✅ done` + overall 狀態）
5. 如 fail：照 `docs/operations/2026-07-15-staging-soak-test.md` 的 Rollback 程序

## 6 個 check 速查（含 G-08 fix 後新加的第 6 個）

| # | Check | Endpoint | 失敗時 debug |
|---|-------|----------|-------------|
| 1 | health | `GET /health` | `curl -v http://localhost:18080/health` |
| 2 | llm_router | `GET /api/llm/health` | 看 `providers` 欄位，應有 3 healthy |
| 3 | capital_flow | `GET /api/capital-flow/summary` | 看 `resonance_dir` 是否有值 |
| 4 | event_prediction | `GET /api/events/prediction` | 看 `predictions | length` 應 ≥ 5 |
| 5 | scheduler | `GET /api/scheduler/status` | `[] | length` 應 ≥ 30 且 `macro_ingest` + `auto_capital_flow` 都在 |
| 6 | detector_scan | `GET /api/detector/scan/status?limit=1` | 503 + "store unavailable" 視為 pass（route reachable but jsonl backend needs sqlite for data — see G-08 follow-up） |

### 第 6 個 check 為何被加？G-08 真相揭露

加 6th check 後，發現 G-08 仍未完全修復 — MCP tool wrapper 已註冊，但 HTTP route 帶 auth 回 404。commit `9a7da23f` 移除 if-else gate，總是註冊路由 + nil store 容錯。詳見 `.omo/audit/2026-07-15-capital-flow-audit-followup.md`（內部） 「G-08 真相揭露 + 修復」段。

## 退出條件（任一未達即失敗）

1. 7 個連續 24h 區間 `overall: "pass"`
2. `event_prediction` 每天都有 5 個 prediction
3. `llm_router` 每天都有 ≥ 1 healthy provider
4. `capital_flow` `resonance_dir` 欄位有值
5. `scheduler` 每天 52 個 tasks 都存在
6. `detector_scan` 每天回 503（route reachable）或 200（sqlite backend 已配）皆視為 pass

## 成功後動作（Day 7 全綠後）

1. 歸檔本文件（建立 `docs/operations/archive/2026-07-15/` 後把本檔移過去）
2. 歸檔 staging-soak-test 計畫文件（同一個 archive 目錄）
3. 改名推廣 `scripts/staging-soak-check.sh` → `scripts/staging-deployment-health-check.sh`
4. 降頻 cron/launchd：daily → weekly（Monday 06:00 UTC）
5. 刪除 `.omo/plans/2026-07-15-capital-flow-audit-followup/`
6. **Production rollout**：照 `docs/operations/production-rollout-runbook.md` Day 8 SOP（merge staging → main → tag → release workflow → smoke test）
7. 在 `.omo/audit/2026-07-15-capital-flow-audit-followup.md`（內部） 「最終驗證條件」勾選全部 → 標記「verified in staging 2026-07-22」

## 失敗後動作（任一 Day fail）

照 `docs/operations/2026-07-15-staging-soak-test.md` §4 Rollback 程序：
1. `docker compose down`
2. `git checkout <pre-audit-followup-commit-sha>`
3. `docker compose build atlas && up -d`
4. 記錄 incident 到 `~/logs/atlas-soak/incidents/$DATE/incident.md`
5. 通知 on-call SRE + release captain
6. 修補後重新部署 + 重置 7-day counter

## 當前狀態摘要（2026-07-15 18:10 UTC）

- **Day 1**：✅ pass（**6/6 checks all pass**，post G-08 fix at commit `9a7da23f`）
- **Time-axis**：
  - 04:02 UTC：5/6 pass（detector_scan ❌ route 404）
  - 07:23-07:25 UTC：commits 推 main + container 重啟
  - 07:26 UTC 起：6/6 pass 穩定
  - 10:06 UTC：最後一次驗證 `{"overall":"pass","pass":6,"fail":[]}`
- **LaunchAgent**：loaded, StartCalendarInterval state=active
- **下次自動跑**：2026-07-16 06:00 UTC（Day 2）
- **Log 累計**：1 個 distinct date entry（`2026-07-15.json`，內含 23 次 LaunchAgent 跑過的 iteration）
- **out.log**：75KB+，期間一過性 `capital_flow` fail（07:21 UTC 部署 gap）+ `detector_scan` 404→503 漸進（07:21 UTC → 07:23 UTC）→ 全部穩定
- **仍 loaded after kickstart**：✅
- **local main == origin/main**：✅ SHA `05ca1423`（距離 Day 1 啟動時的 `156944d8` +22 commits，含 Stage 6 + G-08 fix + follow-up）

## Owner checklist

- [ ] Day 1 ✅ 2026-07-15
- [ ] Day 2  2026-07-16
- [ ] Day 3  2026-07-17（Wave 2 驗證）
- [ ] Day 4  2026-07-18
- [ ] Day 5  2026-07-19（Wave 3 驗證）
- [ ] Day 6  2026-07-20
- [ ] Day 7  2026-07-21（Wave 4 驗證）

## G-11 觀察記錄（geopolitical component）

2026-07-15 Day 1 soak check 觀察：`macro_get_stress_index_current` 的 `geopolitical` component 持續 = 0。
**根因**（PR-FIX-07 commit `e7f25dd7` 已分析）：`dashboard.IngestAndUpdateMacro` → `a.geoProvider.FetchScore` → `narrativeEngine.UpdateMacro` 流程完整，但 staging 環境的 RSS feeds（BBC / Al Jazeera）與 GDELT 2.0 API 連線失敗 → `FetchScore` 報錯被 warn+ignore → `geoScore.Intensity` 留 zero。

**修復路徑**：7-day soak 期間每日觀察 `geopolitical` 值。若 Day 2-3 仍 = 0，需：
1. 確認 staging DNS 能解析 `feeds.bbci.co.uk` 與 `api.gdeltproject.org`
2. 確認 sandbox egress firewall 沒擋 443/tls
3. 若都通但仍 fail：可能 GDELT rate limit（PR-FIX-07 提到 GDELT 在 CI 環境 55s+ 慢）

**Owner**：on-call SRE + release captain
