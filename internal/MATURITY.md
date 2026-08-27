# internal/ Maturity Reference

本文件是 `internal/` 下所有 Go package 的成熟度快速參考表。
每個 package 的 `doc.go` 中有對應的 `Maturity:` 標記，此表應與之保持一致。

成熟度層級定義：

| Tier | 標記 | 含義 | 判斷標準 |
|------|------|------|----------|
| **S** | `stable` | 穩定生產 | API 穩定、在生產執行路徑中、breaking change 需 migration plan |
| **E** | `evolving` | 演進中 | API 可能調整、功能完整但仍在迭代、可能晉升為 stable |
| **X** | `experimental` | 實驗中 | 研究性質、API 不穩定、可能被廢棄、不應被其他模組依賴 |
| **U** | `utility` | 輔助工具 | CLI 工具、資料轉換、一次性驗證 — 非 runtime 一部分 |
| **A** | `archived` | 封存 | 已被 Phase 2 canonical 取代；API frozen，僅 bug fix；新程式碼禁止依賴 |

---

## S · Stable（穩定生產）— 24 packages

所有直接由 `cmd/atlas/main.go` 匯入的模組，處於生產執行路徑中。

| Package | 描述 | 關鍵型別/介面 |
|---------|------|--------------|
| `apigateway` | API Gateway — 資料源統一入口、Rate Limiter、通道管理、背景任務排程（憲法規範） | `Gateway`, `BackgroundTaskManager`, `Fetch()` |
| `baseline` | Baseline policy 版本控制與升降級 | `Policy`, `Promote()`, `Revert()` |
| `bootstrap` | 系統初始化 — HTTP 路由、Dashboard 註冊 | `Bootstrap()` |
| `config` | 環境變數讀取（`ATLAS_*` 前綴）、參數配置管理 | `Config`, `ParametersConfig` |
| `domain` | 領域型別 — canonical types、string enum（全系統依賴） | `Regime`, `Recommendation`, `Position` |
| `eventbus` | 事件匯流排 — Publish/Subscribe | `ChannelEventBus` |
| `strategy_techniques` | 投資心法庫 — 5 層框架（L1~L5）+ 4 核心指標 + 自我修正 | — |
| `experiment` | 實驗生命週期 — mutation → execute → judge → promote/revert | `Executor`, `Judge`, `Candidate` |
| `industry` | 產業生態系 — 參數化分類樹、供應鏈連動、季節性模式、週期羅盤、風險聚合、農曆自動化事件日曆 | `ClassificationTree`, `SupplyChainGraph`, `SeasonalEngine`, `CycleTracker`, `EventCalendar`, `GetAllRisksForIndustry` |
| `janus` | JANUS 跨 cohort regime 偵測與 PRISM 權重動態調整 | `Detector` |
| `ledger` | JSONL append-only 持久化 — outcomes/scorecard | `LoadOutcomes()`, `RecordSessionSummary()` |
| `live` | 即時交易 — broker execution、context 統一、原子寫入（需 flag 啟用） | `Orchestrator` |
| `logging` | 統一日誌介面 — Info/Error 系列 | `Logger` |
| `marketdata` | 資料提供者抽象 — TWSE OpenAPI、Fugle、Hybrid | `Provider`, `HybridProvider` |
| `monitoring` | 監控 API 與 Dashboard — 115 handlers、200 symbols | `Server` |
| `narrative` | 宏觀敘事事件偵測 — 因果鏈、台灣壓力指數 | `NarrativeEvent`, `Ingestor` |
| `orchestrator` | 核心協調層 — `SystemCore`、`PluginHost`、三層 executor 路由 | `SystemCore`, `PluginHost` | Wave 11 L2.1：新增 `llmSectorAgentsPlugin`（Issue #719 wired）— opt-in（env `LLM_SECTOR_AGENTS_ENABLED`），driver 為 nil 時回 no-op pass-through；`SectorAgentLLM` 已用 #711 Phase 3 的 `PlanDriver`+`ReflectDriver` 拆分設計;**Wave 11 L2.3**（v0.0.2.0）：新增 `SemiconductorLLMAgent`(Issue #711 PR5b),LLM-driven sector agent behind `UseLLMSectorAgents` flag(default off)。`llm_driver_adapter.go` + `prompts/` 屬 L2.3 PoC 基礎設施。`sector_agent_llm` 維持 experimental(等 L2.4 觀察窗口驗證) |
| `portprobe` | Stateless TCP port 探測 helper — Probe / LookupOccupant / IsFubonZombie / KillOccupant（wildcard `net.Listen` 無 side effect） | `State`, `Occupant`, `Probe()` |
| `portfolio` | Darwinian 權重管理（`[0.3, 2.5]`）+ FactorEngine（動能/價值/品質） | `Manager`, `FactorEngine` |
| `repository` | PostgreSQL 持久化 — `DualWriteRepository` | `DualWriteRepository` |
| `risk` | 風險管理 — `RiskManager`、VaR、宏觀回撤 | `RiskManager` |
| `storage` | 檔案儲存抽象 — 原子寫入、檔案管理 | — |
| `startup` | 一次性啟動期 preflight 檢查 — `Preflight(claims []PortClaim)` 包 portprobe.Probe + actionable error | `Preflight()`, `PortClaim` |
| `constants` | 跨 binary 共用常數 — atlas HTTP API 預設 listen、fubon-proxy port、ATLAS_BASE_URL fallback | 由 `cmd/*` + `internal/monitoring/api/system/health_handlers.go` 使用 | value-only API（untyped string/int），被 `internal/monitoring` runtime health probe 引用，需與 `docker-compose.yml` / `Makefile` / `services/fubon-proxy/main.py` 同步 |

---

## E · Evolving（演進中）— 39 packages

核心模組，由 stable 模組間接使用，API 可能仍在調整。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `autobacktest` | 自動回測 — 定時背景回測任務 | `Runner` | 由 daily monitor pipeline 使用 |
| `backtest` | 視窗回測 — `Window.Run()` | `Runner` | 由 autobacktest 使用 |
| `capitalflow` | 七維錢潮雷達（3+2+2 分層）分解與共振分析 — 官方actor（外資/投信/自營商）T86 + 行為代理（官股/散戶）proxy + 領先／跨市場訊號（期貨 OI / TSM ADR）Z-score、共振係數、品質分數；`docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 | `ForceExtractor`, `ResonanceEngine`, `CapitalFlowReport`, `CapitalFlowAssessment` | **v0.0.2.0 升級**：Wave 11 shipped；被 eventdriven/recommender/marketexplain 廣泛使用 |
| `db` | PostgreSQL 連線管理 | `DB` | 基礎設施，穩定但未直接出現於 main.go |
| `domain/shared` | 跨模組純計算函式（Sharpe、Sortino、頻率常數）— canonical 位置，呼叫模組以 type alias re-export 維持向後相容 | `ComputeSharpe`, `ComputeSortino`, `Frequency` | 由 `domain/AGENTS.md:30` 規範；`portfolio`/`reporting` 為薄 wrapper |
| `eval` | 模型評估指標與可解釋性工具（SK-12~15） — OOS R²、Sharpe、PermutationImportance、PDP | `EvalResult`, `Predictor` | Fin-Skills 驅動，無 runtime 依賴 |
| `eventdriven` | 事件驅動資金流預測 — 事件日曆→資金流方向 + ETF 規模×權重預估 + 營收驚喜 | `Predictor`, `FlowPrediction`, `ETFEstimate`, `RevenueSurprise` | Wave 11 新增；消費 industry.EventCalendar + capitalflow |
| `eventquality` | 事件資料品質閘門 — 5 rules validator、quality log、cross-source verify、sanitize | `EventValidator`, `QualityLog`, `CrossSourceStore`, `SanitizeTitle` | Stage 2 新增，evolving |
| `feature` | 命名特徵萃取（close, volume, return_1d/5d, hl_ratio, ma_ratio, volume_ratio） — 由 `cmd/backtest-pipeline` 和 `internal/experiment` 共用 | `Registry`, `MakeExtractor`, `ForwardReturnLabel` | 由 backtest-pipeline CLI 和 Judge 重要性運算使用 |
| `forecast` | 個股方向性預測 — per-symbol forecast（directional bias、confidence、severity） | `ForecastResult`, `ForecastEngine`, `TradeSignal` | **v0.0.2.0 升級**：Phase 3.5 M4 ✅ shipped（2026-07-02）；預測模型標記 experimental |
| `fubonproxy` | Fubon-proxy 生命週期管理 — 自動啟動/停止/監控 Python FastAPI 微服務 | `ProcessManager`, `Start()`, `Stop()` | 由 cmd/atlas API 模式使用，非致命失敗 |
| `globalmarket` | 全球總經資料管理 | `Manager` | 由 narrative/industry 使用 |
| `metalearning` | 元學習協調器 — `MetaLearner`、策略選擇優化 | `MetaLearner` | 研究階段，可能晉升 |
| `methodology` | 方法論顧問層 — 根據 YAML 配置將市場 regime 映射至投資方法論規則 | `Advisor`, `RegimeToPeriod` | Wave 11 新增；由 recommender handler 消費 |
| `ml` | 監督式學習模型 — OLS、ElasticNet、PCR、PLS 實作（SK-05~09） | `Model`, `Trainer` | 由 Fin-Skills 規範驅動，供 factor/research 使用 |
| `observability` | OpenTelemetry 追蹤基礎設施 — 父 package，下含 `otel` 子 package（OTel SDK init + span helpers） | `TracerName`（re-exported from `otel`） | Wave 10 L2.1，evolving，OTLP exporter 待 production 評估 |
| `observability/otel` | OpenTelemetry trace 初始化 — stdout exporter、parent/child span helpers，給 `llm.router.Call` 與 `llm/clients.DoRequest` 加 span | `Init()`, `StartSpan()`, `TracerName` | Wave 10 L2.1，evolving，OTLP exporter 待 production 評估 |
| `prism` | Regime-specific 訓練佇列（5 種 regime） | `Queue` | 核心模組，indirect import |
| `realtime` | 即時資料轉接器 — `RealTimeAdapter` | `RealTimeAdapter` | 核心模組，indirect import |
| `replay` | TWSE CSV 載入與 forward return 計算 | — | **v0.0.2.0 升級**：27 個 runtime 檔引用，進入 orchestrator/experiment/backtest/scheduler 核心路徑 |
| `reporting` | 報告生成 — Markdown、ASCII chart、Agent 績效表 | — | **v0.0.2.0 升級**：進入 monitoring dashboard API |
| `retail` | RSI-tw 散戶情緒指數 — 複合零售情緒指標（保證金、VIX、機構流向） | `Calculator` | **v0.0.2.0 升級**：進入 portfolio factor bridge + monitoring API |
| `screener` | 宣告式個股篩選 — P/E、P/B、股息率、動能、成交量 | `Screener` | 由 orchestrator executors 使用 |
| `sim` | 模擬引擎 — 部位狀態轉換、`Engine.RunSymbol()` | `Engine` | 核心模組，indirect import |
| `spawning` | Agent 生成管理 — `SpawningManager`、`PerformSpawningCycle()` | `SpawningManager` | 核心模組，indirect import |
| `strategy` | 策略選擇器與登錄 | `Selector` | 由 orchestrator 使用 |
| `strategy_ranker` | 策略排名、回測驗證與分層 — Sharpe/最大回撤/勝率/TAIEX 相關係數計算 + 排名 + tier 分配 | `Ranker`, `Validator`, `RankedReport`, `StrategyReport` | **v0.0.2.0 升級**：Wave 11 新增；產出供 recommender 使用 |
| `scheduler` | ML 模型重訓排程 — auto_calibration、auto_rollback、seasonal_task、l2_4_auto_cron、system_health 定時任務 | `Manager`, `Dispatcher` | **v0.0.2.0 新增**：scheduler 為獨立 orchestrator pipeline 步驟，產出供 strategy_evolver 與 prism retrain 消費 |
| `stress` | 壓力測試場景 — `RunScenario()` | — | **v0.0.2.0 升級**：進入 orchestrator SystemCore live risk evaluation |
| `subscription` | 使用者訂閱與認證 — SQLite store、JWT auth、3-tier 權限系統、7 天免費試用 | `Store`, `JWTManager`, `ValidateTier` | **v0.0.2.0 升級**：Wave 11 新增；進入 MCP auth + recommender runtime |
| `userstate` | 使用者行為狀態實體 — signal 已讀/追蹤清單/紀律日誌 (§9 追蹤→紀律 資料模型骨架) | `UserSignalState`, `UserWatchlist`, `UserJournal` | Gap 3 R1 新增 (2026-08-07)；僅實體型別，storage/API 為後續 R2-R4；`UserID` 對齊 `subscription.User.ID` |
| `tax` | 台灣稅務計算 — `TaiwanTaxCalculator` | `TaiwanTaxCalculator` | 由 sim 使用 |
| `acceptance` | Acceptance gate pluggable 框架 — `Evaluator`/`Pipeline`/`Registry` 介面，給 `experiment/judge.go` 從 hard-coded switch 漸進遷移（bridge feature flag） | `Evaluator`, `Pipeline`, `Registry`, `FuncEvaluator` | Wave 10 L2.2，evolving |
| `acceptance/builtin` | 17 個 acceptance gate evaluators 實作 — `ImproveSharpeLike`/`PreserveDownsideProtection`/`NoDrawdownSpike`/`FactorWeightStability`/`RetailSentimentFilter`/`NoMaterialDrawdownDegradation`/`NoConstraintBypass`/`MaintainSharpeLike`/`ReduceConcentrationRisk`/`FactorQuality`/`ReduceFalsePositiveRate`/`MaintainCROAuthority`/`ReduceSectorBlindspots`/`MaintainIndustryCoverage`/`ReduceStyleDrift`/`MaintainMomentumCatch`/`RespectHoldingPeriod` | ... | Wave 10 L2.2，evolving，17/17 gates ported |
| `dailyreport` | 每日市場報告自動化 — 模板化報告生成（JSON + Markdown）、歷史 archive、郵件訂閱 | `Generator`, `Handler`, `DataProvider` | Wave 11 新增；API: /api/reports/latest + /archive + /subscribe |
| `recommender` | 投資推薦分層系統 — 依 user tier 返回不同層級推薦內容（public/registered/premium） | `Handler`, `TierRecommendation` | Wave 11 新增；消費 subscription + strategy_ranker |
| `marketexplain` | 「為什麼漲跌」散戶市場解說 — 規則式 compose endpoint，彙整 TAIEX、資金流向、國際環境、風險提示 | `Handler`, `Explanation` | H03 新增；API: /api/market/explain |
| `macroflow` | 宏觀 regime → factor weight 調整引擎 — 6 rules（Yellow/Orange/Red × Calm/Stress）+ 7d max-stale + VIX stress 偵測 | `Engine`, `AdjustmentResult`, `RiskLevel` | Wave 11 L2.4 orchestrator integration；`MacroFlowStrategy` 為 orchestrator pipeline 第 7 步，consumed by `internal/orchestrator`，API 可能依 follow-up 整合演進 |
| `sectorallocation` | 產業權重單一權威 — 統一三路計算（industry/portfolio/monitoring）為多因子引擎（base × cycle × seasonal × linkage × narrative × macro × factor） | `WeightEngine`, `ComputeWeights()`, `ComputeWeight()`, 6 `InputProvider` adapters | SA08 完成：defaultEngine 建構、FileClosureStore 實作、6 個 adapter 工廠、composition.BuildSystem 延遲建構、StrategyEvolver 注入；**v0.0.2.0 晉升 E** |
| `methodology` | ATLAS_METHODOLOGY 憲章→runtime 橋接 — YAML rules loader, Advisor 過濾期別×策略匹配 | `Advisor`, `MethodologyRules` | Wave 11 P0-2 新增；configs/methodology_rules.yaml 為單一來源 |
---

## X · Experimental（實驗中）— 13 packages

研究性質模組，API 不穩定，不應被 stable/evolving 模組依賴。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `liveness` | 任務活性心跳 — BTM task_liveness upsert store、staleness monitor、cron ping 端點（Phase 1, 2026-08-17） | `Store`, `StalenessMonitor`, `HandlePing` | 新上線，API 可能調整 |
| `adversarial` | 對抗性訓練 — `AdversarialTrainer`、`BattleResult`、`StressTest` | `AdversarialTrainer` | **v0.0.2.0 維持 X**：被 orchestrator runtime 使用但無 AGENTS.md，需補文件 |
| `reflexivity` | 自反性價格動態引擎 | `Engine` | **v0.0.2.0 維持 X**：被 orchestrator + sim runtime 使用但無 AGENTS.md，需補文件 |
| `alerting` | Alertmanager webhook receiver — 接收 Alertmanager firing/resolved 警報，in-memory ring buffer 保留最近 1000 筆供 SSE/UI 消費 | `AlertWebhookHandler`, `AlertmanagerPayload`, `AlertmanagerAlert` | 掛載於 `/api/v1/alerts`；待 Prometheus alertmanager targets 與 docker-compose alertmanager service 補齊後晉升 evolving |
| `llm` | LLM 多 Provider 統一介面 — 路由器、能力調度、DataClass 閘門、備援鏈，健康端點 | `ProviderImpl`, `DefaultRouter`, `Capability`, `DataClass` | Wave 11 L2.1：effective routing chain 為 3 層（Primary → Backup1 → LastResort）；`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 為 `[PLANNED]` 常數，無 client 實作（Issue #720）。**Phase 2 canonical 介面（Issue #722）**：Phase 2 canonical 介面已就緒（`adapters` + `capabilities`），承接 `llm_annotator` 的角色。詳見 `docs/llm-integration-strategy-framework.md` |
| `llm/schemas` | LLM 能力輸入/輸出結構合約 — 9 個 capability 的 typed I/O 結構，JSON 序列化 | `RationaleGenerationResponse`, `StrategySummaryResponse`, `PromptLintResponse`, `ScenarioSimulationResponse`, `RiskSurfaceExtractionResponse`, `RegimeExplanationResponse`, `PerformanceForensicsResponse`, `CodeReviewAnnotationResponse`, `SentimentExplanationResponse` | Phase 2 為 9 個 capability handlers 提供型別安全 contract；handler 端用 `json.Marshal/Unmarshal` 對接 Router |
| `llm/clients` | LLM Provider HTTP 客戶端 — DeepSeek V4、MiniMax M3 + 共享 `BaseClient`（retry / rate limit / circuit breaker） | `BaseClient`, `DeepSeekClient`, `MiniMaxClient`, `Message`, `ChatOptions`, `ChatResponse` | Phase 2 新增；MiniMax 附中國國家安全法資料主權警告 |
| `llm/capabilities` | LLM 能力處理器 — 10 個 capability handler（failure_attribution + 9 個新），每個封裝 prompt template + schema-typed I/O + Router 呼叫 | `FailureAttributionHandler`, `RationaleGenerationHandler`, `StrategySummaryHandler`, `PromptLintHandler`, `ScenarioSimulationHandler`, `RiskSurfaceExtractionHandler`, `RegimeExplanationHandler`, `PerformanceForensicsHandler`, `CodeReviewAnnotationHandler`, `SentimentExplanationHandler` | Phase 2 從 1 個擴充至 10 個；Kimi K2.7 已移除（coding plan key 限制 CLI 工具，不可用於 app-level 呼叫） |
| `mcp/anomaly` | MCP audit event 異常偵測 — rolling-window z-score、per-tool/per-tenant error-rate、in-memory ring buffer | `Detector`, `Store`, `AnomalyEvent` | Wave 11 Phase 4 Direction A：僅供 `cmd/atlas-mcp` 消費，不應被其他 stable/evolving 模組依賴 |
| `stocktools` | 個股級查詢端點 — quote、fundamentals、chips、technical | `QuoteHandler`, `FundamentalsHandler`, `ChipsHandler`, `TechnicalHandler` | Wave 11 新增；API: /api/stock/*；供 atlas-mcp stock_* tools 使用 |
| `charter` | 憲章選項與 A/B 統計 — per-run 開關（PeriodOnly/StrategyFilter/MacroFlow/CashReserve/ConvictionFloor）、paired t-test、BCa bootstrap | `Options`, `StepwiseArms`, `PairedTest`, `BCa` | Phase C3 新增（2026-08-22）；實驗性 A/B harness，API 可能調整 |
| `stockpicker` | 個股選股核心 — 三層勝率數學 + outcome/win-rate 儲存 + PIT 回測聚合 job + 可設定條件引擎（PR 1a/1b/1c/2a，2026-08-28） | `WinRate`, `WilsonScoreInterval`, `CalibrationStatusFor`, `NetHit`, `SignalWinRate`, `StockWinRate`, `StrategyWinRate`, `RunBacktest`, `AggregateFromStore`, `GroupAndSummarize`, `Condition`, `ConditionRegistry`, `DefaultConditions` | 含回測聚合 job（`cmd/run-stockpicker-backtest`，`-conditions` 可選條件）；fundamentals 條件僅 live_observe_only；尚未接 runtime 排程/MCP tool；API 可能調整 |
---
## A · Archived（封存）— 1 package

已被 Phase 2 canonical 取代；API frozen，僅接受 bug fix 維護；新程式碼禁止依賴。

| Package | 描述 | 關鍵型別/介面 | 封存原因 |
|---------|------|--------------|----------|
| `llm_annotator` | LLM 歸因標註 — 自然語言解釋 StrategyFrame 失效原因（Kimi/Moonshot API） | `Annotator`, `KimiClient`, `MockAnnotator`, `FailureContext`, `CircuitBreaker`(wrapper), `CircuitState`(alias) | **Wave 11 L2.1（Issue #722）**：Phase 2 canonical 介面（`internal/llm/capabilities/failure_attribution` + `internal/llm/adapters`）已就緒，承接本套件的角色。**Wave 12 Phase 2（Issue #731）**：CircuitBreaker 統一為 `apigateway.CircuitBreaker` canonical owner；本套件保留 `CircuitBreaker` thin wrapper 委派（含 `CircuitState = apigateway.State` 型別別名、`ErrCircuitOpen` sentinel、`Allow()`/`Snapshot()`/`WithNowFunc()`）；4 層 transitive import cycle 已破壞（`monitoring.ChannelHealthStore` 等搬到 `apigateway`）。需 `LLM_ANNOTATOR_API_KEY` 環境變數（透過 apigateway `config.GetSecret` 取得），opt-in 啟用（空時 `/api/strategies/{id}/annotate` 回 50…

---

## U · Utility（輔助工具）— 7 packages

CLI 工具、資料轉換、一次性驗證。非 runtime 一部分。

| Package | 描述 | 關聯 CLI 入口 | 備註 |
|---------|------|--------------|------|
| `importer` | CSV → JSONL 資料匯入 — TWSE、FinMind | `cmd/import-replay` | — |
| `taskexec` | 非同步任務執行器 — `Manager`、`Cancel/Subscribe` | — | **v0.0.2.0 維持 U**：進入 bootstrap + monitoring，但屬輔助基礎設施 |
| `calibration` | 參數校準純邏輯 — GARCH/VaR/Darwinian/Factor 推斷 | `cmd/calibrate-parameters` | 工具層，非 runtime |
| `buildinfo` | Runtime binary metadata — version / commit / build time | 由 `system/health_handlers.go` 與 MCP 透出 | E08 引入，experimental tier |
| `backfill` | 一次性 ledger state 修復工具 — 孤兒 summary.json 補寫、dry-run、安全 idempotent | `cmd/backfill-summaries` | 工具層，非 runtime |
| `taiwanholidays` | 台灣交易日曆單一來源 — lunar/固定假日表（2023-2040）、PreviousTradingDay | `IsTaiwanTradingDay`, `PreviousTradingDay` | P1-8 新增（2026-08-23）；消除 marketdata/industry 雙份 drift |
| `reconcile` | session summary 雙寫對帳 — PG vs JSONL 對稱差、單邊缺口回填（B6） | `cmd/reconcile-sessions` | 工具層，非 runtime；conflicts 永不自動覆寫 |

---

## 非 Package 目錄

| 目錄 | 說明 |
|------|------|
| `testdata/` | Go 標準 `testdata` 目錄，存放測試 fixtures（非 Go package） |

---

## 維護規則

1. **新增模組**：必須同時更新此表與 `doc.go` 中的 `Maturity:` 標記。
2. **成熟度變更**：從 X→E、E→S 需 PR review；從 S→E 或任何降級需 migration plan。
3. **一致性檢查**：CI 應執行 `grep -r "Maturity:" internal/*/doc.go` 並與此表比對。
4. **移除模組**：先標記為 X 一週後，再標記為 A（封存）至少一個 minor 版本週期，最後再刪除目錄。封存期間僅接受 bug fix，禁止新增 feature。
5. **Phase 2 取代 → 封存**：當模組被 Phase 2 canonical 介面完整取代（例如 `llm_annotator` 被 `internal/llm/capabilities/failure_attribution` 取代），可直接標記為 A 並從主動維護清單移除。
