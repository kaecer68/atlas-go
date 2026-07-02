# Atlas Design System

> **單一權威來源**：所有 UI 設計決策以此文件為準。`shared_web/static/css/base/variables.css` 是 token 的可執行規範；本文檔是設計意圖的規範化記錄。

**狀態**：Active（PR #864 / #871 / #874 完成後 + PR-1 補強中）
**對應版本**：v0.0.0.27+
**對應分支**：`feature/retail-landing-pr1-p1-touches`

---

## 1. 設計哲學

### 1.1 雙軌定位

Atlas 同時服務兩個受眾：

| 受眾 | 介面 | 風格關鍵詞 |
|------|------|------------|
| **零售投資人**（`/client/*`） | 編輯式（Editorial） | 紙張感、暖色 accent、serif 顯示字、易讀 |
| **研究 / 管理員**（`/admin/*`） | 儀表板式（Dashboard） | 資訊密度、cyan accent、等寬數字、可掃描 |

**現實**：兩者在程式碼上共用 `shared_web/`，因此 CSS 必須同時服務兩個情境。設計 token 採**雙主題**（`:root` + `:root[data-theme="light"]`），各主題下兩種風格透過 component-level class 切換。

### 1.2 核心原則

1. **同源不分裂**：顏色、字型、間距、陰影、圓角皆由 CSS 變數驅動；禁止 hard-coded hex/rgba。
2. **台股在地化**：市場方向紅漲綠跌；系統狀態綠 OK 紅 ERR。詳見 `variables.css` 開頭註解。
3. **無障礙優先**：所有可互動元素 ≥44px 觸控目標；尊重 `prefers-reduced-motion`；WCAG AA 對比度。
4. **可逆增量**：PR-1 只補缺口（Glossary 接線、motion.css、44px、mobile font），不重寫既有頁面。

---

## 2. Token 系統

### 2.1 色彩語意（canonical 來源：`base/variables.css`）

| Token | 用途 | 預設（深） | 淺色 |
|-------|------|-----------|------|
| `--bg / --panel / --border` | 結構底色 | `#0b0d11 / #13161c / #242a33` | `#f5f5f5 / #ffffff / #e0e0e0` |
| `--text / --muted` | 內文、次要內文 | `#f0f4f8 / #b8c4d0` | `#1a1a1a / #666666` |
| `--accent / --accent-rgb` | 主要 accent | `#4fc1ff / 79,193,255` | `#0066cc / 0,102,204` |
| `--bullish / --bearish` | 市場方向（紅漲綠跌） | `#ef4444 / #10b981` | `#dc2626 / #16a34a` |
| `--color-success / --danger / --warning / --info` | 系統狀態 | 國際通用綠 OK / 紅 ERR / 橘 WARN / 藍 INFO | 同左 |
| `--editorial-ink / --editorial-paper` | 編輯式基底 | `#0F172A / #F5F3EF` | 同左（共用） |
| `--editorial-amber / --editorial-amber-soft` | 編輯式 accent | `#D97706 / #FEF3C7` | 同左 |

### 2.2 字型

| Token | 字型 stack |
|-------|-----------|
| `--font-display` | `'Noto Sans TC', 'Source Han Sans TC', system-ui, ...` |
| `--editorial-serif` | `'Noto Serif TC', 'Source Han Serif TC', 'Songti SC', serif` |
| `--font-body` | `'Inter', system-ui, ...` |
| `--font-mono` | `'JetBrains Mono', 'Fira Code', 'Consolas', monospace` |

### 2.3 間距 / 圓角

| Token | 值 |
|-------|-----|
| `--space-xs / sm / md / lg / xl` | `4 / 8 / 14 / 20 / 32 px` |
| `--editorial-radius / -sm` | `12 / 8 px` |
| `--editorial-shadow` | `0 1px 3px rgba(15,23,42,0.08), 0 4px 12px rgba(15,23,42,0.05)` |

---

## 3. 元件庫（`shared_web/static/css/components/`）

| 元件 | 用途 | 主題感知 |
|------|------|---------|
| `panel.css` | 通用容器 | ✓ |
| `metric.css` | 指標卡 | ✓ |
| `badge.css` | 標籤 / 風險標 | ✓ |
| `modal.css` | 模態框 | ✓ |
| `tabs.css` | 分頁 | ✓ |
| `controls.css` | 按鈕 / 表單 | ✓ |
| `view-controls.css` | 視圖切換控制 | ✓ |
| `empty-state.css` | 空狀態 | ✓ |
| `error-banner.css` | 錯誤橫幅 | ✓ |
| `loading-bar.css` | 載入進度 | ✓ |
| `pipeline.css` | 決策鏈 | ✓ |
| `workflow.css` | workflow 視覺化 | ✓ |
| `inbox-card.css` | inbox 卡片 | ✓ |
| `circuit-breaker.css` | circuit breaker | ✓ |
| `notification*.css` | 通知 / 顏色 | ✓ |
| `refresh*.css` | refresh pill | ✓ |
| `risk-gate-panel.css` | 風險閘面板 | ✓ |

### 3.1 零售頁面專用元件

由 PR #864 抽出，定義於 `components/` 下，並以 `metric-card.js / action-card.js / risk-badge.js / trust-footer.js / tooltip.js` 提供工廠函式：

| JS 工廠 | 對應 CSS class |
|---------|---------------|
| `metricCard({label, value, delta, tone, tooltip})` | `.metric-card` |
| `renderRiskBadge(level, label)` | `.risk-badge` |
| `trustFooter({version, sources, disclaimer})` | `.trust-footer` |
| `renderTooltip(term, explanation)` | `.tooltip` |
| `glossaryTooltip(term)` | 從 `TOOLTIP_GLOSSARY` 查詢 |

---

## 4. Motion 規範

### 4.1 標準時長

| 互動 | 時長 | easing |
|------|------|--------|
| hover lift / shadow | `200ms` | `ease` |
| button press | `150ms` | `ease` |
| tooltip fade | `200ms` | `ease` |
| confidence bar fill | `800ms` | `ease` |

### 4.2 `prefers-reduced-motion: reduce`

**所有帶 transition/animation 的元件必須 disable 動畫**。集中規範於 `base/motion.css`：

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

PR-1 已建立此檔案並在 `main.css` 引入。後續新增元件不需各自宣告 `prefers-reduced-motion` 規則。

---

## 5. 無障礙規範（A11y）

### 5.1 觸控目標

所有可點擊元素 ≥ **44×44 px**（WCAG 2.5.5 AAA；mobile 強制）。實作要點：

- 設定 `min-height: 44px` on button / nav a / clickable card
- 視覺尺寸可能 < 44px 但 padding 補到 44px
- 例：`#sidebar nav a` padding 從 `9px 12px` → 維持視覺但加 `min-height: 44px`

### 5.2 行動裝置字型

- mobile (< 768px) body font ≥ **18px** 確保不需縮放即可閱讀
- mobile heading scale 為 desktop 的 1.125×（line-height 1.4+）
- 規範於 `base/typography.css`，搭配 `@media (max-width: 768px)`

### 5.3 對比度

- 文字 / 背景對比度 ≥ **4.5:1**（WCAG AA Normal Text）
- 大字（≥18px 或 14px bold）對比度 ≥ **3:1**
- 規範由 token 保證；若需新增顏色必須驗證後再寫入

### 5.4 鍵盤可達

- 所有互動元素 `tab` 順序合理（不可跳過關鍵 CTA）
- focus ring 使用 `:focus-visible` + 2px outline，不依賴 hover 視覺

---

## 6. Glossary Tooltips

`components/tooltip.js` 維護 `TOOLTIP_GLOSSARY`：

```js
{
  'Sharpe': '衡量每承擔一單位風險能獲得的超額報酬…',
  'Hit Rate': '策略建議正確的比例…',
  '最大回撤': '投資組合從高點到低點的最大跌幅…',
  'HHI': '赫芬達爾指數，衡量持倉集中度…',
  'Regime': '市場狀態（如多頭、空頭、震盪）…',
}
```

### 6.1 使用方式

```js
import { glossaryTooltip, renderTooltip } from '../components/tooltip.js';

// 查詢 glossary
const html = glossaryTooltip('Sharpe');  // 回傳 tooltip span，無對應則 escape 回原文

// 內聯 term（不入 glossary）
const html = renderTooltip('信心分數', '模型對今日建議的把握程度...');
```

### 6.2 接線原則

- 零售頁面（home / portfolio / dashboard）對**專業術語首次出現**加 tooltip
- tooltip 文字 ≤ 80 中文字；超過用連結展開
- PR-1 已將 home.js 對 `renderTooltip` import 接線

---

## 7. 主題切換（Light / Dark）

### 7.1 觸發

`#sidebar .theme-toggle` 點擊後切換 `document.documentElement.dataset.theme`。

### 7.2 Token 覆寫矩陣

| Token class | `:root`（深） | `:root[data-theme="light"]`（淺） |
|------------|---------------|-----------------------------------|
| `--bg` | `#0b0d11` | `#f5f5f5` |
| `--panel` | `#13161c` | `#ffffff` |
| `--accent` | `#4fc1ff` | `#0066cc` |
| `--bullish / --bearish` | `#ef4444 / #10b981` | `#dc2626 / #16a34a` |
| `--editorial-*` | 固定值 | 固定值（不變） |

編輯式元件（hero / metric-card / recommendation）在兩主題下視覺一致；儀表板元件（panel / badge / grid）在兩主題下重新對比。

---

## 8. 驗收規範

每個 UI PR 必須通過：

1. **Playwright 截圖**：desktop 1440px / tablet 768px / mobile 375px × 兩主題 = 6 張/頁
2. **Lighthouse**：a11y ≥ 95、best-practices ≥ 95、SEO ≥ 95
3. **axe-core**：0 violations
4. **手機視覺驗證**：font-size ≥ 18px、touch target ≥ 44px
5. **reduced-motion**：DevTools 啟用後所有動畫停止

---

## 9. 變更紀錄

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-02 | 初版（PR-1 完成後） |

---

## 10. 參考

- Token 來源：`shared_web/static/css/base/variables.css`
- 設計 audit：`audit_assets/phase2-retail-investor-landing-audit.md`
- 計畫原案：`.omo/plans/retail-investor-landing-design.md`
- 架構：`docs/architecture.md`