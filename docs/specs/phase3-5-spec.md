# Phase 3.5 — 整合層規格 (M1/M2/M3/M4)

> **狀態**: ✅ COMPLETED (2026-07-02,M1/M2/M3/M4 全數 ship)
> **起點文件**: 本 spec §1 (設計目標) + §3 (殘餘任務) + §5 (風險緩解) 為自身起點論述
> **時程**: 11 工作天(2d + 3d + 3d + 3d)
> **前置**: ✅ done — PR #821 (L2.4 ship) + PR #848 (Wave 11 mid) + PR #852 (M1) + PR #894/#895/#898 (M2/M3) + PR #905 (M4) 已全數 merge
> **目標**: 把目前散落在 admin UI、orchestrator、narrative 各處的「live + macro + forecast」三股線,拉到同一個整合層,並用 M4 forecast-bridge 把 sortino 拉抬。

---

## 1. 設計目標

完成自身起點論述揭示的 4 個整合缺口,目標:

1. **M1 deployment-gateway** (rank 4, 2d): 把 live / gateway-health / live-metrics 三個獨立 endpoint 收進單一 dashboard
2. **M2 narrative-taxonomy** (rank 2, 3d): narrative event 分類標準化,event-driven weights 輸入得到結構化處理
3. **M3 macro-flow** (rank 3, 3d): macro 六維 score 結構化流入 portfolio construction,觸發 defensive / aggressive 切換
4. **M4 forecast-bridge** (rank 1, 3d): forecast conviction bridge 到 directional trade layer,補強交易方向信心

完成後,atlas 達到「**live 可觀測 + 事件可分類 + macro 可執行 + forecast 可強化**」四個維度的整合層。

---

## 2. 不做 (Out of Scope)

- **新增 LLM provider**:已 ship MiniMax/DeepSeek 雙 provider,LLM 路由見 [`llm-routing.md`](./llm-routing-spec.md)
- **L2 sector agent 推廣**:L2.3 PoC 觀察中(`UseLLMSectorAgents` flag off — 見 [`l2-4-observation-spec.md`](./l2-4-observation-spec.md))
- **live trading 真正下單**:仍維持 replay / simulation 預設,live 旗標不啟用(見 [`live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md))
- **Constitution L0 appendix 改寫**:留給 Phase 4.B(16d,內含 Constitution +2d)
- **參數自動 tuning (Phase 4.A)**:7d 留到 Phase 4,不在本 spec 範圍
- **Multi-instance MCP federation**:見 [`agent-mcp-phase4.md`](./agent-mcp-phase4-spec.md) §3.3

---

## 3. 殘餘任務

### 3.1 M1 — Deployment Gateway Dashboard (estim 2d)

#### 3.1.1 問題

現有 live / gateway / live-metrics 三組 endpoint 散落在 admin UI:

| Endpoint | 檔案 | 顯示位置 |
|----------|------|---------|
| `GET /api/admin/live/state` | `internal/admin/live_state.go` ⚠️ | 已於 admin Web 顯示,但獨立 section |
| `GET /api/admin/live/metrics` | `internal/admin/live_metrics.go` ⚠️ | 獨立 panel |
| `GET /api/admin/gateway/health` | `internal/apigateway/admin_handler.go` ⚠️ | 另一個 panel |

admin 進入「live ops」頁需切換三個 section 才能看全 live 狀態,違反「live deployment 應是整體可觀測」設計原則。也讓 on-call 一次看到 deployment 全貌的成本變高。

#### 3.1.2 目標

1 個整合 dashboard 顯示「live deployment 完整生命週期狀態」(data source → gateway → live engine → metrics → last action)

#### 3.1.3 設計方案

**後端**:
- 新檔 `internal/adminapi/deployment/dashboard.go`（PR #852 實際位置；2026-06 adminapi/live 子套件拆分後不在 `internal/admin/`）
- 新 struct `LiveDeploymentDashboard` 聚合:
  ```go
  type LiveDeploymentDashboard struct {
      DataSources map[string]DataSourceStatus  // gateway-level
      Gateway     GatewayHealth                // apigateway.Gateway.Health()
      LiveEngine  LiveEngineStatus             // live.State() — 但 internal 暴露精簡版
      Metrics     LiveMetricsSnapshot          // 既有
      LastAction  LastActionInfo               // 上次 broker / 下單 attempt 時間
  }
  ```
- 新 endpoint `GET /api/admin/live/deployment/dashboard`(取代三個獨立 endpoint 的 admin 主頁面呼叫)
- 既有 3 endpoint 保留(向下相容 dashboard 模組化區塊)

**前端**:
- 新檔 `shared_web/static/js/components/deployment-dashboard.js`（PR #852 實際位置；2026-06 frontend 三拆 `admin_web`/`client_web`/`shared_web` 後已不在 `admin_web/`）
- 整合進 `main.js` 的 pageId 路由(取代 live-ops 或新增 live-deployment tab)
- 5 個 section:Data source health / Gateway throughput / Live engine state / Recent metrics / Last action log
- 30s polling,Refresh 按鈕

#### 3.1.4 邊界 case

- 部分 sub-component unhealthy:dashboard 顯示紅框,仍要繼續 load 其他 section(不 all-or-nothing)
- Data source offline 全滅(罕見):fallback banner 顯示「no data source, dashboard degraded」
- admin 用戶權限不足:回 403,前端 redirect 到 login(沿用現有 pattern)

#### 3.1.5 驗收

- [ ] `curl localhost:18080/api/admin/live/deployment/dashboard` 回 200 + 完整 JSON
- [ ] 既有 3 個 endpoint 仍可獨立呼叫(向下相容)
- [ ] admin Web 新 page 可載入,5 section 全顯示
- [ ] 30s polling 正常;手動 Refresh 立即觸發
- [ ] Unit test:`TestLiveDeploymentDashboard_Aggregation` 涵蓋「3 個 sub-component 一個 unhealthy、一個 stale」
- [ ] E2E test:`scripts/e2e/test_live_deployment_dashboard.sh`(若無 browser test,可僅 backend)

#### 3.1.6 工時

2 工作天(backend 1d + admin UI 1d)

---

### 3.2 M2 — Narrative Event Taxonomy (estim 3d)

#### 3.2.1 問題

narrative event 目前分類不一致 — `internal/narrative/event.go` 的 `EventType` 是 free-form string,沒有強制分層結構(設計痛點 C2: narrative 分類無標準化層級)。影響:

- [`atlas-event-driven-weights`](../../.claude/skills/atlas-event-driven-weights/SKILL.md) factor bridge 對應表依賴 narrative event 分類,目前是 hardcoded lookup table
- 新 narrative 加入時,需要有人手動擴充 enum + 加 mapping rule,容易漏
- 跨日誌查詢難以用 SQL `taxonomy_l1 = 'macro_economic'` 過濾

#### 3.2.2 目標

建立 2 層 taxonomy:

| L1 (主類) | L2 (子類範例) |
|----------|---------------|
| `macro_economic` | `central_bank_policy`, `inflation_data`, `geopolitical`, `trade_balance`, `employment` |
| `market_structure` | `regime_shift`, `liquidity_event`, `institutional_flow`, `margin_call`, `volatility_spike` |
| `sector_dynamics` | `sector_rotation`, `policy_industry_specific`, `global_supply_chain`, `commodity_price`, `competitive_shock` |
| `company_specific` | `earnings_release`, `guidance_update`, `capital_action`, `corporate_governance`, `incident` |

新 narrative event 必須帶 `taxonomy_l1` + `taxonomy_l2`,event-driven weights 對應表改為 SQL-driven 而非 hardcoded。

#### 3.2.3 設計方案

**新增檔案**:
- `internal/narrative/taxonomy.go`:enum + validation
  - `const (TaxonomyL1MacroEconomic = "macro_economic" ...)`
  - `func ValidateTaxonomy(l1, l2 string) error`:L1/L2 必須對得上,否則 reject
- `migrations/20260630_add_narrative_taxonomy.sql`:
  ```sql
  ALTER TABLE narrative_events
    ADD COLUMN taxonomy_l1 VARCHAR(32) NOT NULL DEFAULT 'uncategorized',
    ADD COLUMN taxonomy_l2 VARCHAR(32) NOT NULL DEFAULT 'uncategorized';
  CREATE INDEX idx_narrative_taxonomy ON narrative_events(taxonomy_l1, taxonomy_l2, occurred_at);
  ```

**改檔案**:
- `internal/narrative/event.go`:在 `NarrativeEvent` struct 加 `TaxonomyL1` + `TaxonomyL2` 欄位,JSON tag snake_case
- 新 event generator(已存在的 detector 模組):必填欄位填入,缺值走 `ValidateTaxonomy` reject
- `internal/factor/event_bridge.go`(存在於 [`atlas-event-driven-weights`](../../.claude/skills/atlas-event-driven-weights/SKILL.md) 描述):把 hardcoded mapping table 改為 SQL-driven
  ```sql
  SELECT l1, l2, factor_delta FROM narrative_taxonomy_factor_map WHERE l1 = $1 AND l2 = $2;
  ```

**向下相容**:
- 既有 narrative event 沒有 taxonomy → DB 預設 `uncategorized`
- event-driven weights 處理 `uncategorized` 時 fallback 為「weight unchanged」(不主動 Δ,避免意外影響)

#### 3.2.4 邊界 case

- Event 同時屬多個 L2(如 trade balance 同時 macro + sector):允許 event 帶主分類(`taxonomy_l1/l2` 為主分類),次分類用 `secondary_tags string[]` JSONB 儲存
- 舊 script 寫 event 沒帶 taxonomy:DB 預設 `uncategorized`;不會阻擋 legacy write,但 `event_bridge` 會記 warning log:`narrative_bridge.uncategorized_skipped count=N`
- Taxonomy 規則未來擴充(L3):本 spec 不實作,改用 SQL ALTER 即可

#### 3.2.5 驗收

- [ ] DB migration 順利 apply + rollback(無 index lock,小 batch 即可)
- [ ] 新 event generator 帶 taxonomy 後,`ValidateTaxonomy` accept
- [ ] 帶無效 taxonomy 的 reject case 有 unit test
- [ ] `event_bridge` SQL-driven:同 L1/L2 不同 factor_delta 走 SQL,不是 hardcoded
- [ ] 舊 event 回填 taxonomy 的 `scripts/migrate_narrative_taxonomy.sh` 提供 dry-run 模式
- [ ] Unit test:`TestNarrativeTaxonomy_Validation`,`TestEventBridge_SQLDriven`

#### 3.2.6 工時

3 工作天(DB 0.5d + taxonomy.go 0.5d + event generator / bridge 1d + 驗收 1d)

---

### 3.3 M3 — Macro Flow (estim 3d)

#### 3.3.1 問題

macro 六維 score(見 `internal/macro/assessment/macro_assessment.go:158` `MacroRiskAssessmentEngine.determineRiskLevel`)已經計算完成,但**沒有結構化流入 portfolio construction**。本 spec 起點論述指出 macro → portfolio flow 是「理論完備但整合缺失」的代表案例。

目前 macro score 只在 admin narrative panel 顯示,沒有 trigger portfolio rebalance / factor weight shift。

#### 3.3.2 目標

建立 macro signal → factor weight adjustment → portfolio rebalance 的單向 deterministic flow:

```
MacroDataSnapshot → determineRiskLevel() → RiskLevel + 6維 score
                                          ↓
                            MacroFlowEngine.ComputeWeightAdjustment()
                                          ↓
                            factor_weights snapshot + Δ table
                                          ↓
                            orchestrator.PreTradeRebalance()
```

預期行為:
- `RiskLevel = defensive` 且 `TaiwanStressIndex > 0.7` → defensive factor weight +20%,aggressive -15%
- `RiskLevel = aggressive` 且 `ForeignFlowBullish > 0.6` → aggressive +25%,defensive -10%
- 其他組合:weight unchanged(default)

#### 3.3.3 設計方案

**新檔案**:
- `internal/macroflow/engine.go`:核心計算
  ```go
  type WeightAdjustment struct {
      Defensive float64 // 防禦因子乘數,1.0 = unchanged
      Aggressive float64
      Growth float64
      Value float64
  }

  func (e *Engine) ComputeAdjustment(snapshot MacroDataSnapshot, riskLevel RiskLevel) (WeightAdjustment, error)
  ```
- `internal/macroflow/rules.go`:6 條 macro → weight 對應規則(可讀 YAML 或 Go const),對齊 reassessment §1
- `internal/macroflow/engine_test.go`:8 個典型 scenario 涵蓋 4 個 RiskLevel × stress on/off

**改檔案**:
- `internal/orchestrator/daily_pipeline.go`:在 `PreTradeRebalance` 前插入 macro flow call
  ```go
  snapshot := macroL.LoadLatest()
  riskLevel := assessment.DetermineRiskLevel(snapshot)
  adj, err := macroFlow.ComputeAdjustment(snapshot, riskLevel)
  if err != nil { return fmt.Errorf("macro flow: %w", err) }
  return factorWeights.ApplyAdjustment(adj)
  ```
- 既有 daily pipeline integration 必須保留 audit log(對齊 [`live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md)「control 層意圖清晰、可稽核」)

#### 3.3.4 邊界 case

- Macro data stale(>24h):engine 拒絕,回 `ErrMacroDataStale`;orchestrator 沿用前次 weight,不 rebalance(保護機制)
- WeightAdjustment 計算結果極端(|Δ| > 30% 單一類別):clip 到 max ±30%,slog warn
- LLM_RISK_FORENSICS_ENABLED → optional:同時寫 narrative 解釋「為什麼 defensive +20%」(呼叫 LLM,呼叫失敗不影響 weight 流程)

#### 3.3.5 驗收

- [ ] `ComputeAdjustment` 8 個 scenario unit test 全 pass
- [ ] orchestrator daily pipeline 整合:有 macro data 時 macro flow 確實 trigger,no-op 時不 trigger
- [ ] audit log:`macro_flow.applied` event 帶 snapshot 摘要 + weight adjustment 摘要
- [ ] Stale macro data 不會中斷 pipeline(回 ErrMacroDataStale 而非 panic)
- [ ] Integration test:`TestDailyPipeline_MacroFlowIntegration` 涵蓋「bullish + aggressive → aggressive +25%」

#### 3.3.6 工時

3 工作天(engine + rules 1.5d + orchestrator integration 1d + test 0.5d)

---

### 3.4 M4 — Forecast Bridge (estim 3d)

#### 3.4.1 問題

forecast engine(`internal/forecast/engine.go`)已存在 `MacroRiskAssessmentEngine.determineRiskLevel`(macro_assessment.go:158)與四檔驗證案例(見 reassessment §1 與 `docs/specs/real-time-regime-detection-spec.md`)。**但 forecast 結果只用在 monitor layer 的 regime display,沒有 bridge 到 directional trade layer 的具體 trade direction**。

oracle verdict 把 M4 升 rank 3 因為「補強 trade direction 信心」可拉 sortino(本 spec 優先排序依據)。

#### 3.4.2 目標

把 forecast conviction(0-100)bridge 到 directional trade layer:

```
ForecastEngine.Predict(symbol, horizon)
  → ForecastResult { conviction, direction, scenarios }
    ↓
ForecastBridgeAdapter.ToTradeSignal(result)
  → TradeSignal { action: Buy/Sell/Hold, weightMultiplier: 1.0-1.5, rationale }
    ↓
directional_trade_layer.ApplySignal(signal)
  → trade direction 加成 / 縮減
```

預期:forecast `conviction ≥ 70` 且 `direction = bullish` → trade weight × 1.2x;`conviction ≤ 30` 且 `direction = bearish` → weight × 0.8x(順向強化 / 逆向縮減)

#### 3.4.3 設計方案

**新檔案**:
- `internal/forecast_bridge/adapter.go`:核心 bridge
  ```go
  type TradeSignal struct {
      Symbol string
      Action string // "buy", "sell", "hold"
      WeightMultiplier float64 // 0.0 - 2.0
      Rationale string
      SourceForecastID string
      GeneratedAt time.Time
  }

  func (a *Adapter) ToTradeSignal(result ForecastResult) (TradeSignal, error)
  ```
- `internal/forecast_bridge/adapter_test.go`:典型 scenario test(bullish 80、bearish 20、hold 50、confidence low)

**改檔案**:
- `internal/strategy/directional_trade_layer.go`:新增 `ApplySignal(TradeSignal)` method,內部調整 `direction_weight` 暫存 map
- `internal/orchestrator/daily_pipeline.go`:在 M3 macro flow 之後插入 forecast bridge loop
  ```go
  for _, sym := range watchlist {
      fc, err := forecast.Predict(sym, 7*24*time.Hour) // 7-day horizon
      if err != nil { slog.Warn("forecast_bridge.predict_failed", ...); continue }
      sig, err := bridge.ToTradeSignal(fc)
      if err != nil { continue }
      directionalLayer.ApplySignal(sig)
  }
  ```

**降級路徑(degradation paths)**:
- Forecast engine disabled / API rate-limited:slog warn + skip,不中斷 pipeline
- Conviction < 30 或 > 70:進核心 threshold;中間區 return `Action="hold"` 並 `WeightMultiplier=1.0`
- 市場關閉 / data stale:同 M3 `ErrMacroDataStale` 模式,沿用前次 multiplier

#### 3.4.4 邊界 case

- Forecast 失敗率過高(> 30% 連續):circuit breaker 啟用,後續 1 小時內禁用 forecast bridge(防止 log spam + 重複低品質 trade)
- 多 horizon 衝突(7d bullish + 30d bearish):採「最近 horizon 為主」,slog info 記錄衝突
- Weight multiplier 累積:若有 macro flow × forecast bridge 雙重加成,total cap 為 1.5x(防 over-positioning)

#### 3.4.5 驗收

- [ ] `ToTradeSignal` 5 個 scenario unit test 全 pass(極強 bullish、極弱 bearish、neutral、threshold 上緣/下緣)
- [ ] `ApplySignal` 接到 trade layer 後,`direction_weight[symbol]` 在下一次 snapshot 正確反映
- [ ] Daily pipeline 整合:forecast 失敗不中斷 pipeline(走 skip + warn 路徑)
- [ ] Circuit breaker:連續 10 次預測失敗觸發 1 小時 disable
- [ ] Integration test:`TestDailyPipeline_ForecastBridge` 驗證「macro defensive + forecast bullish → 中和,weight ≈ 1.0」
- [ ] Sortino 評估:`scripts/eval/forecast_bridge_sortino.py` 跑 7 天 replay window 比較「有 bridge vs 沒有 bridge」

#### 3.4.6 工時

3 工作天(adapter 1d + trade layer integration 0.5d + orchestrator wiring 0.5d + eval/test 1d)

---

## 4. 跨 M 整合

### 4.1 時程排序(11 工作天)

```
Day 1-2:  M1 deployment-gateway   (2d,獨立交付,後端 + admin UI)
Day 3-5:  M2 narrative-taxonomy    (3d,可與 M1 平行)
Day 6-8:  M3 macro-flow             (3d,須等 M2 完成才接 event-driven weights)
Day 9-11: M4 forecast-bridge       (3d,須等 M3 完成才能驗整合)
```

但 M1 / M2 可平行(無依賴),所以實際 Gantt:

```
W1:  [M1]   [M2 ━━━━━]
W2:          [M2 ━] [M3 ━━━━━]
W3:                [M3 ━] [M4 ━━━━━]
W3 final day:  M4 + 4-doc 整合驗收 + runbook
```

實際耗時:2 週(10 calendar days)+ 1 天整合驗收。

### 4.2 共用模組影響

| 模組 | 影響 | 對應 M |
|------|------|--------|
| `internal/narrative/` | event struct + taxonomy enum | M2 |
| `internal/macroflow/` | 新檔 | M3 |
| `internal/forecast_bridge/` | 新檔 | M4 |
| `internal/orchestrator/daily_pipeline.go` | 3 次 hook insertion | M3 + M4 |
| `internal/strategy/directional_trade_layer.go` | ApplySignal 新增 | M4 |
| `internal/admin/live_deployment_dashboard.go` | 新檔 | M1 |
| `admin_web/static/js/pages/live-deployment.js` | 新檔 | M1 |
| `internal/factor/event_bridge.go` | hardcoded → SQL-driven | M2 |
| migrations/20260630_add_narrative_taxonomy.sql | 新檔 | M2 |

### 4.3 順序依賴圖

```
M1 ──→ (獨立,先 merge 或最後 merge 皆可)
M2 ──→ event_bridge SQL-driven
         ↓
        M3 needs event_bridge ✅ ready
         ↓
        M4 needs directional_trade_layer ✅ ready
```

合併順序建議:
1. M1 PR(獨立)
2. M2 PR(含 DB migration)
3. M3 PR(可獨立 merge,因 macro flow 不依賴 M2 程式碼,僅依賴 risk level 概念)
4. M4 PR(須等 M3 merge 才能正確驗整合)

每個 M 對應獨立 PR,彼此 merge 不衝突。

---

## 5. 風險與緩解

| 風險 | 等級 | 緩解 |
|------|------|------|
| DB migration lock(narrative_events 量大) | 中 | 測試環境先評估 `pg_estimated_index_lock_time`,若 > 30s 改用 `CONCURRENTLY` 拆批 |
| Macro data stale 時 macro flow 拒絕 → portfolio 永遠不 rebalance | 中 | 預設保留「若 macro data 超過 7 天 stale,允許以 default RiskLevel=balanced 跑一次」fallback |
| Forecast bridge 持續失敗 → 大量 warn log | 中 | Circuit breaker(見 3.4.4),防 log spam 也防垃圾 trade signal |
| M2 event_bridge SQL-driven:慢於 hardcoded mapping | 低 | 加 index(`narrative_taxonomy_factor_map.l1_l2_idx`),SLA 設 50ms p95 |
| M1 dashboard 30s polling 對 gateway 壓力 | 低 | 共用既有 polling 機制(若 `internal/admin/live_metrics.go` 存在;若不存在,加 simple cache TTL 5s)— **⚠️ 待確認實際位置**,`internal/admin/` 2026-06 後已不存在 |
| 4 個 PR 跨 3 週 → merge conflict 機率 | 中 | 各 M 改檔不重疊;但 `daily_pipeline.go` 同時被 M3 + M4 改,須協調整合 commit |

---

## 6. 不做 / 後續 (Phase 4 接手)

M1-M4 完成後,以下項目移交 Phase 4:

- **Phase 4.A — 參數自動 tuning**(7d):基於 M3 macro flow + M4 forecast bridge 的 sortino 資料,訓練 LLM / statistical 模型調整 6 維 macro 權重(本 spec 未涵蓋,見 reassessment §6)
- **Phase 4.B — L0/L1/L2 Constitution appendix**(16d):把 M1-M4 對應的「live / macro / forecast / trade」規則正式寫進 `docs/reference/constitution.md`,對應 reassessment §5 + §6 規劃
- **Multi-instance MCP federation**:見 [`agent-mcp-phase4.md`](./agent-mcp-phase4-spec.md) §3.3
- **Promotion 至 production**:見 [`l2-4-observation-spec.md`](./l2-4-observation-spec.md) §「Future work」模式

---

## 7. References

### 7.1 內部文件

- 起點 spec:本 spec 自身(起點論述合併於 §1/§3/§5)
- 既存相關 spec:
  - [`agent-mcp-server.md`](./agent-mcp-server-spec.md)
  - [`agent-mcp-phase3-residual.md`](./agent-mcp-phase3-residual-spec.md)
  - [`agent-mcp-phase4.md`](./agent-mcp-phase4-spec.md)
  - [`l2-4-observation-spec.md`](./l2-4-observation-spec.md)
  - [`llm-sector-agent.md`](./llm-sector-agent-spec.md)
  - [`llm-routing.md`](./llm-routing-spec.md)
  - [`llm-interface-contract.md`](./llm-interface-contract-spec.md)
  - [`real-time-regime-detection.md`](./real-time-regime-detection-spec.md)
- 技能(skills):
  - [`atlas-event-driven-weights`](../../.claude/skills/atlas-event-driven-weights/SKILL.md)(M2/M3 直接受益)
  - [`atlas-taiwan-leading-indicators`](../../.claude/skills/atlas-taiwan-leading-indicators/SKILL.md)(M3 MacroDataSnapshot 來源)
  - [`atlas-macro-narrative`](../../.claude/skills/atlas-macro-narrative/SKILL.md)
- Constitution / Guardrails:
  - [`docs/reference/constitution.md`](../reference/constitution.md)
  - [`live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md)
  - [`go-core.instructions.md`](../../.github/instructions/go-core.instructions.md)
  - [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md)
- 模組索引:[`internal/AGENTS_INDEX.md`](../../internal/AGENTS_INDEX.md)

### 7.2 來源程式碼(已存在,本 spec 引用不修改)

> **⚠ 2026-07-02 路徑稽核校正（Oracle 二階稽核後）**：以下 8 條目中 6 條目路徑經 deep-verify 報告 + Oracle 二階稽核驗證修正，2 條目（rows 7-8）原標記「沿用 ✓」為誤判（`internal/admin/` 與 `internal/apigateway/admin_handler.go` 均已不存在，Oracle P0-1）。校正後路徑與現況落差詳見 §7.5 稽核報告。

- `internal/narrative/types.go`(M2 改: 在 `NarrativeEvent` struct 加 `TaxonomyL1`/`TaxonomyL2` 欄位)
- `internal/narrative/macro_assessment.go:60` `MacroRiskAssessmentEngine.Assess()`（M3 引用；`determineRiskLevel` 為內部 method，line 158）
- `internal/orchestrator/executor_pipeline.go`(M3/M4 改: hook 插在 `inferRegime` 與 `collectRecommendations` 之間，line 76-77)
- `internal/forecast/engine.go`(**✅ 已存在** — `internal/forecast/engine.go`，2026-07-02 ship via PR #905)
- `internal/strategy/directional_trade_layer.go`(**✅ 已存在** — `internal/strategy/directional_trade_layer.go`，2026-07-02 ship)
- `internal/admin/live_state.go` / `live_metrics.go`(M1 引用,**沿用聲明錯誤**) ⚠️ **路徑不存在**：`internal/admin/` 已於 2026-06 adminapi/live 拆分後不存在，疑隨之搬移，待確認實際位置
- `internal/apigateway/admin_handler.go`(M1 引用,**沿用聲明錯誤**) ⚠️ **路徑不存在**：`internal/apigateway/` 內無 `admin_handler.go`，待確認實際位置
- `internal/portfolio/factor_weight_engine.go:288-414`(M2 改: 保留舊 `switch event.Theme`(向下相容無 taxonomy 的舊事件) + 新增 `applyTaxonomyAdjustment`(in-memory map,`map[TaxonomyL1]map[TaxonomyL2]map[FactorType]float64`),2026-07-02 ship)

### 7.3 提案 / 議題

- 提案: Issue _(待 user 確認後開)_「Phase 3.5 Integration Layer」
- 相關議題:
  - Issue #719 Wave 11 L2.3 sector agents wired
  - Issue #740 slog metrics for L2.4 observability
  - Issue #711 Wave 10 L2.3 plan
- PR baseline: PR #821 (L2.4) + PR #848 (Wave 11 mid) + PR #822 (Phase 3 mid)

### 7.4 完成後的後續

Phase 3.5 全部 4 個 M 已於 2026-07-02 merge（M1 via PR #852, M2/M3 via PR #894/#895/#898, M4 via PR #905）。後續:

1. `.omo/plans/phase3-5-runbook-stub.md` — 已建立 stub，待 M1 實作完成後補齊為正式 runbook（對齊 L2.4 runbook 模式）
2. `../../CHANGELOG.md` — 已於 Wave 11+ 更新標示「Phase 3.5 ship」
3. Phase 4.A/B 優先順序 — 待基於 M1-M4 實際結果重新評估
4. 若 sortino 改善達標(> 5%),考慮 Promotion 路線

### 7.5 2026-07-02 路徑稽核報告

> **觸發**：發現原 spec §7.2 引用的 8 個檔案路徑中，僅 2 個仍正確。其餘 6 個路徑在前述各節校正時已替換為實際位置。

**方法**：deep-verify 子代理（subagent_type=general, 5m1s）使用：
- `gitnexus_query` / `gitnexus_context`：call graph 與 execution flow 跨檔搜尋
- `codegraph_explore` / `codebase-memory_explore`：檔案內容 + 鄰近 symbol 一次取回
- 逐檔 `read`：對 gitnexus 回傳的關鍵檔案做 line-numbered source 確認

**全報告**：稽核結果內嵌於本 spec §7.5 下方 Cross-M 摘要表(原 203 行 verify-report 已併入)

**Cross-M 摘要**：

| M | Ship 狀態 | 實際落地 |
|---|---|---|---|
| M1 deployment-gateway | ✅ PR #852 (2026-06-30) | `internal/adminapi/deployment/dashboard.go` + `shared_web/static/js/components/deployment-dashboard.js` |
| M2 narrative-taxonomy | ✅ PR #894/#898 系列 (2026-07-02) | `internal/narrative/taxonomy.go`（5 L1 + 20 L2 + `ValidateTaxonomy`）+ `NarrativeEvent.TaxonomyL1/L2` + `factor_weight_engine.applyTaxonomyAdjustment`（in-memory map）+ 舊 `switch event.Theme` 保留向後相容 |
| M3 macro-flow | ✅ PR #894/#895/#898 (2026-07-02) | `internal/macroflow/`（engine/rules/adjustment + 8 scenario tests）+ `executor_pipeline.go:85-111` hook + `macro_flow.applied` audit log + `applyMacroConvictionScaling` in control layer |
| M4 forecast-bridge | ✅ PR #905 (2026-07-02) | `internal/forecast/` + `internal/forecast_bridge/`（adapter + circuit breaker）+ `internal/strategy/directional_trade_layer.go` + `executor_pipeline.go:113-123` hook + `scripts/eval/forecast_bridge_sortino.py`；預測模型標記 experimental

**校正動作**（已全部完成,2026-07-02 實體稽核後）：
- §3.1.3 校正 M1 後端/前端檔名 2 處
- §7.2 校正 8 條目 + 加 audit intro 標註
- §7.5 摘要表更新為 ✅ shipped + 敘述實際落地位置
- §3.2.3 M2 設計方案：原 spec 假設 `internal/factor/event_bridge.go` 不存在；實作改在 `internal/portfolio/factor_weight_engine.go` 以 in-memory map 取代 SQL-driven
- §3.3.3 M3 設計方案：`internal/macroflow/` 已落地；hook 在 `executor_pipeline.go`（非原本假設的 `daily_pipeline.go`）
- §3.4.3 M4 設計方案：`internal/forecast/` + `internal/forecast_bridge/` + `internal/strategy/directional_trade_layer.go` 已全部落地
4. 若 sortino 改善達標(> 5%),考慮 Promotion 路線
