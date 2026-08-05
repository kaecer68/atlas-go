# PR Lifecycle — 從本地修改到 production 驗收的全流程

> **文件角色**：AI agent + 人類 reviewer 共享的 PR 流程規範。取代過去散落在 `AGENTS.md`、各 runbook、Makefile 註解的 PR 紀律片段。
> **配套文件**：`AGENTS.md` 為入口索引（引用本檔），`docs/operations/production-rollout-runbook.md` 為 staging → production 部署的下一階段（不在本檔範圍）。
> **本檔建立背景**：2026-08-05 v3.0 dispatch thread 暴露兩個洞 — (1) PR-F #1457 偷跑只跑 `ci-gate` 沒跑 `ci-full`,production 才暴露 recovery path 只處理 `status=ok` 不處理 `stale`; (2) PR merge 後沒跑 production 驗收就當 done。**這兩個洞不該再發生**。

## 1. Pre-PR — 本地修改 + 驗證

任何 code 變更 MUST 通過:

| 步驟 | 指令 | 目的 | 強制 |
|------|------|------|------|
| 1.1 讀 `atlas-pre-change-protocol` skill | （不適用） | 確認設計意圖,避免淺層 patch | MUST |
| 1.2 同步測試 | 修改 `internal/<x>.go` → 同 commit 改 `internal/<x>_test.go` | 不留「先 commit 功能測試晚點補」 | MUST |
| 1.3 本地 `go test` 紅綠燈驗證 | `go test ./internal/<x>/ -count=1 -race` | 確認測試 fail → fix → pass | MUST |
| 1.4 格式 | `gofmt -w .` | 不留 gofmt diff | MUST |

## 2. PR-Create — 推上 remote + 開 PR

### 2.1 完整 CI（必跑,非可選）

| 階段 | 指令 | 耗時 | 目的 | 強制 |
|------|------|------|------|------|
| 2.1.1 Fast gate | `make ci-gate` | <30s | 快速 sanity check | MUST（最低門檻） |
| 2.1.2 Full gate | `make ci-full` | ~5-8 min | golangci-lint v2.12.2 + staticcheck + go test -race + cmd/atlas 整合 + coverage ≥ 60% + orphan artifact | **MUST** |
| 2.1.3 Result captured in PR body | （人工） | — | 把 `make ci-full` 結果摘要寫進 PR body 的 "Verification" 段 | MUST |

**跳過 `make ci-full` 偷跑 = 偷工**。如必須跳過（如 rebuild 中 / 時間限制）：

- PR body **必須**明確標示 `KNOWN: ci-full not run, will run after rebase` 並 link 到 blocker
- 不可靜默跳過
- blocker 解除後 24 小時內補跑並把結果補進 PR comment

### 2.2 推上 remote

```bash
git push -u origin <branch>
```

### 2.3 開 PR

```bash
gh pr create --title "<type>(<scope>): <subject>" --body-file /tmp/pr-body-<N>.md
```

PR body **MUST 含三段**：
- **Summary** — 修了什麼 / 加了什麼
- **Root Cause** — 為什麼壞（事實為本,不是猜測）
- **Verification** — 跑了什麼 test,結果是什麼（含 ci-full 結果）

## 3. PR-Review — Reviewer 檢查

### 3.1 Reviewer 必看

| 項目 | 標準 |
|------|------|
| Root cause 是事實還是猜測 | MUST 為事實,有 log / source code / 重現步驟佐證 |
| Test 是否真的 fail → fix → pass | 不可只有 happy path |
| 是否回歸既有功能 | 看 diff 影響面 |
| 是否有「東一塊西一塊」的補丁 | 若有,要求 PR 拆成多個 |
| 是否動到 `docs/operations/` 規範文件 | 若是,必須連規範一起梳理,不可補丁式 |

### 3.2 CI 通過條件

- [ ] `make ci-gate` 過
- [ ] `make ci-full` 過（**不可跳過**）
- [ ] GitHub CI 全綠
- [ ] Reviewer approve

## 4. PR-Merge — 合併到 main

### 4.1 Squash-merge

```bash
gh pr merge <N> --squash --delete-branch --admin
```

### 4.2 合併後 git state

```bash
git fetch --prune
git checkout main
git pull --ff-only
git log --oneline -3  # 確認 HEAD 是新 merge
```

### 4.3 刪除本地 branch（multi-cli protocol 規範）

```bash
git branch -d <branch>
```

## 5. Post-Merge — Production 驗收（**最容易漏的步驟**）

> **PR merge ≠ PR done**。合併後需 deploy + 在 production 跑驗證 checklist 才能視為 PR 完成。

### 5.1 Docker rebuild

**只有人類（kaecer）能執行**:

```bash
make rebuild-all
docker compose restart
```

**AI agent 不得執行**（CLAUDE.md Docker 禁令）。

### 5.2 Binaries 對齊檢查

```bash
make check-binaries  # 應顯示 "ALL BINARIES FRESH"
```

若不 fresh,等 docker 重建完成。

### 5.3 Production Verification Checklist（每個 PR 都必跑）

PR author 或 reviewer **MUST 給出 3-5 個 curl 指令**針對該 PR 修的 channel / endpoint。例如:

```bash
# 範例: PR-F #1457 修 crossmarket recovery
curl -s http://127.0.0.1:18080/api/dashboard/channel-health | jq '.channels[] | select(.channel_id=="us10y" or .channel_id=="vix")'
# 預期: status=ok, updated_at = 重建後時間

docker logs atlas-go 2>&1 | grep "recovery: cleared"
# 預期: [CrossMarket] recovery: cleared 10 macro channels (status=ok)
```

### 5.4 Done Criteria

PR 視為完成 **必須**所有三項：

- [ ] `make ci-full` 過（PR-Create 階段）
- [ ] Docker image rebuild + 重啟 + `make check-binaries` ALL FRESH
- [ ] Production verification checklist **全部**通過

若任一不通過,PR 狀態為「**merged-but-unverified**」,需開 follow-up issue 處理。

## 6. 自動化機制（roadmap,未實作）

目前 PR 流程依賴人 + AI 紀律。**未來可加**的自動化:

| 機制 | 工具 | 目的 |
|------|------|------|
| Pre-push hook 強制 `make ci-full` | `scripts/ci/pre-push.sh` | 偷跑時直接擋下,不靠人記 |
| `make verify-production <pr-number>` | Makefile target | 自動跑 production verification checklist + 結果寫進 PR comment |
| Dead link 自動掃 | `.github/workflows/doc-link-check.yml` | 規範文件東一塊西一塊時自動掃出孤立 reference |

## 7. 違反此規範的後果

- **AI agent 跳過 `make ci-full`**:下次 AI session 開工時,`AGENTS.md` L66 仍要求此項,規範違規會在 review 時被抓
- **PR merge 沒跑 production 驗收就當 done**:commit author 需在 24 小時內補做,或開 follow-up issue
- **東一塊西一塊的補丁式規範文件修改**:reviewer 必須 reject,要求作者重新梳理

## 8. 修訂紀錄

| 日期 | 修訂 | 作者 |
|------|------|------|
| 2026-08-05 | 初版建立,因 2026-08-05 v3.0 PR-F #1457 半失敗教訓 | kaecer dispatch + AI agent |
