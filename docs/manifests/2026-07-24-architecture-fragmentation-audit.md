# Audit Manifest: atlas-go 架構碎片化審計

> **Audit source**: 2026-07-24 系統性 ACI 架構盤查（codebase-memory + codegraph explore）
> **Goal**: 辨識結構碎片化、協調斷裂、重複碼，建立可追蹤的修復路徑
> **Scope**: `internal/` 下 80 packages 的模組粒度、協調機制、事件消費完整性
> **Created**: 2026-07-24
> **Status**: in-progress

---

## Invariant Table

| ID | 問題 | 證據 | 分類 | 決策 | 優先級 |
| **F-01** | `strategy_validator` 獨立套件無必要 | `strategy_validator/` 只被 `strategy_ranker/` import，無獨立邊界 | 結構碎片 | merge → `strategy_ranker/`（`strategy_techniques/` 和 `strategy/` 的 `Registry` 命名衝突，保留分離） | P1 |
| **F-02** | `narrative/` 65 檔案單一套件 | Leiden 叢集 #139 凝聚力 0.783（全專案最低）；detector/calibration/seasonal/geopolitical 混合 | 結構碎片（過大） | split: 拆為子套件 | P1 |
| **F-03** | `system.go` / `system_dispatcher.go` 7 處重複 Publish | PublishSimulationStart/RegimeChange/Recommendation/GuardOutcomes/DarwinianClamping/SimulationComplete — 兩個檔案中幾乎相同的 7 段程式碼 | 重複碼 | extract: 抽取共用方法 | P1 |
| **F-04** | `live/` 與 `orchestrator/` 雙重執行引擎 | live.Orchestrator 和 orchestrator.System 是兩套完全獨立的實作，共享 EventBus/domain 但不共享執行抽象 | 協調斷裂 | design: 抽取 SessionRunner 介面 | P2 |
| **F-05** | eventbus 孤兒事件 | 詳見 `docs/manifests/2026-07-24-eventbus-orphan-audit.md` — 10 事件分類，7 已修復，3 backlog | 事件缺口 | ✅ 大部分已修復 | — |
| **F-06** | 微套件過多 | `paramcheck/`, `portprobe/`, `robustness/`, `risktest/`, `forecast/` + `forecast_bridge/` — 功能單一但獨佔套件 | 結構碎片 | merge: 合併到消費套件 | P2 |
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
| F-01 | merge `strategy*` → `strategy/` | 4 套件無循環依賴、無獨立公開 API 消費者 |
| F-02 | split `narrative/` → 4 子套件 | 凝聚力最低，detector/calibration/seasonal/geopolitical 獨立 |
| F-03 | extract `publishSessionEvents()` | system.go:369,479,530,555,661,673 集中到一個方法 |
| F-04 | defer P2 | 需要設計 SessionRunner 介面，影響兩條執行路徑 |
| F-06 | merge 微套件 | `paramcheck`→`config`, `portprobe`→刪除, `robustness`→`portfolio` |
| F-07/F-08 | document | 補 doc.go，不改變程式碼 |

### Phase C — Implement

| PR | IDs | Scope | Status |
|---|---|---|---|
| PR #1 | F-01 | 合併 `strategy_ranker/validator/techniques` → `strategy/` | pending |
| PR #2 | F-03 | 抽取 `publishSessionEvents()` 消除 system.go/system_dispatcher.go 重複 | pending |
| PR #3 | F-02 | 拆分 `narrative/` → 子套件 | pending |
| PR #4 | F-06 | 合併微套件 | pending |

---

## Backlog

| ID | Problem | Proposed Round |
|---|---|---|
| F-04 | live/orchestrator 雙引擎共用抽象 | Phase 2 |
| F-07 | eventbus/eventdriven/eventquality doc.go | Phase 2 |
| F-08 | llm/llm_annotator doc.go | Phase 2 |

---

## Commit Discipline

- Format: `refactor(manifest): #F-XX <short description>`
- One commit per ID
- PR body: `See docs/manifests/2026-07-24-architecture-fragmentation-audit.md`

---

## Session-End State

- **Done this session**: Phase A (audit), Phase B (plan), 兩份 manifest 建立完成
- **Remaining**: Phase C (implement F-01/F-02/F-03/F-06)
- **Next action**: F-01 — 合併 strategy* 套件
- **Uncommitted code**: yes (manifests + eventbus payloads/publish methods + SSE buffers/subscriptions)
- **Branch / PR**: TBD
