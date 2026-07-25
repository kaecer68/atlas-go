# 本機部署設定

> 從 `CLAUDE.md` §「部署設定」移出。Claude Code 專屬部署細節，非部署任務不需載入。

## 平台

**本機 Docker**（`docker compose` 單機部署，非 production server）。

## 映像來源

- 註冊表：`ghcr.io/kaecer68/atlas-go`（`ci-cd.yml` main/develop 自動建置推送）
- Dockerfile：multi-stage（Node.js 前端 + Go 1.26 後端），expose port 18080
- compose 設定：`docker-compose.yml`（healthcheck、env vars、postgres）

## 環境變數（統一由 `~/.config/atlas-go/.env` 載入）

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

## 部署流程

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

## 部署驗證

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

## Rollback

```bash
# 退回上一個 commit 並重啟
git checkout <previous-sha> -- docker-compose.yml  # 若有 compose 變更
docker compose up -d
```

或使用 ghcr.io tag pinning：修改 `docker-compose.yml` 的 `image:` tag 為上一個版本，重新 `docker compose up -d`。
