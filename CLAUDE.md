# CLAUDE.md — atlas-go 規則索引

## 🌐 語言強制規範（首要規則）

> **全部回覆皆須使用繁體中文（Traditional Chinese）**。除非使用者明確要求使用英文，否則禁止使用英文回應。此規則優先於所有技術指令。

本檔案僅作為工具進入點。所有專案專屬規則、陷阱與禁令，請直接參考 **AGENTS.md**。
本檔案不重複任何規則，以確保單一權威來源，避免 token 重複計費。

全局規則仍遵循 `~/.claude/CLAUDE.md`。

## 快速路由

| 需求 | 文件 |
|------|------|
| 完整模組索引（34 個） | `internal/AGENTS_INDEX.md` |
| 模組成熟度對照 | `internal/MATURITY.md` |
| 跨模組陷阱詳細參考 | `docs/TRAPS.md` |
| 外部依賴與環境狀態 | `docs/ENVIRONMENT.md` |
| 根規則與全域禁令 | `AGENTS.md` |
| 架構憲法 | `internal/apigateway/CONSTITUTION.md` |
| 部署設定（本機 Docker） | `## 部署設定`（下方） |

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
| `LLM_OPENCODE_GO_API_KEY` | OpenCode-Go（自架/本地） | provider 標記為未來規劃，目前 routing chain 有列但未實際掛載 |

| `LLM_ANNOTATOR_API_KEY` | **向後相容** — 早期 `KimiClient` 讀此變數 | 實際值等同 `LLM_MINIMAX_API_KEY`（Kimi K2.7 因 coding plan key 限制已移除） |
| `LLM_RATIONALE_TRANSLATION_ENABLED` | 啟用 `CapabilityRationaleGeneration` hook | default `false` |
| `LLM_PRISM_SCENARIO_ENABLED` | 啟用 `CapabilityScenarioSimulation` hook | default `false` |
| `LLM_NARRATIVE_EXPLAIN_ENABLED` | 啟用 `CapabilityRegimeExplanation` + `CapabilitySentimentExplanation` | default `false` |
| `LLM_RISK_FORENSICS_ENABLED` | 啟用 `CapabilityPerformanceForensics` | default `false` |

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

- **Scoped reads**: Use targeted file paths (e.g. `web/static/css/main.css`) instead of directory reads. Never read `data/` or `.gitnexus/`.
- **/compact between subtasks**: Run `/compact` between independent subtasks to reclaim context window.
- **Frontend scope**: For CSS/JS-only changes, skip impact analysis entirely. Only run `gitnexus_impact` for Go backend changes touching 3+ symbols.
- **Precise file targeting**: Before reading, verify the exact file path with `glob`. Avoid speculative reads of large files.
- **No duplicate rules**: This file intentionally does not repeat AGENTS.md rules. One source of truth only.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (52605 symbols, 165211 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/atlas-go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/atlas-go/clusters` | All functional areas |
| `gitnexus://repo/atlas-go/processes` | All execution flows |
| `gitnexus://repo/atlas-go/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
