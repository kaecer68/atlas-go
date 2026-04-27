# Phase 1 重構實現計劃

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 三項 P2 重構：PluginRegistry 動態載入、Provider streaming 介面、Ledger atomic transaction wrapper

**Architecture:**
- PluginRegistry: 新增 ExecutorLoader 介面，StaticLoader 實作靜態註冊（向後相容），ConfigLoader 預留未來擴展
- Provider: 擴展 StreamingProvider 介面，PollingAdapter 讓現有 provider 支援 streaming
- Ledger: 新增 SessionWriter wrapper，提供 atomic 目錄寫入 + structured error

**Tech Stack:** Go 1.25, sync.Mutex, os.Rename (tmp pattern), context.Context

---

## Task 1: PluginRegistry ExecutorLoader 介面與靜態實作

**Files:**
- Create: `internal/orchestrator/loader.go`
- Modify: `internal/orchestrator/plugin_registry.go:32-54`

- [ ] **Step 1: 建立 loader.go，定義 ExecutorLoader 介面**

```go
// internal/orchestrator/loader.go
package orchestrator

// ExecutorLoader defines the interface for loading executors.
type ExecutorLoader interface {
    LoadRegimeExecutors() ([]RegimeExecutor, error)
    LoadAgentExecutors() ([]AgentExecutor, error)
    LoadControlExecutors() ([]ControlExecutor, error)
}
```

- [ ] **Step 2: 建立 StaticLoader 實作**

```go
// StaticLoader returns hardcoded executors for backward compatibility.
type StaticLoader struct{}

func (StaticLoader) LoadRegimeExecutors() ([]RegimeExecutor, error) {
    return []RegimeExecutor{
        TaiwanMacroRegimeExecutor{},
        ForeignFlowRegimeExecutor{},
    }, nil
}

func (StaticLoader) LoadAgentExecutors() ([]AgentExecutor, error) {
    return []AgentExecutor{
        SemiconductorExecutor{},
        AISupplyChainExecutor{},
        ETFRotationExecutor{},
        FinancialsExecutor{},
        ShippingExecutor{},
        GrowthMomentumExecutor{},
        ValueYieldExecutor{},
        EarningsQualityExecutor{},
        TechnicalBreakoutExecutor{},
    }, nil
}

func (StaticLoader) LoadControlExecutors() ([]ControlExecutor, error) {
    return []ControlExecutor{
        NewCRORiskExecutor(),
        CIOPortfolioExecutor{},
    }, nil
}
```

- [ ] **Step 3: 修改 NewPluginRegistry() 接受 loader 參數（可選，預設 StaticLoader）**

```go
func NewPluginRegistry(loaders ...ExecutorLoader) *PluginRegistry {
    loader := StaticLoader{}
    if len(loaders) > 0 {
        loader = loaders[0]
    }

    regime, _ := loader.LoadRegimeExecutors()
    agent, _ := loader.LoadAgentExecutors()
    control, _ := loader.LoadControlExecutors()

    return &PluginRegistry{
        regimeExecutors:  regime,
        agentExecutors:   agent,
        controlExecutors: control,
    }
}
```

- [ ] **Step 4: 驗證編譯**

Run: `go build ./internal/orchestrator/...`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestrator/loader.go internal/orchestrator/plugin_registry.go
git commit -m "refactor(orchestrator): add ExecutorLoader interface with StaticLoader"
```

---

## Task 2: StreamingProvider 介面與 PollingAdapter

**Files:**
- Create: `internal/marketdata/streaming.go`
- Modify: `internal/marketdata/provider.go`

- [ ] **Step 1: 在 streaming.go 中定義 QuoteHandler 與 StreamingProvider**

```go
// internal/marketdata/streaming.go
package marketdata

import (
    "context"
    "github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteHandler is a callback for receiving real-time quotes.
type QuoteHandler func(quote domain.Quote)

// StreamingProvider supports real-time quote subscription.
type StreamingProvider interface {
    // Subscribe starts streaming quotes for the given symbols.
    // Handler is called for each quote update. Cancel ctx to stop.
    Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error
    // Unsubscribe stops streaming for the given symbols.
    Unsubscribe(ctx context.Context, symbols []string) error
}

// PollingAdapter wraps a polling Provider to implement StreamingProvider.
type PollingAdapter struct {
    Base      Provider
    Interval  int // seconds
}
```

- [ ] **Step 2: 實作 PollingAdapter.Subscribe**

```go
func (p *PollingAdapter) Subscribe(ctx context.Context, symbols []string, handler QuoteHandler) error {
    ticker := time.NewTicker(time.Duration(p.Interval) * time.Second)
    defer ticker.Stop()

    // Initial fetch
    quotes, err := p.Base.GetQuotes(ctx, time.Now(), symbols)
    if err == nil {
        for _, q := range quotes {
            handler(q)
        }
    }

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            quotes, err := p.Base.GetQuotes(ctx, time.Now(), symbols)
            if err != nil {
                continue
            }
            for _, q := range quotes {
                handler(q)
            }
        }
    }
}

func (p *PollingAdapter) Unsubscribe(ctx context.Context, symbols []string) error {
    // No-op for polling adapter (streaming tied to Subscribe context)
    return nil
}
```

- [ ] **Step 3: 驗證 provider.go 仍然編譯**

Run: `go build ./internal/marketdata/...`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/marketdata/streaming.go
git commit -m "feat(marketdata): add StreamingProvider interface and PollingAdapter"
```

---

## Task 3: Ledger SessionWriter 與 structured error

**Files:**
- Create: `internal/ledger/transaction.go`
- Modify: `internal/orchestrator/system.go:169-176`

- [ ] **Step 1: 建立 transaction.go，定義 WriteError 與 SessionWriter**

```go
// internal/ledger/transaction.go
package ledger

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "sync"

    "github.com/kaecer68/atlas-go/internal/domain"
)

// WriteError wraps ledger write operations with context.
type WriteError struct {
    Op   string // "write_outcomes" | "write_rejects" | "write_summary"
    Path string
    Err  error
}

func (e *WriteError) Error() string {
    return fmt.Sprintf("ledger %s: path=%s: %v", e.Op, e.Path, e.Err)
}

func (e *WriteError) Unwrap() error { return e.Err }

// SessionWriteRequest contains all data for a session write operation.
type SessionWriteRequest struct {
    Session  domain.ReplaySession
    Outcomes []domain.RecommendationOutcome
    Rejects  []domain.ScreeningReject
    Summary  *domain.SessionSummary // nil means skip writing summary
}

// SessionWriter provides atomic session directory writes.
type SessionWriter struct {
    store *Store
    mu    sync.Mutex // per-writer lock for concurrent session writes
}

func NewSessionWriter(store *Store) *SessionWriter {
    return &SessionWriter{store: store}
}
```

- [ ] **Step 2: 實作 WriteSession atomic 方法**

```go
func (w *SessionWriter) WriteSession(ctx context.Context, req SessionWriteRequest) error {
    w.mu.Lock()
    defer w.mu.Unlock()

    sessionDir := w.store.sessionDir(req.Session.ID)

    // Create session directory
    if err := os.MkdirAll(sessionDir, 0o755); err != nil {
        return &WriteError{Op: "mkdir", Path: sessionDir, Err: err}
    }

    // Create temp directory for atomic write
    tmpDir := sessionDir + ".tmp"
    if err := os.MkdirAll(tmpDir, 0o755); err != nil {
        return &WriteError{Op: "mkdir_tmp", Path: tmpDir, Err: err}
    }

    // Write outcomes
    if len(req.Outcomes) > 0 {
        path := filepath.Join(tmpDir, "recommendation_outcomes.jsonl")
        if err := writeOutcomes(path, req.Outcomes); err != nil {
            os.RemoveAll(tmpDir)
            return &WriteError{Op: "write_outcomes", Path: path, Err: err}
        }
    }

    // Write rejects
    if len(req.Rejects) > 0 {
        path := filepath.Join(tmpDir, "screened_symbols.jsonl")
        if err := writeRejects(path, req.Rejects); err != nil {
            os.RemoveAll(tmpDir)
            return &WriteError{Op: "write_rejects", Path: path, Err: err}
        }
    }

    // Write summary if provided
    if req.Summary != nil {
        path := filepath.Join(tmpDir, "summary.json")
        if err := writeSummary(path, req.Summary); err != nil {
            os.RemoveAll(tmpDir)
            return &WriteError{Op: "write_summary", Path: path, Err: err}
        }
    }

    // Atomic rename
    if err := os.Rename(tmpDir, sessionDir); err != nil {
        os.RemoveAll(tmpDir)
        return &WriteError{Op: "rename_tmp", Path: sessionDir, Err: err}
    }

    return nil
}

// Helper functions for writing each file type
func writeOutcomes(path string, outcomes []domain.RecommendationOutcome) error {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if err != nil {
        return err
    }
    defer f.Close()
    enc := json.NewEncoder(f)
    for _, o := range outcomes {
        if err := enc.Encode(o); err != nil {
            return err
        }
    }
    return nil
}

func writeRejects(path string, rejects []domain.ScreeningReject) error {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if err != nil {
        return err
    }
    defer f.Close()
    enc := json.NewEncoder(f)
    for _, r := range rejects {
        if err := enc.Encode(r); err != nil {
            return err
        }
    }
    return nil
}

func writeSummary(path string, summary *domain.SessionSummary) error {
    bytes, err := json.MarshalIndent(summary, "", "  ")
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

- [ ] **Step 3: 修改 system.go 使用 SessionWriter**

在 `RunDailySimulation` 中找到三個獨立寫入：
```go
_ = s.ledger.RecordOutcomes(outcomes)
_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
_ = s.ledger.RecordSessionScreeningRejects(s.session.ID, rejects)
```

替換為：
```go
sessionWriter := ledger.NewSessionWriter(s.ledger)
err := sessionWriter.WriteSession(ctx, ledger.SessionWriteRequest{
    Session:  s.session,
    Outcomes: outcomes,
    Rejects:  rejects,
    Summary:  nil, // summary written separately by RecordSessionSummary
})
if err != nil {
    log.Printf("[Ledger] session write failed: %v", err)
}
```

- [ ] **Step 4: 驗證編譯**

Run: `go build ./internal/ledger/... && go build ./internal/orchestrator/...`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/ledger/transaction.go internal/orchestrator/system.go
git commit -m "feat(ledger): add SessionWriter for atomic session writes"
```

---

## Task 4: 整合驗證

**Files:**
- Modify: `internal/orchestrator/system.go:171-186`

- [ ] **Step 1: 確認 system.go 中 RecordSessionSummary 也改用 SessionWriter**

找到 `RecordSessionSummary` 呼叫：
```go
return s.ledger.RecordSessionSummary(s.session, summary)
```

保持不變（因為 SessionWriter.WriteSession 的 Summary 是可選的，Summary 需要單獨寫入）。

- [ ] **Step 2: 執行完整測試**

Run: `go test ./internal/orchestrator/... ./internal/ledger/... ./internal/marketdata/... -v -count=1`
Expected: ALL PASS

- [ ] **Step 3: 執行 gofmt 檢查**

Run: `test -z "$(gofmt -l .)"`
Expected: (empty output)

- [ ] **Step 4: 提交**

```bash
git add -A
git commit -m "test: verify phase1 refactor - all tests pass"
```

---

## Self-Review Checklist

1. **Spec coverage:**
   - [x] PluginRegistry: ExecutorLoader interface + StaticLoader implementation
   - [x] Provider: StreamingProvider interface + PollingAdapter
   - [x] Ledger: SessionWriter with atomic tmp+rename

2. **Placeholder scan:** 無 "TBD"、"TODO"、或模糊描述

3. **Type consistency:**
   - `SessionWriteRequest` 欄位名稱一致
   - `WriteError` 的 Op/Path/Err 結構正確
   - `WriteSession` ctx 參數傳遞正確

---

## Execution Choice

**"Plan complete and saved to `docs/superpowers/plans/2026-04-27-phase1-refactor.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?"**