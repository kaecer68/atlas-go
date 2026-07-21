# CLAUDE.md — atlas-go 規則索引

@AGENTS.md

> **Wave 11 L2.3 PoC + L2.4 觀察窗口** — v0.0.0.18..v0.0.0.21。L2.3 設計見 [`docs/specs/llm-sector-agent.md`](docs/specs/llm-sector-agent.md)、[`docs/specs/agent-loop-state-machine.md`](docs/specs/agent-loop-state-machine.md)、[`docs/guides/adding-sector-agents.md`](docs/guides/adding-sector-agents.md)。L2.4 已 ship(PR #821 merged 2026-06-29),文件永久化於 [`docs/operations/l2-4-runbook.md`](docs/operations/l2-4-runbook.md) + [`docs/specs/l2-4-observation-spec.md`](docs/specs/l2-4-observation-spec.md) + [`docs/operations/l2-4-followup.md`](docs/operations/l2-4-followup.md)(後續工作報告)。`UseLLMSectorAgents` flag 預設 off。

## 🌐 語言強制規範

見 [`AGENTS.md`](AGENTS.md)（跨工具權威來源）。全局規則仍遵循 `~/.claude/CLAUDE.md`。

本檔案為 Claude Code 專屬設定入口。所有跨工具共用規則、陷阱、文件路由見 **[`AGENTS.md`](AGENTS.md)**。

## 快速路由（Claude Code 專屬）

跨工具共用路由見 [`AGENTS.md`](AGENTS.md)。以下僅列本檔案專有內容：

| 需求 | 參考位置 |
|------|---------|
| 前端架構（admin_web / client_web / shared_web） | `## 前端架構`（下方） |
| 部署設定（本機 Docker） | `## 部署設定`（下方） |
| Token 效率規則（Claude Code 專屬） | `## Token Efficiency Rules`（下方） |

## 前端架構

> 2026-06 完成拆分重構:`./web/`(legacy 單體) → `./admin_web/`(管理後台)+ `./client_web/`(投資人介面)+ `./shared_web/`(共用資源)。
> 2026-06-29 後端移除 `web.DistFS` 依賴,root `/` 導向 `/client/`;`./web/` 仍保留在 repo 但已不再 build、不再 serve,待下一輪評估是否整目錄刪除。

### 目錄職責

| 目錄 | 角色 | 對外 URL |
|------|------|---------|
| `admin_web/static/js/` | 管理後台專屬 JS(`main.js`、`component-init.js`、`event-listeners.js`) | `/admin/` |
| `client_web/static/js/` | 投資人介面專屬 JS（`main.js`、`page-shells/`、`components/`） | `/client/` |
| `shared_web/static/js/` | 共用 JS(pages、components、services、shared、bootstrap-utils) | 經 esbuild plugin fallback 引入 |
| `shared_web/static/css/` | 全部 CSS(dark/light 主題、components、layout、pages) | 經 esbuild 打包成 `css/main.css` |

### 入口檔職責

| 檔案 | 職責 |
|------|------|
| `main.js` | 全域狀態、頁面切換(`switchPage`)、動態 import pages、執行各頁 init |
| `component-init.js` | 共用 component 初始化(circuit-breaker、sim-health、performance-report) |
| `event-listeners.js` | DOM event 綁定(sidebar nav、evView 按鈕、shock sim 互動、modal 關閉) |
| `pages/*.js` | 每個頁面的 render 函式,由 `main.js` 動態 import |
| `pages/stock-quote.js` + `services/stock-api-client.js` | v0.0.0.32 個股快查(Issue #1038 / PR #1045)：4 API 並發 + 報價/基本面/籌碼/技術 4 section;Contract 見 [`docs/specs/stock-api-contract-spec.md`](docs/specs/stock-api-contract-spec.md) |
| `page-shells/{login,register,premium,mcp,errors/404}.js` | v0.0.0.31 Phase A0/C 新增的 page shell（tier 認證 + MCP 頁 + 404 fallback）|
| `services/auth.js` | v0.0.0.31 Phase A0 新增：JWT + tier 解析、`initAuth`/`isLoggedIn`/`getTier`/`renderNavState` |
| `components/home-tier-sections.js` | v0.0.0.31 Phase B 新增：tier-gated home dashboard 渲染（capital flow + event prediction + event calendar + recommendations + daily report）|

### esbuild plugin fallback 規則

`esbuild-shared-plugin.mjs`(`shared_web/`)定義:
- 從 `admin_web/static/` 找不到的 import → fallback 到 `shared_web/static/`
- 從 `shared_web/static/` 找不到的 import → fallback 到 `admin_web/static/`(例:`main.js` 中的全域函式)

**陷阱**:若有人刪掉 `shared_web/static/js/pages/xxx.js`,esbuild 會**靜默 fallback 失敗**,UI 卡「載入中...」。CI 驗證腳本 `scripts/ci/check_frontend_imports.sh` 會抓出。

### 前端 CSS 與 JS 規範

- 全部 CSS 變數定義於 `shared_web/static/css/base/variables.css`(canonical)
- Canvas 繪圖色彩用 `getThemeColor()` + `hexToRgba()` 橋接(`shared_web/static/js/shared/utils.js`)
- **JS 端統一色彩邏輯**：`shared_web/static/js/shared/color-tokens.js` 提供 `financialColor()` / `regimeColor()` / `severityColor()` / `confidenceColor()`，為所有頁面色彩判斷的單一權威來源（PR #944 引入）
- 金融語意 Token:`--pnl-profit`/`--pnl-loss`、`--trend-bullish`/`--trend-bearish`、`--metric-good`/`--metric-bad`、`--risk-high`/`--risk-low`、`--capital-inflow`/`--capital-outflow`、`--signal-bullish`/`--signal-bearish`
- 顏色一律用 `var(--...)`,不寫死 hex/rgba

### 路由 + 後端整合

- `cmd/atlas/api_routes.go`:`/admin/` 與 `/client/` 分別掛載 `admin_web.DistFS` 與 `client_web.DistFS`;root `/` 301 導向 `/client/`
- API 端點統一前綴 `/api/...`(dashboard、narrative、industry、taiwan、...)
- v0.0.0.31 新增端點（Phase B 對應）：
  - `/api/capital-flow/{daily,summary}` — 七大資金勢力 + 共振強度（`internal/capitalflow`）
  - `/api/events/{calendar,prediction}` — 事件日曆 + 5 日預測（`internal/eventdriven`）
  - `/api/recommendations` — tier-gated 推薦（`internal/recommender`，需 JWT）
  - `/api/reports/{latest,archive,subscribe}` — 每日報告（`internal/dailyreport`）
  - `/api/auth/{register,login}` + `/api/user/profile` + `/api/user/subscription` — tier 認證（`internal/subscription`）
- 靜態資源經 Go `embed.FS` 嵌入 binary,Docker image 由 multi-stage Dockerfile 重新 `npm run build` 產出 dist

### 前端疑難排解

| 症狀 | 排查 |
|------|------|
| Panel 卡「載入中...」 | 檢查 DevTools Network,確認 `/api/...` 是否 timeout;若 200,檢查 `main.js` 對應 pageId 分支是否有 `loadXxx()` 呼叫 |
| 按鈕沒反應 | 檢查 `event-listeners.js` 是否有對應 listener |
| h2 標題消失 | 檢查 `static/index.html` 對應 panel div 是否包含 `<h2>` |
| 整頁空白 | 確認 `dist/index.html` 是最新 build,`docker compose build atlas` 重新編譯 |

## 部署設定

### 平台
**本機 Docker**（`docker compose` 單機部署，非 production server）。

### 映像來源
- 註冊表：`ghcr.io/kaecer68/atlas-go`（`ci-cd.yml` main/develop 自動建置推送）
- Dockerfile：multi-stage（Node.js 前端 + Go 1.26 後端），expose port 18080
- compose 設定：`docker-compose.yml`（healthcheck、env vars、postgres）

### 環境變數（統一由 `~/.config/atlas-go/.env` 載入）
`config.Load()` 自動讀取以下路徑（`internal/config/config.go:70-71`）：
1. `loadEnvFile(resolveEnvFilePath())` — 專案根 `.env`
2. `loadUserEnvFile()` — `~/.config/atlas-go/.env`（使用者統一管理入口）

| 變數 | 用途 | 備註 |
|------|------|------|
| `LLM_DEEPSEEK_API_KEY` | DeepSeek V4-Pro / V4-Flash | 從 https://platform.deepseek.com 取得 |
| `LLM_MINIMAX_API_KEY` | MiniMax M3（coding plan） | `sk-cp-` 前綴的 minimax-cn-coding-plan key；DataClass≥Regulated 時被 router 閘門 skip |
| `LLM_ANNOTATOR_API_KEY` | **向後相容** — 早期 `KimiClient` 讀此變數 | 實際值等同 `LLM_MINIMAX_API_KEY`（Kimi K2.7 因 coding plan key 限制已移除） |
| `LLM_RATIONALE_TRANSLATION_ENABLED` | 啟用 `CapabilityRationaleGeneration` hook | default `false` |
| `LLM_PRISM_SCENARIO_ENABLED` | 啟用 `CapabilityScenarioSimulation` hook | default `false` |
| `LLM_NARRATIVE_EXPLAIN_ENABLED` | 啟用 `CapabilityRegimeExplanation` + `CapabilitySentimentExplanation` | default `false` |
| `LLM_RISK_FORENSICS_ENABLED` | 啟用 `CapabilityPerformanceForensics` | default `false` |
| `LLM_SECTOR_AGENTS_ENABLED` | 啟用 `SectorAgentLLM` Plan→ToolCall→Reflect loop（Issue #719 wired） | default `false` |

### 部署流程

```bash
# 1. 確認 main HEAD 已是目標版本
git fetch origin main && git log --oneline origin/main -1

# 2. 拉最新映像（CI 已建置並推送 ghcr.io）
docker compose pull

# 3. 重啟服務
docker compose up -d

# 4. 確認容器狀態
docker compose ps
```

### 部署驗證（Health Check）

兩個 endpoint 都必須通過：

```bash
# Liveness（基礎健康）
curl -fsS http://localhost:18080/health

# LLM Readiness（深度健康 — 含 Provider 狀態、Router 版本）
curl -fsS http://localhost:18080/api/llm/health
```

預期回傳：
- `/health`：JSON `{"status":"ok",...}`
- `/api/llm/health`：JSON `{"providers":{"deepseek":{...},"minimax":{...}},"router_version":"v2.1"}`

### 部署後驗證腳本

```bash
#!/usr/bin/env bash
# scripts/verify_deploy.sh
set -e
echo "=== Liveness ==="
curl -fsS http://localhost:18080/health | jq .
echo "=== LLM Health ==="
curl -fsS http://localhost:18080/api/llm/health | jq .
echo "=== Container Status ==="
docker compose ps --format json | jq -s 'map({name, state, health})'
```

### Rollback

```bash
# 退回上一個 commit 並重啟
git checkout <previous-sha> -- docker-compose.yml  # 若有 compose 變更
docker compose up -d
```

或使用 ghcr.io tag pinning：修改 `docker-compose.yml` 的 `image:` tag 為上一個版本，重新 `docker compose up -d`。

## Token Efficiency Rules

- **Scoped reads**: Use targeted file paths (e.g. `shared_web/static/css/main.css`) instead of directory reads. Never read `data/` or `.gitnexus/`.
- **/compact between subtasks**: Run `/compact` between independent subtasks to reclaim context window.
- **Frontend scope**: For CSS/JS-only changes, skip impact analysis entirely. Only run `gitnexus_impact` for Go backend changes touching 3+ symbols.
- **Precise file targeting**: Before reading, verify the exact file path with `glob`. Avoid speculative reads of large files.
- **No duplicate rules**: This file intentionally does not repeat AGENTS.md rules. One source of truth only.


