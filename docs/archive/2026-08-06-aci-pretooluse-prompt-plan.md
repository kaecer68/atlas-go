# Plan: ACI PreToolUse Prompt — 防止 agent 跳過架構查詢

**建立日期**: 2026-08-06
**作者**: AI session (kaecer 主導)
**Branch**: `feat/20260806-aci-pretooluse-prompt`
**PR**: [#1464](https://github.com/kaecer68/atlas-go/pull/1464)
**狀態**: Plan reviewed, awaiting PR #1464 merge
**詳細 PR body 對照**: [`2026-08-06-aci-hook-pr-1464-body.md`](2026-08-06-aci-hook-pr-1464-body.md)

---

## 1. 問題陳述(背景)

atlas-go 是 35k symbols / 104k edges / 2,059 files / 60 modules / 300+ execution flows 的大型 Go codebase。AI agent 在 mid-task 改 hot-path module 時,常見三種反模式:

1. **重複造輪子** — 新增 function/type/module 沒先查 `gitnexus_query`,產生平行實作 → `atlas-pre-change-protocol` Step 0 失敗
2. **補丁式亂加** — 直接 Read 程式就改,沒跑 `gitnexus_impact` 看 blast radius → Step 1 失敗
3. **亂猜測** — 回答事實性問題不先跑 ACI 查 docs/code → 沿襲 `assumes-without-reading-docs` footgun

### 現狀盤點

| 制度 | 形式 | 覆蓋率 |
|------|------|-------|
| `atlas-pre-change-protocol` SKILL | MUST-use,8 步 | ⚠️ 被動,agent 有時跳過 |
| `.agent-hooks/deny-dangerous.sh` | hard block | ✅ 危險指令 |
| `.githooks/pre-push` | 阻擋 push main | ✅ |
| `scripts/session-start.sh` | binary freshness gate | ✅ |
| `.claude/agent-memory/footguns/` | 文件化 | ⚠️ agent 不一定 load |

**缺口**:`atlas-pre-change-protocol` 是被動型 skill,**沒有 hook 在 Read/Edit 前強制提醒**。

## 2. 設計決策

### 2.1 為何走 soft prompt + `.local.json` + 不走 hard block

| 選擇 | 替代 | 為何選這條 |
|------|------|----------|
| **Soft prompt**(additionalContext) | Hard block(`permissionDecision: deny`) | hard block 會讓 agent 無法讀 source code,直接矛盾。hard block 保留給「做了回不去」的操作(同 `deny-dangerous.sh` 原則) |
| **Per-user `.local.json`** | Team-shared `.settings.json` | 個人 workflow 補強,非團隊規範。`.claude/settings.json` 已承載 team-shared config(目前是 `SessionStart` binary-freshness hook) |
| **Curate from existing protocol** | Invent new routing | 避免 convention 膨脹;hook 是「記住要做的事」不是「發明新事」 |

### 2.2 觸發條件(scope)

| Tool | 條件 |
|------|------|
| Read/Edit/Write | `*.go` 路徑在 `internal/` 或 `cmd/` |
| Grep | `path` 包含 `internal/` 或 `cmd/` |
| Bash | `grep`/`rg`/`find` 命令 + `internal/`/`cmd/` 路徑 |
| 其他 | 不觸發 |

### 2.3 去重與常駐

- Session ID 從 stdin JSON 拿
- 記到 `${XDG_CACHE_HOME:-$HOME/.cache}/atlas-aci-prompted/${session_id}.list`
- 同一 session 同一 (tool, file_path) 只提示一次
- 500 行 cap,超過 rotate 為 `.list.1`

### 2.4 Graceful degradation

- 缺 `jq` 時靜默退出 0(不觸發、不 crash session)
- 與 `scripts/session-start.sh` 既有模式一致

## 3. 實作清單

| 檔案 | 變更 | commit? |
|------|------|---------|
| `.agent-hooks/aci-read-prompt.sh` | 新增 6.7KB hook 主程式 | ✅ |
| `.agent-hooks/README.md` | 新增 3KB 工具總覽 | ✅ |
| `.agent-hooks/install.sh` | +chmod +jq 檢查 +啟用提示 | ✅ |
| `.gitignore` | 加 `.claude/settings.local.json` | ✅ |
| `.claude/settings.local.json` | 本機註冊 hook | ❌ gitignored |
| `.claude/agent-memory/footguns/agent-skips-aci-routing.md` | 反模式 + 預防 | ✅ |
| `.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md` | 設計決策紀錄 | ✅ |
| `docs/operations/aci-hook-usage.md` | 4.7KB 啟用 SOP | ✅ |

## 4. 驗證

### 4.1 Hook 行為 13 個情境(全過)

| # | 情境 | 預期 | 實際 |
|---|------|------|-----|
| 1 | Read `internal/orchestrator/agent_loop.go` | 觸發 | ✅ |
| 2 | Read `admin_web/foo.js` | 不觸發 | ✅ |
| 3 | Read `docs/architecture.md` | 不觸發 | ✅ |
| 4 | 同 session 再讀同檔 | 去重 | ✅ |
| 5 | Edit `internal/risk/gate.go` | 觸發 | ✅ |
| 6 | `grep -r "foo" internal/orchestrator/`(目錄) | 觸發 | ✅ |
| 7 | `go test ./internal/orchestrator/...` | 不觸發 | ✅ |
| 8 | Grep 對 `admin_web/` | 不觸發 | ✅ |
| 9 | 不同 session 讀同檔 | 觸發 | ✅ |
| 10 | `rg "foo" cmd/atlas/` | 觸發 | ✅ |
| 11 | `find internal -name "*.go"` | 觸發 | ✅ |
| 12 | `ls internal/` | 不觸發 | ✅ |
| 13 | Read `.env` | 不觸發 | ✅ |

### 4.2 make ci-gate — 10/10 scripts pass, 25.19s

```
✅ ci-gate passed — 可以 push
  gofmt ✅
  go build ✅
  go vet ✅
  go generate drift check ✅
  10 CI scripts ✅
```

### 4.3 Pre-push hook

- `make ci-gate` 自動重跑(25s)
- 7 個檔案差異,不是空 push,允許推

## 5. 為何不修改 `docs/multi-cli-protocol.md`

評估:multi-cli-protocol 講的是 multi-CLI session 隔離(worktree、PR merge 後自動刪分支),與 per-user hook 關連不大。`docs/operations/aci-hook-usage.md` 已涵蓋完整說明。改 multi-cli-protocol 是冗餘,**沒做**。

## 6. 為何不裝 code-review-graph(背景)

最初 user 想裝 code-review-graph 來省 token。盤點後:

- atlas 已有等價工具:`codegraph_explore` / `codebase-memory_explore` / `gitnexus_impact` / `atlas-mcp` 116 個 tools
- CRG 對 Go 的 execution-flow detection 偏弱(已知 issue,JS/Go 還需 work)
- 安裝會污染嚴格管控的 atlas(`.githooks/pre-commit` 已有自寫阻擋 binary hook,與 CRG 的 hook 設計互踩)
- 社群證據:Towards AI 第三方深度評測指「for a 40-file side project, the graph took longer to build than the bug it was supposed to help you find」

**結論:不安裝,改用既有 ACI 工具 + 新 hook 補強 routing compliance**。

## 7. 留底位置(.omo/ → docs/archive/ 轉移)

原本 plan 與 PR body 留在 `.omo/plans/`,這是 gitignored 區。為符合
`multi-cli-protocol.md § Post-merge cleanup` 的歸檔慣例,移到
`docs/archive/2026-08-06-*` 兩份檔(本文 + PR body 副本)。

## 8. 參考

- `atlas-pre-change-protocol` SKILL.md
- `.claude/SKILLS-MAP.md` § AI Coding 技能使用流程
- `docs/reference/traps.md` § 跨模組陷阱「造輪子前先搜尋既有 infrastructure」
- `.claude/agent-memory/footguns/assumes-without-reading-docs.md`
- `.agent-hooks/deny-dangerous.sh`(設計 pattern 對齊)
- Claude Code Hooks 規範:`code.claude.com/docs/en/hooks`
- `docs/multi-cli-protocol.md`(本地 hook 與 post-merge cleanup 慣例)
