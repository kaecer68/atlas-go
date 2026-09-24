# 調查與修復報告 — twse_replay_sync timeout (2026-09-24)

> **範圍**: `channel_health.twse_replay_sync` status=warn（上游 `STOCK_DAY_ALL` timeout）
> **方法**: 生產機（Mac Mini）唯讀實測 + 本機 httptest 重現 + 參數/retry/budget 重新校準
> **結論一句話**: TWSE 端點本身極快（實測 P50 85ms / P95 158ms / max 170ms，20/20 HTTP 200）；2026-09-23 的失敗是**單次 20s per-attempt client timeout 且完全沒有 retry**，一次罕見慢窗就吃掉整天的 replay sync。修法是**有界 retry + 指數退避**（沿用既有參數系統），並讓呼叫端的 context budget 由 retry policy 推導，而不是寫死 60s。

---

## 1. 症狀與事實鏈（生產實證，唯讀）

| 項 | 值 | 來源 |
|---|---|---|
| channel | `twse_replay_sync` status=**warn**, consecutive_failures=**1** | `data/state/channel_health.json` (2026-09-24) |
| last_error | `... STOCK_DAY_ALL: context deadline exceeded (Client.Timeout exceeded while awaiting headers)` | 同上 |
| last_success_at | 2026-09-22T15:30:02Z | 同上 |
| last_fetch_at | 2026-09-23T15:30:23Z（失敗發生於 fetch 開始後恰 20s） | 同上 |
| 排程 | 容器 cron `30 15 * * *` (UTC) = 台北每日 23:30，每日僅一次 | `docker-compose.crons.yml` |
| task_liveness | `cron_replay_sync` last_run 2026-09-23T15:31:35Z, exit code 1, duration 92s | task liveness |
| replay CSV | `data/replay/tw_extended_90days.csv` mtime 2026-09-23T15:31:35Z，完整到 2026-09-23 | 實測 |

**關鍵判定**: 09-22 成功之後排程只跑過**一次**（每日一次），該次失敗 → warn。**不是断轨**、不是多日阻斷，也**不是資料損毀**：同日 `runGapBackfill`（逐檔 `GetDailyQuote`）把 09-23 的 44 筆補回，CSV 不 stale（`twse_replay` channel 09-24 為 ok）。

## 2. 延遲實測（2026-09-24 13:57 台北，生產機 20 次間隔 1s）

| 指標 | 值 |
|---|---|
| HTTP 200 | 20/20（每次 136,359 bytes） |
| min / P50 / P95 / max (`time_total`) | 0.070s / **0.085s** / **0.158s** / 0.170s |
| `time_starttransfer` P50 / connect | 0.032s / ~0.008s |
| 失敗 | 0/20 |

對照（2026-08-18 調查）：07:17–07:58 台北有 >15s 的慢窗；本次事件在 23:30 台北，不在已知晨間慢窗，全機僅 4 筆歷史時間戳（08-14 / 08-15 / 08-17 / 09-23）→ **慢窗是罕見、無固定時段的暫態**。

## 3. 根因（程式碼層，兩處）

1. **`GetQuotes` 完全沒有 retry**（`internal/marketdata/twse_openapi.go`）：單次 `c.httpClient.Do`，`http.Client.Timeout = marketdata.twse_api_timeout_sec`（20s）一發失敗即回 error。
2. **retry 政策即使存在也幫不上忙**：共用的 `fetchWithRetry` 只重試 429/5xx，**不重試 transport error**；而 TWSE 的失敗型態正好是 transport error（`Client.Timeout exceeded`）。
3. **呼叫端 context budget 算錯**（`cmd/daily-replay-sync/main.go`）：寫死 `60*time.Second`，但 3 次嘗試 × 20s + 退避 3s + rate-limit 等待 10s = **73s**；即使有 retry，context 會先在 retry 迴圈中途過期。

## 4. 決策：timeout 維持 20s，改由有界 retry 吸收慢窗

| 選項 | 評估 | 決定 |
|---|---|---|
| 拉長 per-attempt timeout（20s → 40s+） | P95 僅 0.158s，放寬只讓**真心卡住**的請求多等一倍 | ❌ 不做 |
| **有界 retry + 指數退避**（3 attempts，1s/2s，上限 8s） | 沿用既有參數 `marketdata.max_retry_attempts`（3）/ `retry_backoff_ms`（1000），零新增參數；一次慢窗最多 73s 後放棄 | ✅ 採用 |
| 換來源（`openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL`） | 實測可用但**更慢**（2.8–3.4s vs 0.12–0.33s）且 schema 不同（319KB、中文 key），同為 TWSE 上游 | ❌ 不做（記錄為已評估） |
| 退化路徑 | 保留前次 CSV、不覆寫、channel 記為非 ok | ✅ 已實作/保留，並補「0 筆可用資料不得記 ok」 |

`twse_api_timeout_sec = 20` 保留的理由：20s ≈ **126 × P95**，對正常請求毫無風險；罕見慢窗由 retry 吸收，而不是讓所有請求的失敗延遲加倍。

## 5. 落地變更

- `internal/marketdata/retry.go`：`retryConfig` 新增 `retryTransportErrors`（opt-in，其他 provider 行為不變）與 `maxBackoff` 上限；抽出 `retryWait`。
- `internal/marketdata/twse_openapi.go`：`GetQuotes` 走 `fetchWithRetry`；新增 `twseRetryConfig()`、`retryPolicy()`、`SetRetryPolicyForTest`、`SetPerAttemptTimeoutForTest`、**`FetchBudget()`**（呼叫端據此設 context）。
- `cmd/daily-replay-sync/main.go`：context budget 由 `client.FetchBudget()` 推導；延遲以 `WithLatencyMs` 記錄；失敗訊息明寫「CSV 保持前次內容、該日留為 gap」；**新增 0 筆可用資料 → 記 `degraded` 並回錯**（原本會記 `ok`）。

## 6. 狀態語意（回答「warn 對不對」）

`twse_replay_sync` **沒有**在 `channelIDs()` 內（provenance=derived），因此沒有專屬 contract，走 `DefaultChannelContract`：`GraceFailures` 預設 **2**、freshness window 48h。
- 單次失敗 → derived **warn**（不 page）✅ 符合「transient 上游慢」
- 連續 2 次失敗 → **error**（維持告警能力）✅
- 任一次成功 → status=ok、`last_error` 清空、streak 歸零、`last_success_at` 前進 ✅（已加測試釘住）

因此**不需**為此 channel 新增 contract（新增會要求同步進 `channelIDs()`，屬另一個議題）；48h freshness 由 `channel_health_metrics_task` 的 staleness overage 路徑負責，與 status 脫鉤 → 持續斷軌仍會告警。

## 7. 驗收（本次）

- 生產：**未部署**（PR 不 merge）；上線後第一個 23:30 排程應記 ok 並清除 warn。
- 測試：`internal/marketdata`（retry/timeout/5xx/attempt 上限/退避上限/context 中止/FetchBudget）、`cmd/daily-replay-sync`（失敗保留 CSV + 非 ok 記錄、成功清 warn、0 筆 → degraded）、`internal/apigateway`（twse_replay_sync warn→ok、連續失敗→error）。
