# L2.4 Issue Alignment Audit — Issues #825 / #826 vs Real System Gap

> **Phase**: A (Audit) → B (Plan) → C (Implement) → D (Close-out)
> **Trigger**: User asked "這兩個 issue 做完後的價值是什麼?我們的系統有這個缺口嗎?價值與缺口一致嗎?"
> **Skills loaded**: `atlas-pre-change-protocol` (Investigation Mode), `atlas-audit-manifest-protocol`
> **In-scope IDs**: #825, #826
> **Audit date**: 2026-08-06
> **ACI toolchain used**: gitnexus (impact/context/query), codebase-memory (search_graph/get_architecture/explore), codegraph
> **Decision**: **關閉 #825 + #826**。本 manifest 為評估報告,同時作為所有收尾工作完成後的**驗收依據**。
> **Branch**: feat/20260806-l2-4-issue-close (worktree: issue-fix-L2.4)

---

## 0. 執行摘要

Issues #825 (Auto-cron Scheduler) 與 #826 (Promotion Procedure) 的原始設計前提是「L2.4 觀察期先手動跑完 14 天,再自動化 / 提升」。盤查 2026-08-06 的實際 code 與平行軌道後,結論是:

1. **L2.4 觀察期從未啟動**:`use_llm_sector_agents.value` 仍為 `false`(configs/parameters.json:2549),觀察 log 僅為範本(無真實 Day N entry),28+ 天無進展。
2. **Issue #825 的核心交付物 (cron 觸發器) 未 wire-up**:`internal/scheduler/l2_4_auto_cron.go` 的 `ShouldL24AutoCronFire` 是純函式 library,gitnexus impact 顯示 **0 個 affected processes** — 沒有被任何 execution flow 呼叫,沒有 `BackgroundTaskManager.Register()`。
3. **Issue #826 的 promotion chain 中,3c 已預先 ship (PR #1025)**:`SemiconductorLLMAgent` 已拆分 PlanDriver + ReflectDriver;但 `LLMDriver` deprecated alias 仍在 (sector_agent_llm.go:82-85),gitnexus 顯示 22 個 direct importers 但 **0 個 affected processes**(被 import 但未被實際呼叫)。
4. **平行軌道已填補大部分缺口**:
   - C07 sector prediction 的 launch-gate + observation 自動化已真實運轉(`cmd/experimental/c07-obs-collector` + `c07-day-evaluator` + `c07-preflight`,有 docker 部署 + 28 天 spot-check log 累積)
   - L2.4 preflight 已提升為 canonical launch-gate pattern(`internal/startup/preflight.go` 的 `Preflight` / `checkClaim` 函式存在,PR #1037)
   - SA11/SA12 sector-allocation-closure 有自己的觀察 log 與 verifier
5. **唯一剩餘的真實缺口**:其他 12 個 sector 沒有 LLM-driven 變體(`loader.go` 只有 `SemiconductorLLMAgent{}` 是 LLM 變體),但這**不是 #825/#826 要解決的範圍**。

**決策**:關閉 #825 + #826,收尾工作清理 dead code 與文件同步,並在 `docs/manifests/2026-08-06-l2-4-cleanup-implementation.md` 追蹤實作。

---

## 1. 問題 (Problem)

### 1.1 原始 issue 意圖

| Issue | 意圖 | 原始交付物 |
|---|---|---|
| #825 | L2.4 觀察期自動化:消除手動 Day 0 / Day 14 按鈕 | `internal/scheduler/l2_4.go` cron 觸發器,呼叫 `l24Mgr.Start()/Stop()`,AutoEnabled 主開關,5-condition gate |
| #826 | L2.4 promotion:從 experimental 升到 default | 3a source 升級 / 3b default flip + opt-out / 3c 刪 LLMDriver alias / 3d version tag |

### 1.2 盤查問題

1. 這兩個 issue 做完後的價值是什麼?
2. 當前既有系統與功能有這個缺口嗎?
3. Issue 提供的價值與系統缺口一致嗎?
4. 過去兩週這些條件有沒有觸發 / 啟動?
5. 描述是否已過時,與當前 code 是否衝突?

---

## 2. 盤查方法 (Method) — ACI 工具鏈

| 步驟 | 工具 | 目的 |
|---|---|---|
| 1 | `codebase-memory_list_projects` | 確認索引可用 |
| 2 | `gitnexus_check` / `codebase-memory_get_architecture` | 確認索引新鮮度 + 高層架構 |
| 3 | `codebase-memory_search_graph` (×2) | 缺口候選:「sector executor LLM-driven」「observation window cron automation」 |
| 4 | `gitnexus_context` (×3) | 360° 視圖:`SemiconductorLLMAgent` / `ShouldL24AutoCronFire` / `LLMDriver` |
| 5 | `gitnexus_impact` (×2) | Blast radius:`ShouldL24AutoCronFire` (upstream) / `LLMDriver` (upstream) |
| 6 | `gitnexus_query` (×2) | C07 平行軌道執行流 + l2-4 preflight canonical pattern |
| 7 | `codebase-memory_search_graph` (×2) | C07 collector / day-evaluator 真實性 + 13 sector executor 清單 |
| 8 | grep (輔助) | 驗證 wire-up 位置(scripts/, cmd/atlas/main.go) |
| 9 | read | 觀察 log 實際內容、runbook / followup / roadmap 文件 |

> **紀律註記**:先前一輪盤查只用 grep,遺漏 call graph 與跨模組依賴。本輪強制使用 ACI 工具鏈,`gitnexus_impact` 的 affected processes 數是判斷「dead vs alive」的關鍵指標。

---

## 3. 發現 (Findings) — 證據表

| ID | 假設 | 證據 | 判定 |
|---|---|---|---|
| F1 | #825 有價值:移除手動點擊 | 觀察期未啟動,無 baseline;C07 已有同類自動化 | ❌ 價值被 C07 填補 |
| F2 | #825 的 cron 已 ship | `l2_4_auto_cron.go` 存在,但 gitnexus impact = **0 affected processes** | ⚠️ 只 ship gate 邏輯,未 wire-up |
| F3 | #826 有價值:promotion 到 default | 需要 Day 14 觀察通過,而觀察從未啟動 | ❌ 前置條件未滿足 |
| F4 | #826 3c 已 ship | PR #1025 拆分 PlanDriver/ReflectDriver;`LLMDriver` alias 仍在 (sector_agent_llm.go:82-85) | ⚠️ 部分 ship |
| F5 | L2.4 缺口被平行軌道填補 | C07 obs-collector/day-evaluator/preflight 有 production 部署 + spot-check log 累積;`internal/startup/preflight.go` 有 `Preflight`/`checkClaim` | ✅ 已填補 |
| F6 | 真實缺口 = 其他 12 sector 無 LLM 變體 | `loader.go` 只註冊 `SemiconductorLLMAgent{}` 一個 LLM 變體 | ✅ 但非 #825/#826 範圍 |
| F7 | 觀察期未觸發 | `use_llm_sector_agents.value=false` 28+ 天未變;obs log 無真實 entry | ✅ 確認為事實 |

### 3.1 關鍵 ACI 證據詳述

**ShouldL24AutoCronFire (gitnexus_impact upstream)**:
- impactedCount: 860 (transitive),但 **affected_processes: 0**,affected_modules: 0
- → 純函式 library,無執行流呼叫。dead code。

**LLMDriver (gitnexus_impact upstream)**:
- impactedCount: 77, direct importers: 22, **affected_processes: 0**
- → 被 import 但未被呼叫。deprecated alias 存在但無 runtime 效果。

**SemiconductorLLMAgent (gitnexus_context)**:
- 287+ processes 提及,但都是 config 載入鏈 (Main → Config / Main → Factory),非 L2.4 observation 觸發鏈。
- → agent 註冊在 loader,但 gate (flag off) 使其永不執行。

---

## 4. 價值 vs 缺口對齊 (Value vs Gap)

| 缺口 | L2.4 軌道是否對齊 | 誰已填補 |
|---|---|---|
| A: 其他 12 sector 無 LLM-driven 變體 | ❌ 不對齊 (#825/#826 不解) | 無人 — 真實缺口,但超出本 issue 範圍 |
| B: 觀察期自動化 cron | ❌ 不對齊 (未 wire-up) | C07 obs-collector / day-evaluator 已運轉 |
| C: launch-gate 模式標準化 | ✅ 已對齊 | PR #1037 canonicalize + `internal/startup/preflight.go` |
| D: 觀察期工具鏈 | ✅ 已對齊 | C07 + SA11/SA12 完整鏈 |
| E: LLM sector agent framework | ❌ 不對齊 (#825/#826 不解) | 無人 — 真實缺口,超出本 issue 範圍 |
| F: SemiconductorLLMAgent promotion 驗證 | ⚠️ 部分對齊 (需 T15 決策 + 觀察) | 無人 |

**結論**:#825/#826 對齊的缺口已大部分被平行軌道填補;未填補的缺口 (A/E) 超出本 issue 範圍。故 **關閉本 issue**,把剩餘工作以正確範圍重新追蹤。

---

## 5. 決策 (Decision)

1. **關閉 Issue #825** (Auto-cron):核心交付物未 wire-up,且 C07 已提供同類自動化;重新設計前無需保留。
2. **關閉 Issue #826** (Promotion):前置條件 (Day 14 觀察) 未滿足且短期無法滿足;3c 部分已完成的部分保留在 code,不另做 promotion。
3. **收尾工作** 以第二份 manifest (`2026-08-06-l2-4-cleanup-implementation.md`) 追蹤,每項先 ACI 盤查再動工。
4. **真實缺口 (A/E)** 另開新 issue 追蹤,不混入本次收尾。

---

## 6. 驗收標準 (Acceptance — 本 manifest 作為最後驗收依據)

關閉後,收尾工作完成需滿足:

- [ ] AC-1: Issue #825 與 #826 皆已關閉 (GitHub state=closed),關閉 comment 含本 manifest 連結
- [ ] AC-2: `ShouldL24AutoCronFire` / `l2_4_auto_cron.go` 的處置決定已執行 (deprecate 註記或移除),並有 ACI impact 證據
- [ ] AC-3: `LLMDriver` deprecated alias 的處置決定已執行 (保留或移除),並有 ACI impact 證據
- [ ] AC-4: L2.4 相關文件 (followup / runbook / unblocking-roadmap) 已對齊真實狀態 (issue 已關閉、C07 平行軌道、dead code 處置)
- [ ] AC-5: 真實缺口 (A/E) 已開新 issue 追蹤
- [ ] AC-6: 全部變更通過 `make ci-full` (含 go test / lint / coverage)
- [ ] AC-7: 無未 commit 的變更殘留 (working tree clean)
- [ ] AC-8: 本 manifest 的執行摘要與發現 (F1-F7) 與最終 code 狀態一致

---

## 7. Backlog (新 issue 候選)

- [ ] B-1: Generic LLM sector agent framework (缺口 E) — 讓其他 12 個 sector 可插拔 LLM driver
- [ ] B-2: 評估缺口 A 的優先級 — 其他 sector 是否真的需要 LLM-driven 變體,還是 C07 規則式已足夠
- [ ] B-3: T15 USER DECISION — staging 環境 + 觀察者 + Day 0,若 L2.4 觀察期要重啟

---

## 8. Session-end State

- **Done**: 完整 ACI 盤查,評估報告 (本文件),決策關閉 issue
- **Next**: 建立第二份 manifest (`2026-08-06-l2-4-cleanup-implementation.md`) → 分階段實作 → ci-full → PR
