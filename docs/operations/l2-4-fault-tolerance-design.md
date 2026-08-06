# L2.4 Cron Trigger — Fault Tolerance Design

> **Status**: Design document. **Not implemented** (per `docs/archive/l2-4-followup.md` §1 — auto-cron is graduation, not v1). ⛔ **2026-08-06: Issue #825 CLOSED**,本設計未實作;自動化缺口由 C07 平行軌道填補。本文件保留作為未來若重啟 L2.4 auto-cron 的設計參考。
> **Prerequisite**: `followup.md` §1 item 3 — "排程 fault tolerance: 設計降級策略 (trigger 失敗時 log warning 不 panic、scheduler 重啟後恢復 in-flight window)"
> **對應**: `docs/archive/l2-4-followup.md` §1 (Issue #825) + `docs/operations/l2-4-runbook.md` §1

---

## Background

L2.4 observation requires automatic start/stop at `DefaultStartTime` (HH:MM) per `internal/config/parameters.go:86` (already defined as `ParameterMetadata[string]`) without manual button clicks. Current codebase has **no cron-style HH:MM trigger primitive** — only interval-based polling.

## Existing Infrastructure (verified, do NOT reinvent)

### `BackgroundTaskManager` — `internal/apigateway/background.go`

Provides:
- **Interval-based execution** with auto-jitter (`0.1 * Interval`, Register L129-131)
- **Failure handler hook** (`SetFailureHandler` L180 / `safeCallFailureHandler` L188) — nested `defer recover` so handler panic cannot crash process
- **Panic recovery** in task execution (`logAndWrapPanic` L207) — wraps panics as errors
- **Consecutive failure counting** (`Failures()` L61 / `RecordFailure()` L75 / `RecordSuccess()` L68)
- **Recovery handler hook** (`SetRecoveryHandler` L222) — fires when task recovers from N+1 failures back to success
- **Status reporting** (`TaskStatus` struct L317) — for dashboard / external observers
- **Overlap protection** — "前次任務未結束時，新週期直接 skip，需檢查 log 確認" (per `apigateway/AGENTS.md`)

### `internal/scheduler/` package

Domain-specific schedulers (`ml_retrain.go`, `auto_calibration.go`, `auto_rollback.go`, `seasonal_task.go`, `strategy_evolution.go`, `system_health.go`) — all **event-triggered**, not cron-style. No `robfig/cron` dependency in repo (`grep` confirms zero hits in `*.go` + `go.mod`).

## Gap

Neither primitive supports "fire at HH:MM on weekdays". Options for closing the gap:

| Option | Pros | Cons |
|---|---|---|
| A. Add `robfig/cron` dependency | Standard cron parsing, battle-tested | New dep, heavier than needed (1 trigger) |
| B. 30s ticker polling + HH:MM match | Zero new deps, reuses existing BTM | 30s latency worst case, no big deal for daily window |
| C. Calculate next fire time once at startup + sleep until | Lowest CPU, deterministic | Sleep interrupted by atlas restart needs recovery handling |

**Recommendation**: Option **B** (30s ticker wrapped as BTM task). Reasons:
- Zero new dependencies
- Reuses ALL existing fault tolerance (panic recovery, failure counter, recovery handler, status)
- Worst-case 30s drift on trigger is irrelevant for a 24h observation window
- Recovery on restart is straightforward (Status() check on first tick)

## Proposed Design

### New file: `internal/scheduler/l2_4.go` (~50 lines)

```go
package scheduler

type L2_4CronTrigger struct {
    l24Mgr            L2_4Manager     // interface, see below
    defaultStartTime  string          // "HH:MM" from parameters.DefaultStartTime
    defaultStopAfter  time.Duration   // observation period length (e.g., 24h)
    lastTriggeredDate string          // dedup: "YYYY-MM-DD" of last start fire
    mu                sync.Mutex
}

type L2_4Manager interface {
    Start() error
    Stop() error
    Status() string  // "idle" | "running" | "stopped"
}

// Tick (called every 30s by BTM wrapper):
func (t *L2_4CronTrigger) Tick(ctx context.Context) error {
    now := time.Now()
    if now.Format("15:04") != t.defaultStartTime {
        return nil  // not trigger time
    }
    if now.Format("2006-01-02") == t.lastTriggeredDate {
        return nil  // already triggered today
    }
    if t.l24Mgr.Status() == "running" {
        return nil  // manual override in progress; cron defers
    }
    if err := t.l24Mgr.Start(); err != nil {
        return fmt.Errorf("l2-4 cron trigger: start: %w", err)
    }
    t.mu.Lock()
    t.lastTriggeredDate = now.Format("2006-01-02")
    t.mu.Unlock()
    return nil
}
```

### Registration with BackgroundTaskManager

```go
btm.Register(&apigateway.ScheduledTask{
    Name:     "l2_4_cron_trigger",
    Interval: 30 * time.Second,
    ChannelID: "",  // no data fetch — pure scheduler
    Func:     l2_4CronTrigger.Tick,
})
btm.SetFailureHandler(func(name string, consecutive int, err error) {
    logging.Warn("l2_4_cron", "trigger_failed",
        "task", name, "consecutive_failures", consecutive, "err", err.Error())
})
btm.SetRecoveryHandler(func(name string, recoveredFrom int) {
    logging.Info("l2_4_cron", "trigger_recovered",
        "task", name, "recovered_from_failures", recoveredFrom)
})
```

## Recovery Scenarios (each one explicit)

| Scenario | Current behavior (no cron) | With this design |
|---|---|---|
| **Trigger failure** (l24Mgr.Start() panics) | N/A (manual) | `logAndWrapPanic` (background.go L207) catches, logs as error. Failure counter increments. No process crash. |
| **Trigger returns error** | N/A | `SetFailureHandler` logs warning, increments `Failures()`. After 3 consecutive, dashboard badge shows "trigger degraded". |
| **Trigger recovers** (error → success) | N/A | `SetRecoveryHandler` fires, logs "trigger_recovered". Dashboard badge clears. |
| **Scheduler restart during in-flight window** | N/A | First tick after restart reads `l24Mgr.Status() == "running"` → skips new start, logs "window in progress, not restarting" |
| **Missed trigger** (atlas down at HH:MM) | N/A | On restart, next tick at HH:MM+30s fires start (1-day window only, not "catch up yesterday") |
| **Duplicate trigger** (30s tick + edge case) | N/A | `lastTriggeredDate` dedup ensures one fire per calendar day |
| **Manual button override** (user clicks Start in synergy page before cron time) | N/A | Cron reads `Status() == "running"` → defers. User's window runs to completion without cron interference. |
| **Stop trigger** (end of 24h period) | N/A (manual) | Separate 30s poll checks `Status() == "running" && time.Since(startedAt) > defaultStopAfter` → calls `l24Mgr.Stop()` |

## What This PR does NOT do

- Does NOT implement the cron code (blocked per followup.md §1)
- Does NOT add `robfig/cron` dependency
- Does NOT modify `BackgroundTaskManager` or `internal/scheduler/` existing types
- Does NOT touch `l24Mgr` (L2.4 manager) — assumes it already exposes Start/Stop/Status interface (verify before implementation)

## Implementation readiness checklist (for future PR)

When followup.md §1 prerequisites are met, the implementer must:

1. Verify `l24Mgr` (from `internal/monitoring/api/pipeline/l2_4_state.go`) exposes `Start()` / `Stop()` / `Status()` matching `L2_4Manager` interface above
2. Confirm `DefaultStartTime` parsing handles "HH:MM" format (currently string in parameters, needs validation)
3. Add `defaultStopAfter` parameter (currently not in parameters.json — needs addition)
4. Wire registration in `cmd/atlas/main.go` startup path
5. Add observability — emit `l2_4_cron.fired` / `l2_4_cron.skipped` / `l2_4_cron.failed` slog events (matching existing `agent_loop.*` event pattern)
6. Add tests for all 8 recovery scenarios in this doc

## References

- `docs/archive/l2-4-followup.md` §1 (Issue #825)
- `docs/operations/l2-4-runbook.md` §1 (Pre-flight)
- `internal/apigateway/background.go` — BackgroundTaskManager + fault tolerance primitives
- `internal/apigateway/AGENTS.md` — overlap protection + jitter contract
- `internal/config/parameters.go:86` — `DefaultStartTime ParameterMetadata[string]`
- `internal/scheduler/` — domain scheduler home for new file
- `cmd/atlas/main.go:450` — where DefaultStartTime is currently read