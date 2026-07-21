# AGENTS.md — internal/live

實盤交易協調。可靠性邊界：replay / simulation 為可靠預設；`internal/live` 仍有部分整合 TODO。詳見 `.github/instructions/live-trading.guardrails.instructions.md`。

## 核心職責

| 職責 | 檔案 |
|------|------|
| 交易編排 | `orchestrator.go` |
| 券商抽象 | `broker.go` |
| 訂單管理 | `order_manager.go` |
| 熔斷 | `circuit_breaker.go` |
| 狀態持久化 | `store/store.go` |
| Nonce 管理 | `store/nonce_store.go` |
| HTTP Adapter | `http_adapter.go` |
| 事件匯流排 | `eventbus.go` |

## CONVENTIONS

- 預設 dry-run；必須顯式啟用 live broker（`-allow-live-broker`）。
- 狀態更新使用 `StateStore` 原子寫入。
- 測試必須設定 `cfg.BrokerMode = "paper"`。

## ANTI-PATTERNS

- 本地測試切勿使用 `-allow-live-broker` / `-allow-real-signor`。
- `SubmitOrder` 前必須確認 `Broker.Mode()`。
- 不可直接 `os.WriteFile` 寫入狀態。
- 不可繞過 `CircuitBreaker` 直接呼叫 `Broker`。
- nonce 必須單次有效。

## EventPositionUpdate

`orchestrator.go` 向 `BaselineTrigger` 與 `DriftDetector` 發布 `EventPositionUpdate`（目前只發 `"updated"`）。歷史設計詳見 `內部交接（.omo/handoffs/）`（Wave 9 遺留命名）。
