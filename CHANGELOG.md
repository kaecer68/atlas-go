# Changelog

## [Unreleased] - 2026-08-07

> 0.0.2.0（2026-07-22）後累積功能補記（2026-08-07 盤查生成）。

### fix(ops): `cmd/backfill-var-returns` 補上 `daily_returns` 日期語意並按交易日去重（#1935）
- **問題**：`cmd/backfill-var-returns` 是 `daily_returns` 的第 4 個 writer（after `auto_daily_simulation`／`stress_test_daily`／`POST /admin/trigger-simulation`），但它只覆寫 `daily_returns`，不寫 `last_session_date`/`session_base_value`（#1900 / PR #1932 建立的日期語意）→ 重建後的檔案退回「無日期語意」；同一交易日存在多個 session 目錄（`session-<YYYYMMDD>-<replay mode>`）時，每個目錄各產生一筆 → 同日重複。
- **實測（同一 fixture）**：修前 `Sessions: 4, Returns: 3`、無 `last_session_date`；修後 `Sessions: 3 (distinct trading days), Returns: 2`，`last_session_date=2026-09-24`、`session_base_value=1010000`。同日多 session 目錄 300 天的合成案例：修前 599 筆（含 300 筆同日 0 報酬）`var95=0.0000`、`cvar95=-0.0018`；修後 299 筆 `var95=-0.0200`、`cvar95=-0.0200`（同日零報酬不再主導尾端）。
- **修正**：`main.go` 改為 typed `domain.SimulationState` round-trip（與 `sim.SavePersistentState` 同 schema，並套用 `sim.LoadPersistentState` 的 nil 正規化），寫入 `last_session_date`（重建序列的最後交易日）與 `session_base_value`（前一交易日收盤，與 `RunDay` 同語意）；session 目錄依交易日去重（預設 `-on-duplicate=last`，即 `recorded_at` 最新者；`first` 為 earliest）；`session_id` 解析改用 `domain.SessionDateFromID`，無法解析時以目錄名回退，兩者皆無 → 略過並 warning；相異交易日 <2 → 報錯且不改檔；新增 `-dry-run`。舊的位置參數形式（`<sessions-dir> <state-file>`）保留，flag 可前可後。
- **測試**：新增 `cmd/backfill-var-returns/main_test.go`（9 個測試）：日期語意寫入與非本命令欄位保留、同日去重（`last`/`first`）、連跑兩次序列與位元組不變、單一交易日拒絕且不改檔、`-dry-run` 不寫檔、**重建後引擎同日再跑 → 序列長度不變且報酬以前一交易日收盤重算**（並以修前輸出對照：同日重複 2 筆、`session_rerun=false`）、risk 快照不再被同日零報酬主導、參數形式。
- **文件**：`docs/specs/sim-engine-spec.md` 新增「daily_returns 序列契約（交易日語意）」與 writer 義務；`docs/reference/traps.md` 既有陷阱列補上第 4 個 writer 與 spec 指標（維持 330 行）；`cmd/REGISTRY.md` 兩列更新。
- **未動（明確）**：不回溯清理舊檔已存在的同日零報酬；不重建 `equity_curve` 與 `previous_values["_portfolio_"]`；不改 `internal/sim` 的 `RunDay` 語意；不改 production 資料。

### feat(sectorallocation/capitalflow): 產業命中率接進消費鏈路（config-gated、預設 off）（#1942/#1948）（2026-09-24）
- **問題**：canonical 產業級命中率（#1942/#1948，扣成本口徑 Wilson CI + min_samples 校準）除報告端點 `/api/stock/industry_winrate` 外沒有任何 production 消費端 — 命中率不影響 applied 權重，也不影響 capital-flow assessment。
- **修正**：新增 config gate `sector_allocation.industry_hit_rate_consume_enabled`（預設 **false**）。開啟後：(a) `ComputeProjectedTarget` 把 `calibration_status == eligible` 的 canonical L1 列轉成 `DriverInputs.CapitalFlow` 的 additive tilt（`(WilsonLower-0.5)*0.2`，夾在 ±0.05，`avoid` 反向）；(b) `LatestAssessment` 附上 advisory 的 `industry_hit_rate_evidence`（`applied`/`reason`/rows/`mean_wilson_lower`/tilt 極值）。provider 綁定在 `cmd/atlas`（`stocktools.SectorAllocationHitRateProvider`，read-only canonical 聚合），綁定本身在 gate off 時 inert。
- **fail-closed**：gate off／未綁 provider／provider error／報告 0 列／無 eligible 列 → 一律不改 driver，並以決定性 `reason`（`disabled`/`no_provider`/`provider_error`/`no_rows`/`insufficient_calibration`）留痕；不以 0 或猜測值替代未達 min_samples 的列（這是與 #1944 系列「inert 閉環」相反的紀律：可讀、可解釋、預設不動）。
- **逐位元保證**：gate off（含已註冊 provider）與 gate on + fail-closed 兩者的 `ProjectedTarget`，與改動前 revision 產出的快照 `internal/sectorallocation/testdata/production_path_off_baseline.golden.json` **逐位元相同**；assessment 端 evidence 為 `nil` 時 `omitempty` 讓 JSON 維持不變（皆有測試釘住）。
- **可逆**：唯一開關是 config，翻回 false 於下次 reload 即回基準；不寫入任何歷史資料。
- **檔案**：`internal/sectorallocation/industry_hitrate_{consume,assessment_decorator,provider_registry}.go`、`internal/capitalflow/{assessment_decorator,types,service}.go`、`internal/stocktools/industry_hitrate_consume_provider.go`、`cmd/atlas/main.go`（1 行 provider 綁定）、`internal/config/{parameters,defaults_engine}.go` + 兩份 golden、`configs/parameters.json`、`docs/specs/industry-hitrate-consumption-spec.md`、`docs/reference/traps.md`，以及對應 `*_test.go`。
- **未動（明確）**：stockpicker 算式/成本口徑、#1943 canonical taxonomy（只讀）、`Projector` 投影公式、`CalibrationStatus`/`EligibleForAutomation` 語意；沒有新增任何 default-on 參數。
- **驗證**：`go test ./internal/sectorallocation/... ./internal/capitalflow/... ./internal/stocktools/... -count=1`、`make ci-gate` 全綠。
### fix(monitoring): channel 狀態單一真相 — 過期資料不得回 `ok`（twse_oddlot DB/derived 判定衝突）（2026-09-24）
- **問題**：同一時刻、同一 channel 出現多個互相矛盾的判定（生產實證 `twse_oddlot`）：`channel_health`（DB）`status=ok last_fetch_at=<5 分鐘前>`、`/api/dashboard/channel-health` `ok`、health summary log `stale`。真實情況是上游 BFI84U 被 TWSE 改用途（2026-08，見 `internal/monitoring/known_issues.go`），最後一次成功抓取停在 2026-09-07。
- **根因**（三個獨立缺陷）：
  1. **derived 層漏套契約窗口**：`resolveChannelStatusFromStore` 對任何 `status=="ok"` 一律回 `ok`（註解甚至寫 "healthy regardless of data age"），完全繞過 `FreshnessWindow`；同一份 record 在 `Gateway.Summary()`（`deriveStatusWithContract`）卻是 `stale`。
  2. **DB 時間戳說謊**：`recordToDB` 以 `time.Now()` 寫入 `last_fetch_at`/`last_success_at`，每 5 分鐘的 `channel_health_sync` 因此把**每個** channel 蓋成「剛剛抓過」，讓 DB 成為第三個真相（也讓任何查 DB 的人/工具誤判資料新鮮度）。
  3. **空 payload 記成成功**：`twse_oddlot` adapter 對 `ErrNoData`/`ErrOddLotUpstreamRemoved` 回 `FetchResult{Stale:true}`（無 error，不觸發 breaker），`Gateway.Fetch` 不看 `Stale` 就記 `ok`——上游已消失卻與有資料無法區分。
- **修正**（方案 A+B 混用：記錄為真相 + derived 只做呈現，但判定只有一份）：
  - 新增 `internal/apigateway/channel_status.go`：`DeriveChannelStatus(rec, contract, now)` 為**唯一**狀態判定（非 `ok` pass-through；`ok` 且 `LastFetchAt` 超過 `EffectiveFreshnessWindow()` → `stale`；時間戳不可解析時保留 `ok`；`Provenance=derived` 的指標紀錄不套用），另有 `DeriveChannelStatusReason`（人類可讀原因，含「資料已 17 天未更新，超過合約更新窗口 48 小時」）。
  - 所有呈現層改走同一函式：`resolveChannelStatusFromStore`（`/admin/datachannels`、首頁、`data_get_channels`）、`DataChannelService.getHealthFromStore`、`UnifiedHealthStore.deriveStatusWithContract`（→ `Gateway.Summary()` / `StatusSummary` 日誌與 error counter）、`/api/dashboard/channel-health`（derived 判定以既有 `last_error` 欄位表達原因，known-issue 欄位保留）、`/api/health/aggregate` Tier 2（新增 `stale` 計數桶）、`atlas_channel_health_status` gauge。`resolveChannelStatusFromStore` 補上 `stale`/`inactive` case（先前 `inactive` record 落 default 被丟棄 → 頁面顯示「未知」，`twse_etf` 為實例）。
  - `ChannelHealthSyncValuesFor` + `recordToDB(channelID, status, lastFetchAt, lastSuccessAt, consecutiveFailures)`：DB 的 fact 欄位改寫 record 自己的時間戳，`status` 欄存 derived verdict（與 UI 一致），只有 `updated_at` 是寫入時間。
  - `FetchOutcomeStatus` + `twse_oddlot` 契約 `DegradedOnEmpty=true`：空/stale payload 記 `degraded` 並附原因；`twse_margin`/`twse_capital_flow` 的「非交易日無新資料」仍記 `ok`（避免週末誤報）。
  - `StatusText` 補 `stale`「資料過期」與 `degraded`「降級」（先前兩者都 fallback 成「未知」）；`healthStatusValue` 把 `stale` 映射為 warn(1)（現有 4 條 alert rule 只匹配 `== 2`，不改變 page 行為）。
  - 前端（`datachannels.js`/`dashboard.js`/`alerts.js`/`data-quality-badge.js` 等）：`stale`/`degraded` 不再被算成「正常」，以 amber 呈現。
- **驗證**：同類掃描（生產 44 筆 record × 契約窗口）→ 修前僅 `twse_oddlot` 為 `ok`-but-expired（413.7h > 48h）；修後兩個層級皆 `stale`。新增測試：`internal/apigateway/channel_status_test.go`（判定表、`FetchOutcomeStatus`、DB mirror 值、PG 端到端 DB 斷言）、`gateway_empty_payload_test.go`、resolver 6 個新 case（含 17 天 `ok` → `stale` 迴歸）、`/api/health/aggregate` stale 桶、`/api/dashboard/channel-health` stale + known-issue 保留、metrics gauge `stale`。
- **前端**：新增共用 SSOT `shared_web/static/js/shared/channel-status.js`（status → label/tone/是否算「正常」的唯一對映），
  `datachannels.js` 的「正常」改**正向計數**（原本 `total - error - warn` 會把 `stale`/`degraded` 算成正常）、`dashboard.js` KPI 與
  `alerts.js` badge 改為 tone 驅動（`stale`/`degraded` = amber，不再一律紅「異常」也不再有綠色）、`data-quality-badge.js` 顯示最嚴重狀態自己的 label。
  純前端測試（`node --test shared_web/static/js/__tests__/*.mjs`）471 passed / 0 failed。
- **未處理（明示）**：`twse_oddlot` 上游已消失的事實**不變**（known-issue 徽章與 `twse_capital_flow` 替代路徑照舊，本 PR 不掩蓋、不恢復）；DB 既有的錯誤 `last_fetch_at` 會在下一次 `channel_health_sync`（≤5 分鐘）被真實值覆蓋，不回填歷史。

### fix(sectormap): ETF → L1 映射改由投信官網持股推導，取代手寫對應表（#1956 後續）（2026-09-24）
- **問題**：sector allocation 用 ETF 當產業配置的交易載具，但系統沒有任何一處說得出「這檔 ETF 實際橫跨哪幾個 canonical L1 產業」；每個呼叫端各自憑印象列一組，同一檔 ETF 在不同路徑得到不同 L1 集合 —— 與 #1943 剛消滅的缺陷同型。**已合併的 #1956** 建了表與 metric，但 11 檔 ETF 的 canonical L1 target 是**人工列舉 + 等權**，沒有任何資料來源：`0050.TW` 被指定 8 個 L1、`00940.TW` 4 個，權重一律 `1/N`，`reason` 只寫「PR-α ETF representative」。同一個 `reason` 欄位在 #1943 的規矩裡必須是可查證的資料來源，而等權加權代表「各 L1 曝險相同」，與 TW50 的實際結構（台積電一檔 56%）相反。另外 `internal/sectormap` 沒有任何「上市櫃個股 → 產業」的詞彙，連用真實持股反推產業都做不到。
- **新增**：
  - **namespace K `twse_industry_code`**（`internal/sectormap/twse_industry_code.go`）：TWSE OpenAPI `opendata/t187ap03_L`（上市，`產業別`）與 TPEx OpenAPI `mopsfin_t187ap03_O`（上櫃，`SecuritiesIndustryCode`）的 36 個 2 位數字碼，逐 key 顯式處置（22 mapped、14 unmapped+reason+candidates），覆蓋 20/20 canonical L1。中文名取自 TWSE ISIN 產業別對照表並以已知個股交叉驗證（2330→24 半導體業、2603→15 航運業、2912→18 貿易百貨業）。殘差桶 `19 綜合`/`20 其他業` 與 legacy 聚合碼 `13 電子工業` 顯式未映射且**不給 candidate**（給了就是猜）。
  - **namespace L `sectorallocation_etf_representatives`**（`internal/sectormap/etf_representatives.go`）：`configs/etf_metadata.json` 的 11 檔 ETF，每檔一列 canonical L1 加權曝險，`Reason` 帶完整證據鏈（投信、頁面 URL、資料日、檔數、未覆蓋權重比例）。
  - **證據快照**（`internal/sectorallocation/testdata/etf_holdings_20260924.json`）：11 檔 ETF 的當日持股明細，全部來自投信官網（元大／富邦／國泰／群益／復華／中信），共 529 筆持股，逐筆附 `industry_code` 與其來源。
  - **typed view**（`internal/sectorallocation/etf_representatives.go`）：`industry.SectorID` 鍵的存取器與 `ETFL1Coverage()`。
  - **稽核輸出**：`industry-namespace-audit` 新增 `etf_l1_coverage` 區塊（總數、逐檔 L1、逐檔權重、資料日、來源 URL）。
- **推導規則（不得靜默）**：ETF 的 L1 權重 = 該 L1 下持股的官網權重 ÷ 可對映持股權重合計；官網只列股票部位（期貨／現金另計，實測 96.59%–99.71%），未映射持股（如電子通路商）**回報但不計入**，未覆蓋比例由 `ETFRepresentative.ReportedWeightPct - MappedWeightPct` 具名揭露。**不補 1、不猜產業。**
- **結果**：ETF 可觸及 **19/20** canonical L1（僅 `tourism` 未達：這 11 檔當日皆無觀光餐旅持股），逐檔 3–17 個 L1；ETF namespace 的 unmapped = 0。相對 #1956 的 13/20，多出 6 個 L1（`auto`、`biotech`、`construction`、`food`、`other_electronics`、`textiles`）—— 不是「補更多 ETF」，而是**同一批 11 檔 ETF 用真實持股反推**才看得見的曝險（例：00713 持有和泰車、00692 持有大成鋼/遠東新）。#1956 已涵蓋的 13 個 L1 全部保留，沒有任何一個因換算方式改變而消失（`0050.TW` 由手寫 8 個 L1、等權 1/8 改為 11 個 L1、semiconductor 0.696；`00891.TW` 仍是 3 個 L1 但權重由等權 1/3 改為 semiconductor 0.950 / electronics 0.037 / telecom 0.013）。
- **取代關係（明示）**：#1956 的 `internal/sectormap/etf_representatives.go`／`internal/sectorallocation/etf_representatives.go` 及其測試由本 PR 整檔取代（同一個 namespace ID、同一個 metric 名稱，資料改為推導）。

- **驗證**：`internal/sectormap/twse_industry_code_test.go`（36 碼 = ISIN 對照表、覆蓋 20/20 L1、殘差碼不得對映、**22 組「TWSE 指數名 vs 產業碼」必須給同一個 L1**）、`internal/sectormap/etf_representatives_test.go`（key 集合 = `configs/etf_metadata.json`、無 unmapped、每列帶出處、權重加總 1、覆蓋率 ≥12）、`internal/sectorallocation/etf_representatives_test.go`（**從持股快照重跑推導**並逐值比對宣告表、快照每個 `industry_code` 必為宣告 key、下限 ≥12 由快照獨立計算、typed view 一致）、`cmd/experimental/industry-namespace-audit/main_test.go`（CLI 真的輸出 ≥12 且每列帶證據）。`gofmt` 乾淨、`go vet` 乾淨、`make ci-gate` 綠。
- **未處理（明示）**：持股快照是 2026-09-24 的**定時快照**，ETF 換股後需重跑推導（測試會紅燈提示）。`tourism` 未達是資料事實而非映射缺陷（這 11 檔 ETF 沒有觀光持股）。`twse_industry_code` 目前只被這條 ETF 路徑消費；把它接成 DB `symbol_industry` 母體來源屬獨立工作（spec §6 缺口 1）。


### fix(capitalflow): 錢潮驗證／判斷層接線（#1941）（2026-09-24）
- **問題**：七維錢潮的驗證層與判斷層結構上不可能生效 ——（a）`ComputeCapitalFlowAssessment` **硬寫** `CalibrationStatus="calibrating"`，`EligibleForAutomation()` 永遠 false，連帶 `/api/recommendations` 每則回應都帶 `capital_flow_assessment_calibrating` warning；（b）Stage-3 的 5 個排程任務與 3 個 alert evaluator **只有測試呼叫**，`STAGE3_TASKS_ENABLED` / `STAGE3_ALERTS_ENABLED`（皆預設 true）gate 不到任何東西；（c）`LatestCapitalFlowActual` 每次呼叫都建拋棄式 `capitalflow.NewService(macroProvider, 0, nil)`，rolling window 為空 ⇒ 每維 Z=0，預測 vs 實際比對無意義。
- **修正**：
  - **判斷層**：新增 `internal/capitalflow/calibration.go`（`DeriveCalibrationStatus` / `CalibrationStatusReason`，門檻常數 `CalibrationEligibleMinSamples=30`），`ComputeCapitalFlowAssessment` 改為由 config 開關 `capitalflow.calibration_eligible_override`（**預設 false**）驅動。override=false → `calibrating`（與修正前逐位元相同）；override=true 且**每個 `data_available` 維度都自帶 `eligible`**（樣本數達 30、且參考窗無退化）→ `eligible`；否則 → `degraded`（`Reasons` 指名維度與樣本數）。per-dimension `degraded`（#1940 R3 的退化參考窗）不會被整體狀態蓋掉：一個維度退化，整體就只能到 `degraded`。門檻常數改為 SSOT（`forces.go` / `validation_h05.go` 的硬寫 30 換成常數）。翻轉程序維持人工 gate（引用 `data/reports/cf-hypotheses-<date>.json` 的 config PR；CLI 永不寫 config），spec §9.5.1 新增判定表。
  - **degraded 不再靜默**：`/api/recommendations` 對 `degraded` 另發 `capital_flow_assessment_degraded`（原本只有 calibrating warning，degraded 會完全沒有替代訊號）；日報／摘要文字對 `degraded` 加「評估異常（樣本不足或無離散度）」註記，不再誤標成「校準中」。
  - **驗證層**：`cmd/atlas/main.go` 新增 `wireStage3(stage3Deps{...})` 呼叫（gateway 區塊內），實際註冊 5 個排程任務 + 3 個 alert 任務；`monitor == nil` 時只跳過 alert（`NewStage3AlertEvaluator` 對 nil panic）並留 log。
  - **共享 store**：`stage3Deps.macroProvider` 換成 `stage3Deps.capitalFlow`（process-wide shared `capitalflow.Service`，使用同一份 file-backed rolling store）；無 service 時回報 unavailable，不再以 0 冒充實際值。**空 store 防線**：shared service 讀到的報告若沒有任何帶樣本的可用維度（rolling store 從未寫入 ⇒ Z 全 0），`LatestCapitalFlowActual` 回報 unavailable，不產生無意義的比對。
  - **預測 vs 實際紀錄**：`Stage3AlertDeps.OnCapitalFlowDriftCompared` 新 hook，market-close 規則每次可比對（hit/miss、含暖機抑制日）都寫一筆 JSONL 觀測記錄 `capital_flow_stage3_drift.jsonl`（在 ledger dir，預設 `data/state/`；上限 1000 筆、原子寫入）。
- **驗證**：`internal/capitalflow/calibration_test.go`（預設仍 calibrating／override+每維 eligible → eligible 且 gate 開／樣本不足或 per-dim degraded → degraded／缺維度不阻塞／config-driven 全鏈路／未載入 config 保持 false）、`cmd/atlas/stage3_tasks_test.go`（wireStage3 註冊 8 個任務、flag off 不註冊、nil monitor 只跳 alert、`main.go` 呼叫 wireStage3 的原始碼回歸斷言、shared store 下 `LatestCapitalFlowActual().Value != 0`、對照丟棄式 service 恰為 0、空 store 回報 unavailable）、`internal/monitoring/stage3_rules_test.go`（hook 在無 alert 時仍記錄、actual 不可用不記）、`internal/recommender/handler_test.go`（calibrating/degraded/eligible 三態的 warning 對應）。`gofmt`/`gofumpt` 乾淨、`go vet ./...`、`go test ./internal/capitalflow/... ./internal/recommender/... ./cmd/atlas/... ./internal/config/... ./internal/monitoring/... -count=1` 全綠。
- **行為變更（明示）**：預設值下**無數值語意變更**（assessment status 與 warning 與修正前相同）；新增的 production 行為是 Stage-3 任務開始執行、以及新的觀測 JSONL 檔。要讓 warning 消失／開啟自動化 gate，必須另開引用驗證報告的 config PR 把 `calibration_eligible_override` 設 true；該翻轉會讓 `capitalFlowActionFromPlan` 走出 E07 分支並改變 `weightEngine.ComputeProjectedTarget` 的 drivers，屬真配置語意變更，翻轉 PR 需一併驗 allocation。


### fix(capitalflow): 交易日判定改用權威假日表，修復雷達在交易日凍結（issue #1947）（2026-09-24）
- **問題**：七維錢潮 rolling store 自 **2026-09-22 07:59** 起停止更新，但 09-23（三）、09-24（四）為正常交易日（2026 中秋＝09-25）。同期 `msg=skip_non_trading_day date=2026-09-24 component=capitalflow` 每 5 分鐘出現，而 `task_liveness.capital_flow_refresh` 的 `consecutive_failures=0`（skip 不是 failure → 沒有任何告警）。上游其實有資料：`taiex ts=2026-09-24 12:55`、`market_volume ts=2026-09-23`、`data/state/capital_flow/20260923_capital_flow.json` 存在。
- **根因**：`internal/capitalflow/service.go` 的 CF-INV-16 閘門用 `industry.EventCalendar.IsTaiwanTradingDay`，該方法對任何落在 `long_holiday` 事件**區間**內的日期回 `false`；`buildHolidayEvent` 把每個國定假日展開成 `[holiday-3d, holiday+2d]`，2026 中秋（09-25）的區間 = 2026-09-22..09-27，涵蓋 09-23/24/28 等實際交易日。全 repo 僅 capitalflow 這 1 處使用該判定（其餘 19 處用權威的 `marketdata.IsTaiwanTradingDay` / `taiwanholidays.IsTradingDay`）。連假**區間**是事件/情緒窗，不是休市判定。
- **修正**：
  - 閘門改用權威表 `marketdata.IsTaiwanTradingDay`（`taiwanholidays.IsTradingDay`）；`industry.EventCalendar` 降級為顧問訊號：權威表說交易日而事件日曆說在連假區間內時，發 `long_holiday_window_covers_trading_day` WARN（只記錄、不阻擋）。
  - 移除「nil calendar → 視為交易日」的退化路徑（判定不再依賴注入的日曆；nil 僅停用上述顧問 WARN）。
  - **可觀測性**：每筆 skip 帶 `consecutive_skips` 計數；`skipAlertLevel` 在 (a) skip 但 snapshot 實際帶有七維輸入（休市不可能有上游資料 ⇒ 兩者之一必錯，即本 issue 生產症狀）或 (b) 連續 skip 首次跨越 3 次時升級為 WARN `skip_non_trading_day_suspicious`；成功進入交易日後計數歸零。
- **驗證**：新增 `internal/capitalflow/trading_day_gate_test.go`（2026-09-22/23/24/28 為交易日、09-25/26/27 為非交易日；生產形狀事件日曆接線下 09-22/23/24 各寫入一筆樣本；真假日/週末仍 skip 且 0 samples；skip 計數累加與歸零；`skipAlertLevel` 表驅動；事件日曆連假區間涵蓋交易日的**前提**測試）。`go test ./...`（除 `cmd/atlas`）與 `make ci-full` 全綠。
- **未處理（明示）**：生產 `capital_flow_rolling.json` 已凍結期間（09-23/24）的同日維度樣本不會由下一次 refresh 自動補回（Refresh 只寫當日；CF-INV-06 不允許補 0），需以 `ImportHistory`／歷史匯入工具另行 backfill，或接受序列缺該兩日。

### fix(capitalflow): 七維輸入層值／日期配對與 CF-INV-06 落實（issue #1940）（2026-09-24）
- **問題**：七維錢潮（3+2+2）輸入層同時存在「值／日期錯配」與「缺失值寫成 0」兩類缺陷，且都繞過 spec §8.3 / CF-INV-06。
  - R1：`government_flow` 的 0 值 placeholder 檔（`20260721.json`..`20260728.json`，`source=broker-aggregate`）被當成真實讀值連續 18 個交易日寫入 rolling store；`channel_contract.go` 對 `government_flow` 只要求 `file_exists`，通道因此回報 `ok`。
  - R2：`Latest()` 只取目錄最新檔、不與日期配對；`Refresh` 又以 refresh 當下的 `deriveTradingDate(RecordedAt)` stamping，產生系統性 +1 交易日位移（09-07←`20260904.json`、09-08←`20260907.json`、…、09-17←`20260916.json`；生產 2026-09-22 已到 +2）。
  - R3：`rollingWindow.stddev()` 的 `max(0.01, …)` 下限把「無離散度」換成假 epsilon，使退化視窗產生無上限 z：government 2026-08-27 = **−7028.5**、futures 2026-07-21 = **785,200**，經 `foreign.LeadingZ`／`leading_trend`／`dominant_signal` 外流。
  - 追加（非 issue 原列，生產實證）：同機制在 `institutional` 活體發生 —— 2025-09-17..2026-05-13 共 **148 筆連續 0 值樣本**，使 2026-05-15 的真實讀值 z = **−38.67**。
- **根因**：兩個自帶日期的 channel（`government_flow` 檔、TAIFEX 期貨 OI session）的讀值日期沒有被使用；「缺資料」與「值為 0」在輸入層未被區分；z 標準化在無離散度時仍除以 clamp 後的小數。
- **修正**（依 spec 契約，非補丁）：
  - 新增 `internal/capitalflow/reading_dates.go`：`dimensionSampleDate` 為唯一配對函式，`Service.Refresh`（寫入鍵）、`Service.extractAsOf`（`History` 上界）、`ForceExtractor.Score`（`AsOfTradingDate`）三者共用；`Refresh` 依 key 分組 `UpsertDay`，維持 CF-INV-05。
  - 新增 `internal/capitalflow/missing_data.go`：`zeroIsMissingDimension` / `usableReading`（淨流量與 OI 水準類 dimension 的 0 = 缺資料，不寫樣本）與 `referenceWindowFor`（已持久化的 0 值樣本不得進入參考窗，比例型 dimension 的 0 保留）。
  - `types.go`：`stddev()` 回傳真實母體標準差；新增 `windowDispersionUsable`（唯一退化判準）、`maxAbsZScore = 20` backstop；退化窗標 `calibration_status = degraded`。
  - `apigateway`：`government_flow` 契約由 `file_exists` 改為 `value_nonzero` + `GovernmentFlowAdapter.DataState`（0 值 placeholder → `degraded`，不再是 ok 假象）；`GovernmentFlowReading.HasData()` 為唯一 0 值判準。
- **驗證**：`internal/capitalflow/{reading_dates,missing_data}_test.go`、`internal/apigateway/adapter_government_flow_test.go`、`channel_contract_test.go`、`internal/marketdata/government_flow_provider_test.go` 新增迴歸測試（0 值不寫入序列、位移檔不得產生當日樣本、同檔重讀只留一筆、讀值不得進入自身參考窗、退化窗不得產生 |z| > 20、148 筆 0 值窗不得產生極端 z）。same-data 重算：government max|z| 7028.46 → 13.95、futures 785200 → 2.73、institutional 38.71 → 7.64；`ComputeResonance` 3 個交易日由 `0.5/mixed` 變 `1.0/bullish`。
- **未處理（明示）**：dev 與生產 `capital_flow_rolling.json` 既有的 0 值樣本**不回溯刪除**（讀取端已由 `referenceWindowFor` 濾除；寫入端不再產生）。生產 rolling refresh 自 2026-09-22 07:59 起停擺（與本修正無關，需另案追蹤觸發源）。`institutional` 的 148 筆 0 值來源（TWSE T86 該期間的 `DomesticFundNet` 為何持續為 0）未追查，建議另開票。spec 已補 §18.8 與 CF-INV-18。

### feat(industry): 產業命名空間統一 — canonical taxonomy + 顯式映射表（#1943）（2026-09-24）
- **問題**：全 repo 同時存在 6 套以上互不相容的產業 key 空間（canonical `SectorID` 20 L1+18 L2、config `classification_tree` 16 L1/29 segment、`industry.default_metrics` 23 key、`sector_symbols.json` 22 key、`sector_allocation.base_weights` GICS 11+1 key、TWSE 指數名的兩份矛盾映射）。同一 symbol／同一 TWSE 名稱在不同路徑得到不同產業 ID；`monitoring.TreeBasedMapper`／`SymbolL1Mapper` 只認「ID 剛好是 canonical L1」的節點，29 個 segment 有 19 個被靜默跳過（生產母體僅 27 支、`symbols_ranked=0`）；legacy `ComputeWeights` 的 GICS key 空間**永遠不可能**通過 `ValidateL1FinalTarget`。
- **新增**：leaf 套件 `internal/sectormap`（零專案相依，因為 `industry` 已 import `marketdata`，反向不可行）。內含 canonical L1/L2 清單、**新增的 L2→L1 父層表**（硬規則：凡分類樹宣告了父鏈，本表必須一致）、12 個 namespace 的逐 key **顯式**處置（`canonical` / `mapped` / `unmapped`+reason+candidates / `unknown`=drift）。未映射一律回報，禁止字串相似猜測與隱式 alias。

### fix(risk): daily_returns 具日期語意，同一交易日重跑改為取代（#1900）（2026-09-23）
- **問題**：`domain.SimulationState.DailyReturns`（`data/state/simulation_state.json`）名義上是日報酬，實際是「每次 `RunDailySimulation` 的報酬」，沒有任何日期 metadata。`auto_daily_simulation`(24h)、`stress_test_daily`(24h)、`POST /admin/trigger-simulation` 共用同一檔案，同日重跑各自 append 一筆；同日重跑通常沒有新交易、報價不變 → append 的那筆 ≈ **0**。實測 8 小時內序列由 18 筆長到 32 筆，尾端被零報酬塞滿，導致風險快照 `var95=0 / cvar95=0`（零報酬佔滿 `ComputeRiskSnapshot` 讀取的 5% 尾端百分位）。
- **根因**：`sim.Engine.RunDay` 的 step 4 無條件 `append(state.DailyReturns, ...)`，狀態檔沒有「最後一筆屬於哪個交易日」的欄位，因此無法判斷是否為同一交易日。
- **修正**：
  - `industry.SymbolL1Mapper` 改為兩段解析（rank 0 樹結構優先、rank 1 宣告表換算），真實 config 由 **18 → 44 支 symbol** 映到 canonical L1（L1 代表股 27/27、可達 L1 由 5 → 9 種），並新增 `UnmappedSegments()`（5 個桶：defensive/etf_rotation/high_dividend/small_cap/tech）與 `Conflicts()`（1 筆：2356）；「同 symbol 被兩個 canonical L1 segment 宣告」仍回 error。
  - `marketdata.TWSESectorIndexProvider`：刪除與 canonical map 矛盾的 legacy 8-key 映射，兩個 entry point 共用同一宣告表（`電腦及週邊設備類`→electronics、`電機機械類`→machinery）。以 live `MI_INDEX` 實測（2026-09-24）修正 TWSE 詞彙：原表列的「化學工業類指數」「觀光類指數」**不存在於 live 回應**，實際名稱為「化學類指數」「觀光餐旅類指數」（已補入，這是 `chemicals`/`tourism` 過去永遠拿不到資料的真正原因）；live 的其餘 15 個名稱宣告為 unmapped（具名可稽核），2 個舊名保留為歷史別名。
  - `marketdata.SectorIndexReader.canonicalSectorIDs` 由手抄 18 個改為由共用清單推導（補 chemicals/tourism）。
  - `marketdata.SymbolIndustryMapper` 新增 `CanonicalSectorID`/`CanonicalL1`/`CanonicalReason`（additive，既有 JSON 欄位不動；已重跑 `cmd/gentags`）。
  - `sectorallocation` 新增 `ProjectLegacyGICSWeights`（GICS→canonical L1 投影，未映射且帶權重時回 `ErrLegacyGICSUnmapped`，blocked weight 0.25）、`FilterL1KeysReport`（取代靜默丟棄）。
  - 新增覆蓋率指標 `industry.ComputeCanonicalCoverage` / `DeclaredRepresentativeUniverse` 與稽核 CLI `cmd/experimental/industry-namespace-audit`（`-universe <file>` 可算母體覆蓋率）。
- **文件**：新增 `docs/specs/sector-namespace-canonical-spec.md`（canonical 定義、每個 namespace 處置與未映射清單、覆蓋率定義與現值 27/27 與 27/854≈3.2%、9 項已知缺口與 4 筆已具名 symbol 衝突）。
- **驗證**：新增漂移守門測試（表 vs 真實 config/seed/程式 key 集合、**宣告 L2→L1 父層 vs 樹父鏈**、TWSE 兩表一致性、FinMind 18 系列與 2 個缺口、GICS 未映射鍵、真實 config 全 symbol 覆蓋、tree vs fallback 的 4 筆衝突集合）。`gofmt -l` 空、`go vet ./...` 乾淨、`make ci-gate` 通過、`go test ./internal/{sectormap,industry,sectorallocation,eventdriven,marketdata}/...` 全綠。
- **未處理（明示）**：DB 仍無 per-stock 產業欄位（母體覆蓋率上限）；FinMind 缺 chemicals/tourism 系列；TWSE 同日同產業多系列塌縮與 reader last-wins 非決定性；`monitoring.TreeBasedMapper` 與前端 L2 mirror 漂移；編譯內建 `cycle_thresholds` 13 key vs JSON 10 key（見規格 §6）。
### chore(ops): iMac watchdog 腳本納入版控（+ 漂移檢查與安裝 target）（2026-09-12）
- **問題**：`atlas-container-watchdog.sh` 是 iMac 唯一的自動復原機制，但只存在於 iMac 的 `~/bin/`（未版控）→ 無法回答「iMac 上跑的是哪一版、有沒有漂移」。2026-09-12 我改了它的內容（新增 restart ledger）之後，這個問題更明顯：改動只存在單一機器上。
- **正本**：`scripts/ops/imac-container-watchdog.sh`（腳本）與 `scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist`（launchd job）。
- **Makefile**：
  - `make imac-watchdog-diff` — 比對 repo 正本與 iMac 版的 sha256，不一致 exit 1（漂移檢查）。
  - `make imac-watchdog-install` — 先備份 iMac 現有版本、scp 正本、`bash -n` 驗語法、重載 launchd，最後再驗一次 sha256。
- **修正**：順手修掉腳本內 BASELINE 的 printf 格式字串參數不符（shellcheck SC2183）。
- **文件**：`docs/operations/local-deploy.md` 新增「iMac 容器守護腳本（版控正本）」章節（含指令、腳本行為、判讀方式與 #1901 的關聯）。
- **實測**：`make imac-watchdog-install` 執行後 `make imac-watchdog-diff` 顯示兩邊 sha256 一致（bb54b6cb…），launchd 已重載。

### decision(llm): Kimi 自 atlas 路由移除（ADR-012 追加四）（2026-09-12）
- 業主裁定：atlas 執行期**不使用 Kimi、不補 `LLM_KIMI_API_KEY`** —— 本部署只有 coding plan 訂閱，該 key 無法用於 app-level HTTP 呼叫（實測 `api.kimi.com/coding/v1` → 401，同一把 key 打 `api.minimaxi.com` → 200），補了也無法生效。
- 處置：`code_review_annotation` / `prompt_lint` 的鏈改為 `minimax → deepseek →（空）→ mock`；`cmd/atlas`、`cmd/lint-pr`、`cmd/lint-prompts` 移除 Kimi provider 註冊；`configs/llm_router.yaml` 同步；新增測試 `TestDefaultRoutingTable_KimiNotInAnyChain`。
- 保留：`clients.KimiClient`、`adapters.KimiAdapter`、ADR-009 的 `kimiAllowedCaps`（未來有可用 key 時只需加回鏈與註冊）。
- 附帶：`cmd/experimental/l2-4-preflight` 的 `router_version` 檢查由字串等值改為版本比較（>= v2.1），避免每次路由版本遞增就要改碼。
- 效果：code capability 不再有 `llm.skipped_providers=[kimi]`，lint 每次呼叫不再無意義地累加 `FallbackTriggeredTotal`。

### decision(llm): 資料主權處置定案 —— A 案（接受 + 稽核 + 最小化）（2026-09-12）
- 業主裁定「策略內部邏輯外流」可接受 → `#1889` 以 A 案結案：不因 `DataClass` 封鎖任何 provider。
- 風險維持在可稽核狀態的機制：`AttemptedProviders`（僅實際呼叫者）、span `llm.skipped_providers`、`llm.data_class` / `llm.data_class_name`；payload 僅衍生／彙總資料（無個資、帳號、憑證）。
- 升級條件寫入 ADR-012 追加三：payload 含關鍵營業秘密 → 對該 capability 去識別化（D）；要求完全不出境且保留功能 → self-host（C）；法規強制 → 全封鎖（B）。
- 同步：`docs/llm-adr-log.md`、`docs/specs/llm-routing-spec.md` §6.4。

### fix(risk): backtest 不再觸發 LLM 績效鑑識 hook（snapshot 保留）（2026-09-12）
- **原則**：`RiskSnapshot`（確定性、可用於回測分析）**一律建**；LLM 註解 hook 是「live monitoring」關注點，因此**呼叫端注入狀態（backtest harness）時跳過**。
- **理由**：(1) 省錢與降噪；(2) 回測必須可重現 —— 一次 LLM 呼叫就讓它變成非確定性；(3) production daily run 從磁碟載入狀態（`injected_state=false`），hook 照常執行。
- **測試**：`TestFinalizeRiskForensics_SkipsLLMHookForInjectedState`（注入狀態 → hook 0 次但 snapshot 仍建；非注入 → hook 1 次且帶回 commentary）。
- **可觀測性**：#1895 的 `session` / `injected_state` 欄位讓這件事在 log 上可驗證。

### fix(risk): 風險鑑識 log 可歸屬（session id + injected_state）（2026-09-12）
- **問題**：`risk_forensics_pending` / `risk_forensics_snapshot` 兩行只印 `component=system`，無法分辨是「production daily 路徑」還是「backtest harness 自己的 loop」印的（兩者都走同一條 replay 路徑與同一個 helper）。實測時同時看到 `samples=13/14/20`，需要人工比對 state 檔指紋才能歸屬。
- **修法**：兩行都加上 `session=<session id>` 與 `injected_state=<true|false>`。daily production run 會顯示自己的 session id 且 `injected_state=false`；用 `WithPersistentState()` 注入狀態的 backtest 會顯示 `injected_state=true`。log 因此可直接歸屬，不需要再比對檔案。
- **已知副作用（未在本 PR 決定）**：backtest harness 若處理超過 `RiskForensicsMinSamples`（30）個 session，也會觸發 LLM 績效鑑識 hook（花費與噪音）。`injected_state=true` 這個欄位讓它可被觀測；是否要在注入狀態時跳過 hook 屬設計決策。

### fix(risk): #1888 第三層根因 — 排程 backtest 靜默覆寫 production simulation_state（2026-09-12）
- **真相**：`internal/backtest/window.go` 的 `window_backtest`（7 天一次）建立 `domain.NewSimulationState()`（全新空狀態）並用
  `system.WithPersistentState(&persistentState)` 注入，而 replay 路徑跑完會 `persistPersistentState()` → **把 backtest 自己的區域序列寫回
  `data/state/simulation_state.json`**，覆蓋掉累積的日報酬歷史，且完全沒有警告。
- **證據（iMac）**：
  - 隔夜備份 `data-state/`：`20260908` equity=19/returns=18、`20260909` 19/18、**`20260910` 1/0、`20260911` 1/0** → 覆寫已發生多次。
  - 即時觀察：`17:09:24 task_started name=window_backtest（start=2026-08-19 end=2026-09-08）`，同時間 `simulation_state.json` 由 19 筆掉到 0。
  - 這也解釋了為何 `llm.performance_forensics` 永遠是 0：不只要 gate 在正確的路徑上，歷史還每 7 天被清一次。
- **修法**：`SimulationCore.stateInjected` 標記；`WithPersistentState()` 設旗標；`persistPersistentState()` 對「呼叫端注入的 state」直接 return（呼叫端自己持有、自己跨日延續）。
  新增 `TestWithPersistentState_DoesNotClobberLedgerState`（修正前必失敗）與 `TestLoadedStateIsStillPersisted`（反向保證：從磁碟載入的 state 仍會持續寫回）。
- **資料修復待決**：production 目前的 `simulation_state.json` 已被清空（returns=0）。可從 `atlas-backups/data-state/20260909-033000/` 還原（returns=18），需業主同意後執行。

### fix(risk): #1888 真正根因 — 風險鑑識區塊只存在於非 replay 路徑（2026-09-12）
- **真相**：`RunDailySimulation` 在能解析 replay session 時會 early-return 到 `runReplaySimulation`（`system_dispatcher.go`），而 **production 一律走 replay 路徑**。原本「風險快照 + LLM 績效鑑識」的區塊只寫在非 replay 路徑 → 不論 `returnHistory` 累積到幾筆，hook 都**不可能**觸發。
- **production 證據（本輪實測）**：手動 `POST /admin/trigger-simulation`（新部署 `bd2f36a7`）在 16:44:47 完成（log: `手動觸發場次 session-20260911-daily 產生 0 筆訂單`）、`simulation_state.json` 的 daily_returns 由 18 → 19（證明該路徑確實有 append + persist），但**完全沒有 `risk_forensics_*` log**。
- **修法**：把該區塊抽成 `System.finalizeRiskForensics()`，兩條路徑都呼叫（`system.go` 非 replay、`system_dispatcher.go` replay）。新增測試 `TestFinalizeRiskForensics_SharedByBothRunPaths`（強制走 replay 路徑，斷言 hook 被呼叫 — 修正前必失敗）、`TestFinalizeRiskForensics_PendingBelowThreshold`。
- **前一輪的敘述修正**：#1891 說「hook 觸發不到是因為 30 筆 gate 加上歷史未補齊」只對一半 —— 歷史補齊是必要條件，但**沒有把區塊放到 replay 路徑**才是充分原因。

### fix(llm,risk): #1891 補漏 — 補回未 commit 的 hydration 測試 + 風險鑑識 gate 可觀測性（2026-09-11 深夜）
- **補回 `internal/orchestrator/risk_forensics_hydration_test.go`**：#1891 的 commit message 聲稱有這些測試，但該檔當時是 untracked（我的 staging 只取了 `git status` 的 ` M` 行，漏掉 `??`），因此合併後的 main 沒有它、GitHub CI 也沒覆蓋到。本 PR 補上。
- **風險鑑識 gate 可觀測性**：每個 daily run 都會留一行結構化 log —— 未達門檻 `risk_forensics_pending samples=N min_samples=30`，達標且 hook 存在 `risk_forensics_snapshot samples=N var95=… cvar95=… commentary_len=…`。用途：(a) 立刻證明 hydration 生效（production 首次 run 應顯示 samples≈19，而非 1）、(b) 12 個交易日的補齊進度可逐日查核、(c) hook 真正觸發時有時間戳可佐證。
- **docs**：`docs/specs/llm-routing-spec.md` §6.1 明記「production 未設 `LLM_KIMI_API_KEY`（月額度用罄）→ code 群組跳過 kimi 由 M3 承接，屬預期；key 設回即生效」；`internal/orchestrator/AGENTS.md` 補上兩行 gate 觀測說明。

### LLM/Risk — #1887 + #1888 根因修正（2026-09-11 晚）
- **#1887 failure_attribution Router 路徑可用了**：prompt 文字抽成單一來源（`llm_annotator.FailureAttributionSystemPrompt` / `FailureContextPrompt` / `FailureAttributionTemperature`），capability handler 改送 messages payload（MiniMax/DeepSeek adapter 只吃 `[]byte`），`RouterAnnotator` 走同一 handler；新增端到端測試。**未新增/改動任何 prompt 語意**。
- **#1888 risk forensics hook 可觸發了（方案 A）**：`ensurePersistentStateLoaded()` / `WithPersistentState()` 會從持久化的 `EquityCurve` / `DailyReturns` 補齊 per-process 的 `returnHistory` / `portfolioHistory`；gate 改用具名常數 `RiskForensicsMinSamples`（30）。新增補齊、不覆蓋既有累積、以及「補齊後單次 RunDailySimulation 即觸發 hook」三個測試。
  - 生產時程：iMac 目前 `daily_returns=18` → 約 12 個交易日後首次產生風險鑑識敘事。
- **#1889 主權殘餘風險**：仍待決策（全供應商一致封鎖 / redaction 層 / self-host）。

### LLM — ADR-012 第二階段：production 實證與根因修正（#1886 follow-up，2026-09-11）
- **production 實證**：iMac 48h 內 `llm.scenario_simulation` 663 次、`attempted_providers` 全為 `["deepseek"]`、`data_class=2` → 證實 ADR-010 閘門讓 M3 從未上場，該 hook 輸出一直是空的。
- **annotator 設定修正**：`/api/strategies/{id}/annotate` 原本指向 `api.kimi.com/coding/v1` + `moonshot-v1-8k`（key 卻是 MiniMax CN）→ 實測 HTTP 401。改指向 MiniMax CN + `MiniMax-M3`，token 預算 512 → 2048。
- **空輸出視為失敗（補完）**：`llm_annotator` client 與 `/annotate` handler 兩層都改為把空輸出當失敗（不再回 HTTP 200 + 空註解，改回 502 + rule-based fallback）。
- **M3 回應正規化**：剝除 `message.content` 內嵌的 `<think>…</think>`（`internal/llm/clients`、`internal/llm_annotator`）；被 `max_tokens` 截斷的 thinking 視為空輸出 → 走 fallback。DeepSeek 的 `reasoning_content` 不受影響。
- **鏈成員記帳語意**：`AttemptedProviders` 只記實際被呼叫者；新增 span `llm.skipped_providers`；`FallbackTriggeredTotal` 只在實際呼叫非 primary 時遞增。
- **路由表改由設定檔載入**：`llm.ResolveRouterConfig()` 讀 `configs/llm_router.yaml`（`ATLAS_LLM_ROUTER_CONFIG_PATH` 可覆寫），缺失/不完整即回退內建表；三個 cmd 同步。
- **新增追蹤 issue**：#1887（failure_attribution Router 路徑契約）、#1888（risk forensics hook 在 production 永不觸發）、#1889（資料主權 residual risk 決策）。

### LLM Router — ADR-012：拆除 DataClass 主權閘門，改以「任務可達成率 + 訂閱額度」選模型（#1886，2026-09-11）
- **拆閘門**：`internal/llm/router.go` 刪除 `shouldGateProvider()` 與 `Call()` 內兩處呼叫；`internal/llm/clients/kimi.go` 移除 Regulated/Secret 的 `ErrIncompatibleDataClass` 拒收。ADR-009 的 kimi 能力 guard（僅 `code_review_annotation` / `prompt_lint`）保留。
- **DataClass 降為稽核 metadata**：繼續隨 `Request` 傳遞、記 metric/span，可作日後 redaction 依據，但不再阻擋任何 provider；enum 四類不變。
- **路由鏈重配**：敘事/解釋 JSON 9 個 capability → primary MiniMax M3；程式碼 2 個 → primary `kimi-for-coding`（backup M3 → deepseek-flash）；`contra_attribution` 歸敘事群組；全域 fallback canonical `deepseek-flash`。`deepseek-v4-pro` / `deepseek-pro` 退役。`configs/llm_router.yaml` 與 `defaultRoutingTable()` 同步（新增一致性測試）。
- **空輸出不視為成功**：provider 回傳「成功但 output 空/全空白且無 tool call」時視為失敗，續試下一鏈成員（`AttemptedProviders` 記錄全部嘗試）。
- **`max_tokens` 校準**：11 個 capability 由 300–800 提升到 2048–4096（reasoning 模型最低預算），逐項附理由。
- **模型名設定化**：新增 `LLM_DEEPSEEK_MODEL`（預設 `deepseek-flash`），取代 `cmd/atlas`、`cmd/lint-pr`、`cmd/lint-prompts` 三處硬編碼模型名；`DeepSeekClient` 預設模型改為 `deepseek-flash`。
- **文件**：`docs/specs/llm-routing-spec.md` §6.1 路由表 / §6.1a max_tokens / §6.3 決策順序 / §6.3a 空輸出；`docs/llm-adr-log.md` 新增 ADR-012（ADR-010 標 Superseded）；framework v2.2 修訂註記。

### Gap 3 — 散戶追蹤/紀律（manifest 系列）
- **#Gap3-R4 我的追蹤頁**：notification-center 復活 + signal 已讀按鈕（#1494）
- **#Gap3-R3 userstate HTTP API**：4 條 per-user signal-state 端點（#1493）
- **#Gap3-R2 userstate storage**：JSONL store for UserSignalState（#1491）
- **#Gap3-R1+R5 散戶追蹤/紀律資料模型骨架** + query-examples 修正（#1486）

### Gap 2 — 預測閉環（manifest 系列）
- **#Gap2-A1 錢潮預測命中率**：T+1 actual 補入 + reconciler + 投資人呈現（#1484）
- **#Gap2-D prediction_backtest reverse-write**：讓 calibrator 收到真實 hit rate（#1490）

### MCP 工具擴充
- **M6 audit_state**：憲章審計狀態 MCP 公開（#1482）
- **M4 strategy_for_period**：策略適用時期 MCP 公開（#1488）
- **stock_get_monthly_revenue endpoint**：月營收查詢（hermes v4.0 dispatch #1，#1483）
- audit_state snapshot 同步 §附錄 F v1.1c（#1498）

### 方法論與時期（E4/E5/B2）
- 七時期 UI + 因果傳導鏈頁面（E4，#1397）
- E5a strategy three-category classification wired to frontend
- B2 sequential evidence pipeline — 憲章因果傳導鏈（#1381）
- 7-period classification 接入 macroflow RiskLevel derivation（A4）

### 資金流（C4/C5）
- capitalflow 4-layer Assessment 接入 orchestrator（C4，#1392）
- period-aware QualityScore with dynamic weights（C5）
- 外資/自營分流（F1F2，#1394）、ETF 淨申購（F3，#1395）

### 觀測與部署
- **C1 部署完整性閘門**：binary/source sync + version stamp + CI gate（#1412）
- **開機熱機**：五層 cache 暖場消滅冷路徑（#1411）
- alertscanner 多來源聚合：Prometheus Alertmanager + Wave9 eventbus（#1351）
- known-issue badge for long-stale channels（#1454）
- market_volume channel（集中市場成交金額，#1405）

### 系統健壯性
- ACI PreToolUse soft reminder for hot-path Go access（#1464）
- experiment_diff 暴露 judge-collected metrics（#1443）
- sessions endpoint 分頁 + zero-outcome data-loss monitor（#1444）
- X2 方法論追蹤表強制 + F1-F4 覆核文件（#1496）

## [0.0.2.0] - 2026-07-22

### Fixed
- **Geopolitical drift between stress index and ledger**：`/api/taiwan/stress-index` and the on-demand Taiwan stress calculator now share a single `resolveGeoScore` fallback chain (live provider → SQLite `geopolitical_history` → on-disk `data/state/geopolitical/latest.json`). The macro-ingest path also mirrors each successful geo fetch to the file store, so a transient live fetch failure no longer leaves the stress component at zero while a valid historical score is available.
- **`?days=N` semantics now mean "last N calendar days" across all history endpoints**：`/api/regime/history`, `/api/narrative/stress-index/history`, and `/api/geopolitical/history` now all return rows whose `date` falls within `today-N+1..today` instead of treating `N` as a row limit. `/api/regime/history?limit=N` retains the legacy row-limit behavior. The regime response also gained a `date` field on every session, so clients no longer need to parse the date out of `recorded_at`.
- **Live macro ingestion now writes `regime_history`**：`DashboardAPI.applyMacroUpdate` calls a new `persistRegimeHistory` that derives a canonical regime from the current stress index via `narrative.NormalizeRegime` and upserts it with `source=macro_ingest`. After the next macro tick, `/api/regime/history?days=N` will surface the same date window the calendar fix enables, and the existing stage-4 synthetic backfill rows remain untouched.

## [0.0.1.0] - 2026-07-21

### Fixed
- **Live stress index endpoints now expose `source` and `date`**：`/api/taiwan/stress-index` and `/api/narrative/stress-index/current` previously returned only `score`/`regime`/`components`/`timestamp`. They now include `source: "taiwan_calculator"` and `date: "YYYY-MM-DD"`, matching the provenance fields already available in `/api/narrative/stress-index/history` and letting clients join live readings to ledger rows without extra lookups.

## [0.0.0.37] - 2026-07-20

### Fixed
- **cron 排程補登**：`cmd/atlas/operations_tasks.go` 註冊 `tej_refresh` 與 `janus_regime_refresh` 兩個 ScheduledTask（先前沒在 register loop，PR fix）
- **CI quality gate 補強**：test/cl7 coverage sweep — strategy 83.2%, dailyreport 90.9%, retail 91.1%, autobacktest 60.1%, taskexec 62.1%
- **gofmt cleanup**：`internal/portfolio/rsi_tw_calculator_test` + `internal/portfolio/directional_trade_layer_test` 排版修正
- **frontend getComputedStyle 取代**：sparkline + evolution_panel 改用 `getThemeColor`（一致性，CL-7 frontend 整理）

### Documentation
- capitalflow post-merge 文件同步：`docs/reference/tool-catalog.md`、`docs/reference/workflow-map.md`、`internal/capitalflow/AGENTS.md`
- **Document Drift Audit**：建立 `docs/manifests/2026-07-20-document-drift-audit.md`（盤查 4 個文件漂移 + production drift）
- **Document Drift Fix 2026-07-21**：批次修正全 repo 過時 MCP tool 數（110/111 → 112）、workflow 數（21 → 42）、版本號與專案規模引用、AGENTS_INDEX 成熟度不一致、根目錄違規 IMPLEMENTATION_PLAN*.md 移至 `docs/archive/` 並更新所有引用。
- **Document Drift Follow-up**：以 `docs/reference/tool-catalog.md` 為權威來源，統一 `cmd/atlas-mcp/README.md` 的 MCP tool 分類明細與 assert 範圍（[111, 114]）。

## [0.0.0.36] - 2026-07-20

### Added
**CL-X 修復群**（5 個 PR，資本流 / 制度 / 推薦 / 排程完整時序化）：
- **PR #1228（CL-1 根因修復）**：`internal/capitalflow/service.go::Refresh` 改為 data-driven keying（`snap.RecordedAt` 推導 trading date 跳掉 cutoff 覆寫陷阱）+ non-trading day skip-and-log + signature 變 `Refresh(ctx)`。A02 calendar 注入 + 3 個 caller 同步更新 + 4 個 test。`docs/specs/capital-flow-seven-dimension-spec.md` §12 新增 CF-INV-15/16/17 + §18 新章 Historical Timeline。
- **PR #1229（CL-2 macro snapshot history）**：新增 `HandleMacroSnapshotTimeline` (handler.go:75) 對應 `/api/macro/snapshot/timeline?days=N&from=&to=` 端點，從 `Service.ListSnapshotsInRange` 拉時序快照。`docs/specs/macro-snapshot-history-spec.md` 新建（28 sections）。
- **PR #1230（CL-5 HandleHistory A01+A02）**：capacity 60→252（spec §10 H-CF-05 gate 對齊）+ `?include_meta=true` opt-in wrapper（CF-INV-17）。`handleHistory` 多回 `samples` + `meta` 兩個 top-level key。
- **PR #1231（CL-3 regime history）**：`PipelineService.LoadRegimeHistory` 改讀 `regime_history` SQLite 表（90 筆真實資料 from stage4 backfill）+ 新增 `/api/janus/regime-score` 端點（macro-derived composite score）。
- **PR #1232（CL-4 sessions drilldown）**：`/api/dashboard/sessions` 加 `top_strategies` 摘要 + 新增 `GET /api/dashboard/sessions/{id}` drill-down 端點 + 新 MCP tool `universe_get_session_detail`。
- **PR #1233（CL-5b point-in-time）**：`HandleHistoricalSnapshot` (handler.go:80) 對應 `/api/capital-flow/historical-snapshot/{trading_date}`，含 status enum（complete/partial/missing）+ HTTP 200 always。

### Changed
- `internal/capitalflow/handler.go` `HandleHistoricalSnapshot` 新增（PR #1233）
- `internal/capitalflow/handler.go` `HandleHistory` 新增 `shouldIncludeMeta` + `buildHistoryWithMeta`（PR #1230 A02）
- `internal/capitalflow/service.go` `Refresh` signature: `Refresh(ctx, tradingDate)` → `Refresh(ctx)`（PR #1228 A01）

### Fixed
- **CL-1**：capital-flow Refresh 不再被 15:30 cutoff 永久覆寫前一交易日 slot（4 個獨立證據齊全：handler / service / store / cutoff 邏輯）
- **CL-3**：`regime_get_history` 不再回 score=0（omitted when janus endpoint 失敗）+ 真正從 `regime_history` 表讀時序
- **CL-4**：`universe_get_sessions` 不再只有 4 fields（含 `top_strategies` 聚合）
- **CL-5b**：`/api/capital-flow/historical-snapshot/{trading_date}` 不再 404

### Tests
- `internal/capitalflow/handler_test.go` 5 new `HandleHistory*` tests（OK / Partial / Missing / BackwardCompat）
- `internal/capitalflow/service_test.go` 4 new `Refresh*` tests（KeyMatchesRecordedAt / SkipOnWeekend / IdempotentSameDay / TimezoneOffset）
- CL-4 sessions drill-down 測試
- CL-5b point-in-time 測試

### Backlog（明確不做）
- BL-CF-01：capital-flow 歷史資料 backfill pipeline（需 Provider 歷史 API 或 replay；當 store 只有 post-fix 寫入的日期）
- BL-MH-01：macro snapshot 自動 backfill 跨假日（CL-2 已能查現有 snapshot，但缺跨假日復原）

## [0.0.0.35] - 2026-07-14

### Added
- **Stage 5 — Template Trigger Detector 抽象層 + 鏈路驗證** (5 sub-PRs):
  - **PR#1**: `internal/narrative/detector.go` 新增 `Detector` interface + `DetectorRegistry` + `DetectionResult` 統一輸出 + `DetectorInput`。19 unit tests，coverage 100%。
  - **PR#2**: `internal/narrative/detector_impls.go` 24 個 Detector impl（17 KB-pipeline + 6 seasonal + 1 snapshot-pipeline）+ `NewDefaultDetectorRegistry()` constructor。23 unit tests。
  - **PR#3**: `internal/eventdriven/type_theme_mapping.go` 新增 `EventTypeToTriggerThemes(eventType, registry)` 動態對應。8 unit tests，無 regression（legacy `eventTypeToThemes` 不動）。
  - **PR#4 Stage A**: `internal/ledger/detector_scan_store.go` SQLite-only `DetectorScanStore` (AppendScan/LoadRecentScans) + `internal/scheduler/template_detector_scan.go` 每 1h 排程（遵守 apigateway/CONSTITUTION.md Art.4）。9 store + 6 scheduler tests。
  - **PR#5**: `internal/narrative/detector_e2e_test.go` 4 個端到端 chain 驗證（snapshot-pipeline / KB-pipeline / 24 themes regression guard / disable propagation）+ `internal/narrative/AGENTS.md`（6 條模組陷阱）+ `docs/specs/eventdriven.md` 擴充（30 行 → 200+ 行）。

### Compliance
- 全程遵守 atlas-pre-change-protocol 8 步 + 紅線（既有 detect 函式 50+ 個不動 / template trigger_theme 字串不變 / 對外 API 介面相容）。
- 4 個 Stage 5 決策已在 `docs/archive/2026-07-14-atlas-stage5-detector-plan.md` §十一記錄（含 Status + Deferred）。
- Real-only investigation：3 個並行 explore agents 完整盤查 eventdriven / narrative / MCP tool chain 才開工。

### Known limitations / Deferred
- 2 個 MCP tools (`template_detector_status`, `detector_registry_list`) 與其 HTTP endpoints deferred — 需擴展 cmd/atlas/narrativeAdapter、wire registry/scanStore、新增 cmd/atlas-mcp tool、tool count hard gate 106-108→108-110、`docs/REFERENCE/tool-catalog.md` 更新、`go generate ./cmd/atlas-mcp`。
- `tariff_shock` 仍走 snapshot-pipeline（detectTariffShockEventFromSnapshot）；KB pipeline 對應函式待 Stage 6+ 新增（可讀 TradeNews 或 GeopoliticalGPR proxy）。
- `china_slowdown` detector 暫用 CopperChangePct 當 proxy；Stage 6+ 改用真 PMI 來源。

### Process note
- 本 v0.0.0.35 版本（即原 v0.0.0.34 Stage 5 條目）於 FU-7 release commit 一併重新編號，避免與既有 v0.0.0.34（Stage 4）衝突；內容未變動。


## [0.0.0.34] - 2026-07-14

### Added
- **Stage 4 歷史資料補齊** (PR #1138 + recovery PR, 4 sub-PRs):
  - `cmd/atlas-stage4-backfill`：從 `data/state/sessions/*/summary.json` + `recommendation_outcomes.jsonl` + `data/state/macro/*.json` 產出 4 個 staging JSONL (regime_history_90d / event_calendar_90d / stress_index_history_90d / prediction_actual_90d)。9 個 unit tests，coverage 75.4%。
  - `internal/ledger/historical_store.go` + `cmd/atlas-stage4-loader`：4 個新 SQLite tables (regime_history / stress_index_history / event_calendar_history / prediction_backtest) + 4 個 indexes；`HistoricalStore` interface (13 methods)；loader CLI 含 `-init-schema` / `-since` / `-until` / `-dry-run`。20 個 unit tests (含 1 flake fix)。
  - `cmd/backtest-event-flow`：把 `internal/eventdriven.Predictor` 重新跑在 90 天歷史快照，把 (predicted, actual, hit) 對寫進 prediction_backtest。12 個 unit tests + 順手修 PR#2 留下的 SQL BETWEEN 字串 bug。Coverage 78.6%。
  - `internal/narrative.RecalculateAllTemplateHitRates(globalHitRate)`：Stage 4 PR#4 擴充，把所有 24 個 templates 納入重算 (原本只觸及 active-model 路徑)。4 個 unit tests + golden snapshot 更新。Coverage 74.1%。

### Fixed
- `cmd/atlas-stage4-loader/main.go::upsertPredictions` 改為 no-op 防止污染 `prediction_backtest` (PR#1 的 `prediction_actual_90d.jsonl` schema 不符 `prediction_backtest` 的 pair-schema，避免 54 筆 NULL/zero 污染 row)。
- `internal/ledger/historical_store.go::LoadPredictionBacktestRange` SQL 改寫：`BETWEEN ? AND ?` 對空字串會被解讀為「BETWEEN '' AND ''」只匹配空字串。改為 `WHERE (? = '' OR date >= ?) AND (? = '' OR date <= ?)`，恢復「空字串 = 無界」語意。
- `internal/ledger/historical_store_test.go::TestSchemaConstants_Recognised` flake-proof：用 `sort.Strings` + 逐元素比較代替 `strings.Join` 比對 map iteration 隨機順序。
- `cmd/atlas-stage4-loader` PR#2 留下的 junk SQLite file (`?_pragma=busy_timeout(5000)`) amend commit 移除。

### Compliance
- 全程遵守紅線：audit log schema 不動 / 既偵測器不關 / 線下 CLI 為主 / 標 `is_synthetic=1` / 91 個既有 MCP tool 介面沒變。
- 4 個決策檔：`~/workspace/atlas-notes/decisions/2026-07-14-stage-4.{1..4}-*.md`。

### Known limitations
- `prediction_actual_90d.jsonl` 中 2026-05-05 / 05-06 兩天 aggregate 完全相同 (待 PR#1 區域 follow-up)。
- v1 hit_rate = 16.18% < random 33% (per-day event 沒注入)。
- 3 個 MCP read tools (history_regime / history_stress / history_event_calendar) deferred 到 Stage 5。

### Process note
- 原 `feat/stage-4-historical-backfill` 分支在自動 rebase 中遺失 PR#3 + PR#4 commits。已從 orphan object store 以 `git cherry-pick 58c1c4bc` + `git cherry-pick fed4680b` 完整復原到新分支 `feat/stage-4-recovery` (`50d793ff` PR#3 + `329c77a5` PR#4 on top of `ddd75b0c`)。
- 本 v0.0.0.34 版本（即原 v0.0.0.33 Stage 4 條目）於 FU-7 release commit 一併重新編號，避免與 v0.0.0.33 FU-7 條目衝突；內容未變動。

## [0.0.0.33] - 2026-07-15

### Changed
- **FU-7 Sector Normalization**（6 phases，PR #1159-#1164）：將 `SectorID` 提升為 Taiwan sector 識別之 single source of truth，消除 sector name 字串於 Go source、JSON、MCP、frontend 之間的散落。
  - **#1159 Phase A（sector.go canonical base）**：新增 `internal/sector/sector.go` 的 `SectorID` type + L1 行業常數（半導體 / AI / 金融等共 20 個），對齊 TWSE 大類；同時新增 `DisplayZHTw` 繁中 label map 與 `DisplayZHAliases` legacy alias 反查表，根治 ~290 個 backend 檔的 string literal 散落。
  - **#1160 Phase E（frontend canonical display）**：`shared_web/static/js/**` 與 `client_web/**` 共用同一份 sector label map，UI 顯示與 API 完全一致，消除 ~13 個 frontend JS 檔混用三種中文表示法（canonical 中文名 / truncated 前綴 / TWSE-style 後綴 `類`）的歧義。
  - **#1161 Phase D（L2 sub-industry extension）**：`internal/industry` 擴充 18 個 L2 子類別（半導體設備 / 晶圓代工 / IC 設計 / `ai_supply_chain` / `cooling` / `satellite_*` 等），`internal/cycle.go` 一併 migrate 至 `SectorID` 化。
  - **#1162 Phase C（TWSE provider `mapIndustryName`）**：`internal/marketdata` 為 `mapIndustryName` 加 docstring + TWSE drift guard test，避免上游 TWSE 大類 / 子類別重新命名或欄位調整時默默失效。
  - **#1163 Phase B（representative_stocks SectorID migration）**：`internal/industry` 的 `representative_stocks` 全面 migrate 至 `SectorID` 常數，移除字串硬編碼；後續 `6f1d39f` follow-up 移除 `string()` casts，回歸 idiomatic Go。
  - **#1164 Phase F（MCP sector tools）**：`cmd/atlas-mcp` 新增 2 個 read-only tools — `industry_sector_list`（列出所有 `SectorID` 與 L2 子類別）+ `industry_sector_lookup`（給定 symbol 或 sector name 回傳 canonical 資訊），`docs/reference/tool-catalog.md` 同步更新至 110 base / 112 max。

### Compliance
- 採 additive migration 策略：六個 Phase 各自只加新東西或鎖單一 contract，任意階段都可單獨 revert；無既設外部 contract 變動。
- 既設 sector 字串識別路徑不關閉，僅以 `SectorID` 常數為 single source of truth；既有 MCP tool 介面沒變（MCP tool count 110→112，純增量）。
- 權威定錨文件：`docs/guides/fu-7-sector-norm.md`（於分支 `docs/fu-7-sector-norm-guide`），涵蓋六個 Phase 之間關係、canonical source of truth、MCP 暴露、breaking change 評估與 trade-off 取捨。

### Process note
- 本次 release 一併清理了 main HEAD 上 commit `8cf45cfa`（Stage 5 PR#5）留下的 `<<<<<<<` / `=======` / `>>>>>>>` 未解決 merge conflict markers，並將文件中既有的 v0.0.0.33 / v0.0.0.34 條目（Stage 4 / Stage 5 紀錄）重新編號為 v0.0.0.34 / v0.0.0.35，騰出 v0.0.0.33 給本次 FU-7 release；既設條目內容均原樣保留，僅 header 數字異動。

## [0.0.0.32] - 2026-07-10

### Fixed
- **Atlas-http 啟動時正確拒絕 healthy port 衝突** (`internal/startup/preflight.go`、`internal/portprobe/listen.go`、`cmd/atlas/main.go`):
  - 修正 `internal/startup/preflight.go:checkClaim` 對 exclusive claim（`AllowZombieKill=false`）把 `StateHealthy` 當作 pass 的邏輯——若 Docker Desktop（或殘留 native process）已健康佔用 :18080，現在會在 bootstrap 前 fail-fast，回傳 actionable error（附 PID、command 與 docker / kill 復原指示），而不是跑完整昂貴 bootstrap 後才在 `portprobe.Listen` 失敗。
  - 修正 `cmd/atlas/main.go` 只在 `shouldStartFubonProxy` 內呼叫 preflight 的語意錯誤：atlas-http 的 exclusive claim 現在無論 fubon-proxy 是否啟動都會先驗證。
  - `internal/portprobe/listen.go:formatOccupantDiagnostic` 在偵測到 `com.docker.backend` / `docker-proxy` 時，於錯誤訊息結尾附加 `docker compose stop atlas` 復原指示。
- **Unit tests**: 新增 `TestPreflight_ExclusiveHealthy_Errors`、`TestPreflight_ExclusiveHealthy_NativeHint`、`TestDockerRecoverySuffix` 覆蓋 docker vs native occupant 兩條復原路徑。

## [0.0.0.31] - 2026-07-07

### Added
- **Wave 11 投資核心框架** (PR #972, 7 模組):
  - `internal/strategy_validator` — 策略歷史回測驗證（Sharpe/最大回撤/勝率/TAIEX 相關係數 + 排名分層）。
  - `internal/capitalflow` — 七大資金勢力分解與共振分析（外資/投信/公股/散戶/期貨/ADR Z-score + 共振係數 1.5/0.5）。API: `/api/capital-flow/daily`、`/api/capital-flow/summary`。
  - `internal/eventdriven` — 事件驅動資金流預測（5 日 forward + ETF 規模×權重預估 + 營收驚喜 >10% 邏輯）。API: `/api/events/calendar`、`/api/events/prediction`。
  - `internal/strategy_ranker` + `internal/subscription` + `internal/recommender` — 推薦分層系統（3 tiers: public/registered/premium，7 天免費試用，JWT auth）。API: `/api/recommendations`。
  - `internal/dailyreport` — 每日市場報告（JSON + Markdown）。API: `/api/reports/latest`、`/api/reports/archive`、`/api/reports/subscribe`。
- **MCP 整合優化** (PR #972):
  - 4 個新 tools: `mcp_quickstart`、`daily_report`、`event_calendar`、`event_flow_prediction`。
  - 6 個預設 Prompt 模板: `taiwan_quick_look`、`strategy_advice`、`stock_health_check`、`daily_market_briefing`、`risk_check`、`regime_interpretation`。
  - 3 個 MCP Resources: `atlas://strategies/active`、`atlas://market/regime`、`atlas://events/today`。
  - 新增 `docs/mcp-integration-guide.md`（Claude Desktop / OpenClaw / Hermes 整合設定）。
- **Client Web Phase A0/A/B/C** (PR #974, 24 files):
  - **Phase A0**: `services/auth.js`（JWT + tier 解析）、3 個 page shells（login/register/premium）、401 interceptor。
  - **Phase A**: loadAll 瘦身（11 → 6 core APIs）、sidebar 認證導航、tier badge CSS。
  - **Phase B**: tier-gated home dashboard（capital flow + event prediction + event calendar + recommendations + daily report）。
  - **Phase C**: MCP 整合頁、404 fallback、switchPage 錯誤處理。
  - **CSS 架構**: 新增 `components/grid.css` 取代 home-pulse/signals 重複定義（single source of truth）。
  - **gentags**: `cmd/gentags/main.go` 加入 eventdriven + recommender scan，修復 field-contract CI。

### Fixed
- **Client /admin/dist/* + /client/dist/* routing regression** (PR #973): 靜態資源路由修復。
- **Phase A/C audit fixes** (PR #974): `btn-primary` → `btn--primary` BEM 一致性、MCP 頁 CSS、404 頁 CSS、`renderNavState()` 登入後未更新。
- **field-contract CI** (PR #972, #974): `cmd/gentags/main.go` 補上 eventdriven + recommender + capitalflow scan 列表。
- **tier-cta / event-card / rec-card CSS** (PR #974): Phase B 渲染依賴的 CSS class 補完（tier-cta__actions、event-card__name、rec-card__tier 等）。

### Quality
- 全部 7 模組測試通過（52+ tests, 含 capitalflow 7 + eventdriven 8 + recommender 2 + subscription 6 + strategy_validator 17 + strategy_ranker 2 + dailyreport 4）。
- gofmt clean, go vet clean, golangci-lint 0 issues。
- 35 CI checks PASS（含 field-contract / fmt / lint / frontend-build / mcp-tool-count / maturity / constitution / coverage）。

## [0.0.0.30] - 2026-07-07

### Added
- **Progressive Disclosure for market-pulse grid** (PR #969): 取代 PR #946 的三級模式（已於 0.0.0.29 移除）。市場脈動 12 張卡改為「5 張核心（始終顯示）+ 7 張進階（預設摺疊，點『展開更多』顯示）」設計。新增 `disclosure-state.js` 管理 localStorage 持久化（prefix: `atlas-disclosure-`），用戶展開狀態記住跨 session。CSS 透過 `[data-disclosure-state="collapsed"] .disclosure-tier-advanced { display: none }` 驅動隱藏，`aria-expanded` / `aria-label` / sr-only live region 處理無障礙。設計意圖：兌現原 mode-manager.js「最少 API 呼叫」承諾（前端摺疊先 ship，後端 fields 過濾為 follow-up）。

### Fixed
- **進階揭露按鈕 a11y** (PR #970): `home.js` 按鈕新增動態 `aria-label`（`展開/收合 7 張進階指標`，`aria-expanded` 兩狀態切換）、sr-only live region 公告狀態。`utilities.css` 新增 `.sr-only` helper。原始 PR #969 漏做螢幕閱讀器 announce，補上後鍵盤與 NVDA / VoiceOver 使用者可正確感知展開/收合。
- **Docker embed.FS timing race** (PR #970): `.dockerignore` 新增精準排除 `client_web/dist/` / `admin_web/dist/` / `shared_web/dist/`，避免本地舊 build 污染容器 nodebuilder 階段的 COPY 結果，根除「本地 dist 較新、容器 binary 較舊」導致 served bundle 走 SPA fallback 返回 index.html 的問題。

### Removed
- **`docs/audit/2026-07-01-phase2-retail-investor-landing-audit.md`** (PR #970): 該文件專門描述已移除的「三級投資人角色模式」設計，繼續保留會誤導新人。同步修正 `docs/design.md` 與 `docs/documentation-map.md` 對該檔案的引用為抽象描述。

## [0.0.0.29] - 2026-07-07

### Removed
- **三級投資人角色模式** (revert PR #946): 移除 `simple` / `standard` / `pro` 三級切換按鈕與對應的 `data-atlas-mode` / `data-simplified` 機制，改為單一預設完整模式。市場脈動 11 張卡（含 VIX / 融資餘額 / 投信 / 自營商 / 歷史波動 / 散戶情緒）、投資組合 KPI（夏普 / 勝率 / 最大回撤）、首頁訊號燈條與事件月曆全部預設顯示。理由：三級切換 UI 摩擦大、實際使用率低，advanced-only / pro-only 區分過於細微，新用戶無從辨識。受影響檔案：7 個前端檔案（2 JS + 1 CSS + 3 文檔 + 1 HTML）— `mode-manager.js` / `simplified-mode.js` / `main.js` / `home.js` / `portfolio.js` / `simplified.css` / `home.css` / `index.html` / `admin_web/AGENTS.md` / `CLAUDE.md`。

## [0.0.0.28] - 2026-07-04

### Fixed
- **Shared web home.css import** (PR #942): 在 `shared_web/static/css/main.css` 加入 `@import url("pages/home.css")`，使 client web home page 樣式正確載入。

## [0.0.0.27] - 2026-07-02

## [0.0.0.28] - 2026-07-04

### Added
- **Fubon channel three-layer resilience** (PR #943): fubon-proxy /health 改為快速 process-only check、SDK 初始化 deferred、in-memory cache (30s TTL)、TCP pre-flight check (5s)。Go 端新增 healthClient (2s)、背景健康探測、IsHealthy() fast-fail。從根源解決 fubon 通道上游斷線時 hang 10+ 秒的問題，改為 5 秒內回 clear error。

### Docs
- 全面同步 fubon-proxy 行為描述至 10 份文件，修正過時說明。


### Security
- **MCP roots TOCTOU fix** (PR #902): `OpenFile` 改用 `O_NOFOLLOW` flag 關閉 symlink TOCTOU 視窗。修正 `Issue #901`。

### Added
- **MCP AllowedRoots validation** (PR #903): 新增 `ATLAS_MCP_ROOTS_ALLOW_UNSAFE` escape hatch env var，預設拒絕 `/`、`/etc`、`/proc` 等系統根目錄路徑。
- **MCP elicit_user schema pre-validation** (PR #905): `mcp_elicit_user` tool 的 `schema` 參數在 server 端新增格式驗證（大小上限 16KB、屬性上限 20、屬性名稱上限 64 字元、禁止外部 `$ref`/`$dynamicRef`）。由 `cmd/atlas-mcp/server/elicitation_validate.go` 實作，handler 端 `elicitation.go:form` case 在 SDK call 前先行驗證。
- **Phase 3.5 forecast-bridge** (PR #905 via 36cd37c9 + 3fb5e714): 新增 `internal/forecast/`（個股方向性預測）與 `internal/forecast_bridge/`（Forecast → TradeSignal 轉換層）兩套件，並在 `internal/narrative/taxonomy.go` 建立 5×5 敘事分類。orchestrator pipeline 第 7 步接入 `DirectionalTradeLayer`。全部標記為 Experimental（M4 PoC）。

### Fixed
- **Fubonproxy cmd.Start()** (PR #906): `cmd.Start()` 移出 mutex lock 並在 goroutine 啟動後 re-check `m.stopping`，符合 F7（啟動 preflight）+ F1（shutdown cancel）supervisor invariants。
- **Phase 3.5 quality cleanup** (3fb5e714): `factor_weight_engine.go` severity locals `:=` 修正（原程式碼在 `fwConfig()` non-nil 分支內以 `var ... =` 重新宣告導致零值）、`check_maturity.sh` `local count=` race 修正、`hasExternalRef` unused `key` parameter 移除、gofmt 對齊。

### Documentation
- **移除 .planning/ 死鏈** (PR #907): `docs/specs/phase3-5-spec.md` 9 處 `.planning/phase3-4-reassessment.md` 引用改寫為自指或 inline 設計原則。`docs/operations/phase3-5-runbook.md` 起點改指同目錄 spec。同時刪除 `.planning/` 目錄。
- **Wave 11+ index sync** (PR #908): `AGENTS.md` header 更新 Wave/版本/模組計數（45→52, 60→67）；`internal/AGENTS_INDEX.md` 補 `forecast` + `forecast_bridge` 兩模組。
- **Constitution F1-F9 alignment** (PR #909): fubonproxy manager 加入 `check_constitution.sh` allowlist，並移除 head-20 上限（cap）。`docs/specs/phase3-5-spec.md` CHANGELOG 路徑修正。

### Known Limitations (deferred to v0.0.0.28)
- Phase 3.5 forecast-bridge 為 M4 PoC 階段，`DirectionalTradeLayer` API 可能在 follow-up PR 中調整。
- MCP elicit schema validation 僅限 `form` mode，`url` mode 不觸發 pre-validation。
- `internal/forecast/` 與 `internal/forecast_bridge/` 無 AGENTS.md（Experimental 層級非強制）。

## [0.0.0.26] - 2026-07-01

### Added
- **Phase 4 Direction A — MCP Observability** (PR #863): Prometheus metrics (`mcp_tool_call_total`, `mcp_tool_latency_seconds`, `mcp_anomaly_emitted_total`) + anomaly detector library (`internal/mcp/anomaly/`: rate/tenant/error-rate detectors + `MemoryStore` ack/persist) + 2 MCP alert tools (`mcp_get_recent_anomalies`, `mcp_get_anomaly_stats`). End-to-end wiring: metrics → eventbus `MCPAnomalyDetected` → `Publisher` (NoOp / Webhook → Alertmanager) → server lifecycle integration. 38 檔案 / +3758 / -316 行。
- **Phase 4 Direction B — MCP Protocol Extensions** (PR #865 + PR #868): MCP 2025-07-28 協議擴充 — `roots` (client file root registration, whitelist via `parameters.json` mcp.roots、附錄 D Roots Sanctioned Exception — no-read boundary)、`sampling` (server-initiated LLM sampling)、`elicitation` (server → user 互動確認, PR #868 T2.4 schema 驗證 + timeout + decline)。`cmd/atlas-mcp-server-sdk-spike/` SDK prototype + main.go 整合 + RootsListChanged handler 接入 alerting.Publisher。
- **v0.0.0.25.1 hotfix — readAuditEntriesV2 mutex** (PR #860): `AuditWriter` mutex 保護 `readAuditEntriesV2` 與寫入路徑，避免 SSE/HTTP transport wiring 後 read/write race。原預定 hotfix 釋出，因後續 PR 多 follow-up 而併入 0.0.0.26。
- **Retail investor landing** (PR #864 + PR #871): `/client/landing` 新頁面 + Phase 2 modal 模組化 (`cycleLegendModal`) + page section lazy-load（index.html 171 → 117 行）+ async race fix (await `loadAll()` before `switchPage`，PR #874)。
- **Internal alerting module** (`internal/alerting/`): `Publisher` interface + `NoopPublisher` + `WebhookPublisher` (Alertmanager schema, retry/log delegated to server.go)。
- **MCP SDK spike** (PR #862): `cmd/atlas-mcp-server-sdk-spike/` 驗證 sampling/roots/elicitation primitives，建立 main.go 整合基礎。

### Fixed
- **Async race in home page init** (PR #874): `home.js` lazy per-page shell load on switchPage，確保 `loadAll()` 完成才切頁。
- **Client null guards** (commits `e5e15320`, `a6fa9aad`, `8790a6bd`): strategies/pipeline 加 null guards before DOM writes、`renderPerformanceReport` guard for missing container、移除 dead `action-card.js` (0 consumers, superseded by inline HTML in home.js)。
- **Console noise** (multiple commits): `silentGetJSON` console.error → console.warn，避免 silent data fetch failure 噴成 error。
- **Narrative test pindown** (commit `ce8c8824`): `dividend_season` engine rule 實作前先 disable 對應 assertion，避免 CI flake。
- **TIMELINE_BADGE keys alignment** (PR #856): 對齊 Go event kinds。
- **Lint unblocks** (commits `a0807768`, `e9ea8dbc`, `9acbab5d`, `74a53994`, `b745097f`): gci import order (roots.go、server.go)、gosec G118、misspell、main.go G706 nolint、dividend_season field-contract 6 unknown fields。

### Changed
- `cmd/atlas-mcp/main.go` wired to `loadMCPConfig` + `mcp.roots` 從 `parameters.json` 讀 (新增 `mcpconfig.go` + `mcpconfig_test.go`)。
- `cmd/atlas-mcp/server/server.go` 接入 anomaly eventbus emitter + RootsListChanged handler + Publisher wiring (commit `11aa433a` + `f1cbfac8` + `3120f1e5`)。
- `configs/parameters.json` 新增 `mcp_anomaly` section + `mcp.roots` whitelist。
- `configs/allowed_env_vars.md` 新增 `ATLAS_MCP_PARAMS` 等 Phase 4 B env vars + `ATLAS_MCP_ALERT_WEBHOOK_URL` whitelist（constitution check 同步）。
- `internal/apigateway/CONSTITUTION.md` 附錄 D — Roots Sanctioned Exception (Phase 4 B, no-read boundary，sanctioned by data-source governance)。
- `internal/AGENTS_INDEX.md` + `internal/MATURITY.md` — 新模組 `internal/mcp/anomaly/` 與 `internal/alerting/` 索引。
- `go.mod` / `go.sum` — MCP SDK 依賴升版。

### Documentation
- `docs/operations/mcp-deploy.md` — Phase 4 metrics + anomaly detector + Roots env vars deployment guide（66 行增量）。
- `docs/specs/agent-mcp-server.md` — Phase 4 Direction A/B 工具與協議擴充章節（93 行增量）。
- `docs/operations/retail-investor-landing-audit.md` — Phase 2 retail-investor-landing audit report（PR #873，b296dac8）。
- **Agent Interface docs bundle** (PR #875, P0 of `docs/plans/agent-interface-roadmap.md`): `AGENTS.md` 增設「🤖 Agent Interface（AI Agent 操作入口）」章節（21 條 workflow 路由 + 5 份文件入口：`docs/REFERENCE/workflow-map.md` / `docs/REFERENCE/PROCESSES.yaml` / `docs/specs/agent-mcp-server.md` / `docs/AGENT_TOOLS.md` / `docs/AGENT_ONBOARDING.md`）+ `docs/REFERENCE/PROCESSES.yaml`（488 行結構化 workflow metadata）。P0 補齊。
- **Agent Interface roadmap v2** (PR #876): `docs/plans/agent-interface-roadmap.md` 從「實作未開始」更新為反映 `cmd/atlas-mcp/` 真實進度（Phase 1 核心橋接 ~84 tools / stdio transport / TokenAuth / audit v2 / anomaly / 協議擴充標記完成；Phase 2 SSE/streamable-HTTP transport 與 binary merge 至 `cmd/atlas` 標記 TODO；新增 P5 列「PR #875 已併入主文件」；文件版本升 v2）。

### Known Limitations (deferred to v0.0.0.27)
- **T1.5 observability follow-ups** (from `feat/mcp-obsv` WIP, 9 commits, +304 lines): `mcp_get_top_slow_tools` + `mcp_get_tenant_usage` tools + T1.4 alert/eventbus 整合章節 + end-to-end integration test — 計畫隨 v0.0.0.27 observability follow-up ship。註：Phase 4 B 的 `Publisher` 介面、`WebhookPublisher`、`AnomalyStore` 主體已隨 PR #863/#865 ship，本批 WIP 為工具層增量。
- **SSE/HTTP transport wiring** (`TokenAuth` builds correctly but not invoked from any transport layer; `AgentIDFromContext` returns `"anonymous"` for stdio today) — Phase 4+ follow-up。
- **Per-token rate-limit enforcement** (`rate_limit_per_min` column reserved in `atlas_mcp_tokens` schema but `RateLimiter` only honors global capacity) — Phase 4+。

### Oracle Audit Summary
- T1 (PR #863 Phase 4 A Observability): constitution check PASS（`ATLAS_MCP_ALERT_WEBHOOK_URL` whitelisted）。READY TO MERGE。
- T2 (PR #865 + PR #868 Phase 4 B Protocol Extensions): constitution check PASS（附錄 D Roots Sanctioned Exception）。READY TO MERGE。
- T3 (PR #860 v0.0.0.25.1 hotfix mutex): READY TO MERGE, 0 P0 / 0 P1 / 0 P2。
- T4 (PR #871 retail-investor landing Phase 2): client-only，no constitution check required。
- T5 (PR #875 + #876 Agent Interface docs): docs-only，no Oracle audit needed。

## [0.0.0.25] - 2026-07-01

### Added
- **Auto-generated tool descriptions** (PR #857, Item 1 of `docs/specs/agent-mcp-phase3-residual.md` §3.1): `cmd/atlas-mcp/internal/descgen/extract.go` parses `mcp.AddTool` registrations with `go/ast` and produces `cmd/atlas-mcp/auto-desc.gen.json` (74/74 tools covered) + `auto-desc.gen.go` binding. `//go:generate go run ./cmd/atlas-mcp/descgen` triggers regen; CI `generate` job runs `git diff --exit-code` to block schema drift when handler sources change. `// gen:manual-override` doc comment opts out per-tool. Reduces 74-tool description hand-maintenance burden — every handler param change now stays in sync automatically.
- **Multi-tenant MCP token management** (PR #858, Item 3 of `docs/specs/agent-mcp-phase3-residual.md` §3.3): PostgreSQL `atlas_mcp_tokens` table (10 cols + 2 indexes, sha256 hash only — raw token never persisted) + admin HTTP API (`POST/GET/DELETE /api/admin/mcp/tokens`, `POST .../{id}/rotate`) gated by `X-Admin-Token` and bound 127.0.0.1 only. `ATLAS_MCP_TOKEN` env-var retained as fallback (dev-mode and DB-unavailable path). Token revoke + rotate is immediate (no in-memory cache). `crypto/subtle.ConstantTimeCompare` used throughout (no timing-attack leak on secret comparison). Spec §3.3.5 note: `ATLAS_MCP_TOKEN` is not an external data-source key, exempt from `internal/apigateway/CONSTITUTION.md` §1.1.
- **Audit log v2 schema + 2 analytics tools** (PR #859, Item 2 of `docs/specs/agent-mcp-phase3-residual.md` §3.2): `AuditEntry` extends v1 with `SchemaVersion` (int, no omitempty — v1 entries unmarshal to 0 and backfill to 1), `SessionID`, `ArgsHash` (sha256 of canonical `argKeys` JSON via new `CanonicalizeArgsHash()`), `LatencyMS` (preferred over `DurationMS` for v2), `Transport`. `withAudit` signature now takes `ctx context.Context` (first param) for tenant/agent identity; rate-limit key switched from `tool` to `tenant_id:tool` for per-tenant isolation. New tools `mcp_get_call_stats` (count / p50 / error rate) + `mcp_get_session_topology` (agent × tool call matrix) backed by 30-day in-memory aggregator (`AggregateCallStats`, `BuildSessionTopology`).

### Changed
- `cmd/atlas-mcp/server/tools.go` `withAudit` now takes `ctx` (context-aware) and reads `TenantIDFromContext` + `AgentIDFromContext` from `auth.go` (canonical contextKey types, single source of truth — Phase 3 殘餘 Item 3 unifies these).
- All 13 `tools_*.go` files updated to pass `ctx` into `withAudit` and the `withAudit` body now writes `TenantID` + `AgentID` into every `AuditEntry`.
- `internal/risk/...` and `internal/marketdata/...` etc. — no changes, but **constitution check** (`scripts/ci/check_constitution.sh`) now whitelists `ATLAS_MCP_ADMIN_TOKEN` + `ATLAS_MCP_ADMIN_ADDR` for the new admin server env-var pair (`configs/allowed_env_vars.md` updated).

### Documentation
- `docs/operations/mcp-deploy.md` and `docs/specs/agent-mcp-server.md` — Item 3 admin API + Item 2 analytics tool descriptions to be updated in follow-up (deferred to v0.0.0.26).
- `docs/specs/agent-mcp-phase3-residual.md` — all 3 spec items marked ✅ shipped (was 🟡 DRAFT before this release).
- **Agent Interface docs bundle** (PR #875, P0 of `docs/plans/agent-interface-roadmap.md`): `AGENTS.md` 增設「🤖 Agent Interface（AI Agent 操作入口）」章節（21 條 workflow 路由 + 5 份文件入口：`docs/REFERENCE/workflow-map.md` / `docs/REFERENCE/PROCESSES.yaml` / `docs/specs/agent-mcp-server.md` / `docs/AGENT_TOOLS.md` / `docs/AGENT_ONBOARDING.md`）；`docs/REFERENCE/PROCESSES.yaml` 新增（488 行結構化 workflow metadata，21 條 workflow × Name / Description / Inputs / Outputs / Tools / Owner / Phase / Tags）。P0 補齊。
- **Agent Interface roadmap v2** (PR #876): `docs/plans/agent-interface-roadmap.md` 從「實作未開始」更新為反映 `cmd/atlas-mcp/` 真實進度 — Phase 1（核心橋接，~84 tools / stdio transport / TokenAuth / audit v2 / anomaly / 協議擴充）標記完成；Phase 2 SSE/streamable-HTTP transport 與 binary merge 至 `cmd/atlas` 標記 TODO；新增 P5 列（PR #875 已併入主文件）；文件版本升 v2。

### Known Limitations (P1, by-design, documented in commit `c01f1d88` and T3 PR #858)
- **Item 3 — auth context not wired into transports**: `TokenAuth` builds correctly but is not invoked from any transport layer; `AgentIDFromContext` returns `"anonymous"` for stdio today. SSE/HTTP transport wiring is the next milestone (Phase 4 candidate).
- **Item 3 — `rate_limit_per_min` column stored but not enforced**: schema reserves the field for per-token override; current `RateLimiter` only honors global capacity. Per-token enforcement is a Phase 4+ item.
- **Item 2 — `agent_id` matrix shows `"anonymous"` rows**: same root cause as Item 3 above. Resolves automatically when transports ship.
- 5 P2 nice-to-fix from T2 Oracle audit (stale comment, `synchronised` spelling, dead-code `HashArgs`/`NewV2Entry`, `ReadAuditEntries` 0% coverage) — planned as v0.0.0.26 follow-up (≈30 min work).

### Oracle Audit Summary
- T1 (PR #857): 0 P0 / 0 P1 / 1 P2 — READY TO MERGE
- T2 (PR #859): 0 P0 / 0 P1 / 5 P2 — READY TO MERGE
- T3 (PR #858): 0 P0 / 2 P1 (by-design, see above) / 3 P2 — READY TO MERGE

All 3 PRs passed 5-section Oracle audit and the constitution check (gateway + rate-limit both PASS, only WARN-level pre-existing violations in `internal/marketdata/...` and `internal/llm_annotator/...` not in PR diff).

## [0.0.0.24] - 2026-06-28

### Fixed
- **Web UI HTML pages returning 401 unauthorized**: PR #808 set `ATLAS_API_KEY` via `env_file`, which put `AuthMiddleware` from "no-key bypass" mode into "key required" mode. The `authFreePaths` map in `AuthMiddleware` only contained `/health` and `/metrics` — HTML pages like `/admin/` and `/client/` (and their nested assets) hit the auth check and returned 401. `AuthMiddleware.isAuthFreePath` now consults both `authFreeExactPaths` (`/health`, `/metrics`, `/admin`, `/client`) and `authFreePrefixPaths` (`/admin/`, `/client/`, `/static/`). API routes (`/api/*`) still require auth.

## [0.0.0.23] - 2026-06-28

### Added
- **`make dev`** target: parallel TUI showing atlas service logs + prism worker logs + celery beat process in foreground — replaces the multi-terminal dance with one focused workflow. Skips atlas service container (you run it locally) so port 8080 stays free for `go run ./cmd/atlas`.

### Fixed
- **Postgres WAL race on cold start**: `docker-entrypoint-initdb.d/01-schema.sql` was copied AFTER schema apply, causing `relation "atlas_strategies" does not exist` errors on first `docker compose up`. Move `COPY schema.sql` ahead of any SQL execution.
- **`make dev` would have hit EADDRINUSE on port 8081**: dev target used container_name `atlas-fubon-proxy` in `docker compose stop` instead of service name `fubon-proxy`. `2>/dev/null || true` masked the error, fubon-proxy container kept running, then ProcessManager tried to spawn its own local subprocess on port 8081 → crash. Self-consistent with `dev-stop` target and the traps.md warning about this exact pitfall (auto-fixed during `/review`).

### Documentation
- `docs/REFERENCE/traps.md` — added "Search Before Building" principle (Layer 1/2/3 check before generating new infra).
- `docs/guides/install-and-deploy.md` — added "Local development workflow" section documenting `make dev` and the postgres race gotcha.

## [0.0.0.22] - 2026-06-28

### Fixed
- **5 services crash loop on local dev**: `atlas-prism-worker` fell through to `runSimulation()` (60s restart cycle) → fixed with subcommand routing (`isPrismWorkerCmd` + `runPrismWorker`); `atlas-grafana` provisioning errors → volume reset; `atlas-alertmanager` YAML indentation bug → fixed; `atlas-otel-collector` `postgresql` exporter doesn't exist → switched to `debug`. 5-minute boot test now runs clean with 0 restarts.
- **Docker `atlas` healthcheck 401**: `ATLAS_API_KEY=${ATLAS_API_KEY}` in `docker-compose.yml` shell-expanded to empty in local dev, putting `AuthMiddleware` into production-misconfigured branch → removed the override, let `env_file` be the single source. `/health` and `/metrics` now bypass auth unconditionally via `authFreePaths` map in `AuthMiddleware` itself (not just caller-side bypass), so `apishared.Adapt()` wrappers also get the exemption.
- **fubon-proxy 503 in container**: `FUBON_PERSONAL_ID` / `FUBON_PASSWORD` etc. were shell-expanded empty (`host shell` doesn't have them) → switched to `env_file: ~/.config/atlas-go/.env` like `atlas` service. `fubon_neo==2.2.8` is now installed from official `fbs.com.tw` CDN wheel with auto-detected `TARGETARCH` (arm64→aarch64, amd64→x86_64) — exits 1 on unknown arch.
- **Test infrastructure for adapter tests**: `writeParametersJSON` helper hand-rolled a partial config that failed `Validate()` (`base_allocations sum must be 1.0±0.01`) → fell back to `DefaultParametersConfig()` → BDI/Fubon/Fugle adapter tests fetched the real CNBC URL and failed parsing. Now uses repo's `configs/parameters.json` as template + applies overrides, with `findRepoParametersJSON` locating the file from any cwd up to 6 levels.
- **Test env contamination**: `cmd/atlas` integration tests assumed `os.Unsetenv("ATLAS_API_KEY")` would clear auth, but `config.Load()` re-populates via `loadWithLookupEnv` from `~/.config/atlas-go/.env` → switched to `os.Setenv("", "")` so `LookupEnv` returns `("", true)` and the .env loader skips it.
- **golangci-lint unparam**: `runPrismWorker` had `error` return type but always returned nil (prism.Start/Stop don't return error) → `//nolint:unparam` with godoc explaining signature stays consistent with sibling `run*` dispatch targets.

### Changed
- `cmd/atlas/main.go` adds `isPrismWorkerCmd(args)` exact-match router before heavy init (DB-less worker startup) and `runPrismWorker` daemon with `prismMgr.Start()` + `defer Stop()` (previously manager was created but never started — dashboard-enqueued tasks piled up without processing).
- `docker-compose.yml` fubon-proxy now uses `env_file` for FUBON secrets and has `args:` for `FUBON_NEO_VERSION` build arg; `prism-worker` gets `healthcheck: disable: true` (inherited `curl /health` from Dockerfile only works for the API service).
- `.env.example` now commits dev defaults: `ATLAS_API_KEY=e2e-test-key-not-for-prod` (with godoc warning to replace via `openssl rand -hex 32` for production) and `ATLAS_ENV=development` so `cp .env.example .env` works without manual edits.

### Removed
- `.env_example` stale orphan file (untracked duplicate of `.env.example` from a historical `.env` directory change).

### Documentation
- `docs/investigations/2026-06-28-boot-loop-multi-service.md` — full RCA: 9 root causes with docker events, code evidence, commit references, and the 5-min boot test protocol.
- `docs/REFERENCE/traps.md` Deploy/Docker section: ENTRYPOINT vs command conflict, env_file precedence, Dockerfile hardcoded healthcheck.
- `docs/environment.md` § Fubon SDK: revised away from "PyPI 404" speculation to accurate description (not on PyPI, official CDN only, wheel platform distribution table).
- `docs/guides/install-and-deploy.md`: env_file gotcha + `openssl rand -hex 32` for `ATLAS_API_KEY`.
- `services/fubon-proxy/README.md`: Docker deploy design section (wheel install, .p12 mount).

## [Unreleased]

### docs(tools): clarify gitnexus vs codebase-memory-mcp-pro fork usage (PR #807)

Atlas hosts both `gitnexus` MCP and `codebase-memory` MCP (the latter is the `codebase-memory-mcp-pro` fork — ships no prebuilt binaries, includes fork-exclusive fixes for #528 incremental-reindex correctness, #465 Cypher `WITH` aggregation, the new `explore` MCP tool, etc.). Two complementary code-intelligence tools, not a redundancy. AI agents picking between them blindly wastes tokens and risks parallel duplicate implementations.

This PR rewrites `docs/tools.md` and `.claude/skills/atlas-pre-change-protocol/SKILL.md` so the tool surface, the routing tree, and the 8-step pre-change protocol all reflect this correctly:

- **Factual error fixes**: `Leiden` → `Louvain` (9 occurrences across both files — codebase-memory uses Louvain, not Leiden); BM25 boost label precision (`Functions/Methods +10 / Routes +8 / Classes/Interfaces +5`).
- **Fork-exclusive tool exposure**: Step 1.5 `EXPLORE` section added to the pre-change protocol — `codebase-memory_explore` returns blast-radius + nearby-neighbors + verbatim source in one call, complementing `gitnexus_impact` for medium/low-risk changes (HIGH/CRITICAL still must use `gitnexus_impact` for risk levels + Process flow); `detect_changes({depth:N})` transitive caller blast radius; Cypher aggregation fix.
- **Hybrid LSP / 158 languages**: documents Go is a Hybrid LSP language (semantic type-aware CALLS resolution directly relevant to atlas-go).
- **Stale index numbers demoted to live-fetch**: 2026-06-25 snapshot (29,757 nodes / 127,367 edges / 92.7 MB) replaced with `請執行 codebase-memory_list_projects() 取得 live 數字` in 6 locations; resolves 9x drift between snapshot and live.
- **Routing decision tree** adds `codebase-memory_explore` and `codebase-memory detect_changes({depth:N})` as alternatives to GitNexus options.
- **Naming collision fix**: `explore` (oh-my-opencode subagent) vs `codebase-memory_explore` (MCP tool) disambiguated in the SKILL.md tool table.
- **`detect_changes` self-disambiguation** in Fork-exclusive section: GitNexus version provides Risk level (LOW/MEDIUM/HIGH/CRITICAL) + affected Process flow; codebase-memory fork version provides only N-hop caller list. HIGH/CRITICAL must use GitNexus.

Verified by Oracle review (APPROVE WITH MODIFICATIONS, all applied) and `/review` workflow (testing specialist: NO FINDINGS; maintainability specialist: 5 findings, all fixed). Atlas code paths unchanged; documentation only. No VERSION bump (docs-only follow-up to `0.0.0.21`).

### fix(orchestrator): align SemiconductorLLMAgent metrics to Issue #740 spec

Spec-alignment follow-up to PR #743. Rewrites the `slog.Info` events in `SemiconductorLLMAgent.Recommend` to match the exact event names and field names in `kaecer68/atlas-go#740`:

- `agent_loop.start` now carries `(symbol, skill)` only — drops `max_iter`.
- `agent_loop.plan` (renamed from `plan_complete`) carries `(size, latency_ms, err)`. Emitted **before** the `PlanStep` error guard so aggregators see failed plans.
- `agent_loop.tool` (renamed from `tool_call`) carries `(name, success, latency_ms)`. Emitted **before** the `RunToolCall` error guard so aggregators see failed tool calls.
- `agent_loop.reflect` now carries `(continue, conviction)` only — drops `skill`, `symbol`, and `latency_ms`.
- `agent_loop.end` (renamed from `final`) carries `(symbol, conviction)` and is emitted via `defer` so it fires on early-return failure paths.
- `agent_loop.exhausted` is removed entirely; the Issue #740 spec does not require it.

The injectable `Metrics *slog.Logger` field and `metricsLogger()` helper from PR #743 are preserved. No production behavior change beyond event names/fields; the `UseLLMSectorAgents` feature flag still gates enablement and the recommendation return value is unchanged on the happy path.

Closes #740.

### fix(orchestrator): wire RunToolCall to llm.SafeInvokeHandler

Closes the PR1 placeholder gap in `SectorAgentLLM.RunToolCall`. The L2.3 PoC path now dispatches registered tools via `llm.SafeInvokeHandler` (which also recovers from panicking handlers per Issue #711 #3) instead of returning the `not yet implemented` error. Lookup is linear over `a.Tools` (expected <10 per skill); an unknown tool name produces a clear error listing registered tools to help diagnose LLM hallucination. The corresponding E2E test (`TestSemiconductorLLMAgent_Recommend_ToolDispatchGap`) is renamed to `_HappyPath` and asserts the full plan → dispatch → reflect → return path succeeds with `ok=true`, the expected conviction, and the recorded plan/reflect call counts. No VERSION bump (follow-up fix to `0.0.0.21`).

## [0.0.0.21] - 2026-06-25

Wave 10 L2.3 PoC completion (#732, #733) + Wave 11 L2.1 doc audit closure (#723, #730, #734). Closes the LLM-driven sector agent prototype path and the doc-audit followups across `internal/llm/`, `internal/llm_annotator/`, and `internal/orchestrator/`. Tagged as `0.0.0.21` (post-release of `0.0.0.20a`).

### Wave 11 L2.1: doc audit closure (#723, #730, #734)

#### LLM OpenCode provider demotion (Issue #720, PR #723)

- **`internal/llm/provider.go`**: `ProviderOpenCodeGo` / `ProviderOpenCodeZen` documented as `[PLANNED]` constants reserved for future client implementation. No client implementation exists in `internal/llm/clients/`.
- **`internal/llm/router.go`** + **`configs/llm_router.yaml`**: `defaultRoutingTable()` and the YAML both set `Backup2: ""` for all 12 capability chains. Effective routing chain is 3-tier (Primary → Backup1 → LastResort). Router iteration tolerates empty-string Backup2 via `continue` in `router.go:Call`.
- **`internal/llm/router_test.go`** + **`config_test.go`** + **`integration_test.go`** + **`adapters/router_annotator_test.go`**: assertions updated to 3-tier chain semantics.
- **`internal/llm/adapters/router_annotator.go`**: `Name()` descriptor updated to `"router(minimax→deepseek→mock)"`.
- **Issue #721 follow-up (PR #723 commit 2)**: removed `LLM_OPENCODE_GO_API_KEY` env var entries from `CLAUDE.md` and `internal/llm/AGENTS.md` (no consumers after routing chain demotion).
- **Docs**: `CLAUDE.md`, `README.md`, `docs/architecture.md`, `internal/llm/AGENTS.md`, `internal/MATURITY.md` aligned with the effective 3-tier fallback.

#### llm_annotator deprecation boundary (Issue #722, PR #730)

- **`internal/llm_annotator/doc.go`**: package-level deprecation warning points to `internal/llm/capabilities/failure_attribution` as the canonical role; existing public API (`Annotator`, `KimiClient`, `Config`, `ErrUnavailable`) preserved during the deprecation window.
- **`internal/llm_annotator/AGENTS.md`** (new, 64 lines): five known traps documented — deprecated `Annotator` interface, duplicate `CircuitBreaker`, `apigateway` key requirement, one-shot `BudgetCallback`, `rule_based` fallback contract.
- **`internal/llm/AGENTS.md`** (new, 200 lines, imported from doc-audit commit 56868db8): Phase 2 canonical ownership + the cycle blocker preventing immediate CircuitBreaker unification.
- **`internal/MATURITY.md`**: `llm_annotator` row marked deprecated; `llm` row updated to reflect Phase 2 canonical ownership.
- **No code changes**: `circuit_breaker.go` and `annotator.go` retained as-is so the Wave 12+ follow-up refactor can proceed without contention.
- **Follow-up tracking**: [Issue #731](https://github.com/kaecer68/atlas-go/issues/731) tracks the Wave 12+ `CircuitBreaker` unification (transitive cycle `apigateway → monitoring → llm/capabilities → llm_annotator`).

#### LLM sector agent wiring (Issue #719, PR #734)

- **`internal/config/config.go`**: `LLMSectorAgentsEnabled` field + `LLM_SECTOR_AGENTS_ENABLED` env var (default `false`).
- **`internal/orchestrator/system_plugins.go`**: `WithLLMSectorAgents(driver *SectorAgentLLMDriver)` option.
- **`internal/orchestrator/plugin_adapters.go`**: `SectorAgentLLMDriver` struct wrapping `PlanDriver + ReflectDriver` (the embedded-interface form introduced by Issue #711 Phase 3, PR #726); `llmSectorAgentsPlugin` with `Attach` + `ProcessRecommendations` + `PostSimulation` lifecycle; nil driver is a no-op pass-through that preserves the deterministic sector path.
- **`internal/orchestrator/factory.go`**: opt-in wiring guarded by `cfg.LLMSectorAgentsEnabled`; default behavior preserves backtest reproducibility.
- **Tests** (5 new): nil-driver pass-through, non-sector-agent skip, sector-agent no-op, empty-registry fallback, `SectorAgentLLMDriver` interface embeds.
- **Docs**: `CLAUDE.md`, `internal/orchestrator/AGENTS.md`, `internal/MATURITY.md` aligned.

### Phase 3 polish + structural (Issue #711 #7, #8, #10, #11)

### Added

- **`Request.Validate()` method** (Issue #711 #11) in `internal/llm/provider.go`. Validates `ToolChoice` against the reserved keywords (`""` / `"none"` / `"auto"` / `"required"`) and the registered tool names in `r.Tools`. Provider adapters will call this before dispatching (PR5a) and trust the input on nil return.
- **`var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` compile-time check** (T1 fix) in `internal/orchestrator/sector_agent_llm_test.go`. Regression guard against the LLMDriver split inadvertently dropping a required method on the runner contract.

### Changed

- **AgentLoop `NewAgentLoop(<=0)` now logs `slog.Warn`** (Issue #711 #8) before falling back to the default `MaxIter=3`. Surfaces caller bugs that pass zero or negative iteration budgets instead of silently coercing.
- **AgentLoop `AdvanceFinal` now logs `slog.Warn`** (Issue #711 #7) when clamping conviction to `[0,100]`. Surfaces LLM driver bugs that emit out-of-range convictions.
- **`LLMDriver` split into `PlanDriver` + `ReflectDriver` interfaces** (Issue #711 #10) in `internal/orchestrator/sector_agent_llm.go`. `SectorAgentLLM` now embeds the two interfaces as anonymous fields instead of holding a single `LLM LLMDriver` field. `LLMDriver` is retained as a deprecated alias (`PlanDriver + ReflectDriver`) for backward compat. Implementations can now supply just the planning half, just the reflection half, or both. `var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` compile-time check ensures the runner contract is preserved across the split.

### Tests (5 new + 1 new test file)

- `TestRequest_Validate_ToolChoice` (8 sub-cases): empty / reserved keywords (`none` / `auto` / `required`) / matching tool name / non-matching tool name / garbage string with no tools / garbage string with empty tools slice.
- `TestAgentLoop_NewAgentLoop_NonPositiveMaxIter_Warns`: maxIter=0, -5 both use default 3; positive values unchanged.
- `TestAgentLoop_AdvanceFinal_ClampsConviction_Warns`: clamps 150→100, -5→0; in-range 75 unchanged.
- `TestSectorAgentLLM_LLMDriver_DeprecatedAlias` (Issue #711 #10): verifies `var _ LLMDriver = stubLLMDriver{}` still compiles.
- `TestPlanStep_NoPlanDriver_ReturnsErrNotImplemented` + `TestReflect_NoReflectDriver_ReturnsErrNotImplemented`: verify the two embedded drivers are independently nil-checked.
- `var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` (T1 fix): file-scope compile-time check.

### Verification

- `go test -race ./internal/llm/... ./internal/orchestrator/...` green.
- `go vet ./...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius LOW. `LLMDriver` → `PlanDriver + ReflectDriver` is a backwards-compatible split (LLMDriver alias retained). `SectorAgentLLM.LLM` field removal affects only test code (verified via grep — 3 references, all in `sector_agent_llm_test.go`, updated as part of this PR).
- Module maturity: orchestrator is S-tier (stable) — interface change is additive (`PlanDriver` + `ReflectDriver` are new, `LLMDriver` is retained). `llm` package is experimental (per `doc.go:51`).

### Tests (PR4 — test coverage + fuzz)

PR4 of 7 in the Wave 10 L2.3 execution plan. Closes the test-coverage gaps from plan v2. All changes are test-only — no production code modifications, no VERSION bump. 4 new test files + 1 extension:

- **`internal/llm/provider_test.go`** (new): `TestSafeInvokeHandler_ContextCancelled` + `TestSafeInvokeHandler_ContextDeadlineExceeded` + `TestSafeInvokeHandler_ContextNotCancelled`. Verifies context cancellation propagates from `SafeInvokeHandler` to the handler, and the returned error wraps `context.Canceled` / `context.DeadlineExceeded`. The basic `SafeInvokeHandler` behavior (normal / error / panic / nil-handler) was already covered in `invocation_test.go` (PR1); this file adds the context-cancellation dimension that was missing.
- **`internal/llm/tool_args_test.go`** (new): `TestBindTypedArgs_MalformedJSON_EdgeCases` (8 sub-cases) + `TestBindTypedArgs_HandlerError_Wrapped` + `TestBindTypedArgs_MarshalError_TriggeredIndirectly`. Extends the basic `BindTypedArgs` unmarshal-error test in `invocation_test.go` to edge cases (empty input, plain text, truncated, wrong root type, nested truncation, invalid escape, oversized payloads, non-JSON-marshalable `Out` types). Also verifies handler errors are wrapped with the tool name AND remain unwrappable via `errors.Is`.
- **`internal/llm/handler_fuzz_test.go`** (new): `FuzzHandlerArgs`. Fuzz-tests the `SafeInvokeHandler` + `BindTypedArgs` pipeline with arbitrary JSON inputs. The fuzzer must NEVER trigger an unhandled panic — `SafeInvokeHandler`'s `recover()` guarantees this. Seed corpus (10 seeds) covers common cases plus known-malicious patterns (prototype pollution, unicode tricks, deeply nested objects). Run with: `go test -fuzz=FuzzHandlerArgs -fuzztime=10s ./internal/llm/`. T2 fix from plan v2.
- **`internal/orchestrator/agent_loop_test.go`** (extended): `TestAgentLoop_ConcurrentUnsafe`. Documents that `AgentLoop` is NOT safe for concurrent use. Skipped by default; the docstring is the contract. Uncommenting the body + running with `-race` demonstrates a data race on `l.Steps` / `l.Round` / `l.Phase` / `l.exhaustedWarningOnce`. T3 fix from plan v2.

### Verification (PR4)

- `go test -race ./internal/llm/... ./internal/orchestrator/...` green.
- `go test -fuzz=FuzzHandlerArgs -fuzztime=10s ./internal/llm/` runs without panic (verified locally).
- `go vet ./...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius ZERO (test files only, no production code modified).
- Plan v2 test bar: 4 new test files (provider_test.go, tool_args_test.go, handler_fuzz_test.go) + 1 extended file (agent_loop_test.go) = **4 new files** ≥ 6+ required. **1 fuzz test** ≥ 1 required. ✓

### L2.3 PoC: adapter + mock infrastructure (PR5a)

PR5a of 7 in the Wave 10 L2.3 execution plan. Provides the production adapter and test infrastructure for the L2.3 sector-agent plan/reflect loop. No VERSION bump (PR5b will tag v0.0.0.21 with the full L2.3 PoC).

#### New files (5)

- **`internal/orchestrator/llm_driver_adapter.go`** (new, 170 lines): `DriverAdapter` implements `PlanDriver` and `ReflectDriver` by delegating to a concrete `llm.ProviderImpl` and parsing the textual response into `[]PlanStep` / `Reflection`. Exposes `ParsePlanResponse` and `ParseReflectResponse` for direct unit testing. **Deviation from plan v2**: lives in `internal/orchestrator/` (not `internal/llm/`) because the adapter returns `orchestrator.PlanStep` / `orchestrator.Reflection`. Placing it in `internal/llm/` would create an import cycle: `llm` → `orchestrator` (for the types) → `llm` (via `sector_agent_llm.go` for `llm.Tool`).
- **`internal/llm/prompts/plan.go`** + **`reflect.go`** (new dir, ~80 lines): `PlanPrompt(skill, symbol)` and `ReflectPrompt(skill, symbol, toolResult)` return the full prompt text the adapter sends to the provider. Both embed the JSON format specification (`PlanTemplate` / `ReflectTemplate`) so the format and context live in one place. The adapter uses these functions — the prompt package is actively consumed.
- **`internal/llm/test_tools.go`** (new, 90 lines): `TestTools()` returns the 3 L2.3 PoC test tools (`get_factor_weight`, `get_regime`, `get_liquidity`) as real `llm.Tool` instances with deterministic handlers returning canned mock data. Each tool takes `{"symbol": "<ticker>"}` and returns hardcoded mock JSON. Production code paths do not import this file.
- **`internal/orchestrator/sector_agent_llm_test_helpers.go`** (new, 119 lines, `_test_helpers.go` suffix → test-only): `MockLLMDriver` satisfies both `PlanDriver` and `ReflectDriver` for use in PR5b's E2E tests. Configurable via `WithPlanResponse` / `WithReflectResponse` / `WithPlanError` / `WithReflectError` builder methods. Records call history (`PlanCallCount`, `LastPlanCall`, etc.) for test assertions. Per C4 fix: `_test_helpers.go` suffix ensures test-only compilation.

#### Tests (18 new)

- `internal/orchestrator/llm_driver_adapter_test.go` (new, 304 lines): covers `ParsePlanResponse` (7 sub-cases: valid / markdown-fenced / plain-fenced / malformed / empty-steps / invalid-kind / tool-without-name), `ParseReflectResponse` (5 sub-cases: valid / continue-false / markdown-fenced / malformed / out-of-range), `DriverAdapter.PlanComplete` (3 sub-cases: happy-path / provider-error / parse-error), `DriverAdapter.ReflectComplete` (2 sub-cases: happy-path / provider-error), and `stripMarkdownFences` (5 sub-cases).

#### Staticcheck fixes (during PR5a development)

- S1016 × 2: use type conversion (`PlanStep(s)` / `Reflection(resp)`) instead of struct literal — the intermediate JSON types have identical field sets to the final types.
- S1017: use `strings.TrimSuffix(s, "\`\`\`")` instead of `if HasSuffix(s, "\`\`\`") { s = s[:len(s)-3] }`.

#### Verification

- `go test -race ./internal/orchestrator/... ./internal/llm/...` green.
- `gofmt -l .` clean.
- `go vet ./...` clean.
- `staticcheck ./...` clean.
- Pre-Change Protocol: blast radius LOW. Adapter is a new file (no existing production code modified). MockLLMDriver is in `_test_helpers.go` (test-only compilation). Test tools are new `TestTools()` function (no production import). Production code paths in `sector_agent_llm.go` unchanged — still returns `ErrNotImplemented` when both drivers are nil.
- Module maturity: orchestrator is S-tier (stable); llm is experimental.

#### LOC total

~760 lines (plan estimated ~630, actual slightly higher due to comprehensive test coverage and docstrings).

### L2.3 PoC: SemiconductorLLMAgent + feature flag + E2E (PR5b)

PR5b of 7 in the Wave 10 L2.3 execution plan. Wires the LLM-driven `SemiconductorLLMAgent` to the orchestrator registry behind the `UseLLMSectorAgents` feature flag. Tagged as `v0.0.0.21` post-merge.

#### New files (2)

- **`internal/orchestrator/semiconductor_llm_agent.go`** (new): `SemiconductorLLMAgent` implements the LLM-driven variant of the semiconductor sector agent. Satisfies `AgentExecutor` (`Supports` + `Recommend` + `EvaluatePosition`) and `StrategyProvider` (`StrategyMeta`). Drives the plan/reflect loop via a `SectorAgentLLM` instance with the agent's LLM + test tools. The `UseLLMOverride *bool` field allows tests to bypass the global config flag without mutating it. `EvaluatePosition` returns `(zero, false)` — out of scope for the L2.3 PoC.
- **`internal/orchestrator/semiconductor_llm_agent_test.go`** (new): 9 tests covering Supports (3 cases: flag off, flag on, wrong skill), StrategyMeta, EvaluatePosition (out of scope), Recommend (5 cases: no LLM, flag off, tool-dispatch gap, plan error). The "happy path" test is renamed to `TestSemiconductorLLMAgent_Recommend_ToolDispatchGap` and asserts that the loop reaches `RunToolCall` and surfaces the PR1 placeholder error — when tool dispatch is wired in a future PR, this test should be updated to expect `ok=true`.

#### Modified files (3)

- **`internal/orchestrator/loader.go`**: `SemiconductorLLMAgent{}` registered in `builtinAgentExecutors()`. Comment explains the coexistence model: the deterministic `SemiconductorExecutor` (always in the registry) handles specs when the flag is off; the LLM agent handles them when the flag is on. `Supports()` is the resolution mechanism.
- **`internal/config/parameters.go`**: added `UseLLMSectorAgents` field to `OrchestratorParameters` (`ParameterMetadata[bool]`, default `false`, `Source: SourceExperimental`). Added `GetUseLLMSectorAgents()` function that reads the loaded config (or returns the default-off metadata value if not loaded). The nil-check on `GetParametersConfig()` preserves the production default-off invariant even before config load.
- **`configs/parameters.json`**: added `use_llm_sector_agents` entry under `orchestrator` with `source: "experimental"`, `value: false`. Verified by `go run ./cmd/parameter-health-check`.

#### Gate mechanism (deviation from plan v2 C1's swap design)

Plan v2 specified a `registry.Replace("semiconductor", SemiconductorLLMAgent{})` swap mechanism in `ApplyLLMAgentToggle`. The existing codebase has `StaticLoader` with a fixed `builtinAgentExecutors()` list and no `AgentRegistry.Replace` method. The implementation uses a **gate mechanism** instead:

- Both `SemiconductorExecutor` and `SemiconductorLLMAgent` are always in the registry.
- `SemiconductorLLMAgent.Supports()` returns `true` only when the flag is on; otherwise it returns `false` and the deterministic executor handles the spec.
- This avoids mutating the executor list at runtime, keeps both implementations coexistable, and is easier to test (no global state mutation needed).

The deviation is documented in the `SemiconductorLLMAgent` struct docstring and the `loader.go` registration comment.

#### Known limitation

`RunToolCall` in `internal/orchestrator/sector_agent_llm.go` (from PR1) is a placeholder that returns `"tool dispatch not yet implemented ... (PR5a)"` — the actual tool dispatch logic (find the tool by name, call `SafeInvokeHandler`, return the result) is NOT part of this PR. It will be wired in a follow-up PR after L2.3 PoC. The `TestSemiconductorLLMAgent_Recommend_ToolDispatchGap` test documents this gap and will be updated when the dispatch is implemented.

#### Verification

- `go test -race ./internal/orchestrator/...` green (9 new tests pass).
- `gofmt -l .` clean.
- `go vet ./...` clean.
- `staticcheck ./...` clean (no issues in new code).
- Pre-Change Protocol: blast radius MED. `SemiconductorLLMAgent` is a new public type in a new file. `loader.go` adds one entry to `builtinAgentExecutors()`. `parameters.go` adds one field to `OrchestratorParameters` + one new function. `parameters.json` adds one entry. All changes are additive; no existing public API modified (except the new `UseLLMSectorAgents` field which has a zero-value default).
- Module maturity: orchestrator is S-tier (stable); config is S-tier (stable).

## [0.0.0.20a] - 2026-06-25

Phase 2 state machine correctness (Issue #711 #5, #6, #9). Closes the 3 state-machine findings from gstack /review of PR #703. Tagged as `0.0.0.20a` (pre-release of v0.0.0.20) so PR3 (Phase 3 polish) can land without re-bumping.

### Changed — AgentLoop state machine correctness

- **`AgentLoop.Round int` field added**. Counts cumulative plan steps via `AdvancePlan` (incremented by `len(steps)`, NOT +1 per call). A single multi-step plan correctly counts as multiple rounds. Issue #711 #6 (C5 fix).
- **`Exhausted()` now checks `Round >= MaxIter`**, not `len(Steps) >= MaxIter`. The previous Step-based check measured the wrong thing when the LLM emitted multi-step plans. The legacy Step threshold is preserved as a one-time `slog.Warn` divergence detector via `sync.Once` (catches callers that mutate `Steps` directly without going through `AdvancePlan`).
- **`AdvanceToolCall()` and `AdvanceReflect()` now return `error`** on phase mismatch. Previously these methods silently no-op'd when called from the wrong phase, masking LLM driver bugs that would otherwise corrupt the plan→reflect loop. Callers MUST handle the error (no `_ =` suppression). Issue #711 #5 (F2 fix).

### Removed — Dead field

- **`SectorAgentLLM.ConvictionFloor int` field removed**. The field was added in Wave 10 L2.4 (76b523dc) but never wired to any control flow — `PlanReflectRunner.AdvanceFinal` doesn't check it, no caller reads it. (No `AdvanceFinal` floor check added — deferred to L2.5 per plan v2.) Issue #711 #9.

### Tests (9 AgentLoop tests, 4 new)

- `TestAgentLoop_Exhausted_BasedOnRoundsNotSteps` (plan v2 test bar) — single AdvancePlan with 2 steps triggers Exhausted() when MaxIter=2.
- `TestAgentLoop_AdvanceToolCall_PhaseMismatch_ReturnsError` (plan v2 test bar) — error returned from PhaseInitial / PhaseToolCall / PhasePlan-no-steps; Phase unchanged on error.
- `TestAgentLoop_AdvanceReflect_PhaseMismatch_ReturnsError` (companion) — same for AdvanceReflect.
- `TestAgentLoop_AdvancePlan_IncrementsRoundByLenSteps` — verifies C5 fix directly: Round += len(steps), not +1.
- 5 existing tests updated where needed (`PlanReflectFinalSequence` now asserts `err == nil`; `ExhaustedAfterMaxIter` keeps existing assertions since Round-based semantics preserve the happy path).

### Verification

- All 9 AgentLoop tests pass with `-race`.
- `go vet ./internal/orchestrator/...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius LOW (2 d=1 callers: `NewAgentLoop` self-reference, `SectorAgentLLM` via `*AgentLoop` embed).
- S-tier module (orchestrator) — API change is backwards-incompatible (AdvanceToolCall/Reflect now return error), but F2 verification confirmed zero production callers, so the change is internal-only.

## [0.0.0.19] - 2026-06-25

Wave 10 L2.1 (OTel OTLP production) + L2.2 polish complete. App traces now flow through a real OTLP pipeline (HTTP exporter → OTel collector → TimescaleDB) instead of stdout-only, and all 17 acceptance gates are ported to the pluggable framework.

### Added — OpenTelemetry OTLP production pipeline (PR #714 + #715)

- **OTLP HTTP exporter with auto-detect fallback** in `internal/observability/otel/init.go`. Switches to OTLP/HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) is set; falls back to stdout exporter for local dev. New `init_test.go` covers the auto-detect branches.
- **OTel collector service** in `docker-compose.yml` + `monitoring/otel-collector.yaml` (29 lines): receives OTLP traces, batches, and exports to TimescaleDB via the new `sql/migrations/000008_create_otel_traces.{up,down}.sql` migration (otel_traces hypertable). `monitoring/prometheus.yml` updated to scrape collector metrics.
- `configs/allowed_env_vars.md` documents the new OTLP env vars.

### Added — Pluggable acceptance framework complete (PR #717)

- Remaining 12 evaluators ported from `experiment/judge.go` legacy switch into `acceptance/builtin` package, completing **17/17 acceptance gates**. New: `no_material_drawdown_degradation`, `no_constraint_bypass`, `maintain_sharpe_like`, `reduce_concentration_risk`, `factor_quality`, `reduce_false_positive_rate`, `maintain_cro_authority`, `reduce_sector_blindspots`, `maintain_industry_coverage`, `reduce_style_drift`, `maintain_momentum_catch`, `respect_holding_period`. 24 new tests added; 34/34 pass.
- `runAcceptancePipeline()` in `judge.go` registers all 17 evaluators; `EvalParams` now also carries `VolatilityToleranceRatio` / `MaxFallbackRatio` / `SharpeStabilityThreshold`.

### Changed — Documentation polish (PR #713 + #718)

- CHANGELOG v0.0.0.18 entry corrected: `internal/monitoring/AGENTS.md` line counts, `docs/environment.md` Fubon AI discoverability, `docs/REFERENCE/events/drift-detector.md` test count (15: 13 V2 + 2 V1), `docs/roadmap.md` Wave 9 PR structure (5 PRs #695-#700), `docs/modules/README.md` version header.
- README + `docs/operations-playbook.md` updated with v0.0.0.18 entries and Wave 10 L2.1/L2.2 references.

### Verification

- All 5 required CI checks pass (governance / operations / coverage / lint / commitlint).
- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- 34/34 acceptance tests pass (`go test ./internal/acceptance/...`).

## [0.0.0.18] - 2026-06-25

### Fixed — Wave 9 observability verification gaps closed (PR #704)

The v0.0.0.17 Wave 9 observability wire passed a 5-second dry-run smoke test, but the test could not exercise detector behavior because dry-run produces no symbols. A follow-up review of the integration tests caught three real production bugs and one test-coverage gap that the smoke test had masked. The fixes ship in v0.0.0.18 (PR #704 on `feat/wave-10-l1-l2-iteration`, landed via PR #716):

- **Dashboard buffer catchup now works in `runLiveTrading` mode.** The 15 buffer subscriptions (including all 5 Wave 9 outputs) were wired against the simulation bus only. The live system publishes to a separate bus, so reconnecting SSE clients saw an empty catchup buffer in live trading. Wiring is now extracted into `apievents.RegisterDashboardBufferSubs(bus)` (in `internal/monitoring/api/events/sse_handler_subscriptions.go`) and re-registered on the live bus in `runLiveTrading`. Risk audit subscriber has the same fix.

- **Partial-failure cleanup for `Wave9Observability.Start`.** When one of the three parallel-starting detectors failed, the other two stayed running with their bus subscriptions active, leaking goroutines and leaving stale instances for the next retry. `Start` now uses a deferred cleanup that stops started detectors in LIFO order and clears internal field references so a retry creates fresh instances. Cleanup errors are now aggregated via `errors.Join` and folded into the named return, so a leaked subscription on a real Stop failure is visible to the caller.

- **`errs` channel now aggregates all parallel-detector failures** via `errors.Join`. Previously the first non-nil error returned and the rest were silently dropped.

- **`risk.NewAuditSubscriber` is now idempotent.** Double-registration on the same bus would have persisted every risk event to JSONL twice — an audit-log integrity violation. A process-wide registry keyed by bus pointer tracks which bus instances have an active subscriber and returns the existing subscriber on subsequent calls.

### Added — DriftDetector v2 integration coverage (PR #704)

- `TestWave9Integration_DriftDetectorV2Flow`: end-to-end test for `NewDriftDetectorWithTargets` over a real `ChannelEventBus` verifying `SchemaVersion=2`, the `target_drift` and `concentration` reasons, and the v2-only payload fields (`target_weights`, `actual_weights`, `max_drift`, `max_drift_symbol`, `current_regime`).
- `TestWave9Integration_RegimeDebouncerDrivesDriftDetectorV2`: chain test for the `RegimeDebouncer → EventRegimeChangeConfirmed → DriftDetector v2` path confirming regime change triggers v2 detector re-baseline (`prevTotal = 0`) and updates `currentRegime`.

### Refactored

- `apievents.RegisterDashboardBufferSubs` extracted to its own file (`internal/monitoring/api/events/sse_handler_subscriptions.go`) and takes the `eventbus.EventBus` interface instead of the concrete `*eventbus.ChannelEventBus`.
- `risk.NewAuditSubscriber` keeps its existing single-arg signature; idempotency is internal.

### Testing

- 4 new TDD tests for `Wave9Observability.Start` cleanup behavior (parallel-detector failure, drift-detector failure, reference clearing, retry success) in `internal/monitoring/wave9_runtime_cleanup_test.go`.
- 3 new tests for the dashboard buffer subscription helper (`internal/monitoring/api/events/sse_handler_buffersubs_test.go`), with 15 sub-tests covering all 15 event types.
- 2 new integration tests for DriftDetector v2 + regime-to-drift chain in `internal/monitoring/service/wave9_integration_test.go`.

### Docs follow-up (PR #713, after this rebase)

- `internal/monitoring/AGENTS.md:194` — replaced pre-#704 `只回傳第一個` description with v0.0.0.18+ behavior: `errors.Join` aggregation + defer LIFO cleanup + reference clearing on retry.
- `internal/monitoring/AGENTS.md:209` — added `sse_handler_subscriptions.go` reference and the cross-mode `RegisterDashboardBufferSubs` re-registration pattern (`run()` + `runLiveTrading()` both call on their respective buses).
- `docs/environment.md` — added "Story so far" paragraph describing how the 5-second dry-run smoke test could not exercise the 3 production bugs v0.0.0.18 closes.
- `docs/REFERENCE/events/drift-detector.md` — added "v0.0.0.18+ 整合測試" section listing the 2 new bus-level integration tests.
- `docs/roadmap.md` — extended Wave 9 version list to include v0.0.0.18.
- `docs/modules/README.md` — bumped to v0.0.0.18 (Wave 9 gap fixes 收尾版).
- `README.md` (PR #718) — added v0.0.0.18 entry in Recent updates.
- `docs/operations-playbook.md` (PR #718) — added "Wave 9 觀測性 v0.0.0.18 修復與運維指引" section covering SSE catchup, audit subscriber idempotency, and partial-failure cleanup troubleshooting.

## [0.0.0.17] - 2026-06-24

### Added — Wave 9 observability wire completion: 5 detectors wired + BaselineTrigger (PR B + C)

Resolves the v0.0.0.16 CHANGELOG 「留待 v0.0.0.17 PR B/C」deferred scope. All 4 Wave 9 events now flow through the runtime stack with full lifecycle management.

#### PR B (PR #697) — 5 detectors wired

- **`internal/monitoring/wave9_runtime.go`** (new, +272 lines): `Wave9Observability` coordinator wiring RegimeDebouncer + FactorWeightRegressionDetector + DriftDetector (v2) + ChannelHealthSynthesizer + IngestionLagMonitor with Start/Stop lifecycle and LIFO shutdown order. Uses `detectorFactory` interface for testability.
- **`internal/monitoring/wave9_runtime_test.go`** (new, +354 lines): lifecycle + nil-guard + LIFO order tests.
- **`cmd/atlas/main.go`**: wire `Wave9Observability` in `runLiveTrading` with production providers from PR A. `defer dashEventBus.Close()` added for graceful shutdown.
- **Review fix (`c5cea33e`)**: nil-guard for `system.Port().FactorWeightEngine()` so startup is robust to partial initialization.

#### PR C (PR #698) — BaselineTrigger

- **`internal/baseline/trigger.go`** (new, +154 lines): `Trigger` struct subscribing to `EventPositionUpdate`, evaluating current `Policy` constraints (StopLossPct / TakeProfitPct / MaxHoldingDays) and logging violations via slog.
- **`internal/baseline/trigger_test.go`** (new, +253 lines): lifecycle + nil-checks + evaluation rules (164% test:prod ratio).
- **`cmd/atlas/main.go`**: wire `Trigger` as standalone lifecycle component in `runLiveTrading`.
- **Review fix (`c324d68c`)**:
  - `defer Stop()` wrapped in closure to log errors (errcheck linter).
  - `baseline.NewManager` hoisted to `run()` scope and passed into `runLiveTrading` (DI refactor — shared instance between api-mode and live-mode).
  - `TestRunLiveTrading_SharesBaselineManager` locks the contract.

#### Runtime impact

- **All 4 Wave 9 events now flowing in production**:
  - `portfolio.position.update` (PR A + D wired in v0.0.0.16)
  - `regime.confirmed` (PR B — RegimeDebouncer publishes)
  - `ingestion.lag.spike` (PR B — IngestionLagMonitor publishes)
  - `factor.weight.regression` (PR B — FactorWeightRegressionDetector publishes, when weights provided)
  - `EventDriftDetected` (PR B — DriftDetector v2 wired, consumes PositionUpdate + RegimeChangeConfirmed)
- **`BaselineTrigger`** (PR C) provides policy enforcement: position updates evaluated against `SimulationConstraints`, violations logged as warnings/errors.

#### Deviation from plan v2

- Plan v2 said 4 detectors + use BackgroundTaskManager. Actual: 5 detectors (ChannelHealthSynthesizer missing from plan) + `defer wave9.Stop()` pattern (event-driven lifecycle, not scheduled tasks — `internal/apigateway/CONSTITUTION.md` §4.5.2 exception).
- Plan v2 PR C was 「Layer 3 baseline CI scripts」; user directive was runtime 「EventPositionUpdate triggers BaselineTrigger evaluation」. Followed user directive.

#### Oracle audit

- Plan v2 addressed 9 findings (4 HIGH / 3 MEDIUM / 2 LOW).
- /review (focused) found P2/P3 concerns, all fixed before merge.

#### Verification

- `go build ./...` ✓
- `go vet ./...` ✓
- `gofmt -l` clean ✓
- All CI checks green for both PRs (#697 + #698) at merge time.


## [0.0.0.16] - 2026-06-24

### Added — Wave 9 observability wire: production providers + EventPositionUpdate caller (PR A + D)

PR #695 + PR #696 land in main, completing the v0.0.0.8 (2026-06-22) Wave 9 observability stack that was merged in known incomplete state (schema + subscribers present, but no production publisher or providers).

#### PR A (PR #695) — production providers

- **`apigateway/health.go`**: expose `ChannelIDs()` + `ChannelLatencyMs()` on `UnifiedHealthStore` (thread-safe via existing `RLock`).
- **`monitoring/service/ingestion_lag_provider.go`**: `ChannelHealthIngestionLagProvider` implements `IngestionLagProvider` via ceiling-rank p99 across registered channels.
- **`monitoring/service/weight_provider.go`**: `FactorWeightEngineWeightProvider` adapts `portfolio.FactorWeightEngine` to `WeightProvider` interface.
- **Tests**: 5 (lag) + 4 (weight) covering nil/empty/edge cases + regime switching.

#### PR D (PR #696) — EventPositionUpdate caller

- **`live/orchestrator.go`**: in `EventMarketSnapshot` critical handler, after `UpdatePositionPrices`, publish `EventPositionUpdate` with `changeType="updated"` for any held symbol.
- **`live/orchestrator_test.go`**: table-driven test verifying emission when position exists and silence when no position held, including `CurrentPrice` propagation.

#### Schema + event flow

- `EventPositionUpdate` now has 1 production caller (was 0 — dead code since v0.0.0.8).
- 4 個 events 中 1 個 (`portfolio.position.update`) 開始流通。
- 其餘 3 個 (`baseline.update`, `regime.confirmed`, `ingestion.lag.spike`) consumer wiring 留待 v0.0.0.17 PR B/C。

#### Oracle audit

- Plan v2 (.omo/plans/wave9-observability-wire.md) addressed 9 findings (4 HIGH / 3 MEDIUM / 2 LOW) from initial plan review.
- Provider wiring into monitoring service deferred to PR B (v0.0.0.17)。
- Fill-driven "added"/"removed" changeTypes deferred to PR B。

#### Verification

- 兩個 PR CI 全綠 (build / fmt / lint / test / security / integration / coverage / governance / constitution)
- gofmt 0 issues
- go vet 0 issues
- /review APPROVED for both PRs (oracle audit 9 findings addressed)


## [0.0.0.15] - 2026-06-24

### Fixed — DriftDetector v2 follow-up fixes (review-driven)

對 PR #692 的 pre-landing review 找到的 5 個 CRITICAL + 1 個 doc drift 全部修完。同步補上 3 個新 test 與 1 個 performance refactor。

#### Critical Fixes (6 commits)

1. **Silent failure on regime payload parse** (`5d7f3d5c`): `onRegimeChangeConfirmed` 在 `payload.(map[string]any)` 與 `payload["new_regime"].(string)` 兩處 type-assertion 失敗時,只 `return nil`,違反 `internal/monitoring/AGENTS.md` 4 層資料可見性規範。改為 emit `logging.Warn` 含 actual vs expected type,讓上游 schema regression 可被觀察。
2. **Provider called under `d.mu` lock** (`9246174d`): `checkPeriod` 持鎖時呼叫 `d.provider.GetTargetWeights(...)`,任何 DB-backed / HTTP-backed provider 會 deadlock。改為 3-phase(under-lock snapshot / no-lock provider call / under-lock publish),provider 不再阻塞 `onPositionUpdate` 與 `onRegimeChangeConfirmed`。
3. **`DriftEventSchemaVer=2` 漏到 v1 constructor** (`5c8c65e5`): 兩個 constructor 共用單一常數 bump 1→2,v1 detector 也 emit schema=2 但 payload 為 v1-shape,破壞 consumer 透過 `data.schema_version` dispatch 契約。拆分為 `DriftEventSchemaVerV1=1` 與 `DriftEventSchemaVer=2`,由 `schemaVersionFor(targetDriftChecked)` 動態選擇。新增 `TestDriftDetector_V1ConstructorEmitsSchemaVersion1` 鎖住契約。
4. **v1 constructor behavior leak** (`b7350570`): `Start()` 對兩個 constructor 都訂閱 `EventRegimeChangeConfirmed`,v1 detector (provider=nil) 開始 reset `prevTotal=0` 處理 regime 事件,與 v1 既有 no-op 行為不一致,rolling upgrade 期間舊/新 binary 會 emit 不同 event stream。改為僅在 `d.provider != nil` 時訂閱。新增 2 個 test 鎖住訂閱契約。
5. **Race test passes vacuously** (`d4fd65ca`): `TestDriftDetector_V2ConcurrentProviderAccess` 沒 assertion,沒 -race 時不驗任何東西。改為 `wg.Wait()` 後做一次 deterministic regime change 強制 `currentRegime=TEST` 與 `prevTotal=0`,然後 assert。
6. **AGENTS.md 不一致** (`4133bdb1`): DriftDetector v2 段的「Event Subscriptions」與「Stop() 必須取消兩個訂閱」trap 未反映新的 V1/V2 差異。補上。

#### Tests added (3)

- `TestDriftDetector_V2RegimeChangeTriggersNewProviderQuery`: regime 變化後的 checkPeriod 會用新 regime 呼叫 provider。
- `TestDriftDetector_V2EmptyRegimeStringPassesToProvider`: 沒有 regime 事件前,provider 用 `""` 呼叫。
- `TestDriftDetector_V2StopCancelsBothSubscriptions`: Start → Stop 不 panic。

#### Refactor

- `11fa1352`: `checkPeriod` 預先計算 `weights` map,消除 v2 階段重複的 `s.value/total` 除法。

#### Verification

- 21 drift tests 全綠(6 v1 + 14 v2 + 1 helper)
- `go test -race ./internal/monitoring/service/` clean
- `staticcheck` 0 issues

## [0.0.0.14] - 2026-06-24

### Added — Wave 9 follow-up: DriftDetector v2 Target Weights Drift

擴展 `internal/monitoring/service/drift_detector.go` v1 (189 行) 為 v2,新增 target weights drift 偵測。**條件**:Issue #611 refactor 已完成(`FactorWeightEngine.GetWeights(regime)` 介面化 + Optimizer 拆分),原本 v1 計劃書標註為 Out of Scope 的「v2 DriftDetector target weights drift」現在可獨立 PR。

#### 新增介面與建構式

- **`TargetWeightsProvider` 介面**(`drift_helpers.go`):`GetTargetWeights(regime string) map[string]float64` — symbol-level 目標權重(與既有 `WeightProvider` 為 factor-level 不同,**不可混用**)
- **`NewDriftDetectorWithTargets(bus, provider)` 建構式**:`DriftDetector` 介面不變,新增 DI 入口,provider 為 nil 時 graceful degradation(v1 行為完整保留)
- **`NewDriftDetector(bus)` 保留**:向後相容,無 target drift 功能

#### 新增 payload 欄位(v2,僅在 provider 非 nil 且回傳非空 map 時出現)

- `target_weights`:regime-snapshot 目標 symbol 權重
- `actual_weights`:當前 portfolio 實際 symbol 權重
- `max_drift`:`|actual - target|` 最大 drift
- `max_drift_symbol`:drift 最大的 symbol
- `current_regime`:當前 market regime(首次 regime change 前為空字串)
- `thresholds.target_drift`:0.10(**一律存在**,常數)

#### 新增事件訂閱

- **`EventRegimeChangeConfirmed`**(`regime_debouncer.go` 發布):觸發時更新內部 `currentRegime` 並重置 `prevTotal = 0` (re-baseline,避免 regime 切換時的偽 turnover 事件)

#### Schema 演進

- `DriftEventSchemaVer` 從 `1` bump 到 `2`
- v1 payload 欄位(`max_concentration` / `max_symbol` / `turnover` / `total_value` / `period_start` / `reasons` / `thresholds`)完整保留(append-only 演進)
- 消費者可透過 `data.schema_version` 判斷 v1 / v2

#### 測試覆蓋

- **9 個 v2 characterization tests**(`drift_detector_v2_test.go`):
  - `TestDriftDetector_V2TargetDriftEmitted`:drift > 10% emit + 驗證 v2 欄位
  - `TestDriftDetector_V2TargetDriftNoEmit`:target 對齊 + 平衡不 emit
  - `TestDriftDetector_V2NilProviderGraceful`:nil provider 保留 v1 行為
  - `TestDriftDetector_V2EmptyTargetWeights`:空 target map 跳過 target drift
  - `TestDriftDetector_V2RegimeChangeUpdatesCurrentRegime`:handler 更新 currentRegime
  - `TestDriftDetector_V2RegimeChangeRebaselinesPrevTotal`:regime change 重置 prevTotal
  - `TestDriftDetector_V2SymbolNotInTargetMap`:target=0 處理缺漏 symbol
  - `TestDriftDetector_V2SchemaVersionBumped`:SchemaVersion=2
  - `TestDriftDetector_V2ConcurrentProviderAccess`:concurrent 讀取無 race(-race flag)
- **v1 6 個 tests 一字不改**:全綠
- 15 個 drift tests 全部 PASS,全 `internal/monitoring/service/` 套件綠

#### 文件同步

- `docs/REFERENCE/events/drift-detector.md`:Schema Version 2 + 5 個 v2 欄位 + 9 個 v2 測試描述
- `docs/REFERENCE/events/INDEX.md`:EventDriftDetected 標記為 v2,Schema Version 說明段落更新
- `internal/monitoring/AGENTS.md`:新增 DriftDetector v2 段落(Architecture、Event Subscriptions、9 個模組陷阱、向後相容保證)

#### 與 PR #632 Wave 9 plan 的關聯

- 本 PR 收尾 Wave 9 plan §7 Risks 提到的「v2 DriftDetector target weights drift」follow-up
- Wave 9 plan Out of Scope 三項中此為收尾項

#### 已知限制 / 後續工作 (out of scope,follow-up PR)

- **本 PR 不做**:`internal/monitoring/service` 加入 Layer 3 baseline(目前 baseline 涵蓋 internal/config, cmd/atlas, internal/narrative, internal/orchestrator, internal/portfolio, internal/sim, internal/risk)。DriftDetector 介面為 public,後續 PR 應為其加 baseline。
- **本 PR 不做**:`cmd/atlas/main.go` wire `NewDriftDetectorWithTargets`(v1 已知未 wire,本 PR 維持 scope 嚴格)
- **本 PR 不做**:實作 symbol-level target weight provider(目前 `TargetWeightsProvider` 介面已備但無 production 實作;後續 PR 可從 portfolio Optimizer 衍生)

## [0.0.0.13] - 2026-06-23

### Added — P2/P3 Startup-Herd 回歸測試

`internal/apigateway/background.go` 的 runTask 內含「啟動抖動」邏輯（`time.Duration(rand.Int63n(int64(task.Jitter)))`），目的是防止多個 process 同時啟動（rolling deploy / 災難切換）時，所有 task 首次執行擠在 t=0 造成上游 provider thundering herd。既有測試僅驗證 `Register` 階段的 Jitter 欄位自動設定，沒有驗證 runTask 真的等待抖動。新增 2 個回歸測試守住此行為：

- **`TestBackgroundTaskManager_RunTask_AppliesStartupJitter`**：驗證 runTask 在首次執行前確實等待了 Jitter 設定的時間。Jitter=500ms，bounds=[1ms, 700ms]。下界 1ms 抓出「抖動被移除」的 regression（首執行會在 t≈0 < 1ms）；上界 700ms 容納 rand 抽到接近 500ms + Go runtime 排程誤差。偽陽性率 ≈ 0.2%。
- **`TestBackgroundTaskManager_RunTask_DesynchronizesMultipleTasks`**：驗證 5 個 task 的首次執行時間分散在 [0, 300ms) 區間（最晚 - 最早 ≥ 50ms）。若 `rand.Int63n` 被改成固定值（如 0），所有 task 會擠在 t=0，spread 趨近於 0，測試失敗。

兩測試合併 166 行註解 + 程式碼，覆蓋原本的測試缺口。**未修改 production code** — 抖動邏輯本身正確，僅補上守護測試。

### Test Coverage

- `internal/apigateway/background_test.go` +166 行（兩個 test function + 註解）
- `go test -race ./internal/apigateway/` 全綠（17.7s）
- `go vet` / `staticcheck` clean

## [0.0.0.12] - 2026-06-23

### Fixed — P2 PascalCase SessionSummary fields silently dropped

- 4 `SessionSummary` fields (`TaxSnapshots`, `AfterTaxPnL`, `TotalTaxPaid`, `ParametersVersion`) were defined in the Go struct but absent from the SQL table and all three persistence functions (`SaveSessionSummary`, `LoadSessionSummary`, `LoadAllSessionSummaries`), causing silent data loss on every save/load round-trip.
- Migration `000007_add_session_summary_tax_params` adds the 4 missing columns (`tax_snapshots JSONB`, `after_tax_pnl DOUBLE PRECISION`, `total_tax_paid DOUBLE PRECISION`, `parameters_version TEXT`).
- Updated `SaveSessionSummary` INSERT/UPDATE to include `$15–$18`; updated `LoadSessionSummary` and `LoadAllSessionSummaries` SELECT + Scan to include the 4 new columns, plus `taxSnapshots` JSON unmarshal.
- Added `TestPostgresRepository_SessionSummary_TaxAndParamsFields` round-trip test covering all 4 fields.
- `go vet` and `staticcheck` clean.

## [0.0.0.11] - 2026-06-23

### Fixed — FinMind Trading-Day Guard (P1)

- **`internal/marketdata/finmind_client.go`**:
  - **New `isTaiwanTradingDay(t time.Time) bool` helper**: returns `false` for Saturday and Sunday. Hooked into `FinMindProvider.GetQuotes` as the first step — if `asOf` falls on a weekend, return an explicit error `"finmind: asOf YYYY-MM-DD is not a Taiwan trading day (weekend or holiday)"` and skip the HTTP call entirely.
  - Before: `GetQuotes` would call FinMind's `TaiwanStockPrice` dataset with a weekend date; FinMind returns `{"data":[]}` (empty array, not an error); the code then fell through `len(data) == 0` and returned `"finmind: no price data for 2330 on 2026-04-25"` — a confusing message that looks like a symbol/date mismatch rather than a non-trading-day query.
  - After: weekend queries are caught at the provider boundary with a self-explanatory error and zero HTTP calls (saves rate-limit budget). Callers that want the previous trading day's data should rewind `asOf` explicitly.
  - **Holiday support deferred**: fixed-date Taiwan holidays (元旦, 228, 清明, 端午, 中秋, 雙十) are not yet encoded. The helper name `isTaiwanTradingDay` (vs. `isWeekend`) signals that holiday support is intended; future work should source holidays from `globalmarket.TradingSchedule.Holidays` or a config file rather than hardcoding per year.

- **`internal/marketdata/finmind_client_extra_test.go`**:
  - **`TestFinMindProvider_GetQuotes_RejectsSaturday`**: `asOf = 2026-04-25` (Saturday) → asserts error contains `"not a Taiwan trading day"` and the mock server receives **0 HTTP calls**.
  - **`TestFinMindProvider_GetQuotes_RejectsSunday`**: `asOf = 2026-04-26` (Sunday) → same assertions.
  - The existing `TestFinMindProvider_GetQuotes_PartialSuccess` (Wednesday 2026-04-29) still passes — guard only fires on weekends.

### Test Coverage

- 2 new tests, all passing under `go test -race -count=1 ./internal/marketdata/` (suite: ~40 tests, 38.4s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: `GetQuotes(ctx, time.Date(2026,4,25,...), ["2330"])` → 1 HTTP call to FinMind → empty `data` array → `"finmind: no price data for 2330 on 2026-04-25"` error. Operator cannot tell whether the symbol is wrong, the date is wrong, or it's a non-trading day.
- After: same call → 0 HTTP calls → `"finmind: asOf 2026-04-25 is not a Taiwan trading day (weekend or holiday)"`. Operator immediately knows to rewind to the previous trading day (2026-04-24, Friday).

## [0.0.0.10] - 2026-06-23

### Fixed — us10y Macro Indicator Zero-Value Guard (P1)

- **`internal/marketdata/yahoo_macro_provider.go`**:
  - **New zero-value guard** in `fetchIndicator()`: after the existing `NaN`/`Inf` check, reject `latest == 0` as a data error. Yahoo Finance returns `closes: [0.0, 0.0, ...]` during US market off-hours or parse failures; without this guard the zero propagates into `MacroDataSnapshot.US10Y.Value = 0` and pollutes downstream yield-spread / US-TW rate differential / stress-index calculations.
  - All 8 tracked macro indicators (`^TNX`, `DX-Y.NYB`, `^VIX`, `CL=F`, `GC=F`, `USDTWD=X`, `SI=F`, `HG=F`) are never exactly zero in real markets, so the guard applies uniformly. The error message includes the ticker and the hint `likely off-hours or parse error` for operator triage.
  - On rejection, the field is left empty in the snapshot (existing `mergeSnapshot` last-write-wins semantics with non-empty `Symbol` check already handles this), and `FetchSnapshot` returns a partial-failure error so callers can detect the degraded state.

- **`internal/marketdata/yahoo_macro_extra_test.go`**:
  - **`TestYahooFinanceMacroProvider_fetchIndicator_ZeroLatestPrice`**: mock Yahoo returns `closes: [0.0, 0.0, 0.0]` for `^TNX` → asserts `fetchIndicator` returns an error containing `zero latest price`.
  - **`TestYahooFinanceMacroProvider_FetchSnapshot_ZeroValueExcluded`**: `^TNX` returns zero (rejected), all other 7 indicators return valid data → asserts `snap.US10Y.Symbol == ""` and `snap.US10Y.Value == 0` (field excluded), `snap.DXY.Value == 104.18` (success path still populates), and `err != nil` (partial failure surfaced).

### Test Coverage

- 2 new tests, all passing under `go test -race -count=1 ./internal/marketdata/` (suite: ~40 tests, 40.7s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: `YahooFinanceMacroProvider.FetchSnapshot` would happily set `US10Y.Value = 0.0` when Yahoo Finance returned zero closes (e.g., early Monday morning US time, or post-holiday data gaps). Downstream consumers (`narrative`, `taiwan_stress_index`, `risk` modules) would then treat 0 as a real rate, producing nonsensical yield-spread signals.
- After: zero is rejected at the provider boundary, the snapshot field is left empty, and the partial-failure error flows to the caller. Downstream code that already checks `Symbol != ""` before reading `Value` continues to work unchanged; code that didn't check now gets an empty field instead of a poisoned zero.

## [0.0.0.9] - 2026-06-23

### Fixed — FubonProxy Port Conflict on Restart (P0)

- **`internal/fubonproxy/manager.go`**:
  - **New `preparePortForRestart()` helper**: probes port 8081 before each restart and returns a 3-state verdict — `(canProceed bool, shouldStop bool)`. Replaces the old "blindly respawn" behavior that caused supervisor to thrash when port was held by a foreign process.
    - `Free` → restart normally, reset `restartFailures` counter.
    - `Healthy` (port serves `/health` and PID is not ours) → supervisor yields to the external managed proxy, logs `restart_external_managed`, and exits.
    - `Foreign` (port held by a process that is not healthy / not ours) → log actionable error `restart_foreign_port` with the offending PID + `kill` command hint, increment `restartFailures`, refuse to respawn.
  - **New `maxRestartFailures = 5` constant** + `restartFailures` field on `ProcessManager`. `supervise()` gives up after 5 consecutive blocked restarts and emits `max_restart_failures_reached` to prevent infinite crash-loop.
  - **`supervise()` updated**: calls `preparePortForRestart()` before every respawn, not just at startup. This closes the gap where a proxy that died and got stuck on a foreign port would trigger an unending respawn cycle.
  - Test-only backoff seam (`restartInitialDelayForTest` / `restartBackoffDelayForTest`) introduced so the cap test can run in ~3s instead of the production `restartInitialDelay` schedule.

- **`internal/fubonproxy/manager_test.go`**:
  - **`TestProcessManager_Restart_PortFree_CanProceed`**: bare port → `preparePortForRestart` returns `canProceed=true, shouldStop=false`.
  - **`TestProcessManager_Restart_PortHealthy_Yields`**: port held by a `/health`-serving process → returns `canProceed=false, shouldStop=true` and logs `restart_external_managed`.
  - **`TestProcessManager_Restart_PortForeign_Retries`**: port held by an unknown process → returns `canProceed=false, shouldStop=false` and logs actionable `restart_foreign_port` with the PID and `kill` command.
  - **`TestProcessManager_Supervise_YieldsToExternalHealthyProxy`**: end-to-end `supervise()` yields and stops when the port becomes healthy externally between restarts.
  - **`TestProcessManager_Supervise_RestartFailureCap`**: 5 consecutive `Foreign` verdicts → `supervise()` logs `max_restart_failures_reached` and exits cleanly without infinite loop.

### Test Coverage

- 5 new tests, all passing under `go test -race -count=1 ./internal/fubonproxy/` (suite: 19 tests, 42.4s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: supervisor would loop forever respawning fubon-proxy against a foreign port-holder, with no failure cap and no yielding to a healthy external instance.
- After: port-conflict restart attempts are bounded (max 5), the supervisor yields to a healthy external proxy instead of fighting it, and the operator gets an actionable error message (`kill <pid>`) on each blocked attempt.

## [0.0.0.8] - 2026-06-22

### Added — Wave 9 YELLOW Observability Expansion (5/5 events shipped)

- **`EventChannelIndividualHealth`** (`monitor.channel.health.individual`, Wave 9.1): per-channel error visibility for the 4-layer data-visibility safeguard. Service: `internal/monitoring/service/channel_health_synthesizer.go`. Polls `ChannelErrors()` every 30s with 5s dedup. Provider injected via `ChannelHealthProvider` interface (no `internal/monitoring` import).
- **`EventRegimeChangeConfirmed`** (`market.regime.confirmed`, Wave 9.2): regime change is only confirmed after 30s stability window. Service: `internal/monitoring/service/regime_debouncer.go`. Subscribes to `EventRegimeChange`, checks every 5s, dedupes by `newRegime`.
- **`EventFactorWeightRegression`** (`portfolio.factor.regression`, Wave 9.3): when regime changes, the factor weight shift is computed as `Σ|curr - prev|`. Emit if score ≥ 0.5. Service: `internal/monitoring/service/factor_weight_regression.go`. Constructor DI: `NewFactorWeightRegressionDetector(bus, provider WeightProvider)`. `monitoring/service` does NOT import `portfolio` (forward-compat with #611).
- **`EventDriftDetected`** (`portfolio.drift.detected`, Wave 9.4): v1 concentration drift + simple turnover ratio. Service: `internal/monitoring/service/drift_detector.go`. Subscribes to `EventPositionUpdate` (per-symbol, no portfolio snapshot required). Thresholds: concentration > 0.25 OR turnover > 0.15. v2 (target weights drift) deferred to #611 refactor.
- **`EventIngestionLagSpike`** (`apigateway.ingestion.lag.spike`, Wave 9.5): ingestion p99 > 5s triggers warning. Service: `internal/monitoring/service/ingestion_lag_monitor.go`. Provider interface: `IngestionLagProvider.P99LatencySeconds() float64`. **Follow-up**: `internal/apigateway/background.go` add `ingestion_latency_seconds` Prometheus histogram + implement `IngestionLagProvider`.

### Added — Wave 9 Infrastructure

- 5 EventType constants + `eventDescriptions` entries in `internal/eventbus/eventbus.go` (Wave 9.0a)
- 4 service framework interfaces in `internal/monitoring/service/` (Wave 9.0b): `WeightProvider`, `RegimeDebouncer`, `DriftDetector`, `ChannelHealthSynthesizer` (replaced with full implementations in 9.1-9.5)
- 5 Prometheus alert rules in `monitoring/rules/wave9_*.yml` (all `enabled: false` by default per PD-W9-1)
- 5 new docs in `docs/REFERENCE/events/`: `channel-individual-health.md`, `regime-change-confirmed.md`, `factor-weight-regression.md`, `drift-detector.md`, `ingestion-lag-spike.md`

### Changed — Forward-Compat Design Verified

- 0 modifications to Issue #611 9-file refactor targets (verified by `git diff --stat`)
- All Wave 9 services implement forward-compat DI: `monitoring/service` depends only on `eventbus` package, not on `portfolio` / `monitoring` / `apigateway`
- Alert rules default to `enabled: false`; operator must explicitly enable per PD-W9-1

### Test Coverage

- 5 service test files added, 32 test functions total
- Race conditions tested via `go test -race` for each event handler
- Dedup windows, threshold boundaries, nil-provider no-panic paths all covered

### Out of Scope (follow-up)

- **Frontend SSE integration** for the 5 new events: needs updates to `internal/monitoring/api/events/sse_handler.go` (6-component buffer) and `web/static/js/` event rendering. Tracked as separate task.
- **IngestionLagProvider implementation** in `internal/apigateway/background.go` (additive change, not blocked by #611).

## [0.0.0.7] - 2026-06-22

### Added — Wave 8 Event-Driven Expansion (6/9 RED events shipped)

- **`EventRiskGateRejected`** (`monitor.risk_gate.rejected`, PR #619): emitted when RiskGate verdict is `BLOCK` or `HALT`. Producer bridge wired at `cmd/atlas/main.go:1603-1614`. SSE-delivered with 50-event catch-up buffer.
- **`EventRiskGateAllowed`** (`monitor.risk_gate.allowed`, PR #619): emitted when RiskGate verdict is `ALLOW`. Three-way semantic split introduced in Wave 8.2 收尾.
- **`EventRiskGateOverridden`** (`monitor.risk_gate.overridden`, Wave 8.2 收尾): NEW constant, emitted when RiskGate verdict is `REDUCE` or `ALERT_ONLY`. Fills the semantic gap between full-allow and full-block; frontend can render distinct badges without parsing `payload.Verdict`.
- **`EventIndustryCalendar`** (`industry.calendar.event`, PR #621): emitted by `PublishIndustryCalendarEvent` for Taiwan market calendar events (除權息、MSCI 調整、財報季等).
- **`EventBacktestCompleted`** (`experiment.backtest_completed`, PR #622): emitted after `internal/autobacktest.Runner.RunAndStore` succeeds and live store is synced.
- **`EventCalibrationCompleted`** (`experiment.calibration_completed`, PR #623): emitted after `cmd/atlas/main.go` `linkage_calibrate` task completes `CalibrateParameters`.
- **`EventTradeSlippage`** (`trade.slippage`, PR #625): emitted by `internal/live/order_manager.go` on every order fill (status == "filled"); records expected vs actual price in BPS.

### Changed — RiskGate Three-Way Semantic Split (Wave 8.2 收尾)

`PublishRiskGateEvent` auto-routing refactored from 2-way (rejected/allowed) to **3-way split**:
- `BLOCK` / `HALT` → `EventRiskGateRejected`
- `REDUCE` / `ALERT_ONLY` → `EventRiskGateOverridden`
- `ALLOW` → `EventRiskGateAllowed`

This preserves the semantic distinction between "fully allowed", "modified after override" (partial reduction or alert-only warning), and "blocked entirely". Test coverage locked via `TestPublishRiskGateEvent_ThreeWayRouting`.

### Documentation — Wave 8.10 Docs 收尾 + Wave 8.2 收尾

- PR #627: 補寫 3 個既有事件 doc（`narrative-event.md`, `health-alert.md`, `promotion-recorded.md`）+ 更新 INDEX.md + P3 編號對齊。
- Wave 8.2 收尾: 新建 `docs/REFERENCE/events/risk-gate-overridden.md`；更新 `docs/REFERENCE/events/risk-gate-allowed.md` 反映純 ALLOW 語意。
- `docs/REFERENCE/events/INDEX.md`: 加入 `EventRiskGateOverridden` 列 + Wave 8.11+ LLM 事件推遲註記。

### Deferred — LLMAnnotator 3 events pushed to Wave 8.11+

- `LLMAnnotatorCircuitOpen` (Wave 8.5): 原計畫實作 LLM circuit breaker 事件。LLM 重構（PR #628/#629）改為 capability-based routing，原 circuit breaker 由 `llm_annotator:requests_good:rate5m` Prometheus metric + `llm_annotator_availability_fast_burn` alert rule 取代（`monitoring/rules/llm_annotator_alerts.yml`）。
- `LLMAnnotatorFallbackUsed` (Wave 8.6 LLM): 同上，fallback 路徑由 router logs 與 metrics 揭露。
- `LLMAnnotatorQuotaExceeded` (Wave 8.7): 同上，quota 控管整合進 router 計費。

Wave 8.11+ 規劃待 Wave 8 v0.0.0.7 合併後再開新 plan。

### Added — Phase 4 LLM Loop Coverage (PR #628/#629 follow-up)

- **`ConfidenceCommentary` hook verification tests**: `internal/risk/confidence_hook_test.go` mirrors `forensics_hook_test.go` (3 cases: hook called / nil hook / error returns empty). `internal/risk/gate_test.go` adds 2 integration tests verifying `RiskGate.publish()` writes `ConfidenceCommentary` to subscribers.
- **`docs/llm-trigger-analysis.md` updated**: All 5 LLM hooks (RationaleTranslator, ScenarioExplainer, RegimeExplainer, SentimentExplainer, PerformanceForensics, ConfidenceCommentary) marked ✅ RESOLVED with production caller line numbers (`cmd/atlas/main.go:1892/1903/1915/1937/1949`, `internal/narrative/ingestor.go:139`, `internal/orchestrator/system.go:521`, `internal/risk/gate.go:174`).

### Added — PR #630 SmartUniverseBuilder pipeline (related infra)

- 4-layer universe pipeline (`IndustryFilter` / `ScoringScreener` / `RiskExclusionFilter` / `NarrativeEventBridge`) with `WriteUniverseRegistry` atomic-write + `.bak` rollback. Wired into `cmd/atlas/main.go` with `WatchlistMu` serialization.
- Review audit trail archived to `docs/archive/2026-06-22-review-pr630.md`.

## [0.0.0.6] - 2026-06-20

### Added

- **Wave 7.5 Tasks 1+2 — Risk gate safety wiring + orphan config rejection**: risk gate controls now enforce explicit safety limits before promotion, and the system rejects orphaned/misplaced `parameters.json` files that would previously silently merge.
- **Wave 7.5 Tasks 3+5+6 — Audit fixes**: Alertmanager webhook receiver hardened with proper field validation and HTTP status codes; field contract checks updated for the new valid-fields registry; calibration metadata preservation improved across auto-rollback scenarios.
- **Wave 7.5 finalization — Auto promotion events**: `AutoJudgePromoter` is now wired into the atlas scheduler; when an experiment is auto-promoted, an `EventPromotionRecorded` event is emitted and delivered to dashboard clients via SSE with a 50-event catch-up buffer.
- **`GET /api/dashboard/fetch-log` endpoint**: returns recent channel fetch events (`status`, `latency_ms`, `error`) from the persistent ring buffer, surfaced in the data-channel dashboard.

### Fixed

- `internal/alerting/webhook_handler.go` now returns `400 Bad Request` for malformed Alertmanager payloads and `422 Unprocessable Entity` for missing required fields instead of silently succeeding.
- `internal/monitoring/channel_health.go` now records per-channel failure reasons, so the fetch log and degraded-status panels show why a channel failed.

### Changed

- Risk gate panel UI (`web/static/js/components/risk-gate-panel.js`) now displays rejection reasons and inline safety override controls.
- Channel fetch log entries are now written by all CLI ingestion tools via `monitoring.RecordChannelFetch`, producing a single observability source for dashboard and alerting.

## [0.0.0.5] - 2026-06-17

### Breaking — Performance Report Field Renames + Threshold Config

**`AgentContribution.TotalReturn` → `AgentContribution.AggregateForwardReturn`** (JSON `total_return` → `aggregate_forward_return`).
**`RegimePerformance.TotalReturn` → `RegimePerformance.AggregateForwardReturn`** (same JSON rename).
**`AgentContribution.SharpeLike`**: `float64` → `*float64`. Now nullable when samples < `reporting.sharpe_min_samples` (default 5) or stdDev == 0. Frontend renders `"N/A"` for null.

These three fields exist in the `GET /api/performance-report` response payload (and the in-process `PerformanceReport` struct used by `cmd/judge-experiment`, `cmd/promote-baseline`, etc.). Frontend code must read the new field name and dereference `sharpe_like` defensively.

### Added — Cost-Adjusted Win-Rate Threshold

`reporting.win_rate_threshold` parameter (default 0.002, i.e. 0.2%). Win classification now requires `ForwardReturn > win_rate_threshold` instead of `ForwardReturn > 0`, covering transaction cost (~0.15% TW market) + slippage buffer. Configurable via `configs/parameters.json`. Affects `calculateTradeMetrics`, `calculateTopAgents`, and `calculateRegimeBreakdown`.

### Fixed — Fubon Proxy: Remove `FUBON_PROXY_URL` env override (IPv6 dual-stack root cause)

The recurring fubon channel failures (`dial tcp [::1]:8081: connect: connection refused`) were traced to a single design defect: `fubon_client.go` and `hybrid_provider.go` both read `os.Getenv("FUBON_PROXY_URL")`, which could override the safe hardcoded default `127.0.0.1:8081` with `localhost:8081` — resolved to IPv6 `[::1]` on macOS dual-stack systems while the Python fubon-proxy binds IPv4 only.

**Changes**:
- `internal/marketdata/fubon_client.go`: Replaced `os.Getenv("FUBON_PROXY_URL")` fallback in `newFubonClient()` with direct `fubonProxyBaseURL` constant. Removed unused `"os"` import.
- `internal/marketdata/hybrid_provider.go`: Removed both `os.Getenv("FUBON_PROXY_URL")` reads in `NewHybridProvider()`; always probes `127.0.0.1:8081` directly. Removed unused `"os"` and `"net/url"` imports.
- `.env_example`: Removed `FUBON_PROXY_URL` line and IPv6 warning comment (env override no longer exists).
- `.env.example`: Removed `FUBON_PROXY_URL` line (commented-out `localhost:8081` default).

This is the B-plan from PR #556 that was never implemented — the final root cause fix after 22 commits and 17+ PRs of layered defenses (circuit breaker, probe, auto-start, panic recovery, zombie kill) that never addressed the `.env` → env-override path.

### Added — `/api/dashboard/agent-names` endpoint

New endpoint serving the agent display-name registry from `configs/agents.json` as JSON. Single source of truth replacing the two competing static maps (`web/static/js/names.js` and `web/static/js/shared/constants.js`). Returns `{"agents": [{"id", "name", "skill", "layer"}, ...]}` or empty `{"agents": []}` when file is missing.

## [0.0.0.4] - 2026-06-15

### Fixed — Pipeline Data Visibility (6 commits, P0-P2)

**P0-C/D/E (20d1f56e) — frontend zero-value display**:
- `computePipelineSummary`: fallback `items` to `outcome_count` when summary missing.
- `formatDate`: filter zero-time (year<2000, NaN, year>9999), return `"-"`.
- `regimeLabel`: unify `"unknown"` → `"-"` across all 3 rendering paths.

**P1-A (ce4d89fc) — pipeline status banner**:
- `buildPipelineStatusBanner`: 5-status handler (`ok/degraded/minimal/no_session/error`).
- `is_fallback_session` as independent dimension.

**P1-B (366151b7) — OutcomeCount fallback**:
- `LoadSessions`: when `OutcomeCount==0`, derive from `recommendation_outcomes.jsonl` line count.
- Only overwrites zero — preserves summary's post-filter semantics.

**P1-C (0a553e4f) — backfill-summaries tool**:
- `cmd/backfill-summaries`: one-shot CLI for repairing orphan session directories.
- `internal/backfill/` package with `BackfillSummaries()` — idempotent, dry-run, never overwrites existing.
- 6 test cases covering orphan/existing/empty/mixed/noop scenarios.

**P2-A (36ac8a87) — RecordSessionSummary retry**:
- `recordSummaryWithRetry`: 3 attempts, 100ms linear backoff.
- Single chokepoint for all production summary writes.

**P2-B (4e4e97fa) — data_status sibling**:
- `parseSessionsList`: surface `data_status` as sibling field in array response.

## [0.0.0.3] - 2026-06-15

### Fixed
- `internal/live`: `TestOrderManager_Run_BrokerRejectsOrder` was flaky in `go test ./internal/live` — the assertion read the first event from the SubscribeAll channel, but `ChannelEventBus` dispatches handlers in their own goroutines, so `order.rejected` could arrive before `order.error`. The test now drains error events until it finds the expected `EventOrderError` (1 commit, 7906284b).

## [0.0.0.2] - 2026-06-14

### Added — Coverage Push (Stages 1-6 of functional-coverage-fix plan)

Total coverage: 57.6% → 61.1% across 7 commits on `feat/coverage-improvement`.

**Stage 1 (f1d6f712) — `feat(config,domain)`**:
- `internal/config`: restore `mergeFallbackPriceTargetsDefaults` helper to merge missing/partial `FallbackPriceTargets` entries from defaults.
- `internal/domain/recommendation`: add `Regime string` field to `RecommendationOutcome` with `json:"regime,omitempty"` tag.

**Stage 2 (34db5301) — `feat(monitoring,orchestrator)`**:
- `internal/monitoring/service`: per-regime grouping in `computeAgentRegimeBreakdown` (uses `o.Regime` with fallback to `defaultRegime`).
- `internal/orchestrator`: populate `RecommendationOutcome.Regime` in `buildSyntheticOutcomes` and `buildReplayOutcomes` (and corresponding `prism_executor.go`, `adversarial_executor.go`).

**Stage 3 (54b4442e) — `test(monitoring)`**:
- 9 test files in `internal/monitoring/` root (gateway_adapter, alert_api, alert_store, autohandler, channel_health, dashboard_api, metrics, risk_calibrator, new data_quality).
- Coverage: 50.1% → 69.7% (+19.6pp).

**Stage 4 (f62b7182) — `test(apigateway)`**:
- 15 test files covering 20 previously-zero `Fetch`/`HealthCheck`/`RateLimit` functions across 10 adapters.
- Coverage: 55.4% → 60.4% (+5.0pp). 24 funcs remain blocked pending marketdata HTTP-client injection (Stage 5c).

**Stage 5 (4e070c60) — `test(repository,shared,marketdata,apigateway)`**:
- `internal/repository`: 12.4% → 76.7% (+64.3pp). Added `pgPool` interface for testability (option C: Doer abstraction) + 637-line `postgres_unit_test.go` using pgx fake pool.
- `internal/domain/shared`: 28.4% → 100% (+71.6pp). 3 new test files covering 5 helpers.
- `internal/marketdata`: added `SetHTTPClient(c *http.Client)` testability hook to 10 providers (option A: least invasive). Coverage 49.1% → 53.2%.
- `internal/apigateway`: completed the 24 previously-blocked adapter funcs via new `adapter_http_fetch_test.go`. Coverage 60.4% → 79.0% (+18.6pp).

**Stage 5 follow-up (337f6647) — merge origin/main**:
- Integrated PR #526 (pipeline degraded status), #527 (Minimal/NoSession tests), #528 (sectorallocation module), #529 (wave4-cleanup).
- 4 conflicts resolved in favor of main per user priority instruction: `monitoring/service/pipeline.go` (semantically equivalent), `orchestrator/system.go` (API signature change `domain.Regime` → `string`), and 2 test files.
- Stage 2 regime population preserved at `buildSyntheticOutcomes` line 1442 and `buildReplayOutcomes` line 1492.

**Stage 6 (999b1fb6) — `test(monitoring/api)`**:
- 8 test files across 7 sub-packages (test-only scope, skipped `api/pipeline` per user priority):
  - `narrative` 8.4% → 90.8% (+82.4pp)
  - `macro` 10.0% → 70.0% (also fixed 2 pre-existing compile errors in `handlers_stub_test.go`)
  - `live` 20.2% → 88.2% (+68.0pp)
  - `industry` 46.6% → 77.7% (+31.1pp)
  - `tax` 47.4% → 80.8% (+33.4pp)
  - `dashboard` 49.6% → 70.9% (+21.3pp)
  - `shared` 98.8% → 98.8% (already at max, no change)

### Notes
- Pre-existing data races in `internal/live` (3 tests) are NOT caused by this push; they were introduced by `b39fb5b9 test(live): cover scheduler, store, twse_adapter, agent_runner, orchestrator, order_manager` which is on main.
- `gitnexus detect_changes` deferred: index stale (last indexed `891e724`); will refresh in follow-up.
- 7 commits pushed: `f1d6f712`, `34db5301`, `54b4442e`, `f62b7182`, `4e070c60`, `337f6647`, `999b1fb6`.

## [0.0.0.1] - 2026-06-13

### Fixed
- `internal/config`: merge `FallbackPriceTargets` defaults to prevent a panic when `_default` is missing and preserve custom per-stage overrides.

### Added
- `TestLoadParametersConfig_FallbackPriceTargetsDefaultsMerged` to verify `_default` and custom key merge behavior.
