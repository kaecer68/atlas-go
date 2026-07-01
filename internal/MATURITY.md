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

## S · Stable（穩定生產）— 23 packages

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
| `orchestrator` | 核心協調層 — `SystemCore`、`PluginHost`、三層 executor 路由 | `SystemCore`, `PluginHost` | Wave 11 L2.1：新增 `llmSectorAgentsPlugin`（Issue #719 wired）— opt-in（env `LLM_SECTOR_AGENTS_ENABLED`），driver 為 nil 時回 no-op pass-through；`SectorAgentLLM` 已用 #711 Phase 3 的 `PlanDriver`+`ReflectDriver` 拆分設計;**Wave 11 L2.3**（v0.0.0.21）：新增 `SemiconductorLLMAgent`(Issue #711 PR5b),LLM-driven sector agent behind `UseLLMSectorAgents` flag(default off)。`llm_driver_adapter.go` + `prompts/` 屬 L2.3 PoC 基礎設施。`sector_agent_llm` 維持 experimental(等 L2.4 觀察窗口驗證) |
| `portprobe` | Stateless TCP port 探測 helper — Probe / LookupOccupant / IsFubonZombie / KillOccupant（wildcard `net.Listen` 無 side effect） | `State`, `Occupant`, `Probe()` |
| `portfolio` | Darwinian 權重管理（`[0.3, 2.5]`）+ FactorEngine（動能/價值/品質） | `Manager`, `FactorEngine` |
| `repository` | PostgreSQL 持久化 — `DualWriteRepository` | `DualWriteRepository` |
| `risk` | 風險管理 — `RiskManager`、VaR、宏觀回撤 | `RiskManager` |
| `storage` | 檔案儲存抽象 — 原子寫入、檔案管理 | — |
| `startup` | 一次性啟動期 preflight 檢查 — `Preflight(claims []PortClaim)` 包 portprobe.Probe + actionable error | `Preflight()`, `PortClaim` |

---

## E · Evolving（演進中）— 15 packages

核心模組，由 stable 模組間接使用，API 可能仍在調整。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `autobacktest` | 自動回測 — 定時背景回測任務 | `Runner` | 由 daily monitor pipeline 使用 |
| `domain/shared` | 跨模組純計算函式（Sharpe、Sortino、頻率常數）— canonical 位置，呼叫模組以 type alias re-export 維持向後相容 | `ComputeSharpe`, `ComputeSortino`, `Frequency` | 由 `domain/AGENTS.md:30` 規範；`portfolio`/`reporting` 為薄 wrapper |
| `backtest` | 視窗回測 — `Window.Run()` | `Runner` | 由 autobacktest 使用 |
| `db` | PostgreSQL 連線管理 | `DB` | 基礎設施，穩定但未直接出現於 main.go |
| `eval` | 模型評估指標與可解釋性工具（SK-12~15） — OOS R²、Sharpe、PermutationImportance、PDP | `EvalResult`, `Predictor` | 由 robustness 使用，Fin-Skills 驅動 |
| `feature` | 命名特徵萃取（close, volume, return_1d/5d, hl_ratio, ma_ratio, volume_ratio） — 由 `cmd/backtest-pipeline` 和 `internal/experiment` 共用 | `Registry`, `MakeExtractor`, `ForwardReturnLabel` | 由 backtest-pipeline CLI 和 Judge 重要性運算使用 |
| `fubonproxy` | Fubon-proxy 生命週期管理 — 自動啟動/停止/監控 Python FastAPI 微服務 | `ProcessManager`, `Start()`, `Stop()` | 由 cmd/atlas API 模式使用，非致命失敗 |
| `globalmarket` | 全球總經資料管理 | `Manager` | 由 narrative/industry 使用 |
| `metalearning` | 元學習協調器 — `MetaLearner`、策略選擇優化 | `MetaLearner` | 研究階段，可能晉升 |
| `prism` | Regime-specific 訓練佇列（5 種 regime） | `Queue` | 核心模組，indirect import |
| `realtime` | 即時資料轉接器 — `RealTimeAdapter` | `RealTimeAdapter` | 核心模組，indirect import |
| `screener` | 宣告式個股篩選 — P/E、P/B、股息率、動能、成交量 | `Screener` | 由 orchestrator executors 使用 |
| `sim` | 模擬引擎 — 部位狀態轉換、`Engine.RunSymbol()` | `Engine` | 核心模組，indirect import |
| `spawning` | Agent 生成管理 — `SpawningManager`、`PerformSpawningCycle()` | `SpawningManager` | 核心模組，indirect import |
| `strategy` | 策略選擇器與登錄 | `Selector` | 由 orchestrator 使用 |
| `tax` | 台灣稅務計算 — `TaiwanTaxCalculator` | `TaiwanTaxCalculator` | 由 sim 使用 |
| `monitoring/api/dashboard` | Dashboard management center handlers — 資料通道、管線、通道控制、API 金鑰管理 | `Handlers` | 由 monitoring 使用 |
| `monitoring/api/events` | SSE 事件串流 — 轉發 eventbus 事件到 dashboard 客戶端，含 narrative 與 promotion 事件 catch-up 緩衝 | `SSEHandler` | 由 monitoring 使用 |
| `monitoring/api/prism` | PRISM training-results API — 暴露 `[]prism.CompletedTrainingResult`（regime-specific 訓練結果）給 dashboard 頁面 | `HandleTrainingResults` | 由 monitoring 使用，evolving |
| `ml` | 監督式學習模型 — OLS、ElasticNet、PCR、PLS 實作（SK-05~09） | `Model`, `Trainer` | 由 Fin-Skills 規範驅動，供 factor/research 使用 |
| `scheduler` | ML 模型重訓排程器 — 定時從 replay 資料重訓 OLS/ElasticNet/PCR/PLS | `MLRetrainScheduler`, `RetrainAll()`, `GetLatestModel()` | 由 BackgroundTaskManager 排程，evolving |
| `observability` | OpenTelemetry 追蹤基礎設施 — 父 package，下含 `otel` 子 package（OTel SDK init + span helpers） | `TracerName`（re-exported from `otel`） | Wave 10 L2.1，evolving，OTLP exporter 待 production 評估 |
| `observability/otel` | OpenTelemetry trace 初始化 — stdout exporter、parent/child span helpers，給 `llm.router.Call` 與 `llm/clients.DoRequest` 加 span | `Init()`, `StartSpan()`, `TracerName` | Wave 10 L2.1，evolving，OTLP exporter 待 production 評估 |
| `acceptance` | Acceptance gate pluggable 框架 — `Evaluator`/`Pipeline`/`Registry` 介面，給 `experiment/judge.go` 從 hard-coded switch 漸進遷移（bridge feature flag） | `Evaluator`, `Pipeline`, `Registry`, `FuncEvaluator` | Wave 10 L2.2，evolving |
| `acceptance/builtin` | 17 個 acceptance gate evaluators 實作 — `ImproveSharpeLike`/`PreserveDownsideProtection`/`NoDrawdownSpike`/`FactorWeightStability`/`RetailSentimentFilter`/`NoMaterialDrawdownDegradation`/`NoConstraintBypass`/`MaintainSharpeLike`/`ReduceConcentrationRisk`/`FactorQuality`/`ReduceFalsePositiveRate`/`MaintainCROAuthority`/`ReduceSectorBlindspots`/`MaintainIndustryCoverage`/`ReduceStyleDrift`/`MaintainMomentumCatch`/`RespectHoldingPeriod` | ... | Wave 10 L2.2，evolving，17/17 gates ported |

---

## X · Experimental（實驗中）— 12 packages

研究性質模組，API 不穩定，不應被 stable/evolving 模組依賴。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `adversarial` | 對抗性訓練 — `AdversarialTrainer`、`BattleResult`、`StressTest` | `AdversarialTrainer` | 探索性研究 |
| `reflexivity` | 自反性價格動態引擎 | `Engine` | 探索性研究 |
| `retail` | RSI-tw 散戶情緒指數 — 複合零售情緒指標（保證金、VIX、機構流向） | `Calculator` | Phase 1 基準實作，Phase 2 擴充中 |
| `robustness` | 穩健性與敏感度測試（SK-20~22） — SizeGroup、PennyExclusion、Ablation | `Model`, `SizeGroupReport` | Fin-Skills 驅動，實驗中 |
| `stress` | 壓力測試場景 — `RunScenario()` | — | 情境模擬 |
| `swarm` | MiroFish swarm 模擬 + GARCH 波動率 + 策略進化 + API + Agent Skill | `Swarm` | 演進中 |
| `sectorallocation` | 產業權重單一權威 — 統一三路計算（industry/portfolio/monitoring）為多因子引擎（base × cycle × seasonal × linkage × narrative × macro × factor） | `WeightEngine`, `ComputeWeights()`, `ComputeWeight()`, 6 `InputProvider` adapters | 取代硬編碼 12 個 switch case；deprecated: `monitoring/service.calculateWeightDerivation` |
| `alerting` | Alertmanager webhook receiver — 接收 Alertmanager firing/resolved 警報，in-memory ring buffer 保留最近 1000 筆供 SSE/UI 消費 | `AlertWebhookHandler`, `AlertmanagerPayload`, `AlertmanagerAlert` | 掛載於 `/api/v1/alerts`；待 Prometheus alertmanager targets 與 docker-compose alertmanager service 補齊後晉升 evolving |
| `llm` | LLM 多 Provider 統一介面 — 路由器、能力調度、DataClass 閘門、備援鏈，健康端點 | `ProviderImpl`, `DefaultRouter`, `Capability`, `DataClass` | Wave 11 L2.1：effective routing chain 為 3 層（Primary → Backup1 → LastResort）；`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 為 `[PLANNED]` 常數，無 client 實作（Issue #720）。**Phase 2 canonical 介面（Issue #722）**：Phase 2 canonical 介面已就緒（`adapters` + `capabilities`），承接 `llm_annotator` 的角色。詳見 `docs/llm-integration-strategy-framework.md` |
| `llm/schemas` | LLM 能力輸入/輸出結構合約 — 9 個 capability 的 typed I/O 結構，JSON 序列化 | `RationaleGenerationResponse`, `StrategySummaryResponse`, `PromptLintResponse`, `ScenarioSimulationResponse`, `RiskSurfaceExtractionResponse`, `RegimeExplanationResponse`, `PerformanceForensicsResponse`, `CodeReviewAnnotationResponse`, `SentimentExplanationResponse` | Phase 2 為 9 個 capability handlers 提供型別安全 contract；handler 端用 `json.Marshal/Unmarshal` 對接 Router |
| `llm/clients` | LLM Provider HTTP 客戶端 — DeepSeek V4、MiniMax M3 + 共享 `BaseClient`（retry / rate limit / circuit breaker） | `BaseClient`, `DeepSeekClient`, `MiniMaxClient`, `Message`, `ChatOptions`, `ChatResponse` | Phase 2 新增；MiniMax 附中國國家安全法資料主權警告 |
| `llm/capabilities` | LLM 能力處理器 — 10 個 capability handler（failure_attribution + 9 個新），每個封裝 prompt template + schema-typed I/O + Router 呼叫 | `FailureAttributionHandler`, `RationaleGenerationHandler`, `StrategySummaryHandler`, `PromptLintHandler`, `ScenarioSimulationHandler`, `RiskSurfaceExtractionHandler`, `RegimeExplanationHandler`, `PerformanceForensicsHandler`, `CodeReviewAnnotationHandler`, `SentimentExplanationHandler` | Phase 2 從 1 個擴充至 10 個；Kimi K2.7 已移除（coding plan key 限制 CLI 工具，不可用於 app-level 呼叫） |
| `mcp/anomaly` | MCP audit event 異常偵測 — rolling-window z-score、per-tool/per-tenant error-rate、in-memory ring buffer | `Detector`, `Store`, `AnomalyEvent` | Wave 11 Phase 4 Direction A：僅供 `cmd/atlas-mcp` 消費，不應被其他 stable/evolving 模組依賴 |

---

## A · Archived（封存）— 1 package

已被 Phase 2 canonical 取代；API frozen，僅接受 bug fix 維護；新程式碼禁止依賴。

| Package | 描述 | 關鍵型別/介面 | 封存原因 |
|---------|------|--------------|----------|
| `llm_annotator` | LLM 歸因標註 — 自然語言解釋 StrategyFrame 失效原因（Kimi/Moonshot API） | `Annotator`, `KimiClient`, `MockAnnotator`, `FailureContext`, `CircuitBreaker`(wrapper), `CircuitState`(alias) | **Wave 11 L2.1（Issue #722）**：Phase 2 canonical 介面（`internal/llm/capabilities/failure_attribution` + `internal/llm/adapters`）已就緒，承接本套件的角色。**Wave 12 Phase 2（Issue #731）**：CircuitBreaker 統一為 `apigateway.CircuitBreaker` canonical owner；本套件保留 `CircuitBreaker` thin wrapper 委派（含 `CircuitState = apigateway.State` 型別別名、`ErrCircuitOpen` sentinel、`Allow()`/`Snapshot()`/`WithNowFunc()`）；4 層 transitive import cycle 已破壞（`monitoring.ChannelHealthStore` 等搬到 `apigateway`）。需 `LLM_ANNOTATOR_API_KEY` 環境變數（透過 apigateway `config.GetSecret` 取得），opt-in 啟用（空時 `/api/strategies/{id}/annotate` 回 503）。 |

---

## U · Utility（輔助工具）— 7 packages

CLI 工具、資料轉換、一次性驗證。非 runtime 一部分。

| Package | 描述 | 關聯 CLI 入口 | 備註 |
|---------|------|--------------|------|
| `importer` | CSV → JSONL 資料匯入 — TWSE、FinMind | `cmd/import-replay` | — |
| `paramcheck` | 驗證 JSON 參數樹中的 ParameterMetadata 校正證據 | `cmd/validate-parameters` | 由 cmd/validate-parameters 使用，非 runtime |
| `replay` | TWSE CSV 載入與 forward return 計算 | 由 experiment 使用 | 工具層，非 runtime |
| `reporting` | 報告生成 — Markdown、ASCII chart、Agent 績效表 | — | 被其他模組呼叫 |
| `taskexec` | 非同步任務執行器 — `Manager`、`Cancel/Subscribe` | — | 輔助基礎設施 |
| `calibration` | 參數校準純邏輯 — GARCH/VaR/Darwinian/Factor 推斷 | `cmd/calibrate-parameters` | 工具層，非 runtime |
| `risktest` | 風險測試場景 — `RunScenario()` | `cmd/stress-test` | 由 orchestrator 測試使用 |
| `backfill` | 一次性 ledger state 修復工具 — 孤兒 summary.json 補寫、dry-run、安全 idempotent | `cmd/backfill-summaries` | 工具層，非 runtime |

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
