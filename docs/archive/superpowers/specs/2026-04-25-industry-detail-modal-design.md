# 產業詳細分析彈窗設計規格

**日期**: 2026-04-25
**功能**: 產業生態系頁面 - 產業詳細分析彈窗

## 概述

擴展 `showIndustryDetail()` 函式，從目前的 `notify('開發中')` 改為開啟一個完整的 Modal 彈窗，整合該產業的四大維度分析資料。

## 現有基礎

- **前端**: `web/static/index.html` - 產業生態系頁面已有產業地圖、週期羅盤、供應鏈連動、季節性模式四個區塊
- **後端 API**: `/api/industry/cycle`, `/api/industry/linkage`, `/api/industry/seasonality`, `/api/industry/risk`
- **彈窗系統**: 已有 `.modal-overlay` + `.modal` CSS 架構，以及 `diffModal`, `promoteModal`, `infoModal` 三個現有彈窗

## 設計方案

### 方案 A: 整合彈窗（推薦）

開啟一個 Modal，內部以 Tab 或分區方式呈現四個維度：
1. **週期定位** - 景氣循環、庫存週期、資本支出週期、信心度、趨勢
2. **供應鏈連動** - 上游產業、下游產業、相關性矩陣、連動分數
3. **季節性模式** - 當前活躍模式、歷史準確度、典型報酬
4. **風險分析** - 風險列表、最高風險、嚴重程度分佈

**優點**: 一次載入完整資訊，使用者無需切換頁面
**缺點**: 首次載入資料較多

### 方案 B: 輕量彈窗 + 分頁載入

彈窗只顯示基本資訊 + Tab 切換，切換時才載入對應 API

**優點**: 初始載入快
**缺點**: 切換時有延遲感

## 推薦方案: 方案 A（整合彈窗）

原因：
- 現有 API 都是輕量查詢（記憶體內計算）
- 使用者點擊產業卡片的意圖就是「我要看完整資訊」
- 與現有設計風格一致（`infoModal` 也是單一彈窗呈現資訊）

## 彈窗結構

```
┌─────────────────────────────────────────────────────┐
│  📊 半導體產業詳細分析                    [X]       │
├─────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐ │
│  │ 週期定位 │ │ 供應鏈   │ │ 季節性   │ │ 風險   │ │
│  │  (active)│ │          │ │          │ │        │ │
│  └──────────┘ └──────────┘ └──────────┘ └────────┘ │
├─────────────────────────────────────────────────────┤
│                                                     │
│  [Tab 內容區域]                                     │
│                                                     │
│  景氣循環: 擴張期          庫存週期: 回補期         │
│  資本支出: 上升期          信心度: 78%              │
│  趨勢: 正向                                             │
│                                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │ 週期階段視覺化 (進度條)                        │   │
│  │ [復甦]====[擴張]====[成熟]====[衰退]           │   │
│  │        ▲                                          │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
├─────────────────────────────────────────────────────┤
│                                    [關閉]           │
└─────────────────────────────────────────────────────┘
```

## 前端實作

### 新增 HTML（在現有 modal 之後）

```html
<div class="modal-overlay" id="industryModal" role="dialog" aria-modal="true" onclick="if(event.target===this)closeIndustryModal()">
  <div class="modal" style="width:min(720px,94vw)">
    <h3 id="industryModalTitle">產業詳細分析</h3>
    <div class="industry-tabs" id="industryTabs">
      <button class="tab-btn active" data-tab="cycle">週期定位</button>
      <button class="tab-btn" data-tab="linkage">供應鏈</button>
      <button class="tab-btn" data-tab="seasonality">季節性</button>
      <button class="tab-btn" data-tab="risk">風險</button>
    </div>
    <div id="industryModalContent">載入中…</div>
    <div class="control-group" style="margin-top:14px;justify-content:flex-end">
      <button onclick="closeIndustryModal()">關閉</button>
    </div>
  </div>
</div>
```

### 新增 CSS

```css
.industry-tabs { display: flex; gap: 4px; margin-bottom: 14px; border-bottom: 1px solid var(--border); }
.tab-btn { background: transparent; border: none; color: var(--muted); padding: 8px 14px; cursor: pointer; font-size: 13px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
.tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); }
.tab-btn:hover { color: var(--text); }
```

### 新增 JavaScript

```javascript
let currentIndustryId = '';
let currentIndustryData = {};

async function showIndustryDetail(industryId) {
  currentIndustryId = industryId;
  document.getElementById('industryModal').classList.add('show');
  document.getElementById('industryModalTitle').textContent = '載入中...';
  document.getElementById('industryModalContent').innerHTML = '<div class="empty">載入中…</div>';
  
  try {
    const [cycle, linkage, seasonality, risk] = await Promise.all([
      getJSON(`/api/industry/cycle?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/linkage?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/seasonality?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/risk?industry=${encodeURIComponent(industryId)}&symbol=ALL`).catch(() => null),
    ]);
    
    currentIndustryData = { cycle, linkage, seasonality, risk };
    renderIndustryModalContent('cycle');
  } catch (e) {
    document.getElementById('industryModalContent').innerHTML = '<div class="empty">載入失敗</div>';
  }
}

function closeIndustryModal() {
  document.getElementById('industryModal').classList.remove('show');
  currentIndustryId = '';
  currentIndustryData = {};
}

function renderIndustryModalContent(tab) {
  // 根據 tab 渲染對應內容
}
```

## 後端變更

### 修改 `/api/industry/risk`

目前 `handleIndustryRisk` 要求 `symbol` 參數。為了支援產業層級風險查詢，需要放寬限制：

```go
// 當 symbol == "ALL" 時，回傳該產業下所有標的的彙總風險
if symbol == "ALL" {
    // 取得該產業下所有標的，彙總風險
}
```

或新增 `/api/industry/detail?industry=xxx` 整合 API。

## 資料流

```
使用者點擊產業卡片
    ↓
showIndustryDetail(industryId)
    ↓
並行呼叫 4 個 API
    ↓
渲染彈窗內容（預設顯示週期定位 Tab）
    ↓
使用者可切換 Tab 查看不同維度
```

## 驗收標準

- [ ] 點擊產業卡片開啟彈窗
- [ ] 彈窗顯示產業名稱
- [ ] 四個 Tab 可正常切換
- [ ] 週期定位 Tab 顯示：景氣循環、庫存週期、資本支出週期、信心度、趨勢
- [ ] 供應鏈 Tab 顯示：上游、下游、相關性矩陣
- [ ] 季節性 Tab 顯示：活躍模式、歷史準確度
- [ ] 風險 Tab 顯示：風險列表、最高風險
- [ ] 點擊彈窗外部或關閉按鈕可關閉
- [ ] 支援深色/淺色主題
- [ ] 行動裝置友善（寬度自適應）

## 風險與注意事項

1. **API 相容性**: `/api/industry/risk` 目前要求 `symbol` 參數，需要調整為可選或支援 `ALL`
2. **效能**: 四個 API 並行呼叫，任一失敗不影響其他資料顯示
3. **錯誤處理**: 每個 API 失敗時顯示對應錯誤訊息，不讓整個彈窗崩潰
