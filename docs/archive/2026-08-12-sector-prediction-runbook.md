> ⚠️ **DEPRECATED 2026-08-12** — `/client/capital_predictions` 頁面已移除（PR #1523）。本 runbook 中針對 `capital_predictions.js` 摺疊區塊互動的指引不再適用。Sector predictor 邏輯本身（`internal/eventdriven/sector_predictor.go`）仍於 `/api/events/prediction` 服務，home 頁的「未來 5 日錢潮預測」section 仍使用此資料。如需恢復觀察期決策，請參考 git log。

# C07 Sector Prediction — Operations Runbook

> **對應 spec**：`docs/specs/sector-dimension-prediction-spec.md` v1.1
> **對應 invariant tracker**：``.omo/manifests/sector-dimension-prediction-invariant-manifest.md`（內部，gitignored）`
> **對應觀察記錄**：`docs/archive/sector-prediction-observation-log.md`
> **範圍**：Wave 11 C07 — `/api/events/prediction` 新增 `sector_predictions`（L1 板塊 5 日方向預測）上線後 2 週觀察期
> **Issue**: PR #1200 + Issue #1193
> **啟動日期**：PR merge 後 T+0 起算,預估 14 天
> **觀察對象**：rule-based `SectorPredictor` 輸出 vs 整體 `PredictionReport` 一致性 (JSD),以及 frontend 摺疊區塊互動

---

## 1. Pre-flight Checklist

啟用 `SECTOR_PREDICTION_ENABLED` 前,逐項確認:

> **推薦路徑**:先跑 `go run ./cmd/experimental/c07-preflight [http://localhost:18080]`(per [`docs/specs/experimental-feature-launch-gate-spec.md`](#) L2.4-style launch gate pattern)。本節手動清單與 c07-preflight automatable checks 一致;manual checks 仍需 operator 自行確認。

- [ ] **環境選擇**: staging 或專用 L2.4 harness,**不可在 production 直接啟用**(c07-preflight 會擋非 staging URL)。
- [ ] **資料源檢查**: `internal/marketdata/macro_provider.go:MacroDataSnapshot` 在 staging 可取得完整 `ForeignInvestorNet` / `TsmADRDelta` / `NVDA` / `DXY` 4 個 leading indicator。如果任一欄位為 0,先查 channel health(`/api/data/channels`)。
- [ ] **Feature flag flip**:
  - 方法 A：環境變數 `SECTOR_PREDICTION_ENABLED=true`(由 `internal/config/config.go` 的 `SectorPredictionEnabled` 載入,在 `cmd/atlas/main.go` 消費,見 I11)
  - 方法 B：JSON 旗標 `orchestrator.sector_prediction_enabled.value = true`(若尚未從 env 遷移,參考 I11)
  - **兩者僅能擇一**,預設走 env。
- [ ] **重啟服務**: flag 在啟動時讀取,無 hot-reload。執行 `docker compose restart atlas` 確認新行程載入配置。
- [ ] **Boot log 確認**: log 出現 `[EventDriven] sector predictions enabled (macro provider wired)`。若出現 disabled 訊息,代表 macro provider 注入失敗,需檢查 `cmd/atlas/main.go:704-720` 區段。
- [ ] **Health 端點**: `curl -fsS http://localhost:18080/health` 回 `{"status":"ok",...}`,且 `curl -fsS http://localhost:18080/api/events/prediction | jq .sector_predictions | length` = 20。
- [ ] **前端確認**: `http://<host>/client/`,切換到「資金預測」分頁,確認「板塊方向預測」區塊可摺疊/展開,且可顯示 5 必須看板塊。
- [ ] **空狀態測試**: 暫時關閉 `MacroDataSnapshot` 注入或送入空事件,確認 frontend 顯示 empty state(`forecast/capital 區塊保持 200 OK,僅 sector section 顯示「無可用預測」`)。

## 2. Daily Check-in 流程

每日(或每 100 次 request)執行:

### 2.1 指標收集

從 `curl /api/events/prediction` 拉出下列欄位,彙整至觀察記錄(`docs/archive/sector-prediction-observation-log.md`):

| 指標 | 來源 | 計算 |
|------|------|------|
| `jsd.p95` | `sector_predictions[].days[].distribution` 與整體預測比對的 JSD | 95 百分位,從 backend metric 或從 logr 拉出 |
| `jsd.alert_rate` | JSD > 0.25 的請求比例 | 數量 / 總請求 |
| `prediction.latency_p50` | `/api/events/prediction` 含 sector predictions 整體延遲中位數 | 50 百分位 |
| `prediction.latency_p95` | 同上,95 百分位 | < 200ms 為 KPI |
| `confidence.floor_violations` | 任意 sector prediction 中 confidence < 0.40 的數量 | 必須 = 0 |
| `sector.count_per_day` | 每天 sector_predictions 各 day 的 sector 數量 | 必須 = 20 |
| `event.coverage_rate` | events.affected_industries 至少映射到 1 個 L1 sector 的事件比例 | 觀察是否有 silent miss |

### 2.2 Spot-check(每日 3-5 筆)

抽 3-5 個 sector prediction,對比整體 prediction.direction:

- **方向一致率**: sector-weighted direction 與 overall direction 一致比例(預期 ≥ 80%)
- **Driver quality**: top-2 driver 是否真的引用 `MacroDataSnapshot` / event / cycle 任一資訊
- **JSD 警示**: 任意 sector prediction 帶 `consistency_warning=true` 都應 log 警示並 spot-check 模型輸入

### 2.3 自動化工具（推薦）

以下兩個 CLI 工具把 §2.1 指標收集與 §3 驗收評估自動化，operator 只需設定 cron 即可：

| 工具 | 用途 | 執行頻率 |
|------|------|---------|
| `cmd/experimental/c07-obs-collector` | 拉 `/api/events/prediction`，計算 JSD alert rate / latency / confidence violations，附加到 obs log，超標時 fire alert | 每日收盤後 |
| `cmd/experimental/c07-day-evaluator` | 讀 obs log，評估 Day 7 / Day 14 acceptance criteria，產出 pass/fail report | Day 7 / Day 14 |

**Cron 設定範例**（staging）：

```cron
# 每日 15:30 收盤後跑 collector（台股收盤 13:30，留 2h buffer）
30 15 * * 1-5  cd /path/to/atlas && go run ./cmd/experimental/c07-obs-collector --url http://localhost:18080

# Day 7 與 Day 14 早上 09:00 跑 evaluator
0 9 * * 1-5  cd /path/to/atlas && go run ./cmd/experimental/c07-day-evaluator --day 7 --output docs/operations/sector-prediction-eval-day7.md
0 9 * * 1-5  cd /path/to/atlas && go run ./cmd/experimental/c07-day-evaluator --day 14 --output docs/operations/sector-prediction-eval-day14.md
```

**注意事項**：
- Collector 在 flag 未開啟時會記錄 `flag off` 並 exit 0（不視為失敗）。
- Collector 會自動 fire alert 到 `data/state/alerts`（透過 `monitoring.AlertStore`），可在 `/api/alerts` 查詢。
- Evaluator 在任一 MUST criterion 失敗時 exit 1，可接 cron 的 `MAILTO` 或 Slack webhook 通知。
- `spot_check_count` 與 `panic_count` 仍需人工確認（collector 無法自動取得）。

### 2.4 Log 範本（人工補充用）

若需人工補充（例如 spot-check 結果），使用下列範本附加到 obs log：

```markdown
| YYYY-MM-DD | 20 | 0.0% | 145 | 0 | 0 | 15 | auto-collected + manual spot-check |
```

## 3. Acceptance Criteria

### Day 7 checkpoint

| 條件 | 閾值 | 動作 |
|------|------|------|
| `jsd.alert_rate` | < 5% | 超標 → 檢查 macro weight 與 cycle shift,必要時調降 macro beta 或提高 prior shrinkage |
| `prediction.latency_p95` | < 200ms | 超標 → 改為 cron 預計算(`internal/eventdriven/sector_predictor.go` 改為定時任務+ cache) |
| `confidence.floor_violations` | = 0 | 違反 → 立即排查 (invariant I7,不可妥協) |
| `sector.count_per_day` | = 20 | 違反 → 檢查 `industry.L1Sectors()` 與 selector |
| 0 unhandled panic | 必須 | 觸發 → 立即 rollback |
| Spot-check ≥ 15 recs | 必須 | 不足 → 延長觀察至 day 14 |

### Day 14 promotion gate

在 Day 7 條件全部通過後,加驗:

- **整體預測命中率不退步**: 對觀察窗口內所有每日 prediction,計算 top-tier sector 的方向命中率,與上一個月的整體預測命中率 baseline 對比,Δ ≥ -3% 為通過。
- **Driver 可解釋性**: 累計 spot-check ≥ 20 筆,每筆 driver 至少引用 1 個具體的 macro/cycle/event 來源(不可純 prior)。
- **Roll-back 驗證**: 至少一次手動測試把 `SECTOR_PREDICTION_ENABLED` 翻回未設置,並重啟,確認 `sector_predictions` 欄位從回應消失(或變為空 slice),且 1 cycle 內完成切換。

任一條件未過 → 觸發 §4 Rollback,並 file follow-up issue 紀錄根因。

## 4. Rollback Procedure

當 acceptance criteria 任一未達標,或觀察期內出現 panic、latency 暴增、信心違反等異常:

1. **停用 flag**:
   - env 模式：`docker compose down atlas && SECTOR_PREDICTION_ENABLED= docker compose up -d atlas`
   - JSON 模式：將 `orchestrator.sector_prediction_enabled.value` 由 `true` 改回 `false`。
2. **重啟服務**: `docker compose restart atlas`。無熱載入,必須重啟。
3. **驗證切換**: 觀察下一個 `/api/events/prediction` 請求,確認 `sector_predictions` 為空 slice 或欄位缺失(`cmd/atlas/main.go:708-712` 的守衛邏輯)。
4. **記錄異常**: 在觀察記錄(`docs/archive/sector-prediction-observation-log.md`)標註 rollback 時間與觸發條件。
5. **File follow-up issue**: 根因分析(prior 太強? cycle shift 過大? JSD threshold 過嚴?),**未解決前不可重新啟用**。

Rollback 不應影響整體預測:因 sector predictions 為純附加欄位,handler 仍可服務 `PredictionReport` 其餘內容。

## 5. Promotion Procedure

Day 14 acceptance 全部通過後,執行下列步驟(每步獨立 PR):

### 5.1 Day 14 Promotion Checklist

- [ ] **§3 Day 14 criteria 全部 pass**: `jsd.alert_rate < 5%`, `prediction.latency_p95 < 200ms`, `confidence.floor_violations = 0`, `sector.count_per_day = 20`, spot-check ≥ 20 recs, 0 unhandled panic
- [ ] **Top-tier hit rate Δ ≥ -3%**: 對比觀察窗口內每日 top-tier sector 方向命中率 vs 上個月 baseline。使用 `c07-day-evaluator --day 14` 確認 exit code = 0
- [ ] **Driver 可解釋性**: 累計 spot-check ≥ 20 筆,每筆至少引用 1 個 macro/cycle/event 來源
- [ ] **Rollback 驗證通過**: 至少一次手動測試 (見 §5.2)
- [ ] **邀請至少一位 team member 審閱觀察記錄** (`docs/archive/sector-prediction-observation-log.md`),確認無遺漏 edge case
- [ ] **Invariant 確認**: 運行 ``.omo/manifests/sector-dimension-prediction-invariant-manifest.md`（內部，gitignored）` 中所有 automated checks

### 5.2 Rollback Verification Procedure

翻 `SECTOR_PREDICTION_ENABLED` → false → 重啟後確認:

```bash
# 1. 停用 flag
export SECTOR_PREDICTION_ENABLED=false

# 2. 重啟
docker compose restart atlas

# 3. 驗證 sector_predictions 已消失
curl -fsS http://localhost:18080/api/events/prediction | jq '.sector_predictions | length == 0'

# 4. 確認剩餘 prediction report 不受影響
curl -fsS http://localhost:18080/api/events/prediction | jq '.direction != null'

# 5. 恢復 flag
export SECTOR_PREDICTION_ENABLED=true
docker compose restart atlas

# 6. 確認 sector_predictions 恢復
curl -fsS http://localhost:18080/api/events/prediction | jq '.sector_predictions | length == 20'
```

整個流程應在 1 cycle (5min) 內完成切換,不影響其他 prediction report 欄位。

### 5.3 Promotion PR Content

Promotion PR 應包含:

- `internal/config/parameters.go`: 新增常數化參數 (若需 JSON 治理)
- `cmd/atlas/main.go`: 翻 `SECTOR_PREDICTION_ENABLED` default 為 true (獨立 PR)
- `docs/operations/sector-prediction-runbook.md`: 更新 Pre-flight flag 說明為「預設啟用」
- `docs/archive/sector-prediction-observation-log.md`: 附上完整觀察記錄總結
- `shared_web/static/js/...`: 如有關閉 sector predictions 的 frontend fallback 變更

> Step 1 與 Step 2 為**獨立 PR**,不可合併在同一個 commit。這確保 rollback 時可以精確控制只回退 flag 翻轉而不影響參數定義。

### 5.4 Tag 版本

上述變更合併後,標記 `v0.0.0.XX`(具體版本號依當時累積變更決定,參考 `CHANGELOG.md`)。

> 設計保持簡單:promotion 流程不引入新 calculation 路徑,僅翻 default + 文件化對齊。

## 6. Failure Modes & Escalation

| 失敗模式 | 偵測 | 處置 | 升級對象 |
|----------|------|------|----------|
| **JSD alert rate > 10%** | `jsd.alert_rate` metric 超標 | 立即檢查 macro weight,必要時把 `macroExposureAdjustment` 係數 0.15 → 0.05;若仍超標 → rollback | data-science on-call |
| **`confidence < 0.40` 出現** | `confidence.floor_violations > 0` | 違反 I7,立即停用 sector predictions 並排查 (`sector_predictor.go:confidenceFloor`) | engineering on-call |
| **Sector 數量 < 20** | `sector.count_per_day != 20` | 違反 I2,檢查 `industry.L1Sectors()` 是否被改動,或事件 `affected_industries` 過濾錯誤 | engineering on-call |
| **`/api/events/prediction` p95 ≥ 200ms** | latency 監控超標 | 先檢查是否 macro provider 重複查詢(`HandlePrediction` 應取 1 次/請求);若仍超標 → 改 cron 預計算 cache | platform on-call |
| **`sector_predictions = null`** | API response 缺失欄位 | 違反 I1,絕對不可出現;發現立即 rollback 並查 type 定義(`types.go:PredictionReport.SectorPredictions`) | engineering on-call |
| **Frontend 空白區塊 / crash** | `/client/` 開「資金預測」500 或白屏 | 檢查 `capital_predictions.js` 是否對空 `sector_predictions` 做了恰當處理(`renderSectorDetail` 應顯示 empty state) | frontend on-call |
| **`SECTOR_PREDICTION_ENABLED` env 沒生效** | 重啟後 boot log 仍出現 disabled | 檢查 `cmd/atlas/main.go:704-712` 守衛,確認 env var 名稱正確(大小寫敏感);檢查 `compose.yml` env section 是否 propagate | platform on-call |
| **Macro data 全 0** | `MacroDataSnapshot` 為初始值 0 | 違反 `atlas-data-visibility` skill 約束,channel ingest 故障;先查 `/api/data/channels` 與 wave9 alert (`atlas_db_init_failures_total`) | data-platform on-call |
| **Promotion gate 評估衝突** | Day 14 條件多 team 意見分歧 | 以觀察記錄數據為準,由 issue owner 仲裁 | Kaecer (product owner) |

### 溝通管道

- 觀察進度更新: 在本 PR 留言 thread 每日摘要(僅 spot-check 結果,非逐筆 log)。
- 緊急異常(panic、>10s latency、>10% JSD alert): Slack `#atlas-ops` + 開 incident issue,標籤 `incident`。
- 觀察期結束決策: 在觀察記錄末段總結,並 link 到後續 promotion / rollback PR。

## 7. References

- Spec: `docs/specs/sector-dimension-prediction-spec.md`
- Invariant Tracker: ``.omo/manifests/sector-dimension-prediction-invariant-manifest.md`（內部，gitignored）`
- 觀察記錄: `docs/archive/sector-prediction-observation-log.md`
- 實作:
  - Backend: `internal/eventdriven/sector_predictor.go` + `predictor.go` + `handler.go` + `types.go`
  - Frontend: `shared_web/static/js/pages/capital_predictions.js` + `components/` + `__tests__/sector_predictions.test.mjs`
  - 旗標: `cmd/atlas/main.go` (`SECTOR_PREDICTION_ENABLED`)
- 觀察指標規範: 本檔 §2.1
- 資料源:
  - Macro: `internal/marketdata/macro_provider.go`
  - Cycle: `internal/industry/cycle.go`
  - L1 Sectors: `internal/industry/sector.go` (`L1Sectors()`)
  - 事件: `internal/eventdriven/types.go` (`PredictionEvent.AffectedSectors`)
- L2.4 觀察框架 (模板來源): `docs/operations/l2-4-runbook.md`
- 啟發式模型理論背景: `internal/eventdriven/sector_predictor.go` 內 docstring(預設權重來源、JSD 公式)
