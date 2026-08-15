# ACI Routing Reminder Hook — 使用說明

> **對象**: atlas-go 開發者,想在自己的 Claude Code session 內啟用
> 軟性 ACI routing 提醒。
>
> **來源 PR**: `feat/20260806-aci-pretooluse-prompt`(`aci-read-prompt.sh` + agent-memory + `.gitignore` 補完)
>
> **設計決策**: [`.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md`](../../.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md)
>
> **設計決策**: [`.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md`](../../.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md)

## TL;DR

```bash
# 一次性(每個新 worktree 跑一次):
brew install jq                                    # macOS
bash .agent-hooks/install.sh

# 啟用 hook(本機,不會 commit):
mkdir -p .claude
# 建立 .claude/settings.local.json,內容見下方「完整設定」

# 驗證 hook 運作:
# 1. 重新啟動 Claude Code session
# 2. 嘗試 Read internal/orchestrator/agent_loop.go
# 3. 應該在 tool result 後看到一段 ~150 token 的 routing 提示
```

## 為什麼要裝?

atlas-go 是一個 35k symbols / 104k edges / 60 modules 的大型 Go codebase。
AI agent 在 mid-task 改 hot-path module 時,常見三種「偷懶」:

1. **重複造輪子** — 沒先跑 `gitnexus_query` 看是否已有對等實作
2. **補丁式亂加** — 沒先跑 `gitnexus_impact` 看 blast radius
3. **亂猜測** — 回答事實性問題不先跑 ACI 查 docs/code

`atlas-pre-change-protocol` skill 已定義 8 步強制檢查,但它是**被動**的 —
agent 必須記得自己 load。這個 hook 在 `PreToolUse` 時機**主動**注入 routing
提示,讓 agent 在第一次 Read 進 hot-path Go 程式時被提醒。

## 完整設定(`.claude/settings.local.json`)

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read|Edit|Write|Grep|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PROJECT_DIR}/.agent-hooks/aci-read-prompt.sh",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

`.claude/settings.local.json` 已被 `.gitignore` 排除(此次 PR 順手補上防護),
**不會被 commit**。這是 per-user 設定,符合 Claude Code 官方慣例。

## Hook 行為

| Tool 呼叫 | 條件 | 觸發 | 備註 |
|----------|------|-----|------|
| `Read` / `Edit` / `Write` | `*.go` 路徑在 `internal/` 或 `cmd/` | ✅ 提示 | 主要場景 |
| `Grep` | `path` 在 `internal/` 或 `cmd/` | ✅ 提示 | 防止 agent 用 grep 取代 ACI |
| `Bash` | command 包含 `grep`/`rg`/`find` + `internal/` 或 `cmd/` 路徑 | ✅ 提示 | 同上 |
| 其他 tool 或路徑 | — | ❌ 跳過 | 零干擾 |

**特性**:

- **不 block**:agent 仍可自由讀寫,提示只是 nudging
- **去重**:每個 (tool, file_path) 在同一 session 最多提示一次
- **graceful degradation**:缺 `jq` 時靜默跳過,不 crash session
- **常駐路徑**:`${XDG_CACHE_HOME:-$HOME/.cache}/atlas-aci-prompted/${session_id}.list`
  (session 自然結束後不主動清,下次新 session 用新 id 自然新開)

## 觸發後會看到什麼

在 tool result 之後,模型會收到約 150 token 的 routing 提示:

```
AC: atlas-go ACI routing — 你正要讀/改/搜 hot-path Go 檔(屬 internal/ 或 cmd/)。先想一下:

  Step 0 (overlap):  已有對等實作嗎?
                     → gitnexus_query(query="<concept>")
                     → 或 codebase-memory_search_graph(query="<concept>")

  Step 1 (blast radius):  改這個 symbol 會影響誰?
                          → gitnexus_impact(target="<symbol>", direction="upstream")

  Step 1.5 (source context): 要看 caller source code?
                            → codebase-memory_explore(query="<symbol>")

  hot-path 模組 (有 internal/<mod>/AGENTS.md): apigateway / capitalflow / ...
  完整 8 步(必跑): 載入 atlas-pre-change-protocol skill。
  本提醒每檔每 session 只一次,不會再打擾。
```

## 與既有 hook 的關係

| 系統 | 觸發時機 | 角色 |
|------|---------|------|
| `.githooks/pre-commit` | git commit | hard block binary / PID / coverage 進庫 |
| `.githooks/pre-push` | git push | hard block push main / 空 push |
| `scripts/session-start.sh` (`.claude/settings.json`) | session 開頭 | binary freshness gate |
| **`.agent-hooks/aci-read-prompt.sh` (this)** | **PreToolUse** | **soft reminder for ACI routing** |
| `.agent-hooks/deny-dangerous.sh` | 顯式呼叫 | hard block for state-mutating / secrets / production |

## 卸載

```bash
# 停用 hook(保留安裝的檔案):
rm .claude/settings.local.json

# 完全卸載(連 hook 檔案都刪,但保留 README / agent-memory 給他人):
rm .agent-hooks/aci-read-prompt.sh
```

`README.md` / agent-memory 條目建議保留 — 它們文件化這個 anti-pattern,
agent 在下一個 session 看到 footgun 清單時會自己複習。

## 故障排除

### Hook 沒觸發

1. 確認 `jq` 已裝:`command -v jq && jq --version` 應有輸出
2. 確認 `.claude/settings.local.json` 存在且 JSON 合法:`jq . .claude/settings.local.json`
3. 確認 hook script 可執行:`ls -la .agent-hooks/aci-read-prompt.sh`(應有 x)
4. 重新啟動 Claude Code session(改 settings 不會熱生效)

### Hook 觸發太頻繁

每個 (tool, file_path) 在同 session 只提示一次,但跨 session 會重新提示。
若你常跨 session 編輯同一檔,可:

- 暫時關閉:`rm .claude/settings.local.json`
- 永久關閉:在 `.claude/settings.local.json` 把 `matcher` 從
  `"Read|Edit|Write|Grep|Bash"` 改為 `"Edit|Write"`(只提示「即將改」,不提示「讀」)

### Bash 分支漏觸發

`Bash` 分支的 regex 故意做窄(`grep|rg|find` 必須出現 + 路徑必須是
`internal/` 或 `cmd/`)。`bash -c "..."` 包裝或變數插值可能漏掉。
這是 soft layer,**不是安全防護** — `deny-dangerous.sh` 才管 hard block。

## 對團隊的影響

這個 PR **不強制**全 atlas 團隊啟用 hook。`.claude/settings.json`
(team-shared) 未被修改;`.claude/settings.local.json` (per-user) 由各
開發者自行決定是否建立。

如果其他開發者想試,直接照「完整設定」建立即可,不需要動 .git-tracked 檔案。
