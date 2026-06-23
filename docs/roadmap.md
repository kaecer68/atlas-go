# Roadmap

## Phase 1: Foundation

- initialize standalone repository
- add GitHub workflow and templates
- define canonical domain types
- implement simple simulation engine
- support mock and replay market data providers

## Phase 2: Taiwan Replay MVP

- TWSE and TPEX daily adapters
- daily replay runner
- scorecard and performance ledger
- first sector and macro agent prompts
- sessionized replay artifacts

## Phase 3: OpenClaw Training Loop ✅

- agent registry
- prompt versioning
- rolling evaluation windows
- keep-or-revert experiment flow
- ledger-backed weakest-agent selection
- multi-session backtest windows
- intelligent mutation type selection (prompt_tightening, risk_rule_change, portfolio_constraint_revision)
- interactive mutation proposal scripts

## Phase 4: Near-Real-Time Paper Trading ✅

- Fugle snapshots ✅
- TWSE OpenAPI integration ✅
- Hybrid Provider (Fugle + TWSE fallback) ✅
- Live State Store ✅
- Event-driven orchestration ✅
- Monitoring and alerting ✅

## Phase 5: Portfolio Intelligence ✅

- Multi-factor portfolio optimization ✅
- Risk-adjusted position sizing ✅
- Dynamic regime-based allocation ✅
- Agent weighting system ✅
- Style rotation detection ✅
- Post-trade analysis ✅

## Wave 7.5 (2026-06) ✅

Shipped in v0.0.0.6:

- Risk gate safety wiring with explicit promotion limits and rejection reasons.
- Orphan config rejection for misplaced `parameters.json` files.
- Alertmanager webhook receiver hardening and field contract validation.
- Calibration metadata preservation across auto-rollback scenarios.
- Channel health improvements with per-channel failure reasons and fetch log persistence.
- `AutoJudgePromoter` wired into the atlas scheduler.
- `EventPromotionRecorded` event type delivered to dashboard clients via SSE with catch-up support.
- `GET /api/dashboard/fetch-log` endpoint for recent channel fetch events.

## Wave 8 (2026-06, **shipped in v0.0.0.7**)

Detailed plan: [`docs/wave-8-plan.md`](wave-8-plan.md)

目標：把 b29 audit 識別的 4 個 RED + 5 個 YELLOW 事件缺口實作成統一事件流（type 定義 + producer + SSE handler + frontend component）。

### Wave 8 收尾狀態（v0.0.0.7）

- **RED 事件完成度**：6/9 實作完成，3 個推遲（見下表）
- **YELLOW 事件**：5 個全數保留至 Wave 9 plan，未在本 wave 實作
- **三項前置決策（PD-1/PD-2/PD-3）**：PD-1（schema version）、PD-2（JSONL audit）、PD-3（節流）已分別於 eventbus.go、AnnotationStore、monitoring/service 落實

### Wave 8 RED（9 個事件，1 PR = 1 事件）

| # | 事件 | 狀態 | PR / 補充說明 |
|---|------|------|---------------|
| 1 | `RiskGateRejected` | ✅ 已合併 | #619 — BLOCK / HALT 路由 |
| 2 | `RiskGateOverride` → 重構為 `RiskGateOverridden` | ✅ 已合併 + Wave 8.2 收尾 | #620 + Wave 8.2 收尾 — REDUCE / ALERT_ONLY 路由 |
| 3 | `IndustryCalendarEvent` | ✅ 已合併 | #621 |
| 4 | `TradeSlippage` | ✅ 已合併 | #625（經 P3 編號對齊從 8.4 變 8.6） |
| 5 | `LLMAnnotatorCircuitOpen` | ⏸️ 推遲至 Wave 8.11+ | LLM 重構（PR #628/#629）改為 capability-based routing，原 circuit breaker 由 `llm_annotator:requests_good:rate5m` + alert rule `llm_annotator_availability_fast_burn` 取代 |
| 6 | `LLMAnnotatorFallbackUsed` | ⏸️ 推遲至 Wave 8.11+ | 同上，fallback 路徑由 router logs 與 metrics 揭露 |
| 7 | `LLMAnnotatorQuotaExceeded` | ⏸️ 推遲至 Wave 8.11+ | 同上，quota 控管整合進 router 計費 |
| 8 | `BacktestCompleted` | ✅ 已合併 | #622 |
| 9 | `CalibrationCompleted` | ✅ 已合併 | #623 |

> **Wave 8.10 Docs 收尾**（PR #627）：補寫 3 個既有事件 doc（narrative-event.md、health-alert.md、promotion-recorded.md）+ 更新 INDEX.md + P3 編號對齊（TradeSlippage 維持 8.6，LLM 事件預留 8.11+）。
>
> **Wave 8.2 收尾**（本批次）：補實作 `EventRiskGateOverridden` 常數，routing 改為三向 split（BLOCK/HALT → rejected、REDUCE/ALERT_ONLY → overridden、ALLOW → allowed）；補 `risk-gate-overridden.md` 文件 + 更新 `risk-gate-allowed.md` 反映新語意。

### Wave 9 YELLOW（5 個，**plan 已就緒，採 forward-compat 設計**）

**計畫檔案**：[`.omo/plans/wave-9-plan.md`](../.omo/plans/wave-9-plan.md)

**5 個事件**：`ChannelIndividualHealth`、`RegimeChangeConfirmed`、`FactorWeightRegression`、`DriftDetector`、`IngestionLagSpike`

**對應 VERSION**：v0.0.0.8（Wave 8 v0.0.0.7 已收尾）

**關鍵決策（2026-06-22）**：採 **路徑 1：完整 Wave 9 + forward-compat 設計**
- 5 個事件**全部**先做，不阻塞於 Issue #611
- 只讀既有 public API（`ChannelErrors()`、`OnRegimeChange`、`EventRegimeChange` 等）
- debouncer 與 drift 計算完全在 `internal/monitoring/service/` 層，#611 完成後 Wave 9 程式碼不需重做
- 3 個 PD-W9：info severity 預設、外部 debouncer 策略、Prometheus histogram metrics

**7 PR atomic breakdown**：
- Wave 9.0 infrastructure（5 個 EventType slot + 3 個 PD-W9 框架）
- Wave 9.1 `ChannelIndividualHealth`
- Wave 9.2 `RegimeChangeConfirmed`
- Wave 9.3 `FactorWeightRegression`
- Wave 9.4 `DriftDetector`
- Wave 9.5 `IngestionLagSpike`
- Wave 9.6 frontend 整合測試 + docs

**估時**：~7 工作天

### 依賴

- 上游：Wave 7.5 v0.0.0.6 ✅、Phase 2 CLI #610 修復 ✅（PR #618 收尾）
- 下游：Phase 4 Production Trading（Wave 8 SSE 完備）、refactor issue #611

### 邊界（嚴格）

- ✅ 可動：`internal/monitoring/api/events/`、`internal/monitoring/service/`、`web/services/sse.js`、`web/components/event-list.js`、`monitoring/rules/`、`docs/`
- ⛔ 不可動：`internal/llm/`、`internal/llm_annotator/`、`internal/narrative/`、`internal/spawning/`、`internal/orchestrator/`、`cmd/atlas/main.go` provider 區段

---

## Phase 6: Production Trading (Next)

- Decision chain transparency (FactorScores breakdown, ConvictionBreakdown, MacroEvent confidence) ✅
- NT$ currency formatter + after-tax equity curve + cost KPI cards ✅ (Phase 3)
- Experiment monetary NT$ display on evolution panel ✅ (Phase 3)
- Evolution panel chart-based UI upgrade (dual-curve chart, monetary trend visualization)
- Real broker integration
- Live order management
- Risk circuit breakers
- Performance reporting

## P-Infra: Infrastructure Foundation (Future)

Infrastructure improvements required before or during production hardening. Triggered alongside database migration (JSONL → SQLite/PostgreSQL).

| 項目 | 用途 | 不改善的風險 |
|------|------|-------------|
| **Structured logging** (slog/zap) | 360 處 `log.Printf` → JSON structured logs，支援 ELK/Loki 查詢、alerting | Production incident 排查從 30 分變 2 小時 |
| **Context propagation** | 127 處 `context.Background()` → 傳遞 parent context，配合 graceful shutdown | Live mode graceful shutdown 不完整，goroutine 洩漏 |
| **Data Access Layer** | 定義 `OutcomeStore`/`QuoteStore`/`SessionStore` interface，支援 JSONL→SQLite 遷移 | 將來換 DB 幾乎要重寫整個 ledger 層 |

**觸發時機**：當以下任一條件滿足時啟動
- 數據量增長導致 JSONL 效能瓶頸
- 需要多 consumer（如 dashboard、backtest）共用同一筆資料
- Production 環境需要 ELK/Loki 等 structured logging 工具

## Execution Roadmap (2026 Q2-Q4)

### Short Term (2-6 weeks)

- Freeze proposal/commit/event contracts and add validation checks
- Implement decision state machine with explicit transition guards
- Persist trace fields (`proposal_id`, `commit_id`, `approval_id`) in ledger artifacts
- Build guard pipeline v2 with auditable reject reasons
- Deliver first dashboard APIs for macro radar, agent observatory, and forecast-vs-reality

Exit criteria:
- Contract tests pass and are versioned
- Replay runs remain deterministic
- Every experiment record can be reconstructed end-to-end

### Mid Term (6-12 weeks)

- Add macro event ingestion pipeline with factor timeline snapshots
- Add parallel simulation scenarios (base, stress, shock)
- Wire human approval/revert flow into command path
- Introduce weekly governance review with acceptance scorecards

Exit criteria:
- Scenario comparisons are stable across repeated runs
- Human-in-the-loop checkpoints are enforced before promotion
- Guard outcomes are visible in monitoring panels

### Long Term (3-6 months)

- Integrate production broker adapter with strict circuit breakers
- Add SLO-driven operations dashboards and incident runbooks
- Run staged drills for rollback and degraded-data operation
- Promote to production with controlled capital ramp

Exit criteria:
- Staging drills pass with documented evidence
- Rollback completes within defined operational window
- Risk and audit controls satisfy production checklist
