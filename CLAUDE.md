# CLAUDE.md — atlas-go 規則索引

@AGENTS.md

> **Wave 11 L2.3 PoC + L2.4 觀察窗口** — v0.0.0.18..v0.0.0.21。詳見 [`docs/specs/llm-sector-agent.md`](docs/specs/llm-sector-agent.md) (L2.3 plan/reflect 設計)、[`docs/specs/agent-loop-state-machine.md`](docs/specs/agent-loop-state-machine.md)、[`docs/guides/adding-sector-agents.md`](docs/guides/adding-sector-agents.md)。L2.4 觀察規劃見 [`.omo/wave-11-l2-4/`](../.omo/wave-11-l2-4/)。`UseLLMSectorAgents` flag 預設 off。

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
> 重構後 `./web/` 不再對外服務,僅作 archive;後端 `cmd/atlas/api_routes.go` 只掛載 `admin_web.DistFS` 與 `client_web.DistFS`。

### 目錄職責

| 目錄 | 角色 | 對外 URL |
|------|------|---------|
| `admin_web/static/js/` | 管理後台專屬 JS(`main.js`、`component-init.js`、`event-listeners.js`) | `/admin/` |
| `client_web/static/js/` | 投資人介面專屬 JS | `/client/` |
| `shared_web/static/js/` | 共用 JS(pages、components、services、shared、bootstrap-utils) | 經 esbuild plugin fallback 引入 |
| `shared_web/static/css/` | 全部 CSS(dark/light 主題、components、layout、pages) | 經 esbuild 打包成 `css/main.css` |

### 入口檔職責

| 檔案 | 職責 |
|------|------|
| `main.js` | 全域狀態、頁面切換(`switchPage`)、動態 import pages、執行各頁 init |
| `component-init.js` | 共用 component 初始化(circuit-breaker、sim-health、performance-report) |
| `event-listeners.js` | DOM event 綁定(sidebar nav、evView 按鈕、shock sim 互動、modal 關閉) |
| `pages/*.js` | 每個頁面的 render 函式,由 `main.js` 動態 import |

### esbuild plugin fallback 規則

`esbuild-shared-plugin.mjs`(`shared_web/`)定義:
- 從 `admin_web/static/` 找不到的 import → fallback 到 `shared_web/static/`
- 從 `shared_web/static/` 找不到的 import → fallback 到 `admin_web/static/`(例:`main.js` 中的全域函式)

**陷阱**:若有人刪掉 `shared_web/static/js/pages/xxx.js`,esbuild 會**靜默 fallback 失敗**,UI 卡「載入中...」。CI 驗證腳本 `scripts/ci/check_frontend_imports.sh` 會抓出。

### 前端 CSS 與 JS 規範

- 全部 CSS 變數定義於 `shared_web/static/css/base/variables.css`(canonical)
- Canvas 繪圖色彩用 `getThemeColor()` + `hexToRgba()` 橋接(`shared_web/static/js/shared/utils.js`)
- 金融語意 Token:`--pnl-profit`/`--pnl-loss`、`--trend-bullish`/`--trend-bearish`、`--metric-good`/`--metric-bad`、`--risk-high`/`--risk-low`
- 顏色一律用 `var(--...)`,不寫死 hex/rgba

### 路由 + 後端整合

- `cmd/atlas/api_routes.go` L31-38:`/admin/` 與 `/client/` 分別掛載 `admin_web.DistFS` 與 `client_web.DistFS`
- API 端點統一前綴 `/api/...`(dashboard、narrative、industry、taiwan、...)
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
- Dockerfile：multi-stage（Node.js 前端 + Go 1.26 後端），expose port 8080
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
curl -fsS http://localhost:8080/health

# LLM Readiness（深度健康 — 含 Provider 狀態、Router 版本）
curl -fsS http://localhost:8080/api/llm/health
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
curl -fsS http://localhost:8080/health | jq .
echo "=== LLM Health ==="
curl -fsS http://localhost:8080/api/llm/health | jq .
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


