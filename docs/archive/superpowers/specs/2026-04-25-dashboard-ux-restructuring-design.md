# Atlas Dashboard UX 重組設計書

> 日期：2026-04-25
> 範圍：選項 2 — 前端重組 + 充分利用現有 API 資料
> 原則：不新增後端 API，純前端調整 + 利用已有但未使用的資料

---

## 一、問題摘要

| # | 問題 | 影響 | 嚴重度 |
|---|------|------|--------|
| P1 | 頁面順序不符合投資研究思維流 | 使用者需在不同頁面間跳躍 | 高 |
| P2 | 「相對趨勢」命名誤導 | 名稱暗示趨勢分析，實際是控制層結果 | 高 |
| P3 | 總覽 KPI 平鋪無分組 | 缺乏層次感，無法快速定位 | 高 |
| P4 | 宏觀敘事 8 panel 平鋪 | 重要資訊（快照、壓力指數）不明顯 | 中 |
| P5 | 系統管理三頁功能重疊 | 資訊碎片化 | 中 |
| P6 | 大量 API 資料未被前端使用 | 資料浪費 | 中 |
| P7 | 投資管線幫助文字過長 | 佔用內容空間 | 低 |

---

## 二、改動項目

### 改動 1：側邊欄頁面順序重排（P1）

**現行順序**：
總覽 → 宏觀敘事 → 相對趨勢 → 投資管線 → 決策鏈 → AI觀測台 → 模擬交易 → 最新回測 → 控制與稽核 → 信息通道 → 系統警報 → 指標監控 → 產業生態系

**新順序**（投資研究思維流 + 系統管理分區）：
1. 📊 **總覽** — 系統狀態快照
2. 🌍 **宏觀敘事** — 大環境判斷
3. 🏭 **產業生態系** — 產業輪動
4. 📋 **投資管線** — 個股推薦
5. 🔗 **決策鏈** — 推薦理由稽核
6. 🛡️ **風控結果** — 控制層處置結果（原「相對趨勢」）
7. 🤖 **AI 觀測台** — AI 績效回顧
8. 🧪 **模擬交易** — 實驗管理
9. 📈 **最新回測** — 回測報告
10. ⚙️ **控制與稽核** — 人工干預
11. ── *系統管理分隔線* ──
12. 📡 **系統狀態** — 合併：信息通道 + 指標監控
13. 🚨 **系統警報** — 保留獨立頁（警報需單獨關注）

**實作方式**：
- 修改 HTML 中 `<nav>` 的 `<a data-page="...">` 順序
- 修改 `switchPage()` 中的 `titles` 物件鍵值
- 新增側邊欄分隔線（用 `<hr>` 或帶 class 的 `<div>`）

**使用者體驗改變**：
- 研究者按思維流自然瀏覽：大環境 → 產業 → 推薦 → 稽核 → 結果
- 系統管理功能歸為底部群組，不干擾研究流程

---

### 改動 2：「相對趨勢」重新命名（P2）

- 頁面標題：相對趨勢 → **風控結果**
- 側邊欄：`data-page="live"` title 保持 `相對趨勢` → 改為 **風控結果**
- `switchPage titles` 物件：`live: '相對趨勢'` → `live: '風控結果'`
- 頁面標題 id `pageTitle`：隨 titles 物件一起改

---

### 改動 3：總覽 KPI 分組（P3）

**現行**：7 張卡片平鋪成一排

**改為**：三組卡片，每組有標題

```html
<div class="kpi-group-title">🌍 市場環境</div>
<div class="kpi-grid">  <!-- 敘事脈絡 + 市場狀態 -->  </div>

<div class="kpi-group-title">⚠️ 風險信號</div>
<div class="kpi-grid">  <!-- 最差AI + 擁擠標的 + 信息通道預警 -->  </div>

<div class="kpi-group-title">🔧 系統狀態</div>
<div class="kpi-grid">  <!-- 資料時間 + 基線版本 + 實驗狀態 -->  </div>
```

**CSS 新增**：
```css
.kpi-group-title {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 1px;
  margin: var(--space-lg) 0 var(--space-sm);
  padding-bottom: var(--space-xs);
  border-bottom: 1px solid var(--border);
}
.kpi-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: var(--space-sm); }
```

**KPI 重新排列**：
| 群組 | 卡片 | 理由 |
|------|------|------|
| 市場環境 | 敘事脈絡 + 市場狀態 | 「現在什麼局？」 |
| 風險信號 | 最差 AI + 擁擠標的 + 信息通道預警 | 「什麼要注意？」 |
| 系統狀態 | 資料時間 + 基線版本 + 實驗狀態 | 「資料新鮮嗎？」 |

---

### 改動 4：宏觀敘事頁面優先級調整（P4）

**現行**：8 個 panel 無差別平鋪

**改為**：上 方強調區 + 下 方細節區

- **上 方**（全寬，最重要）：
  - 總經快照 + 外資出逃指數（並排）
- **下 方**（2x3 網格，次要）：
  - 敘事事件 + 因果傳導鏈 + 投資模型
  - 因果模板庫 + 散戶情緒 + 季節性分析

**實作**：
```html
<div class="narrative-hero">
  <div class="panel wide">總經快照</div>
  <div class="panel wide">外資出逃指數</div>
</div>
<div class="two-col-grid">
  <div class="panel">敘事事件</div>
  <div class="panel">因果傳導鏈</div>
  <div class="panel">投資模型</div>
  <div class="panel">因果模板庫</div>
  <div class="panel">散戶情緒</div>
  <div class="panel">季節性分析</div>
</div>
```

---

### 改動 5：系統管理頁面合併（P5）

**合併方式**：信息通道 + 指標監控 → 新頁面「系統狀態」

- KPI 區（篩選率、警報觸發、資金階段）合併到上方
- 下方分兩個 tab 或折疊面板：資料通道 | 指標趨勢

系統警報保持獨立（因警報需要單獨處理流程：確認、篩選、刷新）

**實作**：
- 在側邊欄新增 `data-page="system-status"`
- 新頁面 HTML：KPI grid + 分頁切換
- 移除舊的 `data-page="datachannels"` 和 `data-page="metrics"`
- 或者保留原頁面，在側邊欄用分隔線歸類（更安全，改動較小）

**初始策略**：保留原頁面，在側邊欄用分隔線歸類。合併頁面留待選項 3。

---

### 改動 6：利用未被前端使用的 API 資料（P6）

以下資料已由 API 回傳但前端未充分展示：

| API 回傳欄位 | 目前使用情況 | 可如何呈現 |
|---|---|---|
| `PipelineItem.factor_scores` | 決策鏈頁有使用，管線頁未使用 | 投資管線頁面加入因子分數摘要 |
| `PipelineItem.conviction_breakdown` | 決策鏈頁有使用 | 投資管線的每筆推薦加入信念度展開 |
| `GuardOutcome` 清單 | 決策鏈頁有使用 | 風控結果頁面加入守衛結果明細 |
| `/api/dashboard/risk` VaR/CVaR/MaxDD | 指標監控頁未使用 | 總覽或風控結果頁加入風險指標卡片 |
| `/api/dashboard/capital-phase` | 總覽有使用但只用 phase 名稱 | 顯示完整資金階段配置（最小天數、最大回撤限制） |
| `/api/dashboard/tax-snapshot` | 決策鏈頁有使用 | 投資管線頁面加入稅務快照摘要 |
| `UniverseOverlapResponse.screening_criteria` | AI 觀測台未使用 | AI 觀測台加入篩選條件明細 |
| `NarrativeEvent.confidence_source` + `hit_rate` | 敘事頁有使用 | 加強信心度展示（來源 + 歷史命中率） |

**實作方式**：
1. 投資管線 — 在每筆推薦的展開區塊中加入 `factor_scores` 摘要條（3 因子小進度條）
2. 風控結果 — 利用 `/api/dashboard/risk` 加入 VaR/CVaR/MaxDD 卡片
3. 總覽 — 在風險信號群組加入 VaR 95 卡片
4. AI 觀測台 — 在每個 agent 卡片下方加入篩選條件摘要

---

### 改動 7：投資管線幫助文字收合（P7）

**現行**：幫助文字永遠展開，佔用大量空間

**改為**：預設收合，只顯示一行摘要。點擊展開詳細說明。

```html
<details class="help-details">
  <summary><strong>📖 如何解讀本頁</strong> 以下為最新回測場次中，控制層已放行的推薦標的。</summary>
  <p>每筆資料包含策略來源、方向、收盤價與隔日回測報酬。價量標籤與推薦理由供您快速評估是否放行。
  若勾選「顯示全部被過濾項目」，<span class="text-down">紅色邊框列</span>為被控制層擋下的標的，您可點擊「補追」進行人工覆寫。
  <br><span class="text-muted">註：收盤價為回測當日收盤，作為模擬進場的參考基準。目標價與停損價由 Agent 推薦產生。</span></p>
</details>
```

```css
.help-details { margin-bottom: var(--space-md); }
.help-details summary { cursor: pointer; font-size: var(--text-sm); color: var(--muted); }
.help-details[open] summary { margin-bottom: var(--space-sm); }
.help-panel { display: none; }  /* 移除預設展開的 .help-panel 樣式 */
.help-details[open] + .help-panel { display: block; }
```

所有頁面的 `.help-panel` 都改為 `<details>` 收合。

---

## 三、不在此範圍的項目（留待選項 3）

以下項目需要新增後端 API 或重大後端改動，留待選項 3 盤點：

1. **組合持倉總覽** — 需要一個新 API `/api/dashboard/portfolio-summary` 回傳目前持倉、成本、浮動損益
2. **損益歸因分析** — 需要按 AI agent 分解損益貢獻
3. **風險曝露總覽** — 需要產業集中度、因子曝露、相關性矩陣
4. **資訊通道 + 指標監控合併為系統狀態頁** — 需要前端頁面合併（目前保留分頁 + 側邊欄分隔線）
5. **模擬交易 Workflow 進度指示** — 需要後端提供實驗當前階段狀態

---

## 四、選項 3 盤點：缺失的後端 API 與新視角

以下盤點基於三路深度探索（後端 domain 型別、API handler 回傳結構、前端 JS rendering 函式）的交叉驗證結果。

### A. 已有 API 但前端完全未使用的資料（最高優先）

| API 端點 | 未使用欄位 | 投資研究價值 | 改動量 |
|----------|-----------|-------------|--------|
| `/api/dashboard/risk` | **全部** (VaR95, VaR99, CVaR95, MaxDrawdownPct, data_points) | 🔴 極高 — 風險指標是核心需求 | 低 — 加一個 fetch + KPI 卡片 |
| `/api/dashboard/agent-observatory` | `BrokerRuntime` (15+ 欄位：Mode, Adapter, Signer, KeyID, MaxRetries 等) | 🟡 中 — 除錯/稽核價值 | 低 — 在控制與稽核頁加入摺疊面板 |
| `/api/dashboard/live-status` | `circuit_breaker.state_changed_at`, `consecutive_sl`, `cooldown_until`, `intraday_peak`, `day_start_value` | 🟡 中 — 熔斷狀態細節 | 低 — 風控結果頁展開 |
| `/api/dashboard/tax-snapshot` | `snapshots[].DividendTaxRate`, `TransactionTaxRate`, `DividendTax`, `TransactionTax` | 🟢 低 — 稅務明細 | 低 — 展開每檔標的稅務 |
| `/api/dashboard/recommendation-pipeline` | `PipelineItem.RecordedAt`, `Hit` | 🟢 低 — 排序/標記 | 低 |
| `/api/dashboard/macro-radar` | `BrokerRuntime` | 🟡 中 — 稽核價值 | 低 |
| `/api/dashboard/capital-phase` | `MinDaysPerPhase`, `MaxDrawdownLimit`, `SharpeThreshold`, `CapitalLimits` | 🟡 中 — 進階條件 | 低 — 總覽展開 |

### B. 後端已有型別但未暴露 API 的資料（需新增 API）

| 資料來源 | 型別 | 關鍵欄位 | 投資研究價值 | 改動量 |
|----------|------|---------|-------------|--------|
| `internal/sim/` | `SimulationState` | Cash, Positions[], EquityCurve[], DailyReturns[], CurrentDrawdown | 🔴 極高 — 組合持倉總覽的核心 | 中 — 新增 `/api/dashboard/portfolio-state` |
| `internal/domain/` | `Position` | Symbol, Quantity, AverageCost, CurrentPrice, MarketValue, UnrealizedPnL | 🔴 極高 — 持倉損益 | 中 — 同上 API |
| `internal/portfolio/` | `RiskMetrics` | CurrentDrawdown, MaxDrawdown, TotalExposure, ExposureRatio, ActivePositions, ActiveAlerts | 🔴 極高 — 即時風險曝露 | 中 — 新增 `/api/dashboard/risk-realtime` |
| `internal/portfolio/` | `SectorRotationPlan` | Allocations[].Sector/TargetPct/CurrentPct/Delta, PrimaryFlow, Rationale | 🟡 中 — 產業輪動策略 | 高 |
| `internal/risk/` | `MacroAwareDrawdownDecision` | Action, Percentage, MaxExposure, Rationale, StructuralOverride | 🟡 中 — 風控決策透明化 | 高 |
| `internal/risk/` | `GetSectorConstraints()` | map[sector]exposure_multiplier (硬編碼：ai_supply_chain=0.3, gold=1.5 等) | 🟡 中 — 限制說明 | 中 |
| `internal/sim/` | `SimulationReport` | TotalReturn, SharpeRatio, MaxDrawdown, EquityCurve[], AgentHitRates, TradeCount | 🟡 中 — 回測摘要 | 中 — 整合到 `/api/report/latest` |
| `internal/narrative/` | `NarrativeEvent.SourceData` | map[string]float64 (觸發事件的原始數據如 us10y_change_bps) | 🟢 低 — 深度稽核 | 低 |
| `internal/narrative/` | `NarrativeEvent.ConfidenceSource` | string (信心度來源如 "heuristic_fixed_v1") | 🟢 低 — 透明度 | 低 |

### C. 核心缺失視角（需新增 API + 前端頁面）

| 缺失視角 | 描述 | 所需 API | 優先級 |
|----------|------|---------|--------|
| **1. 組合持倉總覽** | 顯示目前持倉、成本、市值、浮動損益 | `GET /api/dashboard/portfolio-state` → Position[] + UnrealizedPnL | 🔴 P0 |
| **2. 損益歸因分析** | 按 AI agent、產業、因子分解損益貢獻 | `GET /api/dashboard/pnl-attribution` → per-agent/per-sector/per-factor P&L | 🟡 P1 |
| **3. 風險曝露總覽** | 產業集中度、因子曝露、相關性風險矩陣 | `GET /api/dashboard/risk-exposure` → sector concentration + factor exposure | 🟡 P1 |
| **4. 淨值曲線圖** | 歷史權益淨值變化趨勢線圖 | `GET /api/dashboard/equity-curve` → date/value pairs | 🟡 P1 |
| **5. 因子效率回顧** | 因子歷史命中率、回歸係數趨勢 | `GET /api/dashboard/factor-performance` → per-factor hit rate over time | 🟢 P2 |

### D. 選項 3 階段性建議

**Phase 3A（低懸果果實，1-2 天）**：
1. 在風控結果頁加入 `/api/dashboard/risk` 的 VaR/CVaR/MaxDD 卡片（API 已存在，只需前端渲染）
2. 在 AI 觀測台加入 `UniverseOverlapResponse.screening_criteria` 的篩選條件顯示（API 已回傳）

**Phase 3B（中期，3-5 天）**：
3. 新增 `/api/dashboard/portfolio-state` API，暴露 Position 資料
4. 新增「組合持倉」頁面，顯示持倉清單 + 損益
5. 新增 `/api/dashboard/equity-curve` API，暴露歷史淨值
6. 在總覽頁加入淨值曲線迷你圖

**Phase 3C（長期，1-2 週）**：
7. 新增損益歸因分析 API + 頁面
8. 新增風險曝露總覽 API + 頁面
9. 整合產業輪動策略視覺化

---

## 五、實作順序

| 步驟 | 改動 | 檔案 | 風險 |
|------|------|------|------|
| 1 | 側邊欄順序重排 + 分隔線 | `index.html` nav 區 | 低 |
| 2 | 「相對趨勢」重新命名 | `index.html` nav + titles | 低 |
| 3 | 總覽 KPI 分組 + 重新排列 | `index.html` overview 頁 | 中 |
| 4 | 宏觀敘事頁面優先級調整 | `index.html` narrative 頁 | 中 |
| 5 | 幫助文字收合 | `index.html` 所有 `.help-panel` | 低 |
| 6 | 風控結果頁加入 VaR 卡片 | `index.html` + JS render 函式 | 中 |
| 7 | 投資管線加入因子分數 | `index.html` renderPipeline 函式 | 中 |
| 8 | 側邊欄分隔線樣式 | `index.html` CSS | 低 |

---

## 六、驗證標準

- [ ] 側邊欄順序符合投資研究思維流
- [ ] 「風控結果」名稱取代「相對趨趨勢」
- [ ] 總覽頁 KPI 分為三群組，各群組有標題
- [ ] 宏觀敘事頁面總經快照與壓力指數在上方
- [ ] 幫助文字預設收合
- [ ] 所有頁面功能正常（API 資料正確載入）
- [ ] `go build ./cmd/atlas/...` 通過
- [ ] 瀏覽器重新整理後可見所有改動