# Atlas-Go 快速啟動與 CI 指令

> 從 `AGENTS.md` 遷移（避免根 AGENTS.md 超過 160 行預算）。
> 完整規範階層見 `docs/reference/guidelines-index.md`。

## 快速啟動

```bash
go run ./cmd/atlas                          # HTTP server (port 18080)
go run ./cmd/run-experiment -brief <file>   # 實驗生命週期
go run ./cmd/judge-experiment               # 評判 (auto-discovers latest)
go run ./cmd/promote-baseline               # 升版 (auto-discovers latest accepted)
go run ./cmd/backtest-window -start ... -end ...
```

---

## 系統初始化順序（bootstrap）

`internal/bootstrap` 初始化流程**不可顛倒**：`InitMetrics() → InitDatabase() → InitStores() → InitRepository() → InitTaskManager()`，接著 `RegisterDashboardRoutes()` 依序註冊各模組路由，最後 `ApplyBrokerConfig()` 驗證 live mode。Repository 依賴 Stores，Stores 依賴 Ledger — 顛倒會 panic。

| 陷阱 | 說明 |
|------|------|
| Broker dry-run 預設 | 未覆寫時 mode=`dry-run`、adapter=`guarded`、signer=`placeholder`；需同時設 `AllowLiveBroker` + `AllowRealSigner` 顯式 opt-in |
| DATABASE_URL 為空時 graceful | `InitDatabase` 回傳 `nil, nil`，後續邏輯需判斷 nil |
| ATLAS_STORE_BACKEND 直接讀 env | `InitStores` 繞過 Gateway，僅限啟動階段使用（祖父條款） |

---

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

**5. Planning artifacts 清理**（PR merge 後必做）：

`.omo/plans/*.md`、`.omo/research/*.md`、`.omo/handoff/*.md` 這些規劃文件**完成就刪掉**,不要留在 working tree 污染未來 context:

```bash
# 刪掉所有過期規劃 .md(慎用,只看 done 的)
rm .omo/plans/*.md .omo/research/*.md .omo/handoff/*.md

# 如果內容有長期保存價值,**不要直接留整份 .md**:
#   - 應先萃取到正式 docs 位置,例如 docs/specs/<feature>.md 或 docs/operations/<feature>-runbook.md
#   - 或若屬於已歸檔的 feature,移到 docs/archive/<feature>-<date>.md
#   - .omo/ 是 gitignored 的「working area」,不是 archive — 不要拿來長期保存
```

警告排查：`branch -d` 報 "not yet merged to HEAD" → 本地 main 落後 origin/main，先跑前兩行 fetch + ff-merge。

---

## v0.0.0.37 新增路由快速驗證

啟動後可立即 curl 驗證 7 個 Wave 11 投資核心框架 endpoint（不需要認證）：

```bash
# 資金流向 + 共振（無認證）
curl -s http://localhost:18080/api/capital-flow/summary | jq .

# 事件日曆 + 5 日預測（無認證）
curl -s http://localhost:18080/api/events/calendar | jq '.events | length'
curl -s http://localhost:18080/api/events/prediction | jq '.predictions | length'

# 每日報告（premium tier 才有完整內容，未登入回 free tier 簡化版）
curl -s http://localhost:18080/api/reports/latest | jq '.tier, .summary'

# 認證（測試用，新用戶自動 7 天 premium 試用）
curl -X POST http://localhost:18080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}' | jq .

# tier-gated 推薦（需 JWT cookie）
curl -s http://localhost:18080/api/recommendations -b "token=<JWT>" | jq .
```

完整 WA-8xx workflow 對應見 [`processes.yaml`](reference/processes.yaml) §9。

---

> **Multi-CLI 並行協議**：[multi-cli-protocol.md](multi-cli-protocol.md)
