# 前端架構

> 從 `README.md` 與 `CLAUDE.md` 合併。前端架構的單一權威來源。

## 目錄職責

| 目錄 | 角色 | 對外 URL |
|------|------|---------|
| `admin_web/static/js/` | 管理後台 JS | `/admin/` |
| `client_web/static/js/` | 投資人介面 JS | `/client/` |
| `shared_web/static/js/` | 共用 JS（pages、components、services） | esbuild plugin fallback |
| `shared_web/static/css/` | 全部 CSS（主題、components、layout、pages） | esbuild 打包 |

Root `/` 301 導向 `/client/`。每個 SPA 使用 History API 做 clean URL routing；Go backend 對 `/client/` 和 `/admin/` 下未匹配路徑回傳 `index.html`。

## CSS 架構

樣式模組化為 50+ 檔案，由各 app 的 esbuild 打包：

```text
shared_web/static/css/
├── main.css                # @import 聚合器（cascade-order）
├── base/                   # Design tokens and resets
│   ├── variables.css       # CSS custom properties（主題）
│   ├── reset.css           # Element resets
│   ├── tables.css          # 表格基礎樣式
│   └── typography.css      # 字型與文字工具
├── layout/                 # 結構性佈局
│   ├── animations.css, grid.css, header.css, page-shell.css
│   ├── responsive.css, sidebar.css, topbar.css
├── components/             # 可重用 UI 元件
│   ├── badge, chain, circuit-breaker, controls, empty-state
│   ├── error-banner, filter-panel, inbox-card, live-progress
│   ├── loading-bar, metric, misc, modal, notification
│   ├── notification-colors, panel, performance-report, pipeline
│   ├── refresh, refresh-pill, sse-status, table-pagination
│   ├── tabs, tool-events, utilities, view-controls, workflow
└── pages/                  # 頁面特定樣式
    ├── industry.css, overview.css, parameters.css
    └── ...

- 全部 CSS 變數定義於 `shared_web/static/css/base/variables.css`（canonical）
- 顏色一律用 `var(--...)`，不寫死 hex/rgba
- 金融語意 Token：`--pnl-profit` / `--pnl-loss`、`--trend-bullish` / `--trend-bearish`、`--metric-good` / `--metric-bad`、`--risk-high` / `--risk-low`、`--capital-inflow` / `--capital-outflow`、`--signal-bullish` / `--signal-bearish`

## JavaScript 模組

### 入口檔職責

| 檔案 | 職責 |
|------|------|
| `main.js` | 全域狀態、頁面切換(`switchPage`)、動態 import pages、執行各頁 init |
| `component-init.js` | 共用 component 初始化(circuit-breaker、sim-health、performance-report) |
| `event-listeners.js` | DOM event 綁定(sidebar nav、shock sim 互動、modal 關閉) |
| `pages/*.js` | 每個頁面的 render 函式,由 `main.js` 動態 import |
| `pages/stock-quote.js` + `services/stock-api-client.js` | v0.0.0.32 個股快查(Issue #1038 / PR #1045)：4 API 並發 + 報價/基本面/籌碼/技術 4 section |
| `page-shells/{login,register,premium,mcp,errors/404}.js` | v0.0.0.31 page shell（tier 認證 + MCP 頁 + 404 fallback）|
| `services/auth.js` | JWT + tier 解析 |
| `components/home-tier-sections.js` | tier-gated home dashboard 渲染 |

Canvas 繪圖色彩用 `getThemeColor()` + `hexToRgba()` 橋接。

### 路由 + 後端整合

- `cmd/atlas/api_routes.go`:`/admin/` 與 `/client/` 分別掛載 `admin_web.DistFS` 與 `client_web.DistFS`;root `/` 301 導向 `/client/`
- API 端點統一前綴 `/api/...`
- 靜態資源經 Go `embed.FS` 嵌入 binary,Docker image 由 multi-stage Dockerfile 重新 `npm run build` 產出 dist
Canvas 繪圖色彩用 `getThemeColor()` + `hexToRgba()` 橋接。

## esbuild plugin fallback

`esbuild-shared-plugin.mjs`（`shared_web/`）定義：
- `admin_web/` 找不到 → fallback 到 `shared_web/`
- `shared_web/` 找不到 → fallback 到 `admin_web/`

陷阱：若刪除 `shared_web/static/js/pages/xxx.js`，esbuild 靜默 fallback 失敗導致 UI 卡「載入中...」。CI 腳本 `scripts/ci/check_frontend_imports.sh` 會抓出。

## API 端點

統一路由前綴 `/api/...`。v0.0.0.31 新增端點：
- `/api/capital-flow/{daily,summary}` — 七維錢潮雷達（3+2+2 分層）Z-score + 共振：見 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04
- `/api/events/{calendar,prediction}` — 事件日曆 + 5 日預測
- `/api/recommendations` — tier-gated 推薦（需 JWT）
- `/api/reports/{latest,archive,subscribe}` — 每日報告
- `/api/auth/{register,login}` + `/api/user/profile` + `/api/user/subscription` — tier 認證

## 疑難排解

| 症狀 | 排查 |
|------|------|
| Panel 卡「載入中...」 | DevTools Network：確認 `/api/...` 無 timeout；若 200 則檢查 `main.js` 對應 pageId 分支 |
| 按鈕沒反應 | 檢查 `event-listeners.js` 對應 listener |
| h2 標題消失 | 檢查 `static/index.html` 對應 panel div 是否含 `<h2>` |
| 整頁空白 | `docker compose build atlas` 重新編譯 dist |

## 相關文件

- `docs/guides/frontend-architecture.md` — 本文件（前端架構單一權威來源）
- `CLAUDE.md` §「快速路由」— Claude Code 前端修改時的入口指向
