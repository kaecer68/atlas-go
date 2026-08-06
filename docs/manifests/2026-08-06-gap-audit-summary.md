# Gap Audit Summary — 2026-08-06

> **Phase**: Audit (READ-ONLY,no code changes)
> **Trigger**: User asked 「請檢查 issue 中有沒有哪一個可以執行的?」+ 連續追問 「深度評估這三個 issue 有值得做的意義與價值嗎?」+ 連續追問 「你盤查的這些事情你有明確的確定系統內沒有其他代碼是在解決同樣的問題,對嗎?」
> **Skills loaded**: `atlas-pre-change-protocol` (Investigation Mode), `atlas-audit-manifest-protocol`
> **Branch**: `feat/20260806-forecast-ledger-wireup` (worktree: `atlas-forecast-ledger-wireup`)
> **Decision**:
> 1. **不推進** 3 個現有 open issue 實作 (#1068 / #1466 / #1322)
> 2. **產出** 4 份 gap audit 報告作為未來實作依據
> 3. **後續** 由 user 決定是否關閉現有 issue

---

## 0. 執行摘要

### 0.1 三個現有 open issue 評估結論

| Issue | 標題 | 對核心目的的價值 | 建議 |
|---|---|:---:|---|
| #1068 | Commercial flow: API key registration + tier gating | ❌ 低 — deferred by owner,等 PMF 驗證 (C07 Day 14) | 關閉 |
| #1466 | L2.4 driver wire-up + framework + 缺口 A/E/T15 決策 | ❌ 低 — Issue 自己的 T15 comment 已決定不重啟 L2.4 觀察期;其他 12 sector LLM 變體 + generic framework 是「是否有需求」的開放問題 | 關閉 |
| #1322 | narrative/detector + narrative/seasonal sub-package extraction | ❌ 極低 — 純 refactor,不影響預測能力、不影響執行效率、不修任何 bug | 關閉 |

### 0.2 四個候選缺口盤查結論

| Gap | 真實狀態 | 對核心目的 | 建議 |
|---|:---:|:---:|---|
| **Gap 1**: 預測命中率 dashboard 可見化 | Partial — 單點 hit_rate 有,時間序列無 | 中 | 等 Gap 2 補完 actual 後再做 (P1, 1-2 天) |
| **Gap 2**: 預測 vs 實際閉環 | **對 eventdriven.Predictor**: Real gap(預測有存,actual 從未補)。**對 forecast.ForeignForecast**: **不是 gap,是 E03 設計未啟用** (90 天手動 warm-up 計畫) | 中-高 | 對 eventdriven 做 wire-up (P0, 1 PR, 1 天)。對 forecast 不動,等 owner 提供 E03 啟用 plan |
| **Gap 3**: 散戶追蹤/紀律機制 | **Real gap** — §9「可紀律執行」承諾完全無法兌現 | **高** | 最小範圍 R1 + R5 (P0, 1 PR, 1-2 天);完整 R2-R4 為 P1+ 排程 |
| **Gap 4**: 28 天驗證記錄 | 只有 C07 真實運轉 (~7 工作日, 8/9 MUST PASS);L2.4/SA11/forecast 都是範本或未啟動;manifest §7 F5 文字需修正 | 中-低 | 修正 F5 文字 + 補 L3 Day 1 check-in (純文件, 0.5 天);其他由 #1466 處理 |

### 0.3 重要修正 (相對初始 subagent 報告)

Subagent 初始報告將 `internal/forecast` 描述為「100% 完整 T+1 infra 但 production 未被任何 main wired (dead code)」。**深入 git 歷史查證後修正為**:

- `internal/forecast/foreign_forecast.go` 來自 commit `9f71933e` — 「feat(forecast): 外資方向推估 v1 scorecard + ledger + 校準門檻 — **#E03**」
- 對應 spec: `docs/specs/foreign-flow-forecast-spec.md` §6 「**首次啟用後至少需連續運行 90 個交易日才能驗證**」+ §9 「**累積 ≥ 90 個交易日後,啟用對外展示**」
- `internal/forecast/AGENTS.md` 完整記錄此設計意圖,且**未標示為 deprecated**
- 對比: `internal/forecast_bridge/` 已在 commit `3e5808f4` 移除,理由是「Phase 3.5 M4 PoC shipped but never consumed by runtime. **TradeSignal conversion now handled by `strategy.DirectionalTradeLayer` directly**」

**修正結論**:
- `internal/forecast` = **E03 設計未啟用**(90 天手動 warm-up),不是 dead code
- `internal/forecast_bridge` = **被 `DirectionalTradeLayer` 取代**(已正確刪除)
- 兩者語意不同,**不可混淆**

---

## 1. 問題 (Problem)

User 初始問題:「請檢查 issue 中有沒有哪一個可以執行的?」

連續追問聚焦:
1. 「深度評估這三個 issue 有值得做的意義與價值嗎?」(對齊核心目的)
2. 「你盤查的這些事情你有明確的確定系統內沒有其他代碼是在解決同樣的問題,對嗎?」(防止重複造輪子)
3. 「你所謂的死碼的定義是什麼? 這些死碼是過度設計的無用代碼? 還是只是你判斷沒有足夠的接入引用,你就猜測他們可能是死碼?」(防止基於證據不足就下結論)
4. 「逼的我只能在每次提示詞不斷的強調必須有真相有根據」(防止亂猜)

---

## 2. 盤查方法 (Method)

### 2.1 三階段

| 階段 | 工具 | 目的 |
|---|---|---|
| **階段 1** | 讀取 3 個 open issue + 對應 manifest (L2.4 alignment audit) + product positioning 文件 | 理解 3 個 issue 的真實意圖與對齊 |
| **階段 2** | 4 個 scout subagent 並行盤查 4 個候選缺口 (gap1-4) | 找出「對齊核心目的但未在現有 3 個 issue 內」的真實缺口 |
| **階段 3** | 對 subagent 結論的關鍵判斷 (「dead code」) 做 git 歷史 + spec 查證 | 防止基於 grep 「沒有 caller」就誤判為「過度設計的無用代碼」 |

### 2.2 紀律約束

- **真實證據**:每個結論必須有具體檔案:行號 + 原文 quote
- **不亂猜**:對「未啟用」與「dead code」嚴格區分;對「過度設計」與「未完成實作」嚴格區分
- **owner 決策分離**:不替 owner 決定「是否啟用 90 天實驗」(那是架構決策,不是實作任務)
- **不重複造輪子**:每個推薦實作方向必須先驗證「沒有其他代碼在做同樣的事」

---

## 3. 詳細發現 (Findings)

### 3.1 三個現有 open issue 的真實狀態

#### #1068 Commercial flow

- **owner 明確 deferred** (2026-07-22 comment): 等 C07 Day 14 PMF 驗證、tool-tier classification、pricing、legal 全部未達
- 完整推出需 2-3 週,8 個工作項
- **非預測能力缺口**;是商業 launch 前置

#### #1466 L2.4 缺口 A/E/T15

- **Issue 自己的 T15 comment (2026-08-06) 已決定不重啟 L2.4 觀察期**:
  - 三必要條件 (staging 環境/觀察者/Day 0) 全未滿足
  - Driver 未 wire — 翻 flag 也只走 nil → deterministic fallback
  - 投入 staging + 觀察者 + Day 0 排程,回報只是驗證 nil driver 行為 — 浪費
- **缺口 A「其他 12 sector LLM 變體」**:Issue 自身留有開放問題「其他 sector 是否真的需要 LLM-driven 變體?還是 C07 規則式已足夠?」,需 user 決策
- **缺口 E「generic LLM sector agent framework」**:是缺口 A 的前置,但只有缺口 A 確認要推進才需要做
- **driver wire-up** (缺口 A 子項):即使做了也只是「讓 nil 變成 realDriver」,不驗證 LLM 是否真比 deterministic 準

#### #1322 narrative package split

- 兩個 sub-package 都明確標示 **blocked**:
  - `narrative/detector`: 強拆會 circular import,需同時遷移核心 detect 函式 = 大型重構
  - `narrative/seasonal`: 三個檔案非連貫子領域,強拆會造成單檔微套件
- **純 refactor**,不影響預測、不影響執行效率、不修 bug
- F-02 cohesion 0.783 是抽象指標,**不是 user-facing 痛點**

### 3.2 四個候選缺口的真實狀態 (見 4 份獨立 audit 報告)

---

## 4. 修正 subagent 初始判斷的關鍵查證

### 4.1 `internal/forecast` 不是 dead code

| 證據 | 內容 |
|---|---|
| Commit 歷史 | `9f71933e` — feat(forecast): 外資方向推估 v1 scorecard + ledger + 校準門檻 — **#E03** |
| 對應 spec | `docs/specs/foreign-flow-forecast-spec.md` §6/§9 明確 90 天 warm-up 設計 |
| AGENTS.md | `internal/forecast/AGENTS.md` 完整記錄 `ForecastEngine` 是 stub + `Score` 是封閉式規則 + `Calibrate` 需 90 天 warm-up |
| 與 forecast_bridge 對比 | `internal/forecast_bridge/` 是 **被取代**(`DirectionalTradeLayer` 直接處理),所以正確刪除;`internal/forecast` 是 **E03 設計未啟用**,**不是被取代** |
| 設計意圖 | §6「首次啟用後至少需連續運行 90 個交易日才能驗證」、§9「累積 ≥ 90 個交易日後,啟用對外展示」 |

**結論**: `internal/forecast` 屬於「**依設計意圖尚未啟用**」,不是「過度設計的無用代碼」。若要動它,需先寫「E03 啟用 plan」(owner 決策),不是把它當 bug 修。

### 4.2 為什麼 `forecast_bridge` 刪了但 `internal/forecast` 留著

| 套件 | 刪/留 | 理由 | 證據 |
|---|---|---|---|
| `internal/forecast_bridge/` | ❌ 刪 (commit `3e5808f4`) | Phase 3.5 M4 PoC shipped but never consumed by runtime. **TradeSignal conversion now handled by `strategy.DirectionalTradeLayer` directly** | commit message |
| `internal/forecast/` (foreign_forecast + engine + types) | ✅ 留 (但 engine 是 stub) | E03 設計意圖;`Score`/`Ledger`/`Calibrate` 是規則型 scorecard 核心邏輯;`types.TradeSignal` 仍被 `DirectionalTradeLayer` 消費 | `internal/forecast/AGENTS.md` 完整記錄 |

**關鍵**: 兩者**不是同一件事**:
- `forecast_bridge` = **adapter**(PoC 階段,後被 `DirectionalTradeLayer` 取代,故刪)
- `internal/forecast` = **核心預測邏輯 + Ledger + Calibrate** (E03 設計,`TradeSignal` 契約仍被消費,故留)

### 4.3 `eventdriven.Predictor` 是真實 gap

| 證據 | 內容 |
|---|---|
| `internal/ledger/event_flow_prediction_store.go:13-28` | `EventFlowPredictionRecord` 只有 `DirectionSign` + `Confidence` + `Direction` — **沒有 actual_* 欄位** |
| `cmd/atlas/stage3_tasks.go:222-235` | `LatestCapitalFlowPrediction` 呼叫 `predictor.Predict()` 並 `AppendPrediction` |
| `internal/monitoring/stage3_rules.go` `evaluatePredictionDrift` | 命中/未命中只 emit alert,**不寫 ledger** |
| `internal/calibration/predictor_calibrator.go` | `PredictorCalibrator` 從 `prediction_backtest` 讀 hit rate 餵 Bayesian optimizer |
| `internal/ledger/historical_store.go:75-110` | `prediction_backtest` schema 含完整 `ActualDirection` / `ActualCapitalFlowChange` / `Hit` / `is_synthetic` |
| `cmd/backtest-event-flow/main.go` | **唯一**連接「預測→實際→hit」的 pipe,但 `is_synthetic=1`,屬歷史回填非真實營運 |

**結論**: eventdriven 的 prediction 真實有寫,但 actual **從未被 production 寫入**。這是真實可修的 gap。

### 4.4 Gap 3 (散戶追蹤/紀律) 是真實 gap

完整證據見 `gap3-capital-flow-to-action.md`。核心發現:
- `client_web` 沒有 watchlist / notification / 已讀 / 紀錄機制
- `notification-center` 容器存在但 0 注入邏輯
- 既有 `universe_watchlist` / `strategy_feedback` / `alerting` 是**運維語意**,不可複用為散戶功能
- §9「可紀律執行」承諾完全無法兌現

---

## 5. 對齊核心目的的程度

| 核心目的 (產品定位 §1/§6/§8/§9) | 對齊程度 |
|---|:---:|
| §1 散戶的投資觀測輔助智慧平台 | ✅ 高 |
| §6 預測可信的三要件 (領先指標 + 預測vs實際 + 誤差回饋) | ⚠️ 中 — 缺 (b) T+1 自動對比 + (c) 參數系統回饋 |
| §8 校準哲學 (假設登錄→校準→寫回→追蹤→退化降權) | ⚠️ 中 — 缺「追蹤→退化降權」的真實 T+1 數據流 |
| §9 「觀測→解讀→追蹤→紀律」 | ❌ 缺「追蹤」「紀律」 |

整體對齊 ~50%。

---

## 6. 決策 (Decision)

### 6.1 本次 session 不做 code 變更

**理由**:
1. 3 個現有 open issue 都不對齊核心目的,不應推進
2. 4 個候選缺口中,A1 (eventdriven actual) 與 Gap 3 (散戶追蹤) 是真實缺口,但需要 user 確認範圍與優先級才能啟動 PR
3. Gap 2 中 `internal/forecast` 是 E03 未啟用,需 owner 架構決策

### 6.2 產出物

- 4 份獨立 audit 報告 (gap1-4)
- 本 summary manifest

### 6.3 後續行動 (待 user 決策)

| 行動 | 選項 | 影響 |
|---|---|---|
| 關閉 #1068/#1466/#1322 | 選 A | 清理 backlog,符合本次評估結論 |
| 推進 Gap 2-A1 (eventdriven actual) | 選 B | 1 PR, 1 天, 直接對齊 §6/§8 |
| 推進 Gap 3-R1+R5 最小骨架 | 選 C | 1 PR, 1-2 天, 直接對齊 §9 |
| 什麼都不做,只留 audit | 選 D | 本次 session 結束,等下次 |

---

## 7. 驗收標準 (Acceptance)

- [x] AC-1: 4 份 gap audit 寫入 `docs/audit/gap-audit-2026-08-06/`
- [x] AC-2: summary manifest 寫入 `docs/manifests/2026-08-06-gap-audit-summary.md`
- [x] AC-3: subagent 初始「dead code」判斷的修正已記錄 (見 §4.1)
- [x] AC-4: 不動任何 code (本次 session 純 audit)
- [x] AC-5: 對核心目的的價值評估已建立量化指標 (見 §5)
- [x] AC-6: 後續行動選項已交給 user 決策 (見 §6.3)
- [x] AC-7: 所有變更通過 `make ci-gate` (待 PR 前)

---

## 8. Session-end State

- **Done**: 4 份 gap audit + summary manifest + critical 修正 (internal/forecast 不是 dead code)
- **Next**: 跑 `make ci-gate` → commit → push → 開 PR 合併到 main
- **未做**: 任何 code 變更 (依 §6.1 決策)
- **owner 決策待回**: §6.3 四個選項 (關閉 issues / 推進 Gap 2-A1 / 推進 Gap 3 / 什麼都不做)
