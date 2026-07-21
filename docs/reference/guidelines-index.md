# Atlas-Go 規範文件索引（Guidelines Index）

**版本**：1.6.1  
**日期**：2026-07-12  
**用途**：所有規範文件的統一入口與權威階層定義。

---

## 規範優先級鏈（Authority Hierarchy）

當不同規範文件對同一事項有衝突或重疊時，**優先級從高到低**依序為：

```
階層 1: 憲法（Constitution）
  └── docs/reference/constitution.md
      深度工作與數學/實證約束；非 CI 強制，但對 optimizer / portfolio / risk 具關鍵約束。
      註：原位於 `.omo/constitution.md`（2026-06-26 PR #751 移至此處，`.omo/` 為 .gitignore 排除，新 clone 不可見）。
  └── docs/reference/iteration-gate.md
      迭代閘門（5 Gate 自我檢查：數學深度、資產通用性、Falsifiability、程式碼預算、回歸測試）。
  └── internal/apigateway/CONSTITUTION.md
      強制規範，CI 自動檢查。違反會阻斷 PR。

階層 2: 領域守則（Domain Instructions）
  └── .github/instructions/go-core.instructions.md
  └── .github/instructions/experiments-guardrails.instructions.md
  └── .github/instructions/live-trading.guardrails.instructions.md
      領域特定規則，PR review 手動檢查。

階層 3: 技能文件（Skills）
  └── .claude/skills/*/SKILL.md
      操作指南與最佳實踐，建議遵循。

階層 4: 模組指南（AGENTS.md）
  └── internal/*/AGENTS.md
      模組特有陷阱與慣例，工作前建議閱讀。

階層 5: 參考文件（Reference Docs）
  └── docs/*.md
      背景知識與設計決策記錄。
```

**衝突處理原則**：
- 高一階層的規範**覆蓋**低一階層
- 同階層衝突 → 以 `constitution.md` 為最終仲裁者
- 若未在任一規範中找到答案 → 以原始碼實作為準

---

## 文件地圖

### 階層 1：憲法（Constitution）

| 文件 | 範圍 | AI 入口可達性 |
|------|------|-------------|
| `docs/reference/constitution.md` | 深度工作要求、矩陣運算、實證驗證、反駁要求、資產通用程式碼、程式碼預算 | ✅ 從 `AGENTS.md` 索引 |
| `internal/apigateway/CONSTITUTION.md` | 數據源管理：Gateway 模式、限流、熔斷、背景任務、環境變數 | ✅ 從 `agents.md` → `copilot-instructions.md` |

### 階層 2：領域守則（Instructions）

| 文件 | 範圍 | 何時閱讀 |
|------|------|---------|
| `.github/instructions/go-core.instructions.md` | Go 編碼：Import 順序、錯誤包裝、介面設計、測試規則 | 修改 `internal/` 或 `cmd/` 下的 Go 程式碼時 |
| `.github/instructions/experiments-guardrails.instructions.md` | 實驗生命週期：Baseline 優先、視窗就緒檢查、接受邏輯 | 修改 `experiment/`、`baseline/`、`evolution/` 時 |
| `.github/instructions/live-trading.guardrails.instructions.md` | Live Trading：Replay 優先、TODO 邊界、風控整合 | 修改 `internal/live/` 或啟用 live mode 時 |

### 階層 3：技能文件（Skills）

> 完整技能地圖與分類體系請見 **`.claude/SKILLS-MAP.md`**（20 技能，5 大分類）。

| 文件 | 範圍 | 何時閱讀 |
|------|------|---------|
| `.claude/skills/atlas-pre-change-protocol/SKILL.md` | 修改程式碼前的 7 步檢查清單 | **修改程式碼前必讀** |
| `.claude/skills/atlas-macro-narrative/SKILL.md` | 宏觀敘事、資金流向推導 | 每日開盤前、重大事件發生時 |
| `.claude/skills/atlas-risk-management/SKILL.md` | 動態倉位調整、回撤機制 | 組合回撤或宏觀風險升級時 |
| `.claude/skills/atlas-strategy-evolution/SKILL.md` | 實驗生命週期、Darwinian 權重 | 每月模型績效評估 |
| `.claude/skills/atlas-swarm-analyst/SKILL.md` | Swarm 模擬結果、市場共識、異常偵測 | Swarm 執行後 |
| `.claude/skills/atlas-data-visibility/SKILL.md` | 四層資料可見性防護 | 資料流/通道變更時 |

**機器人溝通技能**（`robot-communication/`，4 個）：供 OpenClaw/Hermes Agent 載入，為投資人提供每日摘要、組合問答、策略解釋、風險解讀。非 AI Coding 用，詳見 `.claude/SKILLS-MAP.md`。

### 階層 4：模組指南（AGENTS.md）

> **2026-07-11 精簡後**：保留 **15 個** `AGENTS.md` 位置（含根目錄與跨模組集群檔案）。完整清單與涵蓋模組見 [`internal/AGENTS_INDEX.md`](../../internal/AGENTS_INDEX.md)。

| 位置 | 涵蓋範圍 | 獨特內容 |
|------|---------|---------|
| `./AGENTS.md` | 跨模組高頻陷阱 | 前 5 條高頻陷阱速查表；完整列表見 `docs/reference/traps.md` |
| `internal/apigateway/` | apigateway | Gateway.Fetch、BackgroundTaskManager、CircuitBreaker |
| `internal/capitalflow/` | capitalflow / eventdriven / recommender / subscription | 資金流、事件日曆、推薦、認證集群 |
| `internal/fubonproxy/` | fubonproxy | ProcessManager supervisor F1-F9、Stop/backoff |
| `internal/live/` | live | broker execution、nonce、EventPositionUpdate |
| `internal/llm/` | llm | Router 唯一入口、DataClass gate、capability SOP |
| `internal/marketdata/` | marketdata | Provider 抽象、DecodeJSON、fubon URL guard |
| `internal/monitoring/` | monitoring / api/shared | Dashboard API、auth whitelist、Wave 9 |
| `internal/orchestrator/` | orchestrator | Executor 路由、PluginHost、ANTIPATTERNS |
| `internal/strategy_techniques/` | strategy / strategy_ranker / strategy_validator / strategy_techniques | 策略集群（選擇/排名/驗證/心法） |
| `admin_web/` | admin_web | 行事曆組件、參考檔案 |
| `client_web/` | client_web | API field contract、shared_web fallback |
| `cmd/experimental/` | cmd/experimental | dry-run 禁令、dummy mode、live broker |
| `cmd/atlas-mcp/server/` | cmd/atlas-mcp/server | tool 註冊、audit、auth、transport |
| `scripts/openclaw/` | scripts/openclaw | baseline 政策、閘門、dry-run |

### 階層 5：參考文件（Reference Docs）

| 文件 | 範圍 |
|------|------|
| `docs/architecture.md` | 分層設計、元件職責 |
| `docs/environment.md` | 外部依賴與開發環境狀態單一真相來源（PR #700） |
| `docs/ai-agent-architecture.md` | 代理協調、決策流程 |
| `docs/reference/parameter-system.md` | 參數管理、權威溯源 |
| `docs/operations-playbook.md` | 日常運維、mutation 工作流程 |
| `docs/evolution-loop.md` | 接受門檻、循環機制 |
| `docs/iteration-playbook.md` | Mutation 策略模式 |
| `docs/data-sources.md` | 資料匯入、Replay 格式 |
| `docs/script-usage-guide.md` | 輔助腳本使用方式 |
| `docs/archive/2026-06-26-gateway-migration-tracking.md` | 遷移 TODO 追蹤（已封存） |
| `docs/ai-prompt-files.md` | AI prompt 檔案追蹤政策（避免 local-only 漂移） |
| `docs/multi-cli-protocol.md` | 多 CLI 並行 worktree 協議 |
| `docs/operations/l2-4-runbook.md` | L2.4 觀察期操作手冊（pre-flight / daily check-in / acceptance / rollback） |
| `docs/operations/l2-4-followup.md` | L2.4 後續工作報告（auto-cron / CLI flag / promotion 4 步） |
| _（shipped via PR #828,merged 2026-06-29）_ | L2.4 CLI flag 實作：`--use-llm-sector-agents`（plan 內容已併入 `docs/operations/l2-4-followup.md`）|
| `docs/specs/l2-4-observation-spec.md` | L2.4 觀察指標 slog schema 規格（per-reco + aggregate metrics） |
| `docs/tools.md` | ACI 工具（GitNexus / codebase-memory / CodeGraph / atlas-mcp）路由決策樹 |
| `docs/reference/tool-catalog.md` | atlas-mcp tool 決策樹與完整 catalog（112 tools,供外部 AI agent；post-Round-2 dedup 基線） |

---

## 使用情境路由表

依你的工作類型，找出應閱讀的規範文件：

| 工作類型 | 必須閱讀 | 建議閱讀 |
|---------|---------|---------|
| **修改程式碼前** | `atlas-pre-change-protocol/SKILL.md` | `go-core.instructions.md`，對應模組的 `AGENTS.md` |
| **新增/修改 Go 程式碼** | `go-core.instructions.md` | 對應模組的 `AGENTS.md`，`constitution.md`（若涉及資料源） |
| **新增資料源/API 調用** | `constitution.md`（全部 8 條） | `marketdata/AGENTS.md` |
| **執行實驗** | `experiments-guardrails.instructions.md` | `experiment/AGENTS.md`，`baseline/AGENTS.md` |
| **Live Trading** | `live-trading.guardrails.instructions.md` | `constitution.md`，`live/AGENTS.md` |
| **新增參數** | `docs/reference/parameter-system.md` | `go-core.instructions.md` |
| **新增背景任務** | `constitution.md` 第四條 | 無 |
| **修改前端** | `monitoring/AGENTS.md`（snake_case 契約） | 無 |
| **理解系統架構** | `docs/architecture.md`、`.claude/SKILLS-MAP.md` | `docs/ai-agent-architecture.md` |
| **跨模組重構** | `docs/architecture.md`、`.claude/SKILLS-MAP.md` | 涉及的所有模組 `AGENTS.md` |

---

## 快速查詢：規範文件位置

| 目錄 | 存放內容 | 數量 |
|------|---------|------|
| `docs/reference/constitution.md` | 憲法（最高權威） | 1 |
| `.github/instructions/*.md` | 領域守則 | 3 |
| `.claude/skills/*/SKILL.md` | 手寫技能文件 | 20 |
| `internal/*/AGENTS.md` | 模組指南（經 2026-07-11 精簡） | 15 |
| `docs/*.md` | 參考文件 | 10+ |
| `docs/specs/*.md` | 模組規格（從 AGENTS.md 遷移） | 47 |
| `docs/guides/*.md` | 領域指南（從 AGENTS.md 遷移） | 9 |
| `docs/operations/*.md` | 操作手冊 | 10+ |
| `docs/manifests/*.md` | Implementation manifest | 1 |
| `.claude/skills/robot-communication/*/SKILL.md` | 機器人溝通技能 | 4 |
| `.claude/skills/gitnexus/*/SKILL.md` | GitNexus 工具技能 | 6 |

---

## 修訂歷史

| 版本 | 日期 | 修訂內容 |
|------|------|---------|
| 1.6.1 | 2026-06-27 | 修正統計數字（docs/specs 9→14、docs/guides 1→6）— follow-up to PR #793 |
| 1.6 | 2026-06-27 | 同步 Batch 5a-6 AGENTS.md 精簡：內模組 49→21、新增 docs/specs/ 與 docs/guides/ 分類（PR #784-#788）|
| 1.5 | 2026-06-26 | 修復文件斷裂：`.omo/constitution.md` → `docs/reference/constitution.md`，新增 `docs/reference/iteration-gate.md`（PR #752）|
| 1.4 | 2026-06-25 | 加入 `docs/environment.md` 索引；更新版本/日期以反映 PR #700 |
| 1.3 | 2026-06-17 | 修正不存在技能引用（atlas-core-architecture → SKILLS-MAP.md + docs/architecture.md）；更新技能數量為實際值（手寫 10 + 生成 21 + 機器人 4 + GitNexus 6）；新增機器人溝通與自動生成技能分類說明 |
| 1.2 | 2026-06-02 | 修正統計數字：技能文件 5→16、模組指南 21→34、移除 sim 缺失標記（已補齊） |
| 1.1 | 2026-05-29 | 補齊憲法、技能與模組指南索引，修正缺失清單與使用情境路由 |
| 1.0 | 2026-05-22 | 初版，依據 Phase 2 規範盤點建立 |
