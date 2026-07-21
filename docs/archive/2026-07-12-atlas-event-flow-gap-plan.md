# Atlas Event-Flow 缺口補齊 — 實作規劃

> **分支**：`fix/event-flow-gap-stage1`（worktree: `~/workspace/atlas-event-flow-fix/`）
> **基礎**：3be6b900（main HEAD）
> **目標**：讓 `event_flow_prediction` 在真實事件觸發時輸出非 neutral/conf>0.5/可審計因果鏈
> **決策來源**：`~/workspace/atlas-notes/05-decisions/2026-07-12-stage1-event-flow-gap-audit.md` §選項 A
> **語言**：繁體中文（所有 code comment + commit message + PR description）

---

## 一、現況摘要（Stage 1 已驗證）

### 4 個資料源 MCP 驗證結果
| 資料源 | 狀態 |
|--------|------|
| `event_calendar` | ❌ `{"events":[],"total":0}` |
| `event_flow_prediction` | ❌ 5 天 neutral / conf 0.5 / driving_events=null |
| `macro_get_capital_flow_latest` | ⚠️ value 有，change_pct=0，timestamp vs recorded_at 差 1 個月 |
| `macro_get_stress_index_current` | ✅ score=8.757, regime=low（唯一健康） |
| `regime_get_history` 30d | ❌ 全部 score=0 + 時區混亂 |

### 6 個 Critical Bugs
| # | 嚴重性 | 位置 | 根因 |
|---|--------|------|------|
| 1 | 🔴 MOST | `cmd/atlas/main.go:392` | `eventCalendar := industry.NewEventCalendar()` 從不呼叫 `RefreshEvents`，`events` slice 永遠空 |
| 2 | 🔴 | `internal/eventdriven/predictor.go:38` | `capitalFlow: &staticCF{score: 0}` 從未被 `SetCapitalFlow` 覆寫 |
| 3 | 🔴 | `cmd/atlas/main.go:392` vs `internal/monitoring/dashboard_api.go:350` | 兩個獨立 EventCalendar 實例；MCP 那個沒 wire TWSE provider |
| 4 | 🟠 | `internal/eventdriven/predictor.go:48-92` `Predict()` | 完全忽略 `narrative_get_models` 的 Darwinian weights |
| 5 | 🟡 | `macro_get_capital_flow_latest` 回傳欄位 | `ChangePct` 欄位 0 + timestamp 錯位 |
| 6 | 🟡 | `regime_get_history` 30 筆 | regime score 0 + timezone 混亂 |

### 已驗證的關鍵檔案行號
| 檔案:行 | 內容 |
|---|---|
| `cmd/atlas/main.go:392` | 創建 eventCalendar，需加 `RefreshEvents` + `UpdateFromProvider` |
| `cmd/atlas/main.go:642` | `eventdriven.RegisterRoutes(mux, eventCalendar)`，需加 `SetCapitalFlow` |
| `cmd/atlas/operations_tasks.go:195-211` | `auto_calendar_refresh`，目前只 refresh dashboard instance |
| `internal/eventdriven/predictor.go:38` | `capitalFlow: &staticCF{score: 0}` 預設零值 |
| `internal/eventdriven/predictor.go:48` | `Predict(now)` 函式，需整合 Darwinian weights |
| `internal/industry/event_calendar.go:103` | EventCalendar struct 已實作 RefreshEvents/UpdateFromProvider/GetEventTimeline |
| `internal/industry/event_calendar.go:1087` | `UpdateFromProvider(ctx, provider)` 從外部 provider merge 事件 |
| `internal/marketdata/calendar_provider.go:12` | `CalendarEventProvider` interface + `NewTWSECalendarProvider` 已實作 |
| `internal/monitoring/dashboard_api.go:350` | `newWiredEventCalendar(marketdata.NewTWSECalendarProvider())` — 樣板 |

---

## 二、4-PR 拆分與依賴關係

```
PR#1 (P0)  EventCalendar wire up
   │  統一 instance + RefreshEvents + UpdateFromProvider
   │  → MCP event_calendar 開始有事件
   ▼
PR#2 (P1)  CapitalFlowProvider injection
   │  capitalflow.Service 實作 QualityScore + SetCapitalFlow 注入
   │  → MCP event_flow_prediction direction 開始非 neutral
   ▼
PR#3 (P2)  Darwinian weights integration
   │  Predict() 整合 narrative models + trigger_theme 匹配
   │  → MCP event_flow_prediction driving_events 不再為 null
   ▼
PR#4 (P3)  Data visibility + scheduler + template detector
      change_pct/timezone 修補 + 補排程 + 24 template 觸發偵測器
      → 系統整體健康，可審計因果鏈
```

**依賴嚴格** — PR#2 需要 PR#1 的事件日曆真有資料；PR#3 需要 PR#2 的 capitalFlow 有值；PR#4 是最後的健康檢查。

---

## 三、各 PR 的根因解方（非補丁式）

### PR#1 (P0) — EventCalendar wire up
**根因**：MCP 鏈路使用 main.go:392 創建的「空殼」eventCalendar（沒 refresh 過、沒 wire TWSE provider）；dashboard API 用的是另一個 wired 實例。

**解方**：
1. **統一 single source of truth**：在 `main.go` 創建 eventCalendar 時改用 `newWiredEventCalendar(marketdata.NewTWSECalendarProvider())`，與 dashboard API 共用同一個 factory
2. **公開 factory**：把 `newWiredEventCalendar` 從 `dashboard_api.go` 的 private 提到 `industry` 模組，命名 `NewEventCalendarWithProvider(provider)`
3. **啟動時 sync**：factory 內已呼叫 `RefreshEvents(time.Now())` + async `UpdateFromProvider` — 不變
4. **排程補全**：修改 `operations_tasks.go:auto_calendar_refresh`，改 refresh **這個** wired instance（透過依賴注入，傳入 main.go 那個變數）

**避免的補丁式**：不直接在 `RegisterRoutes` 裡補 `RefreshEvents`（會留下 main.go 的另一個空殼未清理，後續維護時容易遺漏）

### PR#2 (P1) — CapitalFlowProvider injection
**根因**：`eventdriven.NewPredictor()` 預設塞了 `staticCF{score:0, label:"neutral"}`；main.go:642 `eventdriven.RegisterRoutes(mux, eventCalendar)` 從未呼叫 `predictor.SetCapitalFlow(...)`。

**解方**：
1. **介面擴展**：檢查 `internal/capitalflow/service.go` 的 Service 是否已有合適 method；如無，新增 `QualityScore() float64` + `QualityLabel() string`（基於 `ComputeResonance` 的 output 或 `LatestDaily` 的 score）
2. **回傳 predictor**：修改 `RegisterRoutes` 改為 `func RegisterRoutes(mux, cal, cf) (*Predictor, error)` 或新增 `RegisterRoutesWithDeps(mux, cal, cf)` 變體（不改既有簽名）
3. **main.go:642 注入**：傳入 `capitalflow` Service 實例
4. **測試**：寫 `eventdriven/predictor_test.go` 驗證注入的 cf 有正確反映到 predictDay 的 conf

**避免的補丁式**：不在 `Predictor.NewPredictor` 內「猜」quality score（會引入 magic number）

### PR#3 (P2) — Darwinian weights integration
**根因**：`Predictor.Predict()` 完全沒看 narrative models，只看 `calendar.GetEventTimeline` + `cf.QualityScore` 兩個 input。

**解方**：
1. **介面新增**：新增 `NarrativeModelProvider interface { Models() []NarrativeModel }`，讓 eventdriven 透過依賴注入從 narrative 模組取得
2. **theme matching**：對 timeline 內每個 event，根據 `e.Name` 或新增的 `e.TriggerTheme` 欄位（若不存在需新增，stage 1 顯示 calendar events 沒有 trigger_theme），匹配 `NarrativeModel.ActiveThemes`，累加 weighted score
3. **加權合併**：最終 `direction` = sign(net_event_weight + cf_score*0.3 + model_weight)，`confidence` = sigmoid(|net| * driver_count)
4. **回填 trigger_theme**：在 `industry.EventRule` / `CalendarEvent` 加 `TriggerTheme string` 欄位（nullable），讓 event 與 narrative models 對齊

**避免的補丁式**：不在 predictDay 裡寫死 template 對應表（會跟 narrative 模組脫節）

### PR#4 (P3) — Data visibility + scheduler + template detector
**根因**：
- Bug #5：`capitalflow` 計算 `change_pct` 時上游資料 source 的 `ChangePct` 是 0（缺 formula）；timestamp 來自 unix 但 recorded_at 是 wall clock，差 1 個月
- Bug #6：`regime` 寫入 history 時沒帶 score；date 用不同 timezone
- 缺排程：capital_flow daily refresh、regime 週期觸發、template detector

**解方**：
1. **change_pct formula**：在 `internal/capitalflow/forces.go` 補上正確公式（based on `daily_change_pct = (today_value - yesterday_value) / yesterday_value`）
2. **timestamp 統一**：所有寫入 history 的地方都用 UTC；MCP 回傳時 `time.RFC3339` 強制 UTC
3. **補排程**：在 `operations_tasks.go` 新增 3 個 task：
   - `capital_flow_refresh`（每日 16:00 收盤後）
   - `regime_history_refresh`（每 6h）
   - `template_detector_scan`（每 1h，掃描 trigger 條件並發 bus event）
4. **template detector**：在 `internal/narrative/detector.go`（新建）實作 trigger_theme → 6 維度 macro data 偵測邏輯，發出 `EventNarrativeTemplateTriggered` 事件

---

## 四、每 PR 的執行流程（嚴格依照）

1. **跑 atlas-pre-change-protocol 8 步**（Step 0 重疊檢查 → Step 7 設計意圖）
2. **寫 code + 對應測試**（同 package `*_test.go`）
3. **跑驗證清單**：
   ```bash
   test -z "$(gofmt -l .)"
   go vet ./...
   staticcheck ./...
   golangci-lint run --timeout=5m
   go test ./internal/eventdriven/... ./internal/industry/... ./internal/capitalflow/... ./internal/narrative/... ./cmd/atlas/...
   go test -coverprofile=coverage.out ./internal/eventdriven/... && go tool cover -func=coverage.out | grep total  # ≥ 60%
   ```
4. **integration test via MCP**：重啟 server，呼叫 MCP 工具驗證回傳值
5. **commit + push**（每 PR 一個 commit，commit message 寫根因）
6. **下一個 PR**

---

## 五、最終驗證清單（4 PR 全完成後跑）

- [ ] `event_calendar` 回傳 ≥10 個事件
- [ ] `event_flow_prediction` 5 天至少 1 天非 neutral 且 conf > 0.5
- [ ] `event_flow_prediction` driving_events 不為 null
- [ ] `macro_get_capital_flow_latest` change_pct 不全為 0
- [ ] `regime_get_history` 至少 1 個 score != 0
- [ ] `regime_get_history` 所有 date 同時區（UTC）
- [ ] 24 templates 都有對應 trigger detector
- [ ] 3 models 權重分布合理（無極端 1e-4 級別）
- [ ] coverage ≥ 60%
- [ ] golangci-lint 0 issues

---

## 六、rollback 策略

每個 PR 都可以 `git revert <sha>` 撤銷；事件鏈路的根因解方（PR#1 統一 instance）是其他 PR 的基礎，所以 PR#1 若失敗需先解 PR#1 再考慮 abort 後續 PRs。

---

*建立時間：2026-07-12 (Asia/Taipei)*
*負責人：kaecer + opencode Sisyphus*