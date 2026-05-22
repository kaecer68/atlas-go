# Phase 1-10 整合實作方案

> **版本**: v1.0  
> **日期**: 2026-05-22  
> **分支基礎**: `main`（Phase 1-5 已合併）  
> **實作分支**: `feat/portfolio-audit-phase6-10`

---

## 前置分析：Phase 1-5 現況

Phase 1-5 的 13 個檔案、2,579 行已完全合併到 `main`（commit `5b56488`）。

```
已合併項目:
├── Benchmark API (benchmark.go + benchmark_test.go)
├── PnL Attribution API (handlers.go)
├── Correlation Matrix API (risk/handlers.go)
├── PnL 分母修正 (live.go)
├── Cross-Footing (live.go)
├── 產業欄位 (live.go)
├── Component VaR (var_calculator.go)
├── Sortino/Calmar (reporting/performance.go — 獨立合併)
├── 前端 Risk Panel (risk-panel.js)
├── 前端 Attribution (attribution.js)
├── 前端 Benchmark (benchmark.js)
├── 前端整合 (portfolio.js)
└── HTML 容器 (index.html)
```

---

## 一、檔案重疊分析

### 1.1 直接重疊（同一檔案將被兩個 Phase 修改）

| 檔案 | Phase 1-5 變更 | Phase 6-10 變更 | 衝突風險 |
|------|---------------|----------------|---------|
| `internal/monitoring/api/risk/handlers.go` | 新增 correlation matrix API（86 行 diff） | Phase 10：補 leo_satellite 前端名稱字典 | 🟢 低 — 不同函式區塊 |
| `internal/risk/var_calculator.go` | 新增 Component VaR（101 行 diff） | Phase 9：暴露 `CheckVaRLimit()` 方法 | 🟡 中 — 需確保 API 向後相容 |
| `web/static/index.html` | 新增三個面板容器（+6 行） | Phase 9：新增 Risk Gate Panel 容器 | 🟢 低 — 不同 DOM 區塊 |

### 1.2 間接重疊（不同檔案，但共用資料流或模組）

| 關聯 | Phase 1-5 檔案 | Phase 6-10 檔案 | 重疊本質 |
|------|---------------|----------------|---------|
| **組合持倉 → Pipeline** | `live.go`（PnL/CrossFoot/Sector） | `pipeline.go`（讀取 portfolio state） | Pipeline 展示的資料來自 live.go 的計算結果。Phase 1-5 改了 PnL 分母和 CrossFoot 計算 → Phase 6 需要確認 pipeline 讀取的是更新後的語義 |
| **風險面板 → Risk Gate** | `risk-panel.js`（前端風險面板） | `risk-gate-panel.js`（Phase 9 新增風控面板） | 兩個面板可能功能重疊。Phase 1-5 的 risk-panel 顯示 VaR/correlation，Phase 9 的 risk-gate-panel 顯示 mode/events。**需要明確分工** |
| **Benchmark → Backtest** | `benchmark.go`（比較計算） | `backtest.go`（回測結果） | 兩者都計算 performance metrics，可能出現「同一指標兩種算法」 |
| **Attribution → Pipeline** | `attribution.js`（PnL 分解） | `pipeline.js`（推薦管線） | 都在 portfolio 頁面，pipeline.js 顯示 recommendation → attribution.js 顯示 PnL 分解。需確保資料一致性 |
| **live.go → Risk Gate** | `live.go`（PortfolioState） | `gate.go`（PreTradeCheck 用 PortfolioState） | Risk Gate 的 PreTradeCheck 依賴 PortfolioState 結構，Phase 1-5 改了 live.go 的欄位 → Phase 9 的 RiskDecision 需對齊 |

---

## 二、工作衝突分析

### 2.1 🔴 需要立即處理的衝突

#### 衝突 1：`live.go` 的 PnL 分母語義變更

**Phase 1-5 做了什麼**：
- `CumulativePnLPct` 的分母改為 `startingCash`（而非第一場 session 的起始值）
- 新增 `CrossFootPnL` 欄位
- 新增 `Sector` 回填

**Phase 6 需要什麼**：
- Pipeline 讀取 portfolio state 來展示推薦績效
- 若 pipeline 引用的「報酬」語義與 live.go 不一致，會導致同一頁面不同區塊顯示不同的「報酬率」

**處置**：Phase 6 實作前，先確認 `pipeline.go` 讀取 portfolio 資料時使用的報酬計算公式與 live.go 一致。

#### 衝突 2：`risk-panel.js` vs `risk-gate-panel.js` 功能分工

**Phase 1-5 做了什麼**：
- `risk-panel.js`：顯示 VaR、Correlation Matrix、槓桿率

**Phase 9 要做什麼**：
- `risk-gate-panel.js`：顯示風控模式、觸發事件、規則統計

**重疊風險**：
- VaR 同時出現在兩個面板（Phase 1-5 的 risk-panel 顯示 VaR 數值，Phase 9 的 Risk Gate 用 VaR 做閾值檢查）
- 使用者可能困惑「為什麼有兩個風險面板」

**處置**：明確定義分工：
- `risk-panel.js`：**風險度量展示**（被動）：VaR/CVaR/Correlation/HHI/槓桿率
- `risk-gate-panel.js`：**風控狀態與操作**（主動）：模式切換/事件列表/規則觸發統計

### 2.2 🟡 需要協調的潛在衝突

#### 衝突 3：Benchmark API 與 Backtest 的指標雙重計算

**Phase 1-5**：`benchmark.go` 計算 TAIEX/portfolio/outperformance/alpha/beta
**Phase 7**：`backtest.go` 的 backtest runner 也計算 performance metrics

**風險**：兩個模組計算同一指標時可能使用不同公式或資料區間。

**處置**：Phase 7 修改時，將 `backtest.go` 的指標計算指向 `benchmark.go` 的方法，或反過來讓 `benchmark.go` 調用 `reporting/performance.go`。

#### 衝突 4：Pipeline JSONL 解析與 Domain 型別演進

**Phase 1-5**：已修改 `live.go` 的 response 結構（新增 CrossFootPnL、Sector）
**Phase 6**：需要修改 `pipeline.go` 的 JSONL 解析以正確讀取這些新欄位

**風險**：若 pipeline.go 使用的 anonymous struct 未對齊 domain 型別的 JSON tag，會靜默失敗（已知陷阱 AGENTS.md 已警告）。

**處置**：Phase 6 實作時檢查 `loadSessionPipelineData()` 中的 parsing struct 是否涵蓋 Phase 1-5 新增的欄位。

---

## 三、工作前後串連分析

### 3.1 🔴 無法並行的工作鏈

```
Phase 8（控制層閉環）
    ↓ 必須先完成
Phase 9（Risk Gate 需要正確的 Intervention 模型）
    ↓ 必須先完成
Phase 9 PostTradeGate（需要 Risk Gate 基礎）
```

```
Phase 6（Pipeline context wiring）
    ↓ 建議先完成
Phase 10（跨板塊驗證，需 pipeline 正確運作才能驗證資料流）
```

```
Phase 1-5（main 已完成）
    ↓ Phase 6-10 從這個基礎出發
Phase 7（回測修復）
    ↓ 獨立，可與 P6/P8 並行
```

### 3.2 🟢 可並行的工作組

| 並行組 | 任務 | 理由 |
|--------|------|------|
| **組 A** | Phase 6 (pipeline) + Phase 7 (backtest) | 操作不同檔案，無資料流依賴 |
| **組 B** | Phase 8 (control) + Phase 10 config 部分 (leo_satellite) | 獨立檔案，互不影響 |
| **組 C** | Phase 9 PreTradeGate + Phase 10 provider 遷移 | PreTradeGate 不依賴 Gateway 遷移 |

### 3.3 🟡 有依賴但可部分並行

```
Phase 6 (pipeline context wiring) ──→ Phase 10 (跨板塊驗證)
                                    ↘
Phase 8 (control 閉環) ──────────────→ Phase 9 (Risk Gate 整合)
```

---

## 四、整合後的統一實作方案

### 階段 A：奠基（第 1 週）

> **目標**：修復會導致錯誤決策或系統不穩定的關鍵缺陷。本階段所有任務可並行。

| ID | 任務 | Phase | 檔案 | 依賴 |
|----|------|-------|------|------|
| A1 | 修復 pipeline narrative/industry context wiring | P6 | `dashboard_api.go`, `pipeline.go` | main |
| A2 | 修復 pipeline.js URL 組裝 bug | P6 | `pipeline.js:645` | main |
| A3 | 強化 pipeline JSONL 解析（legacy 兼容） | P6 | `pipeline.go` | main |
| A4 | 修復回測 goroutine 洩漏 + status 型別化 | P7 | `backtest.go` | main |
| A5 | 修復 SignalEngine type assertion + 分位數 | P7 | `signals.go` | main |
| A6 | 補齊 leo_satellite 同步（圖譜+前端字典+**注意 conflict #1**） | P10 | `supply_chain_graph.json`, `risk/handlers.go`, frontend dict | main |
| A7 | HumanIntervention 補齊 agent_id 寫入 | P8 | `control/handlers.go`, `control.go` | main |

**檢查點 A**：執行 `go build ./... && go test ./...`，確認無 regression。

### 階段 B：閉環（第 2-3 週）

> **目標**：建立控制層執行閉環 + Risk Gate 基礎。**注意 conflict #2, #3**。

| ID | 任務 | Phase | 檔案 | 依賴 | 注意 |
|----|------|-------|------|------|------|
| B1 | approve/reject 進 execute 路徑 | P8 | `system.go:applyHumanOverrides()` | A7 | 確認與 live.go 的組合持倉狀態連動 |
| B2 | set-model-weight 接入 DarwinianWeightManager | P8 | `system.go`, `portfolio/weight_manager.go` | A7 | |
| B3 | 定義 RiskDecision/RiskAction/Verdict 型別 | P9 | `decision.go`（新） | main | |
| B4 | 實作 PreTradeGate（基礎規則：max position/sector/VaR/cash） | P9 | `pre_trade.go`（新） | B3, **conflict #2** | 需參考 live.go 的 PortfolioState 結構 |
| B5 | 實作 RiskGate 入口 + orchestrator 整合 | P9 | `gate.go`（新）, `system.go` | B3, B4 | |
| B6 | risk-panel.js 與 risk-gate-panel.js 分工明確化 | P9 | `risk-panel.js`, `risk-gate-panel.js`（新） | **conflict #2** | 見 2.2 節的分工定義 |
| B7 | Pipeline 與 live.go 的報酬語義一致性驗證 | P6 | `pipeline.go`, `live.go` | A1, **conflict #1** | 見 2.1 節衝突 1 |
| B8 | Benchmark/Backtest 指標計算統一 | P7 | `backtest.go`, `benchmark.go` | A4, **conflict #3** | 見 2.2 節衝突 3 |

**檢查點 B**：整合測試 — Risk Gate 能攔截超限訂單，approve/reject 影響 pipeline。

### 階段 C：進化（第 4-5 週）

> **目標**：策略自我修正、持續學習、完整閉環。

| ID | 任務 | Phase | 檔案 | 依賴 |
|----|------|-------|------|------|
| C1 | 實作 InTradeGate（止損/trailing stop/vol spike） | P9 | `in_trade.go`（新） | B4, B5 |
| C2 | 實作 PostTradeGate（drawdown/Sharpe/regime） | P9 | `post_trade.go`（新） | B4, B5 |
| C3 | HumanIntervention 審批鏈 + 到期失效 | P8 | `recommendation.go`, `control.go` | B1 |
| C4 | 操作者身份驗證 + RBAC | P8 | `control/handlers.go` | B1 |
| C5 | sector-ban 改用 industry 分類服務 | P8 | `system.go` | B1 |
| C6 | dashboard_api.go provider → Gateway 遷移 | P10 | `dashboard_api.go` | A1 |
| C7 | 參數熱更新 + 變更審批 | P10 | `parameters.go`, `parameters.json` | main |
| C8 | 風控事件回測驗證（effectiveness audit） | P9 | `audit.go`（新）, `gate_test.go` | C1, C2 |
| C9 | report/latest 收斂 canonical source | P7/P10 | `report.go`, `window.go` | A4 |

**檢查點 C**：完整風控閉環 — PreTrade → InTrade → PostTrade → 回測驗證。

### 階段 D：卓越（第 6-8 週）

| ID | 任務 | Phase | 檔案 | 依賴 |
|----|------|-------|------|------|
| D1 | 參數自動調優（Bayesian Optimization） | P9 | `auto_tuner.go`（新） | C8 |
| D2 | 壓力測試框架 | P9 | `stress_test.go`（新） | C2 |
| D3 | 交易成本/滑價/市場衝擊建模 | P7/P9 | `cost_model.go`（新） | A4 |
| D4 | 所有 ticker → BackgroundTaskManager | P10 | 多個檔案 | C6 |
| D5 | 策略自我學習閉環 | P9 | `strategy_learner.go`（新） | C8 |

---

## 五、發現的問題與提示

### 5.1 🔴 必須先解決的架構問題

**問題 1：Phase 1-5 的 live.go 報酬語義變更需要全系統驗證**

`CumulativePnLPct` 的分母從 session 起始值改為 `startingCash`，這個變更影響：
- `portfolio.js`（Phase 1-5 已更新）
- `pipeline.js`（Phase 6 需要更新 — **但可能還沒更新！**）
- `attribution.js`（Phase 1-5 需要確認是否對齊）
- `benchmark.js`（Phase 1-5 需要確認是否對齊）

**行動**：在 Phase 6 開始前，用 `git show 5b56488:internal/monitoring/service/live.go` 確認實際的 PnL 計算公式，然後逐一檢查 `pipeline.js`、`attribution.js`、`benchmark.js` 的前端計算是否一致。

---

**問題 2：Phase 1-5 的 Risk Panel 前端 component 可能與 Phase 9 的 Risk Gate Panel 功能重疊**

目前 `risk-panel.js` 的職責是「風險指標展示」，Phase 9 需要「風控操作面板」。兩個面板可能在同一頁面導致認知混亂。

**行動**：在 Phase 9 開始前，確認：
- `risk-panel.js`：改名為 `risk-metrics-panel.js` 或保留，但明確為「唯讀展示」
- `risk-gate-panel.js`：新建，標題為「風控閘道」，包含模式切換按鈕
- 兩個 panel 在 `index.html` 和 `portfolio.js` 中的 container ID 不衝突

---

**問題 3：Phase 1-5 的 Benchmark API 與 Phase 7 回測的指標計算需要統一權威來源**

`benchmark.go` 和 `backtest.go` 各自計算 performance metrics。應收斂為：
- `internal/reporting/performance.go` 為唯一權威來源
- `benchmark.go` 和 `backtest.go` 都調用 `performance.go` 的方法

**行動**：Phase 7 修改前，確認 `reporting/performance.go` 的 API 是否足夠讓 benchmark 和 backtest 共用。

---

### 5.2 🟡 需要注意的環節

**提示 4：Pipeline JSONL 解析與 Domain 型別演進的同步問題**

Phase 1-5 修改了 `live.go` 的 response 結構，Phase 6 需要修改 `pipeline.go`。根據 AGENTS.md 的已知陷阱：「API handler 讀取 JSONL 時，若 anonymous struct 的 JSON tag 用了 PascalCase 而 JSON 實際是 snake_case，unmarshal 會靜默失敗」。

**行動**：Phase 6 修改 `pipeline.go` 時，使用 `go generate .` 自動生成的 `field_names.js` 來確認欄位名稱一致性。不要手寫 JSON tag。

---

**提示 5：leo_satellite 的同步是一個跨 Phase 的工作**

Phase 1-5 已加入 leo_satellite 到 `sector_symbols.json`（commit `9b53285`），但：
- `supply_chain_graph.json` 缺少對應 node（Phase 10 修復）
- `risk/handlers.go` 前端名稱字典缺少（Phase 10 修復）
- `narrative/knowledge_base.go` 缺少 sector-symbol map（Phase 6 修復）
- `industry.go` 的 `GetActiveNarrativeThemes()` 回傳 nil（Phase 10 修復）

**行動**：這是分散在三個 Phase 的工作，需要確保順序正確：先補 knowledge_base.go（Phase 6）→ 再補 graph.json + 字典（Phase 10）→ 最後補 GetActiveNarrativeThemes()（Phase 10）。

---

**提示 6：`feat/trading-infrastructure-audit` 分支可以廢棄**

Phase 1-5 已完全合併到 main。該分支僅剩一個重複 commit。建議刪除本地分支以避免混淆。

```bash
git branch -D feat/trading-infrastructure-audit  # 本地刪除
```

---

### 5.3 執行優先序建議

```
第 1 週：A1-A7 全部（止血 — 6 個獨立任務，可並行）
第 2 週：B1-B4（Risk Gate 基礎 + 控制層閉環）
第 3 週：B5-B8（整合 + 衝突解決）
第 4 週：C1-C5（InTrade/PostTrade Gate + 審批鏈）
第 5 週：C6-C9（架構遷移 + 回測驗證）
第 6-8 週：D1-D5（卓越功能）
```

---

## 六、需要進一步執行的計劃

### 立即行動（本次對話）

1. ✅ Phase 1-10 整合實作方案（本文檔）
2. ⬜ 確認 `live.go` PnL 計算公式，逐一檢查前端的報酬計算一致性（問題 1）
3. ⬜ 定義 `risk-panel.js` 與 `risk-gate-panel.js` 的明確分工（問題 2）
4. ⬜ 確認 `reporting/performance.go` 的共用 API（問題 3）

### 下次會話

5. 開始 Phase A（止血階段）的實作
6. 每完成一個 Phase，執行 `go build ./... && go test ./...`

### 中期跟進

7. Phase C 完成後，進行一次完整的回測驗證（用歷史數據驗證 Risk Gate 的有效性）
8. Phase D 完成後，在模擬環境中運行 1-2 週，觀察 Risk Gate 的行為是否符合預期

---

> **結論**：Phase 1-5 與 Phase 6-10 之間存在 **3 個直接重疊檔案**、**4 個間接資料流依賴**、**2 個必須先解決的架構問題**。按本文的整合方案執行可避免工作衝突，確保各 Phase 正確串連。
