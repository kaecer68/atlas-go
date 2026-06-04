# CLAUDE.md — atlas-go 規則索引

本檔案僅作為工具進入點。所有專案專屬規則、陷阱與禁令，請直接參考 **AGENTS.md**。
本檔案不重複任何規則，以確保單一權威來源，避免 token 重複計費。

全局規則仍遵循 `~/.claude/CLAUDE.md`。

## 多工具協作指引

本專案同時使用 Claude Code 與 OpenCode。為避免重複載入與矛盾：

- **Claude Code**：優先讀取 `AGENTS.md` 取得完整規則。本檔案 (`CLAUDE.md`) 僅作為進入點，不載入重複內容。
- **OpenCode**：遵循其自身 skill 系統。若需本專案規則，統一引用 `AGENTS.md`，禁止在 skill 定義中複製貼上 AGENTS.md 內容。
- **通用原則**：任何工具載入專案規則時，以 `AGENTS.md` 為唯一權威來源。禁止在 CLAUDE.md、skill 檔案、prompt template 中重複定義。

## Token Efficiency Rules

- **Scoped reads**: 使用精確檔案路徑（如 `web/static/css/main.css`）而非目錄讀取。禁止讀取 `data/`、`.codegraph/`、`.gitnexus/`、`graphify-out/`。
- **/compact between subtasks**: 在獨立子任務間執行 `/compact` 回收 context window。
- **Frontend scope**: CSS/JS 變更完全跳過 impact analysis。僅在 Go backend 變更觸及 3+ 符號時執行 `gitnexus_impact`。
- **Precise file targeting**: 讀取前先用 `glob` 確認確切檔案路徑。避免投機性讀取大檔案。
- **No duplicate rules**: 本檔案刻意不重複 AGENTS.md 規則。單一權威來源。

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (38024 symbols, 88043 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
