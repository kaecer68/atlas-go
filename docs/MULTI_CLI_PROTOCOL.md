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
- **sven1103-agent/opencode-worktree-plugin**([GitHub](https://github.com/sven1103-agent/opencode-worktree-plugin),npm package: `@sven1103/opencode-worktree-workflow`):**AI 層 worktree 生命週期工具**(v0.6.3,**目前 latest stable**)。Plugin 暴露兩個 tools,**沒有任何 hook**:
  - `worktree_prepare(title)` — 為當前 session 綁定一個 worktree(branch = `wt/<slug>` 來自 sidecar,自動 `git worktree add` + 切到該 worktree;`title` 至少 3 字元,會被 normalize 成 slug)
  - `worktree_cleanup(mode?, raw?, selectors?)` — 收尾 worktree(`mode` = `preview`(預設)只列不刪 / `apply` 真的刪除;`raw` 顯示原始 entry 物件;`selectors` 過濾要清理的對象)

  ⚠️ **v0.6.3 沒有 `tool.execute.before` hook、不自動重寫 bash/glob/grep/edit 任何工具路徑、不攔截 AI escape worktree、不會在 session 啟動時自動建 worktree**。每個 CLI session 必須在開頭**主動呼叫** `worktree_prepare` 才能拿到隔離;忘了呼叫 = 跟以前一樣 bundling,plugin 不會救你。1.0.0-alpha 線(`@1.0.0-alpha.4`)才有 hook 行為,但未驗證,不要在 production 用。

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

  **Slash commands**(`/wt-new` / `/wt-clean`):**v0.6.3 沒有附帶**。1.0.0-alpha 線的 release asset 可能會有,但**未驗證可用**。目前要 trigger plugin 必須在 chat 直接呼叫 `worktree_prepare` tool 給對應參數,沒有快捷指令。

雙組合分工(對應 v0.6.3 實際行為):
- **worktrunk** = 人類層(操作員手動切換、看佈局、收尾)
- **sven1103** = AI 層 worktree 生命週期(`worktree_prepare` / `worktree_cleanup` 兩個 tools)。**不是 hook、不是自動防護**,需要 CLI session 開頭主動 call `worktree_prepare(title)` 才有隔離;plugin 不會自己動手
- 兩者分工:worktrunk = 人類層手動操作;sven1103 = AI 層在 session 內 explicit 觸發

CLI session 開機流程(v0.6.3):**仍然需要 bootstrap** —— 在 chat 內 call `worktree_prepare(title="<session-purpose>")` 才會建立 worktree;**忘了 call = 跟以前一樣 bundling**,plugin 不會救你。若需要「人類手動給下個 CLI 開新 worktree」,直接 `wt switch -c <branch>` 即可,worktrunk 自動切換目錄。

## sidecar 設定範例

`.opencode/worktree-workflow.json`(per-repo,版本控管)— 完整 JSON 不在 doc 內重複(避免雙方不同步),見 `.opencode/worktree-workflow.json` 實際內容。

關鍵欄位說明(plugin `src/index.js` 消費這些 keys):
- `remote`: 用於偵測 default branch,必填
- `branchPrefix`: **plugin 預設 `wt/`**,建立 branch 命名為 `wt/<slug>`(slug 來自 title,**不是**用現有 branch 名)
- `baseBranch`: 用於 worktree 建立 + cleanup 的 base,預設 = remote default branch
- `worktreeRoot`: 支援 `$REPO` / `$ROOT` / `$ROOT_PARENT` 三種 template(plugin `path.resolve` 時展開)
- `protectedBranches`: 必含 `baseBranch`,plugin 自動加入 `defaultBranch` + `baseBranch` + 此清單(`src/index.js:710`)
- `cleanupMode`: `"preview"`(預設)或 `"apply"`,決定 `wt-clean` 預設是否實際刪除

## 驗證 sven1103 真的有效(v0.6.3)

裝好 sven1103 後,跑下面步驟確認 plugin 真的載入 + tools 可呼叫。注意:**v0.6.3 沒有 path rewriting hook**,所以「阻擋 sibling escape」「阻擋絕對路徑」這類 adversarial test 在 v0.6.3 **不適用** —— 那些需要 1.0.0-alpha 線的 hook 行為,目前未驗證。v0.6.3 只能驗證:

1. **Plugin 載入檢查**:在 CLI session 內呼叫 `worktree_prepare(title="verify-plugin-load")`,回傳 `result.path` 應指向 sidecar 設定的 worktree 根目錄下的 `verify-plugin-load/`(e.g. `../atlas-wt/verify-plugin-load`),branch = `wt/verify-plugin-load`。
2. **Cleanup preview**:呼叫 `worktree_cleanup(mode="preview")`,回傳應列出當前 session 的 worktree 為「可清理」狀態;**預設模式只預覽不刪**。若沒列代表 plugin 沒成功綁定 worktree。
3. **Subagent dispatch 隔離(REQUIRES MANUAL VERIFICATION)**:從 worktree A 用 `task` 委派子任務給另一個 agent。**sven1103 v0.6.3 完全不參與 parent→child session 傳遞,subagent 繼承的是 parent dispatch 時的 CWD**,不是 plugin 行為。請直接實測確認,**不要相信 AI 自報「我在 worktree 內」**。

## v0.6.3 已知限制(Known Limitations)

- **無 hook**:不攔截任何 tool 呼叫,bash/glob/grep/read/edit 都不會被路徑重寫
- **無自動隔離**:session 啟動時不會自動建 worktree,必須在 chat 內**主動** call `worktree_prepare(title)`
- **無 slash commands**:`/wt-new` / `/wt-clean` 在 v0.6.3 不存在,要在 chat 直接呼叫 tool
- **無 subagent 控制**:不會修改 parent→child dispatch 的 CWD 傳遞
- **無 sibling escape 防護**:bash 命令可自由讀 sibling worktree 的檔案,plugin 不會擋
- **1.0.0-alpha 線**(`@1.0.0-alpha.4`)預期會有 hook + 自動隔離 + slash commands,**未驗證可用**,**不要在 production 用**

## 為什麼要分文件

root `AGENTS.md` 套用 160 行硬限制(`quality.yml:agents-md-lines` check),任何模組特定或多 CLI 規則應下沉到 `docs/` 或 `internal/<mod>/AGENTS.md`,由 root 用 1 行 reference 指過去即可。

本檔屬於「操作程序 / playbook」類,依 `AGENTS.md` 內容歸屬規則存放於 `docs/`。
