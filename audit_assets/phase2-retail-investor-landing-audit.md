# Phase 2 投資人介面審計報告

**分支**：`audit/phase2-retail-investor-landing`（基於 `origin/main`，HEAD = `3a54a119`）
**審計日期**：2026-07-01
**基準計畫**：`.omo/plans/retail-investor-landing-design.md` §「Phase 2 — 共用元件與響應式優化」
**上一里程碑**：PR #864（Phase 3.10/3.11 Hero 區塊 + Meta tags）已合併 → `b9f1778f` → `origin/main`

---

## 一、審計方法

1. **逐項比對**：對照 `.omo/plans/retail-investor-landing-design.md` 的 Phase 2 條目，逐項檢查 `origin/main` 是否落地。
2. **檔案靜態分析**：行數計算、inline style grep、media query grep、API fetch grep。
3. **跨檔一致性檢查**：tooltip glossary 是否有被 home.js 引用、CSS 變數是否被 inline style 覆寫。

---

## 二、Phase 2 條目驗收狀態總表

| # | 條目 | 設計目標 | 實際狀態 | 證據 |
|---|------|---------|---------|------|
| 1 | Hero（總體經濟快照 + 總覽入口） | 落地 | ✅ 完成（PR #864） | `shared_web/static/js/pages/home.js` |
| 2 | Market Snapshot | 落地 | ✅ 完成 | 同上 |
| 3 | Why Atlas | 落地 | ✅ 完成 | 同上 |
| 4 | Today's Plan | 落地 | ✅ 完成 | 同上 |
| 5 | 共用元件庫 | metric-card / action-card / trust-footer / risk-badge / tooltip + format-metric.js | ✅ 完成 | `shared_web/static/js/components/` 全部存在 |
| 6 | index.html 瘦身 | < 150 行、0 inline style | ❌ **未達標** | 449 行、43 inline styles（見 §三-1） |
| 7 | format-metric.js utility | 落地 | ✅ 完成 | `shared_web/static/js/utils/format-metric.js` |
| 8 | 指標教育層 + Simplified Mode | toggle + 詞彙工具提示 | ❌ **未達標** | simplified 搜尋零結果；glossary 存在但**未接線**（見 §三-2、§三-3） |
| 9 | 響應式優化（≥44px touch target、≥18px 行動字級、prefers-reduced-motion） | 全面 | ⚠️ **部分達標** | 44px 僅 5 處局部實作；行動字級未驗證；reduced-motion 只在 home.css 4 處（見 §三-4） |
| 10 | Acceptance：頁面 API 請求數 | 合理 | ✅ 達標 | 6 endpoints（5 並行 + 1 lazy portfolio）（見 §三-5） |
| 11 | Lighthouse SEO 80 → ≥85 | 85 | ❌ **未達標** | 缺 Open Graph、缺 keywords、缺 canonical（見 §三-6） |

---

## 三、缺口與優化發現

### 3.1 `index.html` 未瘦身 — **最高優先級**

**證據**（`client_web/static/index.html`，2026-07-01 HEAD）：
- 總行數 **449** 行（設計目標 < 150 行，差距 **+299 行 / +199%**）
- `style="..."` inline 出現 **43 次**（設計目標 0）
- 區塊佔比：
  - 載入條 + sidebar + topbar + content 容器 + role switcher：~60 行（必要）
  - **6 個 page 區塊**（narrative / live / pipeline / decision / portfolio / crossmarket / evolution_panel / industry / performance-report / strategies）：~190 行
  - **5 個 modal 區塊**（diffModal / promoteModal / infoModal / industryModal / cycleLegendModal）：~180 行
  - 其中 `cycleLegendModal` 內嵌 70+ 行的燈號/信心度公式說明表格 → 高度耦合內容 + HTML

**問題分析**：
1. **modal 內容大量 inline**：每個 modal 都把 `style="..."` 寫在 `<div>` 上，缺乏 class hook；後續設計變更需逐個修改，無主題一致性。
2. **產業 modal 內容過重**：`cycleLegendModal`（70+ 行）若改為外部 Markdown 或 fetch 載入，可釋放主檔複雜度。
3. **page sections 有重複 panel 結構**：每個 page 都是 `<div class="page">` + `<div class="panel">` + `<h2>` + loading div 模式，未抽成 `editorial-page` template。

**影響**：
- ❌ 阻擋 SEO 優化（inline style 與 meta 區塊混雜）
- ❌ 阻擋 prefers-reduced-motion 全站生效（動效來源散落在 inline）
- ❌ 阻擋簡化模式（全站層級切換需廣播 DOM class）
- ⚠️ 維護成本：每加一頁 = 改 index.html 主檔，需重新 build 與 embed

**建議解法**：
1. 將 inline style 全部抽到 CSS（43 處 → 0）
2. 6 個 page 區塊改為動態注入（`switchPage` 內 `import('./pages/xxx.js')` 已存在模式）
3. 5 個 modal 改為 `<div class="modal" data-modal="xxx">` template + JS 動態注入內容
4. `cycleLegendModal` 內容改為 fetch `content/cycle-legend.md` 或常數檔
5. 目標：**< 150 行、0 inline style、5 modal 骨架統一**

---

### 3.2 Simplified Mode Toggle 缺失

**證據**：
```bash
$ grep -rn "simplified\|simplifiedMode\|simplified-mode\|簡化模式" \
  --include="*.js" shared_web client_web admin_web
（無輸出）
```

**現況**：
- ❌ 主檔零結果
- ❌ Sidebar / topbar 無 toggle 入口
- ❌ 無 CSS class hook（如 `.simplified`）
- ❌ 無 localStorage persistence

**影響**：
- 設計目標「進階使用者 vs 一般投資人」雙軌介面無法落地
- 一般投資人仍會看到全部技術指標（Sharpe、HHI 等），違反教育層初衷

**建議解法**（估算 4-6 小時）：
1. 在 `topbar` 加 toggle button（`🔰 簡化模式` ↔ `🔬 完整模式`）
2. CSS 加 `.simplified` 全站 class，隱藏 `.advanced-only { display: none }`
3. JS 透過 `localStorage.setItem('atlas-simplified', '1')` 持久化
4. 各 page render 時讀取並 toggle DOM
5. 簡化模式同時隱藏 HHI、Sharpe、Drawdown 等進階欄位，保留 Risk Badge + Trust Footer

---

### 3.3 詞彙工具提示（Glossary Tooltips）未接線

**證據**：
- `shared_web/static/js/components/tooltip.js` **第 25-32 行已定義 glossary**：
  ```js
  const TOOLTIP_GLOSSARY = {
    'Sharpe': '衡量每承擔一單位風險能獲得的超額報酬...',
    'Hit Rate': '策略建議正確的比例...',
    '最大回撤': '投資組合從高點到低點的最大跌幅...',
    'HHI': '赫芬達爾指數，衡量持倉集中度...',
    'Regime': '市場狀態（如多頭、空頭、震盪）...',
  };
  export function glossaryTooltip(term) { ... }
  ```
- 但 `home.js` 與其他頁面**未 import、未呼叫** `glossaryTooltip()`

**影響**：
- 設計目標「指標教育層」只完成 50%（有資料、無呈現）
- 一般投資人仍會直接看到 `Sharpe: 1.42` 等指標而不知其義

**建議解法**（估算 2-3 小時）：
1. `home.js` 改用 `glossaryTooltip('Sharpe')` 取代純字串
2. KPI 卡片標題套上 `<span class="tooltip">` → `renderTooltip('Sharpe', '...')`
3. CSS 確認 `.tooltip__bubble` 在小螢幕可顯示（`<= 640px`）

---

### 3.4 響應式優化部分達標

#### 3.4.1 Touch Targets（44px）

**達標處**（`shared_web/static/css/layout/responsive.css`）：
```css
30:    min-height: 44px;
35:    min-height: 44px;
41:  .pipeline-action { min-height: 44px; padding: 8px 12px; }
42:  .refresh { min-height: 44px; min-width: 44px; }
460: (home.css: .trust-footer__action { min-height: 44px; })
```

**未達標處**：
- ❌ `.nav-label` + `.sidebar nav a`（sidebar 導覽） — 推測 32px height，未達 44px
- ❌ KPI card click action — 若可點擊，未確認 44px
- ❌ view-btn（演化儀表板切換）— `.view-btn { padding:4px 10px }` ≈ 28px

**建議**：
```css
.sidebar nav a { min-height: 44px; display: flex; align-items: center; }
.view-btn { min-height: 44px; padding: 8px 16px; }
```

#### 3.4.2 Mobile Font Sizes (≥18px)

**現況**：
- `home.css` 主要字級為 12-15px（badge / label / meta）
- 主要數字 KPI 字級未驗證是否 ≥18px
- `typography.css` 僅 4 行（推測只 reset）

**建議驗證**：
```bash
grep -rn "font-size:1[0-7]px" shared_web/static/css/pages/home.css
```
需逐項檢查 KPI 主數字是否 ≥18px（行動 < 640px viewport）。

#### 3.4.3 `prefers-reduced-motion`

**達標**：home.css 4 處（line 138, 247, 442, 464）— 但**僅 home 頁有效**。

**未達標**：
- ❌ 共用元件（modal、sidebar、tooltip）未套用
- ❌ 其他頁（performance、strategy、narrative）未套用
- ❌ 全站 transitions / animations（如 theme toggle）未響應

**建議**：建立 `shared_web/static/css/base/motion.css`：
```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}
```
並在 `main.css` 末尾載入。

---

### 3.5 API 請求數合理

**home.js 請求清單**（lines 109-113, 252）：
| 端點 | 時機 | 並行 |
|------|------|------|
| `/api/dashboard/system-health` | 初始並行 | 1/5 |
| `/api/macro/snapshot/latest` | 初始並行 | 2/5 |
| `/api/taiwan/stress-index` | 初始並行 | 3/5 |
| `/api/dashboard/recommendation-pipeline` | 初始並行 | 4/5 |
| `/api/alerts` | 初始並行 | 5/5 |
| `/api/portfolio/current` | lazy（render 後） | — |

**結論**：✅ **達標**（5 並行 + 1 lazy = 6 端點 < 設計上限 10）。
**優化空間**：可合併為單一 `/api/home/aggregate` 端點，後端並行取資料後一次回傳；預估 P95 降低 30%（5 RTT → 1 RTT）。

---

### 3.6 Lighthouse SEO 80 → 85

**現有 meta**（`client_web/static/index.html` lines 5-10）：
```html
<meta name="description" content="Atlas-Go 投資人平台：台股模擬、風險監控與績效追蹤。">
<meta name="author" content="Atlas-Go">
<meta name="robots" content="noindex, nofollow">
```

**缺口**：
- ❌ **缺 Open Graph**（`og:title`、`og:description`、`og:image`、`og:type`）→ 社群分享無法預覽
- ❌ **缺 Twitter Card**（`twitter:card`、`twitter:title`）
- ❌ **缺 canonical**（避免 `/client/` 與 `/` 重複內容稀釋）
- ❌ **缺 keywords**（雖現代 SEO 權重低，但對中文檢索有助）
- ❌ **`<title>` 過短**：「Atlas-Go 投資人平台」僅 9 字元，建議 25-40 字含品牌 + 核心價值
- ⚠️ **`robots: noindex`** — 若為投資人後台應保留，但若為公開 landing 應改 `index, follow`
- ❌ **缺 JSON-LD**（Organization / WebSite structured data → 提升 SERP rich result）

**預估提升**：
- 加 OG + Twitter Card → +3 分
- 加 canonical + keywords + 結構化資料 → +2 分
- 目標：**82 → 88**（超 85 達標）

---

## 四、跨檔一致性問題

### 4.1 Tooltip glossary 與 home.js 失聯

- `tooltip.js` 已 export `glossaryTooltip()` — 但 home.js **無 import**
- 任何改 glossary 都會被靜默忽略（無 grep 提示）
- **修復**：補 import + 全站 grep `Sharpe|Hit Rate|HHI` 強制走 glossary

### 4.2 CSS 變數定義散落

- `variables.css` 87 行定義設計 token
- 但 inline style 仍有 `style="color:var(--accent)"` 等直接引用 — 應改為 class
- **修復**：refactor index.html 同時收斂

### 4.3 測試覆蓋空白

- 前端目前**無 JS 單元測試**（`shared_web/static/js/**` 無 `*_test.js`）
- 設計變更無回歸防護
- **建議**：建立 Vitest + jsdom 環境，至少覆蓋 `format-metric.js`、`glossaryTooltip()`

---

## 五、優先順序建議（給實作規劃用）

| P | 項目 | 預估工時 | 風險 | 解鎖 |
|---|------|---------|------|------|
| P0 | index.html 瘦身（< 150 行、0 inline） | 6-8h | 中（破壞現有 page 結構） | SEO、reduced-motion 全站、simplified mode |
| P0 | Simplified Mode Toggle + 全站 CSS hook | 4-6h | 中 | 教育層落地 |
| P1 | Glossary Tooltips 接線 + 元件 import | 2-3h | 低 | 教育層 50% → 100% |
| P1 | Touch targets 全站 44px | 2h | 低 | WCAG 2.5.5 |
| P1 | Mobile font sizes ≥18px 驗證與補完 | 1-2h | 低 | 行動可讀性 |
| P1 | Reduced-motion 全站（base/motion.css） | 1h | 極低 | WCAG 2.3.3 |
| P2 | Lighthouse SEO 補完（OG、canonical、JSON-LD） | 2-3h | 低 | 85 → 88 |
| P2 | Home API 聚合端點 | 4h | 中（需後端配合） | P95 -30% |
| P3 | 前端 Vitest 基礎 | 6h | 中 | 回歸防護 |

**總預估工時**：28-35 小時（3-4 個 work day）

---

## 六、建議 PR 切分策略

為避免單一 PR 過大，建議切成 4 個 PR：

| PR | 內容 | 大小 | 風險 |
|----|------|------|------|
| **PR-A** | index.html 瘦身 + 5 modal 重構 + page sections 動態注入 | 大（~600 LOC） | 中（需完整瀏覽器回歸） |
| **PR-B** | Simplified Mode + Glossary Tooltips 接線 + base/motion.css | 中（~250 LOC） | 低（純新增） |
| **PR-C** | Touch targets + mobile font sizes 補完 | 小（~50 LOC CSS） | 極低 |
| **PR-D** | Lighthouse SEO 補完（OG、canonical、JSON-LD） | 小（~30 LOC HTML） | 極低 |

每個 PR 各自獨立可 ship；不必等全部完成。

---

## 七、本次審計未涵蓋範圍

1. **效能實測**：未跑 Lighthouse CI、未截 375/768/1280 viewport 截圖
2. **a11y 細節**：未跑 axe-core / NVDA 螢幕閱讀器實測
3. **跨瀏覽器**：未測 Safari / Firefox / Edge
4. **真機測試**：未跑 iPhone/Android 實機
5. **後端 API**：未追蹤各端點 P95 / 錯誤率

建議在 PR-A 合併前補：
- `npx lighthouse http://localhost:8080/client/ --view`（行動 + 桌面）
- `npx playwright test --project=mobile`（375 / 768 / 1280 viewport 截圖）
- `npx axe http://localhost:8080/client/`（a11y 掃描）

---

## 八、結論

Phase 2 設計目標**僅 50% 落地**：
- ✅ 元件庫與 format-metric.js 完成
- ✅ 部分響應式（reduced-motion、touch targets）
- ❌ index.html 瘦身、simplified mode、glossary 接線、SEO 補完均未啟動

建議立即啟動 **PR-A（index.html 瘦身）**作為 Phase 2 收尾主軸，並以 PR-B / PR-C / PR-D 並行清理剩餘缺口。