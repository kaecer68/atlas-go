# 決策：PRISM 重啟評估 + ml_retrain 開關 (2026-08-21)

> 狀態：B5 評估完成 — **PRISM 維持現狀（已啟用且運作中，無需重啟/修碼）；ml_retrain 維持 disabled（鎖定決策）**
> 依據：`docs/decisions/2026-08-21-performance-root-cause-audit.md` §2.5 + Phase B/C 執行方案設計 §B5
> 方法：源碼逐行 + production（iMac `atlas-go-imac` container）logs/task_liveness 複核 + 本機以 production registry/parameters/replay 複製執行 145 筆 training tasks 驗證

## 結論摘要

| 項目 | 決策 | 理由 |
|------|------|------|
| PRISM training | **維持啟用（不重啟、不改碼）** | Phase A 已接線並運作：executor wired、workers 啟動、BTM 6h 排程、JANUS 消費鏈完整；本機複製驗證 145 tasks → 40 completed（真 cohort），無污染 |
| PRISM 29/0 | **stale artifact（2026-07-06 MacBook 殘留檔）** | production 無該檔；`SaveMetrics` 無任何 production caller → 檔案根本沒人寫 |
| PRISM HighVol/Transition/LowVol cohort | **已知限制，記錄觸發條件** | `inferRegime` 只輸出 RISK_ON/RISK_OFF/NEUTRAL；96 日資料 61/35/0 → LowVol 永不產出、HighVol/Transition 結構上不可達 |
| ml_retrain | **維持 disabled（鎖定決策，避免重複評估）** | 消費端仍不存在：`UseMLScoring=false`（prod）、`MLScorer.Train()` 零 caller、`LoadModelState` 零 caller；特徵仍為 OHLCV 統計量（D2 要求重設計） |

## 1. PRISM 29 queued / 0 completed 根因

### 1.1 該數字是 2026-07-06 的 stale snapshot，非現況
- `~/workspace/atlas/data/state/phase3_metrics.json`（MacBook main worktree）：`recorded_at: 2026-07-06T18:29:32Z`，schema 含已移除的 `swarm_running/swarm_consensus_symbols`（PR #964 移除 swarm）。
- iMac production（`/Users/kk/workspace/atlas/data/state/`）：**無 phase3_metrics.json**；`GET /api/dashboard/phase3-status` 回 zero-value（`prism_queued_tasks:0 ... recorded_at:"0001-01-01"`）。
- 源碼：`internal/orchestrator/phase3_metrics.go` `SaveMetrics` **零 production caller**（僅 `seed_smoke_data.sh` 寫空檔）；`docs/data-catalog.md` 標的 producer `internal/monitoring/phase3_metrics.go` 已不存在（文件過時）。

### 1.2 2026-07 當時的根因（Phase A 前）：executor 未接 + worker 未啟動
- `cmd/atlas/main.go` 註解（L622-629）自述：production apiMode 路徑 `prismMgr` 沒接 `WithExecutor` → `prism_training` BTM 拿到 `executor=nil` → `executeTraining` 回 `{Synthetic:true, Error:"no training executor configured"}` → worker 計為 failed（`prism_manager.go` `executeTraining`/`worker`）。
- 舊 phase3_metrics 由 `Phase3Controller`（factory `pm`，未接 executor）統計 → 29 queued（in-memory 排隊未被消化）、0 completed。

### 1.3 現況（Phase A 後，production 實測 2026-08-21）
- container 啟動 log：`[PRISM] training executor wired (replay-backed)` + `prism_manager started regime_queues=5` + `prism_training background task registered (6h interval, replay-backed executor)` + `prism_auto_balancer (5m)`。
- `task_liveness`：`prism_training consecutive_failures=0`、`prism_auto_balancer 0 failures`（每 5m 正常 tick）。
- 消費鏈完整：`operations_tasks.go` `SetOnCompleted` → `janusEngine.RecordTrainingResult`（真結果即時餵 JANUS）；6h tick 清點 + `ClearCompletedResults`；JANUS `Update()` → cohort weights → `ApplyAdjustment`（`internal/janus/engine.go`、`plugin_adapters.go`）。
- `runPrismWorker`（standalone `prism worker` daemon）**無 executor**（Synthetic 護欄）→ 但 production **未部署**此服務（container cmd=`atlas-go -api`；`docker ps` 無 prism-worker container）→ 不影響現況。

## 2. PRISM 產出驗證（本機複製 production 條件）

以 production 完全一致之輸入複製 BTM 排程（29 agents × 5 regimes = 145 tasks，window=30d）：

- registry：`configs/agents.json`（md5 與 iMac 一致 `cc7108a3…`，29 enabled agents）
- replay：iMac production `tw_extended_90days.csv`（96 交易日 × 41 檔，2026-05-17→08-21）
- parameters：production `configs/parameters.json`
- policy：`baseline.DefaultPolicy()`（iMac 無 `data/state/baseline_policy.json`，同 production）

結果：

| regime | completed | 原因 |
|--------|-----------|------|
| Risk-On | 20 | 61 日 RISK_ON 樣本 |
| Risk-Off | 20 | 35 日 RISK_OFF 樣本 |
| High-Vol | 0 | `inferRegime` 結構上不輸出此 regime |
| Low-Vol | 0 | 96 日內 **0 個 NEUTRAL** 日 |
| Transition | 0 | `inferRegime` 結構上不輸出此 regime |

- 145 tasks 總耗時 ~1.3s（10 workers）→ CPU 成本可忽略。
- 失敗為 by-design：「no outcomes for agent in window」（`prism_executor.go` `Run`）— 窗口內無該 regime 日期或該 agent 無 rec；失敗不寫任何狀態（無污染）。
- **40 筆 completed 為真 cohort 指標**（ledger scorecards：HitRate/SharpeLike/MaxDrawdown…），非 Synthetic。

## 3. 決策

### 3.1 PRISM：維持啟用，不需重啟、不需修碼
- 決策樹對應：非「executor 未 wire」、非「Synthetic 護欄 + 無真 cohort」→ 已是「executor wired + workers 跑 + 真 cohort 產出」狀態。
- 「重啟」在 Phase A 已發生（executor 接線 + BTM 註冊）；B5 驗證其運作正確。
- **不改碼**（B5 最小改動原則）。

### 3.2 已知限制（記錄，非 bug）
- `inferRegime`（`internal/orchestrator/executor_regime.go`）僅輸出 RISK_ON/RISK_OFF/NEUTRAL；PRISM 5 regime 中 HighVolatility/Transition 結構上不可達，LowVolatility 僅在出現 NEUTRAL 日時可達（本資料集 0 日）。
- 影響：6h tick 的 105/145 任務必然失敗（無狀態污染，僅 failed 計數）。JANUS 只收到 RiskOn/RiskOff cohort。
- **觸發條件（再評估）**：當 `inferRegime`（或接 JANUS `RegimeDetector` 的 5-regime 判定）能輸出 HighVol/Transition，或資料集出現 NEUTRAL 日 → 重開 B5 評估是否補齊 cohort；在此之前不硬開（避免無意義的 failed 計數噪音）。

### 3.3 ml_retrain：維持 disabled（鎖定決策）
- 消費端不存在（證據 A）：
  - `UseMLScoring`：default false（`defaults_engine.go`），production parameters.json `value:false`，todo「Validate with A/B backtest before enabling」。
  - 即使開啟：`executor_pipeline.go` L26-28 只 `WithMLScorer(NewMLScorer(ml.NewOLS()))`（未訓練）；`MLScorer.Train()` **零 production caller** → `IsTrained()=false` → `plugin_registry.go` L399 跳過 ML scoring。
  - `scheduler.LoadModelState`（讀 ml_models/*.json）**零 caller** → 就算 ml_retrain 產出模型檔也無人載入。
  - iMac `data/state/ml_models/` 4 檔（ols/elasticnet/pcr/pls，trained 2026-08-17）→ 證明曾跑過一次，但無消費。
- 特徵設計（D2 註解）：現行 `extractFeatures` 為 OHLCV z-score + MA 比值；D2 要求七維錢潮/stress-index/regime 特徵重設計 → 維持不啟用。
- **鎖定**：本決策寫入文件，避免每次評估週期重複調查。
- **觸發條件（重啟評估）**：(a) ML 消費端實作完成（`MLScorer.Train()` 以設計後特徵於 init 呼叫，或 `LoadModelState` 接入 scoring 路徑）；(b) `UseMLScoring` 經 A/B 回測驗證（parameters.json todo）；(c) 特徵重設計依 D2 完成。三項齊備才重新評估啟用。

## 4. 驗收對照（B5 驗收標準）

- task_liveness：`prism_training consecutive_failures=0` ✅（production 實測）；`ml_retrain` 無 row（disabled 未執行，符合「明確的 skip 原因」= D2 鎖定）✅
- 產出：PRISM 每 6h tick 產 40 筆真 cohort 結果並餵 JANUS ✅（本機複製驗證；production 無法直接內視 in-memory manager，以 log + liveness 佐證）
- 決策文件：本文件 ✅

## 5. 改動清單

- 本次 B5 無 .go 改動（唯讀評估 + 決策文件）→ 無測試變更。
- 檔案：本決策文件（docs-only）。

## 參考（符號名，行號僅供參考）

- `cmd/atlas/main.go`：`buildPrismTrainingExecutor`、`runPrismWorker`、ml_retrain 註冊區塊（D2 註解）
- `cmd/atlas/operations_tasks.go`：`prism_training` BTM（`SetOnCompleted`/`ScheduleTraining`）
- `internal/prism/prism_manager.go`：`PRISMManager.Start/Stop/worker/executeTraining/ScheduleTraining`
- `internal/orchestrator/prism_executor.go`：`PRISMTrainingExecutor.Run`、`mapDomainRegimeToPRISMTrainingRegime`
- `internal/orchestrator/executor_regime.go`：`inferRegime`
- `internal/orchestrator/phase3_metrics.go`：`Phase3Metrics/SaveMetrics/CollectMetrics`
- `internal/orchestrator/factory.go`：第二個 PRISMManager（factory `pm`，`NewPhase3Controller` 用）
- `internal/orchestrator/executor_pipeline.go`：`ExecuteWithContext`（UseMLScoring 分支）
- `internal/orchestrator/plugin_registry.go`：`WithMLScorer`、`IsTrained` gate
- `internal/orchestrator/ml_scorer.go`：`MLScorer.Train/Score/IsTrained`
- `internal/scheduler/ml_retrain.go`：`MLRetrainScheduler.RetrainAll/LoadModelState`
- `internal/config/parameters_load.go`：`LoadParametersConfig`
- `internal/janus/engine.go`：`RecordTrainingResult/Update`
