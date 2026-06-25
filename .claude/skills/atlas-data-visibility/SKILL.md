---
name: atlas-data-visibility
description: "Use when fixing zero-value display bugs, silent channel failures, or implementing data visibility safeguards. Triggers: zero-card bug, silent fetch failure, status.data_status, ChannelErrors, FetchResult.Fallback, MacroDataSnapshot all-zero"
---

## 問題背景

Cross-market 頁面曾出現「八張卡片全部顯示 0」的嚴重 bug。根目因並非資料源本身為零，而是某些 channel（如 US indices / tech stocks）在背景 fetch 失敗後，系統靜默回傳了 zero-value 的 `MacroDataSnapshot`。前端因為沒有任何錯誤標記，直接把空字串當作正常資料渲染，導致用戶看到一片 0 值卻無從得知資料其實已經失效。

這個 bug 催生了四層漸進式資料可見性保障機制：從最底層的 gateway fetch 結果標記，一路穿透到前端 UI 的紅色錯誤徽章。每一層都負責把「資料可能有問題」這個訊息向上傳遞，不讓任何一層靜默吞掉錯誤。

---

## 四層機制

### Layer 1: Gateway (`internal/apigateway/`)

Gateway 層在 `FetchResult` struct 中新增了 `Stale`、`Fallback`、`LastError` 三個欄位，用來標記這次 fetch 的品質狀態。

**檔案**: `internal/apigateway/provider.go`

```go
type FetchResult struct {
    Data      []byte
    Stale     bool   // true if returned from cache on circuit-breaker open
    Fallback  bool   // true if using fallback provider
    LastError string // last fetch error message
    Timestamp time.Time
}
```

**檔案**: `internal/apigateway/gateway.go`

當 circuit breaker 開啟、系統改走 stale cache 路徑時，gateway 會主動標記 `Stale = true` 並附上錯誤訊息：

```go
// Stale cache path: circuit breaker is open
return FetchResult{
    Data:      cached.Data,
    Stale:     true,
    LastError: fmt.Sprintf("circuit breaker open for %s: %v", channelID, err),
    Timestamp: time.Now(),
}, nil
```

**關鍵設計**: Gateway 不判斷「這筆資料還能不能用」，它只做一件事：誠實報告「我這次是怎麼拿到這筆資料的」。上層自行決定如何處理 stale/fallback 資料。

---

### Layer 2: Adapter (`internal/monitoring/gateway_adapter.go`)

Adapter 層負責把 gateway 的 per-fetch 錯誤彙整成 per-channel 錯誤地圖，讓 service 層可以一次看到「哪些 channel 掛了」。

**檔案**: `internal/monitoring/gateway_adapter.go`

```go
type macroDataGatewayAdapter struct {
    gateway    apigateway.Gateway
    lastErrors map[string]string // channelID -> error message
    mu         sync.RWMutex
}

func (a *macroDataGatewayAdapter) ChannelErrors() map[string]string {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    errs := make(map[string]string, len(a.lastErrors))
    for k, v := range a.lastErrors {
        errs[k] = v
    }
    return errs
}
```

在 `FetchSnapshot` 中，每個 channel 獨立 fetch，失敗時記錄錯誤但不中斷其他 channel：

```go
func (a *macroDataGatewayAdapter) FetchSnapshot(ctx context.Context) (*MacroDataSnapshot, error) {
    snapshot := &MacroDataSnapshot{}
    
    // Clear previous errors
    a.mu.Lock()
    a.lastErrors = make(map[string]string)
    a.mu.Unlock()
    
    // Fetch each channel independently
    for _, channel := range a.channels {
        result, err := a.gateway.Fetch(ctx, channel)
        if err != nil {
            a.mu.Lock()
            a.lastErrors[channel] = err.Error()
            a.mu.Unlock()
            continue // Don't fail the entire snapshot
        }
        
        if result.Stale {
            a.mu.Lock()
            a.lastErrors[channel] = "stale: " + result.LastError
            a.mu.Unlock()
        }
        
        // Merge data into snapshot...
    }
    
    return snapshot, nil
}
```

**關鍵設計**: Adapter 是「錯誤收集器」，不是「錯誤處理器」。它把分散的 channel 錯誤彙整成一份地圖，但不決定這些錯誤是否嚴重到需要降級整個頁面。

---

### Layer 3: Service (`internal/monitoring/service/crossmarket.go`)

Service 層負責「業務語義判斷」：根據 adapter 提供的錯誤地圖和 snapshot 內容，決定整體資料狀態是 ok / degraded / stale。

**檔案**: `internal/monitoring/service/crossmarket.go`

```go
type CrossMarketStatus struct {
    DataStatus     string   `json:"data_status"`     // "ok" | "degraded" | "stale"
    FailedChannels []string `json:"failed_channels"` // which channels failed
    // ... other fields
}
```

`GetStatus()` 的核心邏輯：當發現所有 8 個 US index/tech 欄位都是 zero-symbol 時，標記為 degraded：

```go
func (s *CrossMarketService) GetStatus(ctx context.Context) (*CrossMarketStatus, error) {
    snapshot, err := s.adapter.FetchSnapshot(ctx)
    if err != nil {
        return nil, fmt.Errorf("fetch snapshot: %w", err)
    }
    
    status := &CrossMarketStatus{
        DataStatus: "ok",
    }
    
    // Check for all-zero condition (the bug trigger)
    if s.isAllZero(snapshot) {
        status.DataStatus = "degraded"
        status.FailedChannels = s.extractFailedChannels(snapshot)
    }
    
    // Also incorporate adapter-level channel errors
    for channel, errMsg := range s.adapter.ChannelErrors() {
        if errMsg != "" {
            status.FailedChannels = append(status.FailedChannels, channel)
        }
    }
    
    return status, nil
}

func (s *CrossMarketService) isAllZero(snapshot *MacroDataSnapshot) bool {
    // Check all 8 US index/tech fields
    fields := []string{
        snapshot.Nasdaq.Symbol,
        snapshot.SP500.Symbol,
        snapshot.Dow.Symbol,
        snapshot.VIX.Symbol,
        snapshot.NVDA.Symbol,
        snapshot.TSM.Symbol,
        snapshot.AAPL.Symbol,
        snapshot.MSFT.Symbol,
    }
    
    for _, sym := range fields {
        if sym != "" {
            return false
        }
    }
    return true
}
```

Cache 層也會 co-cache 這個 degraded 狀態：

```go
func (s *CrossMarketService) getCachedSnapshot(ctx context.Context) (*CachedSnapshot, error) {
    // Cache key includes status so degraded state is cached too
    cached := s.cache.Get("crossmarket_snapshot")
    if cached != nil {
        return cached.(*CachedSnapshot), nil
    }
    
    status, _ := s.GetStatus(ctx)
    snapshot := &CachedSnapshot{
        Status:    status,
        Timestamp: time.Now(),
    }
    
    s.cache.Set("crossmarket_snapshot", snapshot, cache.DefaultExpiration)
    return snapshot, nil
}
```

**關鍵設計**: Service 層是「業務判官」。它根據領域知識（8 個 US 欄位全零 = 資料異常）決定整體狀態，並把這個判斷結果寫入 API response，讓前端不需要自己猜。

### Wave 9 擴展：ChannelHealthSynthesizer 與 IngestionLagMonitor

同樣的 L2 `ChannelErrors()` API 在 Wave 9 被 `ChannelHealthSynthesizer` 消費，轉為事件驅動的個別通道健康通知：

| 元件 | 檔案 | 輸入 | 輸出 |
|------|------|------|------|
| `ChannelHealthSynthesizer` | `internal/monitoring/service/channel_health_synthesizer.go` | `ChannelHealthProvider.ChannelErrors()` | `EventChannelIndividualHealth` |
| `IngestionLagMonitor` | `internal/monitoring/service/ingestion_lag_monitor.go` | `IngestionLagProvider.P99LatencySeconds()`（生產實作為 `ChannelHealthIngestionLagProvider`，底層讀取 `UnifiedHealthStore.ChannelLatencyMs`） | `EventIngestionLagSpike` |

兩者都遵循「底層誠實報告、上層業務判斷、事件驅動暴露」的資料可見性模式，與 CrossMarketService 共享同一套 L1/L2 基礎設施。詳見 `internal/monitoring/wave9_runtime.go`。

---

### Layer 4: Frontend (`web/static/js/pages/crossmarket.js`)

Frontend 層負責「用戶可見的錯誤表達」。當收到 `data_status === "degraded"` 或個別欄位 `failed: true` 時，必須讓用戶一眼就知道資料有問題，而不是默默顯示 0。

**檔案**: `web/static/js/pages/crossmarket.js`

`getField()` 在欄位缺少 symbol 時回傳 `failed: true`：

```javascript
function getField(snapshot, fieldName) {
    const field = snapshot[fieldName];
    if (!field || field.symbol === "" || field.symbol === undefined) {
        return {
            value: "—",
            change: "—",
            failed: true,
            errorMessage: field?._error || "資料獲取失敗"
        };
    }
    return {
        value: field.value,
        change: field.change_pct,
        failed: false
    };
}
```

`kpiCard()` 在 `failed=true` 時顯示紅色錯誤徽章：

```javascript
function kpiCard(title, field, options = {}) {
    const card = document.createElement('div');
    card.className = 'kpi-card';
    
    if (field.failed) {
        card.classList.add('kpi-card--error');
        card.innerHTML = `
            <div class="kpi-card__header">
                <span class="kpi-card__title">${title}</span>
                <span class="badge badge--error">資料獲取失敗</span>
            </div>
            <div class="kpi-card__body">
                <span class="kpi-card__value kpi-card__value--error">—</span>
                ${options.showSparkline ? '<div class="sparkline sparkline--error"></div>' : ''}
            </div>
        `;
    } else {
        // Normal rendering...
    }
    
    return card;
}
```

頁面頂部的 degraded banner：

```javascript
function renderDegradedBanner(status) {
    if (status.data_status === "degraded") {
        const banner = document.createElement('div');
        banner.className = 'error-banner error-banner--warning';
        banner.innerHTML = `
            <span class="error-banner__icon">⚠</span>
            <span class="error-banner__text">
                部分資料無法即時更新（${status.failed_channels.join(', ')}）
                顯示數值可能為過期資料或預設值
            </span>
        `;
        document.querySelector('.page-header').prepend(banner);
    }
}
```

**關鍵設計**: Frontend 不判斷「資料有沒有問題」，它只判斷「後端有沒有告訴我這筆資料有問題」。所有的錯誤 UI 都來自後端的明確標記，不是前端自己對數值做 heuristic。

---

## 實作檢查清單

### Layer 1: Gateway

- [ ] `FetchResult` struct 有 `Stale`、`Fallback`、`LastError` 欄位
- [ ] Stale cache path（circuit breaker open）會設定 `Stale = true`
- [ ] Fallback provider path會設定 `Fallback = true`
- [ ] 單元測試驗證：circuit breaker open 時回傳 `Stale=true`

### Layer 2: Adapter

- [ ] Adapter 有 `lastErrors map[string]string` 欄位
- [ ] 暴露 `ChannelErrors() map[string]string` 方法
- [ ] `FetchSnapshot` 對每個 channel 獨立 fetch，失敗時記錄但不中斷
- [ ] 單元測試驗證：部分 channel 失敗時，其他 channel 仍正常回傳

### Layer 3: Service

- [ ] `CrossMarketStatus` 有 `DataStatus string` 和 `FailedChannels []string`
- [ ] `GetStatus()` 能檢測 all-zero 條件並標記 `data_status="degraded"`
- [ ] Cache co-caches 降級狀態（不要讓 cache 蓋掉 degraded 標記）
- [ ] 單元測試驗證：all-zero snapshot → `data_status="degraded"`

### Layer 4: Frontend

- [ ] `getField()` 在 `symbol === ""` 時回傳 `failed: true`
- [ ] `kpiCard()` 在 `failed=true` 時顯示紅色錯誤徽章（不是 0）
- [ ] 頁面頂部有 degraded banner 當 `data_status === "degraded"`
- [ ] 整合測試驗證：degraded API response → 紅色徽章 + banner 顯示

---

## 測試場景

### 場景一：單一 channel 失敗

**條件**: US indices channel 回傳 error，但 tech stocks channel 正常
**預期結果**:
- Layer 1: `FetchResult.LastError` 有錯誤訊息
- Layer 2: `ChannelErrors()["us-indices"]` 有錯誤訊息
- Layer 3: `DataStatus="degraded"`，`FailedChannels=["us-indices"]`
- Layer 4: US indices 卡片顯示紅色「資料獲取失敗」，tech stocks 卡片正常顯示
- PASS: 部分資料正常、部分顯示錯誤，不是全部變 0

### 場景二：全部 channel 失敗（all-zero bug）

**條件**: 所有 8 個 US index/tech channel 都回傳 error 或空 symbol
**預期結果**:
- Layer 1: 每個 `FetchResult` 都有 `LastError`
- Layer 2: `ChannelErrors()` 有 8 個 channel 的錯誤
- Layer 3: `isAllZero()` 回傳 true，`DataStatus="degraded"`
- Layer 4: 所有卡片顯示紅色「資料獲取失敗」，頁面頂部有 degraded banner
- PASS: 不是顯示 8 個 0，而是清楚告知用戶「資料獲取失敗」

### 場景三：Stale cache 路徑

**條件**: Circuit breaker 開啟，gateway 回傳 stale cache 資料
**預期結果**:
- Layer 1: `FetchResult.Stale=true`，`LastError="circuit breaker open..."`
- Layer 2: `ChannelErrors()[channel]` = "stale: circuit breaker open..."
- Layer 3: 若 stale 導致 all-zero，`DataStatus="degraded"`
- Layer 4: 卡片顯示「資料獲取失敗」或「資料可能過期」（依設計而定）
- PASS: 用戶知道看到的是過期資料，不是即時資料

### 場景四：快速恢復

**條件**: Channel 曾經失敗（被標記為 degraded），但在下次 fetch 時恢復正常
**預期結果**:
- Layer 1: `FetchResult.Stale=false`，`LastError=""`
- Layer 2: `ChannelErrors()[channel]` = ""（被清除）
- Layer 3: `DataStatus` 從 "degraded" 變回 "ok"
- Layer 4: 紅色徽章消失，卡片顯示正常數值
- PASS: 錯誤狀態能自動恢復，不需要手動清除

---

## 反模式

### 反模式一：靜默吞掉 channel 錯誤

```go
// BAD: 看到 err 就直接 continue，不記錄、不上報
for _, channel := range channels {
    result, err := gateway.Fetch(ctx, channel)
    if err != nil {
        continue // Silent failure — no one knows this channel died
    }
    // ...
}
```

**後果**: 前端顯示 0 或舊資料，用戶和開發者都不知道某些 channel 已經壞了。

### 反模式二：在 gateway 層決定「資料還能不能用」

```go
// BAD: Gateway 自己決定 stale 資料要不要回傳 error
if result.Stale {
    return nil, fmt.Errorf("data is stale, refusing to return")
}
```

**後果**: Gateway 應該保持「誠實報告」的角色，不應該替上層做決策。上層（service）才知道「stale 到什麼程度還能接受」。

### 反模式三：前端自己做 heuristic 判斷資料是否異常

```javascript
// BAD: 前端看到 value === 0 就猜「這可能是錯誤」
if (field.value === 0) {
    showErrorBadge(); // 但 0 可能是正常值！
}
```

**後果**: 假陽性爆炸。 legitimate 的 0 值（如 VIX=0 某天的確發生過）會被誤標為錯誤。前端應該完全信任後端的 `failed` / `data_status` 標記。

### 反模式四：Service 層不檢查 all-zero 就直接標記 ok

```go
// BAD: 不檢查 snapshot 內容，只看有沒有 error
func GetStatus() {
    if err != nil {
        return "degraded"
    }
    return "ok" // 但 snapshot 可能全零！
}
```

**後果**: Adapter 沒有回傳 error（因為每個 channel 都「成功」fetch 到空資料），但 snapshot 全零。Service 必須檢查業務語義層的異常，不能只看 error 有沒有 nil。

### 反模式五：Cache 蓋掉 degraded 狀態

```go
// BAD: Cache 只存 snapshot，不存 status
cache.Set("snapshot", snapshot.Data) // status 被丟掉了
```

**後果**: 第一次 fetch 標記為 degraded 後，後續從 cache 讀到的資料永遠是 ok（因為 status 沒被 cache）。Cache 必須 co-cache degraded 狀態。

---

## 相關檔案

| 檔案 | 層級 | 用途 |
|------|------|------|
| `internal/apigateway/provider.go` | Layer 1 | `FetchResult` struct 定義 |
| `internal/apigateway/gateway.go` | Layer 1 | Stale cache path 標記邏輯 |
| `internal/apigateway/health.go` | Layer 1/2 | `UnifiedHealthStore` 與 `ChannelLatencyMs`（Wave 9） |
| `internal/monitoring/gateway_adapter.go` | Layer 2 | `ChannelErrors()` 與 per-channel 錯誤收集 |
| `internal/monitoring/service/crossmarket.go` | Layer 3 | `CrossMarketStatus` 與 `isAllZero()` 檢測 |
| `internal/monitoring/service/channel_health_synthesizer.go` | Wave 9 | 將 `ChannelErrors()` 轉為 `EventChannelIndividualHealth` |
| `internal/monitoring/service/ingestion_lag_provider.go` | Wave 9 | `ChannelHealthIngestionLagProvider` 生產實作 |
| `internal/monitoring/wave9_runtime.go` | Wave 9 | 5 個偵測器協調器 |
| `web/static/js/pages/crossmarket.js` | Layer 4 | `getField()`、`kpiCard()`、degraded banner |
| `web/static/css/components/error-banner.css` | Layer 4 | 錯誤 banner 樣式 |
| `web/static/css/components/badge.css` | Layer 4 | 紅色錯誤徽章樣式 |

---

*技能版本: 1.1*
*最後更新: 2026-06-25*
*適用對象: Atlas-Go AI Agent*
