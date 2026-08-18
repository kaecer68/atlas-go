# Atlas Dashboard API Contract

> **文件角色**：定義 admin_web / client_web / shared_web 常用的 dashboard 與 macro endpoint HTTP contract，讓前端開發者不必讀 handler 源碼即可正確消費 API。  
> **狀態**：v1.0（2026-07-12 P2-3 初版）  
> **Source-of-truth**：`internal/monitoring/api/*` 各 handler + `internal/monitoring/service/*`  
> **關聯**：[`docs/swagger.json`](../swagger.json) 提供部分 endpoint 的 OpenAPI schema；本文件補充欄位語義、缺失資料行為與前端注意事項。

---

## §0 前置約定

### 0.1 認證

`/api/dashboard/*` 與 `/api/macro/*` 目前在 `cmd/atlas/main.go isPublicPath` 中皆為 **public path**，瀏覽器呼叫時**不需要 API key / JWT**。若未來加入認證，會另開 spec。

### 0.2 時間格式

- `time.Time` / `recorded_at` / `generated_at` 原則為 **RFC3339**（含時區）。
- `timestamp`（int64）為 **Unix epoch seconds**，來自上游資料源。
- `session_id` 格式通常為 `session-YYYYMMDD-*`；交易日請從 `session_id` 解析，而非 `recorded_at`。

### 0.3 共用錯誤格式

多數 dashboard endpoint 回傳：

```json
{"error": "load xxx: ..."}
```

部分 endpoint 在資料尚未就緒時改回傳 200 + `{"status":"not_available", "message":"..."}`（例如 `/api/dashboard/drawdown`、`/api/dashboard/risk-calibration`），前端應以 `status` 欄位判斷。

常見 HTTP status：

| Status | 情境 |
| --- | --- |
| 200 | 成功；可能是空陣列/全零物件（資料尚未產生） |
| 400 | 必要參數缺失或格式錯誤 |
| 404 | 單一資源不存在（如 `/api/dashboard/data-channels/{name}`） |
| 500/503 | 後端服務未注入、資料檔無法讀取、上游失敗 |

### 0.4 缺失資料語義

| 表示方式 | 意義 | 前端處理 |
| --- | --- | --- |
| `null` / 欄位 omitted | 該指標無資料或未就緒 | 顯示「—」 |
| `NaN` / `Infinity` | 計算失敗或上游資料異常 | 顯示「—」 |
| `0` | 數值型欄位合法零值或資料未就緒 | **不可視為有效數值**；應顯示「—」並檢查 `data_status` / `insufficient_data` |
| 空字串 `""` | 字串型欄位無資料 | 視同 `null`，顯示「—」 |
| 空陣列 `[]` | 該維度尚無紀錄 | 顯示「無資料」 |
| HTTP 200 + `status: "not_available"` | 後端服務存在但資料尚未產生 | 顯示「載入中 / 無資料」 |
| HTTP 503 | 後端依賴未注入 | 顯示 API 錯誤，禁止以 `0` 渲染 |

#### 前端安全格式化層

所有頁面應透過 `shared_web/static/js/shared/format-metric.js` 的 `fmtSafe*` 系列函數渲染數值：

- `fmtSafeNumber(value, opts)` — 一般數值；無效值回傳「—」。
- `fmtSafePct(value, decimals)` — 百分比；無效值回傳「—」。
- `fmtSafeSignedPct(value, decimals)` — 帶正負號百分比；無效值回傳「—」。
- `fmtSafeSigned(value, opts)` — 帶正負號數值（非百分比）；無效值回傳「—」。
- `fmtSafeDrawdown(value)` — 最大回撤；無效值回傳「—」。
- `fmtSafeCurrency(value, opts)` — 幣別；無效值回傳「—」。
- `fmtSafeLargeNumber(value)` — 大數縮放；無效值回傳「—」。
- `isEmptyMetric(value)` — 統一判斷 `null` / `undefined` / `NaN` / `''`。

禁止在 `pages/*.js` / `components/*.js` 中直接呼叫底層 `formatNumber` / `fmtPct` / `fmtSignedPct` / `fmtDrawdown` 等函數後再補 `if (value == null)` 判斷。所有新頁面必須先 import `fmtSafe*`；既有頁面應隨重構逐步遷移。

### 0.5 Regime 列舉

`RISK_ON` / `RISK_OFF` / `NEUTRAL` / `TRANSITIONAL`。

---

## §1 Portfolio & Live Trading

### §1.1 `GET /api/dashboard/portfolio-state`

**Handler**：`internal/monitoring/api/live/handlers.go::HandlePortfolioState`

**Query**：無

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `snapshot_time` | RFC3339 | 快照時間 |
| `cash` | float64 | 現金餘額（台幣） |
| `starting_cash` | float64 | 起始資金（無資料時 omitted） |
| `portfolio_value` | float64 | 總市值 = cash + 持股市值 |
| `realized_pnl` | float64 | 已實現損益（omitempty） |
| `cumulative_pnl` | float64 | 累計損益 = realized + unrealized |
| `cumulative_pnl_pct` | float64 | 累計損益%（相對 starting_cash；starting_cash 為 0 時為 0） |
| `current_drawdown` | float64 | 當前回撤 |
| `max_drawdown` | float64 | 歷史最大回撤（由 equity_curve 計算） |
| `unrealized_pnl_total` | float64 | 未實現損益總計（omitempty） |
| `concentration_ratio` | float64 | HHI 集中度 [0,1] |
| `trade_count` | int | 交易筆數（omitempty） |
| `positions_count` | int | 持股檔數 |
| `positions` | array | 見 `PositionDTO` |
| `equity_curve` | array | 見 `EquityCurvePoint` |
| `cross_foot_pnl` | object | 驗證 portfolio unrealized 與個股加總是否一致 |

**PositionDTO**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `symbol` | string | 股票代號（可能含 `.TW`） |
| `name` | string | 中文名稱（無對應時回傳 symbol） |
| `quantity` | int | 股數 |
| `average_cost` | float64 | 平均成本 |
| `current_price` | float64 | 現價 |
| `market_value` | float64 | 市值 |
| `unrealized_pnl` | float64 | 未實現損益 |
| `pnl_pct` | float64 | 未實現損益%（相對成本） |
| `sector` | string | 產業分類（omitempty） |

**EquityCurvePoint**：`label`（session_id）、`value`（總市值）、`currency`、`after_tax_value`、`tax_paid`。

**CrossFootPnL**：`is_balanced`（差距 < 0.01）、`portfolio_unrealized`、`sum_positions_unrealized`、`difference`。

**缺失資料**：無 live state 時回傳空物件（`{}`），不是 503。

---

### §1.2 `GET /api/dashboard/live-status`

**Handler**：`internal/monitoring/api/live/handlers.go::HandleLiveStatus`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `circuit_breaker.state` | string | `unknown` / `normal` / `cooldown` / `halt` 等 |
| `circuit_breaker.state_changed_at` | RFC3339 | 狀態改變時間 |
| `circuit_breaker.consecutive_sl` | int | 連續停損次數 |
| `circuit_breaker.cooldown_until` | RFC3339 | 冷卻結束時間 |
| `circuit_breaker.intraday_peak` | float64 | 盤中最高市值 |
| `circuit_breaker.day_start_value` | float64 | 日初市值 |
| `portfolio.cash` | float64 | 現金 |
| `portfolio.available_cash` | float64 | 可用現金 |
| `portfolio.total_exposure` | float64 | 總曝險 |
| `portfolio.day_pnl` | float64 | 今日損益 |
| `portfolio.unrealized_pnl` | float64 | 未實現損益 |
| `portfolio.positions_count` | int | 持股檔數 |
| `timestamp` | RFC3339 | 回應產生時間 |

---

### §1.3 `GET /api/dashboard/risk-exposure`

**Handler**：`internal/monitoring/api/live/handlers.go::HandleRiskExposure`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `snapshot_time` | RFC3339 | 快照時間 |
| `var_95` / `var_99` / `cvar_95` | float64 | 風險值 |
| `max_drawdown_pct` | float64 | 最大回撤% |
| `portfolio_value` | float64 | 總市值 |
| `cash_ratio` | float64 | 現金佔比 |
| `position_count` | int | 持股檔數 |
| `sector_exposure` | array | 各產業曝險 `sector`/`sector_label`/`weight`/`est_value` |
| `factor_exposure` | object | `momentum`/`value`/`quality`/`agent`/`total` 平均分數 |
| `concentration` | array | 前 5 大持倉 `symbol`/`market_value`/`weight` |
| `data_points` | int | 計算用的日報酬樣本數 |
| `insufficient_data` | bool | 樣本 < 30 時為 `true` |

**缺失資料**：session 資料不足時 `insufficient_data=true`，風險數值為 `0`，前端應顯示「—」。

---

## §2 System Health & Status

### §2.1 `GET /api/dashboard/system-health`

**Handler**：`internal/monitoring/api/system/handlers.go::HandleSystemHealth`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `baseline_version` | string | 基線策略版本，如 `v42`；無資料時為「未知」 |
| `replay_data_latest_date` | string | replay 資料最新日期 |
| `replay_data_path_ok` | bool | replay 路徑是否可讀 |
| `last_window_id` | string | 最後回測窗口 ID |
| `last_window_generated_at` | RFC3339 | 最後窗口產生時間 |
| `warnings` | array | 人類可讀警示清單（中文） |
| `regime` | string | 當前市場 regime（§0.5） |
| `data_channels` | array | 各 channel 健康狀態 |
| `degraded_channels` | array | status ≠ `ok` 的 channel_id 清單 |
| `cycle_stale` | bool | 產業週期資料是否超過 24h 未更新 |

**DataChannelInfo**：`channel_id`、`label`、`status`（`ok`/`warn`/`error`/`inactive`/`expected_delay`）、`status_text`、`updated_at`。

**缺失資料**：任一 channel 不健康時進入 `degraded_channels`，但不影響整體 200。

---

### §2.2 `GET /api/dashboard/phase3-status`

**Handler**：`internal/monitoring/api/system/handlers.go::HandlePhase3Status`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `prism_queued_tasks` | int | PRISM 佇列中任務 |
| `prism_completed_results` | int | PRISM 已完成任務 |
| `prism_top_agent_id` | string | 目前 Sharpe 最佳 agent |
| `prism_top_agent_sharpe` | float64 | 最佳 agent Sharpe |
| `spawning_active` | int | 活躍 spawn agent 數 |
| `spawning_candidates` | int | candidate 狀態 agent 數 |
| `reflexivity_active_loops` | int | 反射性活躍迴圈數 |
| `adversarial_last_score` | float64 | 最後對抗測試分數 |
| `adversarial_vulnerabilities` | array | 弱點清單 |
| `recorded_at` | RFC3339 | 紀錄時間 |

**缺失資料**：檔案尚未建立時回傳空物件（所有數值為 0），不是錯誤。

---

### §2.3 `GET /api/dashboard/capital-phase`

**Handler**：`internal/monitoring/api/system/handlers.go::HandleCapitalPhase`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `phase` | string | `simulation` / `paper` / `live` / `full` |
| `phase_start_date` | RFC3339 | 進入本階段日期 |
| `days_in_phase` | int | 已進入天數 |
| `total_capital` | float64 | 總資金 |
| `deployed_capital` | float64 | 已部署資金 |
| `reserve_cash` | float64 | 保留現金 |
| `rolling_sharpe` | float64 | 滾動 Sharpe |
| `max_drawdown` | float64 | 最大回撤 |
| `consecutive_losses` | int | 連續虧損次數 |
| `can_advance` | bool | 是否可晉級下一階段 |
| `advance_reason` | string | 晉級/未晉級原因（omitempty） |

---

### §2.4 `GET /api/dashboard/retail-sentiment`

**Handler**：`internal/monitoring/api/system/handlers.go::HandleRetailSentiment`

**Query**：無

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `sentiment_score` | float64? | 散戶情緒分數 [-1,1]（macro 快照就緒時才有值） |
| `margin_change_pct` / `short_change_pct` | float64? | 融資/融券 日變動% |
| `margin_balance` / `short_balance` | float64? | 融資/融券餘額 |
| `day_trading_ratio` | float64? | 當沖成交比 |
| `margin_percentile` | float64? | 融資餘額歷史分位數 [0,1] |
| `extreme_reading` | string | `frenzy` / `neutral` / `fear` |
| `score` / `composite_sentiment` | float64? | 綜合 RSI-tw 分數 |
| `interpretation` | string | 人類可讀解讀（中文） |
| `retail_futures_oi` | float64? | 小台指散戶未平倉多空差 |
| `etf_net_subscription` | float64? | ETF 淨申購張數（⚠️ 資料源已移除，恆為 null） |
| `sentiment_sub_indicators` | object | 子指標（category_a / category_c / category_d） |
| `fetcher_status` | object | 各資料源狀態 `ok`/`error`/`not_available`/`no_data` |

**缺失資料**：若 macro 快照不存在，多數 `*float64` 為 `null`，`interpretation` 為 `"no macro snapshot available"`。

---

## §3 Macro

### §3.1 `GET /api/macro/snapshot/latest`

**Handler**：`internal/monitoring/api/macro/handlers.go::HandleMacroSnapshotLatest`

**Response 200**：`MacroDataSnapshot` 物件，欄位為多個 `MacroDataPoint`；**未就緒的指標會被 omitted**，不會出現 `{"symbol":"","value":0}` 的 zero-value sentinel。

**MacroDataPoint**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `symbol` | string | 指標代號 |
| `value` | float64 | 最新值 |
| `change_pct` | float64 | 日變動% |
| `timestamp` | int64 | Unix epoch seconds |

**頂層欄位**：`us10y`、`dxy`、`vix`、`usd_twd`、`oil`、`gold`、`jpy`、`foreign_investor_net`、`domestic_fund_net`、`dealer_net`、`export_electronics`、`retail_margin_balance`、`retail_short_balance`、`tsmc_revenue`、`sox_index`、`dram_spot_price`、`taiwan_semi_index`、`cowos_utilization`、`capex_growth`、`cpi_yoy`、`bdi`、`silver`、`copper`、`tsm_adr`、`spx_index`、`ndx_index`、`dji_index`、`nvda`、`aapl`、`msft`、`taiex`、`historical_volatility`。

**狀態欄位**：`data_status`（`ok`/`degraded`/`stale`）、`failed_channels[]`、`stale_channels[]`、`recorded_at`。

**缺失資料**：完全無快照 → 404 `{"error":"no macro snapshot available"}`。部分 channel 失敗時仍回傳 200，`data_status` 標記為 `degraded`/`stale`。

---

### §3.2 `GET /api/dashboard/macro-radar`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleMacroRadar`

**Query**：`session_id`（選填，預設最新 session）

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `session_id` | string | session 識別碼 |
| `regime` | string | 該 session 的 regime |
| `guard_outcomes` | array | 各 guard 執行結果 |
| `broker_runtime` | object | broker 執行環境稽核 |
| `recorded_at` | RFC3339 | 紀錄時間 |

**GuardOutcome**：`guard_id`、`guard_skill`、`passed`、`reason`、`input_count`、`output_count`、`severity`（`soft`/`hard`）。

**BrokerRuntimeAudit**：`mode`、`adapter`、`signer`、`signer_version`、`key_id`、`max_retries`、`http_timeout_sec`、`http_attempts`、`retry_status_codes`、`max_clock_skew_sec`、`nonce_ttl_sec`、`nonce_store`、`nonce_store_path`、`nonce_redis_prefix`。

**缺失資料**：無資料時回傳空物件 `{}`。

---

### §3.3 `GET /api/dashboard/us-indices`

**Handler**：`internal/monitoring/api/crossmarket/handlers.go::HandleUSIndices`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `recorded_at` | int64 | 快照 Unix epoch |
| `generated_at` | RFC3339 | 回應產生時間 |
| `indices` | array | 美股指數 `CrossMarketIndex` |
| `tech_stocks` | array | 科技股 `CrossMarketIndex` |
| `data_status` | string | `ok`/`degraded`/`stale` |
| `failed_channels` | array | 失敗 channel 名稱 |
| `stale_channels` | array | 過期 channel 名稱 |

**CrossMarketIndex**：`symbol`、`value`、`change_pct`、`timestamp`。

**缺失資料**：任一 upstream 失敗時回傳 500 + `{"error":"..."}`；正常情況下缺失指標會從陣列中省略。

---

## §4 Pipeline & Agents

### §4.1 `GET /api/dashboard/recommendation-pipeline`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleRecommendationPipeline`

**Query**：

- `session_id`（選填，預設最新）
- `show_all=true`（選填，是否顯示未通過 guard 的項目）

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `session_id` | string | session 識別碼 |
| `regime` | string | 該 session regime |
| `items` | array | 推薦標的 `PipelineItem` |
| `guard_outcomes` | array | guard 結果 |
| `screened_items` | array | 被篩掉的標的 |
| `recorded_at` | RFC3339 | 紀錄時間 |
| `is_fallback_session` | bool | 是否為 fallback session |
| `fallback_message` | string | fallback 原因 |
| `cycle_status` | object | 產業週期狀態 |
| `status` / `status_message` | string | pipeline 載入狀態與訊息 |

**PipelineItem 主要欄位**：`symbol`、`agent_id`、`skill`、`layer`、`side`、`conviction`、`target_price`、`stop_loss_price`、`forward_return`、`hit`、`reason`、`price`、`passed_guards`、`guard_reason`、`tags`、`recorded_at`、`factor_scores`（§4.3）、`conviction_breakdown`、`narrative_event_ids`、`narrative_context`、`industry_context`、`metrics`（PE/PB/殖利率/回測報酬，均為 optional）。

**缺失資料**：無 session 時回傳空 `RecommendationPipelineResponse`（`items: []`）。

---

### §4.2 `GET /api/dashboard/sessions`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleSessions`

**Response 200**：

```json
{
  "sessions": [
    {"session_id": "session-20260711-daily", "recorded_at": "...", "regime": "RISK_ON", "outcome_count": 42}
  ]
}
```

**缺失資料**：無 session 時回傳 `{"sessions":[]}`。

---

### §4.3 `GET /api/dashboard/agent-observatory`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleAgentObservatory`

**Query**：`session_id`（選填）、`limit`（選填，預設 5，最大 50）

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `session_id` | string | session 識別碼 |
| `next_experiment_agent_id` | string | 系統建議下一個實驗對象 |
| `scorecards` | array | agent 績效卡 |
| `recorded_at` | RFC3339 | 紀錄時間 |

**Scorecard 主要欄位**：`agent_id`、`skill`、`layer`、`observations`、`windows`、`hit_rate`、`average_return`、`sharpe`、`max_drawdown`、`t_stat`、`darwinian_weight`、`darwinian_sharpe`（optional）、`regime_breakdown`（optional）、`last_updated_at`。

**缺失資料**：NaN/Inf 會被 sanitized 為 `0` 或 `null`；`darwinian_sharpe` 為 `null` 代表該 agent 尚未被 Darwinian 追蹤。

---

### §4.4 `GET /api/dashboard/forecast-vs-reality`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleForecastVsReality`

**Query**：`agent_id`（選填）、`limit`（選填，預設 20，最大 100）

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `items` | array | 實驗結果比較 |
| `symbol_predictions` | array | 個股預測紀錄 |
| `broker_runtime` | object | broker 執行環境稽核 |

**ForecastVsRealityItem**：`experiment_id`、`proposal_id`、`commit_id`、`approval_id`、`target_agent_id`、`skill`、`mutation_type`、`status`、`baseline_value`、`candidate_value`、`recorded_at`。

**SymbolPredictionItem**：`agent_id`、`symbol`、`side`、`conviction`、`target_price`、`forward_return`、`hit`、`passed_guards`、`recorded_at`、`session_id`。

---

### §4.5 `GET /api/dashboard/regime-history`

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleRegimeHistory`

**Query**：`limit`（選填，預設 30，最大 100）

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `sessions` | array | `{session_id, regime, recorded_at}` |
| `transitions` | array | `{from_regime, to_regime, timestamp}` |
| `current_regime` | string | 最新 regime |

---

### §4.6 `GET /api/dashboard/regime-consistency`（Reconciler v1 P1）

**Handler**：`internal/monitoring/api/pipeline/handlers.go::HandleRegimeConsistency`

**Query**：`days`（選填，look-back 天數，預設 30，最大 365）

**語義**：Phase 2 Reconciler v1 的 regime 三端點一致性檢查 — session-level regime（`LoadSessions` / `sessions/*/summary.json`）vs `regime_history`（權威端, Janus 詞彙 RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL）vs `stress_index_history`（TaiwanStressCalculator 詞彙 low/alert/high/crisis），後者經 `narrative.RegimeVocabularyMapping` cross-walk 成權威詞彙後逐日比對。

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `authoritative` | string | 權威端點（固定 `regime_history`） |
| `window_days` | int | look-back 天數 |
| `availability` | object | 各端點是否可讀（`regime_history` / `stress_index` / `sessions`） |
| `regime_history` | object | `{rows, regimes, latest_date, latest_regime}`（含 stage-4 backfill synthetic rows） |
| `sessions` | object | `{scanned, total, regimes, unknown_count, unknown_ratio}` |
| `stress_index` | object | `{rows, regimes(原始詞彙), normalized(cross-walk 後), latest_date, latest_regime}` |
| `compared_days` | int | 窗口內有權威 regime_history row 的天數 |
| `matches` | int | 全端點一致的天數 |
| `drifts` | int | 任一非權威端點與權威端不一致的天數 |
| `unknown_count` / `unknown_ratio` | int / float | 窗口內 unknown session 數與比例（writer 缺口訊號） |
| `status` | string | `ok` / `drift` / `unknown_high` / `degraded` |
| `drift_details` | array | `{date, authoritative, endpoint, actual, normalized}` |
| `writer_gap` | object | unknown session 歸因（`empty_regime_in_summary` / `missing_summary` / `root_cause`） |

**Status 規則**：`drifts>0` → `drift`；`unknown_ratio>0.2` → `unknown_high`；HistoricalStore 未接線（legacy）→ `degraded`（僅 session 端）。

---

## §5 Experiments

### §5.1 `GET /api/dashboard/experiment-inbox`

**Handler**：`internal/monitoring/api/experiment/handlers.go::HandleInbox`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `pending_judges` | array | 等待評判的實驗 |
| `pending_promotes` | array | 已 Accepted、等待 promote 的最新實驗 |
| `recent_history` | array | 最近 10 筆其他狀態實驗 |
| `baseline_version` | int | 當前 baseline 版本 |
| `items` | array | 上述三類合併清單 |

**ExperimentInboxItem**：`experiment_id`、`target_agent_id`、`skill`、`mutation_type`、`mutation_summary`、`status`、`baseline_value`、`candidate_value`、`baseline_monetary_ntd`、`candidate_monetary_ntd`、`candidate_path`、`reject_reason`、`recorded_at`。

**缺失資料**：無實驗時回傳空陣列。

---

## §6 Data Channels & Pipeline

### §6.1 `GET /api/dashboard/data-channels`

**Handler**：`internal/monitoring/api/dashboard/data_channels.go::HandleDataChannels`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `channels` | array | 各 channel 狀態 |
| `alerts` | array | 需要人類介入的 alert |
| `generated` | RFC3339 | 回應產生時間 |

**DataChannel**：`channel_id`、`country`、`platform`、`api_format`、`path`、`storage`、`status`、`status_text`、`updated_at`、`last_error`（omitempty）、`error_severity`（omitempty）、`enabled`。

**ChannelAlert**：`channel_id`、`status`、`error`。

**狀態語義**：`ok` / `warn` / `error` / `inactive` / `expected_delay`。

---

### §6.2 `GET /api/dashboard/data-pipeline`

**Handler**：`internal/monitoring/api/dashboard/data_pipeline.go::HandleDataPipeline`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `sources` | array | 各資料源新鮮度 |
| `generated` | RFC3339 | 回應產生時間 |

**PipelineSourceStatus**：`source_id`、`producer`、`consumer`、`last_produced`、`last_consumed`、`status`、`lag_human`、`file_path`。

---

## §7 Risk & Drawdown

### §7.1 `GET /api/dashboard/risk`

**Handler**：`internal/monitoring/api/risk/handlers.go::HandleRiskMetrics`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `risk_snapshot` | object | 風險指標 |
| `session_count` | int | 可用 session 數 |
| `gate_mode` | string | RiskGate 模式 |

**risk_snapshot**：`var_95`、`var_99`、`cvar_95`、`max_drawdown_pct`、`data_points`、`insufficient_data`（樣本 < 30 時為 1）。

**缺失資料**：RiskGate 未注入 → 503；session 不足 → `insufficient_data=1`、數值為 `0`。

---

### §7.2 `GET /api/dashboard/risk-calibration`

**Handler**：`internal/monitoring/api/risk/handlers.go::HandleRiskCalibration`

**Response 200**（資料就緒）：

```json
{"report": {...}, "generated": "2026-07-12T00:00:00+08:00"}
```

**Response 200**（資料未就緒）：

```json
{"status": "not_available", "message": "no calibration report available yet", "generated": "..."}
```

**缺失資料**：未校準時回傳 `status: not_available`，不是 error。

---

### §7.3 `GET /api/dashboard/drawdown`

**Handler**：`internal/monitoring/api/dashboard/drawdown.go::HandleDrawdown`

**Response 200**（資料就緒）：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `max_drawdown` | float64 | 最大回撤 |
| `var_95` | float64 | 95% VaR |
| `worst_path` | array | 最差路徑的累積報酬序列 |
| `generated` | RFC3339 | 回應產生時間 |

**Response 200**（資料未就緒）：

```json
{"status": "not_available", "message": "no drawdown simulation available yet", "generated": "..."}
```

---

## §8 Tax

### §8.1 `GET /api/dashboard/tax-snapshot`

**Handler**：`internal/monitoring/api/tax/handlers.go::HandleTaxSnapshot`

**Response 200**：

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `snapshots` | array | 每檔個股的稅務快照 |
| `before_tax_pnl` | float64 | 稅前損益 |
| `after_tax_pnl` | float64 | 稅後損益 |
| `total_tax_paid` | float64 | 總稅額 |
| `total_dividend_tax` | float64 | 股利稅 |
| `is_simulated` | bool | 是否為模擬計算（無持股時為 true） |
| `note` | string | 人類可讀說明 |

**TaxSnapshot**：`symbol`、`dividend_tax_rate`、`transaction_tax_rate`、`dividend_tax`、`transaction_tax`、`total_tax`、`after_tax_pnl`。

**缺失資料**：無持股時 `snapshots:[]`、`is_simulated:true`。

---

## §9 Industry

### §9.1 `GET /api/dashboard/industry-cycle`

**Handler**：`internal/monitoring/api/industry/handlers.go::HandleIndustryCycle`

**Query**：`industry=<string>`（選填；無則回傳全部）

**Response 200**（無 `industry`）：

```json
{
  "industries": [
    {
      "industry": "semiconductor",
      "name": "半導體",
      "business_cycle": "expansion",
      "inventory_cycle": "destocking",
      "capex_cycle": "upturn",
      "confidence": 0.72,
      "is_favorable": true,
      ...
    }
  ],
  "count": 1
}
```

**Response 200**（有 `industry`）：單一產業物件。

| 欄位 | 型別 | 語義 |
| --- | --- | --- |
| `industry` | string | 產業 ID |
| `name` | string | 中文名稱 |
| `business_cycle` | string | 景氣循環階段 |
| `inventory_cycle` | string | 庫存循環階段 |
| `capex_cycle` | string | 資本支出循環階段 |
| `confidence` | float64 | 信心分數 |
| `is_favorable` | bool | 是否處於有利階段 |
| `phase_score` | float64 | 階段分數 |
| `trend` | string | 趨勢方向 |
| `confidence_breakdown` | object | 信心拆解 |
| `threshold_evidence` / `evidence` | object/array | 判定證據 |
| `narrative_theme` | string | 關聯 narrative theme |

**缺失資料**：找不到產業 → 404 `{"error":"industry not found"}`。

---

## §10 跨 API 約定

### 10.1 並發與部分失敗

admin/client 首頁同時發出 10+ 請求。所有 endpoint 互相獨立，前端應以 `Promise.allSettled` 並發，並讓每個 panel 有自己的 loading/error 狀態。

### 10.2 快取建議

| 端點 | 建議 TTL | 理由 |
| --- | --- | --- |
| `/api/dashboard/portfolio-state` | 30s | 即時部位 |
| `/api/dashboard/system-health` | 60s | 健康狀態變化慢 |
| `/api/macro/snapshot/latest` | 60s | macro 快照每數分鐘更新 |
| `/api/dashboard/recommendation-pipeline` | 30s | 推薦可能隨 session 更新 |
| `/api/dashboard/data-channels` | 30s | 頻道狀態 |
| `/api/dashboard/drawdown` | 5 分鐘 | 模擬結果不常變化 |
| `/api/dashboard/tax-snapshot` | 1 天 | 稅務每日計算即可 |

---

## §11 變更紀律

| 變更 | 必須同步 |
| --- | --- |
| 新增 `/api/dashboard/*` endpoint | 新增 §N |
| 修改 response struct（新增/刪除欄位、改單位） | 對應 schema 表 + example |
| 修改缺失資料行為（null vs 0 vs omitted） | §0.4 + 對應 endpoint |
| 修改認證方式 | §0.1 + 所有 endpoint |

| 版本 | 日期 | 變更 |
| --- | --- | --- |
| v1.0 | 2026-07-12 | P2-3 初版：涵蓋 admin/client 常用 dashboard 與 macro endpoint |
| v1.1 | 2026-07-12 | 新增 `portfolio-state.max_drawdown`；TAIEX 資料源透過 `taiex_index` channel 提供 |
