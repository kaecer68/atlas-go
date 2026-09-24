---
title: channel 健康狀態單一真相規格（channel-health-status single truth）
status: active
updated: 2026-09-24
owner: apigateway / monitoring
related:
  - docs/reference/traps.md（channel 狀態只有一個判定）
  - docs/specs/dashboard-api-contract-spec.md
---

# channel 健康狀態單一真相規格

## 1. 問題（2026-09-24 生產實證）

同一個 channel 在同一秒出現多個互相矛盾的判定：

| 層 | `twse_oddlot` 的輸出 | 來源 |
|---|---|---|
| record（`channel_health.json`） | `status=ok last_fetch_at=2026-09-07T00:18:11Z` | `Gateway.Fetch` → `ChannelHealthStore.Record` |
| `/admin/datachannels`、`/api/dashboard/data-channels`、`data_get_channels` | `ok / 正常` | `resolveChannelStatusFromStore`（對任何 `ok` 一律回 `ok`） |
| health summary log、`Gateway.Summary()` | `stale` | `UnifiedHealthStore.deriveStatusWithContract`（有套契約窗口） |
| `/api/dashboard/channel-health`、atlas-mcp `channel_health` | `ok` | `DashboardAPI`（直讀 record） |
| `channel_health`（Postgres） | `status=ok last_fetch_at=<5 分鐘前>` | `recordToDB`（以 `time.Now()` 寫入） |

真實情況：TWSE BFI84U 已被改用途（2026-08，見 `internal/monitoring/known_issues.go`），最後一次成功抓取停在 2026-09-07，資料已 17 天未更新（413.7h > 48h 窗口）。

## 2. 契約（規範）

1. **單一判定函式**：`internal/apigateway/channel_status.go` 的
   `DeriveChannelStatus(rec *ChannelHealthRecord, contract ChannelContract, now time.Time) string`
   是 channel 狀態的唯一權威。任何呈現層、告警、DB mirror 都必須呼叫它或其 `ForID` 變體。
2. **判定順序**：
   1. 無 record → `unknown`
   2. `rec.Status != "ok"` → 原值 pass-through（`warn`/`error`/`degraded`/`inactive` 已是抓取路徑寫入的判定）
   3. `Provenance == "derived"`（vix/us10y 等指標欄位鏡射）→ 原值（本身沒有抓取節奏，不套窗口）
   4. `ok` 且 `LastFetchAt` 超過 `contract.EffectiveFreshnessWindow()`（預設 `StaleDataThreshold=48h`）→ `stale`
   5. 時間戳空/不可解析 → 保留 `ok`（record 壞了，但不是「過期」，誤標會誤導 on-call）
3. **狀態語彙**：`ok` / `warn` / `error` / `degraded` / `inactive`（record 寫入）+ `stale` / `unknown`（僅 derived 產生）。
   新增字串必須同步 `internal/monitoring/service/session.go` 的 `StatusText`（前端 label）與 `monitoring/rules/*.yml`。
4. **空/停用 payload 不得記成 `ok`**：adapter 回 `FetchResult{Stale:true}` 時，
   `FetchOutcomeStatus` 依契約 `DegradedOnEmpty` 決定：宣告者（`twse_oddlot`，上游已消失）記 `degraded` + 原因；
   未宣告者（`twse_margin`/`twse_capital_flow` 非交易日無新資料）維持 `ok`。
5. **DB mirror 語意**：
   - `status` = `DeriveChannelStatus` 的結果（與 UI 同一個字串）
   - `last_fetch_at` / `last_success_at` / `consecutive_failures` = record 的事實值（**禁止**寫入 sync 當下時間；`ChannelHealthSyncValuesFor`）
   - `updated_at` = 該列被寫入的時間
6. **API 呈現層不得新增未註冊的 JSON 欄位**：derived 判定的原因文字走既有 view 欄位（`/api/dashboard/data-channels` 與
   `/api/dashboard/channel-health` 的 `last_error`），因為 `scripts/ci/check_field_contract.sh` 以 `--strict` 在 CI 執行，
   JS 讀取任何沒有對應 Go json tag 的欄位都會 FAIL（handler 內的 local struct 不會被 `cmd/gentags` 掃到）。新增欄位前
   必須先把 response type 放進 `cmd/gentags` 掃描範圍（`internal/monitoring/api/**`、`internal/monitoring/service/**`）。
7. **known-issue 徽章不得因 derived 狀態而消失**：`twse_oddlot` / `twse_etf` 的上游移除事實照舊由
   `internal/monitoring/known_issues.go` 呈現；本規格只讓狀態誠實，不聲稱上游已恢復。

## 3. 驗收

- 同類掃描：所有 record × 契約窗口，`ok`-but-expired 的 channel 在各層都必須是 `stale`（修前只有 `twse_oddlot`）。
- 測試：
  - `internal/apigateway/channel_status_test.go`（判定表、`FetchOutcomeStatus`、DB mirror 值、PG 端到端）
  - `internal/apigateway/gateway_empty_payload_test.go`（空 payload → `degraded`；非交易日 → `ok`）
  - `internal/monitoring/service/channel_status_resolver_test.go`（17 天 `ok` → `stale`、`inactive`/`stale` record 不被丟棄）
  - `internal/monitoring/api/system/health_aggregate_test.go`（Tier 2 `stale` 桶）
  - `internal/monitoring/dashboard_api_test.go`（`/api/dashboard/channel-health` derived + 原因文字（走既有 `last_error`）+ known-issue 欄位保留）
  - `cmd/atlas/channel_health_metrics_task_test.go`（gauge 不得對過期 channel 輸出 0）
