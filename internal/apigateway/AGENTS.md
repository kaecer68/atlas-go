# AGENTS.md — internal/apigateway

**成熟度**: stable
**模組職責**: 統一資料閘道，管理 14+ 資料通道的快取、限速、熔斷與背景任務排程。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|
| `Gateway` | `gateway.go` | 統一入口：`Fetch(channelID)`，含快取/限速/熔斷 |
| `BackgroundTaskManager` | `background.go` | 定時任務排程（jitter、overlap 保護、熔斷整合） |
| `ChannelRegistry` | `gateway.go` | 通道註冊與查詢 |
| `RateLimitManager` | `limits.go` | 各通道限速器管理 |
| `CircuitBreakerManager` | `circuitbreaker.go` | 各通道熔斷器管理 |
| `FetchResult` | `gateway.go` | 結果包裝：Data + Meta（latency/cached/stale） |

## 資料流

```
Caller → gateway.Fetch(channelID)
  → CircuitBreaker 檢查
  → Cache 檢查（命中即回傳）
  → RateLimit 等待
  → Provider.Fetch()
  → Cache 更新 + Health 記錄
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **禁止裸 http.Client** | 所有外部資料抓取必須經 `gateway.Fetch(channelID)`，違者擋 PR |
| **新通道必須註冊兩處** | `limits.go` 加限速 + `gateway.go` 的 `channelIDs()` 加列舉 |
| **禁止裸 goroutine 定時任務** | 所有排程必須用 `BackgroundTaskManager`，參見 `CONSTITUTION.md` 第四條 |
| **jpy_yahoo 與 us_yahoo 共用限速器** | 同一 Yahoo Finance endpoint，流量會互相影響 |
| **janus_regime / sector_data 無限速** | `rate.Inf` 標記，不會觸發等待 |
| **overlap 保護跳過執行** | 前次任務未結束時，新週期直接 skip，需檢查 log 確認 |
| **CONSTITUTION.md 六條憲法** | CI 強制檢查，違反直接拒絕合併 |

## 測試

- `go test ./internal/apigateway/...`
- `background_test.go`：排程與 overlap 測試
- `limits_test.go`：限速器行為測試
- `circuitbreaker_test.go`：熔斷狀態轉換測試
- `gateway_test.go`：Fetch 全鏈路測試
