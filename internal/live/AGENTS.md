# AGENTS.md — internal/live

本目錄負責**實盤交易協調**，是系統從「模擬優先」過渡到「實盤執行」的最後一道閘門。

> **可靠性邊界**（`.github/instructions/live-trading.guardrails.instructions.md`）：replay / simulation 為可靠預設；`internal/live` 仍有部分整合 TODO。

---

## 已知結構債務（P3）

`doc.go` 明確標記本套件為混合職責，部分拆分已 ship、剩餘仍 TODO：

- `live/store/` — Redis nonce store、狀態持久化（基礎設施）**已 ship**（見 `store/store.go` + `store/nonce_store.go`，package alias `livestore`，從 `agent_runner.go:9` import）
- `live/broker/` — 券商執行、訂單管理（業務邏輯）**TODO**
- `live/http/` — HTTP broker adapter（基礎設施）**TODO**

**Same-package 拆分原則**：新增功能應註明屬於哪一類（業務 vs 基礎設施），為剩餘拆分做準備。

---

## 核心職責（簡要）

| 職責 | 檔案 |
|------|------|
| 交易編排 | `orchestrator.go` — 整合 StateStore / EventBus / Broker / OrderManager / CircuitBreaker |
| 券商抽象 | `broker.go` — `Broker` 介面；`DryRunBroker` 不送真實委託；`GuardedLiveBroker` 無 adapter 拒單 |
| 訂單管理 | `order_manager.go` — pending → submitted → filled/rejected/cancelled |
| 熔斷 | `circuit_breaker.go` — 連續錯誤達閾值暫停；持久化 `circuit_breaker_state.json` |
| 狀態持久化 | `store/store.go` — `livestore.StateStore` 原子寫入（部位/訂單/現金） |
| Nonce 管理 | `store/nonce_store.go` — 防重放；預設 Redis，測試可換記憶體實作 |
| HTTP Adapter | `http_adapter.go` — 券商 API（HMAC-SHA256、速率限制、重試） |
| 事件匯流排 | `eventbus.go` — `ChannelEventBus` 解耦 |

詳細契約見 `doc.go:1-23` (package documentation block)。

---

## CONVENTIONS

- **預設安全**：所有實盤功能預設為 dry-run，必須顯式啟用 live broker（`-allow-live-broker`、`-allow-real-signor`）
- **原子寫入**：狀態更新必須使用 `StateStore` 提供的原子檔案寫入
- **Context 統一**：所有長時間運行的操作必須接受 `context.Context`，支援 graceful shutdown
- **BrokerMode 強制**：測試工具必須設定 `cfg.BrokerMode = "paper"`

---

## ANTI-PATTERNS

- **意外啟用實盤**：本地測試切勿使用 `-allow-live-broker` / `-allow-real-signor`
- **未檢查 Broker 模式**：`SubmitOrder` 前必須確認 `Broker.Mode()`，避免 dry-run 預期下送真實委託
- **StateStore 非原子寫入**：直接 `os.WriteFile` 可能導致狀態損毀
- **忽略 CircuitBreaker**：不可繞過 `CircuitBreaker` 直接呼叫 `Broker`
- **Nonce 重複使用**：`nonce_store.go` 產生的 nonce 必須單次有效

---

## KEY TYPES

對應「核心職責」表中的檔案：`Orchestrator`（`orchestrator.go`）、`Broker` 系列（`broker.go`）、`OrderManager`、`CircuitBreaker`、`StateStore`、`ChannelEventBus`、`Scheduler` / `AgentRunner`。

---

## 測試與驗證

```bash
go run ./cmd/experimental/validate-broker   # Broker 簽名格式驗證（dummy 模式）
```

完整驗證清單見 `.github/instructions/live-trading.guardrails.instructions.md`。

---

## EventPositionUpdate 生產發布

`orchestrator.go` 是 `EventPositionUpdate` 的生產呼叫者之一，向 `BaselineTrigger` 與 `DriftDetector` 發布更新；目前只發 `"updated"`，新增/移除部位由模擬引擎與訂單成交路徑負責。詳見 `docs/handoff/2026-wave9-event-position-update.md`。
