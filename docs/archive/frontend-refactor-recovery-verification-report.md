# frontend-refactor recovery 真實路徑 (post-verification 2026-07-04, two rounds)

> 設計 todo 基於 `.omo/notepads/frontend-refactor-recovery.md`(原存於 `docs/branch-hygiene/` 子目錄,2026-07-03 PR #933 移至此處)B/D 段項目。經 2026-07-04 **兩輪 grep 驗證** 後修正。
> 結論: **11 items 中 8 個已 ✅ 完成, 3 個改為正式關檔 (B-1 + B-2 + D-2), 沒有實際實作工作需要做**。

## 1. 驗證結果摘要(兩輪修正後)

| Item | 原始 recovery.md claim | 第 1 輪 (m00102) | 第 2 輪 (m00110-112) **最終** |
|---|---|---|---|
| **B-1 (e2e verification)** | 3 endpoints `/api/agents/observatory` / `/api/metrics/trend` / `/api/prism/results` 缺驗證 | "spot-check needed" | "frontend **0 calls** to these paths; backend uses different paths (`/api/dashboard/agent-observatory` etc.). **Recovery.md claim WRONG; was user misreport**" → **FORMALLY CLOSE** |
| **B-2 (user misreport 確認)** | 3 modules (`liveRadar`/`strategiesContent`) 報案需確認 | "closure recommended" | unchanged → **FORMALLY CLOSE** (需 user 確認後存檔) |
| **D-2 (dead code scan)** | 5 components 可能有 dead code | "5 dead components found" | "all 5 have refs from admin_web/component-init.js + main.js + dist/meta.json. **My m00102 scan WRONG** (只搜 shared_web/, missed admin_web/ + client_web/ refs)" → **NOT a gap; formal closure recommended** |

## 2. 修正版建議事項

### Item-1: B-2 正式關檔
- **動作**: 在 recovery.md(目前 `.omo/notepads/`)新增 section "Closed Items", 記錄 B-2 為「已用戶確認為誤報」
- **前置**: user 口頭或書面確認 macroRadar 已涵蓋 liveRadar 報案 + page-strategies 已涵蓋 strategiesContent 報案
- **估時**: 0h (等 user 確認後我執行)
- **action**: 將 recovery.md B-2 段移至 "Closed (用戶確認誤報)" section

### Item-2: D-2 正式關檔
- **動作**: 在 recovery.md 新增 "Closed Items" section, 記錄 D-2 為「掃描完成, 0 dead code」
- **現有 refs (per m00112 verification)**:
  - `circuit-breaker`: `admin_web/static/js/component-init.js:1` static-import + bundled in `admin_web/dist/` (8 dist/meta.json refs at L583/608/610/819/1061/1063/1211/1385)
  - `deployment-dashboard`: `component-init.js:4` static-import
  - `live-progress`: `dist/meta.json` refs (production)
  - `reasoning-trace`: `admin_web/static/js/main.js:9` static-import (production)
  - `tool-events`: `dist/meta.json` refs (production)
- **估時**: 0h
- **action**: 將 D-2 段更新為「✅ 掃描完成, 5 個 components 均有 refs」

### Item-3: B-1 正式關檔
- **動作**: 與 B-2 一起, 新增 "Closed Items" section
- **說明**: recovery.md B-1 段的 3 endpoints 路徑是誤報。實際 frontend 用的是 `/api/dashboard/...` 系列, backend handlers 已在對應位置:
  - `/api/dashboard/agent-observatory` (`internal/monitoring/api/pipeline/handlers.go:31`)
  - `/api/dashboard/metrics/trend` (`internal/monitoring/api/metrics/handlers.go:32`)
  - `/api/prism/training-results` (`internal/monitoring/api/prism/handlers.go:23`)
- **action**: 將 B-1 段更新為「✅ 用戶報案基於錯誤假設, frontend 從未呼叫所述路徑」

## 3. 排除項目(verified done)

| Item | 當前狀態 (2026-07-04 驗證後) |
|---|---|
| **B-3** (client_web 投資風險總覽轉圈圈) | ✅ 2026-06-27 第二輪已修 |
| **C-1** (shared_web drift backport) | ✅ 2026-06-28 backport 完成 |
| **C-2** (inline style 寫死色碼遷移) | ✅ 部分完成 (variables.css + 6 處 inline rgba) |
| **C-3** (ESM 動態 import CI script) | ✅ 2026-06-28 已建 `scripts/ci/check_frontend_imports.sh` |
| **C-4** (AGENTS.md 路徑修正) | ✅ 2026-06-28 三個 AGENTS.md 統一改為 shared_web/ |
| **D-1** (README/CLAUDE.md 補充) | ✅ 部分完成 (CLAUDE.md L23-67 完整) |
| **H-1** (task-executor.js 平行重複) | ✅ 2026-06-28 刪除 + commits 22949fa0 + 454d9721 |
| **H-2** (CLAUDE.md 補 atlas-pre-change-protocol 入口) | ✅ 2026-06-28 完成 |

## 4. 最終工作量估算

| | 第 1 輪 (m00102) **錯誤** | 第 2 輪 (m00112) **最終** |
|---|---|---|
| 實作 PR | 2 (D-2 刪 5 個 components + B-1 spot-check) | **0** |
| 關檔 action | 1 (B-2 closure) | 3 (B-1 + B-2 + D-2 closure) |
| 總時間 | 3-6h | <1h (只是更新 recovery.md 文件) |

## 5. 設計審計教訓(從兩輪驗證學到)

1. **不要只看單一目錄搜 refs**: `shared_web/` 的 grep 結果不能代表整個 repo; admin_web/ + client_web/ 也有 refs (esbuild plugin fallback targets)
2. **grep filter 要謹慎**: 排除 `internal/eventbus/` 雖然避免噪音, 但也遮蔽了 EventExperiment 定義的真實位置
3. **recovery.md 報案可能是誤報**: 用戶報案時, 實際前端可能根本沒呼叫所述路徑; 需要 grep frontend code 驗證
4. **user m00091 + m00110 修正關鍵**: 「若要實作那些過去未實作的工作, 一定要檢查是否真的沒有做過」

## 6. 參考資料

- **recovery.md source**: `.omo/notepads/frontend-refactor-recovery.md` (325L, gitignored)
- **驗證日期**: 2026-07-04 (兩輪 grep)
- **設計審計方法論**: user m00091 修正 + m00110 二次修正 — 必須逐個 verify 才能假定 work 是需要的
- **注意**: 若日後需執行 git recovery 操作（e.g. reflog 已過期被 gc），請先嘗試 `git fsck --lost-found` 搜尋 dangling commits，或從 `.planning/` 備份還原；90 天預設 gc 可能導致 reflog 無法復原