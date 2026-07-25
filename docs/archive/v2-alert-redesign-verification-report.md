# v2 alert-redesign 升級真實路徑 (post-verification 2026-07-04, three rounds)

> 設計 todo 基於 `.omo/briefs/alert-redesign-v2.md` Part 6 P0 優先序。經 2026-07-04 **三輪 grep 驗證** 後修正的真實路徑。
> 目標: 完成真實 gaps → 把 v2 working draft 升為正式 docs/ 規格。

## 1. 驗證結果摘要(三次修正後)

| P0 Item | 原始 v2 claim | 第 1 輪 (m00093) | 第 2 輪 (m00114) | 第 3 輪 (m00118-119) **最終** |
|---|---|---|---|---|
| **P0-1 Drawdown** | 缺 alert consumer | "REAL GAP - event emits, no consumer" | "WRONG - `drawdown_consumer.go` 已存在 (15L)" | "consumer CODE 完整, 但 `NewDrawdownConsumer` 只在 test file 呼叫, **production 從未 wiring**" → REAL GAP |
| **P0-2 Slippage** | 缺 alert consumer | "ALREADY DONE" | "CONFIRMED" | unchanged → DONE |
| **P0-3 Concentration** | 缺 alert rule | "PARTIALLY DONE" | "rules defined but unwired" | "rules 在 rules.go:114 + :186, 但 production code 沒呼叫 (只有 test refs)" → REAL GAP |
| **P0-4 Experiment** | 缺 event emit | "PARTIALLY DONE" | "EventExperiment not defined (grep filter issue)" | "Events DEFINED in eventbus + LifecyclePublisher 在 L53/55 publishes them, 但 **grep consumer 0 results**" → REAL GAP |

**最終結論**: v2 的 4 個 P0 中, **3 個是真實 gap, 只有 1 個 (P0-2 Slippage) 已 done**。

## 2. 建議 PR 清單(由小到大,獨立可合併)

### PR-1: Drawdown Consumer Wiring
- **目標**: 在 `cmd/atlas/main.go` 加上 `DrawdownConsumer` 的 instantiate + `Start()` 呼叫
- **現有**: `internal/monitoring/drawdown_consumer.go` 完整實作 (15L header `DrawdownConsumer subscribes to EventDrawdownBreach and persists each`), `c.sub = c.bus.Subscribe(eventbus.EventDrawdownBreach, c.handleEvent)` 已就位
- **缺**: production code 沒有呼叫 `NewDrawdownConsumer` (grep 結果只有 `drawdown_consumer_test.go:26/50/116/142`)
- **參考模式**: 抄 `trade_slippage_consumer.go` 的 wiring 位置(可能在 `cmd/atlas/main.go` 或 `internal/monitoring/server` 啟動時)
- **估時**: 2-4h(主要是找到正確 wiring 位置 + instantiate)
- **測試**: 整合測試 event publish → consumer → alert record

### PR-2: Concentration Rule Wiring
- **目標**: 把 `internal/monitoring/rules.go` L114 (`position_concentration`) + L186 (`high_position_concentration`) 從 rule definition 接到 rule engine 實際 firing on events
- **現有**: rules 已定義
- **缺**: 沒有 production code 訂閱或執行這些 rules (grep 結果: 只有 test refs in `rules_test.go` + `monitoring_test.go`)
- **方法**: 在 monitoring server 啟動時 register rules,並 subscribe 到對應 eventbus event
- **估時**: 4-8h
- **測試**: 整合測試 + 單元測試

### PR-3: Experiment Event Consumer
- **目標**: 新增 `internal/monitoring/experiment_consumer.go`(或其他合適位置)訂閱 `EventExperimentAccepted` / `EventExperimentRejected`
- **現有**: `LifecyclePublisher.TransitionAndPublish` 在 `internal/experiment/lifecycle_publisher.go:53-55` 透過 `PublishExperimentLifecycle` 發 event:
  ```go
  case experiment.ExperimentAccepted:
      p.bus.PublishExperimentLifecycle(eventbus.EventExperimentAccepted, payload, "info")
  case experiment.ExperimentRejected:
      p.bus.PublishExperimentLifecycle(eventbus.EventExperimentRejected, payload, "error")
  ```
- **缺**: grep for consumer returned **0 results** - 沒有人訂閱這兩個 events
- **邏輯**: rejected 過多時建 alert record
- **參考模式**: 抄 `trade_slippage_consumer.go` 結構
- **估時**: 4-8h
- **測試**: 整合測試

### PR-4 (升級執行): v2 lift to docs/alerts-redesign.md
- **前置**: PR-1/2/3 全部完成
- **動作**:
  1. 建立升級版 alert-redesign 規格(從 v2 內容,但更新所有 v2 過時引用)
  2. 刪除 `.omo/briefs/ALERT_SYSTEM_REdesign.md` (v1, 15468B, PR #756 故意保留)
  3. 刪除 `.omo/briefs/alert-redesign-v2.md` (v2 working copy, gitignored)
- **估時**: 2-4h

## 3. 排除項目(verified done)

| v2 Item | 當前狀態 (2026-07-04 多次驗證後) |
|---|---|
| P1-1: API 3 endpoints + sort param | `alert_api.go:99-103` `sortAlerts(filtered, sortParam)` 已實作 |
| P1-2 (參數部分): SLA Meta-Alert | `parameters.json:6739-6754` 已包含 `alert_sla_*` + `sla_violation_meta_alert` |
| P1-3 (parameter part): Channel heartbeat staleness | `parameters.json:6736` `channel_heartbeat_staleness_sec` 已存在 |
| P2-1: Decision 1 channel heartbeat | 同上 |
| P0-2: TradeSlippage consumer | `trade_slippage_consumer.go` 已實作且 wired |
| EventBus publish + payload structs | 4 個 events + payloads + `Publish*` functions 全部已實作 (`eventbus.go:96-279`, `1057`, `1068-1075`) |

## 4. 待驗證(留待 PR-1/2 執行階段順便)

- **P1-2 SLA Meta-Alert LOGIC**: 參數已有,缺少的可能是 consumer
- **P1-3 3-fail logic**: v2 提到但未驗證是否實作
- **P2-2/P2-3**: backfill items 狀態

## 5. Bucket B(roadmap 戰略項目,不在本 todo 範圍)

`roadmap-v2.md` Part 3 Bucket B 有 4 個更高戰略價值的 items:

| Item | 評估狀態(2026-07-04 二次驗證後) |
|---|---|
| **B1 Data Access Layer** | grep `DataAccess\|data_access` 0 results → **真的未實作** |
| **B2 Backtest-Live Consistency** | grep `backtest.*live\|paper_trading` 7 hits including `internal/autobacktest/runner.go` → **部分實作**(深度待評估,是否涵蓋 paper trading vs live trading 一致性待查)|
| **B3 HITL Regime** | grep `HITL\|HumanInTheLoop` 0 results → **真的未實作**(roadmap v2 提到的只是概念性註記) |
| **B4 Tax/Liquidity Sizing** | grep `tax.*siz\|liquidity.*siz` 3 hits including `internal/tax/tax_aware_sizing.go` + `internal/config/defaults_portfolio.go` + `docs/specs/taiwan-tax-spec.md` → **有實作**(深度待評估)|

需各自獨立設計 + 評估 + 排程。B1 尤其關鍵(影響所有下游 Backtest/Live/HITL/Tax work),應優先單獨評估。B2 與 B4 因有部分實作,評估範圍可縮小(只需確認現有實作是否涵蓋 roadmap v2 的目標)。

## 6. 時間預估(最終版)

| | v2 自報 | 第 1 輪 (m00093) 估計 | 第 2/3 輪 **最終** |
|---|---|---|---|
| P0-1 (Drawdown wiring) | 4-8h (1 PR) | "DONE" | 2-4h (PR-1) |
| P0-2 (Slippage) | 4-8h (1 PR) | DONE | DONE |
| P0-3 (Concentration wiring) | 8-12h (1-2 PR) | "partial" | 4-8h (PR-2) |
| P0-4 (Experiment consumer) | 8-12h (1-2 PR) | "partial" | 4-8h (PR-3) |
| **總計 P0** | **4-8 PR, 2-3 工作天** | **2-3 PR, 1-3 工作天** | **3 PR, 2-4 工作天** |

## 7. 設計審計教訓(從三輪驗證學到)

1. **不要相信 v2 自報的 filename**: v2 寫 `drawdown_breach_consumer.go`, 實際是 `drawdown_consumer.go` (我第一輪誤判需要 new consumer)
2. **不要相信 grep "0 results"**: 排除 filter 可能遮蔽真實結果 (我第三輪 grep EventExperiment 被 `internal/eventbus/` filter 排除)
3. **scope 應該包含 wiring 檢查**: rule/consumer 定義存在不代表 production 有用 (需 grep production code 找 instantiate 點)
4. **多輪驗證值得**: 三輪逐漸縮小範圍, 從 4 PR → 4 PR (real gaps) → 3 PR (correct)
5. **user m00091 + m00110 修正關鍵**: 「若要實作那些過去未實作的工作, 一定要檢查是否真的沒有做過」

## 8. 參考資料

- **v2 source**: `.omo/briefs/alert-redesign-v2.md` (gitignored working draft)
- **v1 (superseded but preserved)**: `.omo/briefs/ALERT_SYSTEM_REdesign.md` (15468B, PR #756 故意保留)
- **驗證日期**: 2026-07-04 (三輪 grep)
- **升級觸發條件** (v2 Part 9): "完成 P0 全部 4 個 PR 後(估 4-6 PR、2-3 工作天), 把本檔升級到 docs/alerts-redesign.md"
- **設計審計方法論**: user m00091 修正 (避免重複/衝突工作)