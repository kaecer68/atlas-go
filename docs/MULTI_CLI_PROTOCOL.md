# Multi-CLI 並行協議

> 防止多個 CLI session 互相覆蓋 working tree 的標準做法。
> 從 [PR #496](https://github.com/kaecer68/atlas-go/pull/496) 抽出至獨立文件，以避免 root `AGENTS.md` 超過 160 行硬限制（PR #495 rebase 後觸發）。

## 規則

- **新規則**：開新 CLI session 必須先 `git worktree add ../atlas-<task>`
- **Main worktree**（`/Users/kaecer/workspace/atlas`）只給 primary CLI 或 stable 視圖
- **禁止**：兩個 CLI 同時在 main worktree 編輯相同檔案
- **Worktree 命名**：`../atlas-<branch>` 或 `<short-task>`（例：`../atlas-multi-cli-protocol`）
- **結束時**：PR merge → branch 自動刪除 → `git worktree prune`
- **進場前**：必跑 `git worktree list` 確認當前佈局

## 為什麼要分文件

root `AGENTS.md` 套用 160 行硬限制（`quality.yml:agents-md-lines` check），任何模組特定或多 CLI 規則應下沉到 `docs/` 或 `internal/<mod>/AGENTS.md`，由 root 用 1 行 reference 指過去即可。

本檔屬於「操作程序 / playbook」類，依 `AGENTS.md` 內容歸屬規則存放於 `docs/`。
