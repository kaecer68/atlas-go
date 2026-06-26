# AGENTS.md — internal/llm_annotator

> **Deprecated bridge package** — see `doc.go` for the deprecation notice
> and the Wave 12+ follow-up issue. New code should depend on
> `internal/llm/capabilities/failure_attribution` instead.

---

## 角色與現狀

`internal/llm_annotator` 是 Wave 4 期間建立的「LLM 標註器」套件，提供 `Annotator` 介面讓 failure-attribution 路徑可以走 LLM 而非純規則式標註。在 Wave 11 L2.1 文件盤查期間（commit 56868db8），發現以下結構性問題（Issue #722）：

| 議題 | 說明 |
|------|------|
| **概念重疊** | `internal/llm/adapters/annotator_adapter.go` 已成為 Phase 2 canonical 介面（`llm.ProviderImpl`），但 `Annotator` 介面仍保留為「早期 narrow 介面」 |
| **CircuitBreaker 重複實作** | ✅ **已解決**（Wave 12 Phase 2，Issue #731）。本套件 `CircuitBreaker` 已移除，改用 `apigateway.CircuitBreaker`。原本的 4 層 transitive cycle 由於把 `monitoring.ChannelHealthStore` 等型別搬到 `apigateway` 而破壞。 |
| **import cycle 阻擋合併** | ✅ **已解決**（Wave 12 Phase 2，Issue #731） |

## 與 sibling 套件的分工

| 套件 | 角色 | 成熟度 | 引入時機 |
|------|------|--------|---------|
| `internal/llm_annotator` | **早期 narrow 介面保留**（向後相容） | experimental | Wave 4+ |
| `internal/llm/adapters` | **Phase 2 canonical 介面**（`llm.ProviderImpl` wrapper） | evolving | Wave 11 L2.1 |
| `internal/llm/capabilities` | **Phase 2 canonical handler**（12 個 capability） | evolving | Wave 11 L2.1 |

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`Annotator` 介面已 deprecated** | 新程式碼應透過 `internal/llm/capabilities/failure_attribution.FailureAttributionHandler`。`Annotator` 介面僅保留向後相容 |
| **CircuitBreaker 統一在 apigateway（thin wrapper）** | Wave 12 Phase 2 起，本套件的 `*CircuitBreaker` 是 `*apigateway.CircuitBreaker` 的薄封裝，**所有狀態轉移委派給 apigateway canonical owner**。封裝保留：(1) `CircuitState = apigateway.State` 型別別名 + `CircuitClosed/Open/HalfOpen` 常數；(2) `ErrCircuitOpen` sentinel；(3) `Allow() (bool, error)` Wave 4-era helper；(4) `Snapshot() string` 字串視圖；(5) `WithNowFunc(now func() time.Time)` 假時鐘注入（測試必要機制）。`Config.Breaker` 欄位型別為 `*CircuitBreaker`（封裝），由 `newCircuitBreaker()` 建構。 |
| **API key 取得** | `Config.APIKey` 必須透過 `apigateway.MustGet("kimi")` 取得（生產環境），**不可**直接 `os.Getenv`。CI/本地開發才用 `LLM_ANNOTATOR_API_KEY` env var（白名單列於 `configs/allowed_env_vars.md`） |
| **Budget threshold 一次性 callback** | `BudgetCallback` 在累計 token 達 `BudgetThreshold` 時**只會被觸發一次**，內部會 `ForceOpen()` + `SetManualOverride(true)` 熔斷器。**呼叫端不應假設 callback 會多次觸發** |
| **呼叫端 fallback** | `Annotate` 回傳 `ErrUnavailable` 時，**呼叫端必須 fallback 到 `rule_based` 標註**（`strategy_techniques.AttributionMode = rule_based`）。`ErrUnavailable` 不視為可重試錯誤 |

## 開發慣例

- **新 capability handler**：在 `internal/llm/capabilities/` 新增，而非在本套件新增 method。範例見 `internal/llm/capabilities/prompt_lint.go`
- **測試**：使用 mock `Annotator`（見 `internal/llm_annotator/annotator_test.go`），避免真實 API 呼叫
- **CircuitBreaker 測試**：使用 `newCircuitBreaker()`（wrapper 私有建構）建構 breaker；常數為 `apigateway.CircuitBreakerFailureThreshold`。Wrapper 委派給 `apigateway.CircuitBreaker` 提供 `SetManualOverride(true)` + `Reset()` 對應 budget callback 的「停止呼叫直到 operator reset」語意。`WithNowFunc(now)` 提供假時鐘注入，便於 recovery-timeout 邊界測試

## 跨模組整合備忘

- `internal/apigateway.UnifiedHealthStore.Record` 透過 `apigateway.WithLatencyMs` option 函數記錄健康狀態（Wave 12 Phase 2 起從 `monitoring/channel_health.go` 搬到 `apigateway/channel_health.go`，避免 apigateway → monitoring 反向依賴）
- `internal/llm/capabilities/failure_attribution.go` 直接 import 本套件（為了用 `FailureContext` 作為 payload）— Phase 2 起本套件可直接 import `apigateway` 而不產生 cycle（4 層 transitive path 已被切斷）
- `internal/llm/adapters/annotator_adapter.go` 將 `Annotator` 包成 `llm.ProviderImpl`，供 router 整合

## 相關文件

- `internal/llm/AGENTS.md` — Phase 2 canonical LLM 框架說明
- `internal/llm/doc.go` — LLM 模組公共 API 速查
- `internal/apigateway/AGENTS.md` — apigateway 模組（**Phase 2 起為 CircuitBreaker canonical owner**）
- `docs/llm-integration-strategy-framework.md` — 設計權威
- `internal/MATURITY.md` — `llm_annotator` 與 `llm` 條目

## 重構狀態

`internal/llm_annotator` 已 deprecated；Phase 2 canonical 介面已就緒（Wave 12，Issue #731）。已完成事項見 `docs/handoff/2026-wave12-llm-annotator-phase2.md`；後續遷移計劃（未實作、工作區限定）見 `.omo/plans/llm-annotator-removal.md`。