# Audit Manifest: 散戶定位與錢潮鏈路修復

> **Audit source**: 業主定位校準審計（2026-07-17）
> **Goal**: 修復 `docs/audit/2026-07-17-retail-positioning-gap-audit.md` 盤出的全部缺口，讓系統從「展示給人看」進化到「餵給策略用」，並讓散戶在網頁上得到錢潮、策略競爭與白話解釋。
> **Scope**: 審計報告 P0/P1 全部列入執行輪次；P2 以「評估/設計」任務列入（需業主決策後才進實作）。明確不做：實盤下單、新增付費資料源採購（僅產評估文件）。
> **Created**: 2026-07-17
> **Status**: in-progress

---

## 執行策略總覽

| 輪次 | 主題 | 涵蓋線 | 預估 |
|------|------|--------|------|
| 第〇輪 | 定位仲裁文件（先於一切設計變更） | 線 I | 0.5 天（需業主審閱） |
| 第一輪 | 止血：MCP/API 契約 + 前端壞出口 + 資料口徑 | 線 A、線 C、B04-B07 | 3-5 天 |
| 第二輪 | 自癒可觀測 + 進化迴路疏通 | 線 B 其餘、線 D | 3-5 天 |
| 第三輪 | 錢潮因果鏈第一哩：外資期貨 OI + 外資推估 | 線 E | 1-2 週 |
| 第四輪 | 預測閉環 + 策略層接通 | 線 F、線 G | 2-4 週 |
| 第五輪 | 散戶體驗 | 線 H | 2-4 週（H05/H06 視設計結果） |

**依賴關係**：E02 依賴 E01；E03 依賴 E01 的歷史回填；F03 依賴 F02；F06 依賴 D01（排名才有意義）；E05 依賴 I01 的業主裁決。

---

## Invariant Tracker

### 線 A：MCP/API 契約修復（止血）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | `strategy_ranker` MCP 呼叫噴 unmarshal array→map | **accepted**：後端回 bare array，MCP 用 `*map[string]any` 解；測試 mock 成物件助長漏修 | `cmd/atlas-mcp/server/tools_strategy_ranker.go:20`、`tools_strategy.go:35-37`、`tools_strategy_ranker_test.go:11` | 活體 `atlas-mcp_strategy_ranker` 回排名清單無錯；測試 mock 改 array；`go test ./cmd/atlas-mcp/...` PASS | done | none | 與 d1cab67d 修的 scheduler 同模式；審計 §P0-1 |
| A02 | 同模式可能還有漏網（synergy trend、experiment history、parameters snapshots、metrics trend、narrative 系列） | hypothesis：凡 `*map[string]any` decode 的 handler 都可能錯配 | `cmd/atlas-mcp/server/` 全面 grep | 產掃描清單（tool × 後端實際回應形狀）；錯配全修；每個修正附 array mock 測試 | done | none | Agent F §1 未逐一驗證，本 ID 即補驗證 |
| A03 | `trace_get_reasoning` 永遠 400 | **accepted**：tool 無 session_id 參數，後端必填 | `cmd/atlas-mcp/server/tools_llm_trace.go:101-109`、`internal/monitoring/api/pipeline/reasoning_handler.go:43` | tool 加 optional session_id（缺省=最新場次）；無參呼叫回 200 | done | tool-catalog 描述同步 | 審計 §P0-2 |
| A04 | `mcp_quickstart` 子請求全吞錯，agent 無法區分「無資料」vs「後端故障」 | **accepted**：`tools_briefing.go:29-33` 全 `_ =` | `cmd/atlas-mcp/server/tools_briefing.go` | 各區塊附 `degraded`/`error` 標記；部分失敗時回明確狀態 | done | none | 審計 §P0-10 |
| A05 | `system_get_health` 回傳 status 恆空字串 | **accepted**：handler 讀 `raw["status"]`，後端 `SystemHealthResponse` 無此欄位 | `cmd/atlas-mcp/server/tools.go:274` | status 由 channels/warnings 推導（ok/degraded）或明確移除欄位；活體驗證 | done | none | Agent F §1.3 |
| A06 | `task_get_events` 打 SSE endpoint 無法 JSON decode | **accepted**：後端 `text/event-stream`，HttpClient 不支援（d1cab67d 自承 known broken） | `cmd/atlas-mcp/server/tools_task*.go`、`internal/monitoring/api/taskexec/handlers.go:163-196` | 二選一：後端加 JSON variant（建議）或 MCP 端解析 SSE；tool 回事件清單 | done | none | 審計 §P0-10 相關 |

### 線 B：資料正確性與自癒可觀測

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| B01 | autobacktest 每小時空轉洗掉 consecutive_failures，且無 last_error | **accepted**：空轉 `return nil` 也 `RecordSuccess` | `internal/scheduler/background.go:307-325` | no-op tick 記 skipped 不記 success；status API 暴露 last_error；單測覆蓋 | done | none | 審計 §P0-3 |
| B02 | overlap 守門毫秒級誤殺（每天唯一 13:30 有效 tick 可能被吃） | **accepted**：`time.Since(lastRun) < interval` 邊界判斷 | `internal/scheduler/background.go:277` | 每日窗口型任務改以「最後成功執行日」判斷或加容忍區間；活體 log 不再見 autobacktest_daily 被誤殺 | done | none | Agent F §2.2 |
| B03 | replay 停滯時回測靜默 no-op；last_auto_date 無 staleness 告警 | **accepted**：`runner.go:50-53` skip 無事件；health 無此檢查項 | `internal/autobacktest/runner.go`、`internal/monitoring/service/system.go` | backtest staleness >2 個交易日出現在 `system_get_health` warnings | done | none | 審計 §P0-3 |
| B04 | Darwinian history 寫 CWD 相對路徑，trend API 讀停更 7 週舊檔 | **accepted**：`composition.go:237-238` 相對路徑 vs `pipeline.go:914` 絕對路徑 | `internal/orchestrator/composition.go` | history 統一寫入注入的 state dir；`synergy_get_darwinian_trend` 回今日資料；三份 history 收斂（舊檔遷移或刪除） | done | none | 審計 §P0-4 |
| B05 | replay 缺資料時 synthetic outcomes 餵達爾文權重 | **accepted**：`RecordOutcome` 不區分 `IsSynthetic` | `internal/orchestrator/system.go:532-548,783-826`、`internal/portfolio/darwinian_weights.go:265` | synthetic outcomes 不進權重調整（或權重 0 且 ledger 標記）；replay 缺口日達爾文權重不變 | done | none | 審計 §P0-6 |
| B06 | `daily_report` 與 `capital_flow_summary` 同日口径矛盾；global.summary「偏寬鬆」與 status RISK_OFF 矛盾 | hypothesis：兩端點走不同 provider/快照時間，summary 文案與 status 分屬不同生成邏輯 | `internal/dailyreport/report.go`、`cmd/atlas/main.go`（wiring） | daily_report capital 區塊與 capital_flow_summary 同源同值；summary 文案與 status 由同一輸入產生；活體對照一致 | done | none | 審計 §P0-7（2026-07-16 活體實測） |
| B07 | `get_recommendations` free tier 回 `stress_index_unavailable; capital_flow_unavailable` 且零推薦 | hypothesis：recommender 的 provider 未接線或讀不同 state 路徑 | `internal/recommender/handler.go`、`cmd/atlas/wire_recommender.go:58` | free tier 回與 `taiwan_stress_index`/`capital_flow_summary` 一致數值；warning 消失 | done | none | 審計 §P0-8（活體實測） |
| B08 | macro-ingest 部分失敗被 mask 成 ok | **accepted**：`cmd/macro-ingest/main.go:73-78` INTENTIONAL STUB | `cmd/macro-ingest/main.go`、健康記錄 | 子 provider 失敗記 degraded 並指明哪個；channel health 反映真實狀態 | done | none | Agent B §三 |
| B09 | `auto_cycle_update` 連續失敗 2、`auto_experiment` 失敗 1、`calibration_cycle` 停用且 last_run 0001-01-01 | hypothesis：待查（可能資料相依或註冊順序） | 依調查結果 | 失敗任務修復或明確棄用移除；`scheduler_get_status` 無異常紅字 | done | none | 審計 §六 |
| B10 | daily_report on-demand 才生成、archive-state 手動 | **accepted**：無排程 | `internal/dailyreport/report.go:263-266`、scheduler 註冊 | 每日收盤後排程生成日報；archive-state 納入排程（或明確文件化為人工操作） | done | none | Agent F §7 |

### 線 C：前端壞出口（止血）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| C01 | 首頁「查看決策鏈」按鈕跳 404 | **accepted**：`switchPage('decision-chain')` 不在 `SHELL_LOADERS` | `shared_web/static/js/pages/home.js:1036`、`client_web/static/js/main.js`（接現有 `pages/decision-chain.js`） | 按鈕可達決策鏈頁且載入資料；Playwright 回歸 | done | none | 審計 §P0-9 |
| C02 | portfolio 空狀態按鈕無效 | **accepted**：`data-nav` vs 事件委託只認 `a[data-page]`；且目標頁 client 不存在 | `shared_web/static/js/pages/portfolio.js:21`、`event-listeners.js:29` | 按鈕可導航到有效頁（或移除）；Playwright 回歸 | done | none | Agent E §7 |
| C03 | 散戶版 HTML 殘留 admin modal（提示詞比對/晉升 Baseline） | **accepted**：`client_web/static/index.html:166-181` 複製遺留 | `client_web/static/index.html` | 移除；grep client_web 無 baseline_policy/prompt-diff 殘留 | done | none | 審計 §P0-9 |
| C04 | pipeline 場次下拉永遠「載入場次中…」 | **accepted**：`window.pipelineSessions` 只有 client 無處觸發的 reasoning-trace.js 會寫 | `shared_web/static/js/pages/pipeline.js`、`components/reasoning-trace.js` | pipeline 頁自行 fetch 場次清單；下拉顯示真實場次 | done | none | Agent E §7 |
| C05 | premium「立即升級」只跳 alert('開發中') | — | `shared_web/static/js/page-shells/premium.js:62` | 業主裁決：「即將推出 + 留 Email」等候名單 | done | none | c29e50e8；POST /api/waitlist + WaitlistStore（JSONL） |
| C06 | `llms.txt` 過時（79-81 vs 實際 110 tools） | **accepted**：手寫未同步 | `client_web/static/llms.txt` | 更新至 110；長期改由註冊表生成（Backlog） | done | none | Agent E §6 |
| C07 | tool-catalog 稱 experiment_judge 為 LLM 評審（實為統計式 replay judge） | **accepted**：文件失真 | `docs/reference/tool-catalog.md:99` | 描述改為統計式 17-gate judge；與 `internal/experiment/judge.go` 一致 | done | 文件修正 | Agent D §B11 |

### 線 D：進化迴路疏通

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| D01 | 628 筆實驗積壓 planned（每週僅消化 1，無 cap/expire） | **accepted**：產生速度 > 消化速度且無上限 | `internal/experiment/auto.go:39-46`、ledger、（消化策略：批次 expire 過舊 + 速率對齊） | 積壓 <50 且機制上不再無限累積；ledger 有 expire 記錄 | done | none | 審計 §P0-5 |
| D02 | AutoProposer（退化自動提議）從未接線 | **accepted**：僅測試使用，`cmd/atlas` 無 `NewAutoProposer` | `cmd/atlas/main.go`、`internal/experiment/auto_proposer.go` | Sharpe 退化事件自動產生候選並入 ledger；單測/整合測試 | done | none | Agent D §B4 |
| D03 | 晉升時 pre-promotion Sharpe 恆記 0.0 → AutoRollback 退化偵測失效 | **accepted**：`auto_judge_promoter.go:195-198` TODO | `internal/experiment/auto_judge_promoter.go`、`internal/scheduler/auto_rollback.go` | 晉升記錄真實 pre-promotion Sharpe；rollback 退化偵測有測試覆蓋 | done | none | 審計 §P0-5 |

### 線 E：錢潮因果鏈第一哩（P1 核心）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| E01 | 外資台指期未平倉零覆蓋（外資最領先公開訊號） | — | 新增 `internal/marketdata/taifex_oi_provider.go`、`internal/apigateway/register_adapters.go`、排程（收盤後 15:30）、歷史回填 | 每日盤後自動抓外資/投信/自營台指期淨多空；channel health 可觀測；`data_get_channels` 可見 | done | data-sources.md 更新（待補） | bc35701e；端點 MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate，篩 ContractCode=臺股期貨；90 天回填列 BK-12 |
| E02 | Futures force 是回傳 0 的 placeholder | **accepted**：依 E05 §7 重構降維 | `internal/capitalflow/forces.go`（§7 後不再為獨立 force，改掛在 foreign.LeadingZ/LeadingTrend） | foreign score 必帶 futures OI 領先訊號；共振係數規則不變 | done | none | 844ba81f（與 E05 同 commit） |
| E03 | 「外部因子→外資」傳導模型不存在（因果鏈第一哩） | — | 新增 `internal/forecast/foreign_forecast.go`（scorecard + ledger + 校準門檻）；spec 文件 | 設計文件 + 透明加權 scorecard（7 特徵）；Ledger 寫讀；§6 門檻（≥90 日 + ≥55% 命中率）；未達回「校準中」 | done | 新 spec | 9f71933e；拒黑盒 ML，遵守 §8 |
| E04 | Government force 恆 0 且方法論未定義 | — | 設計文件 + v1 檔案型 provider + channel | docs/specs/government-force-proxy.md；v1 採「操作員匯入」誠實模式；缺檔時 force 仍存在但 DataAvailable=false | done | 新設計文件 | 7e228cb1；分點加總自動化列 BK-13 |
| E05 | 七大勢力分類混雜主體與代理（TSM ADR 拿 30% 權重、外資 0%） | **accepted**：依 §7 重構 | `internal/capitalflow/types.go`、`forces.go`、`resonance.go` | 5 subject + 1 leading_indicator（futures, deprecated）+ 1 sentiment（tsm_adr, deprecated）；共振只認 subject；force 帶 Role/Deprecated 旗標；government 帶 DataAvailable 旗標；foreign 帶 LeadingZ/LeadingTrend | done | I01 定位文件 | 844ba81f；向後相容（API 仍回 7 筆） |
| E06 | 七維錢潮分類研究與唯一正本缺失 | accepted：七項資料必須採 3+2+2 分層，不能平權也不能刪訊號 | `docs/specs/capital-flow-seven-dimension-spec.md` | 第一方來源、repository evidence、資料字典、研究衝突規則與 CF-INV 完整 | done | canonical spec | 79277a53 |
| E07 | 七維 backend 仍缺 provenance、分層 assessment，legacy weight/quality 語意不一致 | accepted：E05 只加 role/deprecated，report/service/UI 尚未完整遷移 | `internal/capitalflow/{types,forces,resonance,report,service,assessment}.go` | 3+2+2 assessment；七筆相容；無跨單位權重；automation 僅吃 eligible assessment | done | canonical spec link | commit ccd4e721 |
| E08 | 網頁/MCP/活躍文件與 runtime binary 無法對齊 main 語意 | accepted：活體缺 role 且 aligned 含 ADR；system health 無 commit | UI、MCP descriptions、system health、build flags、active docs | UI 分層；MCP/docs 同義；runtime commit 可查並與部署對帳 | done | active docs sync | commit 2603710b |
| BK-15 | ForceExtractor rolling window process-local，API 讀取會重複 push 同日資料 | accepted：Extract 每次呼叫都 push，daily/summary 同 snapshot 分數不同 | `internal/capitalflow/{rolling_store,forces,service}.go`、production wiring | 交易日去重持久化；restart 恢復；read-only API；missing 不補 0 | done | canonical spec §8 | 本輪拉入，依 backlog rule 僅拉此一項；commit 4eae84e9 |

### 線 F：預測閉環與策略層接通

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| F01 | drift alert 拿 [0,1] confidence 比 [-3,3] QualityScore | **accepted**：單位/語意不同 | `internal/monitoring/stage3_rules.go:199-240`、`cmd/atlas/stage3_tasks.go:216-246` | 統一為方向命中率比較（雙方轉方向）；測試覆蓋 | pending | none | 審計 §P1-3 |
| F02 | prediction_backtest 只跑過 synthetic（真實資料 blocked） | **accepted**：loader 未對真實 staging 執行（G-04 partial） | `cmd/atlas-stage4-loader`、`cmd/backtest-event-flow` | prediction_backtest 表有真實 90 天資料；命中率可查 | pending | none | 2026-07-15 審計 G-04 延續 |
| F03 | predictor 規則參數（0.3 tilt/門檻/BaseWeight）無自動校準 | — | `internal/eventdriven/predictor.go`、校準任務接 `auto_calibrate` 機制 | 誤差回饋校準寫回 parameters.json + audit log；校準後命中率不劣化才生效 | pending | parameter-system.md 更新 | 依賴 F02 |
| F04 | 錢潮預測不進策略層（orchestrator 不讀 eventdriven） | **accepted**：消費者只有 UI/MCP/recommender | `internal/orchestrator/`（接入點設計先行） | 模擬管線以 feature-flag 讀取預測作為方向輸入；關閉時行為不變；A/B 對照報告 | pending | 設計文件 | 審計 §P1-4 |
| F05 | sector_rotator 未接 WeightEngine（TODO 註解） | **accepted**：`portfolio/sector_rotator.go:40` | `internal/portfolio/sector_rotator.go`、`internal/sectorallocation/engine_impl.go` | rotator 使用 WeightEngine 權重；舊路徑 feature-flag；單測 | pending | none | 審計 §P1-4 |
| F06 | 策略競爭空心 + 推薦寫死 | **accepted**：`ComparisonEngine.Record()` 無呼叫端；recommender 寫死 ranked 清單；wire 時建空 engine | `internal/strategy/comparison.go`（呼叫端：outcomes→Record）、`internal/recommender/handler.go:151-172`、`cmd/atlas/wire_recommender.go:58` | ComparisonEngine 有真實 history（GetScore ≠ 恆 0.5）；registered/premium 推薦由實際排名產生；端對端測試 | pending | none | 審計 §P1-5 |
| F07 | 心法庫 hit_rate 無回饋；validate API 不持久化 | **accepted**：`handlers.go:210-255` 只回傳不寫 | `internal/monitoring/api/strategies/handlers.go`、新 FeedbackStore（`data/state/strategy_feedback/<id>.json`） | validate 寫持久化（累積 total_tests/total_hits，hit_rate 重算）；path escape 防禦；測試覆蓋 | done | none | 2026-07-17；主線 wire 至 autobacktest daily 排程後下一次回測會自動累積 |
| F08 | CIRCUIT_BREAKER/VaR 訊號無任何消費者 | **accepted**：只展示 | `internal/autobacktest/loop.go`、`cmd/atlas/main.go:1638-1650` | autobacktest_daily 跑完後接 `SignalApply`：CIRCUIT_BREAKER 觸發 force-open live channels（fugle/fubon/finmind），與 VIX-crisis 路徑對齊 | done | none | 2026-07-17 |

### 線 G：資料通道補齊

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| G01 | 集保股權分散（週頻）零覆蓋 | — | 新增 provider + 排程（每週一）+ 回填 | 每週自動抓股權分散表；個股大戶(>400張)/散戶分級比例可查；歷史回填 | pending | data-sources.md 更新 | TDCC 每週公布（事實查核 §5.1） |
| G02 | 借券賣出餘額（日頻）零覆蓋 | — | 新增 provider + 排程（盤後） | 每日抓借券賣出餘額；個股變化可查；回填 90 天 | pending | data-sources.md 更新 | TWSE 每日公布 |
| G03 | 選舉行情零統計（config 自標 todo） | **accepted**：`configs/parameters.json:3813` | `cmd/calibrate-seasonal`、parameters.json、事件規則 | 區分地方/總統層級校準（證據強度不同）；結果寫回 + 文件更新 | pending | industry-calendar.md 更新 | 事實查核 §5.2：地方 86% vs 總統不一致 |
| G04 | `year_end_rally` 校準值 46% 疑似異常 | hypothesis：校準視窗或報酬計算 bug | `cmd/calibrate-seasonal`、`internal/industry/seasonal_calibrator.go` | 找出根因並修正；校準值回合理範圍；附驗算過程 | pending | none | 審計 §P1-7 |
| G05 | admin 通道頁靜態清單漏列 18 個已註冊通道 | **accepted**：`internal/monitoring/service/data_channels.go` 硬編 | `internal/monitoring/service/data_channels.go`、Handlers.RegisteredChannelIDs、main.go 注入 gateway.ChannelIDs() | 改從 ChannelRegistry 動態產生（fallback entry 標 `registered channel`）；已註冊 channel 不重複；測試覆蓋 | done | none | 2026-07-17 |
| G06 | 個股法人資料最多落後一個月（FinMind 月排程 + 402 配額風險） | **accepted**：cron 每月 1 日 | `cmd/atlas/capital_tasks.go`（T86 FetchSymbolFlow 批次化）或 docker cron | 個股三大法人新鮮度 T+1；FinMind 降為備援 | pending | data-sources.md 更新 | Agent B §五 |

### 線 H：散戶體驗

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| H01 | 策略競爭狀況無 UI 出口（業主明確需求） | — | 新頁/區塊：darwinian status+trend、agent observatory（API 已存在） | 散戶可看 agent 排名/權重變化/勝率，附白話解釋；Playwright 覆蓋 | pending | none | 審計 §P1-8 |
| H02 | 三大法人只有當日快照，無歷史趨勢圖 | — | `shared_web/static/js/pages/`（home 或錢潮頁）；後端 `macro/snapshot/history`、`capital_flow_daily` 歷史能力 | 30/60 日外資/投信/自營累積買賣超趨勢圖上線 | pending | none | Agent E §2 |
| H03 | 站內無 agent 解說入口 | — | 首頁解說按鈕（擴展 strategies 頁 AI 歸因模式）；後端 compose endpoint | 「今天為什麼漲/跌」一鍵白話解說；LLM 失敗降級規則文案 | pending | none | 審計 §七-2 |
| H04 | 缺散戶 explainer compose 型 MCP tool | — | `cmd/atlas-mcp/server/` 新 tool + prompts 劇本 | `explain_market_move`（暫名）註冊；Hermes/OpenClaw 可用；目錄更新 | pending | tool-catalog.md 更新 | Agent F §6 |
| H05 | 散戶無法自行下單模擬（互動式 paper trading） | — | 設計文件先行（範圍/風控/與現有模擬投組關係） | ⚠️**需業主決策**範圍後才實作 | pending | 設計文件 | 審計 §P2-5 |
| H06 | 無端到端鏈路活檢 | — | 新排程 probe：資料→策略→推薦全鏈路合成探測 | probe 每日跑；任一段失效即告警（區分節點） | pending | none | Agent F §7 |

### 線 I：定位文件（第〇輪，先於線 E05）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| I01 | 散戶 persona/網頁優先/台灣前提/三機制/七力分類學未成文 | — | 新增 `docs/reference/product-positioning.md` | 業主審閱通過；成為 E04/E05/H05 等決策的仲裁依據；AGENTS.md 加索引 | pending | **新文件** | 審計 §二 |
| I02 | investor 入口以 MCP 為主軸（與網頁優先矛盾） | — | `docs/investor/README.md`、`README.md:24-35` 敘事順序 | 入口敘事改為「網頁先用、MCP 進階」；與 I01 一致 | pending | 文件修正 | 依賴 I01 |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| 定位/資料/錢潮鏈/策略鏈/UI/MCP 六路審計 | done | `docs/audit/2026-07-17-retail-positioning-gap-audit.md`（含全部檔案:行號） |
| 活體端點驗證（11 個 MCP 工具） | done | 審計 §六；`strategy_ranker` 失敗、`get_recommendations` 警告、daily_report 矛盾等實測 |
| 台灣市場事實查核 | done | 審計 §五（集保週頻、無盤中法人資料、春節 71-75%、選舉證據混合等，附權威連結） |
| 根因假設驗證 | done | 多數 ID 根因已 accepted（表內標注）；少數待實作時深化（B06/B07/B09/G04 標 hypothesis） |

### Phase B — Plan

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker |
| 輪次與依賴排序 | done | 執行策略總覽 |
| 需業主決策項標記 | done | C05、E04、E05、H05（⚠️ 標記） |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| （尚未開始）第一輪從線 A 逐 ID 進行，每 ID 遵循 atlas-pre-change-protocol | - | pending | - |

### Phase D — Close Out

| Task | Status | Evidence |
|------|----|--------|
| 每輪結束更新狀態欄 + PR 引用本 manifest | pending | - |
| 全部完成後將本 manifest 移至 `docs/archive/` | pending | - |

---

## Backlog（P2 評估類，不進當前 scope）

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| BK-01 | 券商分點資料（主力成本線/分點集中度）——成本與合規評估先行 | 2026-07-17 | 評估文件 → 業主決策 |
| BK-02 | 投信「被迫買入」模型（基金/ETF 規模→成分股被迫買盤） | 2026-07-17 | 設計文件 |
| BK-03 | 美股事件→台股供應鏈個股傳導（NVDA 財報事件源） | 2026-07-17 | 設計文件 |
| BK-04 | TPEx 上櫃三大法人/融資券 | 2026-07-17 | 通道補齊輪 |
| BK-05 | EPFR 類外資流向（付費）+ 個別 ETF 申贖/折溢價 | 2026-07-17 | 評估文件 |
| BK-06 | Kimi heuristic 假設清單（連買天數/成本線/門檻）批次校準管線 | 2026-07-17 | 接 F03 校準機制成熟後 |
| BK-07 | llms.txt 由工具註冊表自動生成 | 2026-07-17 | C06 完成後 |
| BK-08 | 背景任務自動 backoff retry（目前只記 failure 等下個 interval） | 2026-07-17 | 線 B 完成後 |
| BK-09 | main.go 的 dwMgr 啟動後不 reload → auto_rollback / D03 的即時 Sharpe 用陳舊資料（auto_propose 已自行 Load，auto_rollback 未改） | 2026-07-17 | 線 B 後續 |
| BK-10 | taifex_daily / twse_etf / twse_oddlot 三通道反覆「rate limit wait: context canceled」自我窒息 | done | 30b902ac；rate-limiter Wait 改 context.WithoutCancel + 15s 上限脫鉤等待；HTTP 仍走呼叫端 ctx |
| BK-11 | dailyreport 的 Strategy / Risk section 仍硬編碼（B06 只修 macro/capital/events/summary） | 2026-07-17 | 線 B 後續 |
| BK-12 | E01 TAIFEX OI 缺 90 天回填（OpenAPI 只回最新日）。需 FinMind `TaiwanFuturesInstitutionalTraders` 或向 TAIFEX 申請歷史資料 API | 2026-07-17 | E03 上線後才能進入 90 日校準 |
| BK-13 | E04 v1 官股行庫代理採「操作員匯入」模式；分點加總自動化（讀證交所每日分點 → 篩官股券商 → 加總）尚未實作 | 2026-07-17 | E04 後續 |
| BK-14 | E04 評估購買第三方整理資料（CMoney/Goodinfo「八大行庫買賣超」）的商業授權與口徑一致性 | 2026-07-17 | E04 評估 |

> **Rule**: 每個 session 最多從 Backlog 拉 1 項進 scope，且須在當前 ID 全 done/paused 後。

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- 一 ID 一 commit；跨檔案邏輯性修改可一 commit 多檔
- 驗收標準未過不得 commit
- PR body 必須引用：`See docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md`
- 不直接推 main；CI 綠才 merge；merge 後依 `docs/multi-cli-protocol.md` 清理分支
- 修改 function/class 前先跑 gitnexus_impact（pre-change protocol）

---

## 溝通規則

- 每完成一個 ID 回報：「#A01 完成，驗證方式：…」
- 每完成一輪更新狀態欄並簡短總結
- 主導下一步，不問「接下來做哪個」；但以下情況暫停並請示：
  1. 標 ⚠️ 的業主決策項（C05、E04、E05、H05）
  2. 需要 API key / 付費資料源 / 部署權限
  3. 修復影響進行中 PR 或他人 worktree
  4. 發現審計未涵蓋的系統性問題（先入 Backlog）

---

## Session-End State

- **Done so far**: 第〇輪+第一輪+第二輪（27 IDs）+ 第三輪 C05 + BK-10 + 線 E（E01-E05）+ 第四輪 F01+F07+F08+G05 共 37 IDs done
- **第四輪重點（線 F+G 低風險子集）**：
  - F01：drift alert 單位統一（CapitalFlowSignal{ Direction, Value }，預測/實際都先轉方向再比對；移除 2σ 比較）
  - F07：FeedbackStore 持久化 validate 結果（累積 total_tests/total_hits，hit_rate 重算）；path escape 防禦
  - F08：CIRCUIT_BREAKER 訊號接 autobacktest daily 排程；觸發 force-open live channels（與 VIX crisis 路徑對齊）
  - G05：admin 通道頁從 ChannelRegistry 動態產生 fallback entry，不再漏列已註冊通道
- **Remaining（本輪未動）**：F02/F03/F04/F05/F06（需要預測閉環接通策略層）、G01/G02/G03/G04/G06（需要新資料源）、線 H 散戶體驗 UI 全段
- **Next action**: 第五輪線 H（H01 策略競爭 UI、H02 歷史趨勢圖、H03 「為什麼漲跌」按鈕、H04 agent explainer MCP tool），或先補 F05/F06 把策略層接通
- **Branch / PR**: `fix/retail-positioning-gap-r1-r2` → PR #1210（累計 41 commits）
- **待驗證（需重啟服務）**: E01 排程 15:30 後實際抓取、E03 上線後 ≥90 個交易日才能驗證校準、Dockerfile 重新 build 啟用 calibrate-seasonal、auto_experiment 下週執行時 backlog 降至 <50

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-17 | 1.0 | Initial manifest（47 IDs + 8 backlog，承接審計報告） | Kimi Code |
| 2026-07-17 | 1.1 | 第〇輪+第一輪完成：16 IDs done（A01-A06、C01-C04/C06/C07、B04-B07） | Kimi Code |
| 2026-07-17 | 1.2 | 第二輪完成：9 IDs done（B01-B03/B08-B10、D01-D03）；新增 Backlog BK-09~BK-11 | Kimi Code |
| 2026-07-17 | 1.3 | 第三輪+收尾：C05（業主裁決 Email 留資）+ BK-10（rate-limit 脫鉤）+ 線 E 全（E01-E05）+ E03；新增 Backlog BK-12~BK-15；branch `fix/retail-positioning-gap-r1-r2` 33 commits 推送，PR #1210 開啟 | Kimi Code |
| 2026-07-17 | 1.4 | 第四輪（線 F+G 低風險子集）：F01 drift alert 單位統一、F07 FeedbackStore 持久化、F08 CIRCUIT_BREAKER 訊號接 live channel force-open、G05 admin 通道頁動態從 registry 產生；branch 累計 41 commits 推送，PR #1210 更新 | Kimi Code |
| 2026-07-17 | 1.5 | 第五輪前置（E06 foundation 註冊）：E06 七維錢潮唯一正本 done（canonical spec commit 79277a53）；註冊 E07（backend 3+2+2 assessment）與 E08（UI/MCP/runtime 對齊）為 pending；自 Backlog 拉入 BK-15（ForceExtractor 交易日去重持久化）為 pending；本版本不 commit，待 BK-15 commit 一併送出 | Kimi Code |
| 2026-07-17 | 1.6 | 第五輪第一段（#BK-15 done）：production 端持久化交易日滾動狀態 — `internal/capitalflow/{rolling_store,forces,service}.go` 單一 file store + `cmd/atlas/{main,wire_recommender,operations_tasks}.go` 共用單一 file store（`cfg.LedgerDir/capital_flow_rolling.json`、capacity 60）；`capital_flow_refresh` 排程改走 `Service.Refresh(ctx, tradingDate)`，tradingDate 由 `currentTaipeiTradingDate` helper 推導（Asia/Taipei、15:30 cutoff、週末 rollback）；focused + full + build + gofmt + manifest + markdown-link 全綠；commit 4eae84e9 | Kimi Code |
| 2026-07-17 | 1.7 | 第五輪第二段（#E07 done）：七維 backend 完成 3+2+2 provenance、四層 `CapitalFlowAssessment`、legacy weight/quality 分流；recommender 每 request 單次 daily fetch，eventdriven 僅在 `EligibleForAutomation()` 通過後使用 legacy score；focused race + full + gofmt + vet + build 全綠；commit ccd4e721 | Kimi Code |
| 2026-07-17 | 1.8 | 第五輪第三段（#E08 done）：UI/MCP/活躍文件/runtime 對齊 — `shared_web/static/js/components/{seven-force-board.js,seven-force-interpretations.js}` 改為 3+2+2 分層（官方法人 / 行為代理 / 領先＋跨市場訊號），刪除「權重 X%」字串、政府 unavailable 顯示「資料不足」；MCP `capital_flow_daily` / `capital_flow_summary` / `daily_report` 描述同步；active docs（product-positioning、architecture、frontend-architecture、agent-mcp-server、tool-catalog、investor README、llms.txt、capitalflow AGENTS、AGENTS_INDEX、MATURITY、recommender deps、capitalflow types）改為「七維錢潮雷達（3+2+2 分層）」摘要並連回 canonical spec；`bash scripts/ci/check_atlas_mcp_docs_consistency.sh` + `check_markdown_links.sh` 全綠；commit 2603710b | Kimi Code |
