# Wave 12 llm_annotator Phase 2 重構交接（2026-06）

> **文件角色**：任務交接。  
> **來源**：原記載於 `internal/llm_annotator/AGENTS.md`，因屬 Wave 12 Phase 2 完成紀錄搬遷至此。  
> **狀態**：Phase 2 已完成；後續遷移見 `.omo/plans/llm-annotator-removal.md`。

## 背景

`internal/llm_annotator` 是 Wave 4 期間建立的早期 narrow 介面。Wave 11 L2.1 文件盤查（Issue #722）發現它與 `internal/llm/adapters/annotator_adapter.go` 概念重疊，且存在 CircuitBreaker 重複實作與 import cycle 風險。

## 已完成事項（Phase 2）

1. 將 `Annotator` 介面標 deprecation 警告（PR #730）。
2. 統一 CircuitBreaker：打破 transitive cycle（Issue #731, PR #737）。
   - 將 `monitoring.ChannelHealthStore`、`RecordOption`、`ChannelHealthRecord`、`WithLatencyMs`、`WithRateLimitRemaining`、`NewChannelHealthStore`、`NewChannelHealthStoreWithPool`、`RecordChannelFetch`、`RecordChannelFetchWithPool` 搬到 `apigateway/channel_health.go`。
   - `monitoring/channel_health_aliases.go` 提供 type aliases 向後相容。
   - 本套件 `Config.Breaker` 與 `KimiClient.breaker` 改持 `*CircuitBreaker`（薄封裝委派 `*apigateway.CircuitBreaker`）。
   - `internal/llm_annotator/circuit_breaker.go` 重新建立為 wrapper，保留 5 個 Wave 4-era API。
   - `apigateway.CircuitBreaker` 新增 `WithNowFunc` method（用 `atomic.Pointer` 避免 lock re-entry deadlock）。

## 現行規則

新程式碼應使用 `internal/llm/capabilities/failure_attribution.FailureAttributionHandler`，而非 `internal/llm_annotator.Annotator`。詳見 `internal/llm_annotator/AGENTS.md`。
