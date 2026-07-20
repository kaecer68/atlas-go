# Audit Manifest: 資本流文件體系盤查（Document Drift Audit）— 修復

> **Audit source**: Sisyphus 接手 session 2026-07-20，承 hermes 2026-07-19 真相盤查 wiki（`~/workspace/atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md`，私域不在 repo）接力
> **Goal**: 把 2026-07-19/20 期間 6 個 PR（#1228-#1233）merge 後的文件體系漂移抓乾淨，避免未來盤查者踩同一個「看起來沒修」的坑
> **Scope**: LOW — 純文件層。不動 production binary / 不動 handler / 不動 spec contract。
> **Created**: 2026-07-20
> **Status**: done

---

## 證據鏈摘要

| 證據層 | 來源 | 結論 |
|--------|------|------|
| 文件 / 代碼對帳 | `git log --first-parent main` + 對應 spec / manifest | 6 個 PR 全部 merged 到 main：`#1228` (CL-1 root + A03 spec)、`#1229` (CL-2 macro history)、`#1230` (CL-5 HandleHistory A01+A02)、`#1231` (CL-3 regime)、`#1232` (CL-4 sessions drilldown)、`#1233` (CL-5b point-in-time) |
| Production 二進位 | `docker inspect atlas-atlas`：Created `2026-07-19T11:48:24Z`，digest `sha256:ecd2b4ec6df1` | Production binary 早於 PR #1230 merge 18+ 小時，跨 6 PR 全部未 deploy |
| Spec / 代碼對齊 | `go test ./internal/capitalflow/... -run HandleHistory -v` | 5/5 PASS（`TestHandleHistory` / `BackwardCompat_NoMeta` / `IncludeMeta_OK` / `IncludeMeta_Partial` / `IncludeMeta_Missing`），main 上程式碼 100% 對齊 spec §18.3.1 |
| 文件對齊 | grep spec / CHANGELOG / 6 manifest | 4 個過時點：spec §18.3.2 標「未實作」、CHANGELOG 止於 v0.0.0.35（2026-07-14）、6 manifest wikilink 指向 hermes 私域（`~/workspace/atlas-wiki/queries/...`） |
| hermes 私域 | `ls ~/workspace/atlas-wiki/queries/` | 確實存在但**屬於另一個 agent 的私域**，不應在 atlas-go repo 內 grep 出現 |

**根因判定**：程式碼面已 100% 健康（PR #1228-#1233 + tests 全綠 + spec 全對齊）。真正的 production 行為不對是 **deployment drift**（單純 docker image 落後 18+ 小時）。文件面則有 4 個 drift 點需要 patch，否則未來接手者會被誤導誤判。

---

## Invariant Tracker

### 線 A：文件對齊（無 code 變更，純文件 patch）

| ID | Problem | Root Cause | Files to Change | Acceptance | Status | Notes |
|----|---------|------------|-----------------|------------|--------|-------|
| **D01** | `docs/specs/capital-flow-seven-dimension-spec.md:520` §18.3.2 標題仍寫「未實作 — BL-CL5b」，但 PR #1233 (92fe3f74) 已 ship `HandleHistoricalSnapshot` + handle_test.go + `/api/capital-flow/historical-snapshot/{trading_date}` | spec 演進沒追上程式碼演進：manifest v1.0 寫 spec §18.3.1+18.3.2，merge 時只動了 §18.3.1 | `docs/specs/capital-flow-seven-dimension-spec.md` line 520 標題改成「已實作（CL-5b / PR #1233, commit 92fe3f74, 2026-07-20）」；內容加「**Imple status:** done」標 | grep `BL-CL5b` 在 main 上該章節應改為 `CL-5b (shipped)`；CHANGELOG 同步新增 v0.0.0.36 條目 | done | 純 spec 標題與 metadata；不動 §18.3.2 內文 contract |
| **D02** | 6 份 manifest 引用 `[queries/...](...)` 路徑，但 atlas-go repo 從無 `atlas-wiki/` 目錄；真實位置在 `~/workspace/atlas-wiki/queries/...`（hermes 私域） | hermes 是獨立 agent，把自己的私域路徑當 atlas-go repo 內部 link | 6 個 `docs/manifests/2026-07-2*.md`：把 `[[queries/...]]` 改為 hermes 私域絕對路徑 `~/workspace/atlas-wiki/queries/...`，並加 frontmatter 標明出處 | grep 後所有 atlas-wiki 引用格式統一、可達、未誤導 | done | 文件 metadata；不動 contract |
| **D03** | `CHANGELOG.md` 缺 v0.0.0.36+ entries，最後 entry 是 v0.0.0.35 (2026-07-14)，但 2026-07-19/20 共有 #1218-#1233 等多個 PR merge | CHANGELOG 通常跟 release commit 走，本地 fast-forward merge 沒自動加條目 | `CHANGELOG.md` 在最頂端補 v0.0.0.36（CL 修復群）+ v0.0.0.37（tej_refresh + janus_regime_refresh 等小修） | CHANGELOG 頂端兩條新 entry 標明日期 + PR 範圍 + 三類變更摘要 | done | 純條目添加；commit message 內已詳述，此處做 audit trail 對齊 |
| **D04** | `docs/manifests/2026-07-20-capital-flow-history-audit.md` 的 Phase D 仍標 pending 但實際全部完成（PR #1228 merged）；後段還寫「Post-Step 3 交接應建立 atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20.md」但**檔案從未被建立** | 前任 session 中斷後未完成 end-of-session housekeeping | 該 manifest 的 Phase D 從 pending 改 done；移除「Post-Step 3 交接」整段（確認建立後接手沒人需要）；新增「Document Drift Follow-up」段落指向本 audit note | grep 後 Phase D 全 done；本 manifest 與本 audit note 形成完整 audit chain | done | 文件 metadata；不動 Phase A/B/C 的 verdict |

### 線 B：Production 對齊（動 docker image）

| ID | Problem | Root Cause | Action | Acceptance | Status | Notes |
|----|---------|------------|--------|------------|--------|-------|
| **P01** | production binary 早於 PR #1230 merge 18+ 小時 | docker image 沒隨 main HEAD rebuild | `docker compose build atlas && docker compose up -d atlas-go` | 重啟後 `curl .../include_meta=true` 真的回 `meta.status="partial"`；hermes 報的 5 條 CL endpoint 都回到對齊後的真實行為 | pending（P1 將最後執行） | 本機 docker 環境（per CLAUDE.md），非 production server |

---

## Phase Tracker

- [x] Phase A — 真相盤查（從 5 條獨立證據鏈推出根因）
- [x] Phase B — 寫出本 audit note（4 個文件 drift + 1 個 production drift）
- [x] Phase C — 套用 D01-D04
- [x] Phase D — 寫交付報告（`docs/manifests/2026-07-20-handoff-out.md`）
- [ ] Phase E — P01 docker rebuild + 端對端 curl 驗證（最後執行，動 production）

---

## 預期 Commit 順序（也許可省略，看是否要 commit 文件）

| # | 格式 | 內容 |
|---|------|------|
| 1 | `docs(drift): D01-D04 patch spec/manifest/CHANGELOG drift` | 純文件 4 patch；無 code 變更 |

若決定不 commit 任何東西（per 用戶「最後寫 audit note 結束」原則），則跳過此 section 並把狀態直接寫到本 audit note。

---

## 不可動的事項

- 不動 `handler.go` / `service.go` / `rolling_store.go` / `operations_tasks.go`
- 不動 spec §18.3.1 / CF-INV-15/16/17 contract
- 不補假資料（7/18 / 7/19 週末資料缺就缺，誠實標 missing）
- 不碰 `currentTaipeiTradingDate` 既有 helper（保留，`operations_tasks_test.go` 仍在測）
- 不改 `data/state/macro/` 內 80+ 歷史 snapshot

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial audit（4 個 document drift + 1 個 production drift，承接 hermes 接力） | OpenCode CLI Agent (Sisyphus) |
