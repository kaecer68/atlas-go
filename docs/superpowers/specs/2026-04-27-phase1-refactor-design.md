# Phase 1 重構設計文件

## 目標

改進架構可擴展性與錯誤處理，進行三項 P2 重構。

---

## 1. PluginRegistry 動態載入

### 現況
```go
func NewPluginRegistry() *PluginRegistry {
    return &PluginRegistry{
        regimeExecutors: []RegimeExecutor{
            TaiwanMacroRegimeExecutor{},
            ForeignFlowRegimeExecutor{},
        },
        agentExecutors: []AgentExecutor{
            SemiconductorExecutor{},
            AISupplyChainExecutor{},
            // ... 9 個靜態實例
        },
        controlExecutors: []ControlExecutor{
            NewCRORiskExecutor(),
            CIOPortfolioExecutor{},
        },
    }
}
```

### 設計

新增 `ExecutorLoader` 介面與兩種實作：

```go
// loader.go
type ExecutorLoader interface {
    Load() ([]RegimeExecutor, []AgentExecutor, []ControlExecutor, error)
}

// 靜態載入（向後相容，預設）
type StaticLoader struct{}
func (StaticLoader) Load() (regime, agent, control []executor, err error) {
    return []RegimeExecutor{TaiwanMacroRegimeExecutor{}, ForeignFlowRegimeExecutor{}},
           []AgentExecutor{SemiconductorExecutor{}, ...},
           []ControlExecutor{NewCRORiskExecutor(), CIOPortfolioExecutor{}},
           nil
}

// 配置驅動載入（未來擴展）
type ConfigLoader struct{ Path string }
func (c ConfigLoader) Load() (...) { /* 從 YAML 讀取 plugin 名稱並反射實例化 */ }
```

**向後相容**：`NewPluginRegistry()` 預設使用 `StaticLoader`，外部呼叫點不需修改。

---

## 2. Provider Streaming 介面

### 現況
```go
type Provider interface {
    Name() string
    GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}
```

### 設計

擴展介面，新增 streaming 方法（不影響舊有 GetQuotes）：

```go
// streaming.go
type QuoteHandler func(quote domain.Quote)

type StreamingProvider interface {
    // 既有介面（保持不變）
    Provider

    // 訂閱即時報價，回調驅動
    Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error
    Unsubscribe(ctx context.Context, symbols []string) error
}

// 適配器：讓現有 polling provider 也支援 streaming
type PollingAdapter struct {
    base Provider
    interval time.Duration
}
func (p *PollingAdapter) Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error {
    // 內部使用 GetQuotes + time.Ticker 模擬 streaming
}
```

**向後相容**：所有現有 Provider 實作仍然滿足 `Provider` 介面，不需要修改。

---

## 3. Ledger Atomic Transaction

### 現況
```go
// system.go 中三個獨立呼叫
_ = s.ledger.RecordOutcomes(outcomes)
_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
_ = s.ledger.RecordSessionScreeningRejects(s.session.ID, rejects)
```

### 設計

新增 `SessionWriter` wrapper，提供 atomic 寫入：

```go
// transaction.go
type SessionWriteRequest struct {
    Session  domain.ReplaySession
    Outcomes []domain.RecommendationOutcome
    Rejects  []domain.ScreeningReject
    Summary  *domain.SessionSummary  // 可選，nil 表示不寫
}

type SessionWriter struct {
    store *Store
    mu    sync.Mutex  // 保護同 session 的併發寫入
}

func NewSessionWriter(store *Store) *SessionWriter {
    return &SessionWriter{store: store}
}

// Atomic 寫入：全部成功或全部失敗（使用目錄层级保證）
func (w *SessionWriter) WriteSession(ctx context.Context, req SessionWriteRequest) error {
    // 1. 建立 tmp 目錄
    // 2. 寫入所有檔案到 tmp
    // 3. os.Rename tmp → sessions/<id>
    // 失敗時 cleanup tmp
}

// 錯誤分類：新增 structured error type
type WriteError struct {
    Op   string  // "write_outcomes" | "write_rejects" | "write_summary"
    Path string
    Err  error
}
func (e *WriteError) Error() string { return fmt.Sprintf("%s: %s: %v", e.Op, e.Path, e.Err) }
```

**向後相容**：`Store` 的三個舊方法仍然保留，内部已使用 tmp+rename 模式。

---

## 驗證方式

```bash
go build ./...
go test ./internal/orchestrator/... ./internal/marketdata/... ./internal/ledger/...
```

---

## 影響範圍

| 檔案 | 變更 |
|------|------|
| `internal/orchestrator/plugin_registry.go` | 新增 loader 介面與實作 |
| `internal/orchestrator/system.go` | 使用 SessionWriter |
| `internal/marketdata/provider.go` | 新增 StreamingProvider 介面 |
| `internal/ledger/transaction.go` | 新增 SessionWriter struct |