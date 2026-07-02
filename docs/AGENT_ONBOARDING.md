# Atlas-go Agent Onboarding — 5 分鐘速讀

> **你是什麼**：你是一個要理解 atlas-go 系統如何運作的 AI Agent。
> **本文目標**：讀完本文即可掌握 80% 的日常知識。剩餘 20% 透過 [`AGENTS.md`](./../AGENTS.md) + 本目錄其他文件查找。

---

## 1. 這是做什麼的？

**Atlas-go** = 模擬優先、稽核導向的台股投資研究系統（Go 1.26）。
- 每天產生股票推薦 + conviction 分數
- 所有決策走 pipeline（data → regime → agent loop → risk gate → recommend）
- 包含 web UI（admin + client）供投資人和後台管理員用
- 有 LLM 整合（DeepSeek、M3 / Kimi）

---

## 2. 不要做的事

| ❌ | 理由 |
|----|------|
| 不要啟用 `-allow-live-broker` | 本機測試用實旗標極危險 |
| 不要繞過 BackgroundTaskManager 啟 goroutine | 破壞可關閉性 |
| 不要繞過 apigateway 直接呼叫外部資料 | 違反 [`internal/apigateway/CONSTITUTION.md`](./../internal/apigateway/CONSTITUTION.md) |
| 不要繞過 DefaultRouter 直接呼叫 LLM clients | 失去健康檢查 + 路由隔離 |
| 不要手動修改 `field_types.ts` / `valid_fields.json` | `go generate .` 會覆寫 |
| 不要硬編碼 magic number | 必須用 [`PARAMETER_SYSTEM.md`](./PARAMETER_SYSTEM.md) |
| 不要 JSON tag 寫成 camelCase | 必須對齊 `domain.*` 的 snake_case |

---

## 3. 怎麼找到答案

### 我要找流程 → 看 [`WORKFLOW_MAP.md`](./WORKFLOW_MAP.md)
21 條 workflow 編號（WA-001 ~ WA-701），分 7 個子層。每條都有入口函式、觸發條件、產出。

### 我要找工具該怎麼呼叫 → 看 [`AGENT_TOOLS.md`](./AGENT_TOOLS.md)
Top 15 高頻工具，含「何時該呼叫」決策樹與 3 個範例對話流程。

### 我要找完整 MCP 規格 → 看 [`specs/agent-mcp-server.md`](./specs/agent-mcp-server.md)
所有 80 個 tool 的 JSON Schema 設計範本、安全邊界、部署指引。

### 我要理解模組 → 看 [`internal/AGENTS_INDEX.md`](./../internal/AGENTS_INDEX.md)
52 個內部模組按 S/E/X/U 成熟度分組。

### 我要避坑 → 看 [`TRAPS.md`](./TRAPS.md)
跨模組陷阱完整參考（單一權威來源）。

### 我要寫程式碼 → 必先讀 [`../.claude/skills/atlas-pre-change-protocol/SKILL.md`](./../.claude/skills/atlas-pre-change-protocol/SKILL.md)
任何程式變更前必跑的 5 步 protocol。

### 我要找建置/部署指令 → 看 [`QUICKSTART.md`](./QUICKSTART.md)

### 我要看即時系統狀態 → 看 [`docs/WORKFLOW_MAP.md`](./WORKFLOW_MAP.md) §5 觸發條件總表

---

## 4. 系統核心概念速記

| 概念 | 一句話 |
|------|--------|
| **Workflow** | 一條業務流程，編號 WA-XXX |
| **Plugin** | 動態掛載的策略模組，可加權重 |
| **AgentLoop** | LLM agent 的 plan → tool_call → reflect 狀態機 |
| **RiskGate** | Pre/In/Post Trade 三段式風險檢查，回傳 RiskDecision |
| **EventBus** | Event-driven workflow 的真相；publish/subscribe |
| **Regime** | 市場體制：RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL |
| **Conviction** | 推薦強度（0-100），>80 = 強烈買進 |
| **Darwinian weight** | 達爾文式策略權重演化（表現好加權、差減權） |
| **Baseline** | 比較基準；新策略實驗前必須先有 baseline |

---

## 5. 程式碼地圖（只列關鍵點）

```
cmd/atlas/main.go              ← 入口 + 4 個 run mode（replay / sim / live）
internal/orchestrator/         ← 業務 workflow 主體
internal/risk/                 ← RiskGate + ApprovalWorkflow
internal/eventbus/             ← event-driven 編排
internal/marketdata/           ← 97 個資料源 provider
internal/llm/                  ← LLM router + 4 capability handlers
internal/spawning/             ← 自動生成新 agent（spec factory）
internal/experiment/           ← mutation / judge / promote / revert
internal/scheduler/            ← 6 個 background task
internal/portfolio/            ← agent_health / post_trade_analyzer
internal/alerts/               ← WA-601 alert 系統
docs/specs/                    ← 17 份設計規格
docs/AGENTS_INDEX              ← 21 個保留模組清單
docs/WORKFLOW_MAP.md           ← 21 條 workflow 總覽（你最該先讀）
```

---

## 6. 10 分鐘上手路徑

1. **2 分鐘**：讀本文（你正在讀的）
2. **3 分鐘**：讀 [`WORKFLOW_MAP.md`](./WORKFLOW_MAP.md) §1~§3（11 個 workflow 速覽）
3. **5 分鐘**：讀 [`AGENT_TOOLS.md`](./AGENT_TOOLS.md) 決策樹 + Top 15
4. （如需寫程式碼）讀 `atlas-pre-change-protocol` skill

完成後你就能：
- ✅ 知道「這個系統在做什麼」
- ✅ 知道「我現在該怎麼做」（看 AGENT_TOOLS）
- ✅ 知道「不要踩什麼坑」（看 TRAPS）
- ⚠️ 不會的：具體 deep dive — 仍需讀對應模組的 AGENTS.md 或程式碼
