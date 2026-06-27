# Git Tool Cache Policy (atlas-go)

> 此文件說明哪些檔案/目錄是**工具自動產生的 cache**,**永遠不該 commit**。
> 判斷原則:rebuild cost < commit cost — 重新生成一個檔案的成本,若比把它 commit 進 git 後維護一致性的成本低,就不該 commit。

## 已知 Tool Cache 清單

| 路徑 | 來源 | 重建指令 | 備註 |
|------|------|---------|------|
| `.gitnexus/` (含 `meta.json`, `lbug`, `lbug.wal`, `parse-cache/`) | GitNexus indexer | `npx gitnexus analyze --skip-agents-md` | 335MB LadybugDB binary + metadata snapshot |
| `.opencode/package.json` | opencode CLI 內部狀態 | (opencode 自動 sync) | 2 個 size,本來就無害 |
| `admin_web/static/` 與 `client_web/static/` 的 dedup 前殘檔 (pages/、components/、services/、css/、__tests__/、shared/components/、bootstrap-utils.js、names.js、shared/app-utils.js 等) | web split dedup (`1635acb2`) 刪除後殘留在磁碟的副本 | `rm -rf ...` (見下方 alias) | 權威來源在 `shared_web/static/js/`;這些是 git 已 rm 的舊版檔案 |

## 判斷原則 (Decision Heuristic)

如果一個檔案符合以下**任一**條件,就不該 commit:

- [ ] **可由命令完全重新生成** — 跑 `npx gitnexus analyze` 或 `opencode sync` 即可還原
- [ ] **包含機器生成的 hash 指向當前 HEAD** — 例如 `meta.json` 的 `lastCommit: "56868db8..."`
- [ ] **包含時間戳** — 例如 `meta.json` 的 `indexedAt: "2026-06-25T08:17:59.657Z"`
- [ ] **已被 `.gitignore` 排除** — 卻因 `git add -f` 或 `git rm --cached` 反向操作而 staged

## 為何這很重要 (Why this matters)

1. **每次 commit 都是決策負擔** — contributor 必須區分「我的 code 變更」vs「工具副作用」vs「索引 stale 快照」
2. **CI 浪費時間** — 假設的「doc consistency check」若建立在 24+ 小時 stale snapshot 上,只比對到昨天的狀態,沒有真實意義
3. **污染 commit history** — 6 個 90 天內的 commit 涉及 `.gitnexus/meta.json`,若該檔本身是 derived artifact,這些 commits 純粹是 noise

## 出現時的處置

### 對 contributor (個人 SOP)

執行 `git cleanup-tools` (global git alias,定義於 `~/.gitconfig`):

```bash
git cleanup-tools   # 還原已知 cache + 清除 dedup 殘檔 + 顯示 status
```

> 此 alias 屬於個人 machine 全域配置,**不在本 repo 內**。其他 contributor 看不到,
> 也不該預期他們有。請在本機 `~/.gitconfig` 自行配置。

#### 完整 alias 定義

```bash
git config --global alias.cleanup-tools '!f() {
  echo "→ Resetting known tool caches...";
  git checkout -- .gitnexus/ 2>/dev/null;
  git checkout -- .opencode/package.json 2>/dev/null;
  echo "→ Removing dedup stale checkout remnants...";
  rm -rf \
    admin_web/static/js/pages/ \
    admin_web/static/js/components/ \
    admin_web/static/js/services/ \
    admin_web/static/js/__tests__/ \
    admin_web/static/js/shared/components/ \
    admin_web/static/css/ \
    admin_web/tests/ \
    client_web/static/js/pages/ \
    client_web/static/js/components/ \
    client_web/static/js/services/ \
    client_web/static/js/__tests__/ \
    client_web/static/js/shared/components/ \
    client_web/static/css/ \
    client_web/tests/ \
    2>/dev/null;
  rm -f \
    admin_web/static/js/bootstrap-utils.js \
    admin_web/static/js/names.js \
    admin_web/static/js/shared/app-utils.js \
    admin_web/static/js/shared/constants.js \
    admin_web/static/js/shared/fetch-error.js \
    admin_web/static/js/shared/utils.js \
    admin_web/static/narrative-dashboard.html \
    admin_web/static/trading-dashboard.html \
    client_web/static/js/bootstrap-utils.js \
    client_web/static/js/names.js \
    client_web/static/js/shared/app-utils.js \
    client_web/static/js/shared/constants.js \
    client_web/static/js/shared/fetch-error.js \
    client_web/static/js/shared/utils.js \
    client_web/static/narrative-dashboard.html \
    client_web/static/trading-dashboard.html \
    2>/dev/null;
  echo "→ Current status:";
  git status --short;
}; f'

### 對 AI agent (本檔的存在意義)

當 AI agent 接手 atlas 工作並看到 `git status` 顯示 `.gitnexus/meta.json` 或 `.opencode/package.json` 修改時:

1. **不要 commit** 這些檔案 (即使 `git add .` 會把它們加進去)
2. **建議** contributor 執行 `git cleanup-tools` (或 `git checkout -- .gitnexus/ .opencode/package.json`)
3. **不要寫 script** 把這些檔案 commit 進 PR — 是 anti-pattern

### 對 CI (基礎設施層級)

- `.gitignore` 已將這兩個路徑完整排除
- 對應的 CI check (e.g. `scripts/verify-gitnexus-stats.sh`) 若仍存在,應是 no-op 或重構為跑 `npx gitnexus analyze` 而非讀 committed snapshot

## 升級 SOP (新 tool 出現時)

1. **觀察** 新工具在 `git status` 出現修改 (例: `.foo/bar.json`)
2. **驗證** 該檔案符合「判斷原則」任一條件:
   - `git log --all --oneline -- <file>` — 確認不是用戶工作
   - `file <file>` 或 hexdump 看看內容性質
   - 若該檔有 hash / timestamp / 指向 HEAD,幾乎確定是 cache
3. **修復**:
   - (a) 加進 `.gitignore` (新路徑)
   - (b) `git rm --cached <file>` (若已 tracked)
   - (c) 把新路徑加入 `git cleanup-tools` alias (個人 config)
4. **文件化**: 把新路徑加入本檔「已知清單」表
5. **開 PR** 含上述 `.gitignore` + 本檔變更

## 變更歷史

- **2026-06-26** 初版建立 (PR follow-up to #749)
  - 依據 4 階段重組的 evidence-based 結論:
    1. `verify-gitnexus-stats.sh` 是 permanent no-op (零 markdown 匹配 grep pattern)
    2. `.gitnexus/meta.json` 是 derived artifact (從 binary lbug 衍生)
    3. codebase-memory 已提供 GitNexus 75% 能力
  - 詳見 PR #749 commit log 與 docs/TOOLS.md 修訂
