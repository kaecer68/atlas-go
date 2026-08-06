# Gap Audit 4 — 28 天驗證記錄現況

> **盤查日期**: 2026-08-06
> **盤查者**: atlas AI (scout subagent, 3m36s 完成)
> **範圍**: READ-ONLY 盤查,未動任何 code
> **ACI 工具鏈**: `glob` (observation-log / spot-check / day-N / *Log*.jsonl)、`grep` (c07-obs-collector / day-evaluator / spot-check-recorder)、`read` (4 個 obs log + 2 個 eval report + 4 個 c07 CLI main.go + docker-compose.yml)

---

## 1. 背景

`docs/manifests/2026-08-06-l2-4-issue-alignment-audit.md` §1 (line 18) 指出「L2.4 觀察期從未啟動」「觀察 log 僅為範本(無真實 Day N entry)」(P3 修正: 原引用 §7,實證查證該句位於 §1 §「現況發現」,§7 為 Backlog)

問題:
- `forecast` (experimental) 是否有任何 spot-check log 累積?
- C07 sector prediction 的 obs-collector + day-evaluator 真的有 28 天資料嗎?
- L2.4 observation log 真的是空的範本嗎?
- 還有沒有其他 module 有類似的 28 天驗證基礎設施?

---

## 2. 現況發現 (Per-Pipeline 證據)

| Pipeline | 觀察 log 設計 | Cron / Docker 部署 | 真實 entry 數 | 觀察期天數 | 缺口 |
|---|---|---|---|---|---|
| **L2.4** | ✅ 範本 | ❌ DEPRECATED 0 callers | **0** | 0 | ✅ 是 |
| **C07 obs log** | ✅ 完整 | ✅ 2 cron containers | **4 row + 20 markers** | ~7 工作日 (2026-07-16→22) | ⚠️ partial — Day 14 rollback FAIL 收尾 |
| **C07 Day 7/14 eval** | ✅ 報告 | ✅ c07-day-evaluator | **2 真實評估** | 2 checkpoints | ⚠️ Day 14 rollback 未驗證 |
| **L3** | ✅ 永久 daily | ✅ Stage 3 register | **0 daily entry** | n/a | ❌ 是 — Day 1 從未填 |
| **SA11/SA12** | ✅ 範本 | ❌ SACClosureStateManager 未啟動 | **0** | 0 | ✅ 是 |
| **forecast** | ✅ Ledger | ❌ 無 docker service | **0 record** | 0 | ⚠️ 見 Gap 2 修正 — E03 設計未啟用,非 bug |
| **retail** | ❌ 無 | ❌ 無 cron | **n/a** | 0 | ⚠️ 設計不同 (rolling 3) |
| **eventdriven** | ❌ 無 | ❌ 無 | **n/a** | 0 | ⚠️ 由 C07 平行覆蓋 |
| **strategy_ranker** | ❌ 無 | ❌ 無 | **n/a** | 0 | n/a (無 28 天設計意圖) |

### 2.1 L2.4 observation log — `docs/archive/l2-4-observation-log.md`
- **Entry 數**: 0 真實 entry (僅範本 + Week 1/2 example)
- **最後更新**: 2026-08-06 由 cleanup manifest 標註「範本」
- **生產部署**: 無 — `ShouldL24AutoCronFire` 已 DEPRECATED (0 production callers per gitnexus_impact),`L2_4_AUTO_CRON_ENABLED` env gate 從未 wire 到 BackgroundTaskManager
- **內容證據**: 檔頭明確標註 ⛔「2026-08-06 狀態:本檔為範本,L2.4 觀察期從未啟動 (`use_llm_sector_agents.value=false` 自 PR #821 至今未變)」

### 2.2 C07 sector prediction observation log — `docs/archive/sector-prediction-observation-log.md`
- **Entry 數**: 4 真實 row + 20 spot-check-record markers + 2 narrative sections
- **最後更新**: 2026-07-22 (最近一筆 spot-check + manual collector run)
- **生產部署**: ✅ 真實部署 — `atlas-cron-c07-collect` (`CRON_SCHEDULE=30 15 * * 1-5`, `docker-compose.yml:337-360`) 與 `atlas-cron-c07-evaluate` (`CRON_SCHEDULE=0 9 * * 1-5`, `docker-compose.yml:368-388`) 皆為 `restart: unless-stopped`
- **4 row 內容 (原文 quote)**:

  ```
  | 2026-07-16 | 20 | 0.0% | 9    | 0 | 0 | 5  | backfilled from PR #1206 spot-check |
  | 2026-07-17 | 20 | 0.0% | 5075 | 0 | 0 | 0  | backfilled — first prod API call, cold-cache p95 |
  | 2026-07-21 | 20 | 0.0% | 11   | 0 | 0 | 0  | backfilled |
  | 2026-07-22 | 20 | 0.0% | 14   | 0 | 0 | 20 | backfilled + manual collector run |
  ```
- **重要 nuance**: 4 筆 row 中 3 筆標 `backfilled` (非 cron 自動累積),真正 cron 自動累積的 entry 約 1 筆 (2026-07-22 manual collector run);spot-check markers 20 個全集中在 2026-07-22 (operator bulk run)

### 2.3 C07 Day 7/14 eval reports — `docs/archive/sector-prediction-eval-{day7,day14-preview-2026-07-22}.md`
- **Entry 數**: 2 真實評估報告 (2026-07-22 20:10 + 22:39)
- **生產部署**: c07-day-evaluator 為 atlas-cron-c07-evaluate 內部 binary
- **內容證據**: `day7.md` 6/6 MUST PASS;`day14-preview-2026-07-22.md` **8/9 PASS,僅 rollback verified FAIL**
- **Day 14 結果 quote**: 「Result: Day 14 acceptance: SOME MUST criteria FAILED / rollback verified | manual | verified | must | ❌ FAIL | 至少一次手動測試把 flag 翻回未設置」

### 2.4 L3 observation log — `docs/archive/l3-observation-log.md`
- **Entry 數**: 0 每日 check-in entry — 僅 Day 0 Pre-flight checklist (部分 ✓) + 啟動日 bootstrap 觀察
- **設計意圖**: permanently operational (非 14 天觀察)

### 2.5 Sector allocation closure observation log — `docs/archive/sector-allocation-closure-observation-log.md`
- **Entry 數**: 0 真實 entry — 唯一行 `(待填入)`
- **觸發狀態**: `SACClosureStateManager` 已 ship 但 `data/state/sac_closure_state.json` 從未生成 → 觀察期未啟動
- **promotion gate `session_count ≥ 20` 永遠 0**

### 2.6 Forecast ledger — `internal/forecast/foreign_forecast.go`
- **Entry 數**: 0 真實 record — `data/state/foreign_forecast/` 目錄不存在
- **設計**: `Ledger` 用 `YYYYMMDD.json` 儲存每日預測 + T+1 outcome;`Calibrate` 需 ≥90 樣本 + ≥55% hit rate
- **生產部署**: 無 — 無 cron / docker service 定期寫入
- **重要修正**: 這是 E03 設計意圖的一部分,見 Gap 2 §2.3 修正說明

---

## 3. 缺口判定

| 類型 | Pipeline | 處理建議 |
|---|---|---|
| **真實缺口** | L2.4 / SA11-SA12 / forecast | L2.4/SA11-SA12 已有對應 issue (#1466);forecast 依 E03 設計未啟用 (見 Gap 2) |
| **流程缺口** | L3 Day 1 check-in 從未填、C07 提前收尾 (非 14 天完整) | 補文件填寫 (L3);C07 已 8/9 PASS,rollover verified FAIL 是已知狀態 |
| **不算缺口** | retail / eventdriven / strategy_ranker | 無 28 天設計意圖,YAGNI 違反 |

---

## 4. 對齊核心目的程度

| Pipeline | 對齊程度 |
|---|:---:|
| C07 (8/9 MUST PASS) | ✅ 中 — 核心驗證目標已部分完成 |
| L2.4 / SA11-SA12 / forecast | ❌ 低 — 從未啟動或無 cron |

整體對齊 ~50%。

---

## 5. 修正 manifest §7 F5 文字建議

F5 文字參考(P3 修正):「28 天 spot-check log 累積」實質引自 l2-4 audit manifest §1 line 22,C07 段落位於該行附近;F5 evidence 表本身(line 76)未直接寫「28 天」字樣。為求精確,以下修正文字主要對齊 §1 line 22 原文:

**修正為**:「C07 有 docker 部署 + **7 個工作日的 obs log + 20 個 spot-check markers + 2 份 Day 7/14 評估報告(8/9 MUST PASS);C07 觀察期於 2026-07-22 因 rollback verified FAIL 提前收尾**」

此修正應寫入下一份 L2.4 cleanup 收尾 manifest 的修正清單,或獨立開一個 cleanup issue。

---

## 6. 建議

1. **保留 C07 已 ship 的 28 天驗證基礎設施** (priority high — 已 production 部署 + 部分驗證通過)
2. **保留 L2.4 observation log 範本** (priority low — Issue #1466 已接手)
3. **不為 retail / eventdriven / strategy_ranker 設計新 28 天 obs log** (YAGNI 違反)
4. **不重啟 L2.4 觀察期** (Issue #1466 接手,需 USER DECISION)
5. **修正 manifest §7 F5 文字** (priority low,cleanup 工作)
6. **不啟動 forecast 90 天實驗** (依 E03 設計需先有啟用 plan)

---

## Summary

- **findings_summary**: C07 是唯一真實運轉的 28 天驗證基礎設施 (~7 工作日, 8/9 MUST PASS);L2.4/SA11/forecast 都是範本/未啟動;manifest §7 F5 文字需修正
- **is_real_gap**: true (L2.4/SA11/forecast 端),但已有 #1466 接手不重複
- **value_to_core_mission**: medium-low
- **recommended_action**: 修正 F5 文字 + 補 L3 Day 1 check-in (純文件填寫, 0.5 天);其他由 #1466 處理
