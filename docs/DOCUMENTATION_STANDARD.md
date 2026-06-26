# 文件存放規範 (Documentation Standard)

> 取代 AGENTS.md §「內容歸屬規則」中模糊部分。明定每種文件的歸屬位置與命名規範。
> 建立日期：2026-06-26（PR #749）

## 原則

1. **`docs/`** — 規範性、權威性、reference / playbook / spec / audit 文件，給人類閱讀
2. **`.omo/`** — AI agent 的 ephemeral working dir，**`.gitignore` 排除**（`/.omo/` 與 `**/.omo/`），新 clone 不會取得此目錄內容。詳見下方「`.omo/` 用途與規範」段。
3. **專案根目錄** — 僅保留通用治理檔（README、AGENTS、CLAUDE、CHANGELOG、LICENSE、NOTICE、SECURITY、CONTRIBUTING、VERSION、Dockerfile、go.mod、docker-compose.yml）

## `.omo/` 用途與規範

### 目的

`.omo/` 是 AI agent 在本機工作時的 **ephemeral working directory**，存放每次 session 產生的 briefs、plans、evidence、traces、notepads 等。`.gitignore` 排除代表：
- **不該被 git 追蹤**（不會 commit 到 origin）
- **新 clone 看不到**（這是 by design，不是 bug）
- 內容純屬 local，agent session 結束可全刪

### 允許的內容

| 子目錄 | 用途 | 生命週期 |
|--------|------|---------|
| `briefs/` | phase 任務 briefs（P0-1、P0-2...） | active → merge 後刪除 |
| `plans/` | 執行 plan | active → merge 後刪除 |
| `evidence/` | 驗證報告（F1-F4、task-N） | short-lived（驗證完即刪）|
| `traces/` | sim 執行 JSONL（`sim-YYYYMMDD.jsonl`）| transient（每 run 留最新即可）|
| `run-continuation/` | session state JSON | session-only（隨 session 結束刪除）|
| `notepads/` | 決策筆記（learnings、issues、decisions）| transient |
| `workspaces/` | 跨 session 工作區協調 | merged 後刪除 |
| `handoffs/` | session 交接 | transient |
| `phaseN/`, `wave-N*/` | 進行中的 phase/wave 規劃 | merged 後刪除 |
| `boulder.json` | active 執行追蹤器 | 短暫 |
| `maps/` | 自動產生的架構快照 | 需定期重新生成（不要 commit） |

### 禁止的內容

| 類型 | 應該放哪 | 為什麼 |
|------|---------|------|
| **canonical 規範** | `docs/CONSTITUTION.md` | 規範必須被新 clone 取得 |
| **憲法級文件** | `docs/`（如 `docs/CONSTITUTION.md`）| 同上 |
| **被 AGENTS.md 引用** | `docs/` 或 `internal/<mod>/` | 引用斷裂風險 |
| **公開給人類的 reference** | `docs/` | 不該藏在 gitignored 目錄 |
| **生產規範** | `internal/<mod>/CONSTITUTION.md` 或 `docs/` | 模組級規範必須可見 |

### Lifecycle

- **active** → 工作中（agent 正在引用）
- **merge 後** → 主動刪除（避免本地污染）
- **無引用 30 天** → 可刪除
- **永遠不 commit**（`.gitignore` 會擋）

### 清理時機

- 每次 PR merge 後檢查 `.omo/` 對應 plan/brief 是否已 merge → 刪除
- 定期（每週或每月）用 `git status` 確認 `.omo/` 不在 staged
- 如果 `.omo/` 總大小超過 100MB，幾乎確定有 stale traces 累積

### 與 `docs/` 的對比

| 維度 | `docs/` | `.omo/` |
|------|---------|---------|
| Git tracked | ✅ | ❌ |
| 對象 | 人 + AI（canonical reference）| AI 個人工作區（ephemeral）|
| 生命週期 | 永久（直到主動 archive）| 短暫（session 級）|
| 新 clone 會拿到 | ✅ | ❌ |
| 該被 commit | ✅ | ❌（`.gitignore` 會擋）|
| 引用風險 | 低（可被 git 驗證）| 高（斷裂 = 引用不存在）|

### 預防文件斷裂 SOP

修改任何文件前，先確認該路徑在 git 內（`git ls-files <path>`）。若引用了 `.omo/` 內檔案：
1. 確認該檔確實是規範（不是 transient agent work）
2. 將檔移到 `docs/`（規範性）或 `internal/<mod>/`（模組級）
3. 更新所有引用該檔的 canonical 文件（AGENTS.md、GUIDELINES_INDEX.md 等）
4. 刪除 `.omo/` 內原始檔

**歷史教訓**：PR #751 即是此 SOP 的實際應用（`.omo/CONSTITUTION.md` → `docs/CONSTITUTION.md`）。

## 文件歸屬對照表

| 文件類型 | 歸屬位置 | 命名規範 |
|----------|---------|---------|
| 操作程序 / playbook | `docs/` | 描述性小寫、可用 `-` 分隔 |
| 規格 (spec) | `docs/specs/` | 模組或系統名稱 |
| 設計文件 / 規劃（active） | `.omo/briefs/`、`.omo/plans/` | `P<n>-<n>_<slug>.md` 或 `YYYY-MM-DD-<slug>.md` |
| 審計報告 | `docs/audit/` | `YYYY-MM-DD-<slug>.md` |
| 證據 / 驗證報告（active） | `.omo/evidence/` | `f<n>-<slug>.md` 或 `task-<n>-<slug>.md` |
| 任務交接 | `docs/handoff/` 或 `.omo/handoffs/`（active） | `YYYY-MM-DD-<topic>.md` |
| 根因調查 | `docs/investigations/` | `YYYY-MM-DD-<symptom>.md` |
| 修復計畫 | `docs/plans/` | `YYYY-MM-DD-<topic>-repair.md` |
| 追蹤紀錄 (sim 跑、runtime) | `.omo/traces/` | `sim-YYYYMMDD.jsonl` |
| 架構圖（auto-generated）| `.omo/maps/` | `<topic>-map.md` |
| Skills / AI 引導 | `.claude/skills/` | `atlas-<topic>/SKILL.md` |
| 開發者 / AI 指南 | `docs/guides/` | `<topic>-guide.md` |
| 快速入門 | `docs/QUICKSTART.md` | 單一檔（不可複製） |

- **日期前綴** `YYYY-MM-DD-`：時序敏感的文檔（handoff、investigation、audit、plan）
- **無日期前綴**：通用 reference（architecture、conventions、QUICKSTART）
- **slug**：小寫、`-` 分隔、無空格、無大寫
- **單數 vs 複數**：用單數（`docs/audit/`、`docs/handoff/`），例外是已存在的複數目錄（`docs/events/`、`docs/plans/`、`docs/wave-11/`）保留不動

## 生命週期

1. **活躍（active）**：當前 phase 使用，所有 AI 都應讀
2. **歸檔（archived）**：phase 完成後超過 60 天且無引用 → 移入 `docs/archive/YYYY/` 或 `.omo/archive/YYYY/`
3. **刪除（delete）**：歸檔超過 6 個月且仍無引用 → 從 repo 刪除（仍可從 git reflog 還原）

完整當前地圖見 `docs/DOCUMENTATION_MAP.md`。建立 audit 流程見 `docs/branch-hygiene/2026-06-26-cleanup.md`（同樣的 SOP 模式可套用到其他清理任務）。