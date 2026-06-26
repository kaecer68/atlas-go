# 文件存放規範 (Documentation Standard)

> 取代 AGENTS.md §「內容歸屬規則」中模糊部分。明定每種文件的歸屬位置與命名規範。
> 建立日期：2026-06-26（PR #749）

## 原則

1. **`docs/`** — 規範性、權威性、reference / playbook / spec / audit 文件，給人類閱讀
2. **`.omo/`** — 執行性、迭代性、機器/AI 工作流產物（briefs、evidence、traces、maps、boulder、handoffs）
3. **專案根目錄** — 僅保留通用治理檔（README、AGENTS、CLAUDE、CHANGELOG、LICENSE、NOTICE、SECURITY、CONTRIBUTING、VERSION、Dockerfile、go.mod、docker-compose.yml）

## 文件歸屬對照表

| 文件類型 | 歸屬位置 | 命名規範 |
|----------|---------|---------|
| 操作程序 / playbook | `docs/` | 描述性小寫、可用 `-` 分隔 |
| 規格 (spec) | `docs/specs/` | 模組或系統名稱 |
| 設計文件 / 規劃 | `.omo/briefs/` 或 `.omo/plans/` | `P<n>-<n>_<slug>.md` 或 `YYYY-MM-DD-<slug>.md` |
| 審計報告 | `docs/audit/` | `YYYY-MM-DD-<slug>.md` |
| 證據 / 驗證報告 | `.omo/evidence/` | `f<n>-<slug>.md` 或 `task-<n>-<slug>.md` |
| 任務交接 | `docs/handoff/` 或 `.omo/handoffs/` | `YYYY-MM-DD-<topic>.md` |
| 根因調查 | `docs/investigations/` | `YYYY-MM-DD-<symptom>.md` |
| 修復計畫 | `docs/plans/` | `YYYY-MM-DD-<topic>-repair.md` |
| 追蹤紀錄 (sim 跑、runtime) | `.omo/traces/` | `sim-YYYYMMDD.jsonl` |
| 架構圖 | `.omo/maps/` | `<topic>-map.md` |
| Skills / AI 引導 | `.claude/skills/` | `atlas-<topic>/SKILL.md` |
| 開發者 / AI 指南 | `docs/guides/` | `<topic>-guide.md` |
| 快速入門 | `docs/QUICKSTART.md` | 單一檔（不可複製） |
| 憲法 / 強制規範 | `docs/CONSTITUTION.md` 或 `internal/<mod>/CONSTITUTION.md` | 兩者皆為最高層級，議題不同（**`.omo/CONSTITUTION.md` 不可作為 canonical 來源** — `.gitignore` 排除）|

## 命名規範

- **日期前綴** `YYYY-MM-DD-`：時序敏感的文檔（handoff、investigation、audit、plan）
- **無日期前綴**：通用 reference（architecture、conventions、QUICKSTART）
- **slug**：小寫、`-` 分隔、無空格、無大寫
- **單數 vs 複數**：用單數（`docs/audit/`、`docs/handoff/`），例外是已存在的複數目錄（`docs/events/`、`docs/plans/`、`docs/wave-11/`）保留不動

## 生命週期

1. **活躍（active）**：當前 phase 使用，所有 AI 都應讀
2. **歸檔（archived）**：phase 完成後超過 60 天且無引用 → 移入 `docs/archive/YYYY/` 或 `.omo/archive/YYYY/`
3. **刪除（delete）**：歸檔超過 6 個月且仍無引用 → 從 repo 刪除（仍可從 git reflog 還原）

完整當前地圖見 `docs/DOCUMENTATION_MAP.md`。建立 audit 流程見 `docs/branch-hygiene/2026-06-26-cleanup.md`（同樣的 SOP 模式可套用到其他清理任務）。