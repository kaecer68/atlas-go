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

---

## S · Stable（穩定生產）— 21 packages

所有直接由 `cmd/atlas/main.go` 匯入的模組，處於生產執行路徑中。

| Package | 描述 | 關鍵型別/介面 |
|---------|------|--------------|
| `apigateway` | API Gateway — 資料源統一入口、Rate Limiter、通道管理、背景任務排程（憲法規範） | `Gateway`, `BackgroundTaskManager`, `Fetch()` |
| `baseline` | Baseline policy 版本控制與升降級 | `Policy`, `Promote()`, `Revert()` |
| `bootstrap` | 系統初始化 — HTTP 路由、Dashboard 註冊 | `Bootstrap()` |
| `config` | 環境變數讀取（`ATLAS_*` 前綴）、參數配置管理 | `Config`, `ParametersConfig` |
| `domain` | 領域型別 — canonical types、string enum（全系統依賴） | `Regime`, `Recommendation`, `Position` |
| `eventbus` | 事件匯流排 — Publish/Subscribe | `ChannelEventBus` |
| `eventlogic` | 事件邏輯 — 系統事件處理規則 | — |
| `experiment` | 實驗生命週期 — mutation → execute → judge → promote/revert | `Executor`, `Judge`, `Candidate` |
| `industry` | 產業生態系 — 供應鏈連動、季節性模式、週期羅盤 | `SupplyChainGraph`, `SeasonalEngine`, `CycleTracker` |
| `janus` | JANUS 跨 cohort regime 偵測與 PRISM 權重動態調整 | `Detector` |
| `ledger` | JSONL append-only 持久化 — outcomes/scorecard | `LoadOutcomes()`, `RecordSessionSummary()` |
| `live` | 即時交易 — broker execution、context 統一、原子寫入（需 flag 啟用） | `Orchestrator` |
| `logging` | 統一日誌介面 — Info/Error 系列 | `Logger` |
| `marketdata` | 資料提供者抽象 — TWSE OpenAPI、Fugle、Hybrid | `Provider`, `HybridProvider` |
| `monitoring` | 監控 API 與 Dashboard — 115 handlers、200 symbols | `Server` |
| `narrative` | 宏觀敘事事件偵測 — 因果鏈、台灣壓力指數 | `NarrativeEvent`, `Ingestor` |
| `orchestrator` | 核心協調層 — `SystemCore`、`PluginHost`、三層 executor 路由 | `SystemCore`, `PluginHost` |
| `portfolio` | Darwinian 權重管理（`[0.3, 2.5]`）+ FactorEngine（動能/價值/品質） | `Manager`, `FactorEngine` |
| `repository` | PostgreSQL 持久化 — `DualWriteRepository` | `DualWriteRepository` |
| `risk` | 風險管理 — `RiskManager`、VaR、宏觀回撤 | `RiskManager` |
| `storage` | 檔案儲存抽象 — 原子寫入、檔案管理 | — |

---

## E · Evolving（演進中）— 14 packages

核心模組，由 stable 模組間接使用，API 可能仍在調整。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `autobacktest` | 自動回測 — 定時背景回測任務 | `Runner` | 由 daily monitor pipeline 使用 |
| `backtest` | 視窗回測 — `Window.Run()` | `Runner` | 由 autobacktest 使用 |
| `db` | PostgreSQL 連線管理 | `DB` | 基礎設施，穩定但未直接出現於 main.go |
| `eval` | 模型評估指標與可解釋性工具（SK-12~15） — OOS R²、Sharpe、PermutationImportance、PDP | `EvalResult`, `Predictor` | 由 robustness 使用，Fin-Skills 驅動 |
| `feature` | 命名特徵萃取（close, volume, return_1d/5d, hl_ratio, ma_ratio, volume_ratio） — 由 `cmd/backtest-pipeline` 和 `internal/experiment` 共用 | `Registry`, `MakeExtractor`, `ForwardReturnLabel` | 由 backtest-pipeline CLI 和 Judge 重要性運算使用 |
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
| `ml` | 監督式學習模型 — OLS、ElasticNet、PCR、PLS 實作（SK-05~09） | `Model`, `Trainer` | 由 Fin-Skills 規範驅動，供 factor/research 使用 |
| `scheduler` | ML 模型重訓排程器 — 定時從 replay 資料重訓 OLS/ElasticNet/PCR/PLS | `MLRetrainScheduler`, `RetrainAll()`, `GetLatestModel()` | 由 BackgroundTaskManager 排程，evolving |

---

## X · Experimental（實驗中）— 5 packages

研究性質模組，API 不穩定，不應被 stable/evolving 模組依賴。

| Package | 描述 | 關鍵型別/介面 | 備註 |
|---------|------|--------------|------|
| `adversarial` | 對抗性訓練 — `AdversarialTrainer`、`BattleResult`、`StressTest` | `AdversarialTrainer` | 探索性研究 |
| `reflexivity` | 自反性價格動態引擎 | `Engine` | 探索性研究 |
| `robustness` | 穩健性與敏感度測試（SK-20~22） — SizeGroup、PennyExclusion、Ablation | `Model`, `SizeGroupReport` | Fin-Skills 驅動，實驗中 |
| `stress` | 壓力測試場景 — `RunScenario()` | — | 情境模擬 |
| `swarm` | MiroFish swarm 模擬 + GARCH 波動率 + 策略進化 + API + Agent Skill | `Swarm` | 演進中 |

---

## U · Utility（輔助工具）— 4 packages

CLI 工具、資料轉換、一次性驗證。非 runtime 一部分。

| Package | 描述 | 關聯 CLI 入口 | 備註 |
|---------|------|--------------|------|
| `importer` | CSV → JSONL 資料匯入 — TWSE、FinMind | `cmd/import-replay` | — |
| `replay` | TWSE CSV 載入與 forward return 計算 | 由 experiment 使用 | 工具層，非 runtime |
| `reporting` | 報告生成 — Markdown、ASCII chart、Agent 績效表 | — | 被其他模組呼叫 |
| `taskexec` | 非同步任務執行器 — `Manager`、`Cancel/Subscribe` | — | 輔助基礎設施 |

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
4. **移除模組**：若模組被廢棄，先標記為 X 一週後再刪除目錄。
