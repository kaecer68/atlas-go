# L2.4 Follow-up Work Report

> **Status**: PR #821 merged 2026-06-29 (commit `f69b3551`). Manual L2.4 observation infrastructure shipped.
> **Scope**: Items deferred from PR #821 that require follow-up work, plus the scope of the docs-migration PR.
> **Audience**: atlas-go ops, on-call engineering, future maintainers.
> **Tracking**: Issue #742 (L2.4 runbook), Issue #740 (L2.4 observation metrics), Issue #711 (Wave 10 L2.3 plan), Issue #825 (Auto-cron Scheduler), Issue #826 (L2.4 Promotion Procedure).
>
> **NOTE**: 本文件為上述 GitHub Issues 的鏡像文件(mirror),以 **GitHub Issue 為唯一權威來源**(single source of truth)。若兩者內容衝突,以 Issue 為準並請更新本文。

This report covers 4 categories of follow-up work:

1. **Auto-cron Scheduler** — graduate from manual buttons to scheduled observation windows
2. **CLI Flag Wiring** — short-term usability improvement for staging
3. **Promotion Procedure** — graduate L2.4 from `experimental` to `default-true`
4. **This PR (docs migration)** — finalize the docs that PR #821 was based on

---

## 1. Auto-cron Scheduler → [Issue #825](https://github.com/kaecer68/atlas-go/issues/825)

### 實作內容

**Status (2026-07-08)**: PR #1029 ships the **defensive 5-condition gate** (`internal/scheduler/l2_4_auto_cron.go`). Cron will NOT fire unless ALL of: (1) env var `L2_4_AUTO_CRON_ENABLED=true`, (2) `parameters.AutoEnabled=true`, (3) observation log exists, (4) Day 7+ entry exists in log, (5) current time in cron window. Default-disabled in every respect. **Gate ships; actual `BackgroundTaskManager.Register()` + `l24Mgr.Start/Stop()` wiring deliberately deferred per prereq #1 + #2** (see 「是否現在可以開始實作？」 below).

Original spec for full implementation (not yet shipped):

- New `internal/scheduler/l2_4.go` package implementing:
  - Cron-style trigger reading `L2_4ScheduleParameters.DefaultStartTime` (HH:MM) on weekdays
  - On trigger: call `l24Mgr.Start()` (which is now seeded from `parameters.json`, see PR #821 commit `f2c37c61`)
  - On period expiry: call `l24Mgr.Stop()` automatically
  - Honours `AutoEnabled ParameterMetadata[bool]` as the on/off master switch
  - Manual buttons (start/stop in synergy page) take precedence; cron defers if a window is already running
- New L2.4 entry in the scheduler task list
- Updated synergy page to show "auto-cron" status badge (active / inactive)

### 目標與目的

Eliminate the manual Day 0 / Day 14 button clicks. Once the manual flow proves out (one full successful observation), the cron makes the observation repeatable and removes the operator overhead. This is the prerequisite for scaling L2.4 beyond a single observation into a periodic regime (e.g., monthly regression windows).

### 創造的價值

- **省人為操作錯誤**: Manual start/stop is racy — operator clicks "start" 1 minute late and loses data. Cron triggers exactly at HH:MM every trading day.
- **可重複執行**: 觀察期可重複跑 (Day 30, Day 60, …),讓 L2.4 從一次性驗證變成持續監控機制。
- **降低 Day 0 啟用門檻**: 新進 ops 不需要看 runbook 才知道要按按鈕。

### 是否現在可以開始實作？

**部分**。Original answer was 「否」(2026-06-29 PR #824);reasoning 仍成立 for prereq #1 + #2:

- Prereq #1 + #2 仍未滿足 — Manual flow (PR #821) 尚未被任何 staging 跑過一輪完整 7-14 天,沒有成功/失敗的 baseline;`AutoEnabled` 開關測試需要 staging 環境(目前 repo 無 staging compose)。
- Cron 故障模式 (missed trigger、duplicate trigger、out-of-window start) 需先有手動流程當 comparison baseline 才能 debug
- Risk: 自動 cron 在沒驗證時 launch = 在 production 跑未驗證邏輯

**但 PR #1029 ship 了 defensive gate 作為 defensible-interpreted 部分實作**:gate 邏輯在 main,但要讓 cron 真正 fire 還是要 prereq #1+2(觀察期成功 + staging 測試)。Net effect:即使有人 merge PR #1029 + 亂設 env var,cron 仍 no-op 直到 Day 7 entry 出現。

### 需要什麼條件才能實作？

1. ⏳ **手動觀察期成功完成一次**: 至少一次 Day 7 / Day 14 通過(可用任何 sector / symbol)
2. ⏳ **`AutoEnabled` 開關測試**: 在 staging 把 `auto_enabled` 翻 `true`,驗證 plugin 不會在 `false` 時觸發
3. ✅ **排程 fault tolerance** (2026-07-08 完成,[PR #1023](https://github.com/kaecer68/atlas-go/pull/1023)) — 降級策略已 ship(trigger 失敗時 log warning 不 panic、scheduler 重啟後恢復 in-flight window)
4. ✅ **runbook §2 改寫** (2026-07-08 完成,[PR #1024](https://github.com/kaecer68/atlas-go/pull/1024)) — Daily Check-in 已改為「review auto-triggered window」流程

**預估時程**: 1 個獨立 PR,中等工作量(2-3 個 review agents 平行)。僅在 prereq #1+2 滿足後啟動;PR #1029 的 defensive gate 可作為前置準備。

---

## 2. CLI Flag Wiring (`--use-llm-sector-agents`) → [PR #828](https://github.com/kaecer68/atlas-go/pull/828) (plan + scaffold)

### 實作內容

- In `cmd/atlas/main.go`, add flag parsing:
  ```go
  useLLMSectorAgents := flag.Bool("use-llm-sector-agents", false,
      "enable LLM-driven sector agent (overrides config parameter)")
  flag.Parse()
  ```
- At config load, if flag is set, override `cfg.Orchestrator.UseLLMSectorAgents.Value = true`
- Update `scripts/atlas` wrapper (if exists) to pass through
- Update `docs/quickstart.md`「啟用觀察期」一節加上 CLI 範例

### 目標與目的

Dev / on-call 可以**不必 commit config 變更**就啟用 L2.4,加快 staging / canary 測試。對 ops 來說,CLI flag 是比編輯 `configs/parameters.json` + 重啟更安全的啟用手段(可審計、容易 roll-back)。

### 創造的價值

- **可審計性**: CLI invocation log 留下 trace;config file 編輯則依賴 git history
- **快速迭代**: 切 canary 群組時不必等 commit + review cycle
- **roll-back 一鍵化**: 同一個 binary 透過不同 flag 啟用 / 停用,不需要修改 config file

### 是否現在可以開始實作？

**可以,短期內**(下個 sprint 內)。難度低、風險低、無前置依賴。

### 需要什麼條件才能實作？

1. 評估誰會用 (dev sandbox / staging / on-call? — 決定 flag 的 default 行為)
2. 確認 `flag.Parse()` 在 atlas binary 的執行順序 (避免在 `config.Load()` 之後才 parse)
3. `docs/quickstart.md` 加上 CLI 範例 (PR 一起)

**預估時程**: 0.5 天工作量,單一 commit,單一 PR

---

## 3. Promotion Procedure (Runbook §5) → [Issue #826](https://github.com/kaecer68/atlas-go/issues/826)

PR #821 已經把 L2.4 從「設計階段」推進到「可手動啟用階段」。但還沒進入「production default」階段。本節列出 promotion 流程的 4 個步驟,每步都是獨立 PR。

### 3a. Source 升級:`experimental` → `empirical`

**實作內容**: `configs/parameters.json` 把 `orchestrator.use_llm_sector_agents.source` 從 `experimental` 改為 `empirical`。`value` 暫不動(仍是 `false`,等下一階段才翻 default)。

**目標**: 標記 L2.4 已從「實驗中」進入「實證階段」。這是 governance 訊號,讓其他 agent / dashboard 可以基於 `source: empirical` 做不同的決策。

**價值**:
- **可追蹤性**: 系統狀態在 git history 有時間戳
- **可審計性**: `ParameterMetadata.Source` 從 `experimental` 升級到 `empirical` 是有 review 紀錄的
- **降耦合**: 後續 Darwinian 權重 / 自動 cron 可以基於 `source` 判斷要不要啟用

**實作時機**: Day 14 通過後立刻做。

**前置條件**:
- Day 7 / Day 14 acceptance criteria 全部 pass(見 `docs/operations/l2-4-runbook.md` §3)
- Runbook §4 rollback 未被觸發

**預估時程**: 10 分鐘,1 個 commit,單一 PR

---

### 3b. Default Flip to `true` + 新增 opt-out flag

**實作內容**:
- `configs/parameters.json` 把 `orchestrator.use_llm_sector_agents.value` 從 `false` 改為 `true`
- 新增 `orchestrator.use_llm_sector_agents_deprecated` 旗標(預設 `true`),讓短期需要回退的環境可以快速 opt-out
- 更新 `internal/orchestrator/orchestrator.go` 的 `Supports()` gate 邏輯:若 `use_llm_sector_agents_deprecated` 為 `true`,fall back 到 deterministic
- 更新 synergy page 顯示 deprecation 警告 + opt-out toggle

**目標**: 把 LLM-driven sector agent 設為 production default。短期 opt-out flag 給 ops 一個 escape hatch。

**價值**:
- **零操作**: 不需每次翻旗標,L2.4 預設就是 on
- **escape hatch**: 萬一 L2.4 在 production 出問題,翻 deprecated flag 即可暫停(不需改 config + 重啟)
- **漸進式遷移**: 從 opt-in 轉 opt-out 給 L2.4 maximum surface area,讓 Darwinian 權重有最多資料

**實作時機**: Source 升級(3a)之後獨立 PR。

**前置條件**:
- 3a 完成
- 至少一次 Day 14 完整通過
- Deprecated flag 設計 review 過(避免變成永久 feature flag)

**預估時程**: 1-2 天工作量,單獨 PR + deprecation warning 的 CSS 更新

---

### 3c. LLMDriver Deprecated Alias 移除

**實作內容**:
- 從 `internal/orchestrator/sector_agent_llm.go` 刪除 `LLMDriver` 單一介面 deprecated alias
- 確認所有呼叫端都改用 `SectorAgentLLMDriver`(包 `PlanDriver` + `ReflectDriver`)
- 更新 `internal/orchestrator/AGENTS.md` 移除相關 warning

**目標**: 清理 deprecated code,讓 `SectorAgentLLMAgent` 結構的 LLM 介面字段明確為 `PlanDriver` + `ReflectDriver`(非 deprecated `LLMDriver`)。

**價值**:
- **程式碼清晰**: 移除 deprecated 入口,新 code 不會誤用
- **編譯期保護**: 介面收斂後,未來想加新 LLM backend 必須實作 `PlanDriver` + `ReflectDriver`,不會走捷徑
- **AGENTS.md 簡化**: 移除「不可用 LLMDriver」的告警(不再有 deprecated alias)

**實作時機**: 3b 之後,所有 L2.4 路徑都驗證跑過一輪後。

**前置條件**:
- 3b 完成 + production 跑過 7+ 天
- ✅ 確認 `grep -r LLMDriver internal/` 沒有非測試用法 — 2026-07-08 audit done ([PR #1024](https://github.com/kaecer68/atlas-go/pull/1024));所有 callers 已改用 `SectorAgentLLMDriver`
- ✅ `sector_agent_llm.go` 介面 split — 2026-07-08 done ([PR #1025](https://github.com/kaecer68/atlas-go/pull/1025)) — `SectorAgentLLMAgent` 已拆出 `PlanDriver` + `ReflectDriver` 兩個獨立介面
- `sector_agent_llm_test.go` 改用新介面

**預估時程**: 0.5 天工作量,純 refactor(無行為變更),需要完整 test 驗證無 regression

**Note**: PR #1025 已 ship 介面 split,但 `LLMDriver` deprecated alias 仍在(向後相容);正式移除要等 3b 後 production 跑 7+ 天無 regression。

---

### 3d. Version Tag

**實作內容**: 標記 release version(`v0.0.0.22` 或 `v0.1.0`,依當時累積變更決定)。Update `CHANGELOG.md` 詳列 L2.4 變更(orchestrator exhausted field + L2.4 scheduling API + synergy page panel + SetConfig seed)。

**目標**: 標記 L2.4 promotion 至 default 的 release boundary,提供 release notes。

**價值**:
- **可追蹤版本**: 客戶 / 維運可定位 L2.4 啟用的版本
- **release notes 集中**: CHANGELOG.md 一次寫齊所有變更,避免散落在多個 commit
- **Roll-back 邊界**: 若新版有問題,可 revert 到 tagged version

**實作時機**: 3a + 3b + 3c 都 merge 後。

**前置條件**: 3a/3b/3c 都 merged。

**預估時程**: 30 分鐘工作,git tag + CHANGELOG 更新 + 1 個 PR

---

## 4. This PR — L2.4 Docs Migration

### 實作內容

This PR 把原本位於 `.omo/wave-11-l2-4/`(gitignored,本地工作目錄)的兩份 L2.4 規劃文件永久化到官方 `docs/`:

- `.omo/wave-11-l2-4/L2_4_RUNBOOK.md` → `docs/operations/l2-4-runbook.md`
  - 清理 file path references (被 PR #821 改動後已移位)
  - 加 frontmatter 對齊 `docs/quickstart.md` 風格
  - 更新 metric 來源 references (從 PR #743 改為 PR #821 merge commit `f69b3551`)
- `.omo/wave-11-l2-4/L2_4_OBSERVATION.md` → `docs/specs/l2-4-observation-spec.md`
  - 拆掉 Promotion / Risk 段落(重複,在 Runbook)
  - 專注在 metrics schema(下游客戶端的 single source of truth)
  - 加 Issue #740 cross-reference
- 新索引紀錄:
  - `docs/reference/guidelines-index.md` 加 L2.4 條目
  - `docs/documentation-map.md` 加 L2.4 文件地圖條目
- `docs/specs/llm-sector-agent-spec.md` 加 L2.4 follow-up cross-link
- 刪除 `.omo/wave-11-l2-4/*.md`(內容已永久化)

> **Note**: 觀察日誌 scaffold 未列入本 PR 範圍,將於後續獨立 PR 處理,避免把半成品 observation log 與 migration 混在同一 commit。

### 目標與目的

`.omo/` 是 gitignored,只給單一 session 用。L2.4 runbook + observation spec 是 **operational/reference 文件**,需要被 ops / on-call / 下游 log 消費端 long-term 看到。永久化到 `docs/` 後:
- 任何 clone 都能看到完整 L2.4 操作手冊
- 與既有 `docs/specs/`、`docs/quickstart.md` 整合
- 索引在 `guidelines-index.md` 中可達

### 創造的價值

- **可達性**: 新進 ops clone repo 後立刻看到 L2.4 runbook,不需要 git archaeology
- **單一權威**: Metrics schema 只在 `docs/specs/l2-4-observation-spec.md`,避免散落多份 doc 衝突
- **可審計性**: PR 從 `.omo/` 到 `docs/` 的轉換有 git history 紀錄

### 是否現在可以開始實作？

**是,本 PR 本身就是**。

### 需要什麼條件才能實作？

無 — PR #821 已 merge,docs 遷移是純文件變更,0 個依賴。

---

## 5. 總時程與優先序

| 項目 | 預估工時 | 優先序 | 阻塞 | 狀態 (2026-07-08) |
|------|---------|--------|------|------|
| This PR (docs 遷移) | 1-2 hours | **現在** | 無 | ✅ 已 ship |
| CLI flag wiring | 0.5 day | 下一個 sprint | 無 | ✅ Shipped ([PR #1021](https://github.com/kaecer68/atlas-go/pull/1021)) |
| Auto-cron scheduler | 2-3 days | Day 14 之後 | 觀察期成功 1 次 | 🟡 Defensive gate shipped ([PR #1029](https://github.com/kaecer68/atlas-go/pull/1029));full impl 等 prereq #1+#2 |
| Source 升級 (3a) | 10 min | Day 14 之後 | Day 14 通過 | ⏳ 未啟動 |
| Default flip (3b) | 1-2 days | 3a 之後 | 3a | ⏳ 未啟動 |
| LLMDriver 移除 (3c) | 0.5 day | 3b 之後 7+ 天 | 3b + production 7+ 天 | 🟡 Split done ([PR #1025](https://github.com/kaecer68/atlas-go/pull/1025));deprecated alias 仍保留(向後相容),等 3b |
| Version tag (3d) | 30 min | 3a/3b/3c 都完成 | 3a/3b/3c | ⏳ 未啟動 |

**Note**: 此表反映 2026-07-08 實際 ship 狀態。後續 prereq 達成後逐項更新。

## 6. References

- `docs/operations/l2-4-runbook.md` — 觀察期操作手冊(本 PR 永久化)
- `docs/specs/l2-4-observation-spec.md` — 觀察指標 schema spec(本 PR 永久化)
- `docs/specs/llm-sector-agent-spec.md` — L2.3 LLM-driven sector agent 設計
- Issue #711 — Wave 10 L2.3 plan
- Issue #740 — L2.4 observation metrics
- Issue #742 — L2.4 runbook tracking
- Issue #825 — Auto-cron Scheduler follow-up
- Issue #826 — L2.4 Promotion Procedure follow-up
- PR #821 — L2.4 observation scheduling API + admin panel + SetConfig seed (commit `f69b3551`)
- PR #828 — L2.4 CLI Flag Wiring plan + scaffold (commit `5f722d10`)
