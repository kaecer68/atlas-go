# Atlas MCP Server — Phase 4 提案規格

> **Audience**：架構師 + user 決策。
> **狀態**：🔴 PROPOSAL（2026-06-30，待 user 從 §3 方向候選中選定）
> **範圍決策**：Phase 3 完工後的下一步。Roadmap 沒列；本 spec 由 (i) Phase 2.2 status 趨勢 (ii) Phase 3 上半 prune (iii) MCP 2026-07-28 spec 演進 (iv) 開源 ecosystem 觀察共同推導。
> **關聯文件**：
> - [`agent-mcp-server.md`](./agent-mcp-server.md)（核心 spec，含 §11 Phase 2.2 Status）
> - [`agent-mcp-phase3-residual.md`](./agent-mcp-phase3-residual.md)（先決）
> - 對齊 agent-interface-roadmap.md roadmap;本 Phase 4 為推導新增(未列於 roadmap,roadmap snapshot 詳見 PR #876)
> - MCP 2026-07-28 spec: <https://modelcontextprotocol.io/specification/2026-07-28>

---

## 1. 為什麼需要 Phase 4

Phase 3 結束時，atlas-mcp 將達到：

- **74 tools**（per [`agent-mcp-server.md` §3.1](./agent-mcp-server.md#3-mcp-tools-清單) 候選清單）
- **3 transports**（stdio / SSE / streamable-HTTP）+ **Bearer auth** + **per-tool rate-limit** + **audit retention**
- **auto-gen description / behavior analysis / multi-tenant token**

但「穩定可用」不等於「production-grade」。Phase 4 必然的壓力點：

1. **真實 agents 的行為反饋**：Phase 3 殘餘的 behavior analysis tool 會 feed 大量資料，需要 (a) automated anomaly detection、(b) usage-based 優化工具鏈
2. **MCP protocol 演進**：2026-07-28 spec 帶來 sampling / roots / elicitation 新能力 — 支援後讓 agent 能呼叫 MCP 內的 LLM、共享檔案系統、主動向使用者提問
3. **Multi-instance federation**：不同團隊 / 不同機器各自跑 atlas-mcp，需要跨 instance shared state（audit、tokens、tool catalog）
4. **長期 agent**：現有 74 tool 都是 short-lived（< 5s）；部分場景（backtest、超參搜索）需要 long-running tool（task graph），否則 transport connection 會被佔滿

## 2. 設計目標

挑選 1-2 條方向後，產出 production-grade MCP server：

- **可觀測**：agent 行為 anomaly 即時告警
- **可擴展**：新 MCP protocol features 30 天內跟進
- **可聯合**：跨 instance 狀態同步（條件性）
- **可承受長期任務**：long-running tool 不阻塞 transport

## 3. 不做 (Out of Scope)

- **新建獨立 atlas-mcp cluster**：本 Phase 4 仍是單 binary 架構；multi-cluster 留給未來
- **新 LLM provider**：沿用 `DefaultRouter` 集中入口（見 [`CLAUDE.md`](../../CLAUDE.md) §LLM 路由）
- **修改 74 既有 tool**：以新增 / 補強為主，避免破壞既有 schema contract
- **GUI / admin_web 變更**：本 Phase 4 專注 MCP server 本身

## 4. 開放議題（需 user 決定方向）

以下 3 條候選方向擇 1（或採 hybrid），每條帶 trade-offs：

### 4.1 Direction A — Observability/Diagnostics-First

**核心新增**：MCP 內建 Prometheus `/metrics` + 自訂 anomaly detector + alert integration。

**符合痛點**：Phase 3 殘餘的 behavior analysis 留下的 raw event stream → 升級為 actionable signal。

**新 tool 範例**：
- `mcp_get_anomaly_history`
- `mcp_get_top_slow_tools`
- `mcp_get_tenant_usage`（per-tenant quota consumption）

**整合**：接 [`internal/alerting/`](../../internal/alerting/) 既有 alert 系統。

**工時估算**：~10 工作天

### 4.2 Direction B — Protocol Extension-First (sampling / roots / elicitation)

**核心新增**：實作 MCP 2026-07-28 spec 三大新 primitives。

| Primitive | 內容 | 對 agent 的價值 |
|-----------|------|----------------|
| Sampling | MCP server 主動呼叫 client 的 LLM | Agent 不需自己帶 LLM，MCP 提供 |
| Roots | Agent 共享檔案系統路徑給 MCP server | Atlas 可讀取 agent 的 workspace 檔案 |
| Elicitation | MCP server 主動向 user 詢問（如補 input） | Atlas 缺資料時互動式詢問 |

**符合痛點**：未來 agent 愈做愈多「需要 LLM 判斷 / 用戶協商」的工作，這些是必備基建。

**對 atlas-go 衝擊**：roots 需存取 filesystem → 撞 [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) 數據源治理邊界（具體 file access 條文待確認）。

**工時估算**：~14 工作天（含 spec 學習 + 三 primitive 各 4-5d）。

> **技術備註**：目前 `go-mcp-sdk v1.6.1`（現用版本）已原生支援三 primitives — `ServerSession.Elicit()`、`ServerSession.ListRoots()`、`RootsListChangedHandler`、`ElicitHandler`。無需版本 bump，主要工作量在應用層 handler 設計 + apigateway Constitution 邊界釐清。

### 4.3 Direction C — Agent Capability Expansion（long-running + code exec）

**核心新增**：async task graph + sandboxed shell exec。

**符合痛點**：現有 74 tool 同步阻塞；backtest 60s+ 會佔 transport connection。

**新概念**：
- `mcp_create_task` / `mcp_get_task` / `mcp_wait_task` — async task lifecycle
- `mcp_run_code` — 沙盒執行 Python snippets（受限 runtime）

**整合**：複用 Phase 3 殘餘的 `scheduler_*` / `task_*` tool 集。

**工時估算**：~20 工作天（sandbox 是大工程）。

---

## 5. 推薦方向

> **預設推薦**：**A (Observability) + B (Protocol Extension) hybrid**，分 2 個 sub-PR。

**理由**：

- A 補齊 Phase 3 殘餘的 behavior analysis 的後半段（從 raw event → actionable signal），是 natural continuation
- B 為 MCP 2026-07-28 spec 必備 — 不做會被 ecosystem 甩開，且 atlas-go `apigateway` 已有 file access 邊界基礎
- C 過大、scope creep 風險高、應留到 Phase 5 評估

**總工時估算**：~24 工作天（A:10d + B:14d，可與 A 重疊 4d 並行 sub-team）。

---

## 6. 若選 A — 詳細 scope（Direction A 預設展開）

> 僅 fill user 選定方向詳細展開；B / C 詳列於 §6.2 / §6.3 附錄。

### 6.1 Direction A — Observability 詳細展開

#### 6.1.1 Prometheus Exporter

**位置**：`/metrics` endpoint，bind 127.0.0.1（與 MCP server 主 port 分開；如 audit log 暴露同樣考量）。

**Metrics 設計**：
- `mcp_calls_total{tool, transport, status}` — Counter
- `mcp_call_duration_seconds{tool, transport}` — Histogram
- `mcp_active_sessions{transport}` — Gauge
- `mcp_token_usage_total{tenant_id}` — Counter（Phase 3 multi-tenant 用）
- `mcp_anomaly_score{tenant_id, anomaly_type}` — Gauge

**整合**：Phase 3B rate-limit counter 重新導出，Phase 3 殘餘 behavior analysis 結果也導出。

#### 6.1.2 Anomaly Detector

**位置**：`internal/mcp/anomaly/`

**演算法**：rolling window + z-score，3 種基線：
- 短窗（5 分鐘）vs 24h 基線 → burst detection
- per-tool error rate 突增 → anomaly_score
- per-tenant error rate 異常 → tenant-misuse detection

**Output**：anomaly_event 通過 `eventbus.Publish()` → 既有 alert system 觸發。

#### 6.1.3 Alert Integration

**新增 tool**：
- `mcp_anomaly_get_recent` — 列出近 N 條 anomaly event
- `mcp_anomaly_ack` — 確認 anomaly（與 HTTP endpoint `/api/alerts/acknowledge` 共用底層 alert store）

**整合**：[`internal/alerting/`](../../internal/alerting/) ChannelHealth aggregator 接入 `mcp_anomaly_*` 為一個 source。

#### 6.1.4 驗收

- [ ] `/metrics` endpoint 暴露 5 種 metrics，可 curl 取得 Prometheus 格式
- [ ] anomaly detector 3 種基線 unit test 通過
- [ ] `mcp_anomaly_get_recent` tool 整合測試（與 alert system 互動）
- [ ] 30 天 audit log 內 anomaly 偵測延遲 < 1 分鐘
- [ ] integration test 模擬 burst / error-spike 兩種 anomaly 情境

#### 6.1.5 工時

10 工作天。

### 6.2 Direction B 概要（待 user 確認後展開）

若 user 選 B，A 與 B 並行：

- 6.2.1 Sampling server-side 實作 + 測試
- 6.2.2 Roots read-only boundary 設計 + 對接 apigateway Constitution
- 6.2.3 Elicitation schema 設計 + UI client 端考量（agent 端，非 web UI）

### 6.3 Direction C — 推遲到 Phase 5 評估

Reasoning：sandbox 涉及 OS-level 隔離（gVisor / nsjail / bwrap），工作量 + 風險太高，應獨立 milestone。

---

## 7. Phase 4 整體驗收標準（依選定方向）

待 user 選定方向後，從下表挑選填入：

| 條件 | A | B | A+B | 備註 |
|------|---|---|-----|------|
| 新 metrics / endpoint / protocol primitive | 5 metrics | 3 primitives | 5+3 | 各方向獨立驗收 |
| 新 MCP tool 數 | 3 | 1-2 | 4-5 | 從候選映射到實際 |
| 整合既有系統 | alerts / eventbus | apigateway Constitution | 兩者 | — |
| 文件更新 | `mcp-deploy.md` + spec §11 | spec §11 + apigateway 條文 | 同 A+B | — |
| 測試新增 | ≥ 30 | ≥ 40 | ≥ 70 | — |

---

## 8. 風險與緩解

| 風險 | 嚴重度 | 緩解 |
|------|--------|------|
| Phase 3 殘餘未完成時啟動 Phase 4（依賴未達） | 高 | §7 驗收條目加 Phase 3 完成 gate；CI 自動阻擋 |
| Prometheus metrics name 與現有監控衝突 | 中 | 加 `mcp_` 前綴隔離 namespace |
| Sampling 呼叫 LLM 增加 latency | 中 | 預設 off + opt-in flag |
| Roots 寫穿 apigateway Constitution | 高 | 預設 read-only 邊界 + audit log 全部 file read |
| Multi-instance federation 需求浮上 | 中 | A + B 完成後做 retro 評估，留 §4.3 之後 |
| Anomaly detector false positive 過多 | 中 | 用戶可調 threshold + 預設保守閾值 |

## 9. 開放議題

1. **方向選擇**：A / B / C / hybrid（A+B）？ — 需 user 決定（見 §4）
2. **若選 A**：Prometheus 是否同時 exposing 給外部 Prometheus scrape？ — 預設僅 bind 127.0.0.1（如需公開，過 WAF）
3. **若選 B**：roots 的 filesystem boundary 採 read-only 還是 read-write？ — 預設 read-only（撞 apigateway 較安全）
4. **若選 C（暫不考慮）**：code sandbox 採 gVisor / nsjail / bwrap 哪一種？ — 預設 gVisor（MCP 多 client 環境最常見），留 Phase 5 評估
5. **Phase 4 是否引入新 LLM provider**？ — 預設不（沿用 `DefaultRouter`）
6. **Phase 4 完成後是否開源 atlas-mcp**：roadmap §5 議題延續到 Phase 4 決定
7. **multi-tenant + multi-instance 是否為 Phase 4 子項**：目前歸在 §1 壓力點 #3 但沒列方向

## 10. 參考

| 文件 | 路徑 |
|------|------|
| Phase 2.2 Status | [`agent-mcp-server.md` §11](./agent-mcp-server.md) |
| Phase 3 殘餘 | [`agent-mcp-phase3-residual.md`](./agent-mcp-phase3-residual.md) |
| Phase 3 PR (上半) | [#842](https://github.com/kaecer68/atlas-go/pull/842) |
| Phase 2.1 transports PR | [#834](https://github.com/kaecer68/atlas-go/pull/834) |
| MCP 2026-07-28 spec | <https://modelcontextprotocol.io/specification/2026-07-28> |
| Roadmap (規劃藍圖) | 詳見 `docs/specs/agent-mcp-server-spec.md` §11 |
| Apigateway 數據源憲法 | [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) |
| Alert system | [`internal/alerting/`](../../internal/alerting/) |
| 部署守則 | [`docs/operations/mcp-deploy.md`](../operations/mcp-deploy.md) |

---

**文件版本**：v0.1 PROPOSAL（2026-06-30）
**下次 review**：user 確定 §4 方向後升 v1 → 與 Phase 3 殘餘 spec 連動進入實作 sprint 規劃
