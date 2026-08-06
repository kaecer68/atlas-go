## Summary

新增一個 **soft reminder 層** PreToolUse hook,在 AI agent 試圖接觸
`internal/` 或 `cmd/` hot-path Go 程式時,主動注入 ~150 token 的
ACI routing 提示,鼓勵 agent 在讀/改/搜之前先跑 `gitnexus_query`、
`gitnexus_impact`、`codebase-memory_explore` 等既有 ACI 工具。

補強 `atlas-pre-change-protocol` skill(被動式,agent 要自己 load
才生效)— hook 在 **PreToolUse 時機自動觸發**,curation 內容
直接抄 skill / SKILLS-MAP.md / traps.md 的既有 routing 規則,
**無新 convention 引入**。

## Root Cause

`atlas-pre-change-protocol` 是 8 步強制 skill,但**被動**:
- Agent 在 mid-task 改 hot-path module 時常跳過 skill load
- 結果產生 `agent-skips-aci-routing` footgun:重複造輪子、
  補丁式亂加、亂猜測已有/未有功能

現有的 deterministic guardrail 都管**輸出**(binary / push / secrets),
**沒人管輸入** — agent 第一次 Read 進 hot-path module 沒有任何 reminder
提醒它「等等,先想 blast radius」。

## Fix

| 檔案 | 變更 | 角色 |
|------|------|------|
| `.agent-hooks/aci-read-prompt.sh` | **新增** 6.7KB executable | hook 主程式,讀 stdin JSON → 判斷 hot-path Go → 注入 `additionalContext` |
| `.agent-hooks/README.md` | **新增** 3KB | 工具總覽、安裝、依賴、與 `deny-dangerous.sh` 關係 |
| `.agent-hooks/install.sh` | **修改** +1 chmod line + jq 依賴檢查 + settings.local.json 提示 | 自動安裝 |
| `.gitignore` | **修改** 加 `.claude/settings.local.json` 規則 | 防止本機 hook 設定被誤 commit(原本缺,發現時已修) |
| `.claude/settings.local.json` | 本機建立(不 commit) | 註冊 `PreToolUse` hook 用,本機生效 |
| `.claude/agent-memory/footguns/agent-skips-aci-routing.md` | **新增** | 文件化反模式 + 預防機制 |
| `.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md` | **新增** | 設計決策:為何走 local hook + soft prompt 不走 hard block |
| `docs/operations/aci-hook-usage.md` | **新增** 4.7KB | 給其他開發者在本機啟用 hook 的完整 SOP |

## Verification

### 1. Hook 行為驗證 — 13 個情境(全過)

| # | 情境 | 預期 | 實際 |
|---|------|------|-----|
| 1 | `Read` `internal/orchestrator/agent_loop.go` | 觸發 | ✅ 觸發 |
| 2 | `Read` `admin_web/foo.js` | 不觸發 | ✅ 不觸發 |
| 3 | `Read` `docs/architecture.md` | 不觸發 | ✅ 不觸發 |
| 4 | 同 session 再讀同檔 | 去重 | ✅ 去重 |
| 5 | `Edit` `internal/risk/gate.go` | 觸發 | ✅ 觸發 |
| 6 | `Bash grep -r "foo" internal/orchestrator/`(目錄) | 觸發 | ✅ 觸發 |
| 7 | `Bash go test ./internal/orchestrator/...` | 不觸發 | ✅ 不觸發 |
| 8 | `Grep` 對 `admin_web/` | 不觸發 | ✅ 不觸發 |
| 9 | 不同 session 讀同檔 | 觸發 | ✅ 觸發 |
| 10 | `Bash rg "foo" cmd/atlas/` | 觸發 | ✅ 觸發 |
| 11 | `Bash find internal -name "*.go"` | 觸發 | ✅ 觸發 |
| 12 | `Bash ls internal/` | 不觸發 | ✅ 不觸發 |
| 13 | `Read .env`(非 Go) | 不觸發 | ✅ 不觸發 |

### 2. `make ci-gate` — 全通過

```
✅ ci-gate passed — 可以 push
  gofmt ✅
  go build ✅
  go vet ✅
  go generate drift check ✅
  10 CI scripts ✅ (0 failed)
```

耗時 25.19s。

### 3. `.gitignore` 改動安全確認

- **新增**:`.claude/settings.local.json`(per-user hook 註冊檔,防被誤 commit)
- **移除**:第 200 行的 `.gstack/`(無前綴,相對路徑)
- **保留**:第 167 行的 `/.gstack/`(root-anchored,**仍在生效**)
- **影響**:`.gstack/` 的 ignore 行為**無實質差異**(原本就有 root-anchored 那條)
- reviewer 若對 `-.gstack/` 困惑,這條已說明:冗餘刪除,主動保留

### 4. Pre-existing 工作區狀態(與本 PR 無關)

`docs/archive/sector-prediction-observation-log.md` (M) 與
`cmd/cron-quote-backfill/data/` (??) 為開 session 前就存在的狀態,
**不納入本 PR**。

## Decision Trade-offs

| 選擇 | 替代 | 為何選這條 |
|------|------|----------|
| **Soft prompt**(additionalContext 注入) | Hard block(`permissionDecision: deny`) | hard block 會讓 agent 無法讀 source code,直接與目標矛盾。soft prompt 留下 audit signal。 |
| **Per-user `.local.json`** | Team-shared `.settings.json` | 個人 workflow 補強,非團隊規範。team 採納需先驗證有效。 |
| **Curate from existing**(`atlas-pre-change-protocol` / SKILLS-MAP) | Invent new routing rules | 避免 convention 膨脹;hook 只是「記住要做的事」不是「發明新事」 |
| **`.gstack/` 冗餘刪除** | revert 整個 .gitignore edit | 實質無差異,revert 會丟掉真正的 .claude/settings.local.json 防護 |

## Risk

- **jq 依賴** — `brew install jq`(`install.sh` 會檢查並提示)
- **Bash regex 較窄** — `bash -c "..."` 包裝或變數插值可能漏。**可接受**:
  - 這是 soft layer,不是 security 防護
  - `deny-dangerous.sh` 是 hard block 防線
- **未強迫全團隊啟用** — 這是 per-user 補強,等驗證有效後再考慮 team 推廣

## Reference

- 設計 plan: `.omo/plans/2026-08-06-aci-pretooluse-prompt.md` (13.6KB)
- 決策紀錄: `.claude/agent-memory/decisions/aci-enforcement-via-local-hook.md` (6.4KB)
- Footgun: `.claude/agent-memory/footguns/agent-skips-aci-routing.md` (4.2KB)
- 既有 routing 規則出處: `atlas-pre-change-protocol` SKILL.md Steps 0/1/1.5
- Claude Code Hook 規範: code.claude.com/docs/en/hooks § PreToolUse decision control
