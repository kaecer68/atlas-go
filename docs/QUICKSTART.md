# Atlas-Go 快速啟動與 CI 指令

> 從 `AGENTS.md` 遷移（避免根 AGENTS.md 超過 160 行預算）。
> 完整規範階層見 `docs/GUIDELINES_INDEX.md`。

## 快速啟動

```bash
go run ./cmd/atlas                          # HTTP server (port 8080)
go run ./cmd/run-experiment -brief <file>   # 實驗生命週期
go run ./cmd/judge-experiment               # 評判 (auto-discovers latest)
go run ./cmd/promote-baseline               # 升版 (auto-discovers latest accepted)
go run ./cmd/backtest-window -start ... -end ...
```

## CI 指令（修改後必跑）

```bash
test -z "$(gofmt -l .)" && go build ./... && go test ./...
go vet ./... && staticcheck ./...
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total
```

## Git 工作流

```bash
# 1. 從最新 main 建立 feature branch
git checkout main && git pull origin main
git checkout -b feat/<descriptive-name>

# 2. 開發並提交
git add -A && git commit -m "feat(scope): description"

# 3. 推送 + PR
git push -u origin feat/<name>
gh pr create --title "feat(scope): description" --body "..." --base main

# NEVER: git push origin main   ← 絕對禁止
```

分支命名：`feat/<name>` / `fix/<name>` / `refactor/<name>`。
Commit 格式：`type(scope): description`。

### 4. Post-merge cleanup（AI 自動執行，不要等使用者指示）

PR 經 GitHub UI merge 後，工作樹 + branch 仍留在本地。**AI 必須自行清理**：

```bash
# 同步本地 main（否則 `branch -d` 會出 "not merged to HEAD" 警告）
git fetch origin main
git checkout main && git merge --ff-only origin/main

# 刪本地 + 遠端 branch（merge commit 已保留在 origin/main，可放心刪）
git branch -d <merged-branch>
git push origin --delete <merged-branch>

# 若曾在獨立 worktree 開發：先切到其他 worktree，再移除空 worktree
git worktree remove <worktree-path>
```

警告排查：`branch -d` 報 "not yet merged to HEAD" → 本地 main 落後 origin/main，先跑前兩行 fetch + ff-merge。

> **Multi-CLI 並行協議**：[docs/MULTI_CLI_PROTOCOL.md](docs/MULTI_CLI_PROTOCOL.md)
