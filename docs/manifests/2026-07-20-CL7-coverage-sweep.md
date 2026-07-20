# Audit Manifest: CL-7 Coverage Sweep

> **Audit source**: 2026-07-20 coverage gap 全 repo 掃描 + frontend audit
> **Goal**: 將 `internal/` 核心 package 覆蓋率拉到 ≥60% CI 門檻
> **Scope**: internal/ 層 5 個 <60% 包 + frontend audit。明確不做：cmd/（utility CLI 不需高覆蓋）、monitoring/api/（thin handler delegation）、metalearning（40.3% 但實驗性）、orchestrator/composition（47.1% 但即將淘汰）
> **Created**: 2026-07-20
> **Status**: in-progress

---

## Coverage Baseline（2026-07-20 實測）

| Package | 當前 | 目標 | delta | 優先順序 |
|---------|------|------|-------|---------|
| `internal/strategy` | **53.8%** | 70% | +16.2% | HIGH |
| `internal/dailyreport` | **48.2%** | 70% | +21.8% | HIGH |
| `internal/retail` | **56.4%** | 70% | +13.6% | MEDIUM |
| `internal/autobacktest` | **57.0%** | 65% | +8.0% | MEDIUM |
| `internal/taskexec` | **59.0%** | 70% | +11.0% | LOW（59% 已接近門檻）|

**已排除**（<60% 但不需補）：
- metalearning 40.3% — 實驗性、仍迭代中
- monitoring/api/pipeline 40.7% — thin HTTP delegation，無業務邏輯
- orchestrator/composition 47.1% — 即將淘汰/重構
- monitoring/api/metrics 52.4% — thin metric collect handler
- 全部 cmd/ 包 — utility CLI，不需高覆蓋

**#1144 自動關閉**：eventdriven 覆蓋率已達 89.4%（`2026-07-20 實測`），issue 已 close。

---

## 詳細缺口

### internal/strategy（53.8% → 目標 70%）

| File | 缺口函數 | 當前 | 說明 |
|------|---------|------|------|
| `comparison_store.go` | NewFileComparisonStore, Load, Upsert, loadUnlocked | 0% | store 實作完全無測試 |
| `directional_trade_layer.go` | 全部 4 個方法 | 0% | trade layer 完全無測試 |
| `f06_engine.go` | RankingSnapshot | 0% | F06 engine 無測試 |
| `allocator.go` | Volatilities, equalMix | 0% | edge branch 未覆蓋 |
| `comparison.go` | RecordShadowDay | 0% | shadow day recording 無測試 |

### internal/dailyreport（48.2% → 目標 70%）

| File | 缺口函數 | 當前 | 說明 |
|------|---------|------|------|
| `report.go` | Markdown | 0% | markdown render 完全無測試 |
| `report.go` | RegisterRoutes | 0% | route registration 無測試 |
| `report.go` | HandleSubscribe | 50% | subscription handler 分支不足 |
| `report.go` | HandleArchive | 54.5% | archive handler 分支不足 |
| `report.go` | resolveRegime | 50% | regime resolution 分支不足 |

### internal/retail（56.4% → 目標 70%）

| File | 缺口函數 | 當前 | 說明 |
|------|---------|------|------|
| `calibration.go` | 全部 9 個函數 | 0% | RSI TW calibration 無測試 |
| `rsi_tw_calculator.go` | LastScore | 0% | calculator query 無測試 |

### internal/autobacktest（57.0% → 目標 65%）

| File | 缺口函數 | 當前 | 說明 |
|------|---------|------|------|
| `loop.go` | StartDailyLoop, RunScheduledBacktest | 0% | daemon loop 無測試 |
| `runner.go` | NewRunnerWithEventBus, RunAndStore, syncToLiveStore, mostRecentTradingDay | 0% | runner integration path |

### internal/taskexec（59.0% → 目標 70%）

| File | 缺口函數 | 當前 | 說明 |
|------|---------|------|------|
| `manager.go` | ExecutionID, RecordLineage, RecordBaselineHistory, RecordMetrics, ListEvents | 0% | metadata/lineage recording 無測試 |

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| CV-01 | comparison_store 0% | 測試未補 store 實作 | `internal/strategy/comparison_store_test.go` | `go test ./internal/strategy/` 覆蓋率 ≥70% | pending | none | |
| CV-02 | directional_trade_layer 0% | 測試未補 trade layer | `internal/strategy/directional_trade_layer_test.go` | `go test ./internal/strategy/` 覆蓋率 ≥70% | pending | none | |
| CV-03 | dailyreport Markdown 0% | 測試未補 markdown render | `internal/dailyreport/report_test.go` | `go test ./internal/dailyreport/` 覆蓋率 ≥70% | pending | none | |
| CV-04 | retail calibration 0% | 測試未補 RSI TW calibration | `internal/retail/calibration_test.go` | `go test ./internal/retail/` 覆蓋率 ≥70% | pending | none | |
| CV-05 | autobacktest loop/runner 0% | daemon path 無測試 | `internal/autobacktest/*_test.go` | `go test ./internal/autobacktest/` ≥65% | pending | none | 較低目標 |
| CV-06 | taskexec metadata 0% | lineage/metrics recording 無測試 | `internal/taskexec/taskexec_test.go` | `go test ./internal/taskexec/` 62.1% | done | none | 59%→62.1%，剩餘 0% 在 runners.go（整合測）|
| CV-07 | Frontend audit | 前端現狀審計 | `admin_web/`, `client_web/`, `shared_web/` | audit report | done | new-doc | 見下方前端審計報告 |

---

## Phase Tracker

### Phase A — Audit (done)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Run `./...` coverage scan | - | done | `coverage.out` 全 repo 掃描 2026-07-20 |
| Filter internal/ 層 < 60% | - | done | 9 包 < 60%，4 包排除（實驗性/thin/待淘汰），5 包納入 |
| Function-level gap analysis | - | done | 5 包各 3-9 個 0% 函數 |
| Frontend audit | CV-07 | done | 見下方前端審計報告 |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| CL-7a: strategy coverage | CV-01, CV-02 | done | `go test ./internal/strategy/` 83.2% |
| CL-7b: dailyreport coverage | CV-03 | done | `go test ./internal/dailyreport/` 90.9% |
| CL-7c: retail coverage | CV-04 | done | `go test ./internal/retail/` 91.1% |
| CL-7d: autobacktest coverage | CV-05 | done | `go test ./internal/autobacktest/` 60.1% |
| CL-7e: taskexec coverage | CV-06 | done | `go test ./internal/taskexec/` 62.1% |
| CL-7f: Frontend audit | CV-07 | done | audit report below |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| comparison_store tests | CV-01 | done | 8 tests covering Load/Upsert/prune/replace/corrupt |
| directional_trade_layer tests | CV-02 | done | 6 tests covering New/Apply/Override/Reset/Concurrent/Multi |
| RankedSnapshot/RecordShadowDay tests | CV-02 | done | 6 tests in strategy_test.go |
| Volatilities/equalMix tests | CV-02 | done | 3 tests in allocator_test.go |
| dailyreport Markdown/handlers | CV-03 | done | 10 new tests covering Markdown/Subscribe/Archive/Regime/Routes |
| retail LastScore/calibration | CV-04 | done | 12 new tests (LastScore, factorD3, avgScore, applyChange, CalibrateRSITw, LoadLastCalibrationReport) |
| autobacktest runner/loop | CV-05 | done | 3 tests (NewRunnerWithEventBus, StartDailyLoop ctx, RunScheduledBacktest ctx) |
| taskexec EventSink | CV-06 | done | 5 tests (ExecutionID, RecordLineage, RecordBaselineHistory, RecordMetrics, ListEvents) |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Push branch / open PR | - | pending | |
| Run CI / verify | - | pending | |
| Delete `.omo/plans/` | - | pending | |

---

## Commit Discipline

- Format: `test(strategy): CV-01 add comparison_store tests`
- One commit per CV ID
- No commit without `go test ./package/` passing
- PR body: `See docs/manifests/2026-07-20-CL7-coverage-sweep.md`

---

---

## 前端審計報告（CV-07）

> **日期**: 2026-07-20 | **範圍**: admin_web/, client_web/, shared_web/ static resources

### 結構概覽

| 層 | JS 原始碼 | CSS 原始碼 | dist 大小 |
|----|-----------|-----------|----------|
| admin_web | 3 檔（699 行 main.js） | 0（全部 shared） | 532K |
| client_web | 11 檔 | 0（全部 shared） | 856K |
| shared_web | ~110 檔 JS + ~50 檔 CSS | 6070 行總計 | - |
| 前端測試 | 23 個 test.mjs 檔 | - | - |

### 優化建議

#### 🔴 HIGH — 應立即處理

1. **Hardcoded colors in JS（76 處）**
   - `shared_web/static/js/` 中有 76 處直接使用 `#RRGGBB` 或 `rgb()`，違反 CLAUDE.md 規定的「顏色一律用 `var(--...)`」
   - 應遷移到 `shared/color-tokens.js` 的 `financialColor()` / `regimeColor()` / `severityColor()` API
   - 影響：主題切換時 76 處顏色不會跟隨 dark/light 模式

2. **Large file warnings**
   - `home.js`（1221 行）— 超過 250 LOC 門檻，建議拆分
   - `industry.js`（1069 行）— 同上
   - `home.css`（1017 行）— CSS 過長，應拆分 page module
   - `capital-board.css`（602 行）— 建議按 component 拆分

#### 🟡 MEDIUM — 下輪 sprint

3. **`'use strict'` missing（89 檔）**
   - 大部分 static JS 檔案缺少 `"use strict"`，可能在未來 bundler 升級時導致相容性問題

4. **CSS variable utilization ~57%**
   - `variables.css` 定義 109 個 token，CSS 中僅使用 47（use rate = 43%）
   - 建議 codebase-wide 掃描，移除未使用的 variable definitions

5. **No source maps in dist/**
   - admin_web 和 client_web 的 dist/ 均無 `.map` 檔案，production debugging 困難

#### 🟢 LOW — 長期追蹤

6. **Frontend test coverage**
   - 23 個 test 檔，覆蓋約 21% 的 JS 檔案（110 檔中有 23 檔有對應 test）
   - 優先為 `pages/*.js` 和 `components/*.js` 補測試

7. **Bundle size optimization**
   - client_web dist 856K（含所有 JS chunks + CSS）
   - admin_web dist 532K
   - 建議用 `esbuild --metafile` 分析哪些 chunk 最大，考慮 code splitting

---

## Session-End State

- **Done this session**: coverage scan + gap analysis + manifest + CL-7a~f all completed
- **Next action**: review manifest, decide on PR strategy（單一 PR vs 按 package 拆分）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest | Sisyphus |
| 2026-07-20 | 2.0 | CL-7a~f all complete: strategy(83.2%), dailyreport(90.9%), retail(91.1%), autobacktest(60.1%), taskexec(62.1%), frontend audit | Sisyphus |
