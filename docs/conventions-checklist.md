# Atlas 開發者規範檢查清單（Conventions Checklist）

> **版本**：v1.0
> **生效日期**：2026-06-03
> **適用範圍**：所有新增或修改 API Client、Provider、ParameterMetadata、Gateway Path 的程式碼
> **執行機制**：PR 人工審查（CI 檢查由 `scripts/ci/check_constitution.sh` 自動執行）
> **語言**：繁體中文

---

## 目錄

1. [新增 API Client / Provider](#1-新增-api-client--provider)
2. [新增 ParameterMetadata 欄位](#2-新增-parametermetadata-欄位)
3. [新增 Provider 接線（Gateway Path）](#3-新增-provider-接線gateway-path)
4. [檔案寫入規範](#4-檔案寫入規範)
5. [背景任務註冊](#5-背景任務註冊)
6. [共享客戶端建立（修正現有問題）](#6-共享客戶端建立修正現有問題)

---

## 1. 新增 API Client / Provider

本節適用於新增外部 API 呼叫端（如 FinMind、Fugle、Yahoo Finance 等），參考已存在的實作範例。

### 參考範例

| 模式 | 檔案 |
|------|------|
| 共享 Singleton（`sync.Once`） | `internal/marketdata/finmind_client.go` |
| 共享 Session（內部封裝） | `internal/marketdata/yahoo_session.go` |
| 共享 Limiter（package-level） | `internal/marketdata/bdi_provider.go` |

### 步驟

#### 步驟 1：檢查是否已存在共享 Singleton

在程式碼中搜尋 `GetShared` 或 `GetShared` + Provider 名稱，確認沒有重複的共享客戶端。

```bash
grep -r "GetShared" --include="*.go" internal/marketdata/
```

若已存在，直接使用該 Singleton，不要建立第二個。

#### 步驟 2：若不存在，建立 `sync.Once` Singleton

在 Provider 檔案的 package-level 宣告三個變數：

```go
var (
    sharedMyProvider     *MyProvider
    sharedMyProviderOnce sync.Once
    sharedMyProviderMu   sync.RWMutex
)
```

#### 步驟 3：加入 `GetSharedXxxClient()` 函式

參考 `internal/marketdata/finmind_client.go` 第 52-61 行：

```go
// GetSharedFinMindClient returns a singleton FinMindClient that all components
// share. Using a single client ensures one token bucket enforces the rate
// limit across all call sites.
func GetSharedFinMindClient(apiKey string) *FinMindClient {
    sharedFinMindClientOnce.Do(func() {
        sharedFinMindClient = &FinMindClient{
            apiKey:      apiKey,
            httpClient:  httpclient.NewFactory().NewClient(30 * time.Second),
            rateLimiter: rate.NewLimiter(rate.Every(time.Minute/finmindRateLimit), finmindBurst),
        }
    })
    return sharedFinMindClient
}
```

#### 步驟 4：註冊 Rate Limiter

在 `internal/apigateway/limits.go` 中：

- 新增 Rate/Burst 常數（第 12-52 行區域）
- 在 `NewRateLimitManager()` 的 `limiters` map 中新增 channel 條目（第 67-92 行區域）

```go
// 常數區段
MyProviderRate  = rate.Every(1 * time.Second)
MyProviderBurst = 1

// limiters map 區段
"my_provider": rate.NewLimiter(MyProviderRate, MyProviderBurst),
```

#### 步驟 5：註冊到 Gateway `channelIDs()`

在 `internal/apigateway/gateway.go` 的 `channelIDs()` 函式中加入新的 channel ID（目前列於第 143-168 行）。

```go
func channelIDs() []string {
    return []string{
        // ... 其他 channels
        "my_provider",   // ← 新增這行
    }
}
```

#### 步驟 6：加入 Channel Adapter

在 `internal/apigateway/register_adapters.go` 的 `RegisterChannelAdapters()` 函式中，仿照現有 pattern 新增初始化與註冊區塊（該函式已有 20+ adapter 的範例可參考）。

```go
// --- My Provider ---
myClient := marketdata.GetSharedMyClient(cfg.MyAPIKey)
myAdapter := NewMyChannelAdapter(myClient)
g.registry.Register("my_provider", myAdapter)
logging.Info("apigateway", "adapter_registered", "channel", "my_provider")
```

#### 步驟 7：更新 constitution.md 附錄 A

在 `internal/apigateway/CONSTITUTION.md` 的附錄 A（15+ 個通道規範表，第 479-498 行）中新增一列：

```
| my_provider | 1/s | Liveness | 否 | ✅ |
```

欄位說明：
- **通道 ID**：與 `channelIDs()` 一致
- **限流策略**：描述實際的 rate limit 設定
- **健康檢查模式**：`Liveness`（實際 API 請求）、`Readiness`（本地檔案檢查）、或 `Computed`（內部計算狀態）
- **背景任務**：若有註冊排程任務，寫 `✅ (interval)`；否則寫 `否`
- **熔斷啟用**：`✅` 或 `❌`

#### 步驟 8：更新 2026-06-26-gateway-migration-tracking.md

在 `gateway migration 追蹤（已合併進 AGENTS.md）` 中記錄新增的 channel 及其狀態。

---

## 2. 新增 ParameterMetadata 欄位

本節適用於在 `ParametersConfig` 中新增一個可調參數。

### 參考範例

| 檔案 | 角色 |
|------|------|
| `internal/config/parameters.go` | 結構定義、`Validate()`、`SaveWithRollback()`、合併邏輯 |
| `internal/config/parameters_defaults.go` | `DefaultParametersConfig()` 所有預設值 |
| `configs/parameters.json` | 執行期設定檔（可由管理員手動或實驗修改） |
| `scripts/add_citations.py` | 自動補齊 citation 後設資料 |

### 步驟

#### 步驟 1：在結構體中新增欄位

在 `internal/config/parameters.go` 對應的結構體中新增欄位。欄位必須使用 `ParameterMetadata[T]` 泛型包裝，並標註 `json` tag。

例如，在 `MarketdataParameters` 結構體（第 385-400 行）中新增：

```go
type MarketdataParameters struct {
    // ... 既有欄位
    MyNewTimeoutSec ParameterMetadata[int] `json:"my_new_timeout_sec"`
}
```

#### 步驟 2：在 `DefaultParametersConfig()` 中加入預設值

在 `internal/config/parameters_defaults.go` 對應的 `defaultXxxParameters()` 函式中加入 `ParameterMetadata` 包裝的預設值。

```go
func defaultMarketdataParameters() MarketdataParameters {
    return MarketdataParameters{
        // ... 既有欄位
        MyNewTimeoutSec: ParameterMetadata[int]{
            Value:     10,                          // 預設值
            Rationale: "Default timeout for X API", // 理由說明
            Source:    SourceHeuristic,             // 建議使用常數列舉
            Todo:      "Calibrate from backtest",   // 待辦事項（可省略）
        },
    }
}
```

可用的 `Source` 常數（定義於 `parameters.go` 第 14-21 行）：

| 常數 | 含義 |
|------|------|
| `SourceLiterature` | 學術文獻/實務文獻 |
| `SourceEmpirical` | 歷史資料分析 |
| `SourceHeuristic` | 領域專家判斷 |
| `SourceInferred` | 自動推論/校準 |
| `SourceCalibrated` | 回測最佳化 |
| `SourceExperimental` | ML 實驗/尚未驗證 |

#### 步驟 3：在合併函式中加入邏輯

在對應的 `mergeXxxDefaults()` 函式中（如 `mergeIndustryDefaults()` 位於 `parameters.go` 第 2608-2644 行），加入零值檢查與預設值回填。

```go
func mergeMarketdataDefaults(cfg *ParametersConfig) {
    def := DefaultParametersConfig().Marketdata
    m := &cfg.Marketdata

    if m.MyNewTimeoutSec.Value == 0 {
        m.MyNewTimeoutSec = def.MyNewTimeoutSec
    }
}
```

#### 步驟 4：如需要，加入驗證邏輯

在 `internal/config/parameters.go` 的 `Validate()` 方法（第 1367 行）中加入對新欄位的驗證。

```go
func (p *ParametersConfig) Validate() error {
    // ... 既有驗證

    if p.Marketdata.MyNewTimeoutSec.Value < 1 {
        return fmt.Errorf("marketdata.my_new_timeout_sec must be >= 1, got %d",
            p.Marketdata.MyNewTimeoutSec.Value)
    }
    return nil
}
```

#### 步驟 5：更新 `configs/parameters.json`

在 `configs/parameters.json` 中對應的區段加入新欄位。

```json
{
  "marketdata": {
    "my_new_timeout_sec": {
      "value": 10,
      "rationale": "Default timeout for X API",
      "source": "heuristic"
    }
  }
}
```

#### 步驟 6：加入 Citation 後設資料（可選，建議）

若參數需要完整的來源追蹤，在 `parameters.json` 中加入 `citation` 區塊：

```json
{
  "marketdata": {
    "my_new_timeout_sec": {
      "value": 10,
      "rationale": "Default timeout for X API",
      "source": "literature",
      "citation": {
        "source_type": "academic_paper",
        "source_reference": "Author et al. (2024)",
        "evidence_quality": "medium",
        "update_policy": "review_quarterly",
        "validation_method": "backtest_optimization",
        "dependencies": [],
        "last_validated": "2026-06-03T00:00:00Z"
      }
    }
  }
}
```

#### 步驟 7：執行 Citation 腳本（如果可用）

```bash
python3 scripts/add_citations.py
```

> **注意**：`scripts/add_citations.py` 會掃描 `parameters.json` 中缺少 `citation` 的欄位並自動補齊模板。

---

## 3. 新增 Provider 接線（Gateway Path）

本節適用於新增一個完整的 Market Data Provider，並將其註冊到 Gateway 通道系統。

### 步驟

#### 步驟 1：建立 Provider 檔案

在 `internal/marketdata/` 目錄下建立 Provider 檔案（如 `my_provider.go`）。

必須實作的介面取決於資料類型：

- **報價資料**：實作 `QuoteProvider` 或 `MarketDataProvider`
- **總經資料**：實作 `MacroDataProvider` 介面，包含 `Name()` 和 `FetchSnapshot(ctx)` 方法

最少需要一個建構子 `NewXxxProvider()`。

```go
package marketdata

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
    "golang.org/x/time/rate"
)

type MyProvider struct {
    client      *http.Client
    rateLimiter *rate.Limiter
}

func NewMyProvider() *MyProvider {
    return &MyProvider{
        client:      httpclient.NewFactory().NewClient(30 * time.Second),
        rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
    }
}

func (p *MyProvider) Name() string {
    return "my_provider"
}

func (p *MyProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
    if err := p.rateLimiter.Wait(ctx); err != nil {
        return MacroDataSnapshot{}, fmt.Errorf("my_provider rate limit: %w", err)
    }
    // ... API 呼叫邏輯
}
```

#### 步驟 2：在 `register_adapters.go` 中註冊

在 `internal/apigateway/register_adapters.go` 的 `RegisterChannelAdapters()` 函式中新增註冊區塊：

```go
// --- My Provider (no API key required) ---
myProvider := marketdata.NewMyProvider()
myAdapter := NewMyChannelAdapter(myProvider)
g.registry.Register("my_provider", myAdapter)
logging.Info("apigateway", "adapter_registered", "channel", "my_provider")
```

#### 步驟 3：在 `limits.go` 中加入 Channel

在 `internal/apigateway/limits.go` 中：

1. 新增 Rate/Burst 常數（第 12-52 行區域）
2. 在 `NewRateLimitManager()` 的 `limiters` map 中加入 channel

```go
// 常數
MyProviderRate  = rate.Every(5 * time.Second)
MyProviderBurst = 1

// limiters map
"my_provider": rate.NewLimiter(MyProviderRate, MyProviderBurst),
```

#### 步驟 4：若為總經資料，在 `gateway_adapter.go` 中新增 channel mapping

在 `internal/monitoring/gateway_adapter.go` 的 `FetchSnapshot()` 方法（第 32-49 行的 `channels` slice）中加入新的 channel mapping：

```go
{channelID: "my_provider", apply: a.applyMyProvider},
```

並實作對應的 `applyMyProvider` 方法：

```go
func (a *macroDataGatewayAdapter) applyMyProvider(snap *marketdata.MacroDataSnapshot, data []byte) {
    var s marketdata.MacroDataSnapshot
    if err := json.Unmarshal(data, &s); err != nil {
        return
    }
    // 將解析結果合併到 snap 的對應欄位
}
```

#### 步驟 5：加入 `channelIDs()`

在 `internal/apigateway/gateway.go` 的 `channelIDs()` 函式（第 143-168 行）中加入新的 channel ID。

#### 步驟 6：更新 constitution.md 附錄 A

在 `internal/apigateway/CONSTITUTION.md` 的附錄 A 通道規範表（第 479-498 行）中新增一列。

#### 步驟 7：更新 2026-06-26-gateway-migration-tracking.md

在 `gateway migration 追蹤（已合併進 AGENTS.md）` 中記錄新增狀態。

---

## 4. 檔案寫入規範

本節規範所有生產環境狀態檔案與設定檔的寫入方式，防止資料損毀。

### 核心原則

| 檔案類型 | 必須使用的方法 | 位置 |
|----------|---------------|------|
| **設定檔** (`parameters.json` 等) | `SaveWithRollback()` | `internal/config/parameters.go` 第 2448 行 |
| **狀態檔** (JSONL、portfolio state 等) | `.tmp + os.Rename` 雙段寫入 | `internal/live/store/store.go` `WriteFileAtomic()` |
| **暫存檔案** | `os.CreateTemp()` | 不要用硬編碼 `.tmp` 後綴 |
| **生產環境** | 禁止直接 `os.WriteFile()` | 沒有 rollback 保護 |

### 詳細規範

#### 4.1 設定檔：使用 `SaveWithRollback()`

`internal/config/parameters.go` 中的 `SaveWithRollback()` 實作了三段式安全寫入（第 2448-2486 行）：

```
.tmp → fsync → rename 現有 → .bak → rename .tmp → 目標 → 刪除 .bak
```

若任一步驟失敗，會自動從 `.bak` 還原，保證不會遺失原始設定。

```go
// ✅ 正確：使用 SaveWithRollback
if err := cfg.SaveWithRollback(parametersPath); err != nil {
    return fmt.Errorf("save parameters: %w", err)
}

// ❌ 錯誤：直接 os.WriteFile 寫入設定檔
data, _ := json.MarshalIndent(cfg, "", "  ")
os.WriteFile(path, data, 0o644)
```

#### 4.2 狀態檔：使用 `.tmp + os.Rename` 模式

`internal/live/store/store.go` 中的 `WriteFileAtomic()` 是 canonical 範例（第 370-391 行）：

```go
// WriteFileAtomic writes content to a temporary file and atomically renames it.
func WriteFileAtomic(path, content string) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-*.jsonl")
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }
    tmpPath := tmp.Name()
    defer func() { _ = os.Remove(tmpPath) }()

    if _, err := tmp.WriteString(content + "\n"); err != nil {
        _ = tmp.Close()
        return fmt.Errorf("write temp file: %w", err)
    }
    if err := tmp.Close(); err != nil {
        return fmt.Errorf("close temp file: %w", err)
    }

    if err := os.Rename(tmpPath, path); err != nil {
        return fmt.Errorf("rename temp file: %w", err)
    }
    return nil
}
```

關鍵要點：
- 使用 `os.CreateTemp()` 而非手動拼接 `.tmp` 路徑（避免碰撞與權限問題）
- 寫入完成後 `Close()` 再 `os.Rename`（確保資料完整 flush 到磁碟）
- `defer os.Remove(tmpPath)` 確保失敗時清理殘留暫存檔

#### 4.3 禁止事項

```go
// ❌ 禁止：直接 os.WriteFile 寫入生產狀態檔
os.WriteFile("data/state/positions_current.json", data, 0o644)

// ❌ 禁止：手動硬編碼 .tmp 路徑
tmpPath := path + ".tmp"
os.WriteFile(tmpPath, data, 0o644)
os.Rename(tmpPath, path)

// ✅ 正確：使用 os.CreateTemp
tmp, _ := os.CreateTemp(dir, ".tmp-*")
tmp.Write(data)
tmp.Close()
os.Rename(tmp.Name(), path)
```

---

## 5. 背景任務註冊

本節規範所有背景排程任務（資料擷取、健康檢查、維護清理）的註冊方式。

### 核心原則（來自 constitution.md 第四條）

> **禁止獨立 goroutine 直接調用 `New*Provider()`。**
> 所有背景任務必須註冊到 `BackgroundTaskManager`，統一調度。

### 參考範例

| 檔案 | 說明 |
|------|------|
| `cmd/atlas/main.go` 第 549-900+ 行 | 所有背景任務的註冊集中點 |
| `internal/apigateway/CONSTITUTION.md` 第 174-331 行 | BackgroundTaskManager 規範 |

### 步驟

#### 步驟 1：在 `cmd/atlas/main.go` 中 `BackgroundTaskManager` 初始化後註冊

所有任務必須在 `taskMgr` 建立後、`taskMgr.Start(ctx)` 之前註冊（參考 `main.go` 第 549 行起）。

```go
taskMgr = apigateway.NewBackgroundTaskManager(gateway)

// 註冊你的任務
_ = taskMgr.Register(&apigateway.ScheduledTask{
    Name:      "my_domain_update",
    ChannelID: "my_provider",
    Interval:  30 * time.Minute,
    Enabled:   true,
    Task: func(ctx context.Context) error {
        _, err := gateway.Fetch(ctx, "my_provider")
        return err
    },
})
```

#### 步驟 2：遵守命名慣例

| 前綴/模式 | 用途 | 範例 |
|-----------|------|------|
| `auto_` | 自動排程資料擷取 | `auto_capital_flow`、`auto_margin`、`auto_export`、`auto_backfill` |
| `channel_health_` | 通道健康檢查 | `channel_health_fugle`、`channel_health_sync` |
| `channel_` | 通道同步（非 `auto_`） | `channel_health_sync` |
| `<domain>_update` | 領域特定更新 | `tsmc_revenue`、`margin_history_backfill` |

#### 步驟 3：設定適當的間隔（Interval）

| 資料類型 | 建議間隔 | 原因 |
|----------|---------|------|
| 即時報價 | 不建議使用 TaskManager | 應使用 WebSocket/串流 |
| 盤中資料（法人、融資） | 30 分鐘 | 交易所更新頻率限制 |
| 總經資料（匯率、指數） | 1-12 小時 | 變動較慢 |
| 健康檢查 | 30 秒 - 1 小時 | 取決於通道重要性 |
| 維護任務（清理、回填） | 24 小時 | 低頻率操作 |

**絕對不要設定 < 1 分鐘的間隔給外部 API 任務**——這會觸發 rate limit 封鎖。

#### 步驟 4：加入 IdempotencyKey（如任務可被外部觸發）

若任務可透過 HTTP endpoint 手動觸發，在註冊時傳入 `IdempotencyKey` 避免重複執行。

#### 步驟 5：加入啟動/完成/錯誤日誌

```go
Task: func(ctx context.Context) error {
    log.Printf("[TaskManager] my_task started")
    result, err := gateway.Fetch(ctx, "my_provider")
    if err != nil {
        log.Printf("[TaskManager] my_task failed: %v", err)
        return err
    }
    log.Printf("[TaskManager] my_task completed")
    return nil
},
```

#### 步驟 6：註冊 FailureHandler（全域一次）

在 `main.go` 中，每個任務不需要各自處理失敗；`taskMgr.SetFailureHandler()` 會處理所有任務（參考第 555-561 行）：

```go
taskMgr.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
    if consecutiveFailures >= 3 {
        monitor.Alert(monitoring.AlertLevelError, "background_task",
            fmt.Sprintf("Task %s failed %d consecutive times: %v", name, consecutiveFailures, err),
            map[string]any{"task": name, "consecutive_failures": consecutiveFailures})
    }
})
```

### 例外情況（允許不使用 BackgroundTaskManager）

以下情境不需要註冊到 TaskManager（完整說明請見 `constitution.md` 第 247-260 行）：

| 情境 | 範例 |
|------|------|
| HTTP 伺服器 | `http.Server.ListenAndServe` |
| 一次性平行計算 | WaitGroup 協調的平行最佳化 |
| 事件監聽 | EventBus 訂閱者 goroutine |
| 單次延遲操作 | `context.WithTimeout` 的有限生命週期 |
| WebSocket 連線 | ping/pong keepalive loop |
| 測試 | 所有 `_test.go` 中的輔助 goroutine |
| Simulation 路徑 | 不經 Gateway 的離線模擬 |

---

## 6. 共享客戶端建立（修正現有問題）

本節提供將現有 per-instance Client 轉換為共享 Singleton 的模板，解決多個獨立 token bucket 可能集體超出 API 限制的問題。

### 參考範例

`internal/marketdata/finmind_client.go` — 最完整的共享客戶端範例，包含：
- `sync.Once` 初始化
- `sync.RWMutex` 執行期更新
- `ResetSharedXxxClient()` 測試重置
- 向後相容的 `NewXxxClient()`

### 步驟

#### 步驟 1：在 Provider 檔案頂部加入 `sync.Once` + `sync.RWMutex`

```go
var (
    sharedMyClient     *MyClient
    sharedMyClientOnce sync.Once
    sharedMyClientMu   sync.RWMutex
)
```

#### 步驟 2：加入 `GetSharedMyClient()` 函式

```go
// GetSharedMyClient returns a singleton MyClient shared across all call sites.
// Using a single client ensures one token bucket enforces the rate limit globally.
func GetSharedMyClient(apiKey string) *MyClient {
    sharedMyClientOnce.Do(func() {
        sharedMyClient = &MyClient{
            apiKey:      apiKey,
            httpClient:  httpclient.NewFactory().NewClient(30 * time.Second),
            rateLimiter: rate.NewLimiter(rate.Every(time.Second), 1),
        }
    })
    return sharedMyClient
}
```

#### 步驟 3：加入 `ResetSharedMyClient()` 供測試使用

```go
// ResetSharedMyClient clears the singleton (for tests).
func ResetSharedMyClient() {
    sharedMyClientMu.Lock()
    defer sharedMyClientMu.Unlock()
    sharedMyClient = nil
    sharedMyClientOnce = sync.Once{}
}
```

> **重要**：`sync.Once` 在 Go 中無法直接重置。必須同時將 `sharedMyClient` 設為 `nil` 並重新賦值 `sync.Once{}`，才能在下一次呼叫時重新初始化。

#### 步驟 4：替換所有 `NewMyClient()` 呼叫點為 `GetSharedMyClient()`

逐步搜尋並替換：

```bash
grep -rn "NewMyClient(" --include="*.go" .
```

替換為 `GetSharedMyClient()` 呼叫。

#### 步驟 5：保留 `NewMyClient()` 並加入棄用註解（向後相容）

```go
// NewMyClient creates a standalone MyClient with its own rate limiter.
// Prefer GetSharedMyClient in production to avoid multiple independent
// token buckets that can collectively exceed the rate limit.
// Deprecated: use GetSharedMyClient for production code.
func NewMyClient(apiKey string) *MyClient {
    return &MyClient{
        apiKey:      apiKey,
        httpClient:  httpclient.NewFactory().NewClient(30 * time.Second),
        rateLimiter: rate.NewLimiter(rate.Every(time.Second), 1),
    }
}
```

#### 步驟 6：執行測試驗證

```bash
# 執行單元測試
go test ./internal/marketdata/...

# 若有用到共享客戶端的整合測試，確保先呼叫 Reset
go test -v ./internal/... -run "TestMyClient"
```

### 共享 Limiter 的替代模式

若不需要完整 Singleton（例如只是一個 package-level rate limiter），可參考 `bdi_provider.go`（第 17 行）的簡化模式：

```go
var bdiSharedLimiter = rate.NewLimiter(rate.Every(5*time.Second), 1)
```

這適用於：Provider 本身無狀態（不需 API key）、只需限制呼叫頻率的場景。

---

## 附錄：快速查核表

以下為每個 PR 必須檢查的項目：

### 新增 API Client

- [ ] 搜尋是否已有共享 Singleton（`grep -r "GetShared"`）
- [ ] 加入 `sync.Once` + `sync.RWMutex`
- [ ] 加入 `GetSharedXxxClient()`
- [ ] 在 `limits.go` 中註冊 rate limiter
- [ ] 在 `gateway.go` 的 `channelIDs()` 中加入
- [ ] 在 `register_adapters.go` 中註冊 adapter
- [ ] 更新 `constitution.md` 附錄 A
- [ ] 更新 `gateway migration 追蹤（已合併進 AGENTS.md）`

### 新增 ParameterMetadata

- [ ] 在 `parameters.go` 結構體中新增欄位
- [ ] 在 `parameters_defaults.go` 中加入 `ParameterMetadata` 預設值
- [ ] 在對應 `mergeXxxDefaults()` 中加入合併邏輯
- [ ] 在 `Validate()` 中加入驗證（如需要）
- [ ] 更新 `configs/parameters.json`
- [ ] 加入 citation 後設資料（如適用）
- [ ] 執行 `scripts/add_citations.py`（如可用）

### 新增 Gateway Path

- [ ] 建立 `internal/marketdata/` 下的 Provider 檔案
- [ ] 在 `register_adapters.go` 中註冊
- [ ] 在 `limits.go` 中加入 channel
- [ ] 若為總經資料：在 `gateway_adapter.go` 中加入 channel mapping
- [ ] 在 `gateway.go` 的 `channelIDs()` 中加入
- [ ] 更新 `constitution.md` 附錄 A
- [ ] 更新 `gateway migration 追蹤（已合併進 AGENTS.md）`

### 檔案寫入

- [ ] 設定檔使用 `SaveWithRollback()`
- [ ] 狀態檔使用 `.tmp + os.Rename`（參考 `WriteFileAtomic()`）
- [ ] 暫存檔使用 `os.CreateTemp()`
- [ ] 沒有直接 `os.WriteFile()` 寫入生產檔案

### 背景任務

- [ ] 透過 `BackgroundTaskManager.Register()` 註冊
- [ ] 名稱符合命名慣例（`auto_` / `channel_health_`）
- [ ] Interval >= 1 分鐘（外部 API）
- [ ] 加入啟動/完成/錯誤日誌
- [ ] 沒有獨立 `go func()` 直接調用 Provider

---

## 附錄 B：相關文件索引

| 文件 | 路徑 |
|------|------|
| 數據源憲法 | `internal/apigateway/CONSTITUTION.md` |
| Gateway 遷移追蹤（已封存） | `gateway migration 追蹤（已合併進 AGENTS.md）` |
| 參數預設值 | `internal/config/parameters_defaults.go` |
| 參數結構定義 | `internal/config/parameters.go` |
| 速率限制設定 | `internal/apigateway/limits.go` |
| Adapter 註冊 | `internal/apigateway/register_adapters.go` |
| Channel IDs | `internal/apigateway/gateway.go` (`channelIDs()`) |
| 原子寫入 | `internal/live/store/store.go` (`WriteFileAtomic()`) |
| 背景任務管理 | `cmd/atlas/main.go` |
| 總經 Gateway Adapter | `internal/monitoring/gateway_adapter.go` |
| CI 憲法檢查 | `scripts/ci/check_constitution.sh` |
| Citation 腳本 | `scripts/add_citations.py` |
| FinMind 共享客戶端 | `internal/marketdata/finmind_client.go` |
| Yahoo 共享 Session | `internal/marketdata/yahoo_session.go` |
| BDI 共享 Limiter | `internal/marketdata/bdi_provider.go` |

---

> **本文件由 Atlas 開發團隊維護，任何修改需經 PR 審查通過。**
