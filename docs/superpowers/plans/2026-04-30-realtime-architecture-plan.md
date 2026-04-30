# Real-Time Quote Architecture Implementation Plan

## 基本資訊

| 項目 | 內容 |
|------|------|
| 計畫名稱 | Real-Time Quote Architecture |
| 目標 | WebSocket-based 即時報價基礎設施 |
| 日期 | 2026-04-30 |
| 狀態 | PLANNING |
| 負責範圍 | `internal/marketdata/realtime/` |

---

## 1. 現況分析

### 1.1 現有 Provider 堆疊

```
FinMindProvider ── HTTP REST (日線 close)
FugleProvider ──── HTTP REST (日內 snapshot)
TWSEClient ─────── HTTP REST (最後備援)
     │
     ▼
PollingAdapter ─── 輪詢包裝（效率低落）
```

### 1.2 現有 WebSocket 資產

| 檔案 | 內容 |
|------|------|
| `internal/marketdata/streaming.go` | `StreamingProvider` 介面 + `PollingAdapter` |
| `docs/archive/phase4-architecture.md` | Phase 4 規劃（含 WebSocket 設計，但未實作） |
| `internal/domain/types.go` | `Quote` 結構定義 |

**無 WebSocket 實作**：目前無 `gorilla/websocket`、`nhooyr/websocket` 或 `gobwas/ws` 依賴。

### 1.3 Fugle 富果行情 WebSocket API

- **端點**：`wss://api.fugle.tw/marketdata/v1.0/stock/streaming`
- **頻道**：`trades`（主要）、`candles`、`books`、`aggregates`、`indices`
- **認證**：API Key via `auth` 事件
- **心跳**：Server 每 30 秒傳送
- **速率限制**：無明確限制，建議 ≤50 symbols/連線

---

## 2. 實作範圍

### 2.1 Phase A：核心基礎設施

#### Step A1：新增目錄與介面

**產出**：`internal/marketdata/realtime/provider.go`

```go
package realtime

// QuoteCallback 是接收即時報價的回調函式。
type QuoteCallback func(quote domain.Quote)

// RealtimeProvider 為即時報價來源的最高層級介面。
type RealtimeProvider interface {
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    Subscribe(symbols []string) error
    Unsubscribe(symbols []string) error
    OnQuote(callback QuoteCallback)
    IsConnected() bool
    Name() string
}
```

**驗證**：
```bash
go build ./internal/marketdata/realtime/
```

#### Step A2：FugleWebSocketProvider 實作

**產出**：`internal/marketdata/realtime/fugle_ws.go`

功能：
- WebSocket 連線管理（使用標準庫 `net/http` + `golang.org/x/net/websocket`）
- 認證流程（`auth` → `authenticated`）
- 訂閱管理（`subscribe` / `unsubscribe` 事件）
- 訊息解析（`trades` 頻道 → `domain.Quote`）
- 重連機制（指數退避：1s → 2s → 4s → ... → 60s 上限）
- 執行緒安全（`sync.RWMutex`）

**驗證**：
```bash
go build ./internal/marketdata/realtime/
go test ./internal/marketdata/realtime/... -v
```

#### Step A3：單元測試

**產出**：`internal/marketdata/realtime/fugle_ws_test.go`

測試案例：
- `TestConnect_AuthSuccess`：認證成功
- `TestConnect_AuthFailure`：認證失敗
- `TestSubscribe_Success`：訂閱成功
- `TestUnsubscribe_Success`：取消訂閱
- `TestReconnect_ExponentialBackoff`：指數退避邏輯
- `TestParseTradeMessage`：Fugle JSON → `domain.Quote`
- `TestDisconnect_Close`：斷線清理

**驗證**：
```bash
go test ./internal/marketdata/realtime/... -v -race
```

### 2.2 Phase B：整合與路由

#### Step B1：Mock WebSocket Server

**產出**：`internal/marketdata/realtime/fugle_ws_mock_test.go`

使用 `httptest.NewServer` + `websocket.NewServer` 建立測試用 Fugle API Mock。

**驗證**：
```bash
go test ./internal/marketdata/realtime/... -v -tags=integration
```

#### Step B2：RealtimeRouter 多路器

**產出**：`internal/marketdata/realtime/router.go`

功能：
- 維護多個 `RealtimeProvider`
- 自動備援切換（`SwitchToNext()`）
- 訂閱狀態同步

**驗證**：
```bash
go build ./internal/marketdata/realtime/
go test ./internal/marketdata/realtime/... -v
```

### 2.3 Phase C：設定與部署

#### Step C1：設定檔

**產出**：`configs/realtime.yaml`

```yaml
realtime:
  enabled: true
  provider: fugle

  fugle:
    api_key: ${FUGLE_API_KEY}
    endpoint: "wss://api.fugle.tw/marketdata/v1.0/stock/streaming"

  reconnection:
    initial_backoff: "1s"
    max_backoff: "60s"
    backoff_factor: 2.0

  channels:
    - trades

  symbols:
    max_per_connection: 50
```

#### Step C2：環境變數

更新 `.env.example`：

```bash
FUGLE_API_KEY=your_api_key_here
ATLAS_REALTIME_ENABLED=true
```

---

## 3. 技術決策

### 3.1 WebSocket 函式庫選擇

| 選項 | 優點 | 缺點 |
|------|------|------|
| `gorilla/websocket` | 廣泛使用、功能完整 | 額外依賴 |
| `nhooyr/websocket` | 輕量、標準庫相容 | 社群較小 |
| `gobwas/ws` | 高效能、零分配 | 學習曲線 |
| 標準庫 + `golang.org/x/net/websocket` | 無新依賴 | API 較舊 |

**建議**：使用 `gorilla/websocket`（社群廣泛使用，文檔完善）。

### 3.2 訊息處理模型

```go
// Goroutine 模型
func (p *FugleWebSocketProvider) run(ctx context.Context) {
    for {
        _, msg, err := p.conn.Read(ctx)
        if err != nil {
            p.handleDisconnect(ctx)
            return
        }
        p.dispatch(msg)
    }
}

func (p *FugleWebSocketProvider) dispatch(msg []byte) {
    var wrapper FugleMessage
    if err := json.Unmarshal(msg, &wrapper); err != nil {
        return
    }
    switch wrapper.Event {
    case "data":
        p.handleData(wrapper)
    case "heartbeat":
        // 忽略
    }
}
```

---

## 4. 依賴變更

```bash
# 新增
go get github.com/gorilla/websocket
```

或使用現有 `golang.org/x/net/websocket`（若已間接引入）。

---

## 5. 風險與緩解

| 風險 | 嚴重度 | 緩解策略 |
|------|--------|----------|
| Fugle API 變更 | 中 | Version lock v1.0；錯誤時自動降級到 REST polling |
| WebSocket 連線不稳定 | 高 | 指數退避重連；備援 REST provider |
| API Key 洩露 | 高 | 僅透過環境變數注入；不寫入程式碼 |
| 過多 symbols 訂閱 | 中 | `max_per_connection: 50` 限制；分批訂閱 |

---

## 6. 預定產出結構

```
internal/marketdata/realtime/
├── provider.go           # RealtimeProvider 介面
├── fugle_ws.go          # FugleWebSocketProvider 實作
├── router.go            # RealtimeRouter 多路器
├── fugle_ws_test.go     # 單元測試
└── fugle_ws_mock_test.go # Mock WebSocket 整合測試

configs/
└── realtime.yaml         # 設定檔

.env.example               # 環境變數範例
```

---

## 7. 驗證檢查點

| 階段 | 檢查點 |
|------|--------|
| Phase A | `go build ./internal/marketdata/realtime/` 成功 |
| Phase A | `go test ./internal/marketdata/realtime/...` 全部通過 |
| Phase B | Mock WebSocket 整合測試通過 |
| Phase C | `go build ./...` 成功，無新增 linter 警告 |

---

## 8. 後續步驟

1. **確認 WebSocket 函式庫**（`gorilla/websocket` vs 標準庫）
2. **與 Orchestrator 整合**（決定 `RealtimeRouter` 如何接入現有系統）
3. **擴展 `candles` 頻道支援**（如需要分鐘 K 資料）
4. **績效監控**（連線穩定性、訊息延遲度量）

---

## 9. Open Questions

| 問題 | 選項 | 建議 |
|------|------|------|
| WebSocket 函式庫？ | `gorilla/websocket` / 標準庫 | `gorilla/websocket` |
| 與 Orchestrator 整合點？ | 直接呼叫 / 事件匯流排 | 待確認 |
| 是否需要 `candles` 支援？ | 是 / 否 | 否（Phase 1） |

---

**Author**: Claude Code
**Date**: 2026-04-30
**Status**: PLANNING - Awaiting WebSocket library decision
