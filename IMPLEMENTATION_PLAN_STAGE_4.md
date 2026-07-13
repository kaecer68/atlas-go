# Atlas Event-Flow Historical Data Backfill — 實作規劃

> **分支**：`feat/stage-4-historical-backfill`（worktree: `~/workspace/atlas-stage4-backfill/`）
> **基礎**：`feat/stage-3.3-ledger-records-gauge` @ `78f4ebbb`（Stage 3 latest）
> **目標**：補齊 90 天歷史資料，使 `event_flow_prediction` 有完整 ground truth 可測試，並重算 23 個 template 的 hit_rate
> **決策來源**：
>   - `~/workspace/atlas-notes/decisions/` — Stage 1-3 決策檔
>   - 本檔第三節「範圍決議」（用戶於 2026-07-14 確認）
> **語言**：繁體中文（所有 code comment + commit message + PR description）

---

## 一、現況摘要（Stage 1-3 已完成，作為 Stage 4 的基礎）

### Stage 1（4-PR 補齊，PR #1102）
- EventCalendar wire-up — `industry.NewEventCalendarWithProvider`，main.go MCP 路徑與 dashboard 統一
- capitalflow injection — `CapitalFlowProvider` 介面（`QualityScore/QualityLabel`）+ `SetCapitalFlow`
- Darwinian narrative models — `NarrativeModelProvider` + `ModelView` + `eventTypeToThemes`
- UTC + scheduling + nil guards — pipeline.go:1089/1095 UTC、janus_regime_refresh 6h

### Stage 2（data quality + EventValidator）
- EventValidator + 5 rules + QualityLog（`internal/eventquality/`）
- EventValidator + QualityLog hook in event_calendar.go
- title sanitize + cross-source verify + template model audit

### Stage 3（scheduling + alerting + observation log）
- 5 個排程：`sync-events-daily` 06:00、`sync-macro-daily` 06:00、`sync-capital-daily` 13:30、`sync-regime-weekly` Mon 08:00、`recalibrate-templates-monthly` 每月 1 號 08:00
- 3 條報警任務 wrapper + opt-out flags
- `RecalculateTemplateHitRates` public wrapper（narrative:451）
- `RecentEventFlowPredictions` 改為 ledger 持久化（JSONL `event_flow_predictions.jsonl`，cap 1000 筆）
- `IsTradingDay` 改用 `EventCalendar.IsTaiwanTradingDay`（避免改 `buildHolidayEvent` 介面）

### Stage 3 defer follow-ups（已完成）
- `MarkDegraded` + channel_health degraded 狀態
- VIXBaselineTracker 252-day rolling median
- PRISM cron 6h → event-driven trigger

### 已驗證的關鍵檔案行號
| 檔案:行 | 內容 |
|---|---|
| `internal/eventdriven/predictor.go:14-130` | `Predictor{calendar, capitalFlow, narrativeModels}` + `Predict(now)` + `predictDay()` |
| `internal/eventdriven/predictor.go:38` | `capitalFlow: &staticCF{score: 0, label: "neutral"}` — 預設 zero |
| `internal/ledger/event_flow_prediction_store.go` | `EventFlowPredictionStore` interface、`JSONLEventFlowPredictionStore`（cap 1000，FIFO）、`AppendPrediction/LoadRecentPredictions/Len/Size` |
| `cmd/atlas/stage3_tasks.go:24-300` | `stage3Deps` + `registerStage3Tasks` + `registerStage3AlertTasks`、5 task + 3 alert 包裝 |
| `internal/narrative/templates.go:1-490` | 23 個 templates（"美國升息/鷹派聯準會"、"日圓套利平倉"、"AI 資本支出激增"…） |
| `internal/narrative/knowledge_base.go:422-454` | `updateTemplateHitRates` + `RecalculateTemplateHitRates` public method |
| `internal/industry/event_calendar.go:111-187` | `EventCalendar` struct + `WithValidator/WithQualityLog/WithCrossSourceStore` + `gateEvent` |

### 已驗證的資料覆蓋（從 `data/state/` 實地掃描 2026-07-14）
| 資產 | 路徑 | 覆蓋 | 備註 |
|---|---|---|---|
| Sessions | `data/state/sessions/session-YYYYMMDD-daily/` | 142 個（2026-01-01 ~ 2026-07-13） | 每個含 `summary.json:regime` |
| Recommendation outcomes | `data/state/recommendation_outcomes.jsonl` | 41,369 行 | 每筆含 `regime` + `factor_scores` + `narrative_hit_rates` + `supporting_events` |
| Macro snapshots | `data/state/macro/2026-XX-XX.json` | 76 個（2026-04-15 ~ 2026-07-13） | 含 `latest.json` + `previous.json` |
| Capital flow | `data/state/capital_flow/2025MMDD.json` | 35 個（2025-05-19 ~ 2025-09-04）| ⚠️ **STALE 10 個月**，需重抓 |
| Experiments | `data/state/experiments/exec-*.json` | 5+ exec + `experiments.jsonl` | 已存在 |

### 真正的歷史缺口（Stage 4 真正缺什麼）
| 缺口 | 現況 | Stage 4 必須補齊 |
|---|---|---|
| 過去 90 天 event_calendar | 只 in-memory（refresh today+today+1），**未持久化** | event_calendar_history 表 |
| 過去 90 天 regime 序列 | 散落 142 個 summary.json，未聚合 | regime_history 表 |
| 過去 90 天 stress_index 數值 | 未儲存（runtime-only） | stress_index_history 表 |
| 過去 90 天 event_flow_prediction 預測 | `event_flow_predictions.jsonl` 只有 forward-looking 1/day | prediction_backtest（重跑 historical） |
| 過去 90 天 template 命中率 | narrative_hit_rates 只有 7 個 active template，其他 16 個=0.5 預設 | 23 個 template 全面重算 |

---

## 二、4-PR 拆分與依賴關係

```
PR#1 (P0)  Backfill pipeline CLI
   │  cmd/atlas-stage4-backfill
   │  從 sessions + recommendation_outcomes.jsonl + macro 抽取 90 天資料
   │  → 產出 staging JSONL：data/staging/event_calendar_90d.jsonl 等
   ▼
PR#2 (P1)  Historical data tables — SQLite schema
   │  internal/ledger/historical_store.go（SQLite）
   │  新增 tables：regime_history, stress_index_history, event_calendar_history
   │  → 新增 MCP tools：get_regime_history_90d / get_stress_history_90d / get_event_calendar_history_90d
   ▼
PR#3 (P2)  Prediction backtest engine
   │  cmd/backtest-event-flow
   │  對過去 90 天每天重跑 event_flow_prediction，比對當天 capital_flow 變化
   │  → 寫入 SQLite prediction_backtest（NOT 進 experiment_history）
   ▼
PR#4 (P3)  Template hit_rate 全 23 個重算 + 持久化
   │  RecalculateTemplateHitRates(include_all=true)
   │  從 prediction_backtest 拉 ev → evt 配對，累計 total_tests / total_hits
   │  → 寫回 narrative_hit_rates 全 23 個
   │  → 驗證 darwinian weight 收斂（recent_error 在 [0.1, 0.4] 區間）
```

**依賴嚴格** — PR#2 需要 PR#1 的 staging JSONL；PR#3 需要 PR#2 的 history tables；PR#4 需要 PR#3 的 prediction_backtest 表。

---

## 三、範圍決議（用戶於 2026-07-14 確認）

### 決議 A：Ground truth 來源
- **選擇**：從 41,369 筆 `recommendation_outcomes.jsonl` 的 `supporting_events` + `regime` + `factor_scores` 反推
- **不採用**：回頭補抓 TWSE/FinMind 90 天 API（避免動到線下真實資料 + 時間成本）
- **設計影響**：backfill pipeline 只讀現有資料，**完全不寫入生產路徑**

### 決議 B：backtest 結果的儲存位置
- **選擇**：新開 **SQLite table** `prediction_backtest`，寫入 `data/state/atlas.db`
- **不採用**：JSONL（無法做時間序列 query）/ 擴充 experiment_history.jsonl（可能踩到「不改 audit log schema」紅線）
- **設計影響**：
  - 使用既有 `data/state/atlas.db`（DATA_ARCHITECTURE.md Layer 18 ⚠️ 待處理）
  - 在該檔新增 1 個 table（**不動既有 6 個 tables**）
  - Stage 4.5 結束時 refresh DATA_ARCHITECTURE.md，把 Layer 18 改為"S"（stable）並標註 Stage 4 啟動

### 決議 C：Template 重算範圍
- **選擇**：**全部 23 個** template 都重算 hit_rate
- **影響**：
  - 原 `RecalculateTemplateHitRates` 只處理「有實際觸發紀錄」的 7 個 templates
  - Stage 4 改為接受 `IncludeAll bool` 參數，當 true 時全 23 個都用 backtest 結果重新計算
  - Stage 4.4 結束時驗證 darwinian weight 收斂（避免重算結果把 weight 推到極端）

---

## 四、各 PR 的根因解方（非補丁式）

### PR#1 (P0) — Backfill pipeline CLI
**根因**：現有資料（sessions + recommendation_outcomes.jsonl + macro）存在但分散在各處，沒有任何工具能把「過去 90 天」的橫切面抽出來。

**解方**：
1. **建立 CLI**：`cmd/atlas-stage4-backfill/main.go`
2. **參數**：
   - `--lookback-days`（預設 90）
   - `--source data/state`（資料根目錄）
   - `--out data/staging`（輸出 JSONL staging 目錄）
3. **輸出 5 個 staging JSONL**：
   - `event_calendar_90d.jsonl`：每筆 `{date, events: [...], source: "session" | "reco"}`
   - `regime_history_90d.jsonl`：每筆 `{date, regime, source_session_id}`
   - `stress_index_history_90d.jsonl`：每筆 `{date, score, regime, components: {}}`（從 macro snapshot 的 `latest.json` + 前次比對）
   - `prediction_input_snapshot_90d.jsonl`：每筆 `{date, events, regime, narrative_models}`（PR#3 會用）
   - `prediction_actual_90d.jsonl`：每筆 `{date, capital_flow_change, hit_outcomes_count}`（PR#3 會用）
4. **冪等性**：command 用 date 做 partition key，已存在的 partition 跳過

**避免的補丁式**：不直接寫 SQL（要等 PR#2 schema），不動既有 ledger code（完全新檔案）

### PR#2 (P1) — Historical data tables（SQLite）
**根因**：現有 `data/state/atlas.db` 是 DATA_ARCHITECTURE.md ⚠️ 待處理 Layer 18，但只要新增 table 而不動既有 6 個 tables，就符合「不動既有 schema」紅線。

**解方**：
1. **新建 store**：`internal/ledger/historical_store.go`
2. **Schema migration**：定義 4 個新 tables（用 CREATE TABLE IF NOT EXISTS）
   ```sql
   CREATE TABLE IF NOT EXISTS regime_history (
     date TEXT PRIMARY KEY,
     regime TEXT NOT NULL,
     source_session_id TEXT NOT NULL,
     captured_at TEXT NOT NULL
   );
   CREATE TABLE IF NOT EXISTS stress_index_history (
     date TEXT PRIMARY KEY,
     score REAL NOT NULL,
     regime TEXT NOT NULL,
     components_json TEXT NOT NULL,
     captured_at TEXT NOT NULL
   );
   CREATE TABLE IF NOT EXISTS event_calendar_history (
     date TEXT NOT NULL,
     event_id TEXT NOT NULL,
     name TEXT NOT NULL,
     event_type TEXT NOT NULL,
     direction TEXT,
     base_weight REAL,
     source TEXT NOT NULL,
     PRIMARY KEY (date, event_id)
   );
   CREATE TABLE IF NOT EXISTS prediction_backtest (
     prediction_date TEXT NOT NULL,
     event_date TEXT NOT NULL,
     predicted_direction TEXT NOT NULL,
     predicted_confidence REAL NOT NULL,
     predicted_sign REAL NOT NULL,
     actual_capital_flow_change REAL,
     actual_outcome_count INTEGER,
     hit INTEGER,  -- 0/1 if available
     is_synthetic INTEGER NOT NULL DEFAULT 1,
     diff_source TEXT,  -- "session-derive" | "replay-backfill"
     captured_at TEXT NOT NULL,
     PRIMARY KEY (prediction_date, event_date)
   );
   ```
3. **MCP tools**：新增 3 個 read-only tools
   - `regime_history_get_90d`
   - `stress_index_history_get_90d`
   - `event_calendar_history_get_90d`
4. **寫入 routine**：從 PR#1 的 staging JSONL load → INSERT OR REPLACE

**避免的補丁式**：不用 Atlas Postgres（schema 不適合時間序列；用 SQLite 與 DATA_ARCHITECTURE.md Layer 18 一致）

### PR#3 (P2) — Prediction backtest engine
**根因**：現有 `event_flow_prediction` 是 forward-looking daily run，沒有「用當時資料 snapshot 重跑過去」的 backtest 能力。

**解方**：
1. **建立 CLI**：`cmd/backtest-event-flow/main.go`
2. **參數**：
   - `--lookback-days 90`
   - `--predictor`（預設從 `eventdriven.NewPredictor`）
   - `--out data/state/atlas.db:prediction_backtest`
3. **執行邏輯**：
   - 對 `prediction_input_snapshot_90d.jsonl` 每筆讀當天的 `events + regime + narrative_models`
   - 呼叫 `Predictor.predictDay()` 算出當天的 predicted_direction / predicted_confidence
   - 比對 `prediction_actual_90d.jsonl` 當天的 capital_flow_change（delta > 0 = inflow，< 0 = outflow，~0 = neutral）
   - `hit` 規則：predicted direction == actual direction AND |predicted_conf - actual_outcome_hit_rate| < 0.3
   - 寫入 prediction_backtest SQLite table，`is_synthetic=1`（因為是從 snapshots 重推）
4. **驗證**：第一輪跑完後 SELECT count(*), avg(hit) GROUP BY prediction_date 看分布

**避免的補丁式**：不在 `Predictor` 內加 backtest 分支（會污染 forward-looking 路徑）

### PR#4 (P3) — Template hit_rate 全 23 個重算
**根因**：`RecalculateTemplateHitRates()` 現版本只處理有實際触發紀錄的 7 個 templates（其餘 16 個 template 的 hit_rate 留 0.5 預設值）。

**解方**：
1. **擴充 API**：`RecalculateTemplateHitRates(includeAll bool)` 接收 `IncludeAll` 參數
2. **資料源**：從 `prediction_backtest:diff_source` 拉取每個 template 的「預測事件 → 實際結果」配對
3. **計算**：每個 template 累計 `total_tests`（預測次數）+ `total_hits`（方向正確次數），更新 `hit_rate = total_hits/total_tests`，並用 EMA-blend（α=0.3）與既有值混合
4. **持久化**：寫回 `parameter_snapshot:narrative_hit_rates` 全 23 個項目
5. **驗證 darwinian weight**：
   - 跑 `go test ./internal/narrative/... ./internal/orchestrator/...`
   - 跑 `enhanced_experiment_runner.go` 5 輪，確認 weight 在 [0.3, 2.5] 區間（資料夾 AGENTS.md 紅線）
   - 如有 template 收斂到極端值，標記為「需 Stage 4.5 followup」

**避免的補丁式**：不寫死 23 個 template 的預設 hit_rate（必須從 backtest 結果計算）

---

## 五、每 PR 的執行流程（嚴格依照）

1. **跑 atlas-pre-change-protocol 8 步**（Step 0 重疊檢查 → Step 7 設計意圖）
2. **寫 code + 對應測試**（同 package `*_test.go`，coverage ≥ 60%）
3. **跑驗證清單**：
   ```bash
   test -z "$(gofmt -l .)"
   go vet ./...
   staticcheck ./...
   golangci-lint run --timeout=5m
   go test ./internal/ledger/... ./internal/eventdriven/... ./internal/narrative/... ./cmd/atlas-stage4-backfill/...
   go test -coverprofile=coverage.out ./internal/ledger/... && go tool cover -func=coverage.out | grep total  # ≥ 60%
   ```
4. **integration test**：
   - PR#1：跑 CLI，從 staging JSONL 抽出 30 天 sample，肉眼對 1 個 session summary
   - PR#2：從 MCP 工具讀取 90 天 regime，跟 summary.json 比對 5 個 sample
   - PR#3：跑 90 天 backtest，SELECT * FROM prediction_backtest LIMIT 5 看結果
   - PR#4：跑 RecalculateTemplateHitRates(includeAll=true)，驗證 23 個項目都有值
5. **commit + push**（每 PR 一個 commit，commit message 寫根因）
6. **下一個 PR**

---

## 六、最終驗證清單（4 PR 全完成後跑）

### 資料覆蓋
- [ ] `event_calendar_history` 至少 60 天有 ≥ 1 個 event
- [ ] `regime_history` 90 天連續（無 gap）
- [ ] `stress_index_history` 至少 60 天有真實 score（不是 NaN 0）
- [ ] `prediction_backtest` 90 天連續，每筆含 `is_synthetic=1` 標記
- [ ] `narrative_hit_rates` 全 23 個 template 都有數值（非 0.5 預設）

### 系統健康
- [ ] MCP tool `regime_history_get_90d` 回傳 ≥ 60 筆且 regime 分布合理
- [ ] MCP tool `stress_index_history_get_90d` 回傳 ≥ 60 筆且 score 有方差（不全為 0）
- [ ] MCP tool `event_calendar_history_get_90d` 回傳的事件數 ≥ session 平均 event count × 60
- [ ] 既有 91 個 MCP tool 介面 **完全不變**（除新增的 3 個 read tools）
- [ ] 既有 audit log schema **完全不改**
- [ ] 既有 5 個事件偵測器 **全開**（沒有任何被關閉）

### 數據品質
- [ ] `prediction_backtest.hit` 平均命中 > 0.55（前 90 天樣本）
- [ ] `prediction_backtest.actual_capital_flow_change` 有 30%+ 樣本非 0
- [ ] darwinian weight **收斂**（所有 weight ∈ [0.3, 2.5]）
- [ ] templates 23 個 hit_rate 分布合理（0.3 ~ 0.7）
- [ ] coverage ≥ 60%

### 工程品質
- [ ] `golangci-lint run --timeout=5m` 0 issues
- [ ] `staticcheck ./...` 0 issues
- [ ] `go vet ./...` 0 issues
- [ ] 既有測試全綠（不引入 regression）
- [ ] 新測試：每個新 component 都有對應 *_test.go

### 文檔
- [ ] `DATA_ARCHITECTURE.md` 更新 Layer 18 → S（stable）
- [ ] `DATA_CATALOG.md` 新增 prediction_backtest / regime_history / stress_index_history / event_calendar_history 條目
- [ ] `TRAPS.md` 新增 Stage 4 任何 trap
- [ ] `CHANGELOG.md` 新增 Stage 4 entries

---

## 七、嚴格遵守的紅線（不可違反）

1. ✅ 不准改既有的 audit log schema — `experiment_history.jsonl` 完全不改
2. ✅ 不准關掉既有事件偵測器 — 5 個 BTM task 全 keep enabled
3. ✅ 不准在 production 直接改東西 — 所有 SQL 寫入用 SQLite（離線分析）
4. ✅ 不准寫死資料進資料庫 — `prediction_backtest.is_synthetic=1` 永遠標記
5. ✅ 不准繞過品質檢查直接寫入事件 — EventValidator 仍生效

**bonus 規範**：
- 不動 Atlas Postgres（用 SQLite 只新增 1 個 table）
- 不動既有 6 個 MCP tool 介面
- 不動既有 23 個 template ID 字串（hit_rate 可改，ID 不能動）
- 不動既有 recommendation_outcomes.jsonl schema

---

## 八、不做的事（Out of scope）

明確不做，避免 scope creep：
- ❌ 重跑真實 90 天 backfill（TWSE/FinMind API backfill）— 用戶決議 A 否決
- ❌ 90 天之前（2025 之前）的歷史回填 — sessions 只到 2026-01-01
- ❌ 即時更新 Atlas Postgres prediction_backtest — 用戶決議 B 選 SQLite
- ❌ 自動 debias template weight（只重算 hit_rate，weight 收斂由 darwinian 自然處理）
- ❌ 把 narrative_hit_rates 從 7 個擴充到 23 個以外的 templates（templates.go 沒有第 24 個）
- ❌ 改既有 forward-looking `event_flow_prediction` 行為（PR#3 只新增 backtest CLI）

---

## 九、rollback 策略

每個 PR 都可以 `git revert <sha>` 撤銷：
- PR#1：刪除 `cmd/atlas-stage4-backfill/` + `data/staging/`
- PR#2：刪除 `internal/ledger/historical_store.go` + 4 個 table DROP（`DROP TABLE IF EXISTS` script）
- PR#3：刪除 `cmd/backtest-event-flow/` + `prediction_backtest` table 清空
- PR#4：用 `RecalculateTemplateHitRates(includeAll=false)` 回滾（保留向後相容路徑）

**關鍵**：PR#4 一定要把 `includeAll` 做成**可選參數**，預設 false（向後相容），這樣 rollback 時只需重新觸發一次正向流程即可。

---

## 十、決策檔輸出

每 PR 結束時在 `~/workspace/atlas-notes/decisions/` 開對應檔案：
- `2026-07-14-stage-4.1-backfill-cli.md`
- `2026-07-14-stage-4.2-historical-tables.md`
- `2026-07-14-stage-4.3-prediction-backtest.md`
- `2026-07-14-stage-4.4-template-recalc.md`

每檔結構：
1. Stage / PR 編號
2. 改動清單（新增 / 修改 / 刪除 檔案）
3. 測試結果（覆蓋率、golangci-lint、go vet）
4. 已知待辦 / follow-up
5. 驗證指令（MCP 查詢 + SQL 查詢 sample）

---

*建立時間：2026-07-14 (Asia/Taipei)*
*分支：`feat/stage-4-historical-backfill`*
*worktree：`~/workspace/atlas-stage4-backfill/`*
*負責人：kaecer + opencode Sisyphus*
*基礎 commit：78f4ebbb（feat/monitoring event_flow_prediction ledger gauge）*
