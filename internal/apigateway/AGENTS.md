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
| `FetchResult` | `provider.go` | 結果包裝：Data + Meta + Stale/Fallback/LastError (circuit-breaker fallback 標記) |

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
| **FetchResult.Fallback 語意** | 當 circuit-breaker 開啟且 stale cache 命中時設為 `true`,呼叫端應視為 last-known-good 而非新鮮資料。對應 `LastError` 帶有原始錯誤訊息。 |
| **新通道必須註冊兩處** | `limits.go` 加限速 + `gateway.go` 的 `channelIDs()` 加列舉 |
| **禁止裸 goroutine 定時任務** | 所有排程必須用 `BackgroundTaskManager`，參見 `CONSTITUTION.md` 第四條 |
| **frankfurter_fx 使用獨立限速器** | Frankfurter API (api.frankfurter.app) 獨立限速，不再與 us_yahoo 共用 |
| **janus_regime / sector_data 無限速** | `rate.Inf` 標記，不會觸發等待 |
| **overlap 保護跳過執行** | 前次任務未結束時，新週期直接 skip，需檢查 log 確認 |
| **CONSTITUTION.md 六條憲法** | CI 強制檢查，違反直接拒絕合併 |
| **YahooEnabled 預設為 true（2026-06 翻轉）** | `internal/apigateway/register_adapters.go` 的 4 個 `if cfg.YahooEnabled` 守門（line 97 + 168 + 183 + 198）預設都會通過；8 個 US 通道（us_spx/us_ndx/us_dji/us_nvda/us_aapl/us_msft/sox_index/tsm_adr + 既有 us_yahoo macro）會全部註冊。明確 opt-out 設 `ATLAS_YAHOO_ENABLED=false`。此翻轉與 PR #484 的 4-layer data-visibility safeguard 互補：PR #484 是上層防護（偵測任何 channel 失敗並透過 `data_status="degraded"` + `failed_channels` 暴露），預設翻轉是下層預防（讓 US 通道不再因為環境變數未設而啞火）。Safeguard 對其他失敗模式（rate limit、network、Yahoo API outage）仍必要。詳見 `.claude/skills/atlas-data-visibility/SKILL.md`。 |
| **CircuitBreaker 擴充方法（Wave 12 Phase 1，Issue #736）** | `SetManualOverride(true)` 讓 success 不再 auto-close — 唯一回 Closed 的路徑是 `Reset()`（會一次清掉 state + failures + halfOpenCalls + manualOverride）。原 Channel breaker 不主動使用；此 API 為 `llm_annotator` budget callback 設計（PR #730 deprecation boundary 的對接點）。Phase 2 已將 `llm_annotator` 本地複製品換成此 canonical owner（Issue #731）。 |
| **ChannelHealthStore 是 apigateway canonical（Wave 12 Phase 2，Issue #731）** | 原本位於 `internal/monitoring/channel_health.go` 的 `ChannelHealthStore`、`ChannelHealthRecord`、`ChannelAlert`、`ChannelFetchLogEntry`、`RecordOption`、`WithLatencyMs`、`WithRateLimitRemaining`、`NewChannelHealthStore`、`NewChannelHealthStoreWithPool`、`RecordChannelFetch`、`RecordChannelFetchWithPool` 全部搬到 `apigateway/channel_health.go`。`monitoring/channel_health_aliases.go` 提供 type aliases 向後相容。**新增**：`apigateway.RecordChannelHealthFromResult`（在 `apigateway/health.go`，與 `apigateway/channel_health.go` 的 `RecordChannelFetch*` 並存 — 後者是 CLI tools 用的舊名 wrapper helper，前者是 `*UnifiedHealthStore` 的 result-based wrapper）。此搬遷打破 4 層 transitive cycle `llm_annotator → apigateway → monitoring → llm/capabilities → llm_annotator`，讓 `llm_annotator` 可以直接 import `apigateway` 使用 CircuitBreaker canonical owner。 |
| **ChannelHealthStore Alerts() stale threshold（2026-06-30）** | `Alerts()` 預設過濾 `LastFetchAt` 超過 `DefaultStaleThreshold`（**1 hour**）的非 ok 記錄 — 解 production dashboard bug（27 天前 transient DNS 失敗的 record 跟 fresh channel 並列誤導判讀）。**設計選擇**：用 `WithStaleThreshold(d)` + `WithNowFunc(now)` 注入（沿用 CircuitBreaker WithNowFunc pattern），不修改 `ChannelHealthRecord` schema；`threshold = 0` disable filter；unparseable timestamp 防禦性保留而非靜默丟棄。`Get(channelID)` 不過濾（直查 raw data，給 audit 用）。 |
| **CircuitBreaker 加 WithNowFunc 假時鐘注入（Wave 12 Phase 2，Issue #731）** | `apigateway.CircuitBreaker` 新增 `WithNowFunc(now func() time.Time)` method，預設為 `time.Now`。`now` 欄位用 `atomic.Pointer` 儲存，讓 `timeNow()` 能在不重新取得 `cb.mu` 鎖的情況下讀取（避免與 `Call` / `ForceOpen` 持鎖路徑 deadlock）。用途：測試可以注入 deterministic clock 模擬 recovery timeout 邊界，取代原本 `lastFailure = time.Now().Add(-X)` 直接寫欄位的 workaround。 |

## 測試

- `background_test.go`：排程與 overlap 測試
- `limits_test.go`：限速器行為測試
- `circuitbreaker_test.go`：熔斷狀態轉換測試
- `gateway_test.go`：Fetch 全鏈路測試
