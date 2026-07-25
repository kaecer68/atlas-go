# 規範階層與使用情境路由

> **用途**：當多份文件衝突時的仲裁規則 + 依任務類型快速定位該讀哪份文件。
> **原則**：本檔案是被引用的路由表，不是內容倉庫。所有細節都在目標文件中。

---

## 規範階層（衝突時優先級由上而下）

| 階層 | 文件 | 範圍 |
|------|------|------|
| 1 | `docs/reference/constitution.md` | 8 條深度憲法：數學模型優先、程式碼預算、修改邊界、迭代閘門 |
| 2 | `internal/apigateway/CONSTITUTION.md` | API Gateway 憲法：Provider 註冊、BackgroundTaskManager、CircuitBreaker |
| 3 | `.github/instructions/go-core.instructions.md` | Go 編碼規則（import 分組、錯誤包裝、介面邊界） |
| 4 | `.github/instructions/experiments-guardrails.instructions.md` | 實驗 baseline 優先、replay 就緒檢查、保守接受邏輯 |
| 5 | `.github/instructions/live-trading.guardrails.instructions.md` | Live trading 協調路徑：replay 優先、TODO 邊界、風控整合 |
| 6 | `docs/reference/iteration-gate.md` | 5 Gate 自我檢查規範 |
| 7 | `AGENTS.md` + `internal/*/AGENTS.md` | 跨模組陷阱 + 模組特定陷阱 |
| 8 | Source code | 最終真相（文件與程式碼衝突時，以程式碼為準） |

> **衝突仲裁規則**：上層推翻下層。憲法 > guardrails > gate > traps > source code 慣例。
> 程式碼是第 8 層的「最終真相」— 文件說不能做但程式碼已經做了 → 文件過時。

---

## 使用情境路由

### 我要修改程式碼

| 情境 | 先讀 |
|------|------|
| 任何 `.go` 修改 | `atlas-pre-change-protocol` skill（7 步強制檢查） |
| 修改 `internal/` 模組 | `internal/AGENTS_INDEX.md` → 對應 `internal/<mod>/AGENTS.md` |
| 修改 API / Gateway | `internal/apigateway/CONSTITUTION.md` |
| 修改實驗 / baseline | `.github/instructions/experiments-guardrails.instructions.md` |
| 修改 live trading 路徑 | `.github/instructions/live-trading.guardrails.instructions.md` |
| 新增/改名 FactorType | `atlas-factor-change-protocol` skill（8 步協議） |
| 涉及 LLM router / provider | `internal/llm/AGENTS.md` + `atlas-llm-provider-capability` skill |
| 前端（JS/CSS） | `CLAUDE.md` §前端架構 |

### 我要除錯 / 審計

| 情境 | 先讀 |
|------|------|
| Bug hunt / 審計 | `atlas-audit-manifest-protocol` skill（根因 → manifest → commit → PR） |
| 跨模組陷阱 | `docs/reference/traps.md` |
| 資料流 / 通道問題 | `atlas-data-visibility` skill |
| 系統健康 / 觀測 | `atlas-monitoring-observability` skill |

### 我要了解架構 / 領域知識

| 情境 | 先讀 |
|------|------|
| 整體架構 | `docs/architecture.md` |
| 參數系統 | `docs/reference/parameter-system.md` |
| 資料流 | `docs/data-sources.md` + `docs/data-architecture.md` |
| MCP server | `cmd/atlas-mcp/README.md` + `docs/specs/agent-mcp-server-spec.md` |
| LLM 整合 | `docs/llm-integration-strategy-framework.md` |
| 模組成熟度 | `internal/MATURITY.md` + `internal/AGENTS_INDEX.md` |

### 我要找文件本身

| 情境 | 先讀 |
|------|------|
| 文件地圖（所有檔案清單） | `docs/documentation-map.md` |
| 文件存放規範 | `docs/documentation-standard.md` |
| 技能地圖 | `.claude/SKILLS-MAP.md` |

---

## 文件與程式碼衝突時的處理

1. **讀文件看它說什麼**（可能過時）
2. **讀程式碼看實際是什麼**（ground truth）
3. **衝突時**：
   - 在文件旁加 `> ⚠️ 已被 <commit/PR> 取代` 註記
   - 不擋自己的 work 等文件同步
   - 若決策本身可疑，直接問 user 是否 override

> 詳見 `docs/reference/traps.md` §「AI-Generated Doc 處理原則」。

---

## 關聯文件

| 檔案 | 用途 |
|------|------|
| `docs/reference/constitution.md` | 深度憲法（8 條文） |
| `docs/reference/iteration-gate.md` | 5 Gate 自我檢查 |
| `docs/reference/traps.md` | 跨模組陷阱（單一權威） |
| `docs/documentation-map.md` | 完整文件目錄索引 |
| `docs/documentation-standard.md` | 文件存放規範 |
| `AGENTS.md` | 跨工具 AI 共用指引 |
| `CLAUDE.md` | Claude Code 專屬設定 |
| `.claude/SKILLS-MAP.md` | 技能分類地圖 |
| `internal/AGENTS_INDEX.md` | 模組成熟度索引 |
