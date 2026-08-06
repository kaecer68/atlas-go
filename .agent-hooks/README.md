# Atlas Agent Hooks

`.agent-hooks/` 收容所有在 AI agent session 中執行的 deterministic guardrails / reminder hooks。
**與 `.githooks/` 不同**: `.githooks/` 是 git 觸發(commit/push),`.agent-hooks/` 是 Claude Code session 內觸發。

## 已收容工具

| 工具 | 角色 | 觸發時機 | 設計 |
|------|------|---------|------|
| [`deny-dangerous.sh`](deny-dangerous.sh) | **Hard block** for destructive / secrets / production 操作 | 由 agent 自行呼叫(`./agent-guard --check '<cmd>'`)或 shell 內可選攔截 | exit 0 = allow,exit 1 = block;`ATLAS_HOOK_MODE=warn\|enforce` 切換 |
| [`aci-read-prompt.sh`](aci-read-prompt.sh) | **Soft reminder** 推 agent 走 ACI routing 流程 | 自動由 Claude Code `PreToolUse` hook 觸發,當 Read/Edit/Write/Grep/Bash 接觸 `internal/` 或 `cmd/` 下的 Go 檔 | 注入 `additionalContext`(~150 token),不 block,每檔每 session 去重一次 |
| [`install.sh`](install.sh) | 安裝入口 — 設 executable + 建 `./agent-guard` symlink + 印使用說明 | 一次性,每 worktree 跑一次 | — |

> `aci-read-prompt.sh` 是 **soft layer**,與 `deny-dangerous.sh` 是 **hard layer** — 兩者互補不衝突。
> 一個管「不要做危險事」,一個管「做事前先想清楚」。

## 安裝

每個 worktree 跑一次:

```bash
bash .agent-hooks/install.sh
```

這會:
1. 確保 `deny-dangerous.sh` 與 `aci-read-prompt.sh` 都是 `chmod +x`
2. 在 repo root 建 `./agent-guard` symlink 指向 `deny-dangerous.sh`
3. 印使用說明

> `aci-read-prompt.sh` 預設 **不啟用** — 它必須透過 `.claude/settings.local.json` 註冊才會被 Claude Code 呼叫。
> 安裝腳本**不會**自動寫 `.claude/settings.local.json`;這是 per-user 選擇,參考 [`docs/operations/aci-hook-usage.md`](../docs/operations/aci-hook-usage.md)。

## 依賴

| 工具 | 依賴 | macOS 安裝 |
|------|-----|-----------|
| `deny-dangerous.sh` | bash 3.2+,內建工具 | 預設已裝 |
| `aci-read-prompt.sh` | bash 3.2+,**jq 1.6+**,git | `brew install jq` |

> `aci-read-prompt.sh` 缺 jq 時會**靜默退出 0**(不觸發提示),不 crash session — 與 `scripts/session-start.sh` 的 graceful-degradation 模式一致。

## 與既有 hook 系統的關係

| 系統 | 觸發者 | 觸發時機 | 設計 |
|------|-------|---------|------|
| `.githooks/pre-commit` | git | 每次 `git commit` | hard block binary / PID / coverage 檔入庫 |
| `.githooks/pre-push` | git | 每次 `git push` | hard block push 到 main / 空 push |
| `scripts/session-start.sh` | Claude Code `SessionStart` | session 開頭 | binary freshness gate(check-binaries → rebuild) |
| `.agent-hooks/aci-read-prompt.sh` | Claude Code `PreToolUse` | 每次 Read/Edit/Write/Grep/Bash 工具呼叫前 | soft reminder for ACI routing |
| `.agent-hooks/deny-dangerous.sh` | 顯式呼叫(`./agent-guard --check`) | 由 agent 自行判斷何時跑 | hard block for state-mutating / secrets / production |

> hook 設計的紅線:**hard block 只用在「做了就回不去」的操作**。`aci-read-prompt.sh` 違反這條(讀程式是無害),所以走 soft reminder;`deny-dangerous.sh` 符合,所以走 hard block。

## 文件

- 使用說明:[`docs/operations/aci-hook-usage.md`](../docs/operations/aci-hook-usage.md)
- 設計 plan:[`.omo/plans/2026-08-06-aci-pretooluse-prompt.md`](../docs/archive/2026-08-06-aci-pretooluse-prompt-plan.md)
- 已知陷阱:[`.claude/agent-memory/footguns/agent-skips-aci-routing.md`](../.claude/agent-memory/footguns/agent-skips-aci-routing.md)
- 設計決策:[`.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md`](../.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md)
