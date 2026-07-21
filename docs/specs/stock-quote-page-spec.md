# Stock Quote Page — Wireframe + Component Tree

> **對應 issue**: #1038 client_web 新增「個股快查」功能
> **對應契約**: [`stock-api-contract.md`](./stock-api-contract-spec.md)
> **狀態**: 設計稿 v1.0(待使用者審閱)
> **最後更新**: 2026-07-09

---

## 1. 頁面定位與導航整合

### 1.1 在 client_web IA 中的位置

```
client_web 導航
├── 首頁 (Home)
├── 個股快查 (Stock Quote)  ← 新增
├── 資金流 (Capital Flow)    [Tier 1+]
├── 事件預測 (Events)        [Tier 1+]
├── 策略排名 (Strategies)    [Tier 1+]
├── 每日報告 (Daily Report)  [Tier 1+]
└── 投資組合 (Portfolio)     [Tier 2+]
```

**插入位置**:在「首頁」與「資金流」之間(直觀呼應「看個股」的場景)。

### 1.2 Deep Link 支援

- 完整 URL:`/client/quote?symbol=2330`
- 路由處理:`shared_web/static/js/main.js` 的 `switchPage()` 新增 `quote` case
- 從其他頁面跳轉入口:
  - **Portfolio 持倉點擊** → `/client/quote?symbol={stockId}`
  - **Strategies 排名個股** → 同上
  - **Recommendations 推薦個股** → 同上(若有推薦 detail 頁)
  - **URL 分享**:使用者可直接貼 `?symbol=2330` 連結

---

## 2. 頁面總覽 Wireframe

```
┌──────────────────────────────────────────────────────────────────┐
│ [logo] atlas           首頁 個股快查 資金流 ...     [user menu]    │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ 個股快查                              [最近查詢: 2330 ▾] │    │
│  │ ┌──────────────────────────────────────────┐ ┌───────┐  │    │
│  │ │ 輸入股票代碼: [ 2330          ] [查詢]    │ │熱門   │  │    │
│  │ └──────────────────────────────────────────┘ │2330   │  │    │
│  │                                              │2454   │  │    │
│  │                                              │2317   │  │    │
│  │                                              │台積電 │  │    │
│  │                                              └───────┘  │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ ╔═══ 台積電 (2330) ═══════════════════════════════════╗  │    │
│  │ ║  即時報價                            資料: Fugle 即時 ║  │    │
│  │ ║  ┌────────┐  漲跌: ▲ +15 (1.25%)                     ║  │    │
│  │ ║  │ 1,215  │  開盤 1,200 / 高 1,225 / 低 1,198       ║  │    │
│  │ ║  └────────┘  成交量 25,432 張                      ║  │    │
│  │ ╚══════════════════════════════════════════════════════╝  │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌────────────────────────────┐ ┌────────────────────────────┐    │
│  │ 基本面                       │ │ 籌碼(三大法人)             │    │
│  │ 資料日期: 2026-07-08(T-1)  │ │ 資料日期: 2026-07-08       │    │
│  │                              │ │                            │    │
│  │ PE     25.3     ━━━━●━━    │ │ 外資   ▲ +12,500 張        │    │
│  │ PB      6.1     ━━━━●━     │ │ 投信   ▲ +3,200 張         │    │
│  │ PS       —      資料未填   │ │ 自營商 ▼ -800 張           │    │
│  │ 殖利率   1.5%   ●━━━━━     │ │ ──────────────────────     │    │
│  │ 產業    半導體              │ │ 合計   ▲ +14,900 張        │    │
│  │ 同產業 PE 中位數: 22.5     │ │ (資料來源: TWSE)            │    │
│  └────────────────────────────┘ └────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ 技術指標(90 日)              計算基準: 2026-07-09 收盤    │    │
│  │ ┌──────────┐ ┌──────────┐ ┌──────────┐                  │    │
│  │ │ SMA20    │ │ SMA50    │ │ RSI14    │                  │    │
│  │ │ 1,210.5  │ │ 1,195.2  │ │   58.3   │                  │    │
│  │ │ 短期偏多 │ │ 中期偏多 │ │ 中性區   │                  │    │
│  │ └──────────┘ └──────────┘ └──────────┘                  │    │
│  │                                                           │    │
│  │ [歷史走勢 sparkline — 需後端擴充 API,暫以 quote 近似]   │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ⚠️ 本系統資料僅供研究參考,不構成投資建議。                       │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Component Tree(ES Module 結構)

### 3.1 檔案布局

```
shared_web/static/js/
├── pages/
│   └── stock-quote.js        ← 新增(page entry,動態 import)
├── components/
│   ├── stock-quote-header.js   ← 新增(報價 + 標題)
│   ├── stock-quote-fundamentals.js  ← 新增(基本面表)
│   ├── stock-quote-chips.js    ← 新增(籌碼條)
│   ├── stock-quote-technical.js ← 新增(技術 KPI + sparkline placeholder)
│   └── stock-quote-search.js   ← 新增(搜尋 + 熱門 + 歷史)
└── shared/
    └── stock-api-client.js     ← 新增(4 API 並發 + 錯誤處理)
```

### 3.2 依賴關係

```
pages/stock-quote.js (主頁)
  ├─ components/stock-quote-search.js
  │   └─ shared/stock-api-client.js (lookup helpers)
  ├─ components/stock-quote-header.js
  │   └─ shared/stock-api-client.js
  ├─ components/stock-quote-fundamentals.js
  │   └─ shared/stock-api-client.js
  ├─ components/stock-quote-chips.js
  │   └─ shared/stock-api-client.js
  ├─ components/stock-quote-technical.js
  │   └─ shared/stock-api-client.js
  └─ shared/trust-footer.js (免責聲明)

shared/stock-api-client.js
  └─ 共用:metric-card, sparkline, glossary-popover, risk-badge
```

### 3.3 與既有元件的復用對照

| 新元件 | 復用既有 | 原因 |
|---|---|---|
| stock-quote-header | `metric-card.js` | 報價大字需要 metric-card 樣式 |
| stock-quote-fundamentals | `glossary-popover.js` | PE/PB/PS 需白話註解 |
| stock-quote-chips | `risk-badge.js` | 買賣超顏色 token |
| stock-quote-technical | `metric-card.js` + `sparkline.js` | 3 個 KPI card + 走勢 placeholder |
| stock-quote-search | — | 新功能,搜尋框自寫 |
| stock-api-client | — | 新工具,封裝 4 API 並發 |

---

## 4. 狀態管理

### 4.1 全域狀態(在頁面內)

```javascript
// pages/stock-quote.js
const state = {
  currentSymbol: null,        // string, e.g., "2330"
  query: {                    // 4 API 的載入狀態
    quote: { status: 'idle', data: null, error: null },
    fundamentals: { status: 'idle', data: null, error: null },
    chips: { status: 'idle', data: null, error: null },
    technical: { status: 'idle', data: null, error: null },
  },
  recentSearches: [],         // string[],localStorage 持久化
  isInitialized: false,
};
```

### 4.2 狀態機

```
idle
  → use 點擊查詢/熱門
  → loading (4 API 並發)
    ├→ 全部成功 → loaded (4 個 section 都顯示資料)
    ├→ 部分失敗 → partial_loaded (失敗的 section 顯示 error)
    └→ 全部失敗 → error_state (整頁顯示錯誤 + 重試)
  → symbol 變更 → idle (重置)
```

### 4.3 並發 + 部分失敗策略

```javascript
// shared/stock-api-client.js
async function fetchStockBundle(symbol) {
  const results = await Promise.allSettled([
    fetchQuote(symbol),
    fetchFundamentals(symbol),     // 內部自動加 .TW
    fetchChips(symbol),
    fetchTechnical(symbol),
  ]);
  // 任一失敗不 throw,個別回傳 {status, data, error}
  return {
    quote: resultToState(results[0]),
    fundamentals: resultToState(results[1]),
    chips: resultToState(results[2]),
    technical: resultToState(results[3]),
  };
}
```

---

## 5. 細節設計

### 5.1 搜尋框(stock-quote-search.js)

| 元素 | 行為 |
|---|---|
| 輸入框 | 4-6 碼數字驗證(只允許 0-9),長度不足不發 query |
| Debounce | 500ms(避免連打觸發) |
| Enter 鍵 | 立即查詢 |
| 熱門個股 | 預設 4 檔:2330/2454/2317/0050,點擊直接查詢 |
| 最近查詢 | localStorage 儲存最近 5 檔,下拉選單 |
| 錯誤訊息 | 「找不到 XXX 這檔股票」/ 「網路連線失敗,稍後再試」 |

### 5.2 報價區(stock-quote-header.js)

| 元素 | 來源 | 顯示 |
|---|---|---|
| 公司名稱 + 代號 | `quote.symbol` + `chips.name` | 大標題 `台積電 (2330)` |
| 最新價 | `quote.last` | 大字 1,215(60px) |
| 漲跌 | `last - open` | ▲ +15 紅 / ▼ -15 綠 |
| 漲跌幅 | `(last - open) / open * 100` | +1.25% 紅 / -1.25% 綠 |
| 開盤/高/低 | `quote.open/high/low` | 三欄並排 |
| 成交量 | `Math.floor(quote.volume / 1000)` | 25,432 張 |
| 資料來源 | 固定 | 「資料:Fugle 即時」 |
| 失敗時 | quote=null | 「報價功能未啟用,請洽管理員」 |

### 5.3 基本面(stock-quote-fundamentals.js)

| 元素 | 來源 | 顯示 |
|---|---|---|
| 表格 | `fundamentals` object | 5 行 × 2 欄 |
| PE/PB/PS | 直接 | 數值 + 同產業中位數對照 |
| 殖利率 | 直接 | 1.5% |
| 產業 | `fundamentals.Sector` | 半導體 badge |
| PE 中位數 | **後續擴充**(目前 stock API 未提供) | 顯示「—」 + tooltip「待資料補齊」 |
| PS 缺失 | `PS === 0` | 顯示「—」 |
| Sector 缺失 | `Sector === ""` | 顯示「未分類」 |
| 資料日期 | 後續擴充(目前 stock API 未提供 mtime) | 顯示「資料日期:T-1(cron 更新)」 |

### 5.4 籌碼(stock-quote-chips.js)

| 元素 | 來源 | 顯示 |
|---|---|---|
| 外資買賣超 | `chips.foreign_investor_net` | ▲ +12,500 張 / ▼ -12,500 張 |
| 投信買賣超 | `chips.domestic_fund_net` | ▲ +3,200 張 |
| 自營商買賣超 | `chips.dealer_net` | ▼ -800 張 |
| 三大法人合計 | 前三者相加 | ▲ +14,900 張 |
| 視覺 | 條狀圖(橫向),顏色依買賣超 | 每行一條 |
| 資料日期 | `chips.date` | YYYY-MM-DD |
| 錯誤處理 | chips=null | 「籌碼資料當日無更新,請稍後再試」 |

### 5.5 技術指標(stock-quote-technical.js)

| 元素 | 來源 | 顯示 |
|---|---|---|
| SMA20 | `technical.sma20` | metric-card + 文字「短期偏多/偏空」 |
| SMA50 | `technical.sma50` | metric-card + 文字「中期偏多/偏空」 |
| RSI14 | `technical.rsi14` | metric-card + 文字「中性區/超買/超賣」 |
| 黃金/死亡交叉 | `sma20 > sma50` | 文字「黃金交叉」/「死亡交叉」 |
| Sparkline | 後續擴充 | 目前顯示「歷史走勢待 API 擴充」placeholder |
| 計算基準 | `new Date().toISOString().slice(0,10)` | YYYY-MM-DD(今日) |
| 顏色警示 | RSI 區間 | **用橘/黃(--risk-high),不用紅色** |

### 5.6 免責聲明(footer)

```html
⚠️ 本系統資料僅供研究參考,不構成投資建議。
投資決策應自行評估風險,並諮詢專業顧問。
```

---

## 6. 響應式設計(3 斷點)

| 斷點 | 寬度 | 布局 |
|---|---|---|
| Desktop | ≥ 1024px | 4 個 section 完整展開,基本面/籌碼並排 |
| Tablet | 768-1023px | 報價區完整,基本面/籌碼並排,技術區堆疊 |
| Mobile | < 768px | 4 section 全部堆疊,accordion 摺疊(預設展開第一個) |

---

## 7. 設計系統對齊

| 元素 | CSS Variable |
|---|---|
| 漲(紅) | `--pnl-profit` / `--trend-bullish` |
| 跌(綠) | `--pnl-loss` / `--trend-bearish` |
| 買超 | `--capital-inflow` |
| 賣超 | `--capital-outflow` |
| RSI 超買 | `--risk-high`(黃/橘,非紅) |
| RSI 超賣 | `--risk-low`(對比色) |
| 中性 | `--text-secondary` |
| 資料來源標籤 | `--text-tertiary` |
| 免責區底色 | `--bg-tertiary` |

**不寫死 hex/rgba**(遵守 CLAUDE.md 前端規範)。

---

## 8. Acceptance Criteria(最終版,取代 issue #1038 原始 AC)

### 8.1 功能性
- [ ] 在 client_web 輸入股票代碼可查詢 4 種資料
- [ ] 4 個 API 並發呼叫,TTFB < 2s(broadband)
- [ ] 任一 API 失敗不影響其他 3 個區塊顯示
- [ ] 熱門個股快捷可一鍵查詢
- [ ] 最近 5 檔查詢歷史(localStorage)
- [ ] 支援 deep link:`/client/quote?symbol=2330`

### 8.2 金融語意
- [ ] 基本面 PS=0 時顯示「—」,非「0」
- [ ] 基本面 Sector="" 時顯示「未分類」
- [ ] 籌碼單位「張」明示,不混淆為「股」
- [ ] 成交量從股換算為張(`Math.floor(volume/1000)`)
- [ ] 顏色符合台股慣例(紅漲綠跌)
- [ ] RSI 超買用黃/橘,非紅色
- [ ] 每個 section 顯示資料來源 + 資料時效
- [ ] 免責聲明永遠顯示於頁尾

### 8.3 設計系統
- [ ] Dark/light theme 切換正常
- [ ] 所有顏色用 CSS variable,不寫死
- [ ] 字體/間距與 home / portfolio 一致
- [ ] 響應式 3 斷點(mobile/tablet/desktop)
- [ ] 與 portfolio/strategies 的個股卡片視覺一致

### 8.4 測試
- [ ] E2E smoke test(client_web/smoke/run.mjs 整合)
- [ ] 視覺 QA(browse tool 對比 home page)
- [ ] a11y(鍵盤導航 + 螢幕閱讀器)
- [ ] 同產業 PE 中位數(等後端擴充,目前允許空)

### 8.5 文件
- [ ] `docs/specs/stock-api-contract-spec.md` 已完成(已 ship)
- [ ] `docs/specs/stock-quote-page-spec.md`(本文件)完成
- [ ] PR 描述引用 stock-api-contract.md

---

## 9. 已知限制與後續工作

| 限制 | 影響 | 緩解 / 後續 |
|---|---|---|
| 4 API 需 JWT 認證 | 未登入使用者看不到 | 透過 subscription 模組引導登入 |
| FugleClient 條件性啟用 | 無 API key 時 quote=500 | UI 顯示「報價功能未啟用」 |
| Technical 缺歷史 bars | 無法畫 sparkline | 等後端擴充 `?days=N` 回傳 30+ 資料點 |
| SectorMedianPE 未在 stock API 暴露 | 無同產業對照 | 後端擴充或前端從 fundamentals 自行計算 |
| QuoteSource 統一介面 | 4 API 各自有 fallback 機制 | 目前 OK,後續可考慮統一 |

---

## 10. 變更紀律

| 觸發 | 同步本文件章節 |
|---|---|
| 修改 stock-api-contract.md | §5 各 section |
| 新增/移除 component | §3 component tree |
| 變更 AC | §8 |
| 變更設計系統 token | §7 |

---

**文件版本**: v1.0(2026-07-09)
**下次 review**: PR #1038 合併時