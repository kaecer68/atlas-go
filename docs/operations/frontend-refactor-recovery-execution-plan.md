# Frontend-Refactor Recovery Items 執行計畫 (post-verification 2026-07-04)

> 來源: `.omo/notepads/frontend-refactor-recovery.md` (最後更新 2026-06-28,共 325 行)
> 本檔基於 2026-07-04 真實驗證,提出剩餘需要實際工作的 items + 已完成的確認。

## 1. 驗證結果總表

| Item | 等級 | recovery.md 聲稱 | 2026-07-04 實際狀態 | 結論 |
|---|---|---|---|---|
| **B-1 e2e 驗證** | P0 | 缺 backend e2e 驗證 3 個 endpoints | e2e infra 存在 (`admin_web/playwright.config.ts`, `client_web/playwright.config.ts`, `cmd/atlas-mcp/e2e_test.go`, `internal/fubonproxy/e2e_test.go`), 但**未確認 3 個特定 endpoint 的 e2e 是否跑過** | **需快速 spot-check** |
| **B-2 用戶誤報確認** | P0 | 用戶報 macroRadar/strategiesContent 為誤報,需與用戶確認 | 無 git commits 確認;屬於用戶互動任務,**無法用程式碼解決** | **建議正式關閉**(無新報案就當 resolved) |
| **D-2 shared_web components/ dead code scan** | P2 | 缺 dead code scan | **REAL GAP** — 掃描發現 5 個 component 0 refs: `circuit-breaker`, `deployment-dashboard`, `live-progress`, `reasoning-trace`, `tool-events` | **需實作** |
| B-3 client_web 風險總覽轉圈圈 | P0 | 待修 | ✅ 已於 2026-06-27 第二輪完成(`client_web/static/js/main.js` L92-107 補 import + L275-298 補 render) | DONE |
| C-1 shared_web drift backport | P1 | 待 backport | ✅ 已於 2026-06-28 完成 | DONE |
| C-2 inline style 色碼遷移 | P1 | 待遷移 | ✅ 部分完成 (variables.css L24/L68/L73 + 6 處 inline rgba) | DONE |
| C-3 ESM 動態 import path 檢查 | P1 | 缺 CI script | ✅ 已建 `scripts/ci/check_frontend_imports.sh` (74 行) | DONE |
| C-4 AGENTS.md 路徑不一致 | P1 | 路徑不一致 | ✅ 已修 (admin_web/AGENTS.md L72 + client_web/AGENTS.md L72) | DONE |
| D-1 README/CLAUDE.md 補充 | P2 | 待補 | ✅ 部分完成 (CLAUDE.md L23-67 完整段已寫;L165 殘留瑕疵待修) | DONE (partial) |
| H-1 task-executor 平行重複 | P2 | 待刪 | ✅ 已刪 (commits `22949fa0` shared_web + `454d9721` web) | DONE |
| H-2 CLAUDE.md 補 atlas-pre-change-protocol 入口 | P2 | 待補 | ✅ 已修 (CLAUDE.md 加 1 行指引 + L165 stale path) | DONE |

## 2. 建議 PR 清單

### PR-1: D-2 Dead Components Cleanup (P2)
- **目標**: 移除 5 個 0-ref components,遵循 H-1 task-executor 模式
- **對象**:
  - `shared_web/static/js/components/circuit-breaker.js`
  - `shared_web/static/js/components/deployment-dashboard.js`
  - `shared_web/static/js/components/live-progress.js`
  - `shared_web/static/js/components/reasoning-trace.js`
  - `shared_web/static/js/components/tool-events.js`
- **驗證步驟**(每個 component):
  1. `git log --all --oneline -- "<file>"` — 確認引入 commit + 無外部引用
  2. grep 確認非 plugin registry / config-driven / reflect dynamic load
  3. grep 確認無 `pages/*.js` / `main.js` / 其他 component 引用
- **刪除步驟**(每個 component):
  1. `git rm <file>`
  2. `git rm legacy/web/static/js/components/<name>.js` (如果存在)
  3. 三邊 `npm run build` 驗證 dist 不變
- **估時**: 2-4h (5 components × 30 分鐘)

### PR-2 (optional): B-1 Endpoint Spot-Check (P0 收尾)
- **目標**: 確認 3 個 endpoint 是否已存在 + 是否有 e2e 覆蓋
- **方法**:
  ```bash
  grep -rn "agents/observatory\|metrics/trend\|prism/results" cmd/atlas/api*.go internal/api/ 2>/dev/null
  grep -rln "agents/observatory\|metrics/trend\|prism/results" --include="*_test.go" . 2>/dev/null | grep -v ".git/"
  ```
- **可能結果**:
  - handler 存在 + test 覆蓋 → 0 PR (僅記錄,正式關閉 B-1)
  - handler 不存在 → 需新增 (1 PR)
  - handler 存在但無 test → 需補 e2e (1 PR)
- **估時**: 1-2h (僅調查)

### B-2: 無 PR (用戶互動任務)
- **狀態**: 用戶報案「liveRadar」(實為 macroRadar) +「strategiesContent」(在 page-strategies) 經盤點為誤報
- **處置**: 在本檔正式標記為「不會修」(除非用戶重新回報)
- **未來**: 若用戶重新報相同症狀,需啟新追蹤流程

## 3. 排除項目 (already done per recovery.md + 2026-07-04 驗證)

| Item | 驗證證據 |
|---|---|
| B-3 client_web 風險總覽 | `client_web/static/js/main.js` L92-107 + L275-298 已有 import + render 呼叫 |
| C-1 shared_web drift backport | `diff -q web vs shared_web` 無差異 (`903971e6` 已關檔) |
| C-2 inline style 色碼遷移 | variables.css L24/L68/L73 + 6 處 inline rgba 已改 `var(--xxx-rgb)` |
| C-3 ESM dynamic import path | `scripts/ci/check_frontend_imports.sh` 存在 (74 行) |
| C-4 AGENTS.md 路徑 | `admin_web/AGENTS.md` L72 + `client_web/AGENTS.md` L72 都指向 `shared_web/...` |
| D-1 README/CLAUDE.md 補充 | CLAUDE.md L23-67 完整;L165 殘留瑕疵為 separate issue |
| H-1 task-executor | commits `22949fa0` + `454d9721` 已刪除兩個位置 (shared_web + web) |
| H-2 CLAUDE.md 補指引 | 已加 1 行指向 atlas-pre-change-protocol |

## 4. 時間預估

| Item | 樂觀估計 |
|---|---|
| PR-1 D-2 dead components (5 個) | 2-4h |
| PR-2 B-1 endpoint spot-check | 1-2h |
| B-2 closure | 0h |
| **總計** | **3-6h (約半天)** |

## 5. 參考資料

- **來源 recovery.md**: `.omo/notepads/frontend-refactor-recovery.md` (gitignored, 325 行, 最後更新 2026-06-28)
- **驗證日期**: 2026-07-04
- **驗證方法**: `git log --since="2026-06-28"` + grep + 文件 read
- **對照**: PR #933 docs cleanup (2026-07-03) 將 recovery.md 從 `docs/branch-hygiene/` 移到 `.omo/notepads/frontend-refactor-recovery.md`,狀態從「active tracker in docs/」改為「active working file in .omo/」
- **H-1 模式參考**: commits `903971e6` (docs) + `22949fa0` (shared_web) + `454d9721` (web),共 3 個 commit + PR #918 合併
- **設計審計方法論**: user m00091 修正 — 必須逐個 verify 才能假定 work 是需要的