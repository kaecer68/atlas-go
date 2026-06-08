# CLAUDE.md — atlas-go 規則索引

本檔案僅作為工具進入點。所有專案專屬規則、陷阱與禁令，請直接參考 **AGENTS.md**。
本檔案不重複任何規則，以確保單一權威來源，避免 token 重複計費。

全局規則仍遵循 `~/.claude/CLAUDE.md`。

## 快速路由

| 需求 | 文件 |
|------|------|
| 完整模組索引（34 個） | `internal/AGENTS_INDEX.md` |
| 模組成熟度對照 | `internal/MATURITY.md` |
| 跨模組陷阱詳細參考 | `docs/TRAPS.md` |
| 根規則與全域禁令 | `AGENTS.md` |
| 架構憲法 | `internal/apigateway/CONSTITUTION.md` |

## Token Efficiency Rules

- **Scoped reads**: Use targeted file paths (e.g. `web/static/css/main.css`) instead of directory reads. Never read `data/`, `.codegraph/`, `.gitnexus/`, or `graphify-out/`.
- **/compact between subtasks**: Run `/compact` between independent subtasks to reclaim context window.
- **Frontend scope**: For CSS/JS-only changes, skip impact analysis entirely. Only run `gitnexus_impact` for Go backend changes touching 3+ symbols.
- **Precise file targeting**: Before reading, verify the exact file path with `glob`. Avoid speculative reads of large files.
- **No duplicate rules**: This file intentionally does not repeat AGENTS.md rules. One source of truth only.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (41276 symbols, 125852 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
