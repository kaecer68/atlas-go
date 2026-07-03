# v2 alert-redesign 升級真實路徑 (post-verification 2026-07-04)

> 設計 todo 基於 `.omo/briefs/alert-redesign-v2.md` Part 6 P0 優先序,但**經過 2026-07-04 真實驗證**,範圍已大幅縮小。
> 目標: 完成剩餘真實 gap → 把 v2 working draft 升為正式 docs/ 規格。

## 1. 驗證結果摘要

v2 寫於 2026-06-26 (10 天前)。本驗證用 grep 確認每個 v2 claim 的當前實際狀態:

| P0 Item | v2 聲稱 (10 天前) | 2026-07-04 實際狀態 | 結論 | 工作量 |
|---|---|---|---|---|
| **P0-1 Decision 8 (Portfolio Drawdown)** | 缺 alert consumer | `EventDrawdownBreach` 已定義 (eventbus.go:96), `PublishDrawdownBreach` 已實作, 測試存在 (`drawdown_breach_test.go`), **`internal/monitoring/` 無對應 consumer** | **REAL GAP** | 1 PR |
| **P0-2 Decision 6 (Trade Slippage)** | 缺 alert consumer | `EventTradeSlippage` 已定義 (eventbus.go:96), `PublishTradeSlippage` 已實作 (eventbus.go:1057), **`internal/monitoring/trade_slippage_consumer.go` 已存在 (134L)** | **ALREADY DONE** | 0 PR |
| **P0-3 Decision 7 (Concentration)** | 缺 alert rule | `ConcentrationScore` 已存在 (`portfolio_risk.go:6`), `max_position_weight_pct` 參數已存在 (`parameters.json:6668`), **`internal/monitoring/rules.go` L114 / L186 已有 rule 名稱定義** | **PARTIALLY DONE** — 需驗證是否實際 firing | 0-1 PR |
| **P0-4 Decision 5 reverse (Experiment)** | 缺 event emit | **`transitionExperimentStatus` 在 `judge.go` L209/216/223/232 共 4 處 call publish event per Decision 5 reverse** (L62 註解明示), `LifecyclePublisher` 已實作; **`internal/monitoring/` 無 alert consumer** | **PARTIALLY DONE** — emit 已做,缺 consumer | 1 PR |

**關鍵結論**: v2 規劃的 4 P0 PR,只有 1 個 (P0-1 Drawdown) 是真正需要從零開始的工作。其他 3 個要嘛已經做完 (P0-2),要嘛需要驗證 + 微調 (P0-3, P0-4)。這也驗證了 user 的方法論修正: 「若要實作那些過去未實作的工作,一定要檢查是否真的沒有做過,不用做重複或重疊,甚至做衝突的工作」。

## 2. 建議 PR 清單(由小到大,獨立可合併)

### PR-1: Drawdown Alert Consumer
- **目標**: 新增 `internal/monitoring/drawdown_breach_consumer.go`
- **訂閱**: `EventDrawdownBreach` event (eventbus.go:96)
- **邏輯**: 從 `DrawdownBreachPayload` 讀 `MaxDrawdownPct` (`eventbus.go:279`),與 threshold 比較,建 alert record
- **新參數**: `configs/parameters.json` 新增 `drawdown_breach_warning_pct` (預設 5%) + `drawdown_breach_error_pct` (預設 10%)
- **參考模式**: 抄 `trade_slippage_consumer.go` (134L template) 的 subscribe / classify severity / build alert record 三段式
- **測試模板**: 對應 `trade_slippage_consumer_test.go`
- **估時**: 4-8h

### PR-2: Experiment Alert Consumer
- **目標**: 新增 `internal/monitoring/experiment_alert_consumer.go`
- **訂閱**: `EventExperiment{Accepted,Rejected}` events
- **邏輯**: 從 `judge.go` L209/216/223/232 的 4 個 `transitionExperimentStatus` call 訂閱 published event,計算 accept/reject ratio,rejection 過高或試驗失敗時建 alert record
- **參考模式**: 同 PR-1 抄 `trade_slippage_consumer.go`
- **估時**: 4-8h

### PR-3 (conditional): Concentration Rule Wiring Verification
- **目標**: 確認 `position_concentration` rule 是否實際 firing on events
- **方法**: 讀 `internal/monitoring/rules.go` L114 (`Name: "position_concentration"`) + L186 (`Name: "high_position_concentration"`),tracing 到 event consumer / channel subscription
- **可能結果**:
  - 已有 wiring → 0 PR,僅調查記錄
  - 缺 wiring → 補 1 PR (subscribe rule to event)
- **估時**: 1-2h (僅調查) 或 4-8h (補 wiring)

### PR-4 (升級執行): v2 lift to docs/alerts-redesign.md
- **前置**: PR-1/2/3 全部完成 + 4. 待驗證 items 全部釐清
- **動作**:
  1. 建立升級版 alert-redesign 規格(從 v2 內容,但更新所有 v2 過時的 line number + 引用)
  2. 刪除 `.omo/briefs/ALERT_SYSTEM_REDESIGN.md` (v1, 15468B, gitignored — PR #756 故意保留)
  3. 刪除 `.omo/briefs/alert-redesign-v2.md` (v2 working copy, gitignored)
- **估時**: 2-4h

## 3. 排除項目 (already done per v2 design)

| v2 Item | 當前狀態 (2026-07-04 驗證) |
|---|---|
| P1-1: API 3 endpoints + sort param | `alert_api.go:99-103` 已實作 `sortAlerts(filtered, sortParam)`,註解直寫 "P1-1: alert-redesign-v2.md Part 6.1" |
| P1-2 (參數部分): SLA Meta-Alert | `parameters.json:6739-6754` 已包含 `alert_sla_critical_sec` (30min) / `alert_sla_error_sec` (2hr) / `alert_sla_warning_sec` (24hr) + `sla_violation_meta_alert` toggle |
| P1-3 (parameter part): Channel heartbeat staleness | `parameters.json:6736` `channel_heartbeat_staleness_sec` 已存在 |
| P2-1: Decision 1 channel heartbeat | 同上,參數已有 |
| EventBus publish + payload structs | 全部 4 P0 events + payloads + `Publish*` functions 都已實作 (`eventbus.go:96-279`, `936-1057`) |

## 4. 待驗證 (留待執行階段,不阻擋 P0-1/P0-4 啟動)

- **P1-2 SLA Meta-Alert LOGIC**: 參數已有 (`alert_sla_*`),但 consumer / rule firing 的邏輯是否實作? → 若 gap,需 1 PR
- **P1-3 3-fail logic**: v2 提到的 3-fail rule 是否實作? → 若 gap,需 1 PR
- **P2-2 / P2-3**: 2 個 backfill items 狀態 → 若 gap,需 1-2 PR

這些待驗證 items 若發現是 gap,可能會額外產生 1-2 PR。**驗證方式**: 在 PR-1 執行期間順便 grep `internal/monitoring/` 所有 consumer file,確認 SLA / 3-fail / backfill 是否已實作,回報結果。

## 5. Bucket B(roadmap 戰略項目,不在本 todo 範圍)

`roadmap-v2.md` Part 3 Bucket B 有 4 個更高戰略價值的 items:

- **B1**: Data Access Layer(Part 2 P-Infra 提前決策的核心)
- **B2**: Backtest-Live Consistency(Part 5 戰略議題)
- **B3**: HITL Regime Override(Part 6 戰略議題)
- **B4**: Tax/Liquidity Sizing(Part 7 戰略議題)

這些**不在本 alert-redesign todo 範圍**。需要各自獨立設計 + 評估 + 排程。B1 尤其關鍵 (影響所有下游 Backtest/Live/HITL/Tax work),應優先單獨評估。

## 6. 時間預估

| | v2 樂觀估計 | 真實驗證後估計 |
|---|---|---|
| P0-1 (Drawdown consumer) | 4-8h (1 PR) | 4-8h (1 PR) — 不變 |
| P0-2 (Slippage consumer) | 4-8h (1 PR) | **0h (已 done)** |
| P0-3 (Concentration wiring) | 8-12h (1-2 PR) | 1-8h (0-1 PR conditional) |
| P0-4 (Experiment consumer) | 8-12h (1-2 PR) | 4-8h (1 PR) |
| **總計 (P0)** | **4-8 PR, 2-3 工作天** | **2-3 PR, 1-3 工作天 (樂觀) / 5-7 工作天 (含 review 循環)** |

**差異**: v2 把所有 4 個 P0 都估 "從零開始";實際上現在 1 個 + 2 個 partial (不是 4 個全新)。整體工作量縮小約 50%。

## 7. 參考資料

- **v2 source (working draft)**: `.omo/briefs/alert-redesign-v2.md` (gitignored)
- **v1 (superseded but preserved)**: `.omo/briefs/ALERT_SYSTEM_REDESIGN.md` (15468B, PR #756 故意保留作為長壽規劃)
- **驗證日期**: 2026-07-04 (grep 驗證所有 v2 line numbers + symbol existence)
- **升級觸發條件** (v2 Part 9): 完成 P0 全部 4 個 PR 後(估 4-6 PR、2-3 工作天),把本檔升級到 `docs/alerts-redesign.md`,刪除 `.omo/briefs/ALERT_SYSTEM_REDESIGN.md` 與 `.omo/briefs/alert-redesign-v2.md` 副本
- **關聯**: `docs/spikes/mcp-go-sdk-spike.md` (L163 提及 roadmap snapshot)
- **設計審計方法論**: user m00091 修正 — 必須逐個 verify 才能假定 work 是需要的