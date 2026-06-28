# CSS Extraction: index.html Inline → External Files

## TL;DR

> **Quick Summary**: 將 `web/static/index.html` 中 1,511 行的內嵌 CSS 抽出至外部檔案，消除與現有外部 CSS 的重複，使 index.html 從 2,246 行降至 ~735 行。
>
> **Deliverables**:
> - 乾淨的 `web/static/css/` 目錄，包含所有樣式（無重複）
> - `index.html` 僅剩 HTML 結構 + `<link>` 引用
> - Dashboard 視覺外觀完全不改
>
> **Estimated Effort**: Quick
> **Parallel Execution**: NO - sequential（單純搬移，不需要平行）
> **Critical Path**: Task 1 → Task 2 → Task 3 → Task 4 → Task 5

---

## Context

### Original Request
使用者發現 `web/static/index.html` 是專案中最大的原始碼檔案（99KB, 2,246 行），其中 1,511 行（67%）為單一 `<style>` 區塊。

### Interview Summary
**Key Discussions**:
- JS 已完全模組化（`js/pages/`, `js/components/`, `js/shared/`, `js/services/`），inline JS 僅剩 import 聲明
- CSS 是唯一尚未拆分的部分
- HTML body 710 行為 SPA DOM 結構，合理不需拆分

**Research Findings**:
- **現有外部 CSS 已存在**：`css/variables.css`(31行)、`css/layout.css`(67行)、`css/skeleton.css`(48行)、`css/components.css`(93行) + 3 個組件 CSS
- 外部 CSS 共 821 行、138 個唯一選擇器
- Inline CSS 有 320 個唯一選擇器 → 約 182 個選擇器僅存在於 inline 中（未被抽出）
- 外部檔案與 inline 有重複內容（variables, layout 等同時存在於兩處）
- Go server 以 `http.FileServer(http.Dir("web/static"))` 服務靜態檔案，新增 CSS 檔案不需改 Go 代碼
- 已有 3 個組件 CSS 被 `<link>` 引用：`order-manager.css`、`performance-report.css`、`circuit-breaker.css`

### Metis Review
**Identified Gaps** (addressed):
- 風險向量 url() 路徑、JS `<style>` 操作、Go server/CSP — 全部確認無風險
- Guardrail: 不得新增 CSS 框架，不得改 JS，不得改 Go server

---

## Work Objectives

### Core Objective
消除 index.html 的 1,511 行 inline CSS，整合至現有 `web/static/css/` 目錄，去除重複。

### Concrete Deliverables
- `web/static/css/main.css` — 包含所有頁面樣式的單一入口檔案
- `index.html` — 僅剩 HTML 結構 + `<link>` + `<script>` import，~735 行

### Definition of Done
- [ ] `index.html` 不含任何 `<style>` 區塊
- [ ] 所有頁面視覺外觀與改動前完全一致（dark mode + light mode）
- [ ] 無 CSS 重複（同一選擇器不會同時出現在兩個檔案中）

### Must Have
- Dashboard、Portfolio、Experiments、Backtest、Evolution、Data Channels、Industry、Risk、Narrative 等所有頁面樣式正確
- Dark mode / Light mode 主題切換正常
- 響應式佈局（桌面 + 行動裝置）正常
- Modal、Tab、Badge、Table 等通用元件樣式正確

### Must NOT Have (Guardrails)
- **禁止**修改任何 Go 原始碼
- **禁止**修改任何 JavaScript 檔案
- **禁止**改變 CSS 的實際效果（只搬移，不改值）
- **禁止**引入 CSS 框架（Tailwind, Bootstrap 等）
- **禁止**過度拆分成幾十個小檔案（保持簡單）

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: NO（前端無測試框架）
- **Automated tests**: None
- **Framework**: none

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Frontend/UI**: Use Playwright（如有）或 curl + Go server 啟動驗證

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Sequential — 單純搬移任務，不適合平行):
├── Task 1: 分析 inline CSS 結構，建立 main.css [quick]
├── Task 2: 刪除 inline CSS，加入 <link> 引用 [quick]
├── Task 3: 去除外部檔案中的重複樣式 [quick]
├── Task 4: 啟動 server 視覺驗證 [quick]
└── Task 5: 清理 + 最終驗證 [quick]
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|-----------|--------|
| 1 | - | 2, 3 |
| 2 | 1 | 4 |
| 3 | 1 | 4 |
| 4 | 2, 3 | 5 |
| 5 | 4 | - |

### Agent Dispatch Summary

- **Wave 1**: 5 tasks → all `quick`

---

## TODOs

- [ ] 1. 建立 main.css：整合 inline CSS 與現有外部檔案

  **What to do**:
  - 讀取 `index.html` 第 7–1517 行的完整 inline CSS
  - 讀取現有外部 CSS：`css/variables.css`, `css/layout.css`, `css/skeleton.css`, `css/components.css`
  - 以 inline CSS 為基準（它是最完整的，含 320 個選擇器），將其內容寫入 `web/static/css/main.css`
  - 確保 `@import` 或內容順序正確：variables → base → layout → components → pages
  - 注意：`css/components/` 下的 3 個組件 CSS（`circuit-breaker.css`, `order-manager.css`, `performance-report.css`）已透過 `<link>` 獨立引用，**不要合併進 main.css**

  **Must NOT do**:
  - 不得修改 index.html（下一個 task 才改）
  - 不得修改任何現有外部 CSS 檔案
  - 不得改變任何 CSS 屬性值

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Wave 1)
  - **Blocks**: Tasks 2, 3
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `web/static/index.html:7-1517` — 完整 inline CSS，這是主要來源（1,511 行，320 選擇器）
  - `web/static/css/variables.css` — 現有外部 variables（31 行），內容與 inline 重複
  - `web/static/css/layout.css` — 現有外部 layout（67 行），內容與 inline 重複
  - `web/static/css/skeleton.css` — 現有外部 skeleton/loading 樣式（48 行）
  - `web/static/css/components.css` — 現有外部通用元件樣式（93 行）

  **API/Type References**:
  - `cmd/atlas/main.go:345,922` — Go server 使用 `http.FileServer(http.Dir("web/static"))` 服務靜態檔案，確認新 CSS 檔案可直接存取

  **External References**:
  - 無（純 CSS 搬移，不需外部文件）

  **WHY Each Reference Matters**:
  - `index.html:7-1517` 是要搬移的內容來源
  - 現有外部 CSS 檔案可能與 inline 重複，需理解其內容以避免重複
  - Go server 的靜態檔案路由確認不需要改任何 Go 代碼

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: main.css 檔案已建立且包含完整樣式
    Tool: Bash
    Preconditions: main.css 尚不存在
    Steps:
      1. 確認 `web/static/css/main.css` 檔案存在：`test -f web/static/css/main.css && echo "EXISTS"`
      2. 確認行數 >= 1400 行（inline CSS 有 1,511 行，整合後應相近）：`wc -l web/static/css/main.css`
      3. 確認包含 CSS 變數定義：`grep -c ':root' web/static/css/main.css`（應 >= 1）
      4. 確認包含 light theme：`grep -c 'data-theme="light"' web/static/css/main.css`（應 >= 1）
      5. 確認包含 sidebar 樣式：`grep -c '#sidebar' web/static/css/main.css`（應 >= 1）
      6. 確認包含 panel 樣式：`grep -c '\.panel' web/static/css/main.css`（應 >= 1）
      7. 確認包含響應式斷點：`grep -c '@media' web/static/css/main.css`（應 >= 1）
      8. 確認不包含 circuit-breaker/order-manager/performance-report 的專屬樣式（那些已獨立引用）
    Expected Result: main.css 存在、行數合理、包含所有必要的通用樣式
    Failure Indicators: 檔案不存在、行數異常少、缺少 key selectors
    Evidence: .sisyphus/evidence/task-1-main-css-created.txt
  ```

  **Commit**: NO（與 Task 2, 3 一起提交）

---

- [ ] 2. 替換 index.html 的 inline CSS 為 `<link>` 引用

  **What to do**:
  - 在 `index.html` 的 `<head>` 區段，刪除整個 `<style>...</style>` 區塊（第 7–1517 行）
  - 在 `<head>` 區段加入 `<link rel="stylesheet" href="/static/css/main.css">`
  - 此 `<link>` 應放在現有的組件 CSS `<link>` 之前（main.css 是基礎樣式，組件 CSS 覆蓋其上）
  - 現有的 3 個組件 `<link>` 保持不動：
    - `/static/css/components/order-manager.css`
    - `/static/css/components/performance-report.css`
    - `/static/css/components/circuit-breaker.css`

  **Must NOT do**:
  - 不得修改 HTML body 結構
  - 不得修改任何 `<script>` 標籤
  - 不得改變組件 CSS 的 `<link>` 引用

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Wave 1)
  - **Blocks**: Task 4
  - **Blocked By**: Task 1

  **References**:

  **Pattern References**:
  - `web/static/index.html:7-1517` — 這是 `<style>` 區塊的精確行號範圍，需要完整刪除
  - `web/static/index.html:1518-1521` — 現有的 3 個 `<link>` 標籤位置，新 `<link>` 應插入在這些之前

  **WHY Each Reference Matters**:
  - 需要精確知道刪除範圍（第 7–1517 行）
  - 需要知道插入位置（在現有 `<link>` 之前）

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: index.html 不再包含 inline style
    Tool: Bash
    Preconditions: Task 1 已完成，main.css 已建立
    Steps:
      1. 確認 index.html 不含 <style> 標籤：`grep -c '<style>' web/static/index.html`（應為 0）
      2. 確認 index.html 包含 main.css 引用：`grep -c 'css/main.css' web/static/index.html`（應為 1）
      3. 確認 index.html 行數大幅減少：`wc -l web/static/index.html`（應 < 800 行）
      4. 確認組件 CSS link 仍存在：`grep -c 'order-manager.css' web/static/index.html`（應為 1）
      5. 確認 HTML body 結構完整：`grep -c '<div id=' web/static/index.html`（應與改動前相同，約 50+ 個）
    Expected Result: index.html < 800 行，無 <style>，有正確的 <link> 引用
    Failure Indicators: 仍有 <style> 標籤、缺少 <link>、行數未減少
    Evidence: .sisyphus/evidence/task-2-html-cleaned.txt
  ```

  **Commit**: NO（與 Task 1, 3 一起提交）

---

- [ ] 3. 去除現有外部 CSS 檔案中的重複樣式

  **What to do**:
  - 比對 `css/variables.css`、`css/layout.css`、`css/skeleton.css`、`css/components.css` 與新的 `main.css`
  - 如果這 4 個檔案的內容完全被 `main.css` 包含（高機率是），則刪除這 4 個檔案
  - 如果有任何內容在 `main.css` 中缺失，則補充進 `main.css` 後再刪除
  - **絕對不要動** `css/components/` 下的 3 個組件 CSS 檔案（它們透過獨立 `<link>` 引用）
  - 更新 `index.html`：移除任何對已刪除檔案的引用（如果有的話）

  **Must NOT do**:
  - 不得修改 `css/components/circuit-breaker.css`
  - 不得修改 `css/components/order-manager.css`
  - 不得修改 `css/components/performance-report.css`

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO（與 Task 2 可同時，但為安全起見 sequential）
  - **Parallel Group**: Sequential (Wave 1)
  - **Blocks**: Task 4
  - **Blocked By**: Task 1

  **References**:

  **Pattern References**:
  - `web/static/css/variables.css` (31行) — `:root` 變數定義，已在 main.css 中
  - `web/static/css/layout.css` (67行) — 佈局樣式，已在 main.css 中
  - `web/static/css/skeleton.css` (48行) — loading/skeleton 樣式
  - `web/static/css/components.css` (93行) — 通用元件樣式

  **WHY Each Reference Matters**:
  - 需要逐一比對這 4 個檔案是否已被 main.css 完全覆蓋
  - 如果有遺漏，需要先補充再刪除

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 重複的外部 CSS 檔案已清除
    Tool: Bash
    Preconditions: Task 1 已完成，main.css 已包含完整樣式
    Steps:
      1. 確認 variables.css 等檔案已刪除或內容為空：
         `for f in variables layout skeleton components; do test -f web/static/css/$f.css && echo "EXISTS: $f.css" || echo "REMOVED: $f.css"; done`
      2. 確認 components 子目錄的 3 個檔案仍在：
         `for f in circuit-breaker order-manager performance-report; do test -f "web/static/css/components/$f.css" && echo "KEPT: $f.css"; done`
      3. 確認 main.css 仍包含所有必要的選擇器（無遺漏）：
         `grep -c ':root' web/static/css/main.css && grep -c '#sidebar' web/static/css/main.css && grep -c '\.panel' web/static/css/main.css && grep -c '@keyframes' web/static/css/main.css`
    Expected Result: 舊的 4 個檔案已刪除，組件檔案仍存在，main.css 內容完整
    Failure Indicators: 組件檔案被誤刪、main.css 遺失選擇器
    Evidence: .sisyphus/evidence/task-3-duplicates-removed.txt
  ```

  **Commit**: YES（與 Task 1, 2 合併提交）
  - Message: `refactor(web): extract inline CSS to external main.css`
  - Files: `web/static/css/main.css`, `web/static/index.html`, `web/static/css/variables.css`, `web/static/css/layout.css`, `web/static/css/skeleton.css`, `web/static/css/components.css`
  - Pre-commit: `go build ./...` (確認 Go 代碼未受影響)

---

- [ ] 4. 啟動 Go Server 進行視覺驗證

  **What to do**:
  - 啟動 `go run ./cmd/atlas`（預設 port 8080）
  - 使用 curl 或瀏覽器工具驗證以下頁面能正確載入：
    - 根頁面 `/` — 確認 HTML 載入、CSS 連結有效
    - `/static/css/main.css` — 確認 CSS 檔案可被正確服務
    - 檢查 HTTP 狀態碼均為 200
  - 確認 `/health` endpoint 正常
  - 比對頁面 HTML 行數（應 < 800 行）

  **Must NOT do**:
  - 不得修改任何原始碼（純驗證）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Wave 1)
  - **Blocks**: Task 5
  - **Blocked By**: Tasks 2, 3

  **References**:

  **Pattern References**:
  - `cmd/atlas/main.go:345` — `http.FileServer(http.Dir("web/static"))` 確認靜態檔案路由
  - `cmd/atlas/main.go:922` — 根路徑 `/` 也使用同一 FileServer

  **WHY Each Reference Matters**:
  - 確認 server 的靜態檔案服務邏輯，確保新的 CSS 路徑可達

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: CSS 檔案可被正確服務
    Tool: Bash (curl)
    Preconditions: Go server 正在運行 (port 8080)
    Steps:
      1. 啟動 server: `go run ./cmd/atlas &` (背景執行)
      2. 等待 server 啟動: `sleep 3`
      3. 檢查 main.css 可達: `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/static/css/main.css`（應為 200）
      4. 檢查 main.css 內容非空: `curl -s http://localhost:8080/static/css/main.css | wc -l`（應 >= 1400）
      5. 檢查 HTML 可達: `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/`（應為 200）
      6. 檢查 HTML 中包含 main.css 引用: `curl -s http://localhost:8080/ | grep 'css/main.css'`（應有輸出）
      7. 檢查 HTML 中不含 <style>: `curl -s http://localhost:8080/ | grep -c '<style>'`（應為 0）
      8. 檢查組件 CSS 仍可達: `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/static/css/components/circuit-breaker.css`（應為 200）
      9. 停止 server: `kill %1`
    Expected Result: 所有 CSS 和 HTML 都返回 HTTP 200，內容正確
    Failure Indicators: HTTP 404、空內容、仍有 <style>
    Evidence: .sisyphus/evidence/task-4-server-validation.txt

  Scenario: Light/Dark theme 切換功能正常
    Tool: Bash (curl)
    Preconditions: Go server 正在運行
    Steps:
      1. 確認 main.css 包含 dark mode 變數: `curl -s http://localhost:8080/static/css/main.css | grep -c ':root'`（應 >= 1）
      2. 確認 main.css 包含 light mode 變數: `curl -s http://localhost:8080/static/css/main.css | grep -c 'data-theme.*light'`（應 >= 1）
    Expected Result: 兩種主題的 CSS 變數都存在
    Failure Indicators: 缺少任一主題定義
    Evidence: .sisyphus/evidence/task-4-theme-variables.txt
  ```

  **Commit**: NO（純驗證）

---

- [ ] 5. 清理 + 最終驗證

  **What to do**:
  - 確認 `web/static/css/` 目錄結構乾淨：
    - `main.css` 存在
    - `variables.css`, `layout.css`, `skeleton.css`, `components.css` 已刪除
    - `components/` 子目錄下的 3 個檔案仍存在
  - 執行 `go build ./...` 確認 Go 代碼未受影響
  - 執行 `go test ./cmd/atlas/...` 確認測試通過（含靜態檔案 server 測試）
  - 最終行數確認：`wc -l web/static/index.html`（應 < 800 行）

  **Must NOT do**:
  - 不得修改任何原始碼（純驗證 + 清理）

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Wave 1)
  - **Blocks**: Final Verification
  - **Blocked By**: Task 4

  **References**:

  **Pattern References**:
  - `cmd/atlas/main_test.go:289` — `TestStaticFileServerServesIndex` 測試靜態檔案服務，確認測試仍通過

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY):**

  ```
  Scenario: 最終狀態確認
    Tool: Bash
    Preconditions: 所有前置 tasks 已完成
    Steps:
      1. 確認 index.html 行數: `wc -l web/static/index.html`（應 < 800）
      2. 確認 index.html 無 <style>: `grep -c '<style>' web/static/index.html`（應為 0）
      3. 確認 main.css 存在且行數合理: `wc -l web/static/css/main.css`（應 >= 1400）
      4. 確認 Go build 通過: `go build ./...`（exit code 0）
      5. 確認 Go test 通過: `go test ./cmd/atlas/...`（exit code 0）
      6. 確認無殘留重複檔案: `for f in variables layout skeleton components; do test -f web/static/css/$f.css && echo "WARNING: $f.css still exists"; done`（應無輸出）
    Expected Result: 所有檢查通過，index.html < 800 行，Go build/test 綠燈
    Failure Indicators: build 失敗、test 失敗、仍有 <style>、行數未減
    Evidence: .sisyphus/evidence/task-5-final-validation.txt
  ```

  **Commit**: NO（純驗證，若有清理需求則 amend 至 Task 3 的 commit）

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. Verify: index.html has no `<style>` tag, main.css exists with >= 1400 lines, all page selectors present, Go build passes.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go build ./...` + `go test ./cmd/atlas/...`. Check index.html for any remaining inline styles. Verify CSS file structure is clean.
  Output: `Build [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Visual QA** — `unspecified-high`
  Start server, curl all CSS endpoints (main.css + 3 component CSS), verify HTTP 200 and non-empty content. Check HTML references are correct.
  Output: `Scenarios [N/N pass] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  Verify: only CSS files and index.html were modified. No Go files changed. No JS files changed. No functional CSS changes (same values).
  Output: `Tasks [N/N compliant] | VERDICT`

---

## Commit Strategy

- **Task 1+2+3**: `refactor(web): extract inline CSS to external main.css` — `web/static/css/main.css` (new), `web/static/index.html` (modified), `web/static/css/variables.css` (deleted), `web/static/css/layout.css` (deleted), `web/static/css/skeleton.css` (deleted), `web/static/css/components.css` (deleted)
  - Pre-commit: `go build ./...`

---

## Success Criteria

### Verification Commands
```bash
# index.html 不再包含 inline CSS
grep -c '<style>' web/static/index.html  # Expected: 0

# main.css 已建立且完整
wc -l web/static/css/main.css            # Expected: >= 1400

# Go 代碼未受影響
go build ./...                           # Expected: success
go test ./cmd/atlas/...                  # Expected: PASS

# 總行數大幅減少
wc -l web/static/index.html              # Expected: < 800

# 組件 CSS 仍獨立存在
test -f web/static/css/components/circuit-breaker.css && echo "OK"
test -f web/static/css/components/order-manager.css && echo "OK"
test -f web/static/css/components/performance-report.css && echo "OK"
```

### Final Checklist
- [ ] All "Must Have" present — 無 <style>，main.css 存在，所有頁面樣式正確
- [ ] All "Must NOT Have" absent — 無 Go 改動，無 JS 改動，無 CSS 值改動
- [ ] Go build + test pass
- [ ] index.html 從 2,246 行降至 < 800 行
