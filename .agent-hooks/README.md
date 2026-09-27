# Atlas Agent Hooks

`.agent-hooks/` 收容所有在 AI agent session 中執行的 deterministic guardrails / reminder hooks。
**與 `.githooks/` 不同**: `.githooks/` 是 git 觸發(commit/push),`.agent-hooks/` 是 Claude Code session 內觸發。

## 已收容工具

| 工具 | 角色 | 觸發時機 | 設計 |
|------|------|---------|------|
| [`deny-dangerous.sh`](deny-dangerous.sh) | **Hard block** for destructive / secrets / production 操作 | ① 自動:Claude Code `PreToolUse`(matcher `Bash`,經下方 adapter;註冊於 tracked 的 `.claude/settings.json`)② 手動:`./agent-guard --check '<cmd>'` | exit 0 = allow,exit 1 = block;**預設即 `enforce`**(2026-09-27,E23);`ATLAS_HOOK_MODE=warn` 為文件化逃生口 |
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
| **預設**(任何 worktree,含 dev) | `enforce` | 擋下(Claude Code hook exit 2) |
| `export ATLAS_HOOK_MODE=warn` | `warn` | 注入明確警告給模型與使用者,**不擋**(文件化逃生口) |
| `ATLAS_ENV=production` 且未設 `ATLAS_HOOK_MODE` | `enforce` | 同上(自 E23 起為冗餘;保留的原因:`ATLAS_HOOK_MODE=""` 這種空字串仍視為未設) |
| guard 執行失敗(例如直譯器缺失) | 任意 | **fail-open**:放行 + stderr 明確告警 |

> 2026-09-27(E23)前預設是 `warn`。翻成 `enforce` **之前**先移除了三個已實測的誤擋面
> (pattern 4 只因指令含 `secret` 一字就判讀機密檔、pattern 8 擋掉部署路徑與 `go test`/`make test`)。
> 每一面都有「修前擋、修後通過」的迴歸斷言在 `tests/scripts/test-agent-hook-wiring.sh`(§E23),再引入就會紅燈。

```bash
# 逃生口:整個 agent session 放寬回 warn(必須在「啟動 agent 的那個 shell」export)
export ATLAS_HOOK_MODE=warn

# 只放行 production 的裸 docker compose(部署用;見下方「production 部署」)
export ATLAS_ALLOW_PROD_COMPOSE=true

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
> 自 E23 起預設 `enforce` ⇒ 被誤判就是真的擋下(不是提示)。因此:
> ① 每個 block 都必須附「合法路徑」提示(`block()` 的第二個參數);
> ② 守門故障時仍 fail-open(不可把整個 session 鎖死);
> ③ 判定必須是「**目標/結構**」而非「指令列出現某個詞」(pattern 1/2/4/5/6/8 皆如此)。

## production 部署與 guard 的關係(E23)

`ATLAS_ENV=production` 下,pattern 8 **不再**擋掉部署本身:

| 指令 | production worktree | 理由 |
|------|--------------------|------|
| `make rebuild-all` | ✅ 通過 | Mac Mini 現行部署入口(`docs/operations/local-deploy.md`) |
| `bash scripts/deploy-staging.sh` | ✅ 通過 | 文件化部署腳本(內部才跑 `docker compose build && up -d`) |
| `ATLAS_GIT_COMMIT=$(git rev-parse HEAD) docker compose up -d` | ✅ 通過 | 文件化的裸 compose 契約(必須帶 commit pin) |
| `docker compose build` / `up -d`(無 pin) | ❌ 擋下 | 需明確意圖:加 `ATLAS_GIT_COMMIT=` pin,或 export `ATLAS_ALLOW_PROD_COMPOSE=true` |
| `go test ./...` / `make test` / `make ci-gate` / `make ci-full` | ✅ 通過 | 測試是唯讀;修復迴圈就在這個 worktree |
| `make dev` / `make clean` / `go run` / 實驗 CLI | ❌ 擋下 | 真正的 dev-only 動作 |

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
