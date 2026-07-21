# 新工作區起步 SOP

> **目的**：當 AI agent 在新工作區（或重新開啟會話）時，依序執行以下步驟，確保**快速理解專案現況**、**避免污染 `.omo/`**、**找到正確的規劃文件**。
> **適用對象**：所有 AI agent（Claude、opencode、其他 CLI）。
> **完成時間**：3-5 分鐘。

---

## 步驟 0：建立 Worktree（隔離環境）

**為何要 worktree**：每個任務的工作目錄應該隔離，避免污染主 branch 的 `.omo/` 內容。

```bash
# 在 worktree 中開新工作（推薦）
git worktree add ../atlas-wt-<task-slug> -b <type>/<task-slug>
cd ../atlas-wt-<task-slug>

# 或在主目錄直接開（僅限短任務）
cd /path/to/atlas
git checkout -b <type>/<task-slug>
```

> **注意**：`.omo/` 是 gitignored，但 **worktree 之間會共用同一個 working tree 的 `.omo/`**——這代表同一份 `.omo/briefs/roadmap.md` 在兩個 worktree 看到的內容可能不同（如果其中一邊修改）。建議**：每個 worktree 開新會話時，先做步驟 5 的 `.omo/` 結構檢查**。

---

## 步驟 1：讀規範（必讀，不超過 3 分鐘）

```bash
# 1.1 必讀精簡版（先看這兩個）
sed -n '1,80p' docs/documentation-standard.md
sed -n '1,80p' docs/documentation-map.md

# 1.2 模組陷阱（修改 Go 程式碼前）
cat AGENTS.md

# 1.3 架構與環境
cat docs/architecture.md
cat docs/environment.md
```

**為何這樣讀**：
- `documentation-standard.md` 與 `documentation-map.md` 決定**檔案該放哪**
- `AGENTS.md` 列出**跨模組陷阱**（避免常見錯誤）
- `architecture.md` 給出**系統全局觀**
- `environment.md` 給出**外部依賴狀態**（如 LLM provider 設定）

---

## 步驟 2：確認 git 狀態（檢查遺留污染）

```bash
# 2.1 確認 branch 與 working tree 乾淨
git status
# 若有 untracked 或 modified，先確認是不是前任 agent 留下的
# 不明檔案 → 移到 .omo/scratch/ 或刪除

# 2.2 確認 main 是最新
git fetch origin main
git log --oneline origin/main -3

# 2.3 若在 worktree，確認分支正確
git branch --show-current
```

**常見污染**：
- `.DS_Store`（macOS 自動產生）→ 加入 `.gitignore` 或忽略
- `*.backup-YYYY-MM-DD`（先前備份）→ 直接刪除
- `*.test` 編譯產物 → 已 `.gitignore`，若在 staged 狀態需要 reset

---

## 步驟 3：確認 `.omo/` 結構合規

**這是關鍵步驟——AI 自由生成子目錄是污染主因**。

```bash
# 3.1 列出 .omo/ 內容
ls -la .omo/

# 3.2 對照白名單（從 documentation-standard.md § .omo/ 子目錄白名單）
# 白名單內（合規）：
#   briefs/ plans/ evidence/ traces/ notepads/ handoffs/ workspaces/
#   run-continuation/ phaseN/ wave-N/ maps/ boulder.json

# 3.3 識別違規項目
WHITELIST="briefs plans evidence traces notepads handoffs workspaces run-continuation maps boulder.json"
for entry in .omo/*; do
    name=$(basename "$entry")
    if [[ ! " $WHITELIST " =~ " $name " ]]; then
        echo "WARN: .omo/$name is NOT in whitelist — review and clean"
    fi
done

# 3.4 確認 evidence/ 內沒有子目錄（禁用巢狀）
find .omo/evidence -mindepth 1 -type d 2>/dev/null | head -5
# 若有輸出 → 違規，請整理為 .omo/evidence/<topic>-task-N.md 形式
```

**違規處理決策樹**：

```
.omo/ 內有違規項目
│
├─ 是先前 AI 留下的污染（不再需要）？
│   ├─ 是 → rm -rf <violator>
│   └─ 否、是有用內容？
│       └─ 移到對應的白名單子目錄（rename to compliant name）
```

---

## 步驟 4：找規劃文件

```bash
# 4.1 長壽規劃（roadmap、跨模組設計）
ls .omo/briefs/
# 預期：roadmap.md, ALERT_SYSTEM_REdesign.md 等

# 4.2 短期 PR 待辦
ls .omo/plans/
# 範例：2026-05-29-etf-nav-finmind.md（未實作）

# 4.3 驗證報告（之前的 task 證明）
ls .omo/evidence/
# 範例：f4-scope-fidelity.md, task-N-*.md

# 4.4 決策筆記
ls .omo/notepads/
# 範例：<topic>/learnings.md

# 4.5 若要找特定主題，使用 grep
grep -rl "<topic>" .omo/ | head -10
```

**若找不到對應的規劃文件**：
1. 可能是 brief 尚未建立 → 評估是否需要新 brief
2. 可能是 brief 升級到 `docs/` 了 → `ls docs/`
3. 可能是 brief 已被清理 → 從 git reflog 找：`git log --all --diff-filter=D --summary | grep <name>`

---

## 步驟 5：決定這次任務的 `.omo/` 規劃

**每個任務都應在 `.omo/` 留下規劃痕跡**，避免「AI 做了什麼沒人知道」。

```bash
# 5.1 若是新任務，建立對應的 brief
# 短任務（單一 PR）：
echo "# <task-slug> Brief" > .omo/plans/$(date +%Y-%m-%d)-<task-slug>.md

# 長任務（多個 PR 或跨模組）：
echo "# <topic> Brief" > .omo/briefs/<topic>.md

# 5.2 若是已存在的 brief，做更新而非新建
ls .omo/briefs/<topic>.md
# 找到後直接編輯更新

# 5.3 命名檢查
# ✅ 合規：.omo/plans/2026-06-26-fix-llm-router.md
# ✅ 合規：.omo/briefs/roadmap.md
# ❌ 違規：.omo/plans/fix-thing.md（缺日期前綴）
# ❌ 違規：.omo/briefs/MyNewPlan.md（大寫、空格）
# ❌ 違規：.omo/audit/foo.md（不在白名單）
```

---

## 步驟 6：完成工作後的清理

```bash
# 6.1 PR merge 前
ls .omo/plans/ .omo/evidence/
# 對應 PR 的 plan/evidence → 在 merge 後刪除

# 6.2 短期驗證報告（task-N-*.txt）保留價值低
# 除非有 audit/regulatory 需要，否則 merge 後刪除

# 6.3 長期 brief（.omo/briefs/）保留
# 除非已升級到 docs/，否則不要刪除

# 6.4 確認 .gitignore 仍然擋住
grep -E "(\.omo|\.opencode)" .gitignore
```

---

## 常見錯誤

### 1. AI 創建了 `audit/` 子目錄

**症狀**：`ls .omo/` 出現 `audit/` 但白名單沒有
**原因**：AI 看歷史 `docs/audit/` 結構（已移除）後想在 `.omo/` 開類似目錄
**修正**：用 `evidence/` 或 `briefs/` 替代，不要開新子目錄

### 2. AI 創建了 `evidence/decision-chain-v2/` 子目錄

**症狀**：`find .omo/evidence -type d` 顯示子目錄
**原因**：AI 把 task 當作 topic 開了子目錄
**修正**：evidence 禁止子目錄，改用 `evidence/task-N-decision-chain-v2.md`

### 3. AI 留下了散落檔案 `.omo/foo.md`

**症狀**：`ls .omo/` 出現 `.md` 檔案而非目錄
**原因**：AI 把 brief 寫成單一檔案而非放進 `briefs/`
**修正**：移到 `.omo/briefs/foo.md`，或合併到既有 brief

### 4. 命名混雜中英文

**症狀**：`etf-nav-finmind-接入計劃.md`
**原因**：AI 看到中文用戶用中文寫 slug
**修正**：用全英文 slug + 在檔案內容中可使用中文。命名規範：`^[a-z0-9-]+\.md$`

### 5. 忘記清理 .DS_Store

**症狀**：`ls .omo/` 出現 `.DS_Store`
**原因**：macOS Finder 自動產生
**修正**：`rm .omo/.DS_Store`（若已在 `.gitignore` 會自動忽略，但工作目錄仍會有）

---

## 自我檢查清單（工作完成時打勾）

- [ ] `.omo/` 內無白名單外的子目錄
- [ ] `.omo/evidence/` 內無子目錄
- [ ] `.omo/` 內無散落 `.md` 檔案（除 `boulder.json`）
- [ ] 命名全小寫、英文、`-` 分隔
- [ ] 短期 plan/evidence 在 PR merge 後刪除
- [ ] 長期 brief 升級到 `docs/` 時同步刪除 `.omo/` 副本
- [ ] `git status` 確認無意外 staged 檔案

---

## 相關文件

- `docs/documentation-standard.md` — 完整文件存放規範
- `docs/documentation-map.md` — 完整文件地圖
- `AGENTS.md` — 跨模組陷阱與模組索引
- `docs/architecture.md` — 系統架構
- `docs/environment.md` — 外部依賴狀態
