# Multi-CLI 並行協議

> 防止多個 CLI session 互相覆蓋 working tree 的標準做法。
> 抽出至獨立文件，以避免 root `AGENTS.md` 超過 160 行硬限制。

## 規則

- **新規則**：開新 CLI session 必須先 `git worktree add ../<branch>` 或用 `wt switch -c <branch>`(worktrunk)
- **Main worktree**（`/Users/kaecer/workspace/atlas`）只給 primary CLI 或 stable 視圖
- **禁止**：兩個 CLI 同時在 main worktree 編輯相同檔案
- **Worktree 路徑**:sven1103 自動建立在 main worktree 的 sibling(`../<slug>`),**branch 命名為 `wt/<slug>`**(plugin 行為:slug 來自 worktree title,**不是**用現有 branch 名)
- **結束時**:PR merge → branch 自動刪除 → `git worktree prune`
- **進場前**:必跑 `git worktree list` 確認當前佈局

## 工具鏈(2026-06-12 修訂)

開新 CLI session 的隔離保障進化成 **worktrunk + sven1103 雙層防護**：

- **Worktrunk**(`brew install worktrunk && wt config shell install`):**人類層**快樂路徑 — 操作員手動快速切換、看佈局。`wt switch -c <branch>` 一行開 worktree 並自動 cd 進去(前提是 shell init 已執行),`wt list` 看當前佈局(自帶 status 標記 `!` / `+` / `^`),`wt remove <branch>` 收尾。**路徑 convention**:與 sven1103 統一,worktree 建立在 main worktree 的 sibling(`$ROOT_PARENT/<slug>`)。
- **sven1103-agent/opencode-worktree-plugin**([GitHub](https://github.com/sven1103-agent/opencode-worktree-plugin),npm package: `@sven1103/opencode-worktree-workflow`):**AI 層根本性 bundling 防護**。其 `tool.execute.before` hook 自動 **重寫所有 tool 呼叫的路徑** —— bash 自動注入 workdir、glob/grep 自動注入 path、已知路徑從 repo 根重寫到綁定 worktree。封鎖無法安全重寫的呼叫。**這就是當初抱怨「AI bundling 別人工作樹」的源頭解 —— AI 連 escape 自己的 worktree 都不可能**。

  **安裝**(per-user,`~/.config/opencode/opencode.json`,**pin 固定版本**避免 schema drift):
  ```json
  "plugin": [
    "...",
    "@sven1103/opencode-worktree-workflow@0.6.3"
  ]
  ```

  **npm 依賴**(per-project,確保 plugin 跟 package.json 同步,**版本需與 plugin 一致**):
  ```sh
  npm install -D @sven1103/opencode-worktree-workflow@0.6.3
  ```

  **Slash commands**(per-project,裝好後可打 `/wt-new` / `/wt-clean`):
  ```sh
  mkdir -p .opencode/commands
  curl -fsSL https://github.com/sven1103-agent/opencode-worktree-plugin/releases/latest/download/wt-new.md  -o .opencode/commands/wt-new.md
  curl -fsSL https://github.com/sven1103-agent/opencode-worktree-plugin/releases/latest/download/wt-clean.md -o .opencode/commands/wt-clean.md
  ```

雙組合分工:
- **worktrunk** = 人類層(操作員手動切換、看佈局、收尾)
- **sven1103** = AI 層(強制路徑隔離、不讓 AI 越界)
- 兩者互補:worktrunk 管 worktree 生命週期,sven1103 管 AI 在 worktree 內的工具呼叫邊界

CLI session 標準開機流程已不再需要手動 bootstrap —— **裝好 sven1103 後,新 session 自動被鎖在 worktree 內,人跟 AI 都不用額外動作**。若需要「手動給下個 CLI 開新 worktree」,直接 `wt switch -c <branch>` 即可,worktrunk 自動切換目錄。

## sidecar 設定範例

`.opencode/worktree-workflow.json`(per-repo,版本控管)— 完整 JSON 不在 doc 內重複(避免雙方不同步),見 `.opencode/worktree-workflow.json` 實際內容。

關鍵欄位說明(plugin `src/index.js` 消費這些 keys):
- `remote`: 用於偵測 default branch,必填
- `branchPrefix`: **plugin 預設 `wt/`**,建立 branch 命名為 `wt/<slug>`(slug 來自 title,**不是**用現有 branch 名)
- `baseBranch`: 用於 worktree 建立 + cleanup 的 base,預設 = remote default branch
- `worktreeRoot`: 支援 `$REPO` / `$ROOT` / `$ROOT_PARENT` 三種 template(plugin `path.resolve` 時展開)
- `protectedBranches`: 必含 `baseBranch`,plugin 自動加入 `defaultBranch` + `baseBranch` + 此清單(`src/index.js:710`)
- `cleanupMode`: `"preview"`(預設)或 `"apply"`,決定 `wt-clean` 預設是否實際刪除

## 驗證 sven1103 真的有效

裝好 sven1103 後,跑下面 3 個 adversarial test 確認隔離真的生效:

1. **Sibling escape 阻擋**:在 worktree A 內跑 `bash: 'ls ../<worktree-B>/README.md'`,應被 rewrite 成 worktree A 內的路徑,或直接 blocked。
2. **絕對路徑阻擋**:在 worktree 內跑 `bash: 'cat /Users/kaecer/workspace/atlas/main-file.txt'`,應被 rewrite 或 blocked。
3. **Subagent dispatch 隔離 (REQUIRES MANUAL VERIFICATION, 假設未經測試)**:從 worktree A 用 `task` 委派子任務給另一個 agent。**sven1103 是 `tool.execute.before` hook,只重寫 tool 呼叫路徑,不會修改 parent→child session 傳遞**。子任務繼承的是 parent dispatch 時的 CWD,不是 plugin 行為。**若 parent CWD 是 worktree A,child 預設也會在 worktree A 跑**,但這是 session inheritance,不是 plugin 提供。請直接實測確認,**不要相信 AI 自報「我在 worktree 內」**。

任何一個失敗 → plugin 沒生效,**不要相信 AI 報告的「我看到的是 worktree 內的檔案」**。

## 為什麼要分文件

root `AGENTS.md` 套用 160 行硬限制(`quality.yml:agents-md-lines` check),任何模組特定或多 CLI 規則應下沉到 `docs/` 或 `internal/<mod>/AGENTS.md`,由 root 用 1 行 reference 指過去即可。

本檔屬於「操作程序 / playbook」類,依 `AGENTS.md` 內容歸屬規則存放於 `docs/`。
