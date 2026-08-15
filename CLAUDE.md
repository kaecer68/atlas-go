# CLAUDE.md — atlas-go 規則索引

@AGENTS.md

> **Wave 11 L2.3 PoC + L2.4 觀察窗口** — v0.0.0.18..v0.0.2.0+。L2.3 設計見 [`docs/specs/llm-sector-agent-spec.md`](docs/specs/llm-sector-agent-spec.md)、[`docs/specs/agent-loop-state-machine-spec.md`](docs/specs/agent-loop-state-machine-spec.md)、[`docs/guides/adding-sector-agents.md`](docs/guides/adding-sector-agents.md)。L2.4 已 ship（PR #821 merged 2026-06-29），文件永久化於 [`docs/operations/l2-4-runbook.md`](docs/operations/l2-4-runbook.md) + [`docs/specs/l2-4-observation-spec.md`](docs/specs/l2-4-observation-spec.md) + [`docs/archive/l2-4-followup.md`](docs/archive/l2-4-followup.md)（後續工作報告）。`UseLLMSectorAgents` flag 預設 off。

## 語言強制規範

見 [`AGENTS.md`](AGENTS.md)（跨工具權威來源）。本檔案為 Claude Code 專屬設定入口。

## Session 開工自檢

開始任何程式碼變更前，MUST 執行：

```bash
git branch --show-current
```

- 若結果為 `main`：**立刻 `git checkout -b feat/<feature-name>` 開新分支**，絕不在 main 上直接改 code。
- 若已在 feature branch 上：可繼續工作。

**違反此規則 = 繞過 PR review / CI 追蹤，不可接受。**

## 快速路由

| 需求 | 參考位置 |
|------|---------|
| 前端架構（admin_web / client_web / shared_web） | [`docs/guides/frontend-architecture.md`](docs/guides/frontend-architecture.md)（單一權威來源） |
| 部署設定 | **production 在 iMac**：`~/workspace/a2a-dev/docs/operations/iMac-RUNBOOK.md`；本機 Docker 開發：`docs/operations/local-deploy.md` |
| Token 效率規則 | `## Token Efficiency Rules`（下方） |
| PR 驗證清單 | `## PR 驗證清單`（下方） |

## Health Check → [`docs/operations/local-deploy.md`](docs/operations/local-deploy.md) §部署驗證

```bash
# production (iMac) — 對外
curl -fsS https://atlas.goluck.uk/health           # Liveness
curl -fsS https://atlas.goluck.uk/api/llm/health    # LLM Readiness (需 API key)
# 本機 dev
curl -fsS http://localhost:18080/health            # Liveness
```

## Token Efficiency Rules

- **Scoped reads**: Use targeted file paths (e.g. `shared_web/static/css/main.css`) instead of directory reads. Never read `data/` or `.gitnexus/`.
- **/compact between subtasks**: Run `/compact` between independent subtasks to reclaim context window.
- **Frontend scope**: For CSS/JS-only changes, skip impact analysis entirely. Only run `gitnexus_impact` for Go backend changes touching 3+ symbols.
- **Precise file targeting**: Before reading, verify the exact file path with `glob`. Avoid speculative reads of large files.
- **No duplicate rules**: This file intentionally does not repeat AGENTS.md rules. One source of truth only.

## PR 驗證清單（必跑）

每筆 PR merge 前，AI 必須跑以下 3 個 gate。不可 skip。

### 1. Route uniqueness check

```bash
make check-routes
```

檢查所有 HTTP 路由是否有**概念衝突**（同一資源兩條不同路徑）或 **canary test stale path**。
若有 `FAIL`，必須修正後才能 merge。

### 2. Hermes consumer smoke test

```bash
make hermes-smoke
```

對 running atlas-go 打出 Hermes（使用 agent）所有驗證過的 endpoint：
- E-01~E-13 audit items（market explain, regime, risk, correlation, strategy, capital flow…）
- data quality, LLM health, stress index, system health

全部 200 才算通過。任何非 200 → 不能 merge。

### 3. Consumer contract check（合約檔）

每個 MCP tool 對應的 HTTP path 必須滿足：
1. 存在於 route table（`make check-routes` 已含）
2. canary test path（`tools_canary_test.go`）與 handler code path（`tools_*.go`）一致
3. consumer（Hermes）可以直打並拿到 200 + 合法 JSON

**違反案例**：`data_get_field_contract` → canary test 寫 `/api/data/field-contract`，但 handler code 打 `/api/field-contract`。
前者 401，後者 200。這種 mismatch 是 E-12 audit 浪費 20 小時的根本原因。

### Gate 順序

```
make check-routes  →  靜態分析（不需 container）
make hermes-smoke  →  動態驗證（需 running atlas-go）

兩者皆 0 fail → 可 merge
任一 fail     → 先修再測
```
