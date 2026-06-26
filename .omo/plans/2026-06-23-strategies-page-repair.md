# REPAIR PLAN: 投資心法頁面數據載入健壯性修復

**日期**: 2026-06-23
**對應**: `docs/investigations/2026-06-23-strategies-page-no-data.md`（同分支）
**目標**: 將 5 種「無數據」失敗模式全部顯式化，並符合 `atlas-data-visibility` L4 規範

---

## 一、修復範圍與非目標

### 範圍（IN）
- `web/static/js/pages/strategies.js`（491 行）—— 主要修改
- `web/static/js/shared/fetch-error.js` —— 新增 `classifyFetchError` 工具
- `web/static/js/__tests__/*.test.mjs` —— 4 個測試檔（拆分為 `fetch-error` / `fetch-error-timeout` / `process-strategies-results` / `render-partial-banner`）

### 非目標（OUT）
- 不修改 backend Go 程式碼
- 不修改 `main.js` 集中 health probe（屬於另一個 cross-cutting fix，留給其他工作區）
- 不修改 `parameters.js` / `performance-report.js` 等其他頁面（同樣留給其他工作區）
- 不修改 CSS（skeleton 動畫暫不改樣式，只用既有 `class="loading"` 觸發可見狀態）

---

## 二、修復項目（按優先級）

### Priority 1：fetchJSON 錯誤分類（解模式 A、B）

**現況**：`fetchJSON` 函式直接拋出 `TypeError` 或 `Error(HTTP ...)`，由 `renderStrategiesPage` 的 catch block 原樣渲染。

**修改**：
- 新增 `web/static/js/shared/fetch-error.js` 工具函式 `classifyFetchError(err, url)`，回傳結構：
  ```js
  { kind: 'network' | 'http_503' | 'http_5xx' | 'http_4xx' | 'http_410' | 'timeout' | 'schema' | 'unknown',
    message: string,
    recoverable: boolean,
    hint: string }
  ```
- `strategies.js` 改用 `classifyFetchError` 後渲染

**測試**：
- mock `fetch` 拋 `TypeError` → 渲染含「後端未啟動」+ 啟動指令
- mock `fetch` 回 503 → 渲染含「registry 未初始化」+ seed 檔案路徑
- mock `fetch` 回 500 → 渲染含「後端錯誤」
- mock `fetch` 回 404 → 渲染含「請求被拒」

### Priority 2：Partial-failure tolerance（解模式 C）

**現況**：`loadStrategiesData` 使用 `Promise.all`，容忍度不對稱（只有 decision-chain 有 catch）。

**修改**：改為 `Promise.allSettled`，並對每條 endpoint 設定分級失敗處理：
- `/api/strategies` → **核心**，失敗 → `dataStatus='partial'`（單一核心失敗）或 `'failed'`（兩個核心失敗）
- `/api/strategies/layers` → **核心**，失敗 → 同上邏輯
- `/api/dashboard/decision-chain` → **輔助**，失敗 → 指標卡顯示「--」+ 黃色 banner「短線指標無法顯示」，不影響 `dataStatus`（核心成功時為 `'ok'`、同時僅 indicators 失敗時降為 `partial`/`failed`）

**`dataStatus` 閾值**（於 `processStrategiesResults()`）：
- `coreFailures === 0` → `'ok'`（strategies 非空）或 `'empty'`（strategies 空）
- `coreFailures === 1` → `'partial'`
- `coreFailures >= 2` → `'failed'`

**測試**：
- mock strategies=ok、layers=fail → `dataStatus='partial'`、頁面顯示 strategies 內容、頂部黃色 banner「部分資料載入失敗」
- mock strategies=fail、layers=ok → `dataStatus='partial'`、顯示錯誤（含啟動指令）
- mock strategies=fail、layers=fail → `dataStatus='failed'`、完整錯誤訊息
- 三條全 fail 但都是 schema → 同 `'failed'`（schema 計入 coreFailures）

### Priority 3：data_status 標記（解模式 D、E）

**現況**：`loadStrategiesData` 用 `|| []` 兜底，無法區分「後端回空陣列」與「schema 不符」。

**修改**：`processStrategiesResults` 回傳結構化結果：
```js
{
  dataStatus: 'idle' | 'ok' | 'partial' | 'empty' | 'failed',
  strategies: [],
  layers: [],
  coreIndicators: null,
  indicatorsError: null,
  errors: { '/api/strategies': classified, ... }
}
```

**說明**：本實作**不**新增獨立的 `'malformed'` 狀態；schema 錯誤（缺 `strategies` / `layers` 欄位）會被計入 `coreFailures`，因此同樣走 `'partial'` / `'failed'` 路徑，並在 errors map 內以 `kind: 'schema'` 區分。

**測試**：
- mock 回 `{ strategies: [] }` → `dataStatus='empty'`、頁面顯示「資料庫為空」banner
- mock 回 `{}`（無 strategies 欄位）→ 計入 coreFailures、`dataStatus='partial'` 或 `'failed'`、errors map 含 `kind: 'schema'`
- mock 回正確資料 → `dataStatus='ok'`

### Priority 4：Loading state 可見（解模式 flicker）

**現況**：先前實作同步移除 loading class，用戶看不到「載入中…」。

**修改**：
- 保留 `class="empty loading"` 直到 `render()` 完成
- skeleton 內每行加 `<div class="skeleton-line"></div>` 觸發 CSS 動畫（CSS 不在此 PR 範圍，但 class 已就位）

**測試**：
- renderStrategiesPage 期間查 DOM 應看到 `class="loading"`
- render 完成後 `class="loading"` 被移除

---

## 三、實作順序（TDD）

1. **Red**：寫測試於 4 個檔案：
   - `web/static/js/__tests__/fetch-error.test.mjs` —— 11 個 case 覆蓋 network / 503 / 5xx / 4xx / 410 / unknown
   - `web/static/js/__tests__/fetch-error-timeout.test.mjs` —— 3 個 case 覆蓋 AbortError
   - `web/static/js/__tests__/process-strategies-results.test.mjs` —— 16 個 case 覆蓋 5 種失敗模式 + data_status 閾值
   - `web/static/js/__tests__/render-partial-banner.test.mjs` —— 10 個 case 覆蓋 banner 渲染
2. **Green**：實作 `classifyFetchError` + 改寫 `fetchJSON` + 改寫 `loadStrategiesData` + 改寫錯誤渲染
3. **Refactor**：抽出 `processStrategiesResults` / `renderPartialBanner` 為 pure function + 模組測試
4. **Verify**：`node --test web/static/js/__tests__/`、`lsp_diagnostics`、`gitnexus_detect_changes`

---

## 四、驗收標準

- [ ] `node --test web/static/js/__tests__/` 全綠
- [ ] 5 種失敗模式都有對應測試
- [ ] 錯誤訊息含可行動 hint（啟動指令、檔案路徑）
- [ ] `dataStatus` 結構與 `processStrategiesResults` 對齊
- [ ] 部分失敗時頁面仍可用（顯示已成功部分 + 警告 banner）
- [ ] `lsp_diagnostics` 0 errors
- [ ] `gitnexus_detect_changes` 影響範圍 ≤ 3 symbols

---

## 五、不在本次範圍（給後續 PR）

- 其他頁面（parameters / performance-report）的 raw fetch 修復
- `main.js` 集中 health probe 增強（提前顯示 banner）
- `cmd/atlas/main.go` 啟動時 fail-fast（seed 載入失敗時不要繼續啟動）
- CSS skeleton 動畫
