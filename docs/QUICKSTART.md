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

> **Multi-CLI 並行協議**：[docs/MULTI_CLI_PROTOCOL.md](docs/MULTI_CLI_PROTOCOL.md)
