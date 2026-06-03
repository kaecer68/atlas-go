# atlas-go 行為契約

> 語言：所有回覆、分析、程式碼註解使用**繁體中文**。
> 全局 `~/.claude/CLAUDE.md` 的核心規則與工具規則在此全部適用。

## 專案身分
- Go 1.25.0，模組 `github.com/kaecer68/atlas-go`，台股模擬投資研究系統
- 無 Makefile，使用原生 Go 工具鏈
- GitNexus repo: `atlas-go`

## 修改前強制協議
修改任何程式碼前，執行 `.claude/skills/atlas-pre-change-protocol/SKILL.md` 的 7 步驟檢查清單。**禁止跳過。**

## Git 規則
- 分支命名：`feat/<name>` / `fix/<name>` / `refactor/<name>`
- 提交前必須通過：`go build ./...` + `go test ./...` + `test -z "$(gofmt -l .)"` + `staticcheck ./...`
- commit 前確認 staging area：`git diff --cached --stat`（pre-commit hook 可能自動變更 staged files）

## 高危陷阱（會導致靜默錯誤，CI 抓不到）

| 陷阱 | 後果 | 預防 |
|------|------|------|
| 重複使用 mutable `[]Recommendation` 跨 simulation run | 資料污染，無報錯 | 每次 run 重建或深拷貝 |
| `Save()` 覆寫 `parameters.json` | raw JSON 欄位（如 `calibration_timestamp`）靜默遺失 | 校準器必須同時寫入 Go struct 與 raw JSON 兩種格式 |
| JSON tag 大小寫不對齊 | API unmarshal 靜默失敗，欄位永遠為 nil | 所有 struct JSON tag 為 snake_case；API parsing struct 必須對齊 |
| `ScreeningCriteria` 過濾 | 標的在進 executor 前被靜默移除，無日誌 | 修改門檻前先跑 `go test ./internal/screener/...` |
| Darwinian 權重夾制 `[0.3, 2.5]` | 超界值靜默正規化，不報錯 | 不可假設超界值會傳播 |

## 架構禁令（CI 強制，違反會拒絕 PR）

- 禁止 optimizer 降級為線性加權 → 違反 `.omo/CONSTITUTION.md` 第一條
- 禁止繞過 `BackgroundTaskManager` 直接啟動背景任務
- 禁止繞過 `ParametersConfig` 在業務邏輯中硬編碼參數
- 禁止繞過 `marketdata.Provider` 直接建立 HTTP client
- 禁止跨層洩漏：domain 型別留在 `internal/domain`，協調邏輯留在 `internal/orchestrator`
- 禁止引入全域可變狀態做執行期協調
- 禁止本地測試時啟用 `-allow-live-broker` 或 `-allow-real-signor`
- 新增/刪除/改名 FactorType → 必須同步 7 個位置（CI `factor-integrity` job 檢查）
- 新增 `internal/` 模組 → 必須有 `doc.go` 含 `// Maturity: <tier>`，並更新 `internal/MATURITY.md`

## 資源導航

| 需求 | 路徑 |
|------|------|
| 專案全貌、建置指令、架構細節 | `AGENTS.md` |
| 架構圖與分層說明 | `docs/architecture.md` |
| 修改前 7 步驟檢查清單 | `.claude/skills/atlas-pre-change-protocol/SKILL.md` |
| 規範衝突時的最終仲裁 | `docs/GUIDELINES_INDEX.md` |
| 憲法級強制規範 | `internal/apigateway/CONSTITUTION.md` |
| 深度憲法（optimizer/portfolio/risk） | `.omo/CONSTITUTION.md` |
| 模組特有陷阱 | `internal/*/AGENTS.md` |
| 實驗安全守則 | `.github/instructions/experiments-guardrails.instructions.md` |
| Live trading 邊界 | `.github/instructions/live-trading.guardrails.instructions.md` |
| Go 編碼規則 | `.github/instructions/go-core.instructions.md` |
| 資料架構權威文件 | `docs/DATA_ARCHITECTURE.md` |
| 技能地圖入口（39 技能） | `.claude/SKILLS-MAP.md` |

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (37909 symbols, 87446 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/atlas-go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/atlas-go/clusters` | All functional areas |
| `gitnexus://repo/atlas-go/processes` | All execution flows |
| `gitnexus://repo/atlas-go/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
