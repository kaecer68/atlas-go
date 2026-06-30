# Atlas-go Roadmap v2（2026-06-26 重設計）

> **狀態**：草稿，待用戶確認。v1 仍保留於 `.omo/briefs/roadmap.md` 對照。
> **取代**：`.omo/briefs/roadmap.md`（已 86% 為歷史 + P-Infra 數字過時 + Execution Roadmap 純猜測 + 漏列 4 個金融工程缺口）
> **設計來源**：`RESEARCH-SUPPLEMENT.md` Part 3.1 + `VALIDATION-REPORT.md` Part 1
> **重設計原則**：
> 1. 已實作段落降級為 `[HISTORICAL]`，列出實際 PR # 供 audit
> 2. P-Infra 不寫會過時的數字，改為「集中點 + 觸發條件 + Data Access Layer 提前」
> 3. Execution Roadmap 改為**需求池**（Bucket A/B/C），不寫時間估算
> 4. 新增 4 個金融工程缺口章節（Trade Slippage / Position Concentration / Portfolio Drawdown / Experiment events）
> 5. 新增 Backtest-to-Live Consistency / HITL Regime / Tax-Liquidity Sizing 三個金融工程議題

---

## Part 1：[HISTORICAL] 已實作歷史

> 本段為「已完成」歷史快照。Phase / Wave 編號保留作為 audit reference。

### 1.1 Phase 1-5（核心引擎）

| Phase | 內容 | 狀態 | 關鍵模組 |
|-------|------|------|---------|
| Phase 1: Foundation | repo、workflow、domain types、sim engine、mock/replay provider | ✅ [HISTORICAL] | v0.0.0.x |
| Phase 2: Taiwan Replay MVP | TWSE/TPEX adapters、daily replay、scorecard、sector/macro prompts、sessionized artifacts | ✅ [HISTORICAL] | v0.0.0.x |
| Phase 3: OpenClaw Training Loop | agent registry、prompt versioning、rolling windows、keep-or-revert、weakest-agent selection、multi-session backtest、intelligent mutation、proposal scripts | ✅ [HISTORICAL] | internal/experiment + evolution |
| Phase 4: Near-Real-Time Paper Trading | Fugle snapshots、TWSE OpenAPI、Hybrid Provider、Live State Store、event-driven orchestration、monitoring/alerting | ✅ [HISTORICAL] | v0.0.0.x |
| Phase 5: Portfolio Intelligence | multi-factor optimization、risk-adjusted sizing、regime-based allocation、agent weighting、style rotation、post-trade analysis | ✅ [HISTORICAL] | internal/portfolio |

### 1.2 Wave 7.5（v0.0.0.6）

✅ [HISTORICAL]：Risk gate safety wiring、orphan config rejection、Alertmanager webhook hardening、calibration metadata preservation、channel health improvements、AutoJudgePromoter、EventPromotionRecorded、GET /api/dashboard/fetch-log

### 1.3 Wave 8（v0.0.0.7）

**RED 事件 6/9 完成**（PR 對照表）：
- ✅ #619 RiskGateRejected（BLOCK / HALT routing）
- ✅ #620 RiskGateOverridden + Wave 8.2 收尾（REDUCE / ALERT_ONLY routing + 3-way split）
- ✅ #621 IndustryCalendarEvent
- ✅ #625 TradeSlippage
- ❌ 推遲至 Wave 8.11+：LLMAnnotatorCircuitOpen / FallbackUsed / QuotaExceeded（被 LLM 重構 capability-based routing + router logs / metrics 取代）
- ✅ #622 BacktestCompleted
- ✅ #623 CalibrationCompleted

**Wave 8.10 Docs**（PR #627）：3 個既有事件 doc + INDEX.md + P3 編號對齊
**Wave 8.2 收尾**：EventRiskGateOverridden 常數 + 3-way split routing

### 1.4 Wave 9（v0.0.0.16/17/18，觀測性）

5 個事件全部上線（PR #695-#700 + v0.0.0.18 gap fixes PR #704）：
- EventPositionUpdate（PR #696）
- RegimeChangeConfirmed + IngestionLagSpike + FactorWeightRegression + DriftDetected v2（PR #697）
- BaselineTrigger（PR #698）
- v0.0.0.18 PR #704：dashboard buffer catchup（live bus 補訂閱）、partial-failure cleanup（LIFO + reference clear）、audit subscriber idempotency、DriftDetector v2 integration tests

### 1.5 Wave 10 L2.x（v0.0.0.19-21）

- v0.0.0.19：OTLP production pipeline（PR #714 + #715，OTel collector + TimescaleDB hypertable）+ pluggable acceptance 17/17（PR #717）
- v0.0.0.20a：AgentLoop state machine correctness（Issue #711 #5/#6/#9，Round-based exhaustion + error return）
- v0.0.0.21：Wave 11 L2.1 doc audit closure（Issue #720/#722） + LLM sector agent wiring（Issue #719, PR #734）
- Unreleased: spec-alignment follow-up to PR #743 + RunToolCall → llm.SafeInvokeHandler wiring

### 1.6 Phase 6 真實狀態盤點

| 子項 | 真實狀態 | 條件 / 限制 |
|------|---------|-------------|
| Decision chain transparency (FactorScores) | ✅ 已實作 | internal/orchestrator/ |
| NT$ currency formatter | ✅ 已實作 | shared_web/static/js/shared/ |
| Experiment monetary NT$ | ✅ 已實作 | evolution panel |
| Evolution panel chart-based UI upgrade | ⚠️ 部分實作 | Canvas chart 有，dual-curve 待驗證 |
| Real broker integration | ✅ 已實作（有條件）| `allow-real-signer` flag 預設關閉 |
| Live order management | ✅ 已實作（有條件）| 同上 |
| Risk circuit breakers | ✅ 已實作 | internal/live/circuit_breaker.go |
| Performance reporting | ✅ 已實作 | Wave 9 observability wire |

**結論**：Phase 6 多數已實作。「Real broker / Live order」是「replay-first safety」**有意的條件限制**，不是「未完成」。

### 1.7 Issue #611 解除確認

✅ **CLOSED**（9 個 sub-issue 全部完成，PR #676 ~ #691 多次合併）

**已完成 refactor**：
- `internal/portfolio/optimizer*.go` 拆 5 檔（optimizer.go + optimizer_drawdown.go + optimizer_frontier.go + optimizer_math.go + optimizer_pipeline.go）
- `internal/orchestrator/system*.go` 拆多檔（從 1772 行縮為 1017 行 + system_dispatcher.go + system_plugins.go + system_risk_session.go + system_record_summary_retry.go 等）
- `internal/portfolio/factor_engine.go`（1289 → 12 行，#691）
- `internal/orchestrator/executors.go`（1284 → 30 行，#684）
- `internal/portfolio/factor_weight_engine.go:79` 提供 `GetWeights(regime string)` 介面

**影響**：**Wave 9 forward-compat 設計完全成立**——Wave 9 程式碼不需重做。`FactorWeightEngine.GetWeights(regime)` 介面已備，DriftDetector v2 可正常運作。

---

## Part 2：P-Infra: Infrastructure Foundation（重新框架）

### 2.1 觸發條件（取代「達到 X 處就啟動」）

啟動 P-Infra 任一項的充分條件（任一滿足即可）：

| 條件 | 說明 | 當前狀態 |
|------|------|---------|
| 數據量增長導致 JSONL 效能瓶頸 | sync 任務執行時間 > 5 分鐘 | 未達 |
| 需要多 consumer 共用同一筆資料 | 第 3 個 consumer 撞到 IO lock | 未達 |
| Production 環境需要 structured logging | ELK / Loki / Datadog 任一被要求 | 未達 |
| **新增消費者需要改 ledger raw IO** | 任何新功能需要直接讀 JSONL | **已是現實** |

### 2.2 集中點分析（取代會過時的數字）

**`log.Printf` 分布**（grep top 5，2026-06-26）：
- `cmd/atlas/main.go` — 76 處（CLI 層，非核心引擎）
- `cmd/atlas/cmd_universe.go` — 31 處
- `cmd/migrate-data/main.go` — 28 處
- `cmd/atlas/calibration_tasks.go` — 20 處
- `cmd/atlas/operations_tasks.go` — 14 處

**`context.Background()` 分布**：1237 處，主要在測試 / 工具程式，不影響 production 路徑

**結論**：
- 結構化 logging 重構應**從 `cmd/atlas/` 層切入**（200+ 處集中），非整個 codebase
- `context.Background()` 重構優先級低（不影響 production，測試路徑不敏感）
- **真正的瓶頸是 Data Access Layer 缺乏介面**（見 2.5）—— 這已是當前瓶頸

### 2.3 Structured Logging（slog / zap）

**範圍**：從 `cmd/atlas/` 200+ 處 `log.Printf` 切入

**理由**：CLI 層是邊界，影響最小；核心引擎（`internal/orchestrator/`、`internal/sim/` 等）已用 `slog.Info`

**Acceptance**：
- `cmd/atlas/` 內 `log.Printf` 為 0
- 所有 log 走 `logging.Info/Warn/Error` 套件（已存在於 `internal/logging/`）
- 結構化欄位：`event`、`component`、`symbol`、`latency_ms` 等依事件而定

### 2.4 Context Propagation

**範圍**：`internal/live/`、`internal/orchestrator/` 的 graceful shutdown 路徑

**Acceptance**：
- top-level function 100% 接收 `ctx context.Context`
- graceful shutdown 路徑中 `ctx` 可正確 cancel

### 2.5 Data Access Layer（**提前**，不應等觸發條件）

> **重大決策**：從原 P-Infra 的「等 DB 瓶頸才做」改為「**現在就做**」。

**現狀問題**：
- `internal/ledger/` 直接讀寫 JSONL
- `internal/repository/` 提供部分 PostgreSQL 介面但未覆蓋全部
- 新增功能（如 dashboard export、third-party consumer）需要直接讀 JSONL，撞到 IO lock
- 每個 PR 都在碰 raw JSONL IO，**已是當前瓶頸**
- 從「個人專案」變「可被他人審計的系統」的瓶頸

**目標介面**：

```go
// internal/ledger/store.go（新增）
type OutcomeStore interface {
    AppendOutcome(outcome domain.Outcome) error
    LoadOutcomes(sessionID string) ([]domain.Outcome, error)
    LoadOutcomesInRange(from, to time.Time) ([]domain.Outcome, error)
}

type QuoteStore interface {
    AppendQuote(quote domain.Quote) error
    LoadQuotes(symbol string, from, to time.Time) ([]domain.Quote, error)
}

type SessionStore interface {
    SaveSessionSummary(summary domain.SessionSummary) error
    LoadSessionSummary(sessionID string) (domain.SessionSummary, error)
    LoadAllSessionSummaries() ([]domain.SessionSummary, error)
}
```

**Acceptance**：
- 3 個 interface 定義完成（`OutcomeStore` / `QuoteStore` / `SessionStore`）
- JSONL 與 PostgreSQL 各有實作
- `internal/ledger/` 內所有 raw IO 改為透過 interface
- 新增 consumer 只需換實作，不改呼叫端

**Owner 條件**：需要熟悉 `internal/ledger/` + `internal/repository/` 兩個 module

---

## Part 3：需求池（取代 Execution Roadmap）

> **重設計理由**：原 §Execution Roadmap 寫「Short 2-6 weeks / Mid 6-12 weeks / Long 3-6 months」這類時間估算沒有依據。本次改為需求池，由 owner 領取，不寫時間。

### 3.1 Bucket A：金融工程缺口（已驗證，建議最優先）

> 詳見 Part 4 + `alert-redesign-v2.md` Part 3（每個 Decision 的現有實作 / 缺口 / 金融工程重要性 / 實作難度）

| ID | 項目 | 對應 alert-redesign-v2 Decision | 實作難度 | 預估工作量 |
|----|------|--------------------------------|---------|----------|
| A1 | Trade Slippage Anomaly Alert | Decision 6 | LOW | 4-8 hr |
| A2 | Position / Sector Concentration Alert | Decision 7 | MEDIUM | 8-12 hr |
| A3 | Portfolio Drawdown Alert | Decision 8 | LOW | 4-8 hr |
| A4 | Experiment events lifecycle（Decision 5 反轉）| Decision 5（反轉）| MEDIUM | 8-12 hr |

**共同特性**：事件或參數已存在，**只缺 alert rule**——實作成本 LOW，金融工程風險降低價值 HIGH

### 3.2 Bucket B：規劃中（已通過初步評估，未排程）

| ID | 項目 | 背景 | Owner 條件 |
|----|------|------|----------|
| B1 | Data Access Layer 介面 | 見 Part 2.5 | 熟悉 ledger + repository |
| B2 | Backtest-to-Live Consistency Gap | 見 Part 5 | 熟悉 replay + live broker |
| B3 | Human-in-the-loop Regime Override | 見 Part 6 | 熟悉 regime detection + orchestrator |
| B4 | Tax / Liquidity-aware 倉位 sizing | 見 Part 7 | 熟悉 portfolio + tax |

### 3.3 Bucket C：架構性（長期框架性工作）

| ID | 項目 | 阻塞 |
|----|------|------|
| C1 | Production broker gradual ramp | B1、B2 |
| C2 | SLO dashboards + incident runbooks | Decision 9（alert-redesign-v2） |
| C3 | Staged drills for rollback + degraded-data operation | C1、C2 |

---

## Part 4：金融工程缺口專章

> 詳細 Decision 內容見 `alert-redesign-v2.md` Part 3。本段為 roadmap 視角的對應說明。

### 4.1 Trade Slippage（A1 / Decision 6）

**現有實作**：`EventTradeSlippage` 事件 + payload schema + `PublishTradeSlippage` 函式（eventbus.go:89/936）已實作，但**無 alert consumer**

**金融工程重要性**：
- 實交易風控核心
- retail investor 對 slippage 極敏感
- 影響「實際報酬 vs 預期報酬」差距

**為什麼放 roadmap**：Trade Slippage 是 **decision chain** 的閉環——slippage 過大會觸發 trade abort 或 sizing 縮小，這影響 orchestrator 而非純 alert

### 4.2 Position / Sector Concentration（A2 / Decision 7）

**現有實作**：`parameters.json:alert.max_position_weight_pct=0.15`、`max_positions_count=20`、`sector_concentration_threshold`、`ConcentrationScore`、`macro_aware_drawdown.go:403` 已使用，但**無 alert rule**

**金融工程重要性**：
- retail investor 最常見失敗模式之一（單押 TSMC / 半導體業）
- 比 channel health 重要 100x
- 已是業界標準風控指標

**為什麼放 roadmap**：倉位集中會影響 portfolio rebalance 決策，需要與 portfolio optimizer 整合

### 4.3 Portfolio Drawdown（A3 / Decision 8）

**現有實作**：`EventDrawdownBreach` 事件 + `concrete_rules.go:38` "portfolio drawdown >10%" + `MaxDrawdown: 0.08`，但**無 alert consumer + 無 publish 點**

**金融工程重要性**：
- retail investor 最關心指標
- 8% / 15% 已是業界標準
- 已有規格但無 alert rule

**為什麼放 roadmap**：drawdown 是 portfolio-level 指標，需要 portfolio snapshot 整合

### 4.4 Experiment events（A4 / Decision 5 反轉）

**現有實作**：`TransitionExperimentStatus(... ExperimentRejected/Accepted)` 已實作（judge.go:188/195/202/211），但**無 event emit**

**金融工程重要性**：
- baseline promotion 直接影響倉位與資金
- audit trail 比 log 強
- retail investor 需要知道「倉位被自動調整」

**為什麼放 roadmap**：experiment flow 影響 baseline 政策 → portfolio 政策 → 倉位，是關鍵 decision chain

---

## Part 5：Backtest-to-Live Consistency Gap（新增）

> **v1 完全沒提，但這是 retail investor 最痛的議題**

### 5.1 問題

replay backtest sharpe 2.0、live mode sharpe 0.5，這個 gap 怎麼衡量？怎麼 reconcile？

**常見 gap 來源**：
1. **Slippage**：backtest 固定 4 bps，live 實際 10-50 bps
2. **Fill rate**：backtest 假設 100%，live 60-90%
3. **Regime drift**：backtest 歷史 regime，live 即時 regime
4. **Data latency**：backtest 當日收盤，live 盤中 tick
5. **Order rejection**：backtest 不模擬券商 reject，live 有

### 5.2 建議方案

1. **每次 live trade 結束後，計算「backtest 同條件下的預期 vs 實際」對比**
2. **新增 BacktestLiveDriftReport**：每月一次，比較 sharpe / hit rate / max drawdown
3. **Drift > 30% 自動降級**：live 切回 replay，等待 manual review

**Owner 條件**：熟悉 backtest 引擎 + live broker + statistics

**Acceptance**：
- BacktestLiveDriftReport 每月自動產生
- 報表含 sharpe / hit rate / max drawdown / slippage 4 個指標
- Drift > 30% 觸發切回 replay + manual review task

---

## Part 6：Human-in-the-loop Regime Override（新增）

> **v1 完全沒提，但 regime 切 RISK_OFF 影響「生計」（= 不交易 = 沒收入）**

### 6.1 問題

當市場 regime 從 RISK_ON 切到 RISK_OFF，**零售投資人需要介入確認**：
1. RISK_OFF = 不交易 = 影響生計
2. 自動切可能錯（regime detection 有 lag）
3. retail investor 對「不交易」的容忍度比機構低

### 6.2 建議方案

1. **Regime 切換需 manual approve**（除非連續 2 天確認）
2. **Dashboard 顯示「regime change 待 review」banner**
3. **Approve / Override 按鈕**：approve 採用 / override 維持前一 regime 24 小時
4. **每次 override 寫入 audit log**（供未來 review）

**Owner 條件**：熟悉 regime detection + dashboard + audit log

**Acceptance**：
- Regime 切換預設 manual approve
- Override 路徑有 audit log
- 自動切條件：連續 2 天確認 + 波動度 > 閾值

---

## Part 7：Tax / Liquidity-aware 倉位 sizing（新增）

> **v1 完全沒提，但這影響實際報酬**

### 7.1 問題

倉位 sizing 應考慮：
1. **交易成本**：手續費 + 證交稅 + 滑價
2. **稅**：台灣股票 ETF 交易稅 + 個人所得稅
3. **流動性**：TWSE 個股流動性差異極大（台積電 vs 小型股）

### 7.2 建議方案

1. `internal/config/defaults_portfolio.go` 新增 `transaction_cost_bps` 與 `tax_pct` 參數
2. 倉位 sizing 計算時扣除 transaction cost + tax
3. 小型股（流動性分數 < 0.3）強制縮小倉位（已有 partial 實作：`LiquidityLowThreshold: -0.3`）
4. Liquidity-aware 入場訊號：低流動性個股需要更強 signal 才進場

**Owner 條件**：熟悉 portfolio + config + tax

**Acceptance**：
- 倉位 sizing 函式接收 `transaction_cost_bps` 與 `tax_pct` 參數
- 小型股倉位上限自動縮減

---

## Part 8：升級時機

完成 Bucket A 全部 4 個 PR 後（估 5-8 PR、3-5 工作天），把本檔升級到 `docs/roadmap.md`，刪除 `.omo/briefs/roadmap.md` 與 `.omo/briefs/roadmap-v2.md` 副本。

---

## 附錄 A：本檔與 v1 對照

| v1 段落 | v2 對應 | 改動 |
|---------|---------|------|
| Phase 1-5 | Part 1.1 | 加 `[HISTORICAL]` 標記 + 關鍵模組 |
| Wave 7.5/8/9 | Part 1.2-1.4 | 加 `[HISTORICAL]` + PR # |
| Wave 10 L2.x | Part 1.5 | 加 `[HISTORICAL]` |
| Phase 6 | Part 1.6 | 改為「真實狀態盤點」+ 條件說明 |
| P-Infra | Part 2 | 重新框架為「集中點 + 觸發條件 + Data Access Layer 提前」 |
| Execution Roadmap | Part 3 | 改為「需求池」Bucket A/B/C |
| （缺）| Part 4 | 新增 4 個金融工程缺口專章 |
| （缺）| Part 5 | 新增 Backtest-to-Live Consistency Gap |
| （缺）| Part 6 | 新增 HITL Regime Override |
| （缺）| Part 7 | 新增 Tax/Liquidity-aware sizing |
| Issue #611 引用 | Part 1.7 | 新增解除確認（CLOSED）|