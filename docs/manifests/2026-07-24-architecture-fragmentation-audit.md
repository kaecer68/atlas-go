# Audit Manifest: atlas-go 架構碎片化審計

> **Audit source**: 2026-07-24 系統性 ACI 架構盤查（codebase-memory + codegraph explore）
> **Goal**: 辨識結構碎片化、協調斷裂、重複碼，建立可追蹤的修復路徑
> **Scope**: `internal/` 下 80 packages 的模組粒度、協調機制、事件消費完整性
> **Created**: 2026-07-24
> **Status**: in-progress

---

## Invariant Table

| ID | 問題 | 證據 | 分類 | 決策 | 優先級 |
| **F-01** | `strategy_validator` 獨立套件無必要 | `strategy_validator/` 已於 PR #1311 合併至 `strategy_ranker/`（validator.go/validator_test.go/validator_doc.go 已搬移）；殘留文件引用待清理 | 結構碎片 | ✅ closed: 合併已完成，文件引用已清理 | P1 |
| **F-02** | `narrative/` 65 檔案單一套件 | Leiden 叢集 #139 凝聚力 0.783（全專案最低）；detector/calibration/seasonal/geopolitical 混合 | 結構碎片（過大） | split: `detector`→`narrative/detector` ✅ safe；`calibration`→`narrative/calibration` ✅ safe；`seasonal` ❌ blocked（非連貫分界：SeasonalBridge 依賴 NarrativeEngine/industry，SeasonalAnalyzer 僅 40 行純數學，lifecycle 屬事件狀態而非季節） | P1 |
| **F-03** | `system.go` / `system_dispatcher.go` Publish 重複 | `PublishSimulationStart`、`PublishRegimeChange`（含 factor engine sync）、`PublishRecommendation` 在 `RunDailySimulation` 與 `runReplaySimulation` 中重複；`publishSessionClose` 已抽但還有 3 組 | 重複碼 | ✅ extract: 抽取 `publishSimulationStart` / `publishRegimeChange` / `publishRecommendation` | P1 |
| **F-04** | `live/` 與 `orchestrator/` 雙重執行引擎 | live.Orchestrator 和 orchestrator.System 是兩套獨立但互補的實作，共享 EventBus/domain 但執行模型不同；已透過 `orchestrator.AdapterProducer` 復用模擬 `ExecuteWithContext` 作為 live 建議來源 | 設計意圖（非斷裂） | ✅ won't fix：記錄為雙引擎架構決策，補 doc.go 說明 | P2 |
| **F-05** | eventbus 孤兒事件 | 詳見 `docs/manifests/2026-07-24-eventbus-orphan-audit.md` — 10 事件分類，7 已修復，3 backlog | 事件缺口 | ✅ 大部分已修復 | — |
| **F-06** | 微套件過多 | `paramcheck/` 已併入 `internal/config`（PR #1318）；`risktest/` 已搬到 `cmd/stress-test/internal/risktest`；`robustness/`、`forecast_bridge/` 已於 PR #1311 移除；`portprobe/` 被 5+ 套件共用、`forecast/` 被 orchestrator + strategy 共用，非碎片 | 結構碎片 | ✅ closed: 已無單一消費者微套件 | P2 |
| **F-07** | 事件驅動相關套件分裂 | `eventbus/`, `eventdriven/`, `eventquality/` — 三個套件名稱暗示相關但無明確依賴邊界文件 | 命名誤導 | ✅ document: 三者獨立，補 doc.go 邊界說明 | P2 |
| **F-08** | `llm/` + `llm_annotator/` 關係不明 | 兩個 LLM 套件無共用介面或依賴圖 | 結構碎片 | ✅ document: `llm/adapters/` → `llm_annotator`，補 sub-packages 說明 | P2 |

---

## 證據摘要

### 總體數據

| 指標 | 數值 |
|---|---|
| Internal packages | 80 |
| 總函數節點 | 12,099 |
| 總邊（CALLS+USAGE） | 175,821 |
| Leiden 叢集 | 12 |
| 最低凝聚力叢集 | #139 (narrative, 0.783) |
| SIMILAR_TO 邊（Jaccard > 0.7） | 少量 clone 輔助函數 |

### 跨套件調用熱點 (Top 10)

| 來源 | 目標 | 調用數 |
|---|---|---|
| apigateway → marketdata | 261 |
| apigateway → logging | 178 |
| orchestrator → logging | 116 |
| orchestrator → portfolio | 111 |
| apigateway → monitoring | 104 |
| orchestrator → config | 81 |
| static → capitalflow | 61 |
| orchestrator → risk | 49 |
| orchestrator → marketdata | 47 |
| apigateway → narrative | 38 |

### 重複碼實例

| 函數 | 位置 |
|---|---|
| `cloneParams` / `clonePositions` / `cloneMap` | `config/inference.go`, `sim/engine.go`, `orchestrator/calibration_engine.go` |
| `parseStatusCodeCSV` / `toRetryableStatusCodeSet` | `bootstrap/broker_config.go`, `live/http_adapter.go` |
| `system.go` / `system_dispatcher.go` 7 處 Publish 重複 | `orchestrator/system.go`, `orchestrator/system_dispatcher.go` |

---

### 雙引擎設計說明（F-04 won't fix 記錄）

| 維度 | `orchestrator.System` | `live.Orchestrator` |
|---|---|---|
| 目的 | 研究/學習：批次評估策略對歷史資料的表現 | 執行：實際下單、管理即時部位 |
| 觸發 | 離線批次：每日一次 `RunDailySimulation` | 事件驅動：market open / intraday cycle / market close |
| 速度要求 | 可花數分鐘（factor engine + PRISM + JANUS） | 5 分鐘 intraday cycle 必須完成 |
| 狀態模型 | 完整歷史軌跡（portfolioHistory、returnHistory） | 當前狀態（positions、cash、dayPnL） |
| 訂單執行 | 無，僅產生 `SimulationResult` | 有，Broker / OrderManager / RiskGate |
| 協調橋樑 | `orchestrator.AdapterProducer` 將 `ExecuteWithContext(...)` 的輸出轉為 `domain.ExecutionInput` 餵給 live 路徑 | |

**判斷**：兩者並非重複實作，而是互補引擎。模擬路徑的輸出已經作為 live 路徑的建議輸入，不存在執行邏輯斷裂。強制抽取 `SessionRunner` 介面會讓 live 路徑拖帶研究型元件，或讓模擬路徑放棄學習 pipeline，因此維持分離。

**未來仍應注意**：若新增第三條執行路徑（例如 paper trading 或回測 replay），才需要重新評估是否引入共用執行抽象。

---

## F-02 Narrative 子套件拆分決策證據

> 基於 codegraph + 手動 import 追蹤的實證分析。

### 現狀

- `internal/narrative` 共 65 檔案，為全專案 Leiden 叢集凝聚力最低（0.783）。
- 已存在成功先例：`internal/narrative/geopolitical` 為獨立 leaf sub-package，無反向依賴 narrative root。

### 評估結果

| 子套件 | 狀態 | 檔案 | 依賴證據 |
|---|---|---|---|
| `narrative/calibration` | ✅ DONE | `weight_calibration.go`, `calibration_baseline.go`, `calibration_scales.go`, `calibration_regime.go`, `calibration_validation.go`, `incremental_validation.go` 與對應 tests；新設 `load_weights_config.go`, `stress_index_config.go`, `helpers.go` | 不引用 `NarrativeEvent` / `NarrativeEngine`；`StressIndexWeightsConfig` 等型別遷入 `narrative/calibration`；`taiwan_stress_index.go` 透過 `calibration_facade.go` 正向 import；無 circular import |
| `narrative/detector` | ❌ BLOCKED | `detector.go`, `detector_impls.go`, 對應 tests | `DetectorInput` 引用 `MarketNarrativeData`（定義於 `narrative_detectors.go`），且 `DetectionResult.ToNarrativeEvent()` 引用 `NarrativeEvent`。若要將 detector 抽為子套件，必須同時遷移 `MarketNarrativeData` 與 detect 函式本體，觸及 narrative 核心 pipeline，風險大於收益 |
| `narrative/seasonal` | ❌ BLOCKED | `seasonal_bridge.go`, `seasonal_analyzer.go`, `lifecycle.go` | 非連貫分界：`seasonal_bridge` 是 narrative→industry 的橋接器；`seasonal_analyzer` 僅 40 行純數學；`lifecycle` 屬事件狀態管理；合併拆分會造成單檔微套件或反向依賴 |

### 實作策略（已修訂）

1. **calibration 已拆**：將 calibration 相關檔案遷入 `internal/narrative/calibration/`，並以 `internal/narrative/calibration_facade.go` 保持 narrative 套件對外公開 API 不變。`LoadWeightsConfig`、`LoadBaselines`、`DefaultRegimeConfig` 等型別與函數維持可透過 narrative 套件存取，避免大規模更新消費者。
2. **detector 不拆**：經實證檢查，`detector.go` 與 `detector_impls.go` 並非只依賴 `marketdata`；它們透過 `DetectorInput.MarketData` 與 `narrativeEventToResult()` 緊密耦合 `MarketNarrativeData` / `NarrativeEvent`。強行拆分會導致 narrative↔detector circular import，或需遷移 detect 函式本體（核心 pipeline），超出本次重構範圍。
3. **seasonal 不拆**：理由同前。

---

## Phase Tracker

### Phase A — Audit (read-only) ✅ DONE

| Task | Status | Evidence |
|---|---|---|
| 取得整體架構 + Leiden 叢集 | ✅ | codebase-memory get_architecture |
| 追蹤 orchestrator System 建構 | ✅ | `internal/orchestrator/system.go:122`, `factory.go:38` |
| 追蹤 eventbus 事件/訂閱矩陣 | ✅ | 50+ types, 15 SSE + 5 monitoring |
| 追蹤 live/orchestrator 雙路徑 | ✅ | `live/orch.go:129` vs `orchestrator/system.go:122` |
| 跨套件調用熱點分析 | ✅ | query_graph CALLS edge aggregation |

### Phase B — Plan

| ID | 決策 | 理由 |
|---|---|---|
| F-01 | ✅ closed | `strategy_validator` 已於 PR #1311 合併至 `strategy_ranker`（validator.go/validator_test.go/validator_doc.go 搬移）；`strategy_techniques` 與 `strategy` 的 `Registry` 命名衝突保留分離 |
| F-02 | calibration → `narrative/calibration` ✅ DONE；detector / seasonal 暫不拆分（見 F-02 證據區） | 凝聚力最低；`geopolitical/` 已獨立；calibration 依賴鏈單向、無 circular import；detector 實際耦合 `MarketNarrativeData` / `NarrativeEvent`，強拆會造成 cycle；seasonal 非連貫分界 |
| F-03 | ✅ extract | 抽取 `publishSimulationStart`、`publishRegimeChange`（含 factor engine sync）、`publishRecommendation` 到 `system.go`；兩條執行路徑共用 |
| F-04 | ✅ won't fix | 雙引擎是設計意圖：orchestrator.System 負責批次研究/學習，live.Orchestrator 負責事件驅動執行；bridge 為 `orchestrator.AdapterProducer` |
| F-06 | ✅ closed | `robustness`/`forecast_bridge` 已移除（PR #1311）；`paramcheck` 併入 `config`（PR #1318）；`risktest` 搬到 `cmd/stress-test/internal/risktest`（PR #1319） |
| F-07/F-08 | document | 補 doc.go，不改變程式碼 |

### Phase C — Implement

| PR | IDs | Scope | Status |
|---|---|---|---|
| PR #1311 | F-06 部分 | 已移除 dead packages `robustness/`, `forecast_bridge/` | ✅ merged |
| PR #1315 | — | `cmd/backtest-window` SQLite 測試 hermetic | ✅ merged |
| PR #1316 | F-03 | 抽取 `publishSimulationStart`/`publishRegimeChange`/`publishRecommendation` | ✅ merged |
| PR #1320 | F-01 | 清理 `strategy_validator` 殘留文件引用 | ✅ merged |
| PR #1321 | F-02 | 拆分 `narrative/calibration` → 新子套件；`narrative/detector` 與 `narrative/seasonal` 維持原狀 | ✅ merged |
| PR #1324 | F-07 / F-08 / F-09 | 補充 doc.go 邊界說明 + docs-only fast-track CI | ✅ merged |
| PR #1318 | F-06 paramcheck | 合併 `internal/paramcheck` → `internal/config` | ✅ merged |
| PR #1319 | F-06 risktest | 搬移 `internal/risktest` → `cmd/stress-test/internal/risktest` | ✅ merged |

---

## Backlog

| ID | Problem | Proposed Round |
|---|---|---|
| F-02 follow-up | `narrative/detector` + `narrative/seasonal` 拆分仍 blocked | issue #1322 |
| F-03 | ✅ closed: Publish 重複區塊已抽取為 System helper methods | — |
| F-07 | ✅ closed: eventbus/eventdriven/eventquality 邊界說明已補充至各 doc.go | PR #1324 |
| F-08 | ✅ closed: `llm` 與 `llm_annotator` 封存/替代關係已補充至 doc.go | PR #1324 |
| F-09 | ✅ closed: docs/manifest 變更 fast-track CI（重型 job skip + docs-only-gate） | PR #1324 |

---

## Commit Discipline

- Format: `refactor(manifest): #F-XX <short description>`
- One commit per ID
- PR body: `See docs/manifests/2026-07-24-architecture-fragmentation-audit.md`

---

## Session-End State

- **Done this session**: `narrative/calibration` 子套件拆分完成，透過 `narrative/calibration_facade.go` 維持公開 API 相容；detector / seasonal 經實證判定為 blocked
- **Remaining**: Phase C 收尾（gofmt / go test / check-binaries / commit / PR）；F-07/F-08 doc.go 補充留待 Phase 2
- **Next action**: 提交 F-02 PR
- **Uncommitted code**: yes (`internal/narrative/calibration/` + `calibration_facade.go` + manifest 更新)
- **Branch / PR**: `fix/f02-narrative-detector-calibration-split` / TBD
