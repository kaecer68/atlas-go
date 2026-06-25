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
| **CircuitBreaker 重複實作** | 本套件的 `CircuitBreaker` 與 `internal/apigateway.CircuitBreaker` 行為對齊但獨立維護，差異在 `manualOverride` 機制 |
| **import cycle 阻擋合併** | transitive 路徑 `apigateway → monitoring → llm/capabilities → llm_annotator` 阻擋合併 CircuitBreaker |

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
| **CircuitBreaker 重複** | 本套件 `CircuitBreaker` 與 `apigateway.CircuitBreaker` 獨立維護，差異在 `manualOverride` 設定。**不可混合使用**：`BudgetCallback` 必須用本套件 breaker；channel 級熔斷必須用 apigateway breaker |
| **API key 取得** | `Config.APIKey` 必須透過 `apigateway.MustGet("kimi")` 取得（生產環境），**不可**直接 `os.Getenv`。CI/本地開發才用 `LLM_ANNOTATOR_API_KEY` env var（白名單列於 `configs/allowed_env_vars.md`） |
| **Budget threshold 一次性 callback** | `BudgetCallback` 在累計 token 達 `BudgetThreshold` 時**只會被觸發一次**，並 ForceOpen（`SetManualOverride`）熔斷器。**呼叫端不應假設 callback 會多次觸發** |
| **呼叫端 fallback** | `Annotate` 回傳 `ErrUnavailable` 時，**呼叫端必須 fallback 到 `rule_based` 標註**（`strategy_techniques.AttributionMode = rule_based`）。`ErrUnavailable` 不視為可重試錯誤 |

## 開發慣例

- **新 capability handler**：在 `internal/llm/capabilities/` 新增，而非在本套件新增 method。範例見 `internal/llm/capabilities/prompt_lint.go`
- **測試**：使用 mock `Annotator`（見 `internal/llm_annotator/annotator_test.go`），避免真實 API 呼叫
- **CircuitBreaker 測試**：使用 `internal/llm_annotator.CircuitBreaker` 而非 `apigateway.CircuitBreaker` — 兩者行為不同（前者有 manualOverride）

## 跨模組整合備忘

- `internal/monitoring/dashboard_api.go` 透過 `monitoring.WithLatencyMs` option 函數呼叫 `apigateway.UnifiedHealthStore.Record`，option 函數定義於 `monitoring/channel_health.go`
- `internal/llm/capabilities/failure_attribution.go` 直接 import 本套件（為了用 `FailureContext` 作為 payload）— 這是 cycle 阻擋合併的環節
- `internal/llm/adapters/annotator_adapter.go` 將 `Annotator` 包成 `llm.ProviderImpl`，供 router 整合

## 相關文件

- `internal/llm/AGENTS.md` — Phase 2 canonical LLM 框架說明
- `internal/llm/doc.go` — LLM 模組公共 API 速查
- `docs/llm-integration-strategy-framework.md` — 設計權威
- `internal/MATURITY.md` — `llm_annotator` 與 `llm` 條目

## 重構時程（追蹤 issue 待開）

Phase 2 canonical 介面已就緒後，本套件保留至 Wave 12+ 由 follow-up issue 統一處理：

1. 將 `Annotator` 介面標 deprecation 警告（本檔已記錄）
2. 將所有 `*Annotator` 直接呼叫遷移到 `*FailureAttributionHandler`
3. 統一 CircuitBreaker：打破 transitive cycle，將 `monitoring.ChannelHealthStore` 內 `RecordOption` 與 `ChannelHealthRecord` 搬到 `apigateway` 內，本套件改用 type alias
4. 刪除 `CircuitBreaker` 重複實作
5. MATURITY.md 標 `llm_annotator` 為 deprecated