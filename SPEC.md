# Real-Time Quote Architecture Specification

## 1. Overview

本規格定義 `atlas-go` 即時報價基礎設施的設計目標、介面契約與實作約束。

**目標**：新增基於 WebSocket 的即時報價能力，作為現有 `HybridProvider`（FinMind/Fugle/TWSE）的補充層。

---

## 2. 現有基礎設施分析

### 2.1 現有 Provider 堆疊

| Provider | 類型 | 用途 |
|----------|------|------|
| `FinMindProvider` | HTTP REST (日線) | 每日收盤報價 |
| `FugleProvider` | HTTP REST (日內) | 盤中快照報價 |
| `TWSEClient` | HTTP REST | 最後備援 |
| `PollingAdapter` | 輪詢 | 將任何 `Provider` 包裝為 `StreamingProvider` |

### 2.2 現有 `Quote` 結構 (`internal/domain/types.go`)

```go
type Quote struct {
    Symbol     string
    Last       float64   // 最新成交價
    Open       float64   // 開盤價
    High       float64   // 最高價
    Low        float64   // 最低價
    Volume     int64     // 成交量
    Market     string    // "TW"
    AsOf       time.Time // 報價時間
    IsTradable bool
    Source     string    // "finmind", "fugle", "twse"
}
```

### 2.3 現有 `StreamingProvider` 介面 (`internal/marketdata/streaming.go`)

```go
type QuoteHandler func(quote domain.Quote)

type StreamingProvider interface {
    Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error
    Unsubscribe(ctx context.Context, symbols []string) error
}
```

**問題**：`PollingAdapter` 依賴 `time.Ticker`，在多訂閱者情境下會重複輪詢，效率低落。

---

## 3. 新增元件設計

### 3.1 `RealtimeProvider` 介面

```go
// QuoteCallback 是接收即時報價的回調函式。
type QuoteCallback func(quote domain.Quote)

// RealtimeProvider 為即時報價來源的最高層級介面。
type RealtimeProvider interface {
    // Connect 建立 WebSocket 連線。
    Connect(ctx context.Context) error

    // Disconnect 關閉 WebSocket 連線。
    Disconnect(ctx context.Context) error

    // Subscribe 訂閱指定 symbols 的即時報價。
    Subscribe(symbols []string) error

    // Unsubscribe 取消訂閱指定 symbols。
    Unsubscribe(symbols []string) error

    // OnQuote 設定報價回調。
    OnQuote(callback QuoteCallback)

    // IsConnected 查詢連線狀態。
    IsConnected() bool

    // Name 回傳 provider 名稱。
    Name() string
}
```

### 3.2 `FugleWebSocketProvider` 實作

Fugle 富果行情 WebSocket API：

- **端點**：`wss://api.fugle.tw/marketdata/v1.0/stock/streaming`
- **驗證**：`auth` 事件攜帶 `apikey`
- **訂閱**：`subscribe` 事件指定 `channel` 與 `symbol`
- **頻道**：`trades`（最新成交）、`candles`（分鐘 K）、`books`（五檔）、`aggregates`（聚合）
- **心跳**：Server 每 30 秒發送 heartbeat

#### 3.2.1 連線生命週期

```
Connect()
  ├─ 建立 WebSocket 連線
  ├─ 發送 auth 事件 {event: "auth", data: {apikey: "..."}}
  └─ 等待 authenticated 事件確認

Subscribe(symbols)
  ├─ 發送 subscribe 事件 {event: "subscribe", data: {channel: "trades", symbol: "2330"}}
  └─ 啟動訊息讀取 goroutine

Disconnect()
  ├─ 發送 unsubscribe 事件（可選）
  └─ 關閉 WebSocket 連線
```

#### 3.2.2 重連機制（指數退避）

```go
const (
    initialBackoff = 1 * time.Second
    maxBackoff     = 60 * time.Second
    backoffFactor  = 2.0
)

func (p *FugleWebSocketProvider) reconnect() error {
    p.mu.Lock()
    p.backoff = p.backoff * backoffFactor
    if p.backoff > maxBackoff {
        p.backoff = maxBackoff
    }
    p.mu.Unlock()

    time.Sleep(p.backoff)
    return p.Connect(context.Background())
}
```

觸發條件：
- WebSocket 連線斷開
- 收到錯誤訊息
- 認證失敗

### 3.3 `RealtimeRouter`（多 Provider 路由）

```go
// RealtimeRouter 整合多個即時 Provider，支援備援切換。
type RealtimeRouter struct {
    providers []RealtimeProvider
    primary   int // 目前主動 provider 索引
    mu        sync.RWMutex
}

// Subscribe 將符號訂閱請求路由到目前 primary provider。
func (r *RealtimeRouter) Subscribe(symbols []string) error

// SwitchToNext 若 primary provider 失敗，自動切換到下一個。
func (r *RealtimeRouter) SwitchToNext() error
```

---

## 4. Fugle WebSocket 訊息格式

### 4.1 認證

```json
// 發送
{"event": "auth", "data": {"apikey": "YOUR_API_KEY"}}

// 接收
{"event": "authenticated", "data": {"apikey": "YOUR_API_KEY"}}
```

### 4.2 訂閱 (`trades` 頻道)

```json
// 發送
{"event": "subscribe", "data": {"channel": "trades", "symbol": "2330"}}

// 接收
{
  "event": "data",
  "data": {
    "symbol": "2330",
    "type": "EQUITY",
    "exchange": "TWSE",
    "date": "2026-04-30T14:30:00.000+08:00",
    "price": 1050.0,
    "unit": 1000,
    "volume": 4778,
    "bid": 1045.0,
    "ask": 1050.0
  },
  "channel": "trades"
}
```

### 4.3 轉換為 `domain.Quote`

```go
func parseFugleTrade(data map[string]interface{}) (domain.Quote, error) {
    quote := domain.Quote{
        Symbol: data["symbol"].(string),
        Last:   data["price"].(float64),
        Volume: int64(data["volume"].(float64)),
        Market: "TW",
        AsOf:   parseTime(data["date"].(string)),
        Source: "fugle-ws",
    }
    return quote, nil
}
```

---

## 5. 與現有系統整合

### 5.1 整合 `HybridProvider`

`RealtimeProvider` 為獨立擴展，不修改現有 `HybridProvider` 結構。建議使用方式：

```go
// 場景 1：純輪詢（現有）
hybrid := NewHybridProvider(finmindKey, fugleKey)
polling := &PollingAdapter{Base: hybrid, Interval: 30}
polling.Subscribe(ctx, symbols, handler)

// 場景 2：即時優先（新建）
realtime := NewFugleWebSocketProvider(fugleKey)
realtime.OnQuote(func(q domain.Quote) { ... })
realtime.Connect(ctx)
realtime.Subscribe(symbols)
```

### 5.2 整合 `StreamingProvider`

擴展 `StreamingProvider` 介面以支援 WebSocket：

```go
// ConnectiveStreamingProvider 為支援主動連線的 StreamingProvider。
type ConnectiveStreamingProvider interface {
    StreamingProvider
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    IsConnected() bool
}
```

---

## 6. 錯誤處理

| 錯誤情境 | 處理策略 |
|----------|----------|
| WebSocket 連線失敗 | 指數退避重連（1s → 2s → 4s → ... → 60s 上限） |
| 認證失敗 | 記錄錯誤，不重連（API Key 問題需人工修復） |
| 訂閱失敗 | 記錄錯誤，返回錯誤給呼叫者 |
| 訊息解析失敗 | 記錄錯誤訊息，繼續處理下一條 |
| Provider 斷線 | 觸發 `SwitchToNext()` 切換備援 |

---

## 7. 速率限制

Fugle 富果行情 WebSocket API 目前**無明確速率限制**，但需遵守：

- 每個連線同時訂閱 symbols 有限制（建議 ≤ 50）
- 若有新 symbols 需求，先 `Unsubscribe` 不再需要的，再 `Subscribe` 新的

---

## 8. 設定檔結構

```yaml
# configs/realtime.yaml
realtime:
  enabled: true
  provider: fugle  # 目前僅支援 fugle

  fugle:
    api_key: ${FUGLE_API_KEY}
    endpoint: "wss://api.fugle.tw/marketdata/v1.0/stock/streaming"

  reconnection:
    initial_backoff: "1s"
    max_backoff: "60s"
    backoff_factor: 2.0

  channels:
    - trades      # 即時成交（主要使用）
    # - candles    # 分鐘K（可選）
    # - books       # 五檔（可選）

  symbols:
    max_per_connection: 50
```

---

## 9. 測試策略

### 9.1 單元測試

- `FugleWebSocketProvider` 狀態機測試（Connect → Subscribe → Disconnect）
- 訊息解析測試（Fugle JSON → `domain.Quote`）
- 重連邏輯測試（計數器、倍數、上限）

### 9.2 整合測試

- Mock WebSocket Server（`net/http/httptest` + `golang.org/x/net/websocket`）
- 端對端訂閱流程測試

---

## 10. 預定產出檔案

```
internal/marketdata/
  ├─ realtime/
  │   ├─ provider.go        # RealtimeProvider 介面
  │   ├─ fugle_ws.go        # FugleWebSocketProvider 實作
  │   ├─ router.go          # RealtimeRouter 多路器
  │   ├─ fugle_ws_test.go   # 單元測試
  │   └─ fugle_ws_mock_test.go  # Mock WebSocket 整合測試

configs/
  └─ realtime.yaml          # 設定檔
```

---

## 11. 約束

1. **不修改現有 Provider**：新實作位於 `internal/marketdata/realtime/` 子目錄
2. **不引入全域狀態**：使用依賴注入，由 caller 持有 `RealtimeRouter` 實例
3. **符合 Go 程式碼慣例**：介面小而聚焦，錯誤包裝，`gofmt` 格式化
4. **支援 `context.Context`**：所有連線操作支援取消

---

## 12. Status

**NEEDS_CONTEXT** - 需要以下資訊才能實作：

1. **WebSocket 函式庫選擇**：目前無 WebSocket 依賴，需確認是否使用 `gorilla/websocket` 或標準庫 `net/http` + `golang.org/x/net/websocket`
2. **長期報價擴展**：`candles`（分鐘K）是否需要與 `domain.Quote` 不同的結構？
3. **與 Orchestrator 整合點**：`RealtimeRouter` 的輸出如何傳遞給現有 Orchestrator？

