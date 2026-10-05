---
title: channel 健康狀態單一真相規格（channel-health-status single truth）
status: active
updated: 2026-10-05
owner: apigateway / monitoring
related:
  - docs/reference/traps.md（channel 狀態只有一個判定）
  - docs/specs/dashboard-api-contract-spec.md
---

# channel 健康狀態單一真相規格

> **2026-09-29 更新（issue #2134）**：本規格的基準案例 `twse_oddlot` 已**退役**。
> - 上游 BFI84U 由 TWSE 移除（2026-08，改服務「得為融資融券有價證券停券預告表」；`MI_INDEX type=ODDLOT` 回空）⇒ 通道無可恢復性。
> - **抓取路徑移除**：`register_adapters.go` 不再註冊 adapter（`twse_oddlot` 與 dash alias `twse-oddlot` 兩者皆不再註冊），啟動時改寫入 `status="inactive"` + 退役原因。
> - **為何不能只移除註冊**：殘留 record 為 `degraded`，§2.2 規則 2b 會在資料齡超過契約窗口（48h）時升級為 `error`、gauge 變 `2` ⇒ 告警`ChannelHealthStatusError{channel="twse_oddlot"}` 永久 firing（實測 2026-09-29：record `degraded`、`last_success 2026-09-07`、gauge `2`）。`inactive` 是直通狀態、不升級、被 `Alerts()` 過濾、gauge 對映 `3`（無規則匹配）。
> - **消費端不受影響**：零售商零股失衡輸入（`a6_odd_lot`）改由 `twse_capital_flow` 代理（`monitoring.NewOddLotFetcher` → `oddLotFromCapitalFlow`，`-tanh(totalNet/30)`）；代理不可用時**回 error、不回 0**，A6 落到 `A6OddLotFallback=0.5`。
> - 同型同判準：`twse_etf`（TWT44U 移除、Fubon PCF 替代）維持未註冊／`inactive`，本次只對齊敘述，不動其實作。
> - 本規格的判定語意（§2）**未變**：退役只是讓某一條 record 不再走「degraded 過期 ⇒ error」的升級路徑，不是放寬規則、不是抑制告警。

> **2026-10-05 更新（前端分類缺陷；F58 的第二個消費者）**：三處修正在同一個 PR。
> 1. **前端跟上交易日契約**：`DeriveChannelStatus` 的規則 2b／3 原本各自再寫一次牆鐘比較
>    （`age > contract.EffectiveFreshnessWindow()`），而告警側（`ChannelDataStale` ⊆ `StalenessOverageSeconds`）
>    已於 #2201（F58）改成交易日感知 ⇒ 同一通道在週末有兩種判定。現在**兩個消費者共用同一個
>    `ChannelContract.StalenessOverageSeconds`**；未宣告 `PublishCalendar` 的通道語意逐字不變。
> 2. **退役成為契約事實**：新增 `ChannelContract.Retirement`（＋狀態 `retired`），宣告 `twse_oddlot`
>    （含 dash alias `twse-oddlot`）與 `twse_etf`。`DeriveChannelStatus` 的**規則 0** 在讀 record 之前
>    就回 `retired`，因此任何殘留 record（甚至 `error`）都無法讓它回到可告警的判定；gauge 對映 `3`。
>    `retired` 與 `inactive` **不同義**：前者是設計退休（不可逆、替代輸入已接線），後者是「現在被關掉」
>    （可逆，例如 operator opt-in 未開通，`tej`）。前端把 `retired` 移出「需關注」，理由降為資訊級。
> 3. **「需關注」三分類**：後端 `ClassifyChannelAttention`
>    （`internal/monitoring/service/channel_attention.go`）把非 ok 的通道分成
>    `system_error`（我們的問題）／`upstream_limit`（已知上游限制，配額・tier）／`expected_wait`（日曆未到），
>    隨 `/api/dashboard/data-channels` 的 `channels[].category` 與 `alerts[].category` 輸出；前端只負責標題與排版。
>    未宣告、無法歸因者一律算 `system_error`（寧可誤指自己，也不要靜默略過）。

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
2. **判定順序**（2026-10-05 起，`PublishCalendar` 感知）：
   0. `contract.Retirement != nil` → `retired`（契約事實優先於任何 record；見上）
   1. 無 record → `unknown`
   2. `rec.Status != "ok"` → 原值 pass-through（`warn`/`error`/`degraded`/`inactive` 已是抓取路徑寫入的判定）；
      `degraded` 例外：資料**逾約**（`StalenessOverageSeconds > 0`）時升級為 `error`（規則 2b）
   3. `Provenance == "derived"`（vix/us10y 等指標欄位鏡射）→ 原值（本身沒有抓取節奏，不套窗口）
   4. `ok` 且 `LastFetchAt` **逾約**（`contract.StalenessOverageSeconds(age, LastFetchAt, now) > 0`）→ `stale`。
      未宣告 `PublishCalendar` 時等價於「超過 `EffectiveFreshnessWindow()`（預設 `StaleDataThreshold=48h`）」；
      宣告 `tw_trading_day` 時改判「資料是否來自最新一個已過發布窗口（台北 18:00）的交易日」
   5. 時間戳空/不可解析 → 保留 `ok`（record 壞了，但不是「過期」，誤標會誤導 on-call）
3. **狀態語彙**：`ok` / `warn` / `error` / `degraded` / `inactive`（record 寫入）+ `stale` / `retired` / `unknown`
   （僅 derived 產生；`retired` 由契約宣告驅動）。
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
- 退役回歸（2026-09-29, #2134）：`internal/apigateway/register_adapters_retired_test.go`（未註冊 ＋ 兩個 ID 皆 `inactive` ＋ 導出值 `≠2` ＋ source-level 禁止任何生產路徑再探測該 ID）、`internal/monitoring/gateway_adapter_test.go`（代理唯一來源、不得回 0）、`cmd/atlas/channel_health_metrics_task_test.go`（修前 `degraded` 過期 ⇒ `2`、退役 ⇒ `3` 兩態）、`internal/monitoring/known_issues_test.go`（敘述必須寫明 RETIRED 與替代輸入）。
- 測試：
  - `internal/apigateway/channel_status_test.go`（判定表、`FetchOutcomeStatus`、DB mirror 值、PG 端到端）
  - `internal/apigateway/gateway_empty_payload_test.go`（空 payload → `degraded`；非交易日 → `ok`）
  - `internal/monitoring/service/channel_status_resolver_test.go`（17 天 `ok` → `stale`、`inactive`/`stale` record 不被丟棄）
  - `internal/monitoring/api/system/health_aggregate_test.go`（Tier 2 `stale` 桶）
  - `internal/monitoring/dashboard_api_test.go`（`/api/dashboard/channel-health` derived + 原因文字（走既有 `last_error`）+ known-issue 欄位保留）
  - `cmd/atlas/channel_health_metrics_task_test.go`（gauge 不得對過期 channel 輸出 0）
- 2026-10-05（前端分類缺陷；F58 第二個消費者）：
  - `internal/apigateway/channel_status_trading_day_test.go`（週末 SBL／gov 不再 stale、週一 18:30 起才 stale、
    未宣告通道語意逐字不變、2b 升級走同一契約、退役契約壓過任何 record）
  - `internal/monitoring/service/channel_attention_test.go`（① 週末分類、③ finmind／tdcc 為已知上游限制、
    預設為系統錯誤、② 退役列 = retired＋資訊級、`GetAlerts` 不含退役通道）
  - `shared_web/static/js/__tests__/datachannels-attention-categories.test.mjs`（前端三分類與退役列）


## 4. 永久損壞 channel 的治理判準：必須退役或修復（issue #2138）

### 4.1 為什麼要立這條

`twse_oddlot` 的告警疲乏不是意外：上游**永久**被移除、替代路徑**早就存在**，但因為「已知問題」只是 UI 標籤、
沒有任何機制強迫決策，`ChannelHealthStatusError` 就**連續 firing 60+ 天**（實測 2026-09-29：唯一 firing 的告警、
gauge=2）。#2134 修掉了那一條，但**沒有東西阻止下一條**。本節把「已知但未處理」變成機器可以把關的東西。

### 4.2 判準（三個條件必須同時成立）

| # | 條件 | 機械化欄位／來源 |
|---|---|---|
| 1 | 上游**永久**不可得，且有**第一方證據** | `KnownIssue.UpstreamRemovedAt`（非空＝宣告為可用性事件；**空字串是一個正面宣告**：「這不是可用性事件」，例如 canonical 健康的死 alias `taifex-daily`） |
| 2 | **替代路徑已存在** | `KnownIssue.ReplacementInput`（空＝沒有可退往之處 ⇒ 只能監控／修復，**不是**退役候選；例如 `bdi`：CNBC `.BADI` 無價且無可用替代） |
| 3 | `degraded`／`error` 且資料齡 > **N × 該 channel 自己的契約窗** | `GovernanceWindowMultiplier` ＋ `apigateway.DataAge`（**與 rule 2b 用同一個資料齡函式**，否則同一秒可能一邊說 error、一邊說不夠嚴重） |

**動作（二選一，且必須留痕）**：**退役**（依 #2134：寫 `status=inactive`＋原因、斷掉所有探測點、保留 channel id 與
known-issues 紀錄）或**修復**（接上新上游／新端點，並把 `DeriveChannelStatus` 的判定條件一併對齊）。
⛔ **禁止**以放寬規則／加抑制／延長契約窗口讓告警消失 —— 那是掩蓋；本判準存在的目的正是讓未處理的已知故障**持續可見**。

### 4.3 為什麼 N = 2 個契約窗（成本理由）

- **N=1 不能當判準**：`degraded` ＋ 資料齡 > **1** 個窗口正是 **E29-3 rule 2b 的升級點**本身（也就是「現在是 error」）。
  用它等於「一變成 error 就要求退役」⇒ 必然誤判**暫態**（上游當日未發布、schema 短暫變動）。
- **N=2** 只多要一個完整窗口的「沒有恢復」證據，同時把成本上界化：**每一個額外窗口都是告警持續 firing 的時間**。
  實證對照：本案燒了 **60 天**，N=2（預設 48h 窗 ⇒ ≈96h）把它壓到 4 天內要決議（≈15×）。
- **一律以「該 channel 自己的契約窗」為單位（不寫死天數）**：週頻通道（TDCC 8 天窗）⇒ ≈16 天、月頻自動更寬 ⇒
  慢速上游不會被誤判。**60 天是「沒人處理」的觀測值，不是目標**。
- 邊界是 **`>`（嚴格大於）**：恰好 N 個窗口**不算**；有測試釘住（`TestEvaluateChannelGovernance_WindowBoundary`）。

### 4.4 期限（`ActionBy`）與兩個介面

- `KnownIssue.ActionBy`＝**決議期限**；`RetiredAt`＝**已處置日期**（讓期限永久轉綠）。
- **CI（靜態、決定性）**：`cmd/check-channel-consistency` 對 registry 逐條檢查 ——
  可用性事件必須宣告處置（`RetiredAt` 或 `ActionBy`，缺一 ⇒ FAIL）；`ActionBy` 已過且無 `RetiredAt` ⇒ **FAIL**。
  - **為什麼是 FAIL 不是 WARN**：WARN 正是讓它靜默 60 天的機制；期限已過是**真事件**（date-driven，非程式改動造成），可接受且刻意。
  - **失敗訊息必須可行動**：指名 channel、`ActionBy`、兩個允許的動作，並明寫禁止的緩解手段。
  - **`now` 一律注入**（`--now=RFC3339`；判斷函式不得呼叫 `time.Now()`）⇒ 測試在任何日期都得到同一判決（無 date-bomb）。
  - **合法續期必須能變綠**：更新 `ActionBy` 並在 PR 說明重新檢視的結果即可 ⇒ 不得讓「已按程序續期」仍紅。
  - **不得稀釋總時長**：報告對**每一個**可用性事件都印 `UpstreamRemovedAt` 與「已 N 天」，連續續期無法藏住拖了多久。
- **執行期（動態）**：`cmd/atlas/channel_health_metrics_task.go` 對**宣告了永久移除**的 known-issue 通道輸出
  `atlas_channel_governance_overdue{channel}`（1＝判準成立）。series 由 registry 界定 ⇒ **有界**（非 known-issue 通道
  **不得**產生任何 series，有測試釘住）。
  - **不加任何 alert rule**：本 gauge **不取代** `ChannelHealthStatusError`（告警壓力仍由既有的 status gauge 承擔）。
    未來若要在它上面加規則，**必須先以真實資料校準門檻**（部署 #2134/#2136 後的實測基線為 **0 條 overdue**）。
  - **提醒是「一次、非持續」**：只在 `0→1` 轉態時，經 `monitoring.GovernanceNotifier` 印**一行警告 log**
    （實作是 `log.Printf`，格式 `[Gateway] channel_governance_overdue …`；本節刻意不寫成「WARN 級」，
    因為它不是 `slog` 級別而是非結構化警告行）—— 會重複的提醒只是第二個 paging 通道，正是本判準要消滅的疲乏。
    這一行**由出貨路徑直接印出**（`exportChannelHealthMetrics` → `governanceReminderLine` → `log.Print`）：
    同一份實作、同一份訊息、同一份測試，避免「測試綠、生產走另一條路」。
  - **行程重啟＝重新開始觀測**：`GovernanceNotifier` 的狀態是**純記憶體**（刻意如此），所以容器重啟後若該通道
    仍然 overdue，會**再印一次**——最多一次，不會變成每 tick 一行。重啟造成的一次重複是**可接受且刻意**的
    （寧可多一次，也不要為了去重而把狀態寫進磁碟、讓它與 record 的真實性脫鉤）。

### 4.5 與 rule 2b 的互動（明文）

`DeriveChannelStatus` 的語意**未改動**：`degraded` ＋ 資料齡超過契約窗仍會升級為 `error`，`ChannelHealthStatusError`
仍會 firing。本判準是**在其之上**多加一層「這件事必須被決議」的治理壓力；它**不**讓告警消失，**不**改變任何 verdict，
也**不**放寬任何窗口。兩者共用同一個資料齡函式（`apigateway.DataAge`），因此不可能出現「告警說 error、治理說不夠嚴重」的矛盾。

### 4.6 驗收

- `internal/monitoring/channel_governance_test.go`：決策表（`degraded`／`error`／`ok`／`inactive`、`degraded` 但有界）、
  **worked example**（`twse_oddlot` 生產形狀 ⇒ 應退役＋逾期的 22.6 天資料）、三個反向案例（暫態 ⇒ 不得退役；
  永久但**無替代**（`bdi`）⇒ 不得退役；死 alias（`taifex-daily`）⇒ 不適用）、**邊界**（恰好 N 窗 vs 超過 1 分鐘）、
  續期合法性（`ActionBy` 前後各一次斷言）、`DaysSinceDeclared` 反稀釋、`GovernanceNotifier`（首見發一次／連續不發／
  恢復後再發／nil 不發／通道互相獨立）。
- `cmd/check-channel-consistency/main_test.go`：registry 靜態契約為綠、**`now` 注入**下的日期閘門（期限前不紅、
  已處置永遠綠、**未續期的開放期限在未來必須紅**）、`bdi` 無替代但仍須有重評估期限。
- `cmd/atlas/channel_health_metrics_task_test.go`：gauge 對 pre-retirement 的 `twse_oddlot` 形狀為 `1`、
  **有界性**（`finmind` 無 known issue ⇒ 不得有 series；`taifex-daily` 宣告非可用性事件 ⇒ 不得有 series）、
  **一次性提醒**（首 tick 發、後續 3 tick 不發、nil notifier 不發）。
