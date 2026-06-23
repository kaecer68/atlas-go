# REPAIR PLAN: 投資心法頁面數據載入健壯性修復

**日期**: 2026-06-23
**對應**: `INVESTIGATION.md`（同分支）
**目標**: 將 5 種「無數據」失敗模式全部顯式化，並符合 `atlas-data-visibility` L4 規範

---

## 一、修復範圍與非目標

### 範圍（IN）
- `web/static/js/pages/strategies.js`（338 行）—— 主要修改
- `web/static/js/shared/app-utils.js` —— 新增 `classifyFetchError` 工具
- `web/static/js/__tests__/strategies.test.mjs` —— 新增（測試 5 種失敗模式）

### 非目標（OUT）
- 不修改 backend Go 程式碼
- 不修改 `main.js` 集中 health probe（屬於另一個 cross-cutting fix，留給其他工作區）
- 不修改 `parameters.js` / `performance-report.js` 等其他頁面（同樣留給其他工作區）
- 不修改 CSS（skeleton 動畫暫不改樣式，只用既有 `class="loading"` 觸發可見狀態）

---

## 二、修復項目（按優先級）

### Priority 1：fetchJSON 錯誤分類（解模式 A、B）

**現況**：`strategies.js:52-60` 的 `fetchJSON` 直接拋出 `TypeError` 或 `Error(... -> status)`，由 line 80 原樣渲染。

**修改**：
- 新增 `app-utils.js` 工具函式 `classifyFetchError(err, url)`，回傳結構：
  ```js
  { kind: 'network', message: '後端未啟動...', recoverable: true, hint: 'go run ./cmd/atlas --api' }
  { kind: 'http_503', message: '策略心法 registry 未初始化...', recoverable: true, hint: 'data/seeds/strategy_techniques.json' }
  { kind: 'http_4xx', message: '請求被拒', recoverable: false }
  { kind: 'http_5xx', message: '後端錯誤', recoverable: true }
  { kind: 'parse', message: '回應格式錯誤', recoverable: false }
  ```
- `strategies.js` 改用 `classifyFetchError` 後渲染

**測試**：
- mock `fetch` 拋 `TypeError` → 渲染含「後端未啟動」+ 啟動指令
- mock `fetch` 回 503 → 渲染含「registry 未初始化」+ seed 檔案路徑
- mock `fetch` 回 500 → 渲染含「後端錯誤」
- mock `fetch` 回 404 → 渲染含「請求被拒」

### Priority 2：Partial-failure tolerance（解模式 C）

**現況**：`strategies.js:249` `Promise.all` 容忍度不對稱（只有 decision-chain 有 catch）。

**修改**：改為 `Promise.allSettled`，並對每條 endpoint 設定最低容忍度：
- `/api/strategies` → **核心**，失敗 → data_status: 'failed'，但仍嘗試顯示 layers（如果有）
- `/api/strategies/layers` → **次要**，失敗 → 顯示 strategies 但 KPI 顯示「5 層覆蓋：--」
- `/api/dashboard/decision-chain` → **輔助**，失敗 → 指標卡顯示「--」

**測試**：
- mock strategies=ok、layers=fail → 頁面顯示 strategies 內容但 KPI 顯示覆蓋「--」、頂部黃色 banner「layers 載入失敗」
- mock strategies=fail、layers=ok → 頁面顯示錯誤狀態（含啟動指令）
- 三條全 fail → 完整錯誤訊息

### Priority 3：data_status 標記（解模式 D、E）

**現況**：line 254-256 用 `|| []` 兜底，無法區分「後端回空陣列」與「schema 不符」。

**修改**：`loadStrategiesData` 回傳結構化結果：
```js
{
  data_status: 'ok' | 'partial' | 'empty' | 'failed',
  strategies: [],
  layers: [],
  coreIndicators: null,
  errors: { '/api/strategies': err, ... }
}
```

**測試**：
- mock 回 `{ strategies: [] }` → data_status='empty'，頁面顯示「資料庫尚無心法，請聯絡管理員」
- mock 回 `{}`（無 strategies 欄位）→ data_status='malformed'，頁面顯示「回應格式錯誤，請檢查後端版本」
- mock 回正確資料 → data_status='ok'

### Priority 4：Loading state 可見（解模式 flicker）

**現況**：line 71-75 同步移除 loading class，用戶看不到「載入中…」。

**修改**：
- 保留 `class="empty loading"` 直到 `render()` 完成
- 用 `app-utils.js:39-41` 的 `renderSkeleton(lines)` 取代 line 83-90
- skeleton 內每行加 `<div class="skeleton-line"></div>` 觸發 CSS 動畫（CSS 不在此 PR 範圍，但 class 已就位）

**測試**：
- renderStrategiesPage 期間查 DOM 應看到 `class="loading"`
- render 完成後 `class="loading"` 被移除

---

## 三、實作順序（TDD）

1. **Red**：寫 `web/static/js/__tests__/strategies.test.mjs`，覆蓋 5 種失敗模式 + data_status + loading state
2. **Green**：實作 `classifyFetchError` + 改寫 `fetchJSON` + 改寫 `loadStrategiesData` + 改寫錯誤渲染
3. **Refactor**：對齊 `app-utils.js` 既有風格（var、function declarations）
4. **Verify**：`node --test web/static/js/__tests__/strategies.test.mjs`、`lsp_diagnostics`、`gitnexus_detect_changes`

---

## 四、驗收標準

- [ ] `node --test web/static/js/__tests__/strategies.test.mjs` 全綠
- [ ] 5 種失敗模式都有對應測試
- [ ] 錯誤訊息含可行動 hint（啟動指令、檔案路徑）
- [ ] `data_status` 結構對齊 `parseSessionsList` 風格
- [ ] 部分失敗時頁面仍可用（顯示已成功部分 + 警告 banner）
- [ ] `lsp_diagnostics` 0 errors
- [ ] `gitnexus_detect_changes` 影響範圍 ≤ 3 symbols

---

## 五、不在本次範圍（給後續 PR）

- 其他頁面（parameters / performance-report）的 raw fetch 修復
- `main.js` 集中 health probe 增強（提前顯示 banner）
- `cmd/atlas/main.go` 啟動時 fail-fast（seed 載入失敗時不要繼續啟動）
- CSS skeleton 動畫
