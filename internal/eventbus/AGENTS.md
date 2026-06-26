# AGENTS.md — internal/eventbus

**成熟度**: stable
**模組職責**: 頻道式事件匯流排，提供 Publish/Subscribe 與 SSE 橋接，支援 42 種事件類型。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|
| `EventBus` | `eventbus.go` | 介面：`Publish`/`Subscribe`/`SubscribeAll`/`Close` |
| `ChannelEventBus` | `eventbus.go` | 頻道實作，單一 dispatcher goroutine |
| `SSEBridge` | `sse_bridge.go` | SSE 客戶端廣播橋接 |
| `BusEvent` | `types.go` | 標準事件：ID/Type/Timestamp/Payload/Severity |
| `Subscription` | `eventbus.go` | 訂閱控制代碼，含 `Cancel` 函式 |

## 資料流

```
Publisher → ChannelEventBus.Publish()
  → 緩衝頻道（drop-on-full 警告）
  → dispatcher goroutine
    → 依 EventType 分發至各 handler
    → 每 handler 獨立 goroutine（panic recovery + 30s timeout）
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **Publish 為 fire-and-forget** | 寫入失敗僅內部記錄，不回傳 error，呼叫方無法感知丟失 |
| **SubscribeAll 接收全部事件** | 無類型過濾，handler 需自行判斷 `EventType` |
| **Handler 超時 30s 強制終止** | 慢速 handler 會被殺掉，事件可能未完整處理 |
| **SSE 客戶端未關閉導致 goroutine 洩漏** | 斷線時必須呼叫 `Cancel()`，否則發送 goroutine 持續阻塞 |
| **頻道滿時最舊事件被丟棄** | SSEBridge 廣播採 oldest-event-drop，客戶端可能漏接 |
| **EnrichEvent 僅處理 map 型 Payload** | 非 `map[string]any` 的 payload 只會拿到基礎描述 |

## 測試

- `go test ./internal/eventbus/...`
- `eventbus_test.go`：Publish/Subscribe 單元測試
- `sse_bridge_test.go`：SSE 連線生命週期測試
