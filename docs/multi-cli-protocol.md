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

## Post-merge cleanup checklist（AI 自動執行）

PR 經 GitHub UI merge 後，**AI 必須自行執行以下 5 步**，不要等使用者指示：

1. `git fetch origin main && git checkout main && git merge --ff-only origin/main`
2. `git branch -d <merged-branch>`（若本地 main 已 ff，無 "not merged to HEAD" 警告）
3. `git push origin --delete <merged-branch>`
4. 若使用獨立 worktree：`git worktree remove <path>`
5. **Planning artifacts 清理**：`rm .omo/plans/*.md .omo/research/*.md .omo/handoff/*.md`（若該 PR 對應的規劃 .md 已 ship）。**有長期保存價值的內容不要留整份 .md** — 先萃取到 `docs/specs/`、`docs/operations/` 等正式位置,或歸檔到 `docs/archive/<feature>-<date>.md`。`.omo/` 是 gitignored working area,不是 archive。

> 本清單不含任何 docker／rebuild 操作；docker 部署由 kaecer 在主 worktree 手動執行（見 `~/.agents/AGENTS.md` § Docker／正式服務禁令）。

完整 SOP 見 `docs/quickstart.md` § Git 工作流 §4。批次清理（超過 5 個
stale branch）見 `.omo/branch-hygiene/`（內部，gitignored）。

## 工具鏈（2026-06 修訂）

atlas-go 不再依賴任何 AI 層 worktree 隔離 plugin（sven1103/opencode-worktree-workflow 已於 v0.0.0.8 退役）。人類操作員使用下列任一工具即可：

- **原生 git**：`git worktree add ../<branch> <base>` 開新 worktree，無額外依賴
- **Worktrunk**（可選，`brew install worktrunk && wt config shell install`）：人類層便捷路徑 — `wt switch -c <branch>` 一行開 worktree 並自動 cd 進去；`wt list` 看當前佈局；`wt remove <branch>` 收尾。**路徑 convention**：worktree 建立在 main worktree 的 sibling（`$ROOT_PARENT/<slug>`）

### 推薦的 AI 層 worktree 整合：kdcokenny/opencode-worktree

若要在 opencode CLI session 內自動建立 worktree 並同步檔案/啟動 terminal，專案使用 [kdcokenny/opencode-worktree](https://github.com/kdcokenny/opencode-worktree)（透過 [ocx](https://github.com/kdcokenny/ocx) 安裝管理）。

**安裝步驟**（per-user，一次即可）：

```bash
# 1. 安裝 ocx CLI（已於 v0.0.0.8 起推薦使用 brew 或 npm global）
brew install kdcokenny/ocx/ocx    # 或：npm install -g ocx

# 2. 在 repo root 初始化 ocx（會建立 .opencode/ocx.jsonc + .opencode/.gitignore）
ocx init

# 3. 安裝 worktree plugin（會更新 .opencode/package.json + 建立 .opencode/plugins/）
ocx add kdco/worktree --from https://registry.kdco.dev
```

**Repo 端**：`ocx init` 與 `ocx add` 會自動建立/更新以下 tracked 檔案：

- `.opencode/ocx.jsonc`：ocx 專案 config（registries）
- `.opencode/package.json`：ocx 管理的 dep manifest（包含 `kdco-primitives` + `kdco/worktree`）
- `.opencode/worktree.jsonc`：plugin-specific config（已預先建立，調整 `sync.copyFiles` / `hooks.postCreate` 以適配專案）

**未追蹤**（per-user 安裝產物，由 `.opencode/.gitignore` 排除）：

- `.opencode/plugins/<plugin>/`：ocx 安裝的 plugin 程式碼（類比 `node_modules/`）
- `.opencode/node_modules/`：plugin 依賴

**用法**：plugin 暴露兩個 tool — `worktree_create(branch, baseBranch?)` 與 `worktree_delete(reason)`。詳細契約見 [kdcokenny/opencode-worktree README](https://github.com/kdcokenny/opencode-worktree#usage)。

**注意**：此 plugin **不是 npm 套件**，**不要**用 `npm install @kdcokenny/opencode-worktree` 或加入 root `package.json`。所有安裝由 `ocx` 管理。

## AI session 開機流程

**主動開 worktree**：在 chat 開頭用 `wt switch -c <branch>`（worktrunk）或 `git worktree add ../<branch> main` 切到隔離的 worktree，**不要直接在 main worktree 開工**。忘了切換 = 跟其他 CLI 共享 main worktree，會互相覆蓋。

無 hook 自動防護、無 plugin 隔離：每個 CLI session 必須自行決定是否在 worktree 內執行。

## 為什麼要分文件

root `AGENTS.md` 套用 160 行硬限制（`quality.yml:agents-md-lines` check），任何模組特定或多 CLI 規則應下沉到 `docs/` 或 `internal/<mod>/AGENTS.md`，由 root 用 1 行 reference 指過去即可。

本檔屬於「操作程序 / playbook」類，依 `AGENTS.md` 內容歸屬規則存放於 `docs/`。
