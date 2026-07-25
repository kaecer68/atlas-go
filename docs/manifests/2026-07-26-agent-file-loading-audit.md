# Audit Manifest: Agent 文件載入鏈審計

> **Audit source**: 使用者從 opencode 遷移到 oh-my-pi，發現文件讀取路徑不一致
> **Goal**: 確保所有 agent 環境（oh-my-pi / opencode / Claude Code）都能讀到完整的專案紀律與流程文件
> **Scope**: 全域 AGENTS.md、repo CLAUDE.md/AGENTS.md、copilot-instructions.md、internal/*/AGENTS.md 的載入鏈、一致性、過時內容
> **Out of scope**: skills 內容審計、CI workflow 審計、module AGENTS.md 內容品質審計
> **Created**: 2026-07-26
> **Status**: in-progress

---

## Invariant Tracker

| ID | Severity | Problem | Root Cause | Files to Change | Acceptance Criteria | Status |
|----|----------|---------|------------|-----------------|---------------------|--------|
| F01 | 🔴 blocker | oh-my-pi 不載入 repo `CLAUDE.md` 與 `AGENTS.md`，AI 缺少分支紀律、PR 驗證清單、高頻陷阱表 | oh-my-pi 讀 `copilot-instructions.md` 而非 repo `CLAUDE.md`；兩者之間沒有 import 鏈 | `.github/copilot-instructions.md` | 加 `@CLAUDE.md` 後，下次 oh-my-pi session 出現 repo CLAUDE.md + AGENTS.md 內容 | ✅ done |
| F02 | 🔴 blocker | `~/.agents/AGENTS.md` 與 `~/.config/opencode/AGENTS.md` 為兩份分歧副本；CI preflight gate 只在一份存在 | opencode 時期建立 `~/.config/opencode/AGENTS.md`，oh-my-pi 時期建立 `~/.agents/AGENTS.md`，兩者獨立演化 | `~/.config/opencode/AGENTS.md`（改為 symlink） | `diff ~/.agents/AGENTS.md ~/.config/opencode/AGENTS.md` 無輸出 | pending |
| F03 | 🟡 important | `AGENTS.md:5` 與 `cmd/atlas-mcp/server/AGENTS.md:4` 寫死參考 `~/.config/opencode/AGENTS.md`，oh-my-pi 環境下為 broken link | opencode 時期 hardcoded 路徑，未考慮多環境 | `AGENTS.md:5`, `cmd/atlas-mcp/server/AGENTS.md:4` | 改為相對參考或 `~/.agents/AGENTS.md`；grep 無殘留 `~/.config/opencode/AGENTS.md` | pending |
| F04 | 🟡 important | Tool count 四處不一致：AGENTS.md 說「113+」、server/AGENTS.md 說「106-108+4」、AGENTS_INDEX.md 說「116」、CI assert 112-114 | 無單一權威來源；各文件獨立更新 | `AGENTS.md:57`, `cmd/atlas-mcp/server/AGENTS.md:30`, `internal/AGENTS_INDEX.md:71` | 全部指向 `docs/reference/tool-catalog.md` 為單一權威來源，不再各自聲明數字 | pending |
| F05 | 🟢 nice-to-have | AGENTS.md 版本資訊過時（Wave 11+、v0.0.0.37+、Last updated 2026-07-12） | 常規 drift | `AGENTS.md:10-12` | 更新到當前 wave/version | pending |
| F06 | 🟢 nice-to-have | `copilot-instructions.md:31-50` 與 `go-core.instructions.md:72-92` build/test 指令重疊 | copilot-instructions.md 最初設計為獨立檔案，後來 go-core.instructions.md 加入相同內容 | `copilot-instructions.md` | copilot-instructions.md 刪除重疊指令區塊，改為指向 go-core.instructions.md | pending |
| F07 | 🟢 nice-to-have | AGENTS.md 157 行，逼近 hard limit 160 行 | 自然增長 | `AGENTS.md` | 降至 155 行以下，或提高 limit（需同步更新 quality.yml agents-md-lines job） | pending |

---

## Phase Tracker

### Phase A — Audit (read-only)

Status: ✅ complete. 證據見本 conversation 的完整審計輸出。

### Phase B — Plan

| Task | ID | Plan |
|------|----|------|
| 修復載入鏈 | F01 | `copilot-instructions.md` 頂部加 `@CLAUDE.md`。CLAUDE.md 已有 `@AGENTS.md`，形成完整鏈：copilot-instructions → CLAUDE.md → AGENTS.md |
| 消滅雙副本 | F02 | `~/.config/opencode/AGENTS.md` 改為 symlink → `~/.agents/AGENTS.md`。先 diff 確認無意外差異後執行 |
| 修復 broken refs | F03 | `AGENTS.md:5` 改 `~/.agents/AGENTS.md`；`cmd/atlas-mcp/server/AGENTS.md:4` 同樣。或統一改用相對路徑 `~/.agents/AGENTS.md` |
| 統一 tool count | F04 | 三處刪除各自數字，改為 "詳見 `docs/reference/tool-catalog.md`"。CI assert 保留不變 |
| 更新版本資訊 | F05 | 確認當前 wave/version 後更新 `AGENTS.md:10-12` |
| 去重指令 | F06 | `copilot-instructions.md` 刪除 §Build & Test Quick Reference，改為指向 `go-core.instructions.md` |
| 防膨脹 | F07 | 評估 AGENTS.md 哪些內容可外移（如 ACI 工具入口可移到 copilot-instructions.md），或提高 hard limit 到 180 |

### Phase C — Implement

| Task | ID | Status | Commit |
|------|----|--------|--------|
| F01: 加 `@CLAUDE.md` import | F01 | ✅ done | pending commit |
| F02-F07: 待使用者決定優先序後執行 | — | pending | — |

### Phase D — Close Out

| Task | ID | Status |
|------|----|--------|
| Commit F01 | F01 | pending |
| Push + PR | — | pending |
| 下次 oh-my-pi session 驗證載入鏈 | — | pending |

---

## Backlog

| ID | Problem | Discovery Time | Notes |
|----|---------|---------------|-------|
| — | `~/.claude/CLAUDE.md` 第 1 行 `@~/.agents/AGENTS.md` 後緊接 `# 核心規則` 而非原始內容 —— 確認這是 import 後的正常行為 | 2026-07-26 | 可能只是 oh-my-pi 解析後的前置內容 |
| — | `internal/live/AGENTS.md` 參考 `~/.config/opencode/AGENTS.md`？需全量 grep 確認 | 2026-07-26 | F03 的延伸，grep 全 repo `~/.config/opencode` |

---

## Commit Discipline

- Format: `fix(manifest): #F01 add @CLAUDE.md import to copilot-instructions`
- One commit per ID where practical
- PR body must reference this manifest

---

## Session-End State

- **Done this session**: F01（載入鏈修復）
- **Remaining**: F02–F07
- **Next action**: commit F01 → push → PR；使用者決定 F02–F07 優先序後分批處理
- **Uncommitted code**: yes（`.github/copilot-instructions.md`）
- **Branch**: main（F01 為單行改動，可安全直接 commit 到 main 或走分支）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-26 | 1.0 | 初始審計 manifest，7 項發現 | AI agent (oh-my-pi) |
