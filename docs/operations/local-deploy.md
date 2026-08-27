# 部署設定（本機 dev + production 雙機）

> **文件角色**：部署的權威說明。涵蓋 MacBook 本機 dev 與 iMac production 兩種情境。
> **雙機治理**：開發在 MacBook、production 在 iMac（方案二，2026-08-15 定案）。
> **跨設備總則**：`~/workspace/a2a-dev/docs/governance/雙機治理憲章.md`；iMac 運維手冊：`~/workspace/a2a-dev/docs/operations/iMac-RUNBOOK.md`。

## 平台架構（方案二真相）

```
MacBook (kaecer) = 唯一開發機           iMac (kk) = 唯一 production 部署機
  ├─ code 編輯 / 測試 / PR               ├─ atlas 11 容器 + litellm 2 容器
  ├─ git push → GitHub                   ├─ git pull（只讀 clone，不 push）
  └─ 本機 dev 驗證（可 build/run）        └─ docker build + compose up（hermes 運維）
```

- **映像來源（production）**：iMac **本地 build**（`atlas-atlas:latest`），**不是** ghcr.io pull。
- **部署流程**：MacBook push → iMac `git pull` → iMac `make rebuild-all`（或 hermes 代勞）。
- **本機 dev（MacBook）**：可用 `make rebuild-all` 起本地容器驗證（不影響 iMac production，不同機器）。

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

> **兩機 .env 分離**：MacBook `~/.config/atlas-go/.env` 指向 dev DB（`atlas_dev`）；iMac `~/.config/atlas-go/.env` 指向 prod DB（`atlas`）。**不可互相覆蓋**。

## 部署流程

### 情境 A：本機 dev 驗證（MacBook）

```bash
# 1. 確認 main HEAD 已是目標版本
git fetch origin main && git log --oneline origin/main -1

# 2. 本地 build + 起容器（完整 stack）
make rebuild-all

# 3. 驗證
docker compose ps
curl -fsS http://localhost:18080/health
```

### 情境 B：production 部署（iMac）

```bash
# 1. MacBook: push 你的修改
git push origin main

# 2. iMac: 同步 + 重建 + 重啟（hermes 可代勞）
ssh kk@kimac "cd ~/workspace/atlas && git pull origin main && make rebuild-all"

# 3. iMac: 驗證正式服務
curl -fsS http://localhost:18080/health
```

> **hermes 代勞**：部署是 hermes（iMac 運維員）的職責。可用 hermes-dispatch skill 派她完成
> `git pull → make rebuild-all → 驗證 /health → 回報`。

> ⚠️ **Darwinian state 同步（2026-08-27 起）**：`data/state/darwinian_history.jsonl` 不是 git
> tracked，部署不會自動帶過去。任何會**重建/重啟 atlas-go 或 atlas-cron-darwinian 容器**的部署，
> 先跑 `~/workspace/atlas/scripts/sync-darwinian.sh`（union merge，只增不減），
> 並遵守「sync 前容器必須停」的硬性規定（避免 torn line）。完整章節見
> a2a-dev `docs/deployment/IMAC-DEPLOY-RUNBOOK.md` §2.1。

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
# 退回上一個 commit 並重啟（iMac）
ssh kk@kimac "cd ~/workspace/atlas && git checkout <previous-sha> && make rebuild-all"
```

> **注意**：iMac 用本地 build image（`atlas-atlas:latest`），Rollback = checkout 舊 commit 重建。
> 已不使用 ghcr.io tag pinning（舊模式，ghcr 已被本地 build 取代）。
