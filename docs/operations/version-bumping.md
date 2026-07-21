# Version Bumping Workflow

> **Owner**: AI coding agent(本專案完全由 AI 編碼,本文件是 canonical SOP)
> **Introduced**: PR #1062(2026-07-09,VERSION 升級為 single source of truth)
> **對應版本**: v0.0.0.32+

## TL;DR

```bash
make bump-version   # 互動式:提示輸入新版本,寫入 VERSION
make sync-version   # 同步 TARGETS doc 文件內的版本字串
make ci             # 本地驗證(check_versions.sh 會跑)
git add VERSION <changed-docs>
git commit -m "chore(release): bump VERSION to X.Y.Z"
gh pr create --base main ...
```

CI 自動跑 `scripts/ci/check_versions.sh`,TARGETS 文件 drift 會 block merge。

## 完整流程

### 1. 決定 bump 類型(Semver)

| 類型 | 時機 | 範例 |
|------|------|------|
| Patch (`X.Y.Z+1`) | bug fix、純文件、chore、dependency bump | `0.0.0.29 → 0.0.0.30` |
| Minor (`X.Y+1.0`) | 新 feature、新 MCP tool、backward-compatible 變更 | `0.0.0.30 → 0.1.0` |
| Major (`X+1.0.0`) | breaking change(本專案目前無) | — |

### 2. `make bump-version`

互動式 prompt 輸入新版本(例:`0.0.0.30`),寫入 `VERSION` 檔。

不動任何 doc 文件——sync 是分開步驟(給 reviewer 機會看 diff)。

### 3. `make sync-version`

讀 `VERSION`,在 TARGETS 列表內把 `v0.X.Y.Z[+]` 同步成當前版本。

**Regex 設計**:`v0\.[0-9]+\.[0-9]+\.[0-9]+(\+?)` 嚴格匹配 `v`-prefix 版本,**不**匹配 `0.0.0.0` IP wildcard(防止 Python proxy bind address `host="0.0.0.0"` 被誤改)。

### 4. `make ci` 或 `bash scripts/ci/check_versions.sh`

驗證 TARGETS 內所有版本 ref 對齊 VERSION。輸出:
- ✅ All `v0.0.0.30` references in TARGETS files are in sync(通過)
- ❌ drift in `AGENTS.md`: 'v0.0.0.29' (expected 'v0.0.0.30')(失敗,需 `make sync-version`)

### 5. Commit + PR

```bash
git add VERSION <changed-docs>
git commit -m "chore(release): bump VERSION to X.Y.Z"
gh pr create --base main --title "chore(release): bump VERSION to X.Y.Z" --body "..."
```

## TARGETS 文件清單

(sync-version.sh + check_versions.sh 共同維護,**兩個檔案的 `TARGETS` array 必須保持同步**)

| 檔案 | 用途 |
|------|------|
| `AGENTS.md` | 專案 constitution 的版本欄 |
| `internal/AGENTS_INDEX.md` | 模組索引(59 模組) |
| `internal/MATURITY.md` | 模組成熟度對照 |
| `internal/fubonproxy/AGENTS.md` | fubonproxy 模組 |
| `cmd/atlas-mcp/server/AGENTS.md` | MCP server 模組 |

**新增 TARGETS 文件**:同步編輯 `scripts/sync-version.sh` 與 `scripts/ci/check_versions.sh` 的 `TARGETS` array。

## 範圍外(明確 NOT synced,故意不做)

| 類型 | 範例 | 為什麼不動 |
|------|------|-----------|
| CHANGELOG | `CHANGELOG.md` | 手寫歷史,由 release 工具自動產生 |
| 歷史文件 | `docs/archive/`、`.omo/investigations/`、`.omo/audit/` | 版本 ref 是內容的一部分 |
| 規範文件 | `docs/specs/`,`docs/operations/{l2-4-*,tier-boundary}` | 記錄特定 version 的設計決策 |
| 事件文件 | `docs/reference/events/` | 記錄事件首次上線的版本 |
| 測試 fixtures | `*_test.go` 內 `"0.0.0.X"` 字串常數 | test 資料,非文件 |
| IP:port 配置 | `monitoring/otel-collector.yaml` | `0.0.0.0:4318` 是 OTLP endpoint |

如果新加的 doc 不在 TARGETS 也不在 allowlist 內,`make sync-version` 不會動它(預期行為)。

## Known limitations

- **Semantic future-version refs**:`reserved for v0.0.0.32` 之類會被 sync 改成當前 VERSION。需手動 review `git diff` 後修正。
- **Multi-machine VERSION 同步**:`VERSION` 檔在 git,靠 PR 同步;無 lockfile。

## 何時該 bump(決策樹)

```
有 PR 要 merge?
├─ 是 → 這個 PR 包含 release 嗎?
│         ├─ 是(new feature/MCP tool/API breaking)→ 在該 PR 內 bump
│         └─ 否(bug fix/純文件/chore)→ 不 bump,留到下次 release
└─ 否 → 不 bump
```

純文件 / internal cleanup PR(如本文件本身的 PR #1062)**不應該**順便 bump——避免一個 PR 改太多事。

## 相關文件

- PR #1062(設計):https://github.com/kaecer68/atlas-go/pull/1062
- `scripts/sync-version.sh`:實際 regex + TARGETS array
- `scripts/ci/check_versions.sh`:CI check 邏輯
- `Makefile`:`version` / `sync-version` / `bump-version` targets
- AGENTS.md "🔗 文件路由" 段:本文件 reference
