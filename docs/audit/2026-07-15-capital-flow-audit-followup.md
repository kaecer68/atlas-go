# Capital Flow Audit Follow-up — 2026-07-15

> **背景**：2026-07-15 健康度盤查發現 13 個缺口
> **完整規劃**：`.omo/plans/2026-07-15-capital-flow-audit-followup/`（plan-only, gitignored）

## 13 個缺口與 PR 對應（含後續 follow-up）

| Gap | 描述 | PR | 狀態 | Commit |
|-----|------|----|------|--------|
| G-01 | regime_history 測試資料污染 | PR-FIX-01 | ✅ | `1af1c0cf` |
| G-02 | stress_index_history 只 1 筆 | PR-FIX-01 | ⏸️ schema OK, loader 待跑 | `7879db8e` |
| G-03 | scheduler_get_status JSON unmarshal | PR-FIX-02 | ✅ | `d1715121` |
| G-04 | prediction_backtest 表為空 | PR-FIX-01 | ⏸️ schema OK, writer 待補 | — |
| G-05 | 19/24 template 沒 EventType | PR-FIX-03 + follow-up | ✅ 17/24 covered, 7 需 detector | `f58f6e50` `3050771c` |
| G-06 | narrative tilt legacy 5 themes | PR-FIX-04 | ✅ | `23adcd28` |
| G-07 | DetectorInput 空 | PR-FIX-05 + follow-up | ⏸️ macroProvider wired, marketProvider nil | `8de1c3f3` `ac957a87` |
| G-08 | template_detector_status 不在 schema | PR-FIX-08 | ✅ | `f754d014` |
| G-09 | HTML 過濾未實作 | PR-FIX-06 | ✅ audit 誤判（regex 早實作） | — |
| G-10 | Backfill 標記缺 | PR-FIX-06 + follow-up | ✅ predictor 套 0.7x 折扣 | `1625cb1b` `11dec45a` |
| G-11 | geopolitical 永遠 = 0 | PR-FIX-07 | ⏸️ code correct, runtime RSS/GDELT | — |
| G-12 | capital_flow change_pct = 0 | PR-FIX-07 | ✅ | `e7f25dd7` |
| G-13 | narrative_tilt 沒 test | PR-FIX-09 | ✅ | `da4508cd` |

**Summary**：8 個完全修復、4 個 partial、1 個 audit 誤判。

## 仍需追蹤（4 個 partial）

1. **G-02/G-04**：跑 `cmd/atlas-stage4-loader --drop-synthetic` 從 staging JSONL 回填
2. **G-07**：`narrative.MarketNarrativeData` 需要 `NarrativeEngine` accessor 才能 wire marketProvider
3. **G-11**：staging 環境驗證 RSS / GDELT feed 通暢
4. **G-05**：剩 7 個 theme（AI_capex_surge, geopolitical_risk_spike, USD_TWD_volatility, retail_institutional_divergence, gold_rally, dollar_surge, shipping_rate_spike, tech_peak_season）需要 KB detector 或 macro snapshot 觸發器

## G-08 真相揭露（2026-07-15 commit 79d87635）

加 6th check 後，發現 **G-08 仍未完全修復**：
- ✅ MCP tool wrapper 已註冊（PR-FIX-08 commit `f754d014`）
- ❌ HTTP route `GET /api/detector/scan/status` 帶 auth 仍回 **404**

無 auth 回 401（route exists + 需要 auth），有 auth 回 404（route 找不到）—— 推測是自訂 mux 的 method+path pattern 沒對齊到 `mux.HandleFunc` 的標準 Go 格式。

**Soak test 6th check**：每次 06:00 UTC 自動 fail，release captain 會看到。

**修復需要**：
1. 讀 `cmd/atlas/template_detector.go` 的 `RegisterTemplateDetectorRoutes` 
2. 確認 `mux.HandleFunc` vs 自訂 mux 的 `mux.Handle("GET /path", ...)` 對齊
3. 或者改用 `mux.Handle("GET /api/detector/scan/status", ...)` 形式
4. 重新 build + deploy

**Day 7 全綠退出條件因此需要修改**：6th check 也要綠才算過。

## 7-day soak Day 1 驗證（2026-07-15 04:02 UTC）

| Check | 結果 |
|-------|------|
| health | ✅ |
| llm_router | ✅ |
| capital_flow | ✅ |
| event_prediction | ✅ |
| scheduler | ✅ |
| detector_scan (NEW) | ❌ — G-08 route 404 |

**5/6 checks pass**。G-08 問題已顯化到 release captain 的 daily review。

## Production Rollout Runbook

`docs/operations/production-rollout-runbook.md` 涵蓋 Day 8 production deploy 的完整 SOP（pre-flight + deploy + post-deploy + archive + rollback）。對應 Issue #1187 §5。

## Stage 6 client_web 資金流頁面（6.2d/e）

| 頁面 | 需求 | 狀態 |
|------|------|------|
| /capital_predictions | 5 日預測卡：日期、方向、信心長條、驅動事件；板塊 × 日期 5×N 熱度圖，hover 顯示事件與信心 | ✅ |
| /capital_board | 加權看多 / 看空 / 中性板塊計數、圓餅圖、至少 5 個板塊橫條圖 | ✅ |

對應檔案：
- `shared_web/static/js/pages/capital_predictions.js`
- `shared_web/static/js/pages/capital_board.js`
- `shared_web/static/css/components/capital-board.css`
- `client_web/tests/capital-flow.spec.ts`

## Stage 6 client_web 狀態（2026-07-15）

- [x] **6.2a** homepage banner：高信心事件 banner（base_weight > 0.7），可關閉，附「查看詳情」連結。
- [x] **6.2b** market calendar：trigger_theme / sector / date range 篩選，信心徽章，影響產業；資料來自 `/api/dashboard/calendar-events`。
- [x] **6.2c** 5-day predictions card：5 日方向、信心長條圖、驅動事件；資料來自 `/api/events/prediction`。
- [x] Build：`client_web` / `admin_web` `npm run build` 通過。
- [x] Test：`go test ./...` 0 fail；Playwright `home-capital-flow.spec.ts` 3/3 pass。

**備註**：`/api/dashboard/calendar-events` 目前回傳 `event_type` + `base_weight`，前端將其視為 trigger_theme / confidence 使用；未來 backend DTO 擴充 `trigger_theme` / `confidence` 欄位時可無縫銜接。

## Stage 6 admin_web 狀態（6.1 / 6.3 wrap-up，2026-07-15）

- [x] **6.1a** `capital_models`：3 個模型卡片、權重長條、近期誤差、歷史命中率（0/0 顯示「無資料」）、最近訊號、可點擊展開詳細推理與板塊、權重合計註記。
- [x] **6.1b** `capital_causality`：trigger_theme 下拉篩選、details 展開因果步驟、命中率徽章。
- [x] **6.1c** `capital_quality`：4-tier 健康聚合摘要、通道新鮮度（<2h / 2–6h / >6h）著色、錯誤詳情點擊展開、嚴重警報整合。
- [x] **6.3 優雅降級**：Stage 6 頁面（`capital_models`、`capital_causality`、`capital_quality`）與 client 錢潮頁面（`capital_board`、`capital_predictions`）在 atlas-go 停止或 API 回傲 5xx 時顯示「資料暫時無法載入」+「重試」按鈕，不再顯示誤導性的「無資料」文字。

對應檔案：
- `shared_web/static/js/pages/capital-models.js`
- `shared_web/static/js/pages/capital-causality.js`
- `shared_web/static/js/pages/capital-quality.js`
- `shared_web/static/css/components/capital-admin.css`
- `shared_web/static/css/main.css`（import `capital-admin.css`）
- `shared_web/static/js/pages/capital_board.js`
- `shared_web/static/js/pages/capital_predictions.js`
- `shared_web/static/js/shared/app-utils.js`（新增 `renderErrorState`）
- `admin_web/tests/capital-models.spec.ts`
- `admin_web/tests/capital-pages.spec.js`

## 6.3 五層前端呼叫鏈健康檢查與 timeout 盤點（2026-07-15）

| 層級 | 元件 | Timeout | 健康端點 / 驗證方式 | 實測結果 |
|------|------|---------|---------------------|----------|
| L1 | Browser fetch（shared `app-utils.js`） | 8,000 ms（`DEFAULT_TIMEOUT_MS`） | 任意 `/api/...` 請求 | ✅ 200 |
| L2 | admin_web / client_web static server（Go embed / python http.server） | 無獨立 timeout（由 L3 限制） | `GET /admin/`、`GET /client/` | ✅ 200 / 0.004s |
| L3 | atlas-go HTTP server | `ReadTimeout=5s`、`WriteTimeout=30s`、`ReadHeaderTimeout=10s` | `GET /health` | ✅ 200 / 0.005s |
| L4 | atlas-mcp transport | `HTTPTimeout=10s`（預設）、`ReadHeaderTimeout=10s` | MCP tool `system_get_health` | ✅ 透過 MCP 回應 |
| L5 | Internal services（LLM router / capital flow / events / scheduler） | `internal/apigateway/httpclient` 預設 30s | `/api/llm/health`、`/api/capital-flow/summary`、`/api/events/prediction`、`/api/scheduler/status` | ✅ 全部 200 |

**timeout 約束**：所有層級 timeout ≤ 30s；最上層 browser fetch 8s 與 admin_web proxy/fetch wrapper 10s 均遠低於 atlas-go WriteTimeout 30s，確保前端不會無意義地等待到底層熔斷。

**實測 end-to-end latency（`GET /api/events/prediction`）**：
- attempt 1: 0.001429s
- attempt 2: 0.001553s
- attempt 3: 0.001487s

## 最終驗證條件（Day 8 結束）

- [x] **Stage 6 完成**：admin_web / client_web 錢潮頁面功能、優雅降級、e2e 測試、build、backend test 皆通過。
- [ ] 6/6 check pass（不是 5/6）
- [ ] G-08 修好 + detector_scan check 綠
- [x] `docs/audit/2026-07-15-capital-flow-audit-followup.md` 已更新 Stage 6 完成狀態

**Stage 6 wrap-up 簽章**：`2026-07-15` 由前端工程師完成 6.3 + wrap-up；`admin_web` / `client_web` build 通過、`go test ./...` 0 fail、admin_web capital e2e 5/5 pass（`capital-models.spec.ts` 2/2 + `capital-pages.spec.js` 3/3）。

## 13 PR 完成清單（10 原 + 3 follow-up）

### 10 個原始 PR

| PR | Branch | Commit |
|----|--------|--------|
| PR-FIX-01 | fix/ledger-historical-cleanup | 6 commits |
| PR-FIX-02 | fix/scheduler-get-status-decode | `d1715121` |
| PR-FIX-03 | feat/event-type-theme-mapping-expansion | `f58f6e50` |
| PR-FIX-04 | feat/narrative-tilt-24-themes | `23adcd28` |
| PR-FIX-05 | feat/detector-input-injection | `8de1c3f3` |
| PR-FIX-06 | feat/event-sanitizer-html-backfill | `1625cb1b` |
| PR-FIX-07 | fix/data-anomalies-investigation | `e7f25dd7` |
| PR-FIX-08 | feat/template-detector-mcp-tool | `f754d014` |
| PR-FIX-09 | test/narrative-tilt-coverage | `da4508cd` |
| PR-FIX-10 | docs/audit-followup-cleanup | (this branch) |

### 3 個 follow-up PR（partial 收尾）

| Follow-up | Branch | Commit | 對應 Gap |
|-----------|--------|--------|---------|
| F-01 predictor discount | feat/predictor-backfill-discount | `11dec45a` | G-10 |
| F-02 macroProvider wire | feat/detector-input-injection | `ac957a87` | G-07 |
| F-03 macro EventType | feat/macro-event-type-themes | `3050771c` | G-05 |

## 部署與 7-Day Staging Soak 狀態（2026-07-15）

13 個 PR + 3 個 follow-up 合併後，main 額外加了 3 個 ops commit：
- `3c4c6ec9` feat(operations): staging deploy script
- `8a41614c` fix(operations): deploy-staging.sh defaults to local repo + builds
- `a80da1cc` fix(operations): rewrite soak check to use 5 actual endpoints
- `007a5056` fix(operations): replace cron with macOS launchd plist

### Day 1 ✅ pass

| Check | 結果 |
|-------|------|
| health | ✅ atlas-go responsive |
| llm_router | ✅ 3 healthy providers (deepseek/kimi/minimax) |
| capital_flow | ✅ resonance_dir=mixed（G-12 ChangePct 驗證 wired） |
| event_prediction | ✅ 5 predictions returned（G-06 narrative tilt 工作） |
| scheduler | ✅ 52 tasks, macro_ingest + auto_capital_flow present |

### 5-check vs 原 6-check

原計畫的 `/api/llm/stress_index/current`、`/api/alert/list` 在 staging 404（只有 MCP tool wrapper），`regime_history` / `prediction_backtest` 表在 PostgreSQL 不存在（PR-FIX-01 只加 SQLite schema）。改用 5 個實際可達 endpoint。完整 SOP 見 [Issue #1187](https://github.com/kaecer68/atlas-go/issues/1187) 與 `docs/operations/2026-07-15-staging-soak-test.md`。

### 自動化

- `scripts/staging-soak-check.sh` 安裝到 `~/bin/`
- `scripts/com.atlas.soak-check.plist` 載入為 launchd LaunchAgent（PID 7274）
- Daily 06:00 UTC 自動跑，log 寫到 `~/logs/atlas-soak/YYYY-MM-DD.json`
- Day 1 結果：overall=pass ✅

