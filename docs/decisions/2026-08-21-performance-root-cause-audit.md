# 績效賠錢根因 + 自我進化機制喪失 — 審查報告與系統性改善規劃

> 審查日期：2026-08-20/21 ｜ 審查者：prime-agent（L2）＋ 平行盤查員（MiniMax-M3 / DeepSeek）
> **審計：kimi-k3（k3-256k）— 裁決 PASS WITH CHANGES，本版已依審計必改清單 7 項全部修正**
> 範圍：① 績效賠錢根因 ② 自我進化機制功能喪失 ③ 投資心法憲章比對與策略 A/B 可行性 ④ 數據回填規劃
> 方法：ACI（codegraph/gitnexus）+ 本機 data/state JSONL 統計 + production（iMac）Postgres/API/容器日誌實測 + k3 以 production 即時複核
> ⚠️ 重要提醒：本機 data/state 為 2026-08-14 快照；凡涉及「現況」的結論均以 k3 production 即時複核為準（08-21）

---

## 0. 結論摘要

**賠錢是真實的（歷史模擬組合最深 −65%），主因是「組合管理失效 + 推薦品質差 + 進化機制從未運作」三層疊加：**

1. **組合管理失效（2026-01~04 深度回撤 −65%）**：早期 session 幾乎滿倉（現金僅剩 12.6%）、CRO guard 形同虛設、持倉無下單管理 → 300 萬跌到 **104.6 萬**；5-6 月回升至 361 萬後，7 月初組合被**靜音重置**回 300 萬（無告警機制，6 月已反覆發生 4 次），績效報告只能看到 6 週（+0.1%）。
2. **推薦品質差（production 實測 07-02→08-21）**：勝率 **41.2%**、profit factor **0.28-0.30**、兩 regime aggregate forward return 皆深度負（RISK_ON −5.16 / RISK_OFF −3.45）；主因是低 conviction 推薦（conv<60 勝率僅 30.7%）與無 regime 防護（RISK_OFF 照推 BUY）。
3. **自我進化機制「從未運作過」**：全歷史 **0 筆實驗晉升**、baseline v19 凍結 4 個月。歷史根因是 **Zero-Signal Trap**（screening 太嚴導致 19/20 agent 終身 0 訊號）——**該問題已於 2026-08-15（commit e22e07ad FIX-2）修復並部署**，production 現已恢復訊號流（詳 §2.1）；但進化迴圈仍卡在 auto_experiment 數據斷糧（20 連敗）與「從未走完一次晉升」。
4. **憲章未落實到決策路徑**：七時期（PeriodDetector）、macroflow 風險調整、時期→策略過濾（Advisor）、時期現金——**全部未接進模擬/回測/即時管線**；「非憲章路線」是現行唯一真實路線。
5. **最系統性的根因（k3 審計新增）**：整條進化失效鏈 **4 個月無人發現**——系統沒有「24h 無進化活動即告警」的健康監控；修復所有失效點前，必須先補可觀測性，否則會再盲。

---

## 1. 績效賠錢根因（三層證據）

### 1.1 組合層級：2026-01~04 深度回撤 −65%（本地 session_summaries 重算，k3 逐月複核）

| 月份 | portfolio_value 末值 | 說明 |
|---|---|---|
| 2026-01 | 2,176,400（min 1,577,623） | 300 萬起步即虧 |
| 2026-02 | 1,383,615 | 續跌 |
| 2026-03 | 1,694,729 | 反彈後再跌 |
| 2026-04 | **1,046,224（−65.1%）** | 最深回撤 |
| 2026-05 | ~2,999,000 | 回升至近平盤 |
| 2026-06 | 3,615,123 → 2,684,959 | 先 +20% 後單日 −316,609（06-27） |
| 2026-07 | 3,000,000（靜音重置） | 歷史被抹除（6 月已反覆重置 4 次） |

**根因（session 細節 + 源碼實測）**：
- 早期 session（2026-01）：首日建倉買入 5 檔（orders=5）後**零下單管理**（無停損/無再平衡），現金僅剩 12.6%（ending_cash=377,017）→ 違反憲章「保守者存活」C5/C15
- `guard_outcomes`：CRO hard gate「passed without filtering」（01-01: 23→23）→ 4 月才開始過濾；CIO soft gate 同期有實際過濾（14/21/17 筆）→「形同虛設」成立於 CRO、不涵蓋 CIO
- **🔴 靜音重置機制（k3 審計發現）**：`system_risk_session.go:136-139` 在 simulation_state 檔案缺失時，**無告警、無日誌、無人工確認**直接 `NewSimulationState(3M)` 重建；6 月至少 4 次 pinned-3M 重置（06-11/17/22/29）→ 這是「半年歷史從 production 視野消失」的機制層根因，且未來必然再發

### 1.2 交易/推薦層級：品質差（production 績效報告實測 2026-07-02→08-21）

| 指標 | 數值 |
|---|---|
| win_rate | **41.2%**（多數交易虧損） |
| profit_factor | **0.28-0.30**（贏小輸大） |
| aggregate_forward_return | RISK_ON **−5.16** / RISK_OFF **−3.45**（兩 regime 皆深度負） |
| total_trades | 1408（real 828 + synthetic 580，合成佔 41%） |
| total_return（組合權益） | +0.10%（因部位極小被稀釋） |

**根因（真實樣本拆解，k3 複核）**：
- **低 conviction = 主要虧損源**：conv<50 勝率 16.4%、conv<60 勝率 30.7%、平均 −0.59%；conv≥80 勝率 65.7% → conviction_floor=35 過低
- **無 regime 防護**：2026-07 全月 RISK_OFF，引擎照推 BUY（低 conviction 佔 36.6%）
- **production execution_policy 實測**：`conviction_floor: 35`、`momentum_crash_protection: false`、`enable_conviction_normalization: false`

### 1.3 數據層級：證據斷鏈（k3 修正缺口方向）

- **樣本污染**：本機 recommendation_outcomes 45,661 筆 **56% is_synthetic**（2026-06 達 77%）；production 合成交易 41%
- **資料中斷**：本機資料止於 2026-07-16；production replay 最後 08-19，`twse_replay` channel error（另有 twse-etf / twse-oddlot circuit breaker / auto_cycle_update 也在 error——k3 補充）
- **績效報告只覆蓋 6 週**：PG session_summaries 僅 07-02 起（48 筆）；本地 2026-01-01→06-30 共 **127 份**歷史 summary 未入 PG → 半年歷史對 production 不可見
- **雙寫分裂（k3 修正方向）**：PG 有 07-02→07-23 共 22 天而檔案系統沒有（缺口方向與初版相反）；檔案系統現有 07-24→08-21 連續
- **schema drift**：experiment 結果檔 `notes` string vs []string（production 14 檔中 1 檔實測）→ parse_experiment_file_failed

---

## 2. 自我進化機制喪失（P0~P3 失效點）

### 2.1 🟠 P0：Zero-Signal Trap — **已於 2026-08-15 修復**（歷史根因，非現況）

- **歷史根因**：screening `momentum_20d_min`（threshold 0.00）在下跌/盤整市場拒絕 9/9 候選（08-07~08-13 本地 trace 一致）→ 19/20 agent `total_signals=0` → Darwinian 無法學習 → auto_propose 4 個 trigger（需 signals≥30/60）全不命中
- **✅ 已修復**：commit **e22e07ad**（2026-08-15，FIX-2「momentum_20d_min 0.0→-0.5」）將 agents.json 的 min 改為 −0.5 並部署。k3 production 即時複核：session-20260820/21 trace **10 recs/day**（修復前 0-2）、production darwinian 已有 **16 個 agent 具 296-1101 signals**
- **⚠️ 後續風險（k3 審計新增）**：新訊號品質未驗證——production darwinian 出現 rolling_sharpe=**20.141/24.298** 等離群值（正常應在 ±3 內），疑 forward return 計算或年化有單位 bug；在錯誤訊號上強化懲罰會放大錯誤

### 2.2 🔴 P1 嚴重：排程數據斷糧（k3 即時複核：20 連敗）

| 任務 | 連敗 | 錯誤 |
|---|---|---|
| `auto_experiment` | **20 天** | 「replay 數據最後日期為 X，但實驗窗口結束於 X+1（缺少 1 天數據）」— validation.go:44 |
| `risk_gate_calibrate` | **20 天** | "self_calibrate: no sessions available" |
| `seasonal_calibration` | **20 天** | "exit status 1" |
| `cron_replay_sync` | **3 次** | "exit code 1"（上次成功 2026-08-17） |

### 2.3 🟠 P1：實驗消化停滯（k3 修正「死鎖」定性）

- **0 晉升全歷史**：本地 experiments.jsonl 1,178 記錄（fold 後 612 unique ID）＝ planned 90 / expired 507 / running 15 / **promoted 0 / accepted 0 / reverted 0**；production PG 四表（experiment_lineage / prompt_experiment_results / baseline_history / spawn_records）**全 0 筆**
- **backlog gate 並未誤觸發（k3 修正）**：`CountUnresolvedPlanned` 按 experiment ID **fold（last-write-wins）**計數，實際 90（08-14 快照）/ 31（現況）< 100 門檻；`ExpireStalePlanned` **已每日自動執行**（auto.go:41，TTL 30 天）。初版「656 planned 永久死鎖」為誤算（656 是含 expired 的歷史記錄數）
- **真正卡點**：15 筆 stuck running（production 現況 13 筆，最老 29 天）＋ auto_propose 每日 50ms 完成但 0 輸出（無法區分 backlog_skip 與無 trigger 命中——無結構化日誌）

### 2.4 🟠 P2：晉升/回滾從未執行過

- baseline_policy **v19 自 2026-04-25 凍結**，promotions 18 筆全在 4 月（含 1 筆 E2E 測試 revert）；prompt_overrides 凍在 4 月版

### 2.5 🟡 P3：周邊停擺

- PRISM training：29 queued / **0 completed**（phase3_metrics.json，最後更新 2026-07-06）；`ml_retrain`：**enabled=False**
- `maturity_tracker.json`：凍在 2026-06-01（`Save()` 無 production 呼叫）
- MacBook 雙機同步：**7 天未同步**（data/state 停在 08-14）→ 這是本報告初版把已修復問題誤判為現況的原因

### 2.6 進化迴圈完整死因鏈（含修復狀態）

```
【歷史】screening momentum_20d_min 太嚴（下跌市 9/9 拒絕）
  → 19/20 agent 0 signals → Darwinian 無輸入 → 權重凍 default
  → auto_propose 4 trigger 全不命中 → 0 新提案
  → auto_experiment 又因 replay 缺 1 天連敗 20 天（現況）
  → 0 judge → 0 promote → baseline v19 凍結 4 個月 → prompt_overrides 凍在 4 月版
【現況】✅ 08-15 已修 screening（e22e07ad）→ 訊號恢復（16 agent）
  → ⚠️ 但 auto_experiment 仍 20 連敗（replay 斷糧）、訊號品質未驗證（sharpe 離群）
  → ⚠️ 且無「24h 無進化活動即告警」→ 4 個月失明期可能重演
```

---

## 3. 投資心法憲章比對 + 策略 A/B 可行性（ds-charter-ab 盤查 + k3 源碼逐行複核）

### 3.1 憲章在決策路徑幾乎沒有落地

審計文件（ATLAS_CONSTITUTION_AUDIT）宣稱 22/22 ✅，但 k3 源碼逐行複核確認：**七時期（PeriodDetector）、macroflow 風險調整、時期→策略過濾（Advisor）、時期現金、RegimeAllocator——全部沒有接進模擬/回測/即時交易決策管線**（`ExecutionContext.PeriodDetector` 全 repo 無非測試賦值、`macroflow.NewEngine` 零呼叫、collectRecommendations 僅按 Layer 過濾）。**現行「非憲章路線」是唯一真實路線**：3 態 regime + 靜態 10% 現金 + 靜態 conviction 60。

### 3.2 關鍵偏離點（28 條規範 C1-C28 嚴重違反摘錄）

| # | 憲章規範 | 狀態 | 證據 |
|---|---------|------|------|
| C5/C15 | 保守者存活：時期現金（低迷 45%/黑天鵝 90%） | ❌ sim 恆用靜態 10% | parameters.json reserve_cash_fraction=0.1 |
| C10 | 七時期判斷（PeriodDetector） | ❌ 決策路徑從不呼叫 | executor_pipeline.go:91 條件永假 |
| C14 | 時期→策略過濾（AllowedStrategies） | ❌ 收集階段無時期過濾 | executor_collection.go:30-36 |
| C17 | macroflow 動態風險調整 | ❌ NewEngine 零呼叫 | executor_pipeline.go:98 永假 |
| C16 | 事件套利風控（<5日/曝險<10%） | ❌ 只在 YAML 註記無 consumer | methodology_rules.yaml L466-467 |
| C18 | 壓力指數閾值（>60 不進場） | ⚠️ 只顯示層 | stress_index.go |
| — | momentum crash protection（VIX 熔斷） | ❌ policy=false + replay 無 VIX | executor_momentum_crash.go:7-14 |
| — | 動態風險閾值 DynamicThresholdEngine | ❌ GetThreshold 零非測試呼叫 | sim/dynamic_threshold.go |

### 3.3 A/B 比較可行性：**部分可行（架構可行、數據不足）**

- ✅ **可行**：cmd/backtest-window 決定性重播；cmd/experimental/janus-backtest 雙 arm 範式（同窗口、雙 LedgerDir、RunMetrics 比較，k3 實測存在）；接線僅 1-3 檔 + `Engine.CharterMode` flag；可觀測差異點明確（Raw/Final recommendations）
- ❌ **數據不足**：7 時期 golden 84/85 天 **69 天 fallback consolidation**（batch1 則為 85 天中 83 Unknown）；replay 44 標的無 macro 標的（VIX/DXY/US10Y/SOX/TSM ADR）→ 回測中 macro layer 與 momentum crash 失效；sessions 僅 7.5 個月 → 7 時期分層需 ≥24 個月
- **A/B 設計**：stepwise 增量開關（PeriodOnly → +StrategyFilter → +MacroFlow → +CashReserve → +ConvictionFloor）；驗收 = 機制驗證 + 績效對比（p<0.05）+ 非退化 + 可追溯；⚠️ 2026-07 RISK_OFF 段是唯一密集對照段，樣本集中度高，解讀需防過度推論；兩 arm 共用 sim 停損/稅務（隔離變數優點，但「非憲章 arm 非無風控」須註明）

### 3.4 策略進化機制（業主問題的直接答案）

現有進化元件（Darwinian、JANUS、experiment A/B、StrategyEvolver）以 3 態 regime + 績效回饋運作，**無憲章時期約束**。
憲章 A/B 的價值：用雙 arm 差異定位「哪些憲章約束有增量、哪些退化」，把憲章從「宣稱」變成「可驗證的策略演化方向」——
每次 stepwise 結果寫回 baseline 政策，即為可運作的策略進化機制。**歷史數據可用**（sessions 165 天 + replay 2024-07 起），
但需先補 macro 標的與 7 時期輸入歷史（§5 R1-R4）。

---

## 4. 系統性改善規劃（三階段，k3 審計修正版）

### Phase A — 立即止血（1 週，不改架構）
| # | 行動 | 對應根因 |
|---|------|---------|
| A1 | 補跑 daily-replay-sync + 修 twse_replay/twse-etf/twse-oddlot circuit breaker/auto_cycle_update + replay 新鮮度告警 | 數據斷糧（2.2） |
| A2 | 修 experiment 解析 bug（notes 型別相容，production 已實測 1/14 檔） | schema drift（1.3） |
| A3 | execution policy：conviction_floor 35→60+、momentum_crash_protection=true、conviction_normalization=true | 推薦品質（1.2） |
| A4 | **驗收已部署的 screening 修復（e22e07ad）**：追蹤 signals→outcomes→Darwinian 權重是否真實演化；**調查 rolling_sharpe 20.14/24.3 離群值（疑 forward return 單位 bug）**；screening threshold 納入變更稽核 | P0 後續風險（2.1） |
| A5 | 清 13 筆 stuck running（production 現況）；auto_propose 的 skip 原因加結構化日誌（區分 backlog_skip vs 無 trigger） | 消化停滯（2.3） |
| A6 | RISK_OFF 期間停推/減倉 gate（conviction≥70 才進） | 無 regime 防護（1.2） |
| A7 | 績效報告去污染：區分 real/synthetic | 樣本污染（1.3） |
| A8 | 歷史 session 遷移（2026-01~06 共 127 份 summary 入 PG） | 績效只覆蓋 6 週（1.3） |
| **A9** | **🔴 靜音重置防護（k3 新增）**：`ensurePersistentStateLoaded` 重建 simulation_state 時必須告警 + 記錄 reset 事件 + 可選人工確認（system_risk_session.go 加 ~5 行） | 靜音重置（1.1） |

### Phase B — 進化迴圈修復（2-4 週）
| # | 行動 | 對應根因 |
|---|------|---------|
| B1 | auto_experiment 窗口 gate：replay 即時同步；缺 ≤1 天時順延而非失敗 | 迴圈斷裂（2.2） |
| B2 | auto_propose trigger 重設計：0 訊號但 weight 偏離 default>30% 也觸發（backlog 部分刪除——fold 語意已確認無死鎖） | 無觸發（2.1） |
| B3 | **先驗證訊號正確性再強化懲罰**（sharpe 離群調查）；Darwinian 懲罰強化 + 零訊號 agent 處理 + 權重變化告警 | 訊號品質（2.1） |
| **B4** | **🔴 evolution 健康告警（k3 新增）**：每 24h 檢查 proposal/judge/promote/revert 是否有活動，無則告警——「4 個月無人發現」的系統性病根 | 可觀測性（2.6） |
| B5 | 重啟 PRISM（29 佇列 0 完成）與 ml_retrain（enabled=False），先評估再啟用 | 停擺（2.5） |
| B6 | session summary PG/檔案雙寫一致性修復（含 PG 缺 07-02→07-23 檔案段） | 雙寫分裂（1.3） |
| B7 | maturity_tracker Save() 補 production 呼叫；雙機同步排程化 | 周邊停擺（2.5） |

### Phase C — 憲章 A/B 與策略進化機制（4-8 週）
| # | 行動 | 依賴 |
|---|------|------|
| C1 | 依 R1-R10 回填（§5） | 數據 |
| **C2** | **接線 CharterMode（進入條件：7 時期 golden ≥70% 非 fallback — k3 提升為硬性 gate）**：PeriodDetector/MacroFlow/Advisor/時期現金進 ExecutionContext | C1 達標 |
| C3 | stepwise 增量 A/B（2026-01→08 窗口先行），仿 janus-backtest 雙 arm | C2 |
| C4 | A/B 結果 → 有效憲章約束寫回 baseline → 進化迴圈以真實數據閉環 | C3 |

---

## 5. 數據回填規劃（R1-R10，ds-charter-ab 完整清單）

| 序 | 回填項目 | 用途 | 來源 | 優先序 |
|----|---------|------|------|-------|
| R1 | replay 加 macro 標的（VIX/DXY/US10Y/SOX/TSM ADR/NVDA） | 回測 macro layer/momentum crash/7 時期單日指標生效 | Yahoo/Fugle/FinMind 既有管道 | 🔴 最高 |
| R2 | 外資期貨未平倉歷史（每日 ≥20 日滾動） | 轉折/上升期判定（FutP/FutD） | taifex channel 擴充 | 🔴 高 |
| R3 | 融資餘額長歷史（2024-07 起） | 低迷/轉折判斷（MPk/MC5） | FinMind TWN-FIIT/TEJ | 🔴 高 |
| R4 | 集中市場成交量長歷史 | 盤整判斷（VMA20） | 既有量能管道 | 🔴 高 |
| R5 | 公股券商買賣超長歷史（⚠️ 依賴 CAPTCHA 解鎖，時程風險） | 低迷判斷（PBBuy） | govflow 管道 | 🟠 中高 |
| R6 | 44 標的全期覆蓋（2024-07 起） | 早期窗口可交易宇宙 | TWSE open data/FinMind | 🟠 中 |
| R7 | 事件日曆長歷史（ETF/MSCI/除權息） | 事件套利 arm（C4/C16） | event_calendar 擴充 | 🟠 中 |
| R8 | 國安基金事件旗標歷史 | 黑天鵝判定 | 人工/新聞表 | 🟡 低 |
| R9 | period_history/regime_history 正式落庫 | 兩 arm 時期分層驗證 | persistPeriodHistory 排程 | 🟠 中 |
| R10 | FinMind 2020-2024 轉格式合併 | ≥24 個月長回測（7 時期分層檢定） | convert-* 工具 | 🟠 中（C3 前） |

**里程碑**：Phase 1（1-2 週）R1-R4 → golden 7 時期 ≥70%（**C2 硬性進入條件**）→ Phase 2 接線 + stepwise A/B → Phase 3 R6+R10 長回測分層檢定。
**原則**：先回填再測試評估（業主指示）；回填後以 Phase C A/B 作為第一個驗證案例。

---

## 6. k3 審計紀錄與修訂對照

> 審計員：kimi-k3（k3-256k）｜ 裁決：**PASS WITH CHANGES**（核心結論全部重現成功；production 即時複核 20+ 主張中 20 項吻合）

| # | 審計意見 | 處理 | 本版狀態 |
|---|---|---|---|
| 1 | P0 Zero-Signal Trap 已於 08-15（e22e07ad）修復，勿當現況；A4 改驗收+訊號品質稽核 | §2.1/§2.6/A4 重寫 | ✅ |
| 2 | 「656 planned 永久死鎖」誤算（fold 後 90/31 < 100；ExpireStalePlanned 已自動跑）；A5/B2 前提修正 | §2.3/A5/B2 重寫 | ✅ |
| 3 | 雙寫缺口方向相反（PG 有 07-02→07-23 檔缺）；歷史 summary 改 127 份 | §1.3 修正 | ✅ |
| 4 | 新增：靜音重置防護（system_risk_session.go 無告警重建）→ Phase A9 | §1.1/A9 新增 | ✅ |
| 5 | 新增：evolution 健康告警（24h 無活動即告警）→ Phase B4 | §2.6/B4 新增 | ✅ |
| 6 | 新增：新訊號品質驗證（sharpe 20.14 離群）→ A4/B3 | §2.1/B3 修正 | ✅ |
| 7 | 修辭：equity_curve 1 點、首日 orders=5、CIO 有過濾、golden batch1 Unknown、channel error 補齊；C2 加硬性 gate | 全文修正 | ✅ |

---

## 7. 附錄：關鍵證據索引

| 證據 | 位置 |
|---|---|
| 歷史組合 −65%（session summaries，k3 逐月複核） | data/state/sessions/*/summary.json（§1.1） |
| 靜音重置機制（無告警重建 3M） | internal/orchestrator/system_risk_session.go:136-139（§1.1） |
| production 績效報告（win 41%/PF 0.28） | iMac :18080 /api/dashboard/performance-report（§1.2） |
| execution_policy（conviction 35 / crash off） | data/state/baseline_policy.json（§1.2） |
| 56% synthetic / 07-16 中斷 | data/state/recommendation_outcomes.jsonl（§1.3） |
| 0 晉升 / 四表 0 筆 | data/state/experiments.jsonl + iMac PG（§2.3） |
| auto_experiment 20 連敗 | iMac task_liveness（§2.2） |
| Zero-Signal Trap 修復（momentum 0.0→-0.5） | git commit e22e07ad（2026-08-15）（§2.1） |
| baseline v19 凍結 4 個月 | data/state/baseline_policy.json（§2.4） |
| PRISM 29/0、ml_retrain off | data/state/phase3_metrics.json（§2.5） |
| 憲章偏離（C10/C14/C17 未接線，k3 源碼逐行複核） | internal/orchestrator/executor_pipeline.go 等（§3.2） |
| 7 時期 golden 69/84 fallback | data/golden/b5-batch2-backtest.txt（§3.3） |
