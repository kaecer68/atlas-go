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

## 5 個 check 速查

| Check | Endpoint | 失敗時 debug |
|-------|----------|-------------|
| health | `GET /health` | `curl -v http://localhost:18080/health` |
| llm_router | `GET /api/llm/health` | 看 `providers` 欄位，應有 3 healthy |
| capital_flow | `GET /api/capital-flow/summary` | 看 `resonance_dir` 是否有值 |
| event_prediction | `GET /api/events/prediction` | 看 `predictions | length` 應 ≥ 5 |
| scheduler | `GET /api/scheduler/status` | `[] | length` 應 ≥ 30 且 `macro_ingest` + `auto_capital_flow` 都在 |

## 退出條件（任一未達即失敗）

1. 7 個連續 24h 區間 `overall: "pass"`
2. `event_prediction` 每天都有 5 個 prediction
3. `llm_router` 每天都有 ≥ 1 healthy provider
4. `capital_flow` `resonance_dir` 欄位有值
5. `scheduler` 每天 52 個 tasks 都存在

## 成功後動作（Day 7 全綠後）

1. 歸檔本文件（建立 `docs/operations/archive/2026-07-15/` 後把本檔移過去）
2. 歸檔 staging-soak-test 計畫文件（同一個 archive 目錄）
3. 改名推廣 `scripts/staging-soak-check.sh` → `scripts/staging-deployment-health-check.sh`
4. 降頻 cron/launchd：daily → weekly（Monday 06:00 UTC）
5. 刪除 `.omo/plans/2026-07-15-capital-flow-audit-followup/`
6. **Production rollout**：merge main → production deploy
7. 在 `docs/audit/2026-07-15-capital-flow-audit-followup.md` 標記「verified in staging 2026-07-22」

## 失敗後動作（任一 Day fail）

照 `docs/operations/2026-07-15-staging-soak-test.md` §4 Rollback 程序：
1. `docker compose down`
2. `git checkout <pre-audit-followup-commit-sha>`
3. `docker compose build atlas && up -d`
4. 記錄 incident 到 `~/logs/atlas-soak/incidents/$DATE/incident.md`
5. 通知 on-call SRE + release captain
6. 修補後重新部署 + 重置 7-day counter

## 當前狀態摘要（2026-07-15 12:02 UTC）

- **Day 1**：✅ pass（5/5 checks all pass）
- **LaunchAgent**：loaded, PID 12270, StartCalendarInterval state=active
- **下次自動跑**：2026-07-16 06:00 UTC（Day 2）
- **Log 累計**：2 entries（initial + kickstart validation）
- **out.log**：正常，無 err
- **still loaded after kickstart**：✅
- **local main == origin/main**：✅ SHA `156944d8`

## Owner checklist

- [ ] Day 1 ✅ 2026-07-15
- [ ] Day 2  2026-07-16
- [ ] Day 3  2026-07-17（Wave 2 驗證）
- [ ] Day 4  2026-07-18
- [ ] Day 5  2026-07-19（Wave 3 驗證）
- [ ] Day 6  2026-07-20
- [ ] Day 7  2026-07-21（Wave 4 驗證）
