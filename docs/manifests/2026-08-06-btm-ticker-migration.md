# Rogue Ticker → BackgroundTaskManager 遷移方案 — Issue #1447

> **狀態**: 方案 (盤查完成 2026-08-06, 實作待排程)
> **Trigger**: Issue #1447 — a5 audit (2026-07-25) Article 4 列 17 個 rogue ticker,違反 apigateway 憲章 §4.5.2 (固定間隔任務應走 BTM)
> **範圍**: 裸 `time.NewTicker` + goroutine → `apigateway.ScheduledTask` + `BackgroundTaskManager.Register()`

---

## 1. 盤查結果 (2026-08-06 以 `grep time.NewTicker` 全 repo 掃描)

### 1.1 需遷移 (17 個, 對應 issue #1447)

| # | 位置 | 現況 | severity |
|---|------|------|----------|
| 1 | `internal/marketdata/fubon_client.go:308` StartHealthProbe → runHealthProbe | 裸 `go c.runHealthProbe(interval)` | high |
| 2 | `internal/marketdata/streaming.go:26` PollingAdapter.Subscribe | `time.NewTicker(p.Interval)` | high |
| 3 | `internal/marketdata/realtime/router.go:232` RealtimeRouter.healthCheckLoop | `time.NewTicker(period)` | medium |
| 4 | `internal/marketdata/realtime/router.go` failoverLoop | ticker (同檔) | medium |
| 5 | `internal/mcp/anomaly/emitter.go:118` AnomalyEmitter.Run | `time.NewTicker(e.cfg.Interval)` | medium |
| 6 | `internal/metalearning/metalearner.go:292` adaptationLoop | `time.NewTicker(ml.config.AdaptationInterval)` | **high** |
| 7 | `internal/monitoring/service/channel_health_synthesizer.go:65` | `time.NewTicker(c.interval)` | medium |
| 8 | `internal/monitoring/service/drift_detector.go:104` | `time.NewTicker(DriftCheckInterval)` | medium |
| 9 | `internal/monitoring/service/ingestion_lag_monitor.go:66` | `time.NewTicker(m.interval)` | medium |
| 10 | `internal/monitoring/service/regime_debouncer.go:80` | `time.NewTicker(RegimeDebounceCheckInt)` | medium |
| 11 | `internal/prism/prism_manager.go:569` autoBalancer (5min) | `time.NewTicker(5*time.Minute)` + `go pm.autoBalancer(pm.stopCh)` | **high** |
| 12 | `internal/realtime/regime_adapter.go:268` RealTimeAdapter.Start | `time.NewTicker(rta.config.UpdateInterval)` | **high** |
| 13 | `internal/spawning/spawning_manager.go:105` runLoop | `time.NewTicker(m.checkInterval)` | medium |
| 14 | `internal/live/scheduler.go:115,170` Scheduler (quotePoller/intradayProcessor/marketTimeScheduler) | `time.NewTicker` ×2 + goroutine | medium |
| 15 | `internal/monitoring/rules.go:55` | `time.NewTicker(e.checkInterval)` | medium |
| 16 | `cmd/atlas-mcp/server/server.go:209` (24h) | `time.NewTicker(24*time.Hour)` | medium |
| 17 | `cmd/atlas-mcp/server/ratelimit.go:140` idleSweep | `time.NewTicker(r.idleSweep)` | medium |

### 1.2 不遷移 (排除理由)

| 位置 | 排除理由 |
|------|----------|
| `internal/config/filelock.go:61` (10ms) | lock wait spin,非固定間隔業務任務 |
| `internal/apigateway/background.go:313` | BTM 自身實作 |
| `internal/marketdata/realtime/fugle_ws.go:409` (pingPeriod) | WebSocket 協議 keepalive,非背景任務 |
| `internal/metalearning/metalearner.go:292` | **production dead code** — `NewMetaLearner` 0 個非 test 呼叫端 (2026-08-06 grep 驗證)。遷移 dead code = 把未實作功能接上 BTM,不做。|
| `internal/marketdata/fubon_client.go:308` | **exponential backoff 狀態機** (runHealthProbe 有 backoff/failure tracking) — BTM 固定 interval 不匹配。保留原生命週期。|
| `internal/realtime/regime_adapter.go:268` | **100ms sub-second 即時迴圈** (processUpdate) — BTM 固定 interval + health 追蹤不適合高頻;且受 `-allow-realtime` flag gate。保留。|

### 1.3 關鍵: BTM 是否支援 Start/Stop 動態生命週期

`internal/apigateway/background.go`:
- `ScheduledTask{Name, Interval, Task BackgroundTaskFunc, Enabled}` — 固定間隔
- `BackgroundTaskFunc = func(ctx context.Context) error` — **已 ctx-aware** (2026-08-06 驗證, 方案原稿 §2.2 疑慮解除)
- `Register()` / `Start(ctx)` / `Stop()` — 全域管理
- **限制**: 每個 task 單一 Interval, 無 on-demand Start/Stop 語意 — 需動態啟停的任務在 Task func 內自行判 skip, 或用 `Enabled` 動態翻轉

**審計決策 (2026-08-06, kaecer 授權)**: 首批遷移 **`internal/prism/prism_manager.go:569` autoBalancer (5min)** — active + 固定間隔 + `Rebalance()` 現成 runOnce。其餘 16 個依 §1.2 排除或後續批次。

---

## 2. 遷移模式 (Design)

### 2.1 標準模式 (純固定間隔) — prism autoBalancer 實作案例

**遷移前** (prism_manager.go):
```go
// Start 內
if pm.config.AutoBalance {
    pm.wg.Add(1)
    go pm.autoBalancer(pm.stopCh)
}

// autoBalancer (5min)
func (pm *PRISMManager) autoBalancer(stopCh <-chan struct{}) {
    defer pm.wg.Done()
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-stopCh: return
        case <-ticker.C: pm.Rebalance()
        }
    }
}
```

**遷移後** (prism_manager.go):
- `Start()` 移除 autoBalancer goroutine 啟動
- 新增 `RegisterAutoBalancer(taskMgr)`:
```go
// RegisterAutoBalancer wires the 5-min queue rebalancer into the
// BackgroundTaskManager (Issue #1447). Previously a rogue ticker
// goroutine; now lifecycle-managed by BTM.
func (pm *PRISMManager) RegisterAutoBalancer(taskMgr *apigateway.BackgroundTaskManager) {
    if !pm.config.AutoBalance { return }
    _ = taskMgr.Register(&apigateway.ScheduledTask{
        Name:     "prism_auto_balancer",
        Interval: 5 * time.Minute,
        Enabled:  true,
        Task: func(ctx context.Context) error {
            pm.Rebalance()
            return nil
        },
    })
}
```
- 呼叫端 (cmd/atlas/main.go): `prismMgr.RegisterAutoBalancer(taskMgr)` 取代 `go pm.autoBalancer()`

### 2.2 動態啟停任務 (需 ctx / 條件 skip)

BTM ScheduledTask.Task 簽名為 `BackgroundTaskFunc` — 檢查是否為 `func() error` (無 ctx)。若是,遷移時需:
- 在 Task 內用內部 state 判 skip (例如 `if !p.enabled.Load() { return nil }`)
- 或先擴充 BTM 支援 ctx-aware task (較大改動, 建議另案)

### 2.3 分批策略 (17 個)

| Batch | 內容 | 風險 |
|-------|------|------|
| Batch 1 (high) | #6 metalearner, #11 prism autoBalancer, #12 regime_adapter, #1 fubon healthprobe | 高價值, 需仔細驗證 |
| Batch 2 (medium data) | #2 streaming, #3/4 router, #7/8/9/10 monitoring service | 中等 |
| Batch 3 (mcp/cmd) | #5 anomaly emitter, #13 spawning, #14 live scheduler, #15 rules, #16/17 mcp server | 低 |

---

## 3. 上游 / 下游 / 影響

| 方向 | 內容 |
|------|------|
| 上游 | 各 service 的 Start/Stop 呼叫端 — 遷移後 Start 改為 Register, Stop 由 BTM 統一 |
| 下游 | BTM 的 task 列表 (dashboard 顯示)、health 監控 — 17 個新 task 進入統一管理 |
| 風險 | 動態啟停語意丟失 (2.2); 遷移後 task 命名需唯一 |

---

## 4. 測試計畫

- 每個遷移: `runOnce()` 單元測試 + 既有 service test 回歸
- BTM 層: task 註冊 + interval 觸發測試 (BTM 已有)
- `go test -race` (ticker loop 改 BTM 後 goroutine 管理改變)

---

## 5. 審計問題 (給 kaecer)

1. 17 個全遷移 or 只遷 high severity 3 個 (issue 明點名)? issue 說「全部 17 個」但 2-3 天工作量
2. BTM 是否需支援 ctx-aware task? (影響 2.2 的動態啟停任務)
3. Batch 1 (high) 先行獨立 PR?

---

## 6. 參考

- Issue #1447
- `docs/manifests/2026-07-25-channel-architecture-audit/a5-violations.json` Article 4 (16 條 + name collision)
- `internal/apigateway/background.go` (BTM)
- 憲章 §4.5.2 (`internal/apigateway/CONSTITUTION.md`)
