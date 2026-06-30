# Atlas-go Agent Interface — 完整 Roadmap

> **規劃日期**：2026-06-30
> **範圍**：將 atlas-go 從「人類導向 web UI 為主」升級為「AI Agent 可操作性雙軌」
> **使用者決策紀錄**：分階段執行、完整暴露 MCP 範圍、每階段 review、全 P0-P4 完整規劃

---

## 1. 規劃結果摘要

| 階段 | 交付物 | 檔案 | 狀態 |
|------|--------|------|------|
| P0 | Workflow Map（21 條 workflow 盤查） | [`WORKFLOW_MAP.md`](../WORKFLOW_MAP.md) | ✅ v1 已交付 |
| P1 | MCP Server 設計規格（70+ tools） | [`specs/agent-mcp-server.md`](../specs/agent-mcp-server.md) | ✅ 規格已交付 |
| P2 | Agent Tools 實戰指南（決策樹 + Top 15） | [`AGENT_TOOLS.md`](../AGENT_TOOLS.md) | ✅ 已交付 |
| P3 | GitNexus Process 標註 SOP | [`PROCESS_ANNOTATION_SOP.md`](../PROCESS_ANNOTATION_SOP.md) | ✅ 已交付 |
| P4 | Agent 5 分鐘 Onboarding | [`AGENT_ONBOARDING.md`](../AGENT_ONBOARDING.md) | ✅ 已交付 |

**結論**：規劃完整（5/5 文件落地）。**實作未開始** — 本 roadmap 是「設計 → 實作」的橋樑，列出實作時程建議。

---

## 2. 為什麼需要這套設計

**核心痛點**（2026-06-30 評估）：atlas-go 雖有 web UI 給人看、有 AGENTS.md 給程式碼編輯 AI 看，但 **沒有一個清晰、可被外部 AI Agent 操作的「理解 + 呼叫」介面**。

**升級後的雙軌**：
```
人類 ↔ web UI (admin_web + client_web) ↔ atlas core
AI Agent ↔ MCP protocol ↔ atlas-mcp (Go binary) ↔ atlas core (via HTTP)
```

---

## 3. 實作時程建議

### Phase 1 — 短期（建議 1-2 週 sprint）

| Task | 工時估算 | 依賴 | 風險 |
|------|---------|------|------|
| `cmd/atlas-mcp/` 雛型（**OFFICIAL `go-sdk` v1.6.1** + stdio transport + auth + audit log） | 3 工作天 | 無（SDK 決策已 spike 完成）| 低（使用官方 stable API） |
| 5 個核心 tool 實作（regime / strategy / experiment / alert / health） | 4 工作天 | 上一行 | 低 |
| README + 完整測試覆蓋 | 2 工作天 | 上一行 | 低 |
| Cursor / Claude Desktop 端到端測試 | 1 工作天 | 上一行 | 中 |
| **小計** | **~10 工作天** | | |

**交付**：`bin/atlas-mcp` 可用 + 5 個 tool + 文件，可讓 `daily briefing` 類 agent 跑通。

> **✅ MCP SDK Spike Gate 已通過**（[`docs/spikes/mcp-go-sdk-spike.md`](../spikes/mcp-go-sdk-spike.md)）— Go 版本完全相符、API stable、License 相容。**GO 進入 Phase 1 實作**

### Phase 2 — 中期（建議 2-4 週 sprint）

| Task | 工時估算 | 依賴 |
|------|---------|------|
| SSE + streamable-HTTP transport | 2 天 | Phase 1 |
| macro / narrative / risk 群組 tool 完整實作 | 5 天 | Phase 1 |
| JSON Schema 完整化 + 3 個整合測試 | 3 天 | 上一行 |
| Audit log retention + 合規檢查 | 1 天 | Phase 1 |
| `docs/PROCESSES.yaml` 建立（含 21 條 workflow 標註） | 3 天 | PROCESS_ANNOTATION_SOP §3 |
| **`AGENTS.md` 加入「Agent 介面」章節**（指向新文件） | 0.5 天 | 全部 |
| **小計** | **~14 工作天** | |

**交付**：完整 75 個 tool + 三種 transport + 標註流程上線。

### Phase 3 — 長期（建議 1-3 月）

| Task | 工時估算 | 說明 |
|------|---------|------|
| 自動生成 tool description（基於 handler source） | 5 天 | 減少維護成本 |
| Agent 行為分析（追蹤實際 MCP call patterns） | 5 天 | 用於 audit + 優化 |
| MCP Resources / Prompts 支援（目前只支援 Tools） | 3 天 | 升級 MCP protocol |
| Multi-tenant MCP token 管理 | 5 天 | 支援多 agent 並行 |
| **小計** | **~18 工作天** | |

---

## 4. 風險與緩解

| 風險 | 嚴重度 | 緩解策略 |
|------|--------|---------|
| Go MCP SDK 缺乏或未成熟 | 中 | Phase 1 先 spike 評估；如無，用 JSON-RPC 2.0 自製薄層 |
| 大量 endpoints → MCP tool 是重複維護負擔 | 高 | Phase 3 自動生成工具 description |
| `docs/PROCESSES.yaml` 與程式碼漂移 | 中 | CI 檢查「改 entry 必改 yaml」 |
| Agent 透過 MCP 觸發破壞性操作（雖已過濾） | 中 | audit log + rate limiting + anomaly detection |
| LLMSectorAgents 等 LLM hook 與 MCP 互衝 | 中 | LLM router 既已存在，MCP 透過 `llm_*` tool 統一入口 |

---

## 5. 開放議題（待決策）

1. **`atlas-mcp` 是否併入 `atlas` 主 binary？**
   - 選 A：分開 binary（單一職責）— 預設
   - 選 B：併入，子命令 `--mcp-server` — 部署簡化但 binary 變大
2. **audit log 保留期？** 預設 30 天，需符合金管會規範？
3. **是否開源 `atlas-mcp`？** 與 atlas-go 採用同樣 Apache 2.0？
4. **是否要支援 WebSocket MCP transport？** 部分 agent 偏好 WS

---

## 6. 下一步（建議立即行動）

1. ✅ **你 review 本 roadmap**（5 個文件）
2. ⏳ **決定要不要進入實作 Phase 1**
3. ⏳ **如要實作**：先跑 `spike` 驗證 Go 端 MCP SDK 可用性
4. ⏳ **spike 通過後**：照 Phase 1 schedule 開 1-2 週 sprint

---

## 7. 文件索引

| 主題 | 路徑 |
|------|------|
| Workflow 總覽 | `docs/WORKFLOW_MAP.md` |
| MCP Server 規格 | `docs/specs/agent-mcp-server.md` |
| Agent 工具實戰 | `docs/AGENT_TOOLS.md` |
| GitNexus 標註 SOP | `docs/PROCESS_ANNOTATION_SOP.md` |
| Onboarding 速讀 | `docs/AGENT_ONBOARDING.md` |
| 本 roadmap | `docs/plans/agent-interface-roadmap.md` |

---

**文件版本**：v1（2026-06-30）
**下次 review**：實作 Phase 1 完成後（約 2 週）
