# AGENTS.md — internal/apigateway

**成熟度**: stable  
**職責**: 統一資料閘道，管理 14+ 資料通道的快取、限速、熔斷與背景任務排程。

## 核心型別

| 型別 | 功能 |
|------|------|
| `Gateway` | 統一入口 `Fetch(channelID)`，含快取/限速/熔斷 |
| `BackgroundTaskManager` | 定時任務排程（jitter、overlap 保護） |
| `RateLimitManager` | 各通道限速器 |
| `CircuitBreakerManager` | 各通道熔斷器 |
| `FetchResult` | Data + Meta + Stale/Fallback/LastError |

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| 禁止裸 `http.Client` | 所有外部資料抓取必須經 `gateway.Fetch(channelID)` |
| `FetchResult.Fallback` | 熔斷開啟且命中 stale cache 時為 `true`，視為 last-known-good |
| 新通道註冊兩處 | `limits.go` 加限速 + `gateway.go` 的 `channelIDs()` 加列舉 |
| 禁止裸 goroutine 定時任務 | 所有排程必須用 `BackgroundTaskManager` |
| `frankfurter_fx` 獨立限速 | 不再與 `us_yahoo` 共用 |
| `YahooEnabled` 預設 true | 明確 opt-out 設 `ATLAS_YAHOO_ENABLED=false` |
| 完整憲法 | 見 `internal/apigateway/CONSTITUTION.md` |

## 測試

- `background_test.go`：排程與 overlap
- `limits_test.go`：限速器
- `circuitbreaker_test.go`：熔斷狀態轉換
- `gateway_test.go`：Fetch 全鏈路
