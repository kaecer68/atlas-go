# 前端審計修復 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development or executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 修復前端審計中已重現的測試隔離、頁面路由、PRISM markup、deep-link、overlay 與 smoke 覆蓋問題，讓兩側 E2E 與 smoke 可辨識真實資料錯誤。

**Architecture:** 保留現有 SPA 與 shared renderer。測試 server 使用每個專案的絕對 dist 路徑與獨立 port；前端路由只補齊既有 page shell/loader，不新增平行架構；API 失敗由既有 fetch/error state 顯式呈現。

**Tech Stack:** Vanilla JavaScript、TypeScript、Playwright、Node test、esbuild、Python static server。

---

### Task 1: 修正 Admin Playwright server 隔離

**Files:**
- Modify: `admin_web/playwright.config.ts`
- Test: `admin_web/tests/capital-models.spec.ts`, `admin_web/tests/capital-pages.spec.js`

- [ ] **Step 1: 建立可重現 baseline**

Run:
```bash
cd admin_web && npx playwright test --reporter=line
```
Expected: current wrong-app/client HTML and connection-refused failures are observable.

- [ ] **Step 2: 改用絕對 dist 路徑與 admin 專用 port**

`webServer.command` 必須透過 `process.cwd()`/config file URL 解析到 `admin_web/dist` 的絕對路徑，避免從不同 worker cwd 解讀 `dist`；保持測試 baseURL 與 server port 一致，並避免 reuseExistingServer 共享污染實例。

- [ ] **Step 3: 重跑 Admin E2E**

Run:
```bash
cd admin_web && npx playwright test --reporter=line
```
Expected: 不再載入 client_web branding；資本頁面、risk commentary、PRISM 測試各自失敗原因可獨立呈現。

---

### Task 2: 補齊 Admin PRISM 頁面

**Files:**
- Modify: `admin_web/static/index.html`
- Modify: `admin_web/static/js/main.js` 或現有 PRISM page module
- Test: `admin_web/tests/prism-training-results.spec.ts`

- [ ] **Step 1: 確認測試期待的 DOM 與 loader**

讀取測試中 `#page-prism`、`#prismContent`、`switchPage('prism')` 的完整使用方式，沿用現有頁面 section 與 loader 命名。

- [ ] **Step 2: 先讓 DOM 缺失測試維持 RED**

確認 baseline 在正確 admin server 下以 `#prismContent not found` 失敗。

- [ ] **Step 3: 補上最小既有模式 markup/loader**

加入 `page-prism` 與 `prismContent`，接到現有 PRISM training endpoint 與 empty state renderer，不新增第二套資料格式。

- [ ] **Step 4: 驗證 PRISM 兩個測試**

Run:
```bash
cd admin_web && npx playwright test tests/prism-training-results.spec.ts --reporter=line
```
Expected: table 與 empty state 均通過。

---

### Task 3: 接通 Client evolution_panel 與 deep-link activation

**Files:**
- Modify: `client_web/static/js/main.js`
- Modify: `client_web/static/index.html`（僅在既有 markup 缺少必要資料時）
- Test: `client_web/tests` 新增路由回歸測試或既有 route test

- [ ] **Step 1: 寫 route regression test**

測試 `switchPage('evolution_panel', true)` 後 active page 必須是 `page-evolution_panel`，不是 `page-errors/404`；測試直接 deep-link `/client/home`、`/client/capital_board`、`/client/stock-quote?symbol=2330` 時 page 必須 active。

- [ ] **Step 2: 執行確認 RED**

Run:
```bash
cd client_web && npx playwright test <new-or-existing-route-test> --reporter=line
```
Expected: evolution_panel 進入 404，deep-link activation 至少有一個失敗。

- [ ] **Step 3: 接上既有 loader**

在 `SHELL_LOADERS` 補上 `evolution_panel` 的既有 page shell/module；修正 initial route 時序，使 page container 建立後才呼叫 `switchPage`，不改變其他頁面 fallback 規則。

- [ ] **Step 4: 驗證 route tests**

同一測試命令，Expected: all route assertions pass。

---

### Task 4: 修正 onboarding overlay 與導覽互動

**Files:**
- Modify: `client_web/static/js` 中現有 onboarding module
- Test: `client_web/tests/narrative-explanation.spec.ts` 或對應 onboarding test

- [ ] **Step 1: 建立 overlay interaction regression test**

首次載入 overlay 時，測試必須能透過既有 dismiss/continue control 關閉 overlay，之後 `a[data-page="narrative"]` 可點擊並切換頁面。

- [ ] **Step 2: 執行確認 RED**

Expected: current overlay intercepts pointer events and click times out.

- [ ] **Step 3: 修正既有 onboarding flow**

沿用既有 localStorage key 與 overlay control；確保關閉後移除 pointer interception、更新 focus，且不讓 overlay 永久擋住主導覽。

- [ ] **Step 4: 驗證 narrative E2E 與 onboarding tests**

Run:
```bash
cd client_web && npx playwright test tests/narrative-explanation.spec.ts <onboarding-test> --reporter=line
```
Expected: both pass。

---

### Task 5: 修正 stock quote API 測試/服務邊界

**Files:**
- Modify: `client_web/playwright.backend.config.ts` 或測試 server routing config
- Modify: `client_web/tests/client-web-trust.spec.ts` only if baseURL contract is wrong
- Test: `client_web/tests/client-web-trust.spec.ts`

- [ ] **Step 1: 用 curl/Playwright request 重現 HTML response**

確認 `/api/stock/quote?symbol=2330` 的實際 status、content-type、前 80 bytes，定位是 static fallback 還是 backend route 未註冊。

- [ ] **Step 2: 維持 RED**

測試必須繼續以非 JSON response 失敗，避免改測試掩蓋 route bug。

- [ ] **Step 3: 修正測試 backend/server routing**

讓 `/api/*` 不被 static SPA fallback 吞掉；若專案既有 backend test server，讓 `client_web` trust suite 使用該 server 的 baseURL。不得新增裸 HTTP client 或修改 backend 商業邏輯以迎合測試。

- [ ] **Step 4: 驗證 quote penetration**

Run:
```bash
cd client_web && npx playwright test tests/client-web-trust.spec.ts --grep "stock quote|stock/quote" --reporter=line
```
Expected: API JSON 與頁面 stock quote header 均通過。

---

### Task 6: 擴充 smoke 覆蓋與 API failure assertions

**Files:**
- Modify: `admin_web/smoke/run.mjs`
- Modify: `client_web/smoke/run.mjs`
- Modify: `admin_web/smoke/known-issues.json`, `client_web/smoke/known-issues.json` only for verified intentional warnings
- Test: both smoke runners

- [ ] **Step 1: 先將缺漏頁面加入 smoke**

Client default list 至少加入：
```text
capital_predictions,capital_board,evolution_panel,decision-chain
```
Admin default list 至少包含 PRISM page if page is restored。

- [ ] **Step 2: 增加 API failure classification**

Smoke 對 page 觸發的 HTTP 4xx/5xx 記錄為 failure，只有明確列在既有 known-issues 且有原因的項目才 warn；empty state 必須與 API error state 分開。

- [ ] **Step 3: 執行兩側 smoke**

Run:
```bash
cd admin_web && node smoke/run.mjs
cd ../client_web && node smoke/run.mjs
```
Expected: all listed pages pass without hidden API failures。

---

### Task 7: 完整驗證、重複檢查與交付

**Files:**
- No production file changes unless verification exposes a scoped regression.

- [ ] **Step 1: 執行前端完整測試**

```bash
npm --prefix admin_web test
npm --prefix client_web test
npm --prefix admin_web run build
npm --prefix client_web run build
npm --prefix admin_web run test:e2e -- --reporter=line
npm --prefix client_web run test:e2e -- --reporter=line
```

- [ ] **Step 2: 執行瀏覽器重複檢查**

驗證首頁、所有導覽、deep-link、主要按鈕、console、network、API data-to-DOM、空資料/error state、NaN/undefined/null。

- [ ] **Step 3: 執行系統收尾檢查**

```bash
make check-binaries
git cleanup-tools
git status --short
```

- [ ] **Step 4: Commit、push、建立 PR**

依功能拆分 commit，建立 PR 到 `main`，PR body 附完整測試結果與已知風險。

- [ ] **Step 5: 等待 CI 並合併**

確認 PR review/CI 通過後合併到 `main`。

- [ ] **Step 6: 合併後重複驗證**

在 merge result 上重跑 build、unit、E2E、smoke、binary freshness，確認 HEAD 與部署 binary 對齊。
