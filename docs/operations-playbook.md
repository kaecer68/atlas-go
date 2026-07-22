# Operations Playbook

## Purpose

This document explains how to operate `atlas-go` correctly day to day.

## Operating Modes

### 1. Single Session Replay

Use when:

- validating one replay date
- checking agent output shape
- verifying ledger output
- testing one importer or one prompt adjustment

Core command:

```bash
go run ./cmd/atlas
```

### 2. Replay Import

Use when:

- normalizing TWSE or TPEX open-data files
- preparing replay-ready datasets
- moving from raw CSV into internal JSONL

Core command:

```bash
go run ./cmd/import-replay -source samples/replay/twse_stock_day_all_sample.csv -target data/replay/tw_open_data.jsonl
```

### 3. Window Backtest

Use when:

- evaluating agent behavior over a period
- choosing the weakest agent
- generating mutation candidates

Core command:

```bash
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

## Standard Operating Procedure

1. Verify the agent registry path and replay data path.
2. Confirm the replay date or backtest window.
3. Run import if the source data is still raw.
4. Run a replay session or window backtest.
5. Inspect:
   - session summary
   - outcomes
   - experiments
   - weakest-agent output
6. Decide whether the result is exploratory or ready for mutation design.

For mutation runs, prefer explicit mode selection:

- isolated validation mode: `--no-fallback --no-auto-pivot`
- guarded throughput mode: default auto-pivot with `--min-sample-for-rank <n>`

## Dashboard Operations

The Unified Control Tower has been split into two SPAs: `admin_web/` (operator) and `client_web/` (investor). Use them for real-time monitoring, approval, and intervention.

### Entry points

- Admin: `http://localhost:18080/admin/` after starting `go run ./cmd/atlas -api`.
- Investor (default landing): `http://localhost:18080/` redirects to `/client/`.

### Page workflow (decision logic)

Operate the dashboard left-to-right in normal conditions:

1. **總覽** -- 快速掌握系統健康度
2. **宏觀敘事** -- 理解驅動今日 regime 的宏觀故事
3. **相對趨勢** -- 確認熔斷機制、投資組合損益與總經雷達
4. **投資管線** -- 審查個別推薦並執行人為過濾
5. **AI 觀測台** -- 檢視最弱 agent 成績卡與標的池重疊
6. **模擬交易** -- 評判結果、比對 diff、晉升已通過的候選
7. **最新回測** -- 閱讀最近一次回測窗口的完整報告與下載連結
8. **控制與稽核** -- 執行覆寫、管理基線版本、閱讀稽核紀錄

### 總覽快速判讀

本頁是基於最新回測窗口計算出的**系統狀態快照**，不是即時行情，而是 Atlas 在指定 replay 資料區間內，透過多層 AI Agent 模擬與風控後匯總出的「當前體制判斷」。

7 張 KPI cards 依 top-down 順序排列：

- **資料時間** -- 回測資料的最新日期與窗口生成時間，放在首列以宣告「這組數據所對應的時間線」。
- **基線版本** -- 現行生效的 baseline policy 版本號，說明當前統計是基於哪一版政策運算出來的。
- **敘事脈絡** -- 當前最活躍的宏觀事件主題、外資出逃指數分數/等級，以及前往宏觀敘事頁籤的捷徑。宏觀脈絡決定所有下游的倉位規模與過濾條件。
- **市場狀態** -- 當前市場 regime（RISK_ON／NEUTRAL／RISK_OFF）。 regime 文字本身以色呈現：綠色代表風險偏好、積極配置（RISK_ON），黃色代表中性謹慎（NEUTRAL，倉位上限 85%），紅色代表風險趨避、建議降低曝險（RISK_OFF）。點擊卡片可查看三種體制的詳細解讀與操作建議。
- **最弱 Agent** -- 下一個突變候選人及其 Sharpe-like 指標。
- **實驗狀態** -- 待評判 / 待晉升的數量。
- **擁擠標的** -- 當 >=3 個 agents 重疊在同一標的，或 style-layer 標的池重疊過高時標示。

點擊任何一張分析卡片（除資料時間與基線版本外）會彈出說明視窗，解釋該指標的背後意義、對投資決策的影響，以及下一步該注意什麼。

### 宏觀敘事頁籤

在決定倉位規模或產業配置前閱讀：

- **總經快照** -- DXY-美元指數、US10Y-美債10年期、VIX-波動率指數、USD/TWD-匯率、原油、黃金、日圓，以及三大法人資金流數據。面板頂部有資料通道健康度燈號（🟢 正常 / 🟡 延遲 / 🔴 缺失），三大法人資金流也有自己的獨立燈號與更新時間，用於即時判斷宏觀數據採集是否及時。
- **外資出逃指數** -- 0–100 分，由 6 大子項加權組成，反映外資撤離台灣市場的壓力程度：DXY-美元指數（15%）、US10Y-美債10年期（20%）、外資流向（25%）、VIX-波動率指數（15%）、日圓-套利平倉壓力（10%）、地緣政治風險（15%）。等級標示為低壓 / 警戒 / 高壓 / 危機。在高壓 / 危機 regime 下，可考慮降低曝險或收緊控制層過濾條件。
- **敘事事件 / 因果傳導鏈 / 投資模型** -- 解釋當前 regime 為何被如此評分，以及哪些產業被看好或應迴避。因果傳導鏈中的主題 ID 與名稱均已本地化為中文，每個步驟都會標示影響的板塊標籤；負數影響力以紅字呈現。

### 相對趨勢頁籤

- 頁面頂端有控制層處置結果的解讀說明，強調這一頁呈現的是「風控長／投資長對 AI 推薦的最終處置結果」。
- **總經敘事脈絡橫幅**（雷達上方）重複顯示壓力分數、主要事件、情緒方向與壓力等級建議，並根據當前 narrative model 自動列出看多/看空板塊，讓你在監控即時執行時不需要來回切換到宏觀敘事頁籤。
- 總經雷達顯示最新回測場次的 regime、guard 處置紀錄（放行/過濾/阻擋），以及最終放行標的的前 5 檔清單（含公司名稱、信念、遠期報酬）。
- 即時狀態欄寬縮小為 170px；若系統處於 Simulation 模式，會顯示「目前以 replay 資料進行回測模擬，未連接 live broker」的說明，而非空白「無資料」。

### 投資管線頁籤

本頁呈現的是**最新回測場次中，經過控制層審核後的推薦標的清單**。這不是即時行情，而是 Atlas 在指定 replay 區間內模擬與風控後的結果。

#### 預設視圖與完整視圖

- **預設**：僅顯示 `passed_guards=true`（控制層已放行）的推薦。
- **顯示全部被過濾項目**：勾選後會額外出現紅色邊框的列，這些是被 CRO 或 CIO 擋下的推薦。

#### 欄位說明

表格包含標的、公司名稱（由前端 `STOCK_NAME_MAP` 映射 24 檔常見台股）、策略來源（Agent + Skill）、來源層、方向、收盤價、目標價、停損價、信念、隔日回測報酬、價量標籤與推薦理由。

- **信念**與**隔日回測報酬**欄位標題旁有 ℹ️，點擊可查看數值意義。
- **擁擠標籤**（`warn`）當原因包含 `[crowded:N agents]` 時出現，表示 CIO 層已對該標的套用信念懲罰。

#### 人工覆寫操作（可選）

投資管線提供三種按鈕，讓操作者對 AI 推薦進行單筆覆寫。這些動作會寫入控制稽核紀錄，並在後續回測執行時生效。**不進行任何操作不會造成錯誤。**

| 按鈕 | 出現條件 | 語義與後續影響 |
|------|----------|----------------|
| **放行** | 僅對 `passed_guards=true` 的列顯示 | 人工背書此推薦，後續回測不會將它濾除。 |
| **否決** | 僅對 `passed_guards=true` 的列顯示 | 人工拒絕此推薦，後續回測會強制排除該 (標的, Agent) 組合。 |
| **補追** | 僅對 `passed_guards=false`（被過濾）的列顯示 | 語義同「放行」——對已被控制層擋下的標的進行人工強制納入。 |

**使用時機舉例**：
- **放行**：某推薦被 CIO 因信念稍低而擋下，但你基於外部消息（如新聞、財報）認為應該進場。
- **否決**：某 Agent 推薦了你投資紀律中絕不碰的標的，或出現系統尚未反應的負面事件。
- **補追**：某標的被風控長過濾，但你判斷該過濾條件在當下過於保守，決定強制納入。

這三個按鈕本質上是同一套人工覆寫機制的三種語境表現；預設情況下（不做任何點擊），系統完全依照控制層自動規則運行。

### AI 觀測台頁籤

- Agent 觀測台：完整的成績卡表格（窗口數、命中率、Sharpe、最大回撤、Darwinian 權重）。
- 標的池重疊：顯示每個 agent 的標的池與 style-layer 重疊矩陣。重疊 >=3 檔標的者以黃色高亮。

### 模擬交易頁籤

- **評判** 直接從收件匣對待評判實驗進行評判。
- **差異** 開啟並排的基線與候選 prompt 比對。
- **晉升** 將已接受的實驗移入現行基線政策（寫入 `data/state/baseline_policy.json`）。
- 晉升歷史顯示過去接受/拒絕的紀錄與版本號。

### 最新回測頁籤

- 顯示最近一次回測窗口的詳細報告、績效摘要與 Markdown 下載連結。

### 控制與稽核頁籤

- **資料採集健康度** -- 集中監控 10 項宏觀與資金流資料來源的即時狀態（DXY、US10Y、VIX、匯率、原油、黃金、日圓、三大法人）。綠燈表示 24 小時內有更新，黃燈表示延遲，紅燈表示缺失。大面積紅燈時應檢查網路連線或資料供應商（Yahoo Finance / TWSE）狀態。
- **Agent 覆寫** -- 暫停 / 恢復個別 agents。
- **產業封鎖** -- 封鎖或解除產業（例如 `半導體`）。
- **基線管理** -- 從下拉選單晉升實驗，或附帶原因回滾到先前版本。
- **人工干預紀錄** -- 完整的批准、拒絕、暫停、封鎖與回滾稽核軌跡。

---

## Wave 9 觀測性運維

Wave 9 在 live mode 啟動時會載入 `internal/monitoring/wave9_runtime.go` 的 `Wave9Observability`，協調 5 個偵測器。日常運維時請注意以下事項。

### 確認偵測器已啟動

啟動日誌應出現：

```
started component=wave9_observability
```

停止時應出現：

```
wave9_observability stopped
```

若只有 `started` 而無對應 `stopped`（例如程序異常退出），請檢查 goroutine 洩漏或 `Stop()` 是否被呼叫。

### 5 個 Wave 9 事件與建議回應

| 事件 | 意義 | 建議回應 |
|------|------|---------|
| `monitor.channel.health.individual` | 單一資料通道錯誤率或延遲異常 | 檢查該 channel 對應的資料供應商狀態；與資料採集健康度頁面交叉比對 |
| `market.regime.confirmed` | 新 regime 已穩定 30 秒 | 觀察後續 `portfolio.factor.regression` 與 `portfolio.drift.detected` 是否觸發 |
| `portfolio.factor.regression` | regime 變化後 factor weight 位移超過閾值 | 檢視投資組合權重是否需隨 regime 調整；留意策略權重漂移 |
| `portfolio.drift.detected` | 單一持倉集中度 > 25%、turnover > 15% 或 target drift > 10% | 評估是否人工 rebalance；若 `BaselineTrigger` 已啟用，檢查 structured log 中的 `baseline_violation` |
| `apigateway.ingestion.lag.spike` | API Gateway p99 latency > 5s | 檢查網路延遲、資料供應商回應速度、背景任務佇列長度 |

### 外部相依設定

Wave 9 的生產 provider（`ChannelHealthProvider`、`IngestionLagProvider`、`WeightProvider`、`TargetWeightsProvider`）需要正確的環境與外部服務。詳見 `docs/environment.md`。

## Wave 9 觀測性 v0.0.0.18 修復與運維指引

v0.0.0.18（PR #704，`feat/wave-10-l1-l2-iteration`）修了 v0.0.0.17 Wave 9 observability wire 的三個 production bug 與一個測試覆蓋缺口。以下是運維時需要注意的場景：

### 1. SSE catchup 在 `runLiveTrading` 是空的

**症狀**：Dashboard 重新連線後看不到即時事件（空白 timeline），但 `run()`（模擬模式）正常。

**根因**：v0.0.0.17 的 15 組 buffer 訂閱只註冊在 `dashEventBus`（模擬用 bus），但 `runLiveTrading` 的 Wave 9 偵測器 publish 到 `eventBus`（另一個獨立 bus 實例），所以 live mode 的 SSE catchup buffer 永遠是空的。

**v0.0.0.18 修法**：批次訂閱 helper `apievents.RegisterDashboardBufferSubs(bus)`（`internal/monitoring/api/events/sse_handler_subscriptions.go`）現在接受 `eventbus.EventBus` 介面。`cmd/atlas/main.go` 的 `run()` 與 `runLiveTrading()` 都會各自呼叫 `RegisterDashboardBufferSubs` 並傳入當前使用的 bus（`dashEventBus` / `eventBus`），確保 live mode SSE catchup 正常。

**運維檢查**：在 live mode 啟動 log 中搜尋 live bus 上的 buffer 訂閱確認。若 SSE 仍為空：確認 `runLiveTrading` 呼叫了 `RegisterDashboardBufferSubs(eventBus)`。

### 2. `risk.NewAuditSubscriber` 重複登錄導致 JSONL 重複

**症狀**：`data/audit/` 目錄下的 JSONL 檔案中每筆風險事件都出現重複記錄。

**根因**：v0.0.0.17 的 `risk.NewAuditSubscriber(bus)` 每次呼叫都會建立新的 subscriber 並註冊到 bus，沒有檢查是否已存在。`cmd/atlas/main.go` 如果在同一 bus 實例上呼叫多次（如 live mode 初始化與 dashboard 初始化路徑重疊），就會產生重複的事件記錄。

**v0.0.0.18 修法**：`risk.NewAuditSubscriber` 現在是 **idempotent**（等冪 — 多次呼叫同一 bus 實例只會建立一個 subscriber）。內部使用 process-wide registry（`var auditSubscribers = make(map[*eventbus.ChannelEventBus]*AuditSubscriber)`）keyed by bus pointer，後續呼叫回傳已存在的 subscriber。

**運維檢查**：檢查 `data/audit/` 目錄的 JSONL 檔案中 `recorded_at` timestamp 是否唯一。`cmd/atlas/main.go` 的 `run()` 與 `runLiveTrading()` 各自呼叫 `risk.NewAuditSubscriber` 在不同的 bus 上是安全的（兩個 bus 實例不同，各自有一個 subscriber）。

### 3. `Wave9Observability.Start` 部分失敗時 goroutine 洩漏

**症狀**：平行啟動 3 個 detector（DriftDetector / ChannelHealthSynthesizer / IngestionLagMonitor）時某一個失敗，log 顯示 error 但其他兩個仍在執行，且 retry Start 時報錯。

**根因**：v0.0.0.17 的 `Start` 在平行啟動失敗後沒有 cleanup 已成功啟動的 detector，也沒有清空內部 field references。retry 時會嘗試啟動已在執行的 detector 實例。

**v0.0.0.18 修法**：`Start` 現在使用 `defer` cleanup，以 LIFO 順序 `Stop()` 已成功啟動的 detector，並把 `w.started = false` 與所有 detector 欄位設為 nil（retry 拿到 fresh instances）。`errs` channel 改用 `errors.Join` 聚合所有平行啟動失敗（不再只回傳第一個）；cleanup 過程中的 `Stop()` 失敗同樣以 `errors.Join` 摺入最終回傳的 error。呼叫端可從 error chain 區分「partial-failure 但 cleanup 乾淨」與「partial-failure 且 leaked subscriptions」。

**運維檢查**：若 `started component=wave9_observability` 出現後隨即出現 error log，檢查 error chain 中是否包含 `errors.Join` 的多個 error。cleanup 乾淨的情況下 retry `Start` 應能成功。


## 啟動 readiness 與 startup deadline v0.0.0.23 運維指引

v0.0.0.23（PR #886，`fix/review-fixes-round1`）新增 `GET /ready` readiness 端點，並在 server goroutine 加上 10 秒 startup deadline。同時把 SSE 相容性所需的 `WriteTimeout` 從 10s 放寬到 30s。運維在解讀 healthcheck 或啟動耗時時需注意以下場景：

### 1. `/health` 與 `/ready` 的差別

**症狀**：Docker / K8s 把 `/health` 與 `/ready` 都當作 liveness probe，但同一個容器可能 `/health=200` 且 `/ready=503`。

**根因**：兩個端點檢查不同面向：
- `GET /health`（`cmd/atlas/api_routes.go:91`）：只檢查 atlas_http / fubon_proxy port 是否有在 listen（liveness）
- `GET /ready`（`cmd/atlas/api_routes.go:92`）：檢查 Postgres 連線、replay data 檔案存在、Gateway channel 數量（readiness）

**v0.0.0.23 設計意圖**：Docker healthcheck 用 `/health` 做 liveness（短輪詢，失敗 = kill container），用 `/ready` 做 readiness（長輪詢，失敗 = 暫時不送流量但容器繼續跑）。兩者皆已加入 `isPublicPath` 白名單，不需要 API key 即可訪問。

**運維檢查**：K8s deployment 應分開設定兩種 probe：
```yaml
livenessProbe:
  httpGet: { path: /health, port: 18080 }
  initialDelaySeconds: 10
  periodSeconds: 30
readinessProbe:
  httpGet: { path: /ready, port: 18080 }
  initialDelaySeconds: 30
  periodSeconds: 10
```

`/ready` 回 503 時，response body 的 `checks` map 會列出失敗原因（例如 `"postgres": "failed: connection refused"`），可用於 alerting 判斷哪個 dependency 出問題。

### 2. 啟動時固定 10 秒延遲

**症狀**：`run_api` 或 `runLiveTrading` 啟動後 log 顯示 `server_startup_ok component=main addr=:18080` 總是在啟動後約 10 秒才出現，期間無任何 progress log。

**根因**：v0.0.0.23 引入 server goroutine 兩階段 lifecycle：
1. **Startup window（10s）**：goroutine 啟動後主流程等待 `time.NewTimer(10s)` 或 `srvErr` / signal / `deps.shutdown` 任一觸發
2. **Running phase**：進入 graceful shutdown select loop

第一階段 timer 會**固定等到 10 秒**才記錄 `server_startup_ok`，因為 listener 的 first `Accept` 發生在 goroutine 內部（`deps.listenAndServe(srv)`），主流程無法從外部感知「伺服器已開始 accept 連線」的瞬間。

**這不是 bug**：是 trade-off。10s 是保守的安全預設 — 若 `portprobe.Listen` 或 `srv.Serve` 阻塞（DNS 解析、kernel backlog 飽和），程序會在 10 秒內 noise-exit，而非永久 hang。

**運維檢查**：若 startup 耗時成為性能問題（例如 K8s rolling deploy 太慢），可考慮：
- 短期：把 `cmd/atlas/main.go:1422` 的 `10 * time.Second` 提取為 `time.Duration` flag（CLI 參數）
- 長期：改為 event-driven — listener first Accept 透過 channel 通知主流程（需先包裝 `net.Listener` 為 `trackingListener`）

### 3. `WriteTimeout: 30s` 對 SSE 的影響

**症狀**：`internal/monitoring/api/events/sse_handler.go` 的 SSE long-lived 連線在 dashboard 長時間停留時偶爾被切斷。

**根因**：v0.0.0.23 之前 api 模式 `http.Server` 只設 `ReadHeaderTimeout: 10s`，`WriteTimeout` 預設為 0（無限）。v0.0.0.23 為了 slowloris 防護補上 `WriteTimeout: 30s`（覆蓋 api 與 live 模式）。SSE handler 共用同一個 `http.Server`，30s 對 SSE 心跳（典型 15-25s）足夠緩衝。

**運維檢查**：若 SSE 仍被切斷，確認 client 端 heartbeat interval < 25s。若 client 端無法控制（如瀏覽器原生 EventSource），可考慮把 SSE path 改用獨立 `http.Server` with separate timeout config（後續 PR）。

### 4. Live mode `/ready` 的 `gateway_channels: skipped`

**症狀**：live mode 啟動後 `GET /ready` 回 `checks.gateway_channels = "skipped (mode does not initialize Gateway)"`，status 仍是 `ready`。

**根因**：v0.0.0.23 新增 `readyChecker.skipGateway` 欄位（`cmd/atlas/api_routes.go:130` 附近）。live mode（`runLiveTrading`）不初始化 `apigateway.Gateway`，所以 `gatewayChan=0` 是預期行為，不應該 fail readiness。`runLiveTrading()` 在 `cmd/atlas/main.go:1747` 設 `skipGateway: true`。

**運維檢查**：這是預期行為，不需要處理。若看到 api mode 也回 `skipped`，則是 bug — `run()` 應該永遠不設 `skipGateway`。

---


## Operator Techniques

### Start small

Use one replay date first. Confirm the ledger and summary artifacts before running wider windows.

### Keep raw and normalized data separate

- raw files: source dumps
- normalized JSONL: replay-ready internal format

This keeps importer bugs from polluting analysis.

### Treat session artifacts as evidence

The files in `data/state/sessions/<session-id>/` are not logs for decoration. They are the evidence trail for why a future prompt mutation was justified.

### Respect sample-size limits

If only one or two sessions exist, use the result for orientation, not for strong model claims.

When running `today-start`, sample size also affects mutation-type ranking:

- mutation types with insufficient historical sample count are excluded from weighted ranking
- raise `--min-sample-for-rank` when you prefer conservative switching
- lower it only for exploratory search, and treat outcomes as low-confidence

### Understand guard outcomes

`today-start` can skip or switch before execution:

- `Primary mutation marked futile...`: recent runs for that mutation type are all non-improving in the same window
- `Primary cycle skipped due to futility guard...`: skip path (usually with `--no-auto-pivot`)
- `[pivot] Switching primary mutation type...`: auto-pivot picked an alternative using weighted ranking

Interpret these as control signals, not errors.

### Read the weakest-agent result with context

The weakest agent is a candidate for investigation, not automatic proof that the prompt is bad. Check:

- number of observations
- regime context
- concentration of failures
- data completeness
- required skills and forbidden actions from the registry policy

## Artifacts Checklist

For a healthy run, expect:

- `recommendation_outcomes.jsonl`
- `experiments.jsonl`
- `summary.json`
- window summary when a backtest window is run

## Failure Handling

If a run looks wrong, inspect in this order:

1. replay source path
2. session date and forward-return availability
3. registry load path
4. outcome file contents
5. weakest-agent selection logic

If mutation flow behaves unexpectedly, also inspect:

6. futility guard status in `scripts/openclaw/today_start.sh`
7. `--min-sample-for-rank` value and candidate sample counts (`n` in pivot logs)

## Baseline Promotion

After an experiment is accepted:

1. Promote the accepted result into the baseline policy store.
2. Confirm the baseline policy version and promotion history changed.
3. Re-run replay or backtest commands so the next cycle uses the promoted baseline.

This keeps runtime execution, replay compare, and future mutations aligned to the same formal baseline.

## Human Approval Workflow

Use the human-in-the-loop wrapper as the default decision entrypoint for promote/reject/revert.

### Decision Entry

```bash
# approve and promote
./scripts/openclaw/human_approval.sh --approve --experiment <exp-id> --reason "Passes replay and guard gates"

# reject (audit-only)
./scripts/openclaw/human_approval.sh --reject --experiment <exp-id> --reason "Insufficient improvement evidence"

# revert baseline
./scripts/openclaw/human_approval.sh --revert --reason "Rollback after post-promotion alert"
```

### Audit Artifact Check

Each decision writes one event file under `data/state/approvals/`.
Validate that the event contains required fields:

- `decision_id`
- `timestamp`
- `actor`
- `action`
- `reason`
- `dry_run`

### Event Replay (Dry-Run First)

Use approval event replay to verify the decision can be reconstructed from audit artifacts:

```bash
# replay one stored approval/reject/revert event without state mutation
./scripts/openclaw/replay_approval_event.sh --event data/state/approvals/<decision-file>.json --dry-run
```

### One-Command Verification

Run the dedicated checker when changing decision scripts or event schema:

```bash
./scripts/openclaw/verify_human_approval_event.sh
```

This verifies:

- event JSON schema fields are present and correctly typed
- event file persistence matches emitted decision payload
- replay wrapper can reconstruct and execute a dry-run decision from stored event

### CI Gate Requirement

Governance and operations verifiers are enforced in CI as dedicated jobs in `.github/workflows/ci.yml`:

- workflow: `ci`
- job: `governance`
- job: `operations`

For branch protection, require both status checks `ci / governance` and `ci / operations` so promote/reject/revert logic, replay determinism, M5 scenario verification, and M8 operations drills cannot regress silently.

### Branch Protection Setup (GitHub)

Preferred path (automation + guided approval):

```bash
# default: dry-run, show current config, options, and risk notes
./scripts/openclaw/setup_branch_protection.sh

# apply after reviewing prompts and confirmation phrase
./scripts/openclaw/setup_branch_protection.sh --apply
```

The setup script includes anti-misconfiguration checks:

- always starts in dry-run mode
- shows current protection config before proposing changes
- explains option-level trade-offs and risk consequences
- requires explicit final confirmation before apply
- creates a pre-apply snapshot under `data/state/branch-protection-snapshots/`

Optional snapshot location override:

```bash
./scripts/openclaw/setup_branch_protection.sh --apply --backup-dir data/state/custom-branch-protection-backups
```

Restore from a previous snapshot:

```bash
# preview restore payload and risk notes (dry-run)
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json

# apply restore (requires explicit confirmation phrase)
./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<snapshot>.json --apply
```

Restore mode anti-misconfiguration checks:

- snapshot file must exist and include `owner/repo/branch`
- snapshot target must match current repository and branch
- snapshot must contain a valid `protection` object
- restore mode still requires explicit human confirmation before apply

Recommended repository setting path:

1. GitHub repository -> Settings -> Branches
2. Add or edit branch protection rule for `main`
3. Enable `Require status checks to pass before merging`
4. Select required checks:
   - `ci / governance`
   - `ci / operations`
5. Optional but recommended:
   - Enable `Require branches to be up to date before merging`
   - Enable `Require conversation resolution before merging`
6. Save rule and verify by opening a test PR

Quick verification checklist after saving:

- PR Checks tab shows both `ci / governance` and `ci / operations`
- Merge button stays blocked until both checks pass
- Failed operations or governance jobs block merge as expected

The CI governance job runs strict mode by default:

```bash
./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
```

Use this strict mode after scenario design is calibrated for your replay window.

## Operations Gate (M8)

Use the operations gate verifier for staging-safe production-readiness checks:

```bash
./scripts/openclaw/verify_operations_gate.sh
```

What it checks:

- runbook command coverage for rollback and replay workflow
- Prometheus config sanity for atlas metrics scraping
- dry-run rollback drill via human approval event + replay
- human approval event schema/replay contract

Optional deep mode:

```bash
./scripts/openclaw/verify_operations_gate.sh --with-governance
```

Use `--with-governance` when you want to chain M8 checks with strict governance verification in one run.

## Rollback and Replay Workflow

### Revert Decision

To record a revert decision for an experiment:

```bash
./scripts/openclaw/human_approval.sh --revert <experiment-id> --reason "performance regression"
```

### Approval Event Replay

To replay an approval event for testing:

```bash
./scripts/openclaw/replay_approval_event.sh --event <event-file>
```

### Approval Event Verification

To verify approval event schema and replay contract:

```bash
./scripts/openclaw/verify_human_approval_event.sh
```

### Strict Governance Gate

To run strict governance verification with scenario diversity:

```bash
./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
```

---

## Port 18080 Conflict Recovery

atlas 啟動時撞到 host port 18080 已被佔用,是最常見的本機與 Docker 啟動失敗情境之一。本節列出症狀辨識、兇手定位、修法選擇與驗證清單,涵蓋本機 binary 與 Docker 雙路徑。

### 1. Symptom

以下任一現象出現,即可判定為 port 18080 衝突:

- atlas 啟動 log 出現 `bind: address already in use` 後退出。
- `docker compose up` 啟動 atlas container 後反覆 restart loop,`docker compose ps` 顯示 atlas 狀態在 `restarting` 與 `exited` 之間反覆切換。
- `curl -fsS http://localhost:18080/health` 連線失敗(connection refused 或 timeout)。
- `docker compose logs atlas` 出現 atlas 抱怨 18080 bind 失敗的 stack trace。

### 2. Detection

先找出佔住 18080 的行程 PID,再判斷它的身分。

```bash
# macOS / Linux:列出 LISTEN 中的 18080
lsof -i :18080

# 或更精準,只列 LISTEN 狀態
sudo lsof -nP -iTCP:18080 -sTCP:LISTEN
```

拿到 PID 後,確認是哪個 process:

```bash
ps -p <PID> -o pid,command
```

根據 `command` 欄位判斷身分:

- 含 `atlas` 或 `cmd/atlas` → atlas zombie(上一次啟動沒清乾淨)。
- 含 `fubon` 或 `fubonproxy` → fubon-proxy(別直接殺,見 §3 Decision Tree 分支 B)。
- 含 `node`、`python`、`webpack`、`vite`、`next dev` 等 → 開發伺服器。
- 其他 → foreign process,可能是 IDE preview、舊版應用或背景工具。

### 3. Decision Tree

依兇手身分決定處置方式。三條分支互斥,選一條執行。

#### 3.1 atlas zombie

代表上次啟動沒正常結束,18080 仍被舊行程咬住。

```bash
# 優雅終止(預期 SIGTERM 會讓 graceful shutdown 跑完)
kill <PID>

# 若 SIGTERM 無效(行程沒反應或已死鎖),改用 SIGKILL
kill -9 <PID>

# 然後重啟 atlas
go run ./cmd/atlas
```

驗證 `lsof -i :18080` 應回空。

#### 3.2 fubon-proxy

fubon-proxy 是另一個獨立服務,啟動時若與 atlas 撞 18080 也不奇怪。重點是:**不要直接 `kill`**,可能導致它的 supervision 狀態損壞或留孤兒行程。

正確處置(三選一):

1. **用 fubon-supervisor graceful stop**:若 supervisor 已啟動,呼叫它的 stop API 或 CLI,讓它走完整 shutdown 流程,18080 釋出後再啟動 atlas。
2. **改 fubon-proxy 啟動 port**:若環境支援,暫時把 fubon-proxy 綁到 8082(`-addr :8082` 之類的 flag,視 fubonproxy 套件啟動方式而定),讓 atlas 拿回 18080。
3. **重啟 atlas container 等 supervisor 重試**:若 atlas 是 Docker 啟動而 fubon-proxy 是 host process,先關 fubon-proxy 後 `docker compose restart atlas`,atlas 啟動時若仍撞 18080 會在 supervisor 的 backoff 後重試,等到 18080 釋出就成功。

#### 3.3 foreign process

不是 atlas 也不是 fubon-proxy,是其他開發工具或舊 process 佔住 18080。兩個選擇:

**c1. 停掉那個 process**(若你知道它是什麼且可停):

```bash
kill <PID>      # 優雅終止
kill -9 <PID>   # 必要時強制
```

**c2. 改 atlas 啟動 port**(若不適合停 foreign process):

本機 binary:

```bash
go run ./cmd/atlas -addr :8082
```

Docker compose:編輯 `docker-compose.yml`,把 atlas service 的 host port 從 `18080:18080` 改成 `8082:18080`:

```yaml
services:
  atlas:
    ports:
      - "8082:18080"
```

改完後 `docker compose up -d atlas`,驗證時用 `curl http://localhost:8082/health`。

### 4. Docker Mode Caveat

container 內的 `localhost:18080` 永遠是空閒的(每個 container 有自己的 loopback 介面)。以下情境很常誤導:

> `docker compose up` atlas container 顯示啟動成功,`docker compose ps` atlas 是 `Up`,但 host 端 `curl localhost:18080/health` 失敗。

這不是 atlas container 內部 port 被佔,而是 **host port 18080** 已經被另一個 process 咬住,Docker daemon 無法把 host 18080 轉發到 container 18080。Docker log 不會報錯,因為 container 內 bind 的是 container 自己的 18080,完全沒問題。

修法是操作 host 上的舊容器或 process:

```bash
# 先看哪個舊容器還掛著 18080
docker ps --filter expose=18080

# 停掉所有 atlas 相關舊容器
docker compose down

# 或強制刪除所有 expose 18080 的容器
docker rm -f $(docker ps -aq --filter expose=18080)

# 然後重新啟動
docker compose up -d atlas
```

### 5. Recovery Steps

依執行路徑選擇對應的精準命令。

#### 5.1 本機 binary

```bash
# 一行殺光所有 LISTEN 18080 的行程(謹慎使用,確認沒有重要 process)
lsof -ti :18080 | xargs kill -9

# 重啟 atlas
go run ./cmd/atlas
```

#### 5.2 Docker

```bash
# 完整重啟 atlas 服務
docker compose down && docker compose up -d atlas

# 驗證 health
curl -fsS http://localhost:18080/health
```

### 6. Verify

確認衝突解除且 atlas 正常服務:

- [ ] `curl -fsS http://localhost:18080/health` 回 200,內容 `{"status":"ok",...}`。
- [ ] `docker compose ps` atlas 狀態為 `Up` / `healthy`(若用 Docker)。
- [ ] `lsof -i :18080` 顯示 atlas 行程(或 docker-proxy)為唯一 LISTEN 進程。
- [ ] Dashboard 可訪問:`http://localhost:18080/admin/` 與 `http://localhost:18080/client/` 都能載入。
