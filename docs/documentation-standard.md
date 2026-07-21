# 文件存放規範 (Documentation Standard)

> 取代 AGENTS.md §「內容歸屬規則」中模糊部分。明定每種文件的歸屬位置、命名規範與生命週期。
> 維護者：見 `docs/documentation-map.md` 「動作紀錄」段。
> 最後更新：2026-07-21（manifests/ 治理：加入 docs/manifests/ 治理專節 + .omo/manifests/ 白名單 + 判斷流程更新）

## 三層結構原則

| 層級 | 用途 | Git 追蹤 | 新 clone 可見 | 對象 |
|------|------|---------|--------------|------|
| **`docs/`** | 規範、權威 reference、playbook、stable spec | ✅ | ✅ | 人 + AI（canonical） |
| **`.omo/`** | AI agent ephemeral working dir（嚴格白名單） | ❌ | ❌ | AI 個人工作區 |
| **根目錄** | 通用治理檔 | ✅ | ✅ | 全體 |

### 根目錄僅保留（白名單）

`README.md` `AGENTS.md` `CLAUDE.md` `CHANGELOG.md` `LICENSE` `NOTICE` `SECURITY.md` `CONTRIBUTING.md` `VERSION` `Dockerfile` `docker-compose.yml` `go.mod` `go.sum` `.gitignore` `.gitattributes` `.editorconfig` `.golangci.yml`

**禁止根目錄新增任何 `.md` 文件**（除了上述白名單）。新內容請先判斷歸屬再放。

---

## `docs/` 規範

### 內容判斷準則（**先回答 yes 才放**）

- [ ] 對 6 個月後新貢獻者仍有參考價值？
- [ ] 內容已穩定，未來 1 年內不會大改？
- [ ] 屬於「規範」「手冊」「架構」「領域知識」「憲法」「playbook」其中之一？

**任一為 no → 進 `.omo/`**。

### 子目錄結構

```
docs/
├── reference/constitution.md     # 深度開發憲法（8 條文）
├── reference/iteration-gate.md   # 5 Gate 自我檢查
├── quickstart.md                 # 5 分鐘入門（單一權威）
├── reference/guidelines-index.md # 規範階層
├── maturity.md                   # 模組成熟度
├── architecture.md               # 分層架構
├── reference/traps.md            # 跨模組陷阱
├── conventions-checklist.md      # 慣例檢查表
├── reference/parameter-system.md # 參數管理
├── json-schema-standard.md       # JSON schema 規範
├── operations-playbook.md        # 操作 playbook
├── iteration-playbook.md         # 迭代 playbook
├── evolution-loop.md             # 演化循環
├── data-sources.md               # 資料源說明
├── environment.md                # 外部依賴與環境
├── tools.md                      # 工具清單
├── audit-trail.md                # 稽核軌跡
├── branch-hygiene/              # branch 維護紀錄（PR #748 模式）
├── manifests/                    # invariant tracker 治理模板（僅 README.md + TEMPLATE.md；個別 manifest 請放 .omo/manifests/）
├── audit/                       # 審計報告（YYYY-MM-DD-slug.md）
├── handoff/                     # 任務交接（YYYY-MM-DD-topic.md）
├── investigations/              # 根因調查（YYYY-MM-DD-symptom.md）
├── incidents/                   # 線上事件紀錄（YYYY-MM-<short-name>.md）
├── modules/                     # 模組操作手冊（穩定後的 module runbook）
├── research/                    # 技術研究報告（有長期參考價值的研究產出）
├── spikes/                      # spike / PoC 報告（已完成並有教學價值）
├── specs/                       # 規格（topic-spec.md）
# 注意：docs/plans/ 已移除。所有具體修復/執行計畫請放 .omo/plans/（短期，merge 後刪）。
├── guides/                      # 指南（topic-guide.md）
└── archive/                     # 歸檔（見下方專節）
```

### `docs/archive/` 用途（**嚴格**）

**只放對 6 個月後新貢獻者有教學價值的歷史檔案**：

| 該放 | 不該放 |
|------|--------|
| 重大架構演進最終快照（2026-06-15-phase5-architecture 最終狀態）| 短期 plan/spec（merge 後 git reflog 是真相）|
| 重大決策的 audit 報告（避免重蹈覆轍）| 觀察期日誌（過渡性）|
| 有歷史教訓的 incident postmortem | 過時 migration（CHANGELOG 已有）|
| 重大規則演進（2026-06-15-experiment-baseline-report 稀疏資料教訓）| 純粹 snapshot（無結論、無教訓）|

**入 archive 前必答**：「新 clone 用戶 6 個月後會從這份檔案學到 CHANGELOG 看不到的東西嗎？」

- 是 → 進 archive
- 否 → 刪除（git reflog 仍可恢復）


### `docs/manifests/` 治理（**2026-07-21 新增**）

`docs/manifests/` 目錄**僅保留兩個永久治理文件**：

- `README.md` — manifest 機制說明、建立流程、驗證工具使用方式
- `TEMPLATE.md` — invariant tracker 標準模板（Phase A/B/C/D + Backlog + Commit Discipline）

**個別 manifest（審計/修復追蹤文件）不應放在 `docs/manifests/`**。它們是 transient investigation artifacts，應放在 `.omo/manifests/`（見下方 `.omo/` 白名單）。

#### Manifest 生命週期

| 階段 | 位置 | 動作 |
|------|------|------|
| **建立** | `.omo/manifests/YYYY-MM-DD-slug.md` | 從 `docs/manifests/TEMPLATE.md` 複製 |
| **進行中** | `.omo/manifests/` | 正常編輯、commit、PR |
| **完成** | 判斷後處理 | 見下方 promotion 路徑 |
| **PR merge** | 自動清理 | `scripts/verify-manifest.sh` 檢查；完成後決定歸檔或刪除 |

#### Manifest 完成後 Promotion 路徑

```
Manifest done →
  ├─ 含 stable spec-level invariant → promote 到 docs/specs/<topic>-spec.md（提取 invariants，非直接搬移）
  ├─ 有 6 個月教學價值（重大 bug 的根因分析、架構決策教訓）→ docs/archive/YYYY-MM-DD-slug.md
  └─ 無長期價值（單純修復追蹤）→ 刪除（git reflog 可恢復）
```

#### 自動化檢查

```bash
# 檢查 .omo/manifests/ 內是否有 7 天以上未更新的 done manifest
./scripts/cleanup-manifests.sh --stale-days 7
```

**禁止**：

- ❌ 在 `docs/manifests/` 存放個別審計 manifest（應放 `.omo/manifests/`）
- ❌ 已完成 manifest 無限期留在 `.omo/manifests/`（7 天後提示清理）
- ❌ 把 manifest 當 spec 用（應 extract invariant 到 `docs/specs/`）

**archive 內禁止新增子目錄**（`archive/superpowers/`、`archive/plans/` 都已撤銷）。所有歸檔檔案直接放 `archive/YYYY-MM-DD-<slug>.md`。

### `docs/` 生命週期

1. **active** — 當前使用
2. **stale 60 天無引用** → 評估是否進 `docs/archive/`
3. **archive 超過 6 個月無引用** → 從 repo 刪除（git reflog 可恢復）

---

## `.omo/` 規範（**最重要，本節是紀律核心**）

### 核心原則

- **`.omo/` 是 AI 個人工作區**，**新 clone 看不到**（by design）
- **AI 禁止自由生成新子目錄**——只能使用下方白名單
- **每個子目錄有嚴格命名規範與生命週期**——避免你描述的「內容到處散落」問題
- **工作區結束時主動清理**——避免你描述的「維護成本高」問題

### 完整子目錄白名單（**這是所有 AI 必須遵守的**）

| 子目錄 | 用途 | 命名規範 | 生命週期 |
|--------|------|---------|---------|
| `briefs/` | **長壽** phase 規劃 brief（roadmap、跨模組設計） | `<topic>-brief.md`（無日期前綴） | active → 設計穩定後升級到 `docs/` |
| `plans/` | **短壽** 執行計畫（具體 PR 的待辦） | `P<n>-<slug>.md` 或 `YYYY-MM-DD-<slug>.md` | merge 後**必須刪除** |
| `evidence/` | **短壽** 驗證報告（f1-f4 通過證明） | `f<n>-<topic>.md` 或 `task-<n>-<topic>.md` | 驗證完即刪 |
| `traces/` | ~~sim 執行 JSONL~~ （已遷移至 `data/state/traces/`） | — | — |
| `notepads/` | 跨 session 決策筆記 | `<topic>/learnings.md` 等子目錄 + 檔案 | 寫滿或過時即歸檔或刪 |
| `handoffs/` | session 交接 | `YYYY-MM-DD-<topic>.md` | session 結束即刪 |
| `workspaces/` | 跨 session 工作區協調 | `<workspace-name>/` | merged 後刪 |
| `run-continuation/` | session state JSON | `session-<id>.json` | session 結束即刪 |
| `phaseN/`, `wave-N/` | 進行中的 phase/wave 工作目錄 | `phase<N>/<slug>.md` | merged 後刪 |
| `boulder.json` | 執行追蹤器 | — | 任務完成即清 |
| `maps/` | 自動產生的架構快照 | `<topic>-map.md` | 重新生成時覆蓋舊檔 |
| `manifests/` | **短壽** invariant tracker（審計/修復追蹤 manifest） | `YYYY-MM-DD-<slug>.md` | 完成後 promote/archive/delete；7 天 stale 提示清理 |

### **禁止的子目錄**（歷史教訓）

以下子目錄名稱**禁止使用**（過去 AI 自由生成導致的污染）：

- ❌ `archive/`（應刪除內容，不該再「歸檔」）
- ❌ `audits/`（複數，與 `docs/audit/` 衝突；放 `evidence/`）
- ❌ `client_ui/`、`drafts/`、`investor-ui/`（無命名規範，內容混亂）
- ❌ `live-mode-macro-boundary.md`（獨立檔案，應放 `briefs/`）
- ❌ `session-summary-YYYY-MM-DD.md`（應放 `handoffs/` 或 `notepads/`）
- ❌ `evidence/<topic>/`（**禁止子目錄**——直接用 `evidence/<topic>-task-N.md`）

**歷史違規子目錄在 PR #756 全部清理**。

### 命名規範細則

| 規則 | 範例 |
|------|------|
| 全小寫、`-` 分隔 | `alert-redesign-brief.md` ✅，`AlertRedesign.md` ❌ |
| **簡述性 slug**（不要把整段標題放進檔名）| `roadmap.md` ✅，`2026-06-26-roadmap-v0.0.0.22-update.md` ❌ |
| 短壽內容用日期前綴（時序敏感） | `plans/2026-06-26-llm-router-fix.md` ✅ |
| 長壽內容**不用**日期前綴 | `briefs/roadmap.md` ✅，`briefs/2026-roadmap.md` ❌ |
| 子目錄禁止巢狀（除 `workspaces/` 與 `phaseN/`）| `evidence/foo/bar.md` ❌ |
| 單數優先（`brief/`、`plan/`、`evidence/`）| 與 `docs/` 保持一致 |

### `.omo/` 生命週期（**AI 必須主動執行**）

| 時機 | 動作 |
|------|------|
| 每次 PR merge | 檢查 `plans/` 對應檔 → 刪除；檢查 `evidence/` 對應檔 → 刪除 |
| 每次 session 結束 | 檢查 `handoffs/` → 確認已交接或刪除；`run-continuation/` → 刪除 |
| 每次工作區起步 | `git status` 確認 `.omo/` 不在 staged；無用內容立即清 |
| 內容升級到 `docs/` 時 | `.omo/` 內副本**立即刪除**（避免引用混淆）|
| 30 天無引用 | 可手動刪除（不會影響 git reflog）|
| 6 個月無引用 | 必須刪除 |

**禁止**：

- 在 `.omo/` 內新增白名單外的子目錄
- 在 `evidence/`、`plans/` 等短壽子目錄內建巢狀子目錄
- 把規範性內容放進 `.omo/`（規範必須在 `docs/`）
- 跨工作區共用 `.omo/` 內容（每個工作區獨立）

### Wave 工作目錄生命週期（`.omo/wave-N/` 專節）

Wave 目錄用於階段性開發窗口（如 L2.3 PoC、L2.4 觀察期）。與一般短壽目錄不同，wave 有明確的 promotion/rollback 路徑：

**放入條件**（任一即放）：
1. **PLANNED / IN PROGRESS** — 未啟動或正在執行的階段
2. **Wave-specific 暫存** — 例如觀察記錄（啟用後建立）
3. **Wave cleanup** — 歸檔報告準備

**Promotion 路徑**（wave 結束後）：
```
Wave 完成 → 文件分類 →
  ├─ 永久 reference → docs/specs/ 或 docs/guides/
  ├─ 觀察期文件 → .omo/wave-N/（帶 status banner）
  └─ 一次性 audit → docs/archive/
```

**Rollback 路徑**：
- 失敗 → 帶 status banner 移至 `docs/archive/YYYY-MM-DD-<wave>-resolved.md`
- 觀察期結束 → 成功 promotion 升級到 `docs/`；rollback 如上

**規則**：
- 已 ship 的永久文件直接進 `docs/specs/`、`docs/guides/`，**不留在 wave 目錄**
- wave 目錄在 `.omo/` 下（非 `docs/`），避免規範目錄被暫存內容污染
- 每個 wave 目錄應有 README 標註狀態（PLANNED / IN PROGRESS / COMPLETED）

> 歷史參考：`docs/wave-11/`（2026-06-28 解散）— L2.4 規劃 → PR #824 永久化為 `docs/operations/l2-4-runbook.md` + `docs/specs/l2-4-observation-spec.md` + `docs/operations/l2-4-followup.md`（見該 PR 的 work report 與 commits）。

### 判斷流程：放 `docs/` 還是 `.omo/`？

```
這個內容是什麼？
│
├─ 規範 / 憲法 / playbook / 架構 / 領域知識？
│   └─ 是 → 進 docs/（必須 git tracked）
│
├─ 跨模組、跨 session、需要被未來讀到？
│   ├─ 是、且穩定 → 進 docs/
│   └─ 是、但還在變 → 進 .omo/briefs/（長壽）
│
├─ 短期的 PR 待辦、驗證報告？
│   └─ 是 → 進 .omo/plans/ 或 .omo/evidence/（短壽）
│
├─ 審計/修復追蹤 manifest（invariant tracker）？
│   └─ 是 → 進 .omo/manifests/（短壽；完成後 promote/archive/delete）
│
├─ sim 執行追蹤 JSONL？
│   └─ 是 → 進 data/state/traces/（由 `LifecycleManager` 管理保留）
│
└─ session 內的工作記憶、交接？
    └─ 是 → 進 .omo/notepads/ 或 .omo/handoffs/（transient）
```

### 防止「AI 自由生成子目錄」機制

每次 AI 準備建立新 `.omo/<dir>/` 時，必須：

1. **先 grep** `documentation-standard.md` § `.omo/` 完整子目錄白名單
2. **若新目錄不在白名單**：停手，問使用者是否要擴充白名單
3. **若白名單內已有相似用途**：用既有目錄 + 命名規範
4. **若確定要新增**：必須在 PR 中同步更新 `documentation-standard.md` 與 `documentation-map.md`

**自我檢查指令**：

```bash
# 工作區起步時確認 .omo/ 結構合規
ls -la .omo/
# 對照白名單，任何不在白名單的子目錄/檔案都需清理
```

---

## 工作區起步 SOP（新 AI 開工作區必讀）

```bash
# 1. 讀規範（精簡版必讀）
cat docs/documentation-standard.md | head -100
cat docs/documentation-map.md | head -80

# 2. 確認 .omo/ 結構合規
ls .omo/
# 若有白名單外的目錄，記得先清理（見 PR #756）

# 3. 找規劃文件
ls .omo/briefs/         # 長壽規劃
ls .omo/plans/          # 短期計畫

# 4. 確認 .gitignore 有 .omo/ 與 .opencode/
grep -E "(\.omo|\.opencode)" .gitignore
```

---

## 完整文件歸屬對照表

| 文件類型 | 歸屬位置 | 命名規範 |
|----------|---------|---------|
| 規範 / 憲法 / playbook | `docs/` | 無日期前綴，描述性小寫 |
| 架構 / 領域知識 | `docs/` | 無日期前綴 |
| 規格 (spec) | `docs/specs/` | `<topic>-spec.md` |
| 開發者指南 | `docs/guides/` | `<topic>-guide.md` |
| 審計報告 | `docs/audit/` | `YYYY-MM-DD-<slug>.md` |
| 任務交接（穩定版） | `docs/handoff/` | `YYYY-MM-DD-<topic>.md` |
| 根因調查 | `docs/investigations/` | `YYYY-MM-DD-<symptom>.md` |
| 線上事件紀錄 | `docs/incidents/` | `YYYY-MM-<short-name>.md` |
| 模組操作手冊 | `docs/modules/` | `<module>.md` + `README.md` |
| 技術研究報告 | `docs/research/` | `<topic>.md` |
| spike / PoC 報告 | `docs/spikes/` | `<topic>-spike.md` |
| 操作 runbook / 驗證報告 | `docs/operations/` | `<topic>-runbook.md` / `<topic>-verification-report.md` |
| 歸檔（教學價值） | `docs/archive/` | `YYYY-MM-DD-<slug>.md` |
| **長壽 brief（跨 session 規劃）** | `.omo/briefs/` | `<topic>-brief.md` 或 `<topic>.md` |
| **短期 PR 待辦 / 修復計畫** | `.omo/plans/` | `P<n>-<slug>.md` 或 `YYYY-MM-DD-<slug>.md` |
| Manifest 治理模板 | `docs/manifests/` | `README.md` + `TEMPLATE.md` |
| Invariant tracker manifest（個別審計/修復） | `.omo/manifests/` | `YYYY-MM-DD-<slug>.md` |
| 驗證報告 | `.omo/evidence/` | `f<n>-<topic>.md` 或 `task-<n>-<topic>.md` |
| sim 輸出 | `data/state/traces/` | `sim-YYYYMMDD.jsonl` |
| session 交接 | `.omo/handoffs/` | `YYYY-MM-DD-<topic>.md` |
| 決策筆記 | `.omo/notepads/` | `<topic>/learnings.md` 等 |
| Skills / AI 引導 | `.claude/skills/` | `atlas-<topic>/SKILL.md` |

### 命名格式細則

- **日期前綴 `YYYY-MM-DD-`**：時序敏感（handoff、investigation、audit、plan）
- **無日期前綴**：通用 reference（architecture、conventions、QUICKSTART、brief）
- **slug**：小寫、`-` 分隔、無空格、無大寫
- **單數 vs 複數**：用單數，例外是已存在的複數目錄（`docs/reference/events/`）保留不動
- **P<n> 編號**：plans 與 evidence 的 P0-1、P0-2 編號可選，但建議用於大型 multi-PR 規劃

---

## 動作紀錄

完整當前地圖見 `docs/documentation-map.md`。清理 SOP 模式見 `docs/branch-hygiene/2026-06-26-cleanup.md`（同樣的 SOP 模式可套用到其他清理任務）。
