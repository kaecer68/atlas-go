# AI Prompt 檔案管理政策

> 規範 atlas-go 專案中 **AI 生成的 prompt 檔案**（bootstrap、執行指令、skill frontmatter 等）應如何 commit 與追蹤，避免 local-only 漂移脫離主流工具鏈。

## 核心原則

**AI 生成的 prompt 檔案應 commit，不得以「local-only」為由留在工作目錄而未追蹤。**

理由：
- 主流工具鏈會演進（`graphify` → `GitNexus`），prompt 中的工具引用必須隨之更新
- 散落在每位開發者 `/tmp/` 或 `~/.opencode/prompts/` 的 local prompt 沒有 review 路徑，工具引用過期也沒人發現
- PR #690 的契機正是因 `graphify` 工具退役後，三份 Wave 8/9 prompt 仍引用過時工具指令，花時間考古才找到原始檔案

## 適用範圍

| 類型 | 位置 | 範例 |
|------|------|------|
| **OpenCode CLI 啟動 prompt** | `.opencode/prompts/*.md` | `<wave>-bootstrap.md`（Wave 8/9 範本已於 2026-08-17 移除） |
| **Skill frontmatter / 內文** | `.claude/skills/**/*.md` | `atlas-pre-change-protocol/SKILL.md` |
| **Experiment prompt** | `prompts/agents/*.md`、`prompts/experiments/` | 任何 agent 或 experiment 的 prompt 模板 |
| **Session handoff prompt** | `.opencode/handoffs/*.md` | 多 CLI session 之間的銜接 prompt |

## 追蹤規則

1. **AI prompt 檔案必須進 git**，即使是「一次性」或「個人 draft」
2. **新增 prompt 檔案時一併 commit**（不要 `git status` 看到一堆 `Untracked files` 才想到）
3. **prompt 內引用的工具/指令若退役**，必須在下次 commit 該 prompt 時一併更新
4. **「歷史 artifact」標記**：若 prompt 已不再使用（例如 Wave 收尾後），在檔案頂部加 `⏪ 歷史狀態` banner 說明淘汰時間與取代工具，不要刪除（保留為 audit trail）

## .gitignore 設定

`.opencode/` 目錄預設為 gitignored（避免 local config 污染），但以下子目錄應 **un-ignore 並追蹤**：

```gitignore
/.opencode/*
!/.opencode/commands/      # OpenCode CLI 指令
!/.opencode/prompts/       # AI 啟動 prompt（本文主題）
```

> ⚠️ 重要：`!/.opencode/prompts/**` 不可行 — git 規則在目錄被 `/.opencode/*` match 後無法 re-include 子檔案。必須 un-ignore **整個目錄**（`!/.opencode/prompts/`）才能讓子檔案可追蹤。

## 檢查工具

提交前用以下指令確認沒有遺漏的 prompt 檔案：

```bash
# 列出 .opencode/ 下所有 untracked 的 prompt 檔案
git ls-files --others --exclude-standard .opencode/prompts/

# 列出所有引用過時工具的 prompt（依當前工具鏈調整 pattern）
grep -rln "graphify|sven1103" .opencode/prompts/ .omo/audit/\|grep -rln "graphify|sven1103" .opencode/prompts/ .omo/audit/
```

## 違規情境處理

發現 untracked AI prompt 檔案時：

1. **若內容有效**：直接 `git add`，commit 並寫明用途
2. **若內容過期**：在檔案頂部加 `⏪ 歷史狀態` banner 標註，commit 為「archive: <description>」
3. **若是純個人 scratch**：複製到 `.opencode/scratch/`（若該目錄存在）或本地 `~/.config/opencode/scratch/`，不 commit

## 為什麼這條規則存在

PR #690（2026-06-24）清理了 graphify 工具鏈。過程中發現：

- `.opencode/prompts/wave-8-bootstrap.md` 等 3 個檔案因為 local-only 沒進 git（2026-08-17 已連同 wave-9-bootstrap 一併移除，wave 8/9 早已結束）
- 內容仍引用 `graphify` 等已退役工具
- 為更新引用，需在 working tree 重新建立檔案並 commit，耗時且容易出錯

未來若新增 AI 工具（例如實驗性的 `gitnexus_explore`），所有 prompt 引用都應隨工具演進更新——前提是檔案在 git 裡、能被 CI 與 reviewer 看見。

## 相關文件

- `.opencode/prompts/`：實際 prompt 檔案位置
- `.claude/SKILLS-MAP.md`：Claude/OpenCode skill 索引
- `docs/operations-playbook.md`：日常運維流程（包含 prompt 維護節奏）
- `docs/quickstart.md`：開發者快速上手（提示開發者檢查 prompt 工具引用是否仍有效）
