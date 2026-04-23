# Atlas-Go UI 修復實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基於 UI-REVIEW.md 的稽核結果，修復 Atlas-Go 前端的所有關鍵與中級問題，提升可及性、一致性與視覺品質。

**Architecture:** 所有變更集中在單一檔案 `web/static/index.html`（2361 行的 SPA）。透過修改 CSS `<style>` 區塊與少量 JavaScript，逐步導入設計系統變數、無障礙支援與互動回饋。

**Tech Stack:** HTML5, CSS3 (CSS Variables, Grid, Flexbox), Vanilla JavaScript

---

## 檔案結構

| 檔案 | 職責 | 變更範圍 |
|------|------|---------|
| `web/static/index.html` | 主 SPA（HTML + CSS + JS） | 修改 `<style>` 區塊 (lines 7-200) 與少量 JS |
| `UI-REVIEW.md` | 稽核報告（已存在） | 參考用，不修改 |

---

## Task 1: 添加全局焦點樣式 (Focus Styles)

**優先級:** 🔴 立即 (Blocking)  
**問題:** 完全無 `:focus` 或 `:focus-visible` 樣式，鍵盤導航完全無法使用  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 在 `*` 選擇器後添加全局焦點樣式**

  在 line 11 `* { box-sizing: border-box; }` 後面添加：

  ```css
  *:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  ```

  **修改位置:** line 11 之後

- [ ] **Step 2: 為通知關閉按鈕添加 hover 狀態**

  在 line 78 `.notification .close` 規則中添加 hover：

  ```css
  .notification .close:hover { color: var(--text); }
  ```

  **修改位置:** line 78 之後

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "a11y: add global focus-visible styles and notification close hover"
  ```

---

## Task 2: 提升 --muted 對比度至 WCAG AA 標準

**優先級:** 🔴 立即 (Blocking)  
**問題:** `--muted: #8e99ab` 在 `--bg: #0b0d11` 上對比度僅 4.2:1，低於 WCAG AA 的 4.5:1  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 修改 CSS 變數中的 --muted 值**

  將 line 10 的：
  ```css
  --muted: #8e99ab;
  ```
  改為：
  ```css
  --muted: #9aa5b8;
  ```

  **修改位置:** line 10

- [ ] **Step 2: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "a11y: increase --muted contrast from 4.2:1 to 4.6:1 for WCAG AA compliance"
  ```

---

## Task 3: 將按鈕硬編碼色彩遷移至 CSS 變數

**優先級:** 🔴 立即 (Blocking)  
**問題:** 主按鈕 `#1f6feb` 與危險按鈕 `#da3633` 硬編碼，破壞設計系統單一真實來源  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 在 :root 添加按鈕色彩變數**

  在 line 10 的 `:root` 中添加：
  ```css
  --primary: #1f6feb; --danger: #da3633;
  ```

  **修改位置:** line 10，在 `--warn: #f5a623;` 之後

- [ ] **Step 2: 修改 .control-group button.primary 使用變數**

  將 line 47：
  ```css
  .control-group button.primary { background: #1f6feb; border-color: #1f6feb; }
  ```
  改為：
  ```css
  .control-group button.primary { background: var(--primary); border-color: var(--primary); }
  ```

  **修改位置:** line 47

- [ ] **Step 3: 修改 .control-group button.danger 使用變數**

  將 line 48：
  ```css
  .control-group button.danger { background: #da3633; border-color: #da3633; }
  ```
  改為：
  ```css
  .control-group button.danger { background: var(--danger); border-color: var(--danger); }
  ```

  **修改位置:** line 48

- [ ] **Step 4: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "design-system: migrate button colors to CSS variables (--primary, --danger)"
  ```

---

## Task 4: 為寬表格添加水平捲動

**優先級:** 🟡 短期 (High Impact)  
**問題:** 決策鏈 13 欄表格、投資管線 12 欄表格在窄螢幕下會撐破容器  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 添加 .table-wrapper CSS class**

  在 line 92（`#pipelineTable th, #pipelineTable td` 規則）之後添加：

  ```css
  .table-wrapper { overflow-x: auto; max-width: 100%; }
  .table-wrapper table { min-width: 640px; }
  ```

  **修改位置:** line 92 之後

- [ ] **Step 2: 為決策鏈表格添加 wrapper**

  搜尋決策鏈中的表格（約 line 1105, 1151-1156, 1230, 1239 附近），將每個 `<table>` 包裝為：
  ```html
  <div class="table-wrapper">
    <table>...</table>
  </div>
  ```

  **修改位置:** 決策鏈頁面中的表格（約 lines 1100-1250）

- [ ] **Step 3: 為投資管線表格添加 wrapper**

  將 line 1429-1458 的 `<table id="pipelineTable">` 包裝為：
  ```html
  <div class="table-wrapper">
    <table id="pipelineTable">...</table>
  </div>
  ```

  **修改位置:** 投資管線頁面表格（約 line 1429）

- [ ] **Step 4: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "responsive: add horizontal scroll wrappers for wide tables"
  ```

---

## Task 5: 添加按鈕 :active 狀態

**優先級:** 🟡 短期 (High Impact)  
**問題:** 按鈕無 active/pressed 狀態，使用者無法感知已點擊  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 為 .control-group button 添加 :active**

  在 line 49 `.control-group button:hover` 之後添加：

  ```css
  .control-group button:active { transform: translateY(1px); }
  ```

  **修改位置:** line 49 之後

- [ ] **Step 2: 為 .pipeline-action 添加 :active**

  在 line 89 `.pipeline-action:hover` 之後添加：

  ```css
  .pipeline-action:active { transform: translateY(1px); opacity: .7; }
  ```

  **修改位置:** line 89 之後

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "ux: add active/pressed states to buttons"
  ```

---

## Task 6: 為 Modal 添加無障礙屬性

**優先級:** 🟡 短期 (High Impact)  
**問題:** Modal 缺少 `aria-modal`、`role="dialog"`、焦點陷阱、Escape 鍵關閉  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 為三個 modal 添加 ARIA 屬性**

  搜尋三個 modal 的 HTML（約 lines 435-471）：
  - `diffModal`
  - `promoteModal`
  - `infoModal`

  為每個 modal 的 `<div class="modal-overlay" ...>` 添加：
  ```html
  role="dialog" aria-modal="true"
  ```

  **修改位置:** lines 435, 449, 462

- [ ] **Step 2: 添加 Escape 鍵關閉 modal 的 JavaScript**

  在 `<script>` 區塊中（約 line 473 之後），添加：

  ```javascript
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeDiff(); closePromote(); closeInfo();
    }
  });
  ```

  **修改位置:** script 區塊開頭（約 line 473 之後）

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "a11y: add modal aria attributes and Escape key handler"
  ```

---

## Task 7: 定義間距尺度變數

**優先級:** 🟡 短期 (High Impact)  
**問題:** 無間距尺度，padding/margin/gap 值散落各處（8px, 10px, 12px, 14px, 16px, 18px）  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 在 :root 添加間距變數**

  在 line 10 的 `:root` 中添加：
  ```css
  --space-xs: 4px; --space-sm: 8px; --space-md: 14px; --space-lg: 20px; --space-xl: 32px;
  ```

  **修改位置:** line 10

- [ ] **Step 2: 將主要間距值遷移至變數（第一輪）**

  修改以下規則使用變數：
  - line 22 `.container { ... gap: 14px; padding: 14px; }` → `gap: var(--space-md); padding: var(--space-md);`
  - line 24 `.panel { ... padding: 14px; }` → `padding: var(--space-md);`
  - line 58 `.inbox-grid { ... gap: 12px; }` → `gap: var(--space-sm);` (或保留 12px，因為無對應變數)
  - line 167 `.kpi-grid { ... gap: 14px; }` → `gap: var(--space-md);`
  - line 175 `.kpi-card { ... padding: 14px; }` → `padding: var(--space-md);`

  **修改位置:** lines 22, 24, 58, 167, 175

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "design-system: add spacing scale variables (--space-xs to --space-xl)"
  ```

---

## Task 8: 將內聯 grid 樣式改為 CSS class

**優先級:** 🟢 中期 (Medium Impact)  
**問題:** 敘事頁、即時趨勢頁、控制頁使用 `style="display:grid;grid-template-columns:..."`  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 添加 .two-col-grid 與 .three-col-grid CSS class**

  在 line 198（`</style>` 之前）添加：

  ```css
  .two-col-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-md); }
  @media (max-width: 1100px) { .two-col-grid { grid-template-columns: 1fr; } }
  .three-col-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--space-md); }
  @media (max-width: 900px) { .three-col-grid { grid-template-columns: 1fr; } }
  ```

  **修改位置:** line 198 之前

- [ ] **Step 2: 將敘事頁內聯 grid 改為 class**

  搜尋 line 257 附近的：
  ```html
  <div id="narrativePage" ... style="display:grid;grid-template-columns:1fr 1fr;gap:14px">
  ```
  改為：
  ```html
  <div id="narrativePage" ... class="two-col-grid">
  ```

  **修改位置:** line 257

- [ ] **Step 3: 將其他內聯 grid 改為 class**

  搜尋並替換其他內聯 grid：
  - line 277: `style="display:grid;grid-template-columns:minmax(0,1fr) 170px;gap:14px"` → 保留（特殊佈局，不適用通用 class）
  - line 354: `style="display:grid;grid-template-columns:1fr 1fr;gap:14px"` → `class="two-col-grid"`
  - line 985: `style="display:grid;grid-template-columns:1fr 1fr;gap:14px"` → `class="two-col-grid"`

  **修改位置:** lines 354, 985

- [ ] **Step 4: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "refactor: extract inline grid styles to reusable CSS classes"
  ```

---

## Task 9: 為載入狀態添加動畫

**優先級:** 🟢 中期 (Medium Impact)  
**問題:** 所有區塊使用靜態「載入中…」文字，無旋轉動畫或骨架屏  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 添加 CSS 載入動畫**

  在 line 198（`</style>` 之前）添加：

  ```css
  @keyframes spin { to { transform: rotate(360deg); } }
  .loading::before {
    content: "";
    display: inline-block;
    width: 14px; height: 14px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-right: 8px;
    vertical-align: middle;
  }
  ```

  **修改位置:** line 198 之前

- [ ] **Step 2: 將「載入中…」文字改為使用 .loading class**

  搜尋所有「載入中…」文字（約 lines 258-265, 276-279, 297, 307-308, 313-314, 328-330, 347, 389-390, 406, 425），將：
  ```html
  <div class="empty">載入中…</div>
  ```
  改為：
  ```html
  <div class="empty loading">載入中…</div>
  ```

  **修改位置:** 所有「載入中…」出現的位置

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "ux: add loading spinner animation to placeholder states"
  ```

---

## Task 10: 建立字階與字重變數

**優先級:** 🟢 中期 (Medium Impact)  
**問題:** 字體大小從 10px 到 22px 散落，無模組化比例；字重無系統化管理  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 在 :root 添加字階與字重變數**

  在 line 10 的 `:root` 中添加：
  ```css
  --text-xs: 10px; --text-sm: 12px; --text-base: 13px; --text-lg: 15px; --text-xl: 18px; --text-2xl: 22px;
  --font-normal: 400; --font-medium: 500; --font-semibold: 600; --font-bold: 700;
  ```

  **修改位置:** line 10

- [ ] **Step 2: 將主要字體大小遷移至變數（第一輪）**

  修改以下規則：
  - line 28 `.metric .label { font-size: 11px; ... }` → 保留 11px（無對應變數）
  - line 33 `table { ... font-size: 13px; }` → `font-size: var(--text-base);`
  - line 34 `th, td { ... }` → 繼承 table
  - line 37 `.badge { font-size: 11px; ... }` → 保留 11px
  - line 45 `.control-group input, .control-group select { ... font-size: 12px; }` → `font-size: var(--text-sm);`
  - line 46 `.control-group button { ... font-size: 12px; }` → `font-size: var(--text-sm);`
  - line 52 `.workflow .step { ... font-size: 12px; }` → `font-size: var(--text-sm);`

  **修改位置:** lines 33, 45, 46, 52

- [ ] **Step 3: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "design-system: add typography scale and font-weight variables"
  ```

---

## Task 11: 統一 factorBar/layer cards/status lights 色彩為 CSS 變數

**優先級:** 🟢 中期 (Medium Impact)  
**問題:** factorBar、layer cards、status lights 使用硬編碼色彩，未對應設計系統  
**檔案:** `web/static/index.html`

- [ ] **Step 1: 在 :root 添加層級與狀態色彩變數**

  在 line 10 的 `:root` 中添加：
  ```css
  --layer-1: #3b82f6; --layer-2: #8b5cf6; --layer-3: #10b981; --layer-4: #f59e0b; --layer-5: #ef4444; --layer-6: #6366f1;
  --status-ok: #10b981; --status-warn: #f59e0b; --status-err: #ef4444; --status-unknown: #9ca3af;
  ```

  **修改位置:** line 10

- [ ] **Step 2: 修改 factorBar 函數使用變數**

  搜尋約 line 1007-1014 的 `factorBar()` 函數，將：
  ```javascript
  const color = pct > 70 ? '#22c55e' : pct > 40 ? '#f59e0b' : '#ef4444';
  ```
  改為：
  ```javascript
  const color = pct > 70 ? 'var(--up)' : pct > 40 ? 'var(--warn)' : 'var(--down)';
  ```

  **修改位置:** line 1011

- [ ] **Step 3: 修改 layerCard 函數使用變數**

  搜尋約 line 1314-1319 的 `layerCard()` 函數中的硬編碼色彩，改為使用 `--layer-1` 到 `--layer-6`。

  **修改位置:** lines 1314-1319

- [ ] **Step 4: 修改 statusLight 函數使用變數**

  搜尋約 line 1563 的 `statusLight()` 函數，將硬編碼色彩改為 `var(--status-ok)` 等。

  **修改位置:** line 1563

- [ ] **Step 5: Commit**

  ```bash
  git add web/static/index.html
  git commit -m "design-system: unify factorBar, layer cards, and status lights to CSS variables"
  ```

---

## 驗證清單

在所有 Task 完成後執行：

- [ ] **視覺驗證:** 在瀏覽器中開啟 `web/static/index.html`，確認：
  - Tab 鍵導航可見藍色焦點框
  - 按鈕點擊有下移效果
  - Modal 可按 Escape 關閉
  - 載入狀態顯示旋轉動畫
  - 表格在窄螢幕下可水平捲動

- [ ] **對比度驗證:** 使用瀏覽器 DevTools 或 axe DevTools 確認：
  - `--muted` 文字對比度 ≥ 4.5:1
  - 所有按鈕文字對比度 ≥ 4.5:1

- [ ] **程式碼驗證:**
  ```bash
  go build ./...
  go test ./...
  ```

---

## 附錄：變更摘要

| Task | 類型 | 影響範圍 | 無障礙 | 設計系統 |
|------|------|---------|--------|---------|
| 1. 焦點樣式 | CSS | 全局 | ✅ | ✅ |
| 2. Muted 對比度 | CSS | 全局 | ✅ | — |
| 3. 按鈕色彩變數 | CSS | 按鈕 | — | ✅ |
| 4. 表格水平捲動 | CSS + HTML | 表格 | ✅ | — |
| 5. 按鈕 active | CSS | 按鈕 | ✅ | — |
| 6. Modal 無障礙 | HTML + JS | Modal | ✅ | — |
| 7. 間距變數 | CSS | 全局 | — | ✅ |
| 8. Grid class | CSS + HTML | 版面 | — | ✅ |
| 9. 載入動畫 | CSS + HTML | 載入狀態 | ✅ | — |
| 10. 字階變數 | CSS | 全局 | — | ✅ |
| 11. 色彩統一 | CSS + JS | 元件 | — | ✅ |

---

*計畫建立日期: 2026-04-21*  
*基於: UI-REVIEW.md 稽核結果*
