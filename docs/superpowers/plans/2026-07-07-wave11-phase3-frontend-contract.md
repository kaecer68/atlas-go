# Wave 11 Phase 3: 前端欄位合約與頁面生命週期

> 目標：修復 PR #971–#975 重構後，client_web 頁面與後端 API 欄位名稱不匹配、smoke test selector 過時、以及頁面資料載入生命週期的缺口。

## 背景

AGENTS.md 高頻陷阱已明確指出兩個前端問題：

1. **Frontend-backend field name mismatch**：`CompositeMacroProvider` 回傳 `sox_index`/`spx_index`/`ndx_index`/`dji_index` 與 `score`，但前端若用舊欄位名 `sox`/`index` 會永遠拿到 `null`，導致 silent render failure。
2. **Page ID rename (overview → home)**：Phase 1 IA redesign 將 `data-page="overview"` 改為 `"home"`，smoke test 的 `PAGES_ARG` 與 `PAGE_SELECTORS` 必須同步更新。

本階段要系統性盤查 `client_web` 所有頁面與對應後端 API 的欄位合約，修正過時欄位與 selector，並補強 smoke test 覆蓋。

## 階段範圍

**屬於 Phase 3**：

1. 盤查 `client_web/static/` 各頁面 JS，對照後端 API response keys
2. 修正 CompositeMacroProvider / macro snapshot 相關欄位名稱
3. 更新 smoke test 的 `PAGES_ARG` 與 `PAGE_SELECTORS`（overview → home）
4. 補齊或修正 `client_web/tests/` 中對應的單元/整合測試
5. 檢查頁面資料載入生命週期（loading、error、empty state、retry）
6. 更新 `client_web/AGENTS.md` 與相關前端文件
7. commit → push → PR #978

**不屬於 Phase 3**（留待 Phase 4+）：

- 後端資料提供者 stub 移除
- 新增頁面功能
- 安全強化

## 實作步驟

### 3.1 盤查前端欄位使用

- 搜尋 `client_web/static/` 中引用 `sox`、`spx`、`ndx`、`dji`、`index`（作為 macro score）、`overview` 的位置
- 對照後端 `internal/monitoring/service/macro.go` 或 `CompositeMacroProvider` 的實際回傳欄位
- 列出所有欄位名稱漂移點

### 3.2 修正欄位名稱

- `sox` → `sox_index`
- `spx` → `spx_index`
- `ndx` → `ndx_index`
- `dji` → `dji_index`
- `index`（在 macro snapshot context 中）→ `score`

### 3.3 更新 smoke test

- `client_web/smoke/run.mjs`：更新 `PAGES_ARG` 與 `PAGE_SELECTORS` 中 `overview` → `home`
- `admin_web/smoke/run.mjs`：同步檢查是否也有 overview → home 的遺留

### 3.4 補強測試

- 確認 `client_web/tests/` 有覆蓋 macro snapshot render
- 若無，新增最小測試確保欄位映射正確

### 3.5 頁面生命週期檢查

- 檢查 home 頁面是否有 loading、error、empty state、retry 機制
- 若資料載入失敗，頁面是否給出明確回饋（而非空白或無限 loading）

### 3.6 文件更新

- `client_web/AGENTS.md`：更新已知陷阱與欄位合約
- `docs/superpowers/plans/2026-07-07-wave11-phase3-frontend-contract.md`（本文件）

## 驗收標準

- [ ] `client_web` build 成功
- [ ] `client_web` smoke test 通過
- [ ] `client_web/tests/` 全綠
- [ ] 前端無引用舊欄位名 `sox`、`spx`、`ndx`、`dji`（在 macro context 中）
- [ ] 前端無引用舊 page id `overview`
- [ ] PR #978 已開出

## 風險與注意

- 前端欄位修正必須與後端 API 實際回傳一致，修改前須用 `curl /api/macro/snapshot/latest | jq keys` 確認
- smoke test 若依賴外部資源（如後端 server 啟動），本地執行時需確認環境
- 不要在本階段引入新功能，只做對齊與修復
