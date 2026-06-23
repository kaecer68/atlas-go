# INVESTIGATION: 投資心法頁面「無數據載入」根因調查

**日期**: 2026-06-23
**分支**: `fix/strategies-page-backend-unavailable`
**模式**: Read-only audit（依 `atlas-pre-change-protocol` Step I-1 ~ I-4）
**範圍**: `web/static/js/pages/strategies.js`（338 行）

---

## 一、問題描述

使用者回報前端【投資心法】頁面顯示「載入失敗：Failed to fetch」，但頁面原始報錯訊息背後至少隱藏 5 種不同的失敗模式，目前程式碼無法區分。

---

## 二、載入流程追蹤（line 62-99）

```text
renderStrategiesPage(root)             ← line 62
  ├─ root.innerHTML = 載入中…            ← line 71  (class="empty loading")
  ├─ slot.innerHTML = renderSkeleton()  ← line 75  (4 個空 div 佔位)
  ├─ try {
  │    await loadStrategiesData()       ← line 77
  │    render()                         ← line 78  (5 個子 render)
  │  } catch (e) {
  │    slot.innerHTML = 載入失敗：e.message  ← line 80
  │  }
```

---

## 三、五種「無數據」失敗模式

### 🔴 模式 A：Backend 完全不可達

| 項目 | 細節 |
|---|---|
| 觸發 | 3 條 endpoint 全部連線失敗（TCP RST） |
| fetchJSON 行為 | `await fetch()` 拋 `TypeError: Failed to fetch` |
| 結果 | 顯示「載入失敗：Failed to fetch」（line 80） |
| 用戶能否自救 | ❌ 不知道是 port 沒開、後端沒起、還是 CORS |

### 🔴 模式 B：Backend 起來但 registry 未初始化（503）

| 項目 | 細節 |
|---|---|
| 觸發 | `cmd/atlas/main.go:972-983` 載入 `strategy_techniques.json` 失敗 → handler 仍 wire 但每次回 503 |
| fetchJSON 行為 | `r.ok === false` → 拋 `Error('/api/strategies -> 503')` |
| 結果 | 顯示「載入失敗：/api/strategies -> 503」（line 80） |
| 用戶能否自救 | ❌ 看到 HTTP code 但不知道是 seed 檔案問題 |

### 🔴 模式 C：3 條 endpoint 部分失敗（最危險的靜默類型）

| 項目 | 細節 |
|---|---|
| 觸發 | `/api/strategies` 成功、`/api/strategies/layers` 失敗 |
| loadStrategiesData 行為 | line 249 `Promise.all` —— **任何一個 reject 就 throw** |
| 結果 | **整個頁面直接進入錯誤狀態**，成功的 strategies 資料丟掉不顯示 |
| 為什麼危險 | decision-chain 有 `.catch(() => null)`（line 252），但前 2 條沒有，**容忍度不對稱** |

### 🟡 模式 D：Backend 回 200 但資料為空

| 項目 | 細節 |
|---|---|
| 觸發 | seed 載入成功但 9 條心法全被 filter 掉、或 schema 改變 |
| fetchJSON 行為 | `r.ok === true`，回傳 `{ strategies: [] }` |
| 結果 | STATE.strategies = [] → renderStrategyCards line 155 顯示「此層尚無心法，點擊下方「＋ 新增心法」開始建立」 |
| **問題** | 用戶看到「此層尚無心法」會以為是 UI 沒新增按鈕（其實 backend 沒資料）—— **靜默失敗，沒任何錯誤標記** |

### 🟡 模式 E：Backend 回 200 但 schema 不符

| 項目 | 細節 |
|---|---|
| 觸發 | 後端欄位改名（例如 `core_indicators` → `indicators`） |
| loadStrategiesData 行為 | line 254-256 用 `|| []` / `|| null` 兜底，**不驗 schema** |
| 結果 | STATE.strategies = []、STATE.coreIndicators = null → 頁面顯示空 KPI 卡 + 空策略卡 |

---

## 四、載入狀態生命週期問題

### 🔴 Line 71 → 75 的「loading 看不到」

```js
root.innerHTML = `
  <div id="strategiesContent" class="empty loading">載入中…</div>   ← line 71
`;
const slot = root.querySelector('#strategiesContent');
slot.classList.remove('loading');                                       ← line 74（同步立即移除）
slot.innerHTML = renderSkeleton();                                      ← line 75（換成 4 個空 div）
```

**問題**：`class="empty loading"` 設了又同步移除，**用戶根本看不到「載入中…」**，直接看到 skeleton。Skeleton 也沒有 `animation` class，是純空白 div —— 與「已載入但資料為空」視覺上幾乎一樣。

### 🟡 renderSkeleton（line 83-90）沒有任何動畫

```js
function renderSkeleton() {
  return `
    <div class="kpi-grid" id="kpiStrip"></div>
    <div class="kpi-grid mt-sm" id="coreIndicatorStrip"></div>
    ...
  `;
}
```

4 個空容器，沒有 `skeleton-line` class（雖然 `app-utils.js:39-41` 有 `renderSkeleton(lines)` 工具但**這個頁面沒用**）。

---

## 五、Schema 假設清單（無驗證）

`loadStrategiesData`（line 254-256）假設：

| 假設 | 對應欄位 | 假設失敗時的表現 |
|---|---|---|
| `strategiesResp.strategies` 是陣列 | line 254 | 若後端回 `[]` 或欄位改名 → 「此層尚無心法」 |
| `layersResp.layers` 是陣列 | line 255 | 若為 null → KPI 卡顯示「5 層覆蓋：0/5」 |
| `chainResp.core_indicators` 是物件 | line 256 | 若為 null → 4 個指標卡顯示 0.00% |
| `s.layer` ∈ {L1, L2, L3, L4, L5} | line 154 | 若後端出現 L6 → 在「全部」tab 出現但切到 L1-L5 都看不到 |

**沒有任何欄位驗證、沒有 console.warn、沒有資料狀態標記（`data_status`）** —— 對照 `atlas-data-visibility` skill 的 L4 要求，這頁完全沒實作。

---

## 六、風險矩陣（同一頁面內）

| 失敗情境 | 用戶看到 | 後端 log | 開發者察覺難度 |
|---|---|---|---|
| Backend 沒起 | 載入失敗：Failed to fetch | 無（連線沒到） | 容易（看訊息） |
| Backend 503 | 載入失敗：/api/strategies -> 503 | 有 | 容易 |
| 部分 endpoint 失敗 | 載入失敗：第一個失敗的 URL -> status | 有 | 中等 |
| **回空陣列** | 「此層尚無心法」 | 無 | **困難（容易誤判 UI 問題）** |
| **Schema 不符** | 空 KPI 卡 + 空策略卡 | 無 | **困難** |
| **Loading 看不到** | （直接看到 skeleton / 空內容） | 無 | 困難 |

---

## 七、結論

【投資心法】頁面的數據載入**沒有健壯的檢查機制**：
- 載入狀態看不到（loading flicker）
- 錯誤訊息未翻譯（Failed to fetch 直接給用戶）
- 部分失敗容忍度不對稱（decision-chain 有 catch，其他沒有）
- Schema 無驗證（空資料 vs 壞資料無法區分）
- 無 data_status 標記（不符合 atlas-data-visibility L4 規範）

5 種失敗模式中，模式 A（backend 沒起）只是冰山一角；模式 D（回空陣列）才是最容易誤導用戶的隱性 bug。

---

## 八、給實作者的具體建議

見 `REPAIR_PLAN.md`（同一分支內）。
