# Atlas 數據源憲法（Data Source Constitution）

**版本**：v1.0  
**生效日期**：2026-05-13  
**適用範圍**：所有新增或修改外部數據源抓取的程式碼  
**執行機制**：CI 自動檢查 + PR 人工審查  

---

## 前言

本系統目前管理 **16 個信息通道**、**3 套健康檢查實作**、**9 個背景任務**，存在以下結構性問題：

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
		// Yahoo Finance: 共享 limiter（us_yahoo 共用同一端點）
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
		// ✅ 合規：us_yahoo 使用共用 limiter
yahooLimiter := rate.NewLimiter(YahooFinanceRate, 1)

registry.Register("us_yahoo", provider, yahooLimiter)
registry.Register("frankfurter_fx", provider, fxLimiter)  // 獨立 limiter，非 Yahoo endpoint

// ❌ 違規：各自獨立 limiter
egistry.Register("us_yahoo", provider, rate.NewLimiter(YahooFinanceRate, 1))
registry.Register("frankfurter_fx", provider, rate.NewLimiter(YahooFinanceRate, 1))
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
# 完整檢查由 scripts/ci/check_constitution.sh 自動執行
bash scripts/ci/check_constitution.sh
```

### 4.5 操作指引（Operational Guidance）

#### 4.5.1 何時應該使用 BackgroundTaskManager

以下情況**必須**使用 BackgroundTaskManager：

| 情境 | 說明 | 範例 |
|------|------|------|
| **定時資料擷取** | 以固定間隔從外部 API 抓取資料 | `auto_capital_flow`（30min）、`macro_ingest`（5min） |
| **週期運算任務** | 定時執行內部計算或狀態更新 | `auto_daily_simulation`（24h）、`auto_experiment`（7d） |
| **維護任務** | 定時清理或同步 | `storage_cleanup`（24h）、`channel_health_sync`（5min） |

#### 4.5.2 例外情況（允許不使用 BackgroundTaskManager）

以下 `go func()` 模式**不需**改用 BackgroundTaskManager：

| 情境 | 說明 | 範例 |
|------|------|------|
| **HTTP 伺服器** | `http.Server.ListenAndServe` 的標準 goroutine | `cmd/atlas/main.go` 的 HTTP server |
| **一次性/有限並行** | WaitGroup 協調的平行計算，完成後結束 | `phase3_controller.go` 的平行最佳化 |
| **事件監聽** | 事件匯流排的訂閱者 goroutine | `eventbus/`、`live/orchestrator.go` 的錯誤通道監聽 |
| **單次延遲操作** | 使用 `context.WithTimeout` 的有限生命週期 goroutine | `backtest.go` 的 timeout-based 執行 |
| **協議保持** | WebSocket ping/pong、連線生命週期 | `fugle_ws.go` 的 ping loop |
| **測試** | 測試環境中的輔助 goroutine | 所有 `_test.go` 檔案 |
| **專用排程編排器** | 內建 `next13_30()` 時間計算的自包含排程迴圈，與 TaskManager 的固定間隔模式不同 | `autobacktest.StartDailyLoop`（與 `auto_daily_simulation` 目的不同：前者為風險訊號回測，後者為每日投資決策）|
| **Live-mode 狀態評估器** | 需要即時 `stateStore` 的長期存活評估器，僅在 live mode 使用 | `ruleEngine.Start(ctx, stateStore)`（live mode 專用；api mode 已透過 TaskManager `rule_engine_check` 使用 `EvaluateRules(nil)`）|
| **元件依賴注入** | Provider 作為內部元件的依賴傳入，而非直接發送 API 請求 | `industry.NewDataAggregator(..., finmindClient)`、`margin_history_loader.go` 的 `MarginHistoryBackfiller`—這些是元件組合，非資料擷取 |
| **Simulation 路徑** | 不經 Gateway 的離線模擬路徑，直接建立 Provider 進行回測 | `orchestrator/system.go`、`orchestrator/composition.go` 中的 provider 建立（無可用 Gateway）|
| **Supervisor 模式 goroutine** | 受 F1-F9 supervisor invariants 管轄的 lifecycle-bound goroutine，與 `Start()`/`Stop()` 配對，使用 cancel context 而非固定 interval | `internal/fubonproxy/manager.go` 的 `supervise()` 主迴圈與 `Start()` 內 `m.waitForHealthy(ctx)` 健康檢查 goroutine（詳見 `.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md`）|

**例外原則**：若該 goroutine 在 `Start()` 時啟動、在 `Stop()` 時結束、且有明確的排程間隔 → **不例外，必須使用 TaskManager**。

#### 4.5.3 註冊流程

```go
// Step 1: 建立 ScheduledTask
taskMgr.Register(&apigateway.ScheduledTask{
    Name:     "my_task",
    ChannelID: "",           // 非資料源任務可留空
    Interval: 30 * time.Minute,
    Jitter:   3 * time.Minute, // 自動預設為 Interval 的 10%
    Enabled:  true,
    Task: func(ctx context.Context) error {
        // 你的業務邏輯
        return nil
    },
})

// Step 2: 所有註冊完成後啟動
taskMgr.Start(ctx)

// 選擇性：設定全域失敗處理器
taskMgr.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
    if consecutiveFailures >= 3 {
        alert(fmt.Sprintf("Task %s failed %d times", name, consecutiveFailures))
    }
})
```

#### 4.5.4 子系統如何取用 TaskManager

建議透過依賴注入（Dependency Injection）傳遞 TaskManager，**禁止使用全域變數或 `init()`**：

```go
// ✅ 合規：透過建構子注入
func NewSubSystem(taskMgr *apigateway.BackgroundTaskManager) *SubSystem {
    taskMgr.Register(&apigateway.ScheduledTask{
        Name: "subsystem_task",
        Interval: 1 * time.Hour,
        Enabled: true,
        Task: func(ctx context.Context) error {
            return s.doWork(ctx)
        },
    })
    return s
}

// ❌ 違規：全域變數 + init()
var taskMgr *apigateway.BackgroundTaskManager
func init() {
    taskMgr = &apigateway.BackgroundTaskManager{} // 違反 DI 原則
}
```

#### 4.5.5 命名規範

| 規範 | 說明 | 範例 |
|------|------|------|
| 名稱使用 snake_case | 統一命名風格 | `auto_capital_flow`、`channel_health_sync` |
| 前綴標示類別 | `auto_` = 自動排程、`channel_` = 資料通道 | `auto_backfill`、`channel_health_sync` |
| 名稱唯一 | 不可與已註冊任務重名 | 註冊時會靜默覆蓋同名任務 |

#### 4.5.6 故障處理

| 行為 | 說明 |
|------|------|
| **連續失敗** | TaskManager 會持續記錄，可透過 `SetFailureHandler` 設置告警 |
| **熔斷互動** | TaskManager **不預先檢查** `breaker.IsOpen()`，一律呼叫 `task.Task(ctx)`，由 `gateway.Fetch` 內部的 `breaker.Call()` 處理 Open→HalfOpen→Closed 轉換。若熔斷中，`gateway.Fetch` 仍會依 Art. 5.4 返回 stale cache + `last_error`，但**任務函式本身必須被執行**，才能讓半開探測有機會觸發。 |
| **重疊保護** | 若前一輪執行尚未完成，下一輪會被跳過（`task_skipped_overlap`） |
| **啟動抖動** | 啟動時自動加入隨機 Jitter（預設 Interval 的 10%），防止驚群效應 |

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
- [ ] 是否更新文件？→ 是否更新 `docs/data-sources.md`？
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

## 附錄 A：16 個通道規範

| 通道 ID | 限流策略 | 健康檢查模式 | 背景任務 | 熔斷啟用 |
|---------|---------|-------------|---------|---------|
| us_yahoo | 共享 1/s | Liveness | 否 | ✅ |
| twse_replay | 不限流 | Readiness | ✅ (24h) | ❌ |
| twse_capital_flow | 1/5s | Readiness | ✅ (30min) | ✅ |
| fugle | 60/min | Liveness | 否 | ✅ |
| fubon | Per-min | Liveness | 否 | ✅ |
| finmind | 6/s (免費) | Liveness | 否 | ✅ |
| frankfurter_fx | 1/10s (獨立) | Liveness | 否 | ✅ |
| geopolitical | 1/min | Liveness | ✅ (6h) | ✅ |
| twse_margin | 1/5s | Readiness | ✅ (30min) | ✅ |
| export_statistics | 1/5s | Readiness | ✅ (12h) | ✅ |
| tsmc_revenue | 繼承 FinMind | Liveness | ✅ (24h) | ✅ |
| geopolitical_taiwan | 1/min | Liveness | ✅ (6h) | ✅ |
| janus_regime | 不限流 | Computed | 否 | ❌ |
| tej | Per-sec + daily | Liveness | 否 | ✅ |
| exchange_rate | 1/5s | Liveness | 否 | ❌ |
| sox_index | 1/5s | Liveness | 否 | ❌ |
| sector_data | 不限流 | Readiness | 否 | ❌ |
| government_flow | 2s/req（自訂） | Readiness | `government_flow_aggregate` (28h) | ❌ |
| bdi | 1/5s | Liveness | 否 | ❌ |

## 附錄 B：違規處理流程

1. **CI 階段**：自動檢查失敗 → PR 無法合併
2. **Code Review 階段**：Reviewer 發現違規 → 要求修正
3. **生產環境發現**：事後審計發現違規 → 建立 Issue 追踪整改
4. **嚴重違規**（如未經 Gateway 直接調用付費 API）→ 立即回滾 + 事故調查

## 附錄 C：修訂歷史

| 版本 | 日期 | 修訂內容 |
|------|------|---------|
| v1.0 | 2026-05-13 | 初版，經 Oracle 審核後發布 |
| v1.1 | 2026-07-01 | 新增附錄 D：Roots Sanctioned Exception（Phase 4 Direction B — MCP Protocol Extensions） |

## 附錄 D：Roots Sanctioned Exception（MCP Server 客戶端宣告檔案根目錄之唯讀例外）

> **定位**：本附錄記錄對「第一條：統一入口原則」之狹義、有限度例外。`atlas-mcp` server 透過 MCP RootsV2 capability 接收 client 宣告的 `file://` 根目錄，並對其下的檔案提供 read-only 讀取（`mcp_roots_read_file` 與 `mcp_roots_list`）。此例外不同於「對外資料源整合」——它是用戶本地端工作目錄的直觀代理（intuitive proxy），不屬於 atlas-go 之資料源治理範疇。

### D.1 例外之嚴格條件

下列條件**必須同時滿足**，違反任一項即視同違反第一條：

1. **方向性**：僅允許 client → server 的 read-only 操作。任何寫入、修改、刪除、重新命名、副本外洩（透過 URL/Path 注入）皆在禁止之列。
2. **路徑遍歷硬化（Path Traversal Hardening）**：
   - `filepath.Clean` 必須在比對前套用
   - `filepath.Abs` 必須解析為絕對路徑
   - **`filepath.EvalSymlinks` 必須在 prefix check 之前呼叫**，以防止 symlink 逃逸出宣告的根目錄（attacker 在宣告根目錄內建立指向 `/etc/passwd` 的 symlink 即構成逃逸）
3. **檔案大小上限**：預設 1 MiB；超過 cap 一律拒絕（`io.LimitReader` + `info.Size()` 雙重把關）。
4. **常規檔案限制**：僅允許 regular file（`Mode().IsRegular()`），拒絕 device、socket、FIFO、symlink-to-non-regular。
5. **Audit 強制**：每一次讀取都必須寫入 audit log（JSONL），欄位包含 `path`、`size_bytes`、`ts`、`tenant_id`，事後可追溯。
6. **綁定位址**：SSE/HTTP 模式必須 bind `127.0.0.1`（不對外暴露），依循既有部署規範（`docs/operations/mcp-deploy.md`）。
7. **Capability 缺失處理**：client 未宣告 `RootsV2` capability 時，回傳 **explicit error**（`errors.New("mcp_roots_list: client does not support roots")`）而非 soft fallback（與 `mcp_sample_llm`、`mcp_elicit_user` 之處理一致）。

### D.2 適用工具

| 工具 | 說明 | Capability Gate |
|------|------|-----------------|
| `mcp_roots_list` | 列出 client 宣告的 `file://` 根目錄 | `RootsV2` |
| `mcp_roots_read_file` | 讀取宣告根目錄下之檔案（唯讀） | `RootsV2` |

### D.3 預設值與環境旗標

| 環境變數 | 用途 | 預設 |
|----------|------|------|
| `ATLAS_MCP_ROOTS_ENABLED` | 啟用 roots 工具（Phase 4 B 已 ship，預設跟隨 server 註冊流程） | `true`（與 RootsConfig 並用） |
| `RootsConfig.ReadSizeCap` | 單次 read size cap（bytes） | `1048576`（1 MiB） |
| `RootsConfig.AllowedRoots` | Client 未宣告 roots 時的靜態 allow-list fallback | `[]` |

### D.4 變更紀律與修訂程序

本例外之任何擴張（新增 write capability、放寬 size cap、放棄 `EvalSymlinks`、新增路徑 glob 支援等）**必須**：

1. 走本憲法第六條（憲法優先原則）之修訂流程（見 附錄 B：違規處理流程）
2. 提交 PR 並於 description 明列「擴張附錄 D 例外邊界」字樣
3. 經 Atlas 數據源治理委員會（或同等架構審查會議）簽核
4. 在「附錄 C：修訂歷史」新增一行

### D.5 對應的 PR / 變更

| PR / Branch | 日期 | 變更摘要 |
|-------------|------|---------|
| `feat/mcp-protoext`（Phase 4 Direction B） | 2026-07-01 | 新增 `mcp_roots_list` + `mcp_roots_read_file` + 本附錄 D；Oracle 審核發現 P0 後追加 `EvalSymlinks` 對稱防護與 capability-miss explicit error |

### D.6 後續追蹤（Follow-up）

- [ ] **架構審查簽核**：本附錄 D 為初始草擬版本，須於下次 Atlas 數據源治理委員會會議提交正式簽核。會議結論將以本附錄之 v1.2 條目記錄。
- [ ] **Audit retention 政策**：目前 audit log 預設無 fsync；長期保留（≥ 90 天）政策待制定。
- [ ] **Symlink escape regression test**：`TestMCPRootsReadFile_SymlinkEscape_Rejected` 已於 PR #B（Phase 4 B）新增；後續需在 CI 增加 fuzz 測試（隨機路徑）。

---

**本憲法由 Atlas 數據源治理委員會維護，任何修改需經架構審查會議通過。**
