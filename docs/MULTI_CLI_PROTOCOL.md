# Multi-CLI 並行協議

> 防止多個 CLI session 互相覆蓋 working tree 的標準做法。
> 抽出至獨立文件，以避免 root `AGENTS.md` 超過 160 行硬限制。

## 規則

- **新規則**：開新 CLI session 必須先 `git worktree add ../<branch>`，或在已安裝 worktrunk 的環境用 `wt switch -c <branch>`
- **Main worktree**（`/Users/kaecer/workspace/atlas`）只給 primary CLI 或 stable 視圖
- **禁止**：兩個 CLI 同時在 main worktree 編輯相同檔案
- **Worktree 路徑**：建立在 main worktree 的 sibling（`../<slug>`），**branch 命名為 `wt/<slug>` 或 `chore/<slug>` 等任務前綴**
- **結束時**：PR merge → branch 自動刪除 → `git worktree prune`
- **進場前**：必跑 `git worktree list` 確認當前佈局

## 工具鏈（2026-06 修訂）

atlas-go 不再依賴任何 AI 層 worktree 隔離 plugin（sven1103/opencode-worktree-workflow 已於 v0.0.0.8 退役）。人類操作員使用下列任一工具即可：

- **原生 git**：`git worktree add ../<branch> <base>` 開新 worktree，無額外依賴
- **Worktrunk**（可選，`brew install worktrunk && wt config shell install`）：人類層便捷路徑 — `wt switch -c <branch>` 一行開 worktree 並自動 cd 進去；`wt list` 看當前佈局；`wt remove <branch>` 收尾。**路徑 convention**：worktree 建立在 main worktree 的 sibling（`$ROOT_PARENT/<slug>`）

## AI session 開機流程

**主動開 worktree**：在 chat 開頭用 `wt switch -c <branch>`（worktrunk）或 `git worktree add ../<branch> main` 切到隔離的 worktree，**不要直接在 main worktree 開工**。忘了切換 = 跟其他 CLI 共享 main worktree，會互相覆蓋。

無 hook 自動防護、無 plugin 隔離：每個 CLI session 必須自行決定是否在 worktree 內執行。

## 為什麼要分文件

root `AGENTS.md` 套用 160 行硬限制（`quality.yml:agents-md-lines` check），任何模組特定或多 CLI 規則應下沉到 `docs/` 或 `internal/<mod>/AGENTS.md`，由 root 用 1 行 reference 指過去即可。

本檔屬於「操作程序 / playbook」類，依 `AGENTS.md` 內容歸屬規則存放於 `docs/`。
