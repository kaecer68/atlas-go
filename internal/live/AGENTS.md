# AGENTS.md — internal/live

本目錄負責**實盤交易協調**，是系統從「模擬優先」過渡到「實盤執行」的最後一道閘門。

---

## OVERVIEW

`internal/live` 提供實盤交易編排、券商執行、訂單管理、熔斷機制與狀態持久化。本套件目前混合基礎設施與業務邏輯，是已知的結構債務（P3 — 未來重構目標）。

**已知限制**：`doc.go` 明確標記本套件為混合職責，未來將拆分為：
- `live/broker/` — 券商執行、訂單管理（業務邏輯）
- `live/store/` — Redis nonce store、狀態持久化（基礎設施）
- `live/http/` — HTTP broker adapter（基礎設施）

---

## 核心職責

### 1. 交易編排 (`orchestrator.go`)
- `Orchestrator` 是實盤交易的核心協調器，管理市場資料訂閱、訂單執行、部位追蹤與風險監控。
- 整合 `StateStore`、`EventBus`、`Broker`、`OrderManager`、`CircuitBreaker`。
- 透過 `Scheduler` 與 `AgentRunner` 驅動交易決策循環。

### 2. 券商抽象 (`broker.go`)
- `Broker` 介面定義最小下單能力。
- `DryRunBroker`：預設實作，**永遠不送真實委託**，僅回傳可稽核結果。
- `GuardedLiveBroker`：Phase 6 實盤骨架，未配置 adapter 時**永遠拒單**。
- `LiveExecutionAdapter`：實盤 adapter 的最小能力介面。

### 3. 訂單管理 (`order_manager.go`)
- 訂單生命周期管理：pending → submitted → filled/rejected/cancelled。
- 與 `Broker` 互動執行下單，並追蹤成交狀態。

### 4. 熔斷機制 (`circuit_breaker.go`)
- 當連續錯誤或異常狀態達到閾值時，暫停交易以防止損失擴大。
- 狀態持久化至 `circuit_breaker_state.json`。

### 5. 狀態持久化 (`store.go`)
- `StateStore` 負責實盤狀態的讀寫（部位、訂單、現金）。
- 採用原子寫入，防止狀態損毀。

### 6. Nonce 管理 (`nonce_store.go`)
- 防止重放攻擊的 nonce 產生與驗證。
- 預設使用 Redis，單元測試時可替換為記憶體實作。

### 7. HTTP Adapter (`http_adapter.go`)
- 券商 API 的 HTTP 客戶端實作，含 HMAC-SHA256 簽名。
- 整合速率限制與重試邏輯。

### 8. 事件匯流排 (`eventbus.go`)
- `ChannelEventBus`：基於 Go channel 的內部事件發布/訂閱機制。
- 用於解耦 orchestrator 與各子系統（如 metrics、logging）。

---

## CONVENTIONS

- **預設安全**：所有實盤相關功能預設為 dry-run，必須顯式啟用 live broker（`-allow-live-broker`、`-allow-real-signor`）。
- **原子寫入**：狀態更新必須使用原子檔案寫入，禁止就地覆寫。
- **Context 統一**：所有長時間運行的操作必須接受 `context.Context`，支援 graceful shutdown。
- **BrokerMode 強制**：測試工具必須強制設定 `cfg.BrokerMode = "paper"`。

---

## ANTI-PATTERNS

| 陷阱 | 說明與預防 |
|------|-----------|
| **意外啟用實盤** | `cmd/atlas` 的 `-allow-live-broker`、`-allow-real-signor` 等旗標，本地測試時切勿意外啟用。 |
| **未檢查 Broker 模式** | 呼叫 `SubmitOrder` 前必須確認 `Broker.Mode()`，避免在 dry-run 預期下送出真實委託。 |
| **StateStore 非原子寫入** | 直接使用 `os.WriteFile` 覆寫狀態檔可能導致損毀，必須使用 `StateStore` 提供的原子寫入方法。 |
| **忽略 CircuitBreaker** | 熔斷觸發後必須停止下單，不可繞過 `CircuitBreaker` 直接呼叫 `Broker`。 |
| **Nonce 重複使用** | `nonce_store.go` 產生的 nonce 必須單次有效，禁止快取或重複使用。 |

| **混合職責直接修改** | 本套件已知混合基礎設施與業務邏輯，新增功能時應註明屬於哪一類，為未來拆分做準備。 |

---

## KEY TYPES

| 結構體/介面 | 檔案 | 用途 |
|------------|------|------|
| `Orchestrator` | orchestrator.go | 實盤交易核心協調器 |
| `Broker` | broker.go | 券商下單介面 |
| `DryRunBroker` | broker.go | 安全預設實作（不送真實委託） |
| `GuardedLiveBroker` | broker.go | 實盤骨架（無 adapter 時拒單） |
| `OrderManager` | order_manager.go | 訂單生命周期管理 |
| `CircuitBreaker` | circuit_breaker.go | 熔斷機制 |
| `StateStore` | store.go | 狀態持久化 |
| `ChannelEventBus` | eventbus.go | 內部事件匯流排 |
| `Scheduler` | scheduler.go | 交易排程 |
| `AgentRunner` | agent_runner.go | Agent 決策驅動 |

---

## 測試與驗證

```bash
# 單元測試
go test ./internal/live/...

# Broker 簽名格式驗證（dummy 模式）
go run ./cmd/experimental/validate-broker
```

---

## Wave 9 — EventPositionUpdate 生產發布

`internal/live/orchestrator.go` 是 `EventPositionUpdate` 的生產呼叫者之一，向下游 `BaselineTrigger` 與 `DriftDetector` 發布部位更新。live orchestrator 目前只發布 `"updated"`；新增/移除部位的發布由模擬引擎與訂單成交路徑負責。詳細發布點、`changeType` 語意與下游訂閱者見 `docs/handoff/2026-wave9-event-position-update.md`。
