# Atlas 數據源憲法（Data Source Constitution）

**版本**：v1.0  
**生效日期**：2026-05-13  
**適用範圍**：所有新增或修改外部數據源抓取的程式碼  
**執行機制**：CI 自動檢查 + PR 人工審查  

---

## 前言

本系統目前管理 **14 個信息通道**、**3 套健康檢查實作**、**8 個背景任務**，存在以下結構性問題：

- 7 個通道無 Rate Limiter，高頻調用時容易被封鎖
- 3 套健康檢查實作導致兩頁面（總覽 vs 信息通道管理）顯示不一致
- 8 個背景任務零協調（無抖動、無熔斷、無互斥）
- FinMind 免費 API 已停用（402），需替代方案
- 散落式 `os.Getenv` 和 `New*Provider()` 調用導致維護困難

本憲法旨在建立統一規範，防止未來迭代時再次出現「AI 偷跑建立獨自數據源抓取」的情況。

---

## 第一條：統一入口原則

### 1.1 核心規定

**任何代碼禁止直接調用 `os.Getenv("XXX_API_KEY")` 建立 HTTP client。**

所有外部 API 必須通過 `apigateway.Fetch(channelID)` 或 `ProviderRegistry` 調用。

### 1.2 允許的例外（白名單）

以下場景允許直接使用 `os.Getenv`，但必須註冊在 `configs/allowed_env_vars.md` 中：

| 變數名稱 | 用途 | 允許位置 |
|---------|------|---------|
| `ATLAS_API_KEY` | API 認證（非數據源） | `cmd/atlas/main.go` |
| `ATLAS_STATE_DIR` | 狀態目錄路徑 | `cmd/atlas/main.go` |
| `ATLAS_WORK_DIR` | 工作目錄路徑 | `cmd/atlas/main.go` |

### 1.3 違規示例

```go
// ❌ 違規：直接建立 HTTP client
apiKey := os.Getenv("FINMIND_API_KEY")
client := &http.Client{}
resp, _ := client.Get("https://api.finmindtrade.com/...")

// ✅ 合規：通過 Gateway 調用
result, meta, err := gateway.Fetch(ctx, "finmind")
```

### 1.4 CI 檢查

```bash
# scripts/ci/check_constitution.sh
# 檢查所有 os.Getenv 是否在白名單中
grep -r "os.Getenv" --include="*.go" . | grep -v "configs/allowed_env_vars.md" > /tmp/env_violations.txt
if [ -s /tmp/env_violations.txt ]; then
    echo "憲法違規：發現未授權的 os.Getenv 調用"
    cat /tmp/env_violations.txt
    exit 1
fi
```

---

## 第二條：強制限流原則

### 2.1 核心規定

**每個 Channel 必須註冊 RateLimiter，預設保守策略（1 req / 5s）。**

### 2.2 限流常數（硬編碼）

所有限流參數必須硬編碼在 `internal/apigateway/limits.go` 中，禁止動態計算：

```go
package apigateway

import "golang.org/x/time/rate"

const (
    // Yahoo Finance: 共享 limiter（us_yahoo + jpy_yahoo 共用同一端點）
    YahooFinanceRate = rate.Every(1 * time.Second)
    
    // TWSE OpenAPI: 5 req/sec per IP
    TWSEOpenAPIRate = rate.Every(200 * time.Millisecond)
    
    // TWSE 三大法人: 保守策略
    TWSECapitalFlowRate = rate.Every(5 * time.Second)
    
    // FinMind: 免費 600/hr
    FinMindFreeRate = rate.Every(6 * time.Second)
    FinMindPaidRate = rate.Every(1 * time.Second + 800*time.Millisecond)
    
    // Fugle: 60/min (Basic)
    FugleBasicRate = rate.Every(time.Second)
    
    // Geopolitical: RSS 源
    GeopoliticalRate = rate.Every(10 * time.Second)
    
    // Export Statistics: 手動觸發不應受限
    ExportStatisticsRate = rate.Every(5 * time.Second)
)
```

### 2.3 共享限流器

共享同一 API 端點的通道必須使用同一個 `rate.Limiter` 實例：

```go
// ✅ 合規：us_yahoo 和 jpy_yahoo 共用 limiter
yahooLimiter := rate.NewLimiter(YahooFinanceRate, 1)

registry.Register("us_yahoo", provider, yahooLimiter)
registry.Register("jpy_yahoo", provider, yahooLimiter)

// ❌ 違規：各自獨立 limiteregistry.Register("us_yahoo", provider, rate.NewLimiter(YahooFinanceRate, 1))
registry.Register("jpy_yahoo", provider, rate.NewLimiter(YahooFinanceRate, 1))
```

---

## 第三條：健康追蹤原則

### 3.1 核心規定

**所有調用必須回傳 metadata（latency_ms, rate_limit_remaining, timestamp）。**

### 3.2 Metadata 結構

```go
type FetchMetadata struct {
    ChannelID           string    `json:"channel_id"`
    LatencyMs           int64     `json:"latency_ms"`
    RateLimitRemaining  int       `json:"rate_limit_remaining"`
    Timestamp           time.Time `json:"timestamp"`
    Cached              bool      `json:"cached"`
    Stale               bool      `json:"stale"`
}
```

### 3.3 健康檢查統一入口

所有健康檢查必須通過 `UnifiedHealthStore`，禁止直接操作 `channel_health.json`：

```go
// ✅ 合規
gateway.Health().Record(channelID, status, errMsg, 
    apigateway.WithLatencyMs(meta.LatencyMs),
    apigateway.WithRateLimitRemaining(meta.RateLimitRemaining),
)

// ❌ 違規：直接寫入 JSON
store := monitoring.NewChannelHealthStore(dir)
store.Record(channelID, status, errMsg)
```

### 3.4 健康檢查語義統一

| 檢查類型 | 適用通道 | 說明 |
|---------|---------|------|
| **Liveness** | Fugle, Fubon, FinMind, TEJ | 實際發 API 請求測試連通性 |
| **Readiness** | TWSE Replay, Capital Flow, Margin, Export | 檢查本地檔案存在性與更新時間 |
| **Computed** | Janus Regime | 檢查內部計算狀態 |

禁止混合語義——同一頁面中所有通道必須使用同一種檢查模式或明確標註。

---

## 第四條：背景任務註冊制

### 4.1 核心規定

**禁止獨立 goroutine 直接調用 `New*Provider()`。**

所有背景任務必須註冊到 `BackgroundTaskManager`，統一調度。

### 4.2 註冊介面

```go
type BackgroundTaskManager struct {
    gateway  *Gateway
    registry map[string]*ScheduledTask
}

type ScheduledTask struct {
    Name     string
    Interval time.Duration
    Jitter   time.Duration
    Task     TaskFunc
    Enabled  bool
}

func (m *BackgroundTaskManager) Register(task ScheduledTask) error {
    // 檢查是否已註冊到 Gateway
    if !m.gateway.HasChannel(task.ChannelID) {
        return fmt.Errorf("task %s: channel %s not registered in gateway", task.Name, task.ChannelID)
    }
    m.registry[task.Name] = &task
    return nil
}
```

### 4.3 違規示例

```go
// ❌ 違規：獨立 goroutine 直接建立 Provider
go func() {
    provider := marketdata.NewTWSECapitalFlowProvider(path)
    provider.Fetch(ctx)
}()

// ✅ 合規：註冊到 TaskManager
taskMgr.Register(&ScheduledTask{
    Name:     "auto_capital_flow",
    Interval: 30 * time.Minute,
    Jitter:   3 * time.Minute,
    Task:     func(ctx context.Context) error { return gateway.Fetch(ctx, "twse_capital_flow") },
})
```

### 4.4 CI 檢查

```bash
# 檢查是否有未註冊的 go func()
grep -r "go func()" --include="*.go" internal/ > /tmp/goroutine_check.txt
# 與註冊表比對，差異報 warning
```

---

## 第五條：熔斷強制原則

### 5.1 核心規定

**連續 3 次失敗自動熔斷，退避 5 分鐘。**

### 5.2 熔斷參數（硬編碼）

```go
const (
    CircuitBreakerFailureThreshold = 3           // 連續失敗次數
    CircuitBreakerRecoveryTimeout  = 5 * time.Minute // 熔斷後恢復時間
    CircuitBreakerHalfOpenMaxCalls = 2            // 半開狀態最大試探次數
)
```

### 5.3 熔斷行為

```go
type CircuitBreaker struct {
    state        State     // Closed, Open, HalfOpen
    failures     int       // 連續失敗計數
    lastFailure  time.Time // 最後失敗時間
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if cb.state == Open {
        if time.Since(cb.lastFailure) < CircuitBreakerRecoveryTimeout {
            return fmt.Errorf("circuit breaker open for channel %s", cb.channelID)
        }
        cb.state = HalfOpen
    }
    
    err := fn()
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        if cb.failures >= CircuitBreakerFailureThreshold {
            cb.state = Open
        }
        return err
    }
    
    cb.failures = 0
    cb.state = Closed
    return nil
}
```

### 5.4 熔斷後行為

熔斷後必須返回緩存數據 + `stale` 標記：

```go
func (g *Gateway) Fetch(ctx context.Context, channelID string) (*FetchResult, error) {
    breaker := g.breakerManager.Get(channelID)
    
    err := breaker.Call(func() error {
        result, err := g.fetchFromProvider(ctx, channelID)
        // ...
    })
    
    if err != nil && breaker.IsOpen() {
        // 返回緩存數據 + stale 標記
        return g.cache.GetStale(channelID)
    }
    
    return result, err
}
```

---

## 第六條：憲法優先原則

### 6.1 核心規定

**任何新增數據源或修改現有數據源的 PR，必須通過憲法合規檢查。**

### 6.2 PR Template Checklist

```markdown
## 數據源變更檢查清單

- [ ] 是否新增數據源？→ 是否註冊到 `apigateway.ChannelRegistry`？
- [ ] 是否新增 API Key？→ 是否加入 `configs/allowed_env_vars.md`？
- [ ] 是否新增背景任務？→ 是否註冊到 `BackgroundTaskManager`？
- [ ] 是否設定 Rate Limiter？→ 是否加入 `internal/apigateway/limits.go`？
- [ ] 是否實作健康檢查？→ 是否通過 `UnifiedHealthStore` 記錄？
- [ ] 是否新增 HTTP 調用？→ 是否通過 `gateway.Fetch()`？
- [ ] 是否更新文件？→ 是否更新 `docs/data_sources.md`？
```

### 6.3 CI 檢查流程

```yaml
# .github/workflows/constitution.yml
name: Constitution Check
on: [pull_request]

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Check os.Getenv usage
        run: bash scripts/ci/check_env_vars.sh
        
      - name: Check gateway registration
        run: bash scripts/ci/check_gateway_registration.sh
        
      - name: Check rate limiter config
        run: bash scripts/ci/check_rate_limits.sh
        
      - name: Check background task registration
        run: bash scripts/ci/check_background_tasks.sh
```

### 6.4 檢查腳本

```bash
#!/bin/bash
# scripts/ci/check_env_vars.sh

# 白名單
ALLOWED_VARS=("ATLAS_API_KEY" "ATLAS_STATE_DIR" "ATLAS_WORK_DIR")

# 檢查所有 os.Getenv
grep -r "os.Getenv" --include="*.go" . | while read -r line; do
    var=$(echo "$line" | grep -oP '(?<=os.Getenv\(").+?(?="\))')
    if [[ ! " ${ALLOWED_VARS[@]} " =~ " ${var} " ]]; then
        echo "❌ 違規: $line"
        echo "   變數 '$var' 不在白名單中"
        exit 1
    fi
done

echo "✅ os.Getenv 檢查通過"
```

---

## 附錄 A：14 個通道規範

| 通道 ID | 限流策略 | 健康檢查模式 | 背景任務 | 熔斷啟用 |
|---------|---------|-------------|---------|---------|
| us_yahoo | 共享 1/s | Liveness | 否 | ✅ |
| twse_replay | 不限流 | Readiness | ✅ (24h) | ❌ |
| twse_capital_flow | 1/5s | Readiness | ✅ (30min) | ✅ |
| fugle | 60/min | Liveness | 否 | ✅ |
| fubon | Per-min | Liveness | 否 | ✅ |
| finmind | 6/s (免費) | Liveness | 否 | ✅ |
| jpy_yahoo | 共享 1/s | Liveness | 否 | ✅ |
| geopolitical | 1/10s | Liveness | ✅ (6h) | ✅ |
| twse_margin | 1/5s | Readiness | ✅ (30min) | ✅ |
| export_statistics | 1/5s | Readiness | ✅ (12h) | ✅ |
| tsmc_revenue | 繼承 FinMind | Liveness | ✅ (24h) | ✅ |
| geopolitical_taiwan | 1/10s | Liveness | ✅ (6h) | ✅ |
| janus_regime | 不限流 | Computed | 否 | ❌ |
| tej | Per-sec + daily | Liveness | 否 | ✅ |

## 附錄 B：違規處理流程

1. **CI 階段**：自動檢查失敗 → PR 無法合併
2. **Code Review 階段**：Reviewer 發現違規 → 要求修正
3. **生產環境發現**：事後審計發現違規 → 建立 Issue 追踪整改
4. **嚴重違規**（如未經 Gateway 直接調用付費 API）→ 立即回滾 + 事故調查

## 附錄 C：修訂歷史

| 版本 | 日期 | 修訂內容 |
|------|------|---------|
| v1.0 | 2026-05-13 | 初版，經 Oracle 審核後發布 |

---

**本憲法由 Atlas 數據源治理委員會維護，任何修改需經架構審查會議通過。**
