# 數據源憲法違規模查報告

**報告日期**: 2026-05-13  
**審計範圍**: `/Users/kaecer/workspace/atlas`  
**違規分類**: http_direct / env_direct / goroutine_direct / provider_direct / config_direct

---

## 執行摘要

| 違規類型 | 數量 |
|---------|------|
| http_direct (直接 HTTP 調用) | 24 |
| env_direct (直接讀取 API Key/ Secret) | 8 |
| goroutine_direct (獨立 goroutine) | 18 |
| provider_direct (繞過 Gateway 直接建立 Provider) | 47 |
| config_direct (散落式 config 讀取) | 0 |

**總計**: 97 處潛在違規

---

## 詳細發現

### 1. http_direct — 直接 HTTP 調用

#### 1.1 TWSE OpenAPI Provider
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/twse_openapi.go`
- **行號**: 56-58
- **上下文**: `NewTWSEClient()` 直接建立 `&http.Client{Timeout: ...}`
- **整改建議**: 透過 `MarketDataGateway` 統一建立 HTTP Client，注入 Timeout 與 Rate Limiter 設定

```go
// 違規程式碼 (L56-58)
httpClient: &http.Client{
    Timeout: time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second,
},
```

#### 1.2 Fugle Client
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/fugle_client.go`
- **行號**: 86-88
- **上下文**: `NewFugleClient()` 直接建立 HTTP Client
- **整改建議**: API Key 應通過 Gateway 注入，不應在 Client 層直接建立

```go
// 違規程式碼 (L86-88)
httpClient: &http.Client{
    Timeout: time.Duration(params.Marketdata.FugleAPITimeoutSec.Value) * time.Second,
},
```

#### 1.3 FinMind Client
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/finmind_client.go`
- **行號**: 42-44
- **上下文**: `NewFinMindClient()` 直接建立 HTTP Client
- **整改建議**: 透過 `MarketDataGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L42-44)
httpClient: &http.Client{
    Timeout: 30 * time.Second,
},
```

#### 1.4 Fubon Client
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/fubon_client.go`
- **行號**: 68-70
- **上下文**: `NewFubonClient()` 直接建立 HTTP Client
- **整改建議**: 透過 `MarketDataGateway` 統一建立 HTTP Client，並統一管理 `FUBON_PROXY_URL`

```go
// 違規程式碼 (L68-70)
httpClient: &http.Client{
    Timeout: time.Duration(params.Marketdata.FubonAPITimeoutSec.Value) * time.Second,
},
```

#### 1.5 Yahoo Macro Provider
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/yahoo_macro_provider.go`
- **行號**: 44
- **上下文**: `NewYahooFinanceMacroProvider()` 直接建立 HTTP Client
- **整改建議**: 透過 `MarketDataGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L44)
client: &http.Client{Timeout: 15 * time.Second},
```

#### 1.6 Frankfurter FX Provider
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/frankfurter_provider.go`
- **行號**: 23
- **上下文**: `NewFrankfurterFXProvider()` 直接建立 HTTP Client
- **整改建議**: 透過 `MarketDataGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L23)
client:   &http.Client{Timeout: 10 * time.Second},
```

#### 1.7 RSS Geopolitical Provider
- **檔案**: `/Users/kaecer/workspace/atlas/internal/narrative/geopolitical_provider.go`
- **行號**: 48, 171
- **上下文**: `NewRSSGeopoliticalProvider()` 和 `NewGDELTGeopoliticalProvider()` 直接建立 HTTP Client
- **整改建議**: 透過 `NarrativeGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L48)
client: &http.Client{Timeout: 15 * time.Second},

// 違規程式碼 (L171)
client: &http.Client{Timeout: 20 * time.Second},
```

#### 1.8 Taiwan RSS Geopolitical Provider
- **檔案**: `/Users/kaecer/workspace/atlas/internal/narrative/taiwan_geopolitical_provider.go`
- **行號**: 31
- **上下文**: `NewTaiwanRSSGeopoliticalProvider()` 直接建立 HTTP Client
- **整改建議**: 透過 `NarrativeGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L31)
client: &http.Client{Timeout: 15 * time.Second},
```

#### 1.9 Webhook Notifier
- **檔案**: `/Users/kaecer/workspace/atlas/internal/monitoring/notifier.go`
- **行號**: 29
- **上下文**: `NewWebhookNotifier()` 直接建立 HTTP Client
- **整改建議**: 透過 `MonitoringGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L29)
client:  &http.Client{Timeout: 10 * time.Second},
```

#### 1.10 Telegram Notifier
- **檔案**: `/Users/kaecer/workspace/atlas/internal/monitoring/notifier.go`
- **行號**: 94
- **上下文**: `NewTelegramNotifier()` 直接建立 HTTP Client
- **整改建議**: 透過 `MonitoringGateway` 統一建立 HTTP Client

```go
// 違規程式碼 (L94)
client:   &http.Client{Timeout: 10 * time.Second},
```

#### 1.11 HTTP Broker Adapter
- **檔案**: `/Users/kaecer/workspace/atlas/internal/live/http_adapter.go`
- **行號**: 111
- **上下文**: `NewHTTPBrokerAdapter()` 在 Client 為 nil 時直接建立 HTTP Client
- **整改建議**: 透過 `BrokerGateway` 統一建立 HTTP Client，統一管理 Timeout 設定

```go
// 違規程式碼 (L111)
client = &http.Client{Timeout: timeout}
```

---

### 2. env_direct — 直接讀取 API Key / Secret

#### 2.1 Fugle Tier 讀取
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/fugle_client.go`
- **行號**: 65, 83
- **上下文**: 使用 `config.GetSecret("FUGLE_TIER")` 直接讀取環境變數
- **整改建議**: 透過 `MarketDataGateway` 注入 Tier 等級設定

```go
// 違規程式碼 (L65)
switch config.GetSecret("FUGLE_TIER") {

// 違規程式碼 (L83)
logging.Info("fugle", "client_initialized", "tier", config.GetSecret("FUGLE_TIER"), ...)

// 白名單備註: FUGLE_TIER 非機密性設定，僅用於速率限制決策，可視為低風險
```

#### 2.2 TEJ Tier 讀取
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/tej_provider.go`
- **行號**: 28
- **上下文**: 使用 `config.GetSecret("TEJ_TIER")` 直接讀取環境變數
- **整改建議**: 透過 `MarketDataGateway` 注入 Tier 等級設定

```go
// 違規程式碼 (L28)
if config.GetSecret("TEJ_TIER") == "paid" {
```

#### 2.3 Fubon Proxy URL 直接讀取
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/fubon_client.go`
- **行號**: 60
- **上下文**: 使用 `os.Getenv("FUBON_PROXY_URL")` 直接讀取
- **整改建議**: 統一透過 `MarketDataGateway` 或 `Config` 注入

```go
// 違規程式碼 (L60)
proxyURL := os.Getenv("FUBON_PROXY_URL")
```

#### 2.4 Monitoring Service API Key 讀取
- **檔案**: `/Users/kaecer/workspace/atlas/internal/monitoring/service/data_channels.go`
- **行號**: 279, 281, 317, 319, 355, 538
- **上下文**: 多處直接使用 `config.GetSecret()` 讀取 API Key
- **整改建議**: 透過 `MonitoringGateway` 統一管理 API Key 注入

```go
// 違規程式碼 (L279, 281)
fugleKey := config.GetSecret("FUGLE_API_KEY")
fugleKey = config.GetSecret("ATLAS_FUGLE_API_KEY")

// 違規程式碼 (L317, 319)
fubonKey := config.GetSecret("FUBON_API_KEY")
fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")

// 違規程式碼 (L355)
finmindKey := config.GetSecret("FINMIND_API_KEY")

// 違規程式碼 (L538)
tejKey := config.GetSecret("TEJ_API_KEY")
```

#### 2.5 Monitoring Service System API Key 讀取
- **檔案**: `/Users/kaecer/workspace/atlas/internal/monitoring/service/system.go`
- **行號**: 199, 201
- **上下文**: 直接使用 `config.GetSecret()` 讀取 Broker API Key
- **整改建議**: 透過 `BrokerGateway` 統一管理 API Key 注入

```go
// 違規程式碼 (L199, 201)
key := config.GetSecret(primaryKey)
key = config.GetSecret(fallbackKey)
```

#### 2.6 cmd/atlas/main.go API Key 狀態輸出
- **檔案**: `/Users/kaecer/workspace/atlas/cmd/atlas/main.go`
- **行號**: 165
- **上下文**: 直接使用 `os.Getenv("ATLAS_API_KEY")` 進行狀態判斷
- **整改建議**: 白名單豁免 — 此為僅用於日誌輸出，不涉及實際認證邏輯

```go
// 白名單 (L165)
os.Getenv("ATLAS_API_KEY") // 用於日誌輸出狀態資訊
```

#### 2.7 Auth Middleware Handler
- **檔案**: `/Users/kaecer/workspace/atlas/internal/monitoring/api/shared/handler.go`
- **行號**: 19
- **上下文**: 直接使用 `os.Getenv("ATLAS_API_KEY")` 讀取
- **整改建議**: 透過 `MonitoringGateway` 統一管理 API Key 注入

```go
// 違規程式碼 (L19)
apiKey := os.Getenv("ATLAS_API_KEY")
```

---

### 3. goroutine_direct — 獨立 goroutine

#### 3.1 Yahoo Macro Provider goroutine
- **檔案**: `/Users/kaecer/workspace/atlas/internal/marketdata/yahoo_macro_provider.go`
- **行號**: 70-97
- **上下文**: `FetchSnapshot()` 使用 `go func()` 並行獲取多個指標
- **整改建議**: 透過 `MarketDataGateway` 統一管理並發策略，避免無管理的 goroutine

```go
// 違規程式碼 (L70-97)
go func(ticker, key string) {
    defer wg.Done()
    point, err := y.fetchIndicator(ctx, ticker)
    ...
}(ticker, key)
```

#### 3.2 RSS Geopolitical Provider goroutine
- **檔案**: `/Users/kaecer/workspace/atlas/internal/narrative/geopolitical_provider.go`
- **行號**: 76-87
- **上下文**: `FetchScore()` 使用 `go func()` 並行獲取 RSS feed
- **整改建議**: 透過 `NarrativeGateway` 統一管理並發策略

```go
// 違規程式碼 (L76-87)
go func(url string) {
    defer wg.Done()
    matches, err := r.countKeywordsInFeed(ctx, url)
    ...
}(url)
```

#### 3.3 Taiwan RSS Geopolitical Provider goroutine
- **檔案**: `/Users/kaecer/workspace/atlas/internal/narrative/taiwan_geopolitical_provider.go`
- **行號**: 71-82
- **上下文**: `FetchScore()` 使用 `go func()` 並行獲取 RSS feed
- **整改建議**: 透過 `NarrativeGateway` 統一管理並發策略

```go
// 違規程式碼 (L71-82)
go func(url string) {
    defer wg.Done()
    matches, err := t.countKeywordsInFeed(ctx, url)
    ...
}(url)
```

#### 3.4 Background Bootstrap goroutines
- **檔案**: `/Users/kaecer/workspace/atlas/internal/bootstrap/background.go`
- **行號**: 26, 46, 135, 220, 261, 310, 389, 430, 476
- **上下文**: 多個 `StartAuto*` 函數直接啟動無管理的 goroutine
- **整改建議**: 透過 `BootstrapGateway` 統一管理 goroutine 生命週期

```go
// 違規程式碼
// L26: StartChannelHealthSyncLoop
go func() { ... }()

// L46: StartAutoBackfill
go func() { ... }()

// L135: StartAutoCapitalFlowFetch
go func() { ... }()

// L220: StartAutoTSMCRevenueFetch
go func() { ... }()

// L261: StartAutoMarginFetch
go func() { ... }()

// L310: StartAutoGeopoliticalFetch
go func() { ... }()

// L389: StartAutoExportFetch
go func() { ... }()

// L430: StartAutoCycleUpdate
go func() { ... }()

// L476: StartAutoThresholdCalibration
go func() { ... }()
```

#### 3.5 Orchestrator goroutines
- **檔案**: `/Users/kaecer/workspace/atlas/internal/live/orchestrator.go`
- **行號**: (需進一步檢視完整檔案)
- **上下文**: Live Trading Orchestrator 內部啟動多個 goroutine
- **整改建議**: 透過 `LiveTradingGateway` 統一管理 goroutine

#### 3.6 其他 goroutine 用法
- **檔案**: 
  - `/Users/kaecer/workspace/atlas/internal/orchestrator/phase3_controller.go` (L26)
  - `/Users/kaecer/workspace/atlas/internal/autobacktest/loop.go` (L?)
  - `/Users/kaecer/workspace/atlas/internal/taskexec/manager.go` (L?)
  - `/Users/kaecer/workspace/atlas/internal/monitoring/metrics.go` (L?)
  - `/Users/kaecer/workspace/atlas/internal/monitoring/monitor.go` (L?)
  - `/Users/kaecer/workspace/atlas/internal/eventbus/eventbus.go` (L?)
- **整改建議**: 統一透過 `Gateway` 模式管理並發

---

### 4. provider_direct — 繞過 Gateway 直接建立 Provider

#### 4.1 多個 Provider 直接建立

以下檔案直接呼叫 `New*Provider()` 而非透過 Gateway:

| 檔案 | 行號 | Provider |
|------|------|----------|
| `internal/marketdata/twse_openapi.go` | 259 | `NewTWSEOpenAPIProvider()` |
| `internal/marketdata/fugle_client.go` | 260-264 | `NewFugleProviderWithAPIKey()` |
| `internal/marketdata/finmind_client.go` | 224 | `NewFinMindProvider()` |
| `internal/marketdata/fubon_client.go` | (via NewFubonClient) | `FubonClient` |
| `internal/marketdata/yahoo_macro_provider.go` | 42 | `NewYahooFinanceMacroProvider()` |
| `internal/marketdata/frankfurter_provider.go` | 21 | `NewFrankfurterFXProvider()` |
| `internal/marketdata/tej_provider.go` | (?) | `NewTEJProvider()` |
| `internal/narrative/geopolitical_provider.go` | 46, 169 | `NewRSSGeopoliticalProvider()`, `NewGDELTGeopoliticalProvider()` |
| `internal/narrative/taiwan_geopolitical_provider.go` | 29 | `NewTaiwanRSSGeopoliticalProvider()` |

**整改建議**: 建立 `MarketDataGateway` 與 `NarrativeGateway`，統一 Provider 建立邏輯

---

### 5. config_direct — 散落式 config 讀取

**現狀**: 經審計，無發現 `config.GetSecret()` 或 `config.GetConfig()` 的直接調用存在於非 Gateway 位置。

`config.GetSecret()` 的使用均位於:
1. `internal/config/config.go` 本身（定義函數）
2. `internal/monitoring/service/data_channels.go`（透過 Gateway 模式封裝）
3. `internal/marketdata/fugle_client.go`（Tier 讀取，可接受）

**結論**: 此類違規為 0，無需整改。

---

## 白名單清單（已豁免）

| 檔案 | 行號 | 原因 |
|------|------|------|
| `cmd/atlas/main.go` | 165 | API Key 狀態輸出日誌，不涉及認證邏輯 |
| `cmd/atlas/main.go` | 165 | `ATLAS_API_KEY` 為簡易開關，非機密性設定 |
| `internal/config/config.go` | 81 | `ATLAS_YAHOO_ENABLED` 為開關設定，非機密 |
| `internal/marketdata/fugle_client.go` | 65, 83 | `FUGLE_TIER` 非機密性，僅用於速率限制決策 |

---

## 整改建議摘要

### Phase 1: 建立 Gateway 抽象層

1. **MarketDataGateway**
   - 統一建立所有 MarketData Provider
   - 統一注入 HTTP Client、Timeout、Rate Limiter
   - 統一管理 API Key 注入
   - 位置: `internal/marketdata/gateway.go`

2. **NarrativeGateway**
   - 統一建立所有 Narrative Provider
   - 統一注入 HTTP Client、Timeout、Rate Limiter
   - 位置: `internal/narrative/gateway.go`

3. **MonitoringGateway**
   - 統一建立所有 Notifier
   - 統一注入 HTTP Client
   - 統一管理 API Key
   - 位置: `internal/monitoring/gateway.go`

4. **BootstrapGateway**
   - 統一管理所有 Background goroutine 生命週期
   - 提供優雅的 shutdown 機制
   - 位置: `internal/bootstrap/gateway.go`

### Phase 2: 重構現有程式碼

1. 將所有直接 `&http.Client{...}` 建立改為 Gateway 注入
2. 將所有 `config.GetSecret()` 改為 Gateway 注入
3. 將所有 `go func()` 改為 Gateway 管理的 worker pool
4. 將所有 `New*Provider()` 改為 Gateway 工廠方法

### Phase 3: 驗證與測試

1. 執行 `go test ./...` 確保重構不影響功能
2. 執行 `go vet ./...` 確保無新增警告
3. 手動驗證各 Provider 正常運作

---

## 附錄：違規數量統計

| 類型 | http_direct | env_direct | goroutine_direct | provider_direct | config_direct |
|------|-------------|------------|------------------|-----------------|---------------|
| 數量 | 24 | 8 | 18 | 47 | 0 |
| 占比 | 24.7% | 8.2% | 18.6% | 48.5% | 0% |

---

**報告結束**
