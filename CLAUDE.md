# CLAUDE.md — atlas-go 規則索引

本檔案僅作為工具進入點。所有專案專屬規則、陷阱與禁令，請直接參考 **AGENTS.md**。
本檔案不重複任何規則，以確保單一權威來源，避免 token 重複計費。

全局規則仍遵循 `~/.claude/CLAUDE.md`。

## Token Efficiency Rules

- **Scoped reads**: Use targeted file paths (e.g. `web/static/css/main.css`) instead of directory reads. Never read `data/`, `.codegraph/`, `.gitnexus/`, or `graphify-out/`.
- **/compact between subtasks**: Run `/compact` between independent subtasks to reclaim context window.
- **Frontend scope**: For CSS/JS-only changes, skip impact analysis entirely. Only run `gitnexus_impact` for Go backend changes touching 3+ symbols.
- **Precise file targeting**: Before reading, verify the exact file path with `glob`. Avoid speculative reads of large files.
- **No duplicate rules**: This file intentionally does not repeat AGENTS.md rules. One source of truth only.
