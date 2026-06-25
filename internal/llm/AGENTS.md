# AGENTS.md — internal/llm

本目錄是 `atlas-go` 的 **LLM 整合基礎設施層**，提供 capability-based 多 Provider 路由 + DataClass 治理閘門 + 熱路徑護欄。

> **設計權威**：`docs/llm-integration-strategy-framework.md`（4-digit semver 規範）。
> **路由表來源**：`configs/llm_router.yaml`（runtime 載入）；fallback 預設見 `router.go:defaultRoutingTable()`。
> **成熟度規則**：`internal/MATURITY.md` LLM 相關條目（`llm` / `llm/schemas` / `llm/clients` / `llm/capabilities` / `llm_annotator`）。

---

## 核心職責

- **Provider 抽象**（`provider.go`）：定義 `ProviderImpl` 介面、`Capability`（能力描述）、`DataClass`（資料分級）、`RoutingChain`（備援鏈）。
- **路由引擎**（`router.go`）：`DefaultRouter` 實作 Primary → Backup1 → Backup2 → LastResort 4 層 fallback chain；強制執行 DataClass 閘門（`ADR-010`）。
- **設定載入**（`config.go`）：`LoadRouterConfig(path)` 讀 YAML 並驗證所有 capability/provider 名稱；`TryLoadRouterConfig` 在錯誤時 fallback 預設表。
- **健康端點**（`health.go`）：聚合所有已註冊 Provider 的 `HealthStatus` 與 circuit breaker 狀態，供 `/api/llm/health` 暴露。

## 子模組佈局

| 子目錄 | 行數規模 | 內容 |
|--------|---------|------|
| `clients/` | ~3K | 3 個 HTTP Provider client（DeepSeek V4 / MiniMax M3 / Kimi K2.7）+ 共享 `BaseClient`（retry / rate-limit / circuit breaker）+ `metrics.go` |
| `schemas/` | ~2K | 9 個 capability 的 typed I/O contract（JSON-serialized, Zod-compatible JSON Schema） |
| `capabilities/` | ~3K | 10 個 capability handler（typed payload → Router → typed response） |
| `adapters/` | ~1K | Annotator / Router 整合層，給非 LLM-aware 模組（e.g. `internal/llm_annotator`）注入 |

修改任何子模組前，**必須先讀該子目錄的對應 skill**：

- `clients/` → `.claude/skills/generated/clients/SKILL.md`（若存在，否則直接讀碼）
- `capabilities/` → handler-specific 測試檔（每個 capability 一個 `*_test.go`）
- `schemas/` → `schemas_test.go`

---

## 公共 API 速查

完整清單見 `doc.go:15-30`。以下是常用對外契約：

| 函數 / 介面 | 用途 | 觸發位置 |
|------------|------|---------|
| `NewDefaultRouter(impls ...ProviderImpl) *DefaultRouter` | 啟動時 wiring router with default routing table | `cmd/atlas/main.go`, `cmd/lint-pr/main.go`, `cmd/lint-prompts/main.go` |
| `NewDefaultRouterFromConfig(cfg RouterConfig, impls ...ProviderImpl)` | 載入 `configs/llm_router.yaml` 後呼叫 | `cmd/atlas/main.go` |
| `TryLoadRouterConfig(path string) RouterConfig` | 啟動時載入，失敗 fallback 預設 | `cmd/atlas/main.go` |
| `DefaultRouter.Call(ctx, Request) (Response, error)` | 任何 LLM 呼叫的入口 | 透過 capability handler |
| `DefaultRouter.Health() map[Provider]HealthStatus` | `/api/llm/health` 端點 | `internal/monitoring/api/llm/handlers.go` |
| `Capability` / `DataClass` / `Provider` 型別 | 強型別契約 | 跨模組共用 |

### 12 個 Capability（命名須與 `provider.go:28-41` 完全一致）

| Constant | 設計來源（doc §6.1） | 典型用途 |
|----------|----------------------|---------|
| `CapabilityFailureAttribution` | `strategy.failure_attribution` | StrategyFrame 失效歸因 |
| `CapabilityCodeReviewAnnotation` | `dev.code_review_annotation` | PR review 自動標註 |
| `CapabilityPromptLint` | `dev.prompt_lint` | Prompt template 健康檢查 |
| `CapabilityRationaleGeneration` | `narrative.rationale_translation_fallback` | 中文敘事翻譯/生成 |
| `CapabilityStrategySummary` | `strategy.frame_summary` | StrategyFrame 摘要 |
| `CapabilityRiskSurfaceExtraction` | `spawning.gap_description_enrichment` | 風險面提取 |
| `CapabilityRegimeExplanation` | `narrative.event_headline` | Regime 變化解釋 |
| `CapabilityPerformanceForensics` | `risk.confidence_calibration_commentary` | 表現歸因 |
| `CapabilityScenarioSimulation` | `orchestrator.prism_cohort_insight` | PRISM cohort 洞見 |
| `CapabilitySentimentExplanation` | `narrative.sentiment_explanation` | 情緒指標解釋 |
| `CapabilityContraAttribution` | Phase 2 擴充 | 反向歸因（adversarial） |
| `CapabilityConfidenceCommentary` | Phase 3.3 非阻塞 bypass | 信心校準旁路 |

新增 capability **必須同步 4 個位置**（CI 強制）：
1. `provider.go` Capability 常數
2. `router.go:defaultRoutingTable()` routing chain
3. `config.go:isKnownCapability()` switch case
4. `configs/llm_router.yaml` routing_chains entry

---

## 陷阱與反模式

### 1. 直接呼叫 Provider（繞過 Router）
**症狀**：在業務邏輯直接 `minimaxClient.Call(ctx, req)` 跳過 router。
**後果**：繞過 DataClass 閘門 → regulated 資料可能送中國管轄 provider；繞過 circuit breaker；繞過 fallback chain；observability span 缺失。
**正確**：一律透過 `DefaultRouter.Call()` 或對應的 `capabilities/*Handler`。

### 2. 在 hot-path（S/E-level）直接 import
**症狀**：`internal/sim/`、`internal/experiment/` 等 S/E 模組 import `internal/llm/`。
**後果**：模擬執行被 LLM 網路延遲拖慢，replay 可重現性破壞。
**正確**：hot-path 呼叫必須 async 或 fallback-safe（見 `doc.go:32-33`）；觀察窗口內使用 deterministic 預設值。

### 3. MiniMax DataClass 閘門繞過
**症狀**：呼叫時 `req.Options.ForceProvider = &ProviderMiniMax` + `req.DataClass = DataClassRegulated` 試圖強制走 MiniMax。
**後果**：router 會回 `ErrProviderDisabled`（`router.go:110`）。這是有意設計（ADR-010），不可繞過。
**正確**：regulated 資料只能走 DeepSeek / OpenCode-Go / Mock 路徑。

### 4. Kimi K2.7 已被移除
**症狀**：使用 `LLM_ANNOTATOR_API_KEY` 配 Kimi API。
**後果**：`LLM_ANNOTATOR_API_KEY` 與 `LLM_MINIMAX_API_KEY` 同源（`sk-cp-` 前綴的 minimax-cn-coding-plan key），呼叫 Kimi 端點會被拒絕。
**正確**：Phase 1 起改用 MiniMax client。`LLM_ANNOTATOR_API_KEY` 為向後相容保留，新程式碼用 `LLM_MINIMAX_API_KEY`。

### 5. ProviderOpenAI 已 deprecated
**症狀**：使用 `ProviderOpenAI` 常數。
**後果**：保留是為向後相容；新整合應使用 `ProviderKimi` / `ProviderMiniMax` / `ProviderDeepSeek` / `ProviderOpenCodeGo` / `ProviderOpenCodeZen` / `ProviderMock`。

### 6. Tool 呼叫的 InputSchema 須為 JSON Schema (Zod-compatible)
**症狀**：`Tool.InputSchema` 傳 Go struct。
**後果**：router 與 providers 預期 raw JSON；Go struct 序列化會被當成空 payload。
**正確**：使用 `json.RawMessage` 內含標準 JSON Schema 字串。

---

## 開發慣例

### 新增 Provider client

1. 在 `clients/` 新建 `<name>.go`，**嵌入 `BaseClient`** 取得 retry / rate-limit / circuit breaker。
2. 實作 `ProviderImpl.Supports(cap Capability) bool` — 列出支援的 capability 清單。
3. 實作 `ProviderImpl.Call(ctx, req Request) (Response, error)` — 序列化 request 到 provider 原生格式，回傳 `Response{Provider, Usage, Latency}`。
4. 在 `router.go:isKnownProvider()` 加入新 provider。
5. 在 `provider.go` Provider 常數區塊宣告。
6. **禁止**硬編碼 API key — 一律透過 `apigateway.Fetch(channelID)` 或啟動時注入。

### 新增 Capability handler

1. 在 `capabilities/` 新建 `<name>.go`，定義 `<Name>Handler` struct + `New<Name>Handler(router llm.Router)` constructor。
2. 實作 typed `<Name>Payload` 與 `<Name>Response`（放在 `schemas/`）。
3. handler 內部把 payload 包成 `llm.Request{Capability, Payload, DataClass, Options}`，呼叫 `router.Call()`，unmarshal 回 typed response。
4. 在 `capabilities/<name>_test.go` 寫 mock router 測試。

### 修改 routing table

- 改 `configs/llm_router.yaml`（runtime 來源）— **首選**。
- 改 `router.go:defaultRoutingTable()`（fallback 預設，僅在 YAML 不可用時生效）。
- 兩者**必須同步**，否則重啟時行為可能漂移。

---

## 環境變數與 secrets

| 變數 | 用途 | 來源 |
|------|------|------|
| `LLM_MINIMAX_API_KEY` | MiniMax M3 API key（`sk-cp-` 前綴） | 從 minimax-cn-coding-plan 取得 |
| `LLM_DEEPSEEK_API_KEY` | DeepSeek V4-Pro / V4-Flash | 從 https://platform.deepseek.com 取得 |
| `LLM_OPENCODE_GO_API_KEY` | OpenCode-Go 本地/自架（若啟用） | 自架部署決定 |
| `LLM_ANNOTATOR_API_KEY` | **向後相容** — 等同 `LLM_MINIMAX_API_KEY` | 歷史誤標，保留 |
| `LLM_RATIONALE_TRANSLATION_ENABLED` | 啟用 rationale 翻譯 hook | default `false` |
| `LLM_PRISM_SCENARIO_ENABLED` | 啟用 PRISM scenario 說明 hook | default `false` |
| `LLM_NARRATIVE_EXPLAIN_ENABLED` | 啟用 regime + sentiment 解釋 hook | default `false` |
| `LLM_RISK_FORENSICS_ENABLED` | 啟用 performance forensics hook | default `false` |

---

## 觀察窗口（Observation Window）

依據 `sector_agent_llm.go` 設計：

- **`SectorAgentLLM.LLM == nil`** 時 runner 回 `ErrNotImplemented`，deterministic 路徑保留 — 這是**預期**行為，不是 bug。
- Feature flag `UseLLMSectorAgents` 控制 LLM-driven sector agents 啟用（預設關閉）。
- 在觀察窗口內，backtest 必須使用 deterministic 路徑以保證可重現性。

---

## 測試慣例

```bash
# 聚焦測試
go test ./internal/llm/...
go test ./internal/llm/clients/...
go test ./internal/llm/capabilities/...
go test ./internal/llm/adapters/...

# Integration（含 router + clients 端對端）
go test -run Integration ./internal/llm/...
```

新增 capability 時**必須**寫 handler 單元測試（mock router），確認：

1. typed payload 正確轉 `llm.Request`（含 DataClass）
2. router 回 `llm.Response` 正確 unmarshal 回 typed response
3. router 回錯時 handler 回有意義的 Go error（不可吞錯）
4. fallback chain 行為（primary fail → backup1 → ... → lastResort）符合預期

---

## 相關文件

- `docs/llm-integration-strategy-framework.md` — 設計權威
- `docs/llm-promotion-evaluation.md` — LLM 晉升評估流程
- `docs/llm-trigger-analysis.md` — LLM 觸發點分析
- `configs/llm_router.yaml` — runtime routing table
- `internal/MATURITY.md` — LLM 相關條目（`llm` / `llm/schemas` / `llm/clients` / `llm/capabilities` / `llm_annotator`）
- `.claude/skills/generated/llm/SKILL.md` — 生成的技能摘要（若存在）