# L2.4 Cleanup Implementation Manifest

> **Phase**: B (Plan) → C (Implement) → D (Close-out) — **全部完成 2026-08-06**
> **Trigger**: User decision — 關閉 Issue #825 + #826,依第一份 manifest (`2026-08-06-l2-4-issue-alignment-audit.md`) 驗收
> **Branch**: feat/20260806-l2-4-issue-close (worktree: issue-fix-L2.4)
> **Date**: 2026-08-06
> **Commits**: 2fc7ce56 (manifests) / 74e5830e (M2) / c99e531d (M3) / d6cffd11 (M4) / 1f28afa5 (gitignore)
> **CI**: `make ci-full` 通過 (coverage 67.8%, 全測試 green)

---

## 0. ACI 盤查紀律 (MANDATORY — 每個主項目動工前必讀)

> 此紀律是本 manifest 的**最高優先級規則**。違反 = 該主項目失敗,退回重做。
> 目的是防止「做到一半就偷懶跳過 ACI,退回 grep-only 盤查」的錯誤重演。

**每個主項目動工前,必須依序完成:**

1. **Skill 載入**:讀 `skill://atlas-pre-change-protocol` (Investigation Mode) + `skill://atlas-audit-manifest-protocol`
2. **Blast radius**:`gitnexus_impact({target: "<symbol>", direction: "upstream"})`
   → 記錄 affected_processes 數 (0 = dead, >0 = alive, 需列出哪些 process)
3. **重疊偵測**:`codebase-memory_search_graph({query: "<concept>"})`
   → 確認沒有平行實作已存在
4. **360° 視圖**:`gitnexus_context({name: "<symbol>"})` — 需要時
5. **Source 確認**:`codebase-memory_explore` 或 `read` 拿逐行 source,不重開已讀檔案
6. **grep 僅輔助**:只用來確認 wire-up 位置 (scripts/, cmd/atlas/main.go 等),禁止作為主要盤查手段

**驗證輸出**:每個主項目完成後,在該項目 section 記錄:
- 用到的 ACI 工具與查詢
- affected_processes 數
- 處置決定 + 理由
- 測試 / ci-full 結果

---

## 1. 主項目清單 (Phases)

| ID | 項目 | 類型 | 對齊缺口 | Acceptance |
|---|---|---|---|---|
| M1 | 關閉 Issue #825 + #826 | 流程 | — | GitHub state=closed + comment 含 manifest 連結 |
| M2 | `ShouldL24AutoCronFire` dead code 處置 | Code | B | 決定 deprecate 或移除,有 ACI 證據 |
| M3 | `LLMDriver` deprecated alias 處置 | Code | — | 決定保留或移除,有 ACI 證據 |
| M4 | L2.4 文件對齊真實狀態 | Doc | C/D | followup / runbook / roadmap 反映 issue 已關閉 + C07 平行軌道 |
| M5 | 真實缺口 (A/E) 開新 issue | 流程 | A/E | 新 issue 建立,追蹤缺口 A/E |
| M6 | 全量測試 + 驗收 | Verification | — | make ci-full 通過 + 對第一份 manifest AC-1..AC-8 |

---

## 2. M1 — 關閉 Issue #825 + #826

### 2.1 動作
- `gh issue close 825 --comment "<manifest 連結 + 摘要>"`
- `gh issue close 826 --comment "<manifest 連結 + 摘要>"`
- 關閉 comment 需引用 `docs/manifests/2026-08-06-l2-4-issue-alignment-audit.md`

### 2.2 Acceptance
- `gh issue view 825` / `826` → state=closed
- comment 存在且含 manifest 連結

### 2.3 驗證輸出
- ✅ #825 CLOSED (2026-08-06),comment 含 manifest 連結
- ✅ #826 CLOSED (2026-08-06),comment 含 manifest 連結
- `gh issue view 825/826 --json state` → CLOSED

---

## 3. M2 — ShouldL24AutoCronFire dead code 處置

### 3.0 ACI 盤查 (動工前必做)
- [ ] `gitnexus_impact({target: "ShouldL24AutoCronFire", direction: "upstream"})` — 已知 0 affected processes
- [ ] `codebase-memory_search_graph({query: "auto cron observation window"})` — 確認 C07 已覆蓋
- [ ] `gitnexus_context({name: "ShouldL24AutoCronFire"})` — 確認 callers
- [ ] `grep` (輔助) — scripts/check_no_duplicate_preflight.sh:33 有引用 `internal/scheduler/l2_4_auto_cron.go`(會影響 M2 處置)

### 3.1 處置選項 (依 ACI 結果選擇)
- **Option A (deprecate)**:在檔頭加 deprecation 註記,指向 C07 自動化。保留 code 與 test。
- **Option B (移除)**:刪除 `l2_4_auto_cron.go` + `l2_4_auto_cron_test.go`,並更新 `scripts/ci/check_no_duplicate_preflight.sh` 移除該路徑。
- **Option C (wire-up)**:把 gate 接到 `BackgroundTaskManager` — **不建議**,因 issue 已關閉,觀察期未啟動,C07 已覆蓋。

### 3.2 Acceptance
- ✅ 處置決定有 ACI 證據:`ShouldL24AutoCronFire` gitnexus impact = **0 affected processes**;grep 確認 production 0 caller (僅 test)
- ✅ **Option A (deprecate) 執行**:檔頭加 DEPRECATED 註記 (指向 C07 覆蓋 + issue 關閉),保留 code + test 供未來復用
- ✅ `go test ./internal/scheduler/` pass;ci-gate pass (commit 74e5830e)

---

## 4. M3 — LLMDriver deprecated alias 處置

### 4.0 ACI 盤查 (動工前必做)
- [ ] `gitnexus_impact({target: "LLMDriver", direction: "upstream"})` — 已知 22 direct importers,0 affected processes
- [ ] `codebase-memory_search_graph({query: "PlanDriver ReflectDriver SectorAgentLLMDriver"})` — 確認新介面已存在
- [ ] `read internal/orchestrator/sector_agent_llm.go:76-116` — 看 alias 定義 + callers
- [ ] `grep -rn "LLMDriver" --include="*.go" internal/` — 列全部引用點

### 4.1 處置選項
- **Option A (保留)**:deprecated alias 保留,加更強註記。理由:promotion 未完成,移除需動 22 個 importers。
- **Option B (移除)**:刪 alias + 更新 22 個 importers 用 `SectorAgentLLMDriver` + 刪 test `TestSectorAgentLLM_LLMDriver_DeprecatedAlias` + 更新 AGENTS.md。

### 4.2 Acceptance
- ✅ 處置決定有 ACI 證據:`LLMDriver` gitnexus impact = 22 direct importers (package 級) / **0 affected processes**;grep 確認 production 0 type usage (僅定義 + test 斷言)
- ✅ **Option B (移除) 執行**:刪 interface (sector_agent_llm.go) + 刪 test `TestSectorAgentLLM_LLMDriver_DeprecatedAlias` + 清 AGENTS.md 2 條警告
- ✅ 移除後 grep 驗證無 `LLMDriver` type 殘留;`go test ./internal/orchestrator/` pass;ci-gate pass (commit c99e531d)

---

## 5. M4 — L2.4 文件對齊真實狀態

### 5.0 ACI 盤查 (動工前必做)
- [ ] `codebase-memory_search_graph({query: "l2-4 followup runbook roadmap"})` — 找出所有引用 L2.4 的文件
- [ ] read 下列文件確認現況:
  - `docs/archive/l2-4-followup.md`
  - `docs/operations/l2-4-runbook.md`
  - `docs/operations/l2-4-unblocking-roadmap.md`
  - `docs/archive/l2-4-observation-log.md` (範本狀態)

### 5.1 變更內容
- 文件狀態列更新:issue #825/#826 → **closed** (不再「BLOCKED on T15」)
- 補充 C07 平行軌道已填補缺口的說明
- 觀察 log 標註「範本,觀察期未啟動」
- 移除或標記過時的「auto cron deferred」等敘述

### 5.2 Acceptance
- ✅ 5 份 L2.4 文件更新:followup (status + §1 + §3c + §5 表)、unblocking-roadmap (狀態表 + Step 6/7 + banner)、observation-log (範本標記)、runbook (status + §5 step 3)、fault-tolerance-design (closure note)
- ✅ 無「BLOCKED on T15」「等 3b」「deprecated alias 仍在」等過時 claim (grep 驗證)
- ✅ ci-gate pass (commit d6cffd11)

---

## 6. M5 — 真實缺口 (A/E) 開新 issue

### 6.0 ACI 盤查
- [x] 用第一份 manifest §7 Backlog (B-1/B-2/B-3) 作為新 issue 內容來源
- [x] 確認無重複 issue (gh issue list 搜尋) — 無重複

### 6.1 動作
- ✅ `gh issue create` → **Issue #1466**: 「[gap] L2.4 收尾後的真實缺口:A (其他 sector LLM 變體) + E (generic LLM framework) + T15 決策」

### 6.2 Acceptance
- ✅ Issue #1466 存在 (OPEN),body 含缺口分析 + 引用 manifest

---

## 7. M6 — 全量測試 + 驗收

### 7.1 全量測試
```bash
make ci-full
```
涵蓋:gofmt → build → vet → generate drift → golangci-lint → staticcheck → go test → go test -race → cmd/atlas 整合測試 → shell script 檢查 → coverage ≥ 60% → orphan artifact 檢查

**Result (2026-08-06)**: ✅ **全部通過** — coverage 67.8% (≥60%),go test / race / lint / staticcheck / integration / layer3-snap / markdown links / orphan check 全 green。

### 7.2 驗收 (對第一份 manifest)
逐項核對 `docs/manifests/2026-08-06-l2-4-issue-alignment-audit.md` §6 AC-1..AC-8:
- [x] AC-1: issue 已關閉 + comment 含 manifest 連結 — #825/#826 `state=CLOSED` (2026-08-06),comment 引用 audit manifest
- [x] AC-2: ShouldL24AutoCronFire 處置有 ACI 證據 — gitnexus impact 0 affected processes;Option A (deprecate) 已執行,檔頭註記指向 C07
- [x] AC-3: LLMDriver 處置有 ACI 證據 — 22 direct importers / 0 affected processes;Option B (移除) 已執行,interface 與 deprecated test 皆刪除
- [x] AC-4: 文件對齊 — followup / runbook / roadmap / observation-log / fault-tolerance-design + AGENTS.md 皆更新 (PR #1467 modified 清單)
- [x] AC-5: 新 issue 建立 — Issue #1466 OPEN,body 含缺口 A/E 分析 + T15 決策
- [x] AC-6: make ci-full 通過 — §7.1 記錄 coverage 67.8%,全測試 green
- [x] AC-7: working tree clean — 合併時無 L2.4 變更殘留;僅餘測試產生的 runtime state (`cmd/cron-quote-backfill/data/`,非本軌道產物)
- [x] AC-8: 發現 F1-F7 與最終 code 一致 — dead code 已標註/移除,文件反映 issue 已關閉 + C07 平行軌道

### 7.3 Push + PR
```bash
git push -u origin feat/20260806-l2-4-issue-close
gh pr create --title "chore(l2-4): close #825 #826 + cleanup dead code + docs alignment" --body "See docs/manifests/2026-08-06-l2-4-issue-alignment-audit.md + 2026-08-06-l2-4-cleanup-implementation.md"
```

---

## 8. Commit 紀律

- 每主項目一個 commit,格式 `<type>(l2-4): #<issue> <描述>`
- 例: `docs(manifest): #825 #826 關閉評估報告`、`chore(scheduler): #825 deprecate ShouldL24AutoCronFire dead code`、`docs(l2-4): #826 文件對齊真實狀態`
- 每 commit 前跑 `make ci-gate`,每主項目完成跑 `make ci-full`
- 禁止合併多個主項目到一個 commit (除非純文件)

---

## 9. Red Flags

- 「這只是小改,不用跑 ACI」→ 違反 §0,退回
- 「grep 就夠了」→ 違反 §0,退回
- 「ShouldL24AutoCronFire 沒人 call,直接刪」→ 未先跑 gitnexus_impact,退回
- 「LLMDriver 22 個 importers 太多,先不動」→ 未先評估 Option A/B,退回
- 「改完不跑 make ci-full」→ 違反 §7,退回
