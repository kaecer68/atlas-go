# Atlas Agent Hooks

`.agent-hooks/` 收容所有在 AI agent session 中執行的 deterministic guardrails / reminder hooks。
**與 `.githooks/` 不同**: `.githooks/` 是 git 觸發(commit/push),`.agent-hooks/` 是 Claude Code session 內觸發。

## 已收容工具

| 工具 | 角色 | 觸發時機 | 設計 |
|------|------|---------|------|
| [`deny-dangerous.sh`](deny-dangerous.sh) | **Hard block** for destructive / secrets / production 操作 | ① 自動:Claude Code `PreToolUse`(matcher `Bash`,經下方 adapter;註冊於 tracked 的 `.claude/settings.json`)② 手動:`./agent-guard --check '<cmd>'` | exit 0 = allow,exit 1 = block;`ATLAS_HOOK_MODE=warn\|enforce` 切換 |
| [`pretooluse-deny-dangerous.sh`](pretooluse-deny-dangerous.sh) | **Adapter** — 把 Claude Code hook payload(stdin JSON)的 Bash 指令交給 `deny-dangerous.sh`,再把判定翻成 hook 協定 | 自動:本 repo 每次 Bash tool call | 純 glue(不重複 pattern 邏輯);warn ⇒ exit 0 + 注入訊息,enforce ⇒ exit 2 擋下,守門壞掉 ⇒ fail-open + 告警 |
| [`aci-read-prompt.sh`](aci-read-prompt.sh) | **Soft reminder** 推 agent 走 ACI routing 流程 | 自動由 Claude Code `PreToolUse` hook 觸發,當 Read/Edit/Write/Grep/Bash 接觸 `internal/` 或 `cmd/` 下的 Go 檔 | 注入 `additionalContext`(~150 token),不 block,每檔每 session 去重一次 |
| [`install.sh`](install.sh) | 安裝入口 — 設 executable + 建 `./agent-guard` symlink + 印使用說明 | 一次性,每 worktree 跑一次 | — |

> `aci-read-prompt.sh` 是 **soft layer**,與 `deny-dangerous.sh` 是 **hard layer** — 兩者互補不衝突。
> 一個管「不要做危險事」,一個管「做事前先想清楚」。

## 安裝

**hard layer(deny-dangerous) 不需要任何安裝步驟**:`pretooluse-deny-dangerous.sh`
已在 tracked 的 `.claude/settings.json` 以 `PreToolUse` / matcher `Bash` 註冊 ⇒
任何人 clone/worktree 開 Claude Code session,每次 Bash tool call 都會經過它
(2026-09-26 E5 接線;在此之前這個守門**從未被自動呼叫**)。

soft layer 與便利 alias 仍需一次性動作:

```bash
bash .agent-hooks/install.sh
```

這會:
1. 確保 `deny-dangerous.sh`、`pretooluse-deny-dangerous.sh` 與 `aci-read-prompt.sh` 都是 `chmod +x`
2. 在 repo root 建 `./agent-guard` symlink 指向 `deny-dangerous.sh`(該 symlink 被 `.gitignore` 排除)
3. 印使用說明

> `aci-read-prompt.sh` 預設 **不啟用** — 它必須透過 `.claude/settings.local.json` 註冊才會被 Claude Code 呼叫。
> 安裝腳本**不會**自動寫 `.claude/settings.local.json`;這是 per-user 選擇,參考 [`docs/operations/aci-hook-usage.md`](../docs/operations/aci-hook-usage.md)。

## 模式政策與 rollback（hard layer）

| 情境 | 判定模式 | 行為 |
|------|---------|------|
| 本機 dev worktree(預設) | `warn` | 注入明確警告給模型與使用者,**不擋** |
| `export ATLAS_HOOK_MODE=enforce` | `enforce` | 擋下(Claude Code hook exit 2) |
| `ATLAS_ENV=production` 且未設 `ATLAS_HOOK_MODE` | `enforce` | 同上 |
| guard 執行失敗(例如直譯器缺失) | 任意 | **fail-open**:放行 + stderr 明確告警 |

```bash
# 切成 enforce(單一 session,不必改檔)
export ATLAS_HOOK_MODE=enforce

# 放寬回 warn(即使在 production 機上)
export ATLAS_HOOK_MODE=warn

# 一鍵還原接線:移除 tracked settings.json 內的 PreToolUse 段
python3 - <<'PY'
import json
p = ".claude/settings.json"
s = json.load(open(p))
s["hooks"].pop("PreToolUse", None)
json.dump(s, open(p, "w"), indent=2)
PY
```

> 影響面:一旦接線,**此 repo 內所有 Claude Code Bash 呼叫**都會先經過 `deny-dangerous.sh`。
> 因此判定必須保守:warn 為預設,且守門故障時 fail-open(不可把整個 session 鎖死)。

## 依賴

| 工具 | 依賴 | macOS 安裝 |
|------|-----|-----------|
| `deny-dangerous.sh` | bash 3.2+(**不可**用 `${var,,}`,見下),內建工具 | 預設已裝 |
| `pretooluse-deny-dangerous.sh` | bash 3.2+,**python3**(解析 hook JSON),`deny-dangerous.sh` | `python3` 由 repo 既有工具鏈提供(`scripts/*.py`、`tests/scripts/*`) |
| `aci-read-prompt.sh` | bash 3.2+,**jq 1.6+**,git | `brew install jq` |

> `aci-read-prompt.sh` 缺 jq 時會**靜默退出 0**(不觸發提示),不 crash session — 與 `scripts/session-start.sh` 的 graceful-degradation 模式一致。
> `pretooluse-deny-dangerous.sh` 缺 python3、payload 不是合法 JSON、或 `deny-dangerous.sh` 執行失敗時
> **不會靜默**:一律 fail-open(exit 0)並在 stderr 印出明確告警 —— 接線後的 silent no-op 就是這個 PR 要消滅的缺陷。
>
> ⚠️ `${var,,}`(小寫化)是 **bash 4** 語法。`#!/usr/bin/env bash` 在 macOS 可能解析到 `/bin/bash` 3.2,
> 屆時 `deny-dangerous.sh` 會以 `bad substitution` 對**每個**指令中止(exit 1)⇒ 良性指令被誤判為違規。
> 2026-09-26 已改用 `tr`。`tests/scripts/test-agent-hook-wiring.sh` 會用系統 bash(含 3.2)重跑整個判定矩陣。

## 與既有 hook 系統的關係

| 系統 | 觸發者 | 觸發時機 | 設計 |
|------|-------|---------|------|
| `.githooks/pre-commit` | git | 每次 `git commit` | hard block binary / PID / coverage 檔入庫 |
| `.githooks/pre-push` | git | 每次 `git push` | hard block push 到 main / 空 push |
| `scripts/session-start.sh` | Claude Code `SessionStart` | session 開頭 | binary freshness gate(check-binaries → rebuild) |
| `.agent-hooks/aci-read-prompt.sh` | Claude Code `PreToolUse`(per-user,`.claude/settings.local.json`) | 每次 Read/Edit/Write/Grep/Bash 工具呼叫前 | soft reminder for ACI routing |
| `.agent-hooks/pretooluse-deny-dangerous.sh` | Claude Code `PreToolUse`(tracked,`.claude/settings.json`,matcher `Bash`) | 每次 Bash tool call | adapter → hard block verdict for state-mutating / secrets / production |
| `.agent-hooks/deny-dangerous.sh` | 顯式呼叫(`./agent-guard --check`)＋ 上述 adapter | agent 自行判斷 / 每次 Bash tool call | hard block for state-mutating / secrets / production |

> hook 設計的紅線:**hard block 只用在「做了就回不去」的操作**。`aci-read-prompt.sh` 違反這條(讀程式是無害),所以走 soft reminder;`deny-dangerous.sh` 符合,所以走 hard block。

## 文件

- 使用說明:[`docs/operations/aci-hook-usage.md`](../docs/operations/aci-hook-usage.md)
- 設計決策（plan 已刪除，決策為 durable artifact）:[`.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md`](../.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md)
- 已知陷阱:[`.claude/agent-memory/footguns/agent-skips-aci-routing.md`](../.claude/agent-memory/footguns/agent-skips-aci-routing.md)
- 設計決策:[`.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md`](../.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md)
