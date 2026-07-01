# Atlas-go Agent Interface — 完整 Roadmap

> **規劃日期**：2026-06-30
> **最後更新**：2026-07-01
> **範圍**：將 atlas-go 從「人類導向 web UI 為主」升級為「AI Agent 可操作性雙軌」
> **使用者決策紀錄**：分階段執行、完整暴露 MCP 範圍、每階段 review、全 P0-P4 完整規劃；實作進度同步更新

---

## 1. 規劃結果摘要

| 階段 | 交付物 | 檔案 | 狀態 |
|------|--------|------|------|
| P0 | Workflow Map（21 條 workflow 盤查） | [`WORKFLOW_MAP.md`](../WORKFLOW_MAP.md) | ✅ v1 已交付 |
| P1 | MCP Server 設計規格（70+ tools） | [`specs/agent-mcp-server.md`](../specs/agent-mcp-server.md) | ✅ 規格已交付 |
| P2 | Agent Tools 實戰指南（決策樹 + Top 15） | [`AGENT_TOOLS.md`](../AGENT_TOOLS.md) | ✅ 已交付 |
| P3 | GitNexus Process 標註 SOP | [`PROCESS_ANNOTATION_SOP.md`](../PROCESS_ANNOTATION_SOP.md) | ✅ 已交付 |
| P4 | Agent 5 分鐘 Onboarding | [`AGENT_ONBOARDING.md`](../AGENT_ONBOARDING.md) | ✅ 已交付 |
| P5 | Agent 介面章節併入 `AGENTS.md` | [`AGENTS.md`](../../AGENTS.md) | ✅ PR #875 |

**結論**：規劃完整（5/5 文件落地 + P5 已進主文件）。**實作已進行中** — `cmd/atlas-mcp/` 已實作約 84 個 tools、stdio transport、auth/audit/anomaly/協議擴充；SSE/streamable-HTTP transport 與 `cmd/atlas` binary 合併仍待辦。本 roadmap 同步更新為「規劃 + 實作狀態」的單一真相來源。

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

| Task | 狀態 | 實際進度 |
|------|------|---------|
| `cmd/atlas-mcp/` 雛型（**OFFICIAL `go-sdk` v1.6.1** + stdio transport + auth + audit log） | ✅ 已完成 | `cmd/atlas-mcp/` binary 可編譯；stdio transport 為唯一生產 transport；TokenAuth + optional DB TokenStore + admin HTTP API（127.0.0.1）已實作；audit log v2（含 retention、cleanup、ArgsHash、SessionID、Transport）已上線。 |
| 核心 tool 實作（regime / strategy / experiment / alert / health 等） | ✅ 已完成 | 目前註冊約 **84 個 tools**（橫跨 19 個檔案），涵蓋 regime、strategy、experiment、alert、health、macro、narrative、risk、portfolio、llm、swarm、anomaly 等群組。 |
| README + 完整測試覆蓋 | ⚠️ 部分完成 | README 與 server 測試存在；transport 與部分 handler 仍需補齊測試。 |
| Cursor / Claude Desktop 端到端測試 | ⏳ 待進行 | 受 stdio-only transport 限制，尚未進行桌面 client 端對端驗證。 |

**交付**：`bin/atlas-mcp` 可用 + ~84 個 tool + 文件，可讓 `daily briefing` 類 agent 跑通。

> **✅ MCP SDK Spike Gate 已通過**（[`docs/spikes/mcp-go-sdk-spike.md`](../spikes/mcp-go-sdk-spike.md)）— Go 版本完全相符、API stable、License 相容。**Phase 1 實作已完成**

### Phase 2 — 中期（建議 2-4 週 sprint）

| Task | 狀態 | 實際進度 |
|------|------|---------|
| SSE + streamable-HTTP transport | ⏳ 待辦 | `main.go` 註解聲稱 Phase 2 已加入 SSE + streamable-HTTP，但實際 production 路徑僅 `mcp.StdioTransport{}`；相關 transport 程式碼尚未實作或啟用。 |
| macro / narrative / risk 群組 tool 完整實作 | ✅ 已完成 | 已隨 84 tools 一併上線；涵蓋 macro、narrative、risk、portfolio、industry 等群組。 |
| JSON Schema 完整化 + 3 個整合測試 | ⚠️ 部分完成 | tool schema 透過 SDK 註冊；整合測試覆蓋率仍需補強。 |
| Audit log retention + 合規檢查 | ✅ 已完成 | v2 schema 含 retention、cleanup loop；SessionID / ArgsHash / Transport 欄位已落地。 |
| `docs/PROCESSES.yaml` 建立（含 21 條 workflow 標註） | ✅ 已完成 | PR #875 併入 `docs/PROCESSES.yaml`（35 WA entries）與 `AGENTS.md`「Agent 介面」章節。 |

**交付**：完整 ~84 個 tool + stdio transport 上線；SSE/streamable-HTTP 為 Phase 2 唯一剩餘大項。

### Phase 3 — 長期（建議 1-3 月）

| Task | 狀態 | 實際進度 |
|------|------|---------|
| 自動生成 tool description（基於 handler source） | ✅ 已完成 | `cmd/atlas-mcp/descgen/` 已實作；目前約 84 tools 的 description 由 descgen 產生。 |
| Agent 行為分析（追蹤實際 MCP call patterns） | ⚠️ 部分完成 | audit/anomaly/metrics 已記錄 call patterns；專屬分析報告與 dashboard 未建立。 |
| MCP Resources / Prompts 支援（目前只支援 Tools） | ✅ 已完成 | `registerResources()` 與 `registerPrompts()` 已在 `cmd/atlas-mcp/server/server.go` 註冊並呼叫。 |
| Multi-tenant MCP token 管理 | ⚠️ 部分完成 | DB-backed token store 與 admin HTTP API 已存在；多 tenant 隔離與權限模型未完整定義。 |

**交付**：自動化 description、Resources/Prompts、audit/anomaly 已上線；multi-tenant 權限與行為分析報告為長期優化項。

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

1. **`atlas-mcp` 是否併入 `cmd/atlas` 主 binary？**
   - 選 A：分開 binary（單一職責）— **目前現狀**
   - 選 B：併入，子命令 `--mcp-server` — 部署簡化但 binary 變大；目前程式碼中 **未發現** 任何併入實作或子命令註冊
2. **SSE + streamable-HTTP transport 是否要啟用？** 程式碼註解聲稱 Phase 2 已加入，但實際 production 路徑仍只有 stdio。
3. **audit log 保留期？** 預設 30 天，需符合金管會規範？
4. **是否開源 `atlas-mcp`？** 與 atlas-go 採用同樣 Apache 2.0？
5. **是否要支援 WebSocket MCP transport？** 部分 agent 偏好 WS

---

## 6. 下一步（建議立即行動）

1. ✅ **你 review 本 roadmap**（5 個文件 + 實作現狀）
2. ✅ **Phase 1 實作已完成**（~84 tools、stdio、auth/audit/anomaly）
3. ⏳ **決定 Phase 2 剩餘大項**：SSE + streamable-HTTP transport 是否要實作與啟用
4. ⏳ **決定 binary 合併議題**：`atlas-mcp` 維持獨立 binary，或併入 `cmd/atlas --mcp-server`
5. ⏳ **補強測試與端到端驗證**：特別是 transport 層與桌面 client（Cursor / Claude Desktop）整合測試

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

**文件版本**：v2（2026-07-01）
**下次 review**：SSE + streamable-HTTP transport 與 binary 合併議題決策後（預計 1-2 週內）
