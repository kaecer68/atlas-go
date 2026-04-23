# Skills Map (Current)

> **最後更新**：2026-04-21（v1.3 完成後）
>
> 本文件是 atlas-go 的「現行技能地圖」，只描述目前程式與設定真正在用的技能、規則與流程。
>
> v1.3 新增四大功能模組：自然語言生成（NLG）、台灣稅務計算（Tax）、即時警報系統（Alerts）、階段式資金管理（Capital Management）。

## 1. 文件用途

- 對齊技能定義與執行行為
- 降低文件與程式脫節風險
- 讓 propose / execute / judge / decide 的責任邊界清楚可查

## 2. 真實來源（Source of Truth）

當本文與執行結果衝突時，以下內容優先：

1. `configs/agents.json`（與 `configs/agents.yaml` 並存對應）
2. `cmd/atlas/main.go`（runtime flags 與 guard）
3. `internal/live/*`（broker mode、nonce store、execution 路徑）
4. `internal/orchestrator/*`（含 `registry.go`、`plugin_control.go`、`executors.go`）
5. `internal/sim/*`（模擬引擎與部位 sizing）
6. `internal/experiment/*`（mutation、judge、promote）
7. `internal/narrative/*`（敘事事件、因果模板、投資模型與動態權重）
8. `internal/monitoring/dashboard_api.go`（報表輸出欄位）
9. `scripts/openclaw/today-start.sh`
10. `data/state/experiments/*.json`
11. `web/static/index.html`（控制塔前端介面）

> 註：`cmd/backfill-replay`、`cmd/daily-replay-sync`、`cmd/check-data-health`、`cmd/geo-ingest` 屬於資料管線或健康檢查工具，其行為規格詳見 §4.6 與 §4.12，不列為技能定義的 Source of Truth。
> 
> v1.3 新增 Source of Truth：
> - `internal/tax/*`（稅務計算與扣稅後部位調整）
> - `internal/monitoring/alert_store.go`、`notifier.go`、`alert_api.go`（警報持久化與通知）
> - `internal/narrative/report_generator.go`、`nlg_templates.go`（自然語言報告生成）
> - `internal/risk/capital_controller.go`、`approval_workflow.go`（資金階段控制與人工審批）
> - `internal/portfolio/capital_allocator.go`（多策略資金分配）

## 3. 目前啟用技能（來自 agents.json）

| Layer | Skill | 備註 |
|---|---|---|
| context | `taiwan_macro` | |
| context | `foreign_flow` | |
| sector | `semiconductor_desk` | Universe: 2330, 2303, 2454, 3034；支援宣告式篩選 |
| sector | `ai_supply_chain_desk` | Universe: 2382, 6669, 3017, 3037；支援宣告式篩選 |
| sector | `financials_desk` | Universe: 2881, 2882, 2891；支援宣告式篩選 |
| sector | `shipping_desk` | Universe: 2603, 2609, 2615；支援宣告式篩選 |
| sector | `etf_rotation_desk` | Universe: 0050, 0056, 00878；支援宣告式篩選 |
| style | `growth_momentum` | Universe: 2317, 2382, 2454, 3034, 3037, 6669；支援宣告式篩選 |
| style | `value_yield` | Universe: 2881, 2882, 2886, 2891, 0056, 00878；支援宣告式篩選 |
| style | `earnings_quality` | Universe: 2330, 2308, 3008, 1301, 1303, 1326；支援宣告式篩選 |
| style | `technical_breakout` | Universe: 2330, 2317, 2382, 2881, 2603, 2609, 0050；支援宣告式篩選 |
| superinvestor | `druckenmiller_macro` | 支援宣告式篩選 |
| superinvestor | `aschenbrenner_ai_compute` | 支援宣告式篩選 |
| superinvestor | `baker_deep_tech` | 支援宣告式篩選 |
| superinvestor | `ackman_quality` | 支援宣告式篩選 |
| control | `cro_risk` | conviction floor 過濾；支援宣告式篩選 |
| control | `cio_portfolio` | 聚合 + crowding penalty；支援宣告式篩選 |

## 4. 核心機制更新（近期變更）

### 4.1 Universe 分化（Style Layer）

為解決 90-day synergy 的 100% symbol overlap 問題，四個 style agent 已各自分配 6–7 檔流動性標的：

- `growth-momentum-01` → 科技/成長股
- `value-yield-01` → 金融/高股息/ETF
- `earnings-quality-01` → 傳產/半導體獲利品質股
- `technical-breakout-01` → 跨產業技術面突破標的

當 agent 未設定 Universe（`[]`）時，後端 fallback 至 `DefaultSymbols()`（23 檔流動標的）。

### 4.2 Crowding Penalty（CIO Portfolio 層）

`CIOPortfolioExecutor` 與 `CIOPortfolioExecutorWithWeights` 已加入擁擠懲罰：
- 當同一標的有 ≥3 個 agents 推薦時，該 symbol 的 avg conviction × 0.7
- Reason 前綴自動標註 `[crowded:N agents]`
- Dashboard 投資管線表格以黃色 badge 高亮顯示

### 4.3 NEUTRAL Regime Sizing

`sim.Engine` 在 `RegimeNeutral` 時自動將 `maxPositionWeight` 下修為原本的 0.85（縮減單一部位上限約 15%），以降低低信心 regime 的 drawdown 風險。

### 4.4 Dashboard 介面重構

`web/static/index.html` 已重構為 Unified Sidebar SPA，左側 sidebar 寬度 144px，切換 9 個功能頁（總覽 / 宏觀敘事 / 相對趨勢 / 投資管線 / AI 觀測台 / 模擬交易 / 最新回測 / 控制與稽核 / 信息通道）。

- **總覽**：頁面頂端有「如何解讀本頁」說明，強調這是基於最新回測窗口的系統狀態快照。8 張 KPI cards 中，第一列固定為「資料時間」與「基線版本」以宣告時間線與統計基準；其餘卡片（敘事脈絡、市場狀態、表現最差 AI、實驗狀態、擁擠標的、信息通道預警）皆可點擊彈出說明視窗，解釋指標意義與後續注意事項。
- **宏觀敘事**：整合原 `narrative-dashboard.html` 的 6 大面板。總經快照中的指標改為「英文+中文」雙語標示（如 DXY-美元指數），並在頂部增加資料通道健康度燈號（🟢/🟡/🔴）。台灣市場壓力指數納入 6 大子項：DXY-美元指數、US10Y-美債10年期、外資流向、VIX-波動率指數、日圓-套利平倉壓力、地緣政治風險，並可點擊標題旁的 ℹ️ 查看區間解讀。因果傳導鏈全面本地化為中文，主題 ID 與名稱均為中文，每個步驟都會顯示影響的板塊標籤；負數影響力以紅字呈現。
- **相對趨勢**：頁面頂端有控制層處置結果的解讀說明。總經敘事脈絡橫幅（雷達上方）顯示 stress score、top event、情緒方向、壓力等級建議，以及根據 narrative model 自動匹配的看多/看空板塊。總經雷達顯示最新回測場次的 control-layer 處置紀錄（放行/過濾/阻擋）與可進場標的前 5 檔。即時狀態欄寬縮小為 170px；若系統處於 Simulation 模式，會顯示「未連接 live broker」的說明而非空白「無資料」。
- **信息通道**：新增第 9 個 sidebar tab，展示 6 條外部資料通道（US Yahoo Finance、TWSE Replay、TWSE Capital Flow、Fugle、Japan Yahoo Finance、Geopolitical Risk RSS+GDELT）的健康狀態，包含燈號、最後更新時間與錯誤訊息。後端透過 `internal/monitoring/channel_health.go` 持久化每次抓取的成敗紀錄，確保通道異常能即時反映。
- **投資管線**：頁面頂端有「如何解讀本頁」說明。表格列出最新場次的所有通過推薦，新增「公司名稱」欄位（由前端 `STOCK_NAME_MAP` 映射 24 檔常見台股），「信念」與「遠期報酬」欄位標題旁皆有 ℹ️ 可點擊查看數值意義；擁擠交易以黃色 badge 高亮。可逐筆批准 / 拒絕 recommendation，這些動作會寫入控制稽核紀錄。
- **模擬交易**：可視化評判 / 晉升流程與 diff 預覽。
- **控制與稽核**：除原有的 pause/resume agent、產業封鎖、baseline 晉升/回滾與稽核紀錄外，新增「資料採集健康度」面板，可集中監督 6 項外部資料來源的即時狀態。
- 舊書籤相容：`trading-dashboard.html` 與 `narrative-dashboard.html` 均自動 redirect 回 `/`。

### 4.5 Dashboard 資料一致性與術語本地化

- **Session 一致性修復**：`handleRecommendationPipeline` 已改為統一使用 `loadSessionSummary("")`，與 `handleMacroRadar`、`handleAgentObservatory` 採用相同的「按 `SessionID` 中的交易日降冪取最新（`RecordedAt` 僅作為平手輔助）」邏輯，解決了投資管線與總經雷達可能顯示不同場次的 bug。
- **Guard 原因本地化**：`internal/orchestrator/executors.go` 中的控制層 reason 字串已從英文改為中文業務語言（如「未過濾任何推薦，全部放行」、「過濾了 N 筆推薦，僅保留符合條件的標的」、「強制阻擋全部推薦，當日不進場」）。
- **Agent 顯示名稱本地化**：`configs/agents.json` 與 `configs/agents.yaml` 的 17 個 agent `name` 欄位已全部改為中文，與前端 `AGENT_NAME_MAP` 對齊。
- **信息通道預警**：`web/static/index.html` 的總覽頁新增「信息通道預警」KPI 卡片，即時提示異常或延遲的資料通道；點擊卡片可直接跳轉至信息通道頁查看詳細燈號與錯誤原因。

### 4.6 Replay 資料自動同步與回填機制

為解決回測資料（`data/replay/tw_extended_90days.csv`）停滯於歷史日期的問題，新增以下工具：

- **`cmd/daily-replay-sync`**：每日自動透過 TWSE OpenAPI 抓取當日所有上市股票的 OHLCV，過濾出 `DefaultSymbols()` 的 23 檔標的後，直接 append 到 `data/replay/tw_extended_90days.csv`。已實測於 2026-04-14 成功抓取並寫入 24 筆紀錄。每次抓取成敗會同步寫入 `data/state/channel_health.json`，供信息通道頁面顯示。
- **`cmd/backfill-replay`**：透過 **FinMind API** (`TaiwanStockPrice`) 回填缺失的歷史日線資料。TWSE OpenAPI 的 `STOCK_DAY` 端點對歷史日期返回 HTML 錯誤頁，不支援程式化歷史回填；而 FinMind 提供穩定的歷史股價查詢，且其 2026 年資料與現有 replay 資料完全對齊。已實測回填 2026-04-01 ~ 2026-04-14 共 209 筆紀錄（涵蓋 32 檔標的、6 個交易日）。
- **`scripts/merge-twse-csv.py`**：用於手動合併從 TWSE 網站下載的單日 CSV。可處理欄位名稱映射（如「證券代號」、「成交股數」、「開盤價」等），自動去重後合併進主 replay CSV。

**啟動時自動回填**：`cmd/atlas` 啟動 API server 時，會在背景 goroutine 自動檢查 replay CSV 的最新交易日。若發現與「上一個應有資料的交易日」存在缺口，會自動觸發 `backfill-replay` 進行 FinMind 回填（受 5 分鐘超時保護），確保系統停機維護後重新上線時資料不會斷層。

**自動排程抓取**：`docker-compose.yml` 已新增 `cron-replay-sync` 服務，每日台股收盤後 15:30 自動執行 `daily-replay-sync`。

**使用方式**：
```bash
# 每日自動同步（建議設為 cron job）
go run ./cmd/daily-replay-sync

# 透過 FinMind API 回填歷史缺失資料
go run ./cmd/backfill-replay -start 2026-04-01 -end 2026-04-14

# 手動合併從 TWSE 下載的歷史 CSV
python scripts/merge-twse-csv.py --source ~/Downloads/twse_20260401.csv --target data/replay/tw_extended_90days.csv
```

### 4.7 宏觀敘事動態權重與因果論述

為讓【宏觀敘事】頁面的「投資模型」與「因果模板庫」從靜態展示轉為可驗證的決策輔助，進行了以下強化：

- **投資模型動態權重**：`internal/narrative/knowledge_base.go` 新增 `EvaluateModels(replayPath)`。每次開啟【宏觀敘事】頁面時，系統會即時讀取 `tw_extended_90days.csv`，比較最近 10 個交易日內「模型偏好板塊」vs「模型迴避板塊」的 5 日遠期報酬差，計算出 `RecentError = 1 - 正確率`，再透過 inverse-error weighting 自動調整三個模型的權重（總和為 1.0）。前端以進度條與顏色標籤即時呈現權重與誤差等級。
- **完整投資邏輯論述（Rationale）**：`CausalTemplate` 與 `InvestmentModel` 結構新增 `Rationale` 欄位，並為全部 6 個因果模板與 3 個投資模型撰寫了詳細的中文論述，說明「為什麼應該押注 A 板塊、迴避 B 板塊」的底層經濟與市場機制。前端在投資模型卡片與因果模板庫表格中均提供「展開完整論述」按鈕，讓使用者能直接閱讀決策背後的說服力邏輯。

### 4.8 宣告式個股篩選（Screening Layer）

為讓系統具備真正的選股能力，在 `sector/style` executor 生成推薦**之前**新增 `internal/screener/` 宣告式篩選層：

- **篩選條件**：`ScreeningCriteria` 支援 P/E 區間、P/B 區間、股息率區間、20 日動能區間、日內成交量下限、最小總因子分數（`min_total_factor_score`）與必填因子清單。
- **資料來源**：`FactorEngine`（`internal/portfolio/factor_engine.go`）提供動能/價值/品質/綜合分數計算；基本面資料來自 `FundamentalProvider`（`data/fundamentals.json`）。
- **配置位置**：`configs/agents.json` 中已為 13 個 sector / style / superinvestor agent 加入 `screening_criteria`，例如：
  - `value-yield-01`：`pe max: 18`、`pb max: 2`、`dividend_yield min: 2.0`
  - `growth-momentum-01`：`volume_intraday min: 1_000_000`、`momentum_20d min: 0.0`
- **動態訊號強度對齊**：sector executors 的 `dynamicSignalStrength` 會自動讀取該 agent 的 `screening_criteria.volume_intraday.min` 作為成交量激勵門檻，使 executor 的訊號強度與篩選閘門保持一致。
- **注意事項**：篩選條件過嚴可能導致某檔標的「完全沒有推薦」，這是預期行為。調整門檻前請先用 `go test ./internal/screener/...` 驗證。

#### 篩選後的 FactorScores

被篩選層拒絕的標的（`ScreeningReject`）同樣會附加完整 `FactorScores`（含 `Breakdown`），可用於前端調試或放行參考。 Breakdown 結構如下：

| 欄位 | 說明 |
|------|------|
| `score` | 該因子計算結果（-1 ~ 1） |
| `weight` | 該因子權重 |
| `formula` | 計算公式（如 `clamp(ret20 / 0.30, -1, 1)`） |
| `raw_inputs` | 原始輸入值（如 `ret20: 0.0275`） |
| `is_fallback` | true = 無歷史資料，使用日內價格或猜測值 |

#### 行業信念計算透明化（Phase 2）

所有 sector/style executor 的 `Recommendation` 現在都附加 `ConvictionBreakdown`，完整揭露信念分數的計算過程：

```
conviction_breakdown: {
  base: 70,    // 起始信念分
  floor: 60,   // 最低保護分
  final: 76,   // 最終信念分
  steps: [
    { rule: "rotation_boost", delta: +6, reason: "rotation keyword + last > open" },
    { rule: "volume_penalty", delta: -5, reason: "below avg volume" }
  ]
}
```

每個 step 代表一條增減分規則，來自 `convictionBuilder`（`internal/orchestrator/conviction_builder.go`）。

#### 宏觀敘事信心度（Phase 3）

每個偵測到的 `NarrativeEvent` 現在都有 `confidence_source`（固定值 `heuristic_fixed_v1`）與 `historical_hit_rate`（0.58 ~ 0.81 之間），用於評估事件偵測的歷史可靠性。

| 主題 | 歷史命中率 |
|------|-----------|
| AI_capex_surge | 0.81 |
| US_rates_up | 0.72 |
| JPY_carry_unwind | 0.68 |
| geopolitical_risk | 0.65 |
| oil_price_shock | 0.58 |

### 4.9 自然語言生成（NLG）

`internal/narrative/report_generator.go` 與 `nlg_templates.go` 實作中文自然語言報告生成：

- **日報產生器**：`GenerateDailySummary(date, events, recommendations, risk)` 組合市場概覽、宏觀敘事、推薦標的、風險評估四大區塊，輸出結構化的 `DailySummaryReport`
- **因子分數解釋**：`ExplainFactorScores(rec)` 為每個推薦標的生成中文因子分析（動能/價值/品質），說明分數來源與計算邏輯
- **API 端點**：`/api/dashboard/daily-summary` 回傳 JSON 格式的日報，供前端「總覽」頁面展示
- **模板系統**：所有 NLG 輸出基於預定義中文模板（`TemplateMarketOverview`、`TemplateNarrativeSummary` 等），確保輸出穩定且可預期

### 4.10 台灣稅務計算（Tax Module）

`internal/tax/` 提供台灣市場專用稅務計算：

- **股利稅**：28%（含 NHI 附加費），`CalculateDividendTax(dividendAmount)`
- **證交稅**：0.3%（僅賣出方），`CalculateTransactionTax(sellNotional)`
- **部位稅務快照**：`CalculatePositionTax(pos, sellPrice, dividendReceived)` 計算單一部位的總稅負擔與稅後損益
- **投組稅務彙總**：`CalculatePortfolioTax(positions, sellPrices, dividends)` 計算全部持有部位的稅務影響
- **稅務調整損益**：`SimulationResult` 新增 `BeforeTaxPnL`、`AfterTaxPnL`、`TotalTaxPaid`、`TaxSnapshots` 欄位
- **扣稅後部位調整**：`TaxAwareSizer` 在部位配置前預留證交稅資金，降低實際進場金額

### 4.11 即時警報系統（Alerts）

`internal/monitoring/` 擴展為完整警報生命週期管理：

- **警報持久化**：`AlertStore` 以 JSONL 格式保存所有警報，支援 `Save`、`LoadAll`、`LoadUnacknowledged`、`Acknowledge` 操作
- **通知通道**：
  - **Telegram Bot**：`TelegramNotifier` 透過 Bot API 發送格式化警報訊息（含 emoji 嚴重程度標示）
  - **Email**：`EmailNotifier` 透過 SMTP 發送警報郵件
  - **Webhook**：`WebhookNotifier` 透過 HTTP POST 將警報 JSON 推送至外部系統
- **警報整合**：`Monitor` 整合 `AlertStore` 與 `Notifier`，規則觸發時自動持久化並分發通知
- **API 端點**：`/api/alerts`（列表）、`/api/alerts/unacknowledged`（未確認）、`/api/alerts/acknowledge`（確認）
- **配置**：`AlertChannelConfig` 統一管理各通道的認證資訊（bot token、SMTP 設定、webhook URL）

### 4.12 階段式資金管理（Capital Management）

`internal/risk/` 與 `internal/portfolio/` 實作四階段資金部署機制：

- **資金階段**：`CapitalPhase` enum 定義四個階段
  - `simulation`（A）：模擬階段，無資金限制
  - `paper`（B）：模擬交易，10% 資金上限
  - `live`（C）：實盤交易，30% 資金上限
  - `full`（D）：全自動擴展，100% 資金
- **晉升條件**：`CapitalPhaseController.CanAdvance()` 檢查三項指標
  - 最低 30 天階段停留期
  - 滾動 Sharpe > 1.0
  - 最大回撤 < 10%
- **人工審批**：`Live → Full`（C→D）晉升需人工核准。`ApprovalWorkflow` 以 JSONL 持久化審批請求，支援 `RequestApproval`、`Approve`、`Reject` 操作
- **資金分配**：`CapitalAllocator` 根據當前階段限制計算可部署資金，並依推薦標的數量均分。`ReallocateWithTax()` 進一步扣除預期稅負後重新分配
- **系統整合**：`SystemCore` 每日模擬後自動更新滾動 Sharpe 與回撤，檢查晉升條件。`cmd/atlas/main.go` 初始化時注入 `CapitalPhaseController` 與 `ApprovalWorkflow`

### 4.13 資料健康檢查與地緣政治風險擷取

- **`cmd/check-data-health`**：檢查 replay CSV、ledger、基線政策等核心資料的完整性與一致性，輸出健康報告
- **`cmd/geo-ingest`**：擷取並彙整地緣政治風險資料（GDELT + RSS），供 `internal/narrative/geopolitical_provider.go` 使用
- **`cmd/daily-replay-sync`** 與 **`cmd/backfill-replay`**：詳見 §4.6，已新增 `check-data-health` 自動觸發機制於系統啟動時

## 5. 技能分層與責任

### 5.1 Domain Skills（研究與觀點）

| Group | Skills | 主要責任 |
|---|---|---|
| Context | `taiwan_macro`, `foreign_flow` | 給市場狀態與風險偏向 |
| Sector | `semiconductor_desk`, `ai_supply_chain_desk`, `financials_desk`, `shipping_desk`, `etf_rotation_desk` | 產業面候選與敘事約束 |
| Style | `growth_momentum`, `value_yield`, `earnings_quality`, `technical_breakout` | 進出場品質與風格濾鏡（已分配獨立 Universe） |
| Superinvestor | `druckenmiller_macro`, `aschenbrenner_ai_compute`, `baker_deep_tech`, `ackman_quality` | 高信念補充觀點 |

> 所有 sector/style/control executor 的 Recommendation 現在都附加 FactorScores（含 Breakdown）與 ConvictionBreakdown，用於前端決策鏈透明度展示。

### 5.2 Control Skills（風控與整合）

| Skill | 責任 |
|---|---|
| `cro_risk` | 後置風險過濾（hard guard），低於 conviction floor 者阻擋 |
| `cio_portfolio` | 最終聚合、排序，並套用 crowding penalty |

### 5.3 Operating Skills（流程運作）

以下屬於流程能力，主要由 scripts / cmd 與 orchestration 實作：

**資料與回測：**
- `replay_operator`
- `backtest_operator`
- `ledger_operator`
- `data_import_operator`
- `data_health_checker`（v1.3：`cmd/check-data-health`）

**監控與警報：**
- `monitoring_operator`
- `alert_dispatcher`（v1.3：Telegram/Email/Webhook 通知）
- `channel_health_monitor`（`internal/monitoring/channel_health.go`）

**敘事與巨集觀：**
- `narrative_engine`
- `macro_ingestor`
- `geopolitical_risk_monitor`
- `geo_ingestor`（v1.3：`cmd/geo-ingest`）
- `taiwan_stress_calculator`
- `capital_flow_provider`

**風控與資金：**
- `system_guardrail`
- `risk_manager`（v1.3：含 `CapitalPhaseController`）
- `capital_allocator`（v1.3：`internal/portfolio/capital_allocator.go`）
- `tax_calculator`（v1.3：`internal/tax/taiwan_tax.go`）

**研究與報告：**
- `research_auditor`
- `report_generator`（v1.3：`internal/narrative/report_generator.go`）
- `approval_workflow`（v1.3：`internal/risk/approval_workflow.go`）

### 5.4 Evolution Skills（演化能力）

- `weak_agent_selector`
- `prompt_mutator`
- `experiment_designer`
- `experiment_judge`

### 5.5 Platform Skills（AI 助理層可用技能）

- `graphify`
- `kimi-cli-help`
- `skill-creator`

## 6. Mutation Profiles（現行）

支援的 mutation type：

**通用型：**
- `prompt_tightening`
- `risk_rule_change`
- `portfolio_constraint_revision`

**產業/風格特化型**（v1.2+ 新增）：
- `semiconductor_desk`
- `financials_desk`
- `technical_breakout`
- `etf_rotation_desk`
- `value_yield`
- `growth_momentum`
- `ai_supply_chain_desk`
- `shipping_desk`
- `earnings_quality`

實作位置：`internal/experiment/executor.go`

## 7. Judge 規則（現行）

實作位置：`internal/experiment/judge.go`

### 7.1 接受條件

- 必須 `candidate > baseline`
- 最小改善門檻：
  - `prompt_tightening`: 0.0005
  - `risk_rule_change`: 0.001
  - `portfolio_constraint_revision`: 0.001
- 觀測數門檻：
  - `level_3_regime_aware`: 12（風險/約束型 +1）
  - `level_2_window_validated`: 8（風險/約束型 +1）
  - default: 3（風險/約束型 +1）

### 7.2 檢查項數量門檻

| Maturity | 基礎檢查項 | prompt_tightening | risk_rule_change | portfolio_constraint_revision |
|---|---|---|---|---|
| `level_3_regime_aware` | 4 | 4 | 5 | 6 |
| `level_2_window_validated` | 3 | 3 | 4 | 5 |
| default | 2 | 2 | 3 | 4 |

## 8. today-start Guard 與選擇器

實作位置：`scripts/openclaw/today-start.sh`

### 8.1 連敗 futility guard

- 同 `agent + window + mutation_type`
- 最近 3 筆皆 `candidate <= baseline` → 判定為 futile

### 8.2 最小樣本門檻

- `--min-sample-for-rank N`，預設 `2`
- 歷史樣本數 `n < N` 者不參與加權排名

### 8.3 加權排名公式

- `avg_delta = avg(candidate - baseline)`（lookback=5）
- `weighted = avg_delta * min(1, n/5)`

## 9. 文件維護規範

以下變更必須同步更新本文件：

### v1.1/v1.2 維護項目
1. `configs/agents.json` 的技能集合或 Universe 變更
2. `executor.go` 的 mutation 模板語句
3. `judge.go` 的接受門檻或檢查項
4. `today-start.sh` 的 guard、排名參數
5. Dashboard 前端結構或 API 欄位變更
6. `internal/narrative/*` 的模板、模型、權重計算邏輯或論述內容變更
7. `internal/monitoring/channel_health.go` 的通道健康紀錄格式變更
8. `internal/monitoring/dashboard_api.go` 的 API parsing struct JSON tag（務必與 `domain.*` struct 的 JSON tag 保持 snake_case 一致）
9. 決策鏈透明度功能（FactorScores Breakdown、ConvictionBreakdown、MacroEvent 信心度）的任何變更

### v1.3 新增維護項目
10. `internal/tax/*` 的稅率、稅種或計算邏輯變更（台灣稅法調整時）
11. `internal/monitoring/alert_store.go`、`notifier.go`、`alert_api.go` 的警報格式、通知通道或 API 欄位變更
12. `internal/narrative/report_generator.go`、`nlg_templates.go` 的報告區塊、模板語句或輸出格式變更
13. `internal/risk/capital_controller.go` 的資金階段定義、晉升條件或限制比例變更
14. `internal/risk/approval_workflow.go` 的審批流程狀態或持久化格式變更
15. `internal/portfolio/capital_allocator.go` 的資金分配演算法或權重計算變更
16. `cmd/atlas/main.go` 的資金管理初始化參數或階段預設值變更
