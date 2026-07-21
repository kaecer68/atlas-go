# Atlas MCP Server — Phase 3 殘餘任務規格

> **Audience**：Go 工程師接手 Phase 3 殘餘工作。
> **狀態**：🟡 DRAFT（2026-06-30，等待 user review）
> **範圍決策**：PR #842 (`feat/atlas-mcp-phase3`, 9428ef40) 已完成 Phase 3 上半 (audit retention + Phase 3B rate-limiting + Resources + Prompts + 2 errcheck fixes)。殘餘工作對齊 agent-interface-roadmap.md §3 原 4 項中扣除 Resources/Prompts 後的剩餘 3 項(roadmap snapshot 詳見 PR #876)。
> **關聯文件**：
> - [`agent-mcp-server.md`](./agent-mcp-server.md)（核心 spec，含 §11 Phase 2.2 Status）
> - 對齊 agent-interface-roadmap.md roadmap §3 Phase 3 原 4 項規劃(snapshot 詳見 PR #876)
> - [`docs/operations/mcp-deploy.md`](../operations/mcp-deploy.md)（部署守則）
> - PR #842: <https://github.com/kaecer68/atlas-go/pull/842>

---

## 1. 設計目標

完成 `agent-interface-roadmap.md §3「Phase 3 — 長期」` 原 4 項中扣除已 merge 的 Resources/Prompts 後，剩餘 3 項：

1. **自動生成 tool description** — 從 handler source 反推 description / JSON Schema，減少 74 個 tool 的手寫維護成本
2. **Agent 行為分析** — 追蹤實際 MCP call patterns、結構化 audit log、用於異常偵測與優化
3. **Multi-tenant MCP token 管理** — 多 agent 共享單一 atlas-mcp 進程時的 token 隔離、配額、撤銷

完成後，atlas-mcp 達到「**自動維護 + 可觀測 + 多租戶**」三個維度的 production-grade。

## 2. 不做 (Out of Scope)

- **新增業務 tool**：74 tools 已達 [`agent-mcp-server.md` §3.1](./agent-mcp-server.md#3-mcp-tools-清單) 候選上限 (70)，不再開新業務 endpoint。meta-tool（如 `mcp_get_call_stats`/`mcp_get_session_topology`）屬 system-level 觀測，豁免此限制。
- **新增 transport**：stdio / SSE / streamable-HTTP 三種已足夠覆蓋當前 agent 客戶端（Cursor / Claude Desktop / OpenCode）。
- **新增 LLM hook**：已有 `llm_*` tool 統一入口，`DefaultRouter` 已集中（見 [`CLAUDE.md`](../../CLAUDE.md) §LLM 路由）。
- **Multi-instance federation**：留給 Phase 4 評估（見 [`agent-mcp-phase4.md`](./agent-mcp-phase4.md) §3.3）。
- **動態 tool 註冊**：留給 Phase 4。

## 3. 殘餘任務

### 3.1 Item 1 — 自動生成 tool description（estim 5d）

#### 3.1.1 問題

當前 74 個 tool 的 `description` / `inputSchema` 是手寫的（位於 `cmd/atlas-mcp/server/tools_*.go` 各 handler 內）。每次 handler 改 input 參數，必須同步更新 schema — 是 high-maintenance 痛點（agent-interface-roadmap.md §4 列為「高」風險,snapshot 詳見 PR #876）。

#### 3.1.2 目標

從 handler function source 反推：

- `description`（由 Go function 名稱、godoc comment、request/response struct tag 組合）
- `inputSchema`（由 handler request struct 的 `json:` tag、`validate:` tag、struct field doc 推導）

#### 3.1.3 設計方案

**方式**：`go/ast` 解析 `cmd/atlas-mcp/server/tools_*.go`，提取每個 tool registration 對應的 handler func + request struct，生成 `cmd/atlas-mcp/auto-desc.gen.json`。

**整合**：
- 用 `//go:generate` 觸發（命令列範例如 `go generate ./cmd/atlas-mcp/...`）
- CI `generate` job 驗證「程式碼變更時 `auto-desc.gen.json` 必須同步」（與現有 `field_types.ts` / `valid_fields.json` 同 SOP — 見根 `AGENTS.md` §JSON tag snake_case）
- 生成的 schema 在 `atlas-mcp` startup 時掛載到每個 tool 的 `mcp.WithDescription()` / `mcp.WithInputSchema()`

**邊界 case**：
- 帶 `// gen:manual-override` doc comment 的 handler 跳過自動生成（保留人工 override）
- `optional` / `omitempty` JSON tag 對應到 schema 的 `not required`
- `enum` struct type（用 `enum:` tag 或定義 `StringEnum` interface 觸發 enum schema）

#### 3.1.4 驗收

- [ ] `go generate ./cmd/atlas-mcp/...` 重新生成後 `git diff` 只有 schema 變化（無意外 noise）
- [ ] 74 tools 中 ≥ 60 個 description 完全自動生成（保留 manual override 不超過 14 個）
- [ ] CI `generate` job 驗證：handler source 變更時對應 schema 必須隨之更新
- [ ] Unit test 涵蓋至少 5 個典型 edge case（帶 override / 無 doc / nested struct / enum / time.Duration）

#### 3.1.5 工時

5 工作天（依 `agent-interface-roadmap.md §3` 估算）

---

### 3.2 Item 2 — Agent 行為分析（estim 5d）

#### 3.2.1 問題

當前 audit log（PR #842 §5ee23896）記錄每筆 MCP call 的 timestamp、tool、tenant、status，但**沒有結構化的行為模型**：

- 無法回答「某 agent 在 1 小時內呼叫了哪些 tool」
- 無法識別「anomalous call patterns」（如短時間大量 `experiment_*` 觸發）
- 無法做 usage-based 優化（如哪些 tool latency 高、哪些 parameters 觸發錯誤）

#### 3.2.2 目標

建立 agent 行為資料模型（結構化 event schema）+ 聚合 query API。

#### 3.2.3 設計方案

**Event schema**（擴充現有 audit JSONL 結構，向下相容 v1）：
```json
{
  "schema_version": 2,
  "ts": "2026-07-01T09:00:00.000Z",
  "session_id": "uuid-v4",
  "agent_id": "claude-desktop-xyz",
  "tool": "regime_get_history",
  "args_hash": "sha256(...)",
  "status": "ok|error",
  "latency_ms": 42,
  "transport": "stdio|streamable-http|sse"
}
```

**新 tool**（在 [`agent-mcp-server.md` §3.1](./agent-mcp-server.md#3-mcp-tools-清單) `WA-606 系統健康` 加入）：
- `mcp_get_call_stats` — 回傳近 N 分鐘 call count / p50 latency / error rate
- `mcp_get_session_topology` — 回傳 agent_id ↔ tool call matrix（用於行為 audit）

**資料來源**：從現有 audit log JSONL 解析（不引入新 storage，30 天 retention 內 in-memory 聚合）。

**Privacy**：不存 raw arg，只存 hash — 防 sensitive arg leak。

#### 3.2.4 驗收

- [ ] audit log entry schema 升級到 v2，向下相容 v1 解析（支援 `schema_version` 欄位判斷）
- [ ] 2 個新 tool 通過 unit test + integration test
- [ ] p50 / p95 latency 透過 metric 暴露（Prometheus `/metrics` 或新 MCP tool）
- [ ] 30 天 retention 內 query 延遲 < 200ms（in-memory aggregated）
- [ ] agent_id 與 Item 3 multi-tenant token 整合驗證（token.revoked 後 agent_id 改為 "anonymous"）

#### 3.2.5 工時

5 工作天

---

### 3.3 Item 3 — Multi-tenant MCP token 管理（estim 5d）

#### 3.3.1 問題

當前 Bearer token 機制（見 [`agent-mcp-server.md` §2.1 transport_auth.go](./agent-mcp-server.md)）是「one global token」模式：

- `ATLAS_MCP_TOKEN` 環境變數一進程只有一個
- 多個 agent 共享單一 atlas-mcp 進程時無法區隔（誰做了什麼）
- 無 per-tenant rate limit（Phase 3B 已加，但仍是 global counter）
- 無 token 撤銷（rotate 環境變數要重啟）

#### 3.3.2 目標

支援 multi-tenant token 模式：

- 一個 atlas-mcp 進程持有 N 個 tenant token
- 每個 token 對應一個 `tenant_id` + `agent_id`
- token 註冊中心支援 CRUD + 撤銷 + rotate（不重啟進程）

#### 3.3.3 設計方案

**Token 存儲**：PostgreSQL 新 table `atlas_mcp_tokens`（schema 見 §3.3.4）。

**API**：
- HTTP 管理 API（admin only，需 X-Admin-Token，bind 127.0.0.1）— 註冊 / 撤銷 / rotate
- 既存 Bearer token 不變 — 只是 backend 從 env-var 改為 database lookup

**整合點**：
- audit log `agent_id` 從 token 反推
- per-tenant rate limit counter 用 token 內 `tenant_id` 作為 key（Phase 3B 改造）
- 撤銷：token row `revoked_at` 非空 → 認證 reject

**邊界 case**：
- DB 連線失敗 → fail-closed（拒絕所有 bearer auth，回到 stdio permissive）
- Token 即將過期 → auto-refresh（透過 management API 提前 rotate）
- 環境變數與 DB 同時配置：DB 優先，env-var 為 fallback（向後相容現有部署）

#### 3.3.4 Schema

```sql
CREATE TABLE atlas_mcp_tokens (
  token_id UUID PRIMARY KEY,
  token_hash VARCHAR(64) NOT NULL UNIQUE,  -- sha256 of bearer token, never store raw
  tenant_id VARCHAR(64) NOT NULL,
  agent_id VARCHAR(128) NOT NULL,
  scopes JSONB NOT NULL DEFAULT '[]',     -- e.g., ["read-only", "admin"]
  rate_limit_per_min INT,                 -- nullable = inherit global
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,                 -- nullable = non-expiring
  revoked_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ
);
CREATE INDEX idx_atlas_mcp_tokens_hash ON atlas_mcp_tokens(token_hash);
CREATE INDEX idx_atlas_mcp_tokens_tenant ON atlas_mcp_tokens(tenant_id);
```

#### 3.3.5 驗收

- [ ] 註冊 / 撤銷 / rotate 三個 management API 測試通過（含未授權拒絕測試）
- [ ] 撤銷後 bearer auth 立即 reject（不需重啟）
- [ ] 舊 `ATLAS_MCP_TOKEN` env var 模式保留為 fallback（DB 未配置時 fallback） — `ATLAS_MCP_TOKEN` 非外部數據源 API key，不受 [`CONSTITUTION §1.1`](../../internal/apigateway/CONSTITUTION.md) `os.Getenv` 禁止管轄
- [ ] per-tenant rate limit 與 Phase 3B 整合驗證（每 tenant 獨立 counter）
- [ ] DB migration script 測試（up + down）

#### 3.3.6 工時

5 工作天

---

### 3.4 (Optional) Item 4 — 待 user 指定

Roadmap §3 原列 4 項，其中 Resources/Prompts (3d) 已在 PR #842 提前完成。若 user 認為 Phase 3 殘餘應有第 4 項補充，候選方向：

| 候選 | 範圍 | 估時 | 備註 |
|------|------|------|------|
| Prometheus `/metrics` exporter for MCP | expose `mcp_calls_total{tool,status}`, `mcp_call_duration_seconds_bucket{tool}` | 2d | 與 Item 2 部分重疊，可整併 |
| Tool 動態熱加載（不重啟進程） | 從 `cmd/atlas-mcp/tools.d/*.yaml` 讀取新 tool definition | 3d | Phase 4 候選，提前做有價值 |
| MCP Roots + Elicitation 支援 | 實作 2026-07-28 MCP spec 新規範 | 5d | Phase 4 候選，部分提前有價值 |

> **請 user 在 review 時指定**：
> - (i) 不加第 4 項（採 3 項保守，總工時 15d）
> - (ii) 加其中 1-2 項延伸 Phase 3（總工時 17-20d）

## 4. 跨切面議題

每個 Item 都必須遵守：

- **測試覆蓋率**：每個新功能 `*_test.go` 涵蓋，CI 60% 門檻（見根 `AGENTS.md`）
- **文件同步**：[`mcp-deploy.md`](../operations/mcp-deploy.md) 與 [`agent-mcp-server.md`](./agent-mcp-server.md) §11 status block 同步更新
- **事件一致性**：新事件統一通過 `eventbus.Publish()`，監聽端已存在於 `internal/orchestrator/eventbus.go`
- **權限邊界**：不繞過 `internal/apigateway/CONSTITUTION.md`（數據源治理 6 條文 + 3 附錄）
- **JSON tag snake_case**：所有 API parsing struct 對齊 `domain.*` 的 snake_case tag（見根 `AGENTS.md` §高頻陷阱速查）
- **唯讀 Close**：errcheck 處理遵循 `.github/instructions/go-core.instructions.md`（`_ = f.Close()` for read-only）

## 5. 驗收標準（Phase 3 整體完工）

PR #842 + Phase 3 殘餘 3-item 全數完成後：

1. `agent-mcp-server.md` §11 status block 更新：「Phase 3 ✅ COMPLETE — auto-gen desc + behavior analysis + multi-tenant」
2. 74 tools → ≥ 60 個 description 自動產生（保留 ≤ 14 manual override）
3. 2 個新行為分析 tool 上線，audit log v2 schema 啟用
4. Multi-tenant token DB schema + management API 完成
5. `go test -race ./cmd/atlas-mcp/...` 全綠，新增 test 數 ≥ 50
6. `docs/operations/mcp-deploy.md` 反映 v2 behavior（含 multi-tenant + admin API 章節）
7. README `cmd/atlas-mcp/README.md` 章節「Multi-tenant Setup」上線
8. ci-cd.yml pipeline 全綠

## 6. 風險與緩解

| 風險 | 嚴重度 | 緩解 |
|------|--------|------|
| Auto-generated description 質量不穩定 | 中 | 預留 manual override 標記 + Item 1 完成後 survey 4 個工具做 A/B 測試 |
| Behavior analysis 大量 JSONL 解析吃記憶體 | 中 | 用 streaming + bounded window（如 parse 最近 1GB 或 24h） |
| Multi-tenant token DB schema migration 在現有部署失敗 | 高 | 設計 backfill script + dry-run 模式 + 先 staging 跑 |
| Token hash 改變導致現有 env-var token 失效 | 低 | 同步保留 env-var fallback（per §3.3.5 條 3）|
| Rate limit 用 token 而非 IP 造成 IP-spoofing 顧慮消失 | 低 | stdio transport 無此問題；HTTP transport 仍受 TLS + Bearer 保護 |

## 7. 開放議題

1. **Item 4 是否納入？** 請 user 在 review 時決定（見 §3.4 表）
2. **Multi-tenant token storage**：本 spec 採 PostgreSQL；是否改用 Redis（更快但多一個 dependency）？ — 預設 PostgreSQL（apigateway 已有）（可推翻）
3. **Audit log v2 向下相容期**：現有 v1 reader 保留多久？ — 預設永久保留（Go json.Unmarshal 天然忽略未知欄位）（可推翻）
4. **Phase 3 完成的 milestone tag**：採 `v0.0.0.25` 後綴？ — 預設 yes（可推翻）
5. **agent_id 與 session_id 的關係**：一 agent 可有多個 session 嗎？多 agent 共用 session 嗎？

## 8. 參考

| 文件 | 路徑 |
|------|------|
| Phase 2.2 Status | [`agent-mcp-server.md` §11](./agent-mcp-server.md) |
| Phase 3 roadmap 原規劃 | 詳見 `docs/specs/agent-mcp-server-spec.md` §11 |
| Phase 3 PR (上半) | [#842](https://github.com/kaecer68/atlas-go/pull/842) |
| Phase 2.1 transports PR | [#834](https://github.com/kaecer68/atlas-go/pull/834) |
| Apigateway 數據源憲法 | [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) |
| 部署守則 | [`docs/operations/mcp-deploy.md`](../operations/mcp-deploy.md) |
| Go 核心守則 | [`.github/instructions/go-core.instructions.md`](../../.github/instructions/go-core.instructions.md) |

---

**文件版本**：v0.1 DRAFT（2026-06-30）
**下次 review**：user review 後 v1 → 轉給工程師進入實作
