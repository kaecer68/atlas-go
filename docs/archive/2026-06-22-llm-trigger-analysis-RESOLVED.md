# LLM 掛載觸發分析報告

**日期**: 2026-06-20（初版）  
**更新**: 2026-06-22（Phase 4 閉環完成後標記為 RESOLVED）  
**分支**: `feature/llm-phase3-main-wiring` → 合併至 main（PR #630）  
**範圍**: 5 個 Phase 2 LLM 功能掛載（RationaleTranslator, ScenarioExplainer, RegimeExplainer/SentimentExplainer, PerformanceForensics, ConfidenceCommentary）

> **狀態更新（2026-06-22）**：Phase 4 LLM 閉環已實作完成。下方矩陣中標示為 **NO** 的項目已全數升級為 **RESOLVED**。具體接線位置見「第五節：Phase 4 閉環實作記錄」。

---

## 第一節：掛載啟用矩陣

| 掛載 | 模組 | 插入點 | 呼叫者路徑 | 目前是否觸發？ | 觸發條件 |
|------|------|--------|--------------|----------------|------------|
| **RationaleTranslator** | `narrative` | `rationale_corpus.go:228` | `GET /api/dashboard/recommendation-pipeline` → `pipeline/handlers.go:281,284` → `narrative.TranslateReason()` → passthrough seam | **✅ RESOLVED**（main.go 已接線） | `LLM_RATIONALE_TRANSLATION_ENABLED=true` → `cmd/atlas/main.go:1892` 賦值 → 既有 `TranslateReason()` 呼叫鏈自動觸發 |
| **ScenarioExplainer** | `orchestrator` | `prism_executor.go:115` | PRISM 訓練任務佇列 → `PRISMTrainingExecutor.Run()` → 計算 `TrainingResult` 後之 hook | **✅ RESOLVED**（main.go 已接線） | `LLM_PRISM_SCENARIO_ENABLED=true` → `cmd/atlas/main.go:1903` 賦值 → 既有 PRISM executor 呼叫鏈自動觸發 |
| **RegimeExplainer** | `narrative` | `explain_hooks.go:27` | `AnnotateEvent()` — **已新增生產環境呼叫者** | **✅ RESOLVED**（main.go + ingestor.go 雙重接線） | `LLM_NARRATIVE_EXPLAIN_ENABLED=true` → `cmd/atlas/main.go:1915` 賦值 → `internal/narrative/ingestor.go:139` 新增 `AnnotateEvent(ctx, e)` 呼叫 |
| **SentimentExplainer** | `narrative` | `explain_hooks.go:33` | `AnnotateEvent()` — **已新增生產環境呼叫者** | **✅ RESOLVED**（main.go + ingestor.go 雙重接線） | 同上 |
| **PerformanceForensics** | `risk` | `forensics_hook.go:21` | `AnnotateSnapshot()` — **已新增生產環境呼叫者** | **✅ RESOLVED**（main.go + system.go 雙重接線） | `LLM_RISK_FORENSICS_ENABLED=true` → `cmd/atlas/main.go:1937` 賦值 → `internal/orchestrator/system.go:521` 新增 `risk.AnnotateSnapshot()` 呼叫 |
| **ConfidenceCommentary** | `risk` | `confidence_hook.go:18` | `EnrichDecision()` — **已新增生產環境呼叫者** | **✅ RESOLVED**（main.go + gate.go 雙重接線） | `LLM_CONFIDENCE_COMMENTARY_ENABLED=true` → `cmd/atlas/main.go:1949` 賦值 → `internal/risk/gate.go:174` 內 `RiskGate.publish()` 呼叫 |

---

## 第二節：各掛載詳細分析

### 掛載 1：`narrative.RationaleTranslator`

**呼叫者函數/方法**：

| 檔案 | 行號 | 上下文 |
|------|------|--------|
| `internal/monitoring/api/pipeline/handlers.go` | 281 | `Reason: narrative.TranslateReason(item.Reason)` |
| `internal/monitoring/api/pipeline/handlers.go` | 284 | `GuardReason: narrative.TranslateReason(item.GuardReason)` |
| `internal/narrative/rationale_corpus.go` | 115, 145, 205, 219 | `TranslateReason()` 內部遞迴呼叫 |

**觸發該呼叫者的 API 端點**：
- `GET /api/dashboard/recommendation-pipeline` — 儀表板推薦管線端點，每次載入時渲染 `Reason` 和 `GuardReason` 欄位

**觸發該呼叫者的排程任務**：無（此為請求-回應 API 端點）

**觸發該呼叫者的 CLI 指令**：無

**觸發機率**：**高** — 儀表板管線頁面每次載入時，`TranslateReason()` 都會被呼叫。但因為 `RationaleTranslator` 從未被賦值（永遠為 nil），所以永遠無法抵達 LLM 程式碼路徑。

**LLM pipeline 的實際工作方式**：`TranslateReason()` 使用靜態 `reasonCorpus`（135 個中英文對照）進行查找。若未命中，則進入 passthrough seam（第 228 行），該處檢查 `RationaleTranslator != nil`。由於變數從未賦值，此檢查永遠失敗，英文文本將原樣返回。

---

### 掛載 2：`orchestrator.ScenarioExplainer`

**呼叫者函數/方法**：

| 檔案 | 行號 | 上下文 |
|------|------|--------|
| `internal/orchestrator/prism_executor.go` | 115 | `ScenarioExplainer(ctx, result)` — 計算 `TrainingResult` 後 |
| `internal/orchestrator/prism_executor.go` | 41 | `PRISMTrainingExecutor.Run()` — 執行器進入點 |
| `internal/prism/prism_manager.go` | 483 | `ex.Run(*task)` — 訓練任務反覆運算器 |

**觸發該呼叫者的 API 端點**：無（PRISM 是後台處理）

**觸發該呼叫者的排程任務**：
- `auto_swarm_simulation`（每 30 分鐘） — `cmd/atlas/main.go:2045` → `ctrl.RunSwarmCycle(baseState)` → Phase3Controller → PRISM queue
- 任何透過 `NewProductionSystemWithEventBus()` → `factory.go:66` → `prism.NewPRISMManager()` + `plugin_adapters.go:189` → `NewPRISMTrainingExecutor()` 建立 System 的排程任務

**PRISM Executor 接線路徑**：
```
cmd/atlas/main.go (auto_daily_simulation, auto_swarm_simulation, 等)
  → orchestrator.NewProductionSystemWithEventBus()
    → factory.go:66 → prism.NewPRISMManager()
    → factory.go:75 → system.WithJANUS(janusEngine, pm)
  → plugin_adapters.go:189 → p.manager.WithExecutor(NewPRISMTrainingExecutor(...))
  → prism_manager.go:483 → ex.Run(*task)
    → prism_executor.go:41 → Run()
      → prism_executor.go:115 → if ScenarioExplainer != nil { ... }
```

**觸發機率**：**中** — PRISM 訓練在 swarm 排程任務及（潛在）其他排程任務中執行。但因為 `ScenarioExplainer` 從未被賦值，所以永遠無法抵達 LLM 程式碼路徑。

---

### 掛載 3：`narrative.RegimeExplainer` + `narrative.SentimentExplainer`

**呼叫者函數/方法**：

| 檔案 | 行號 | 上下文 |
|------|------|--------|
| `internal/narrative/explain_hooks.go` | 26 | `AnnotateEvent()` — 定義 |
| `internal/narrative/explain_hooks_test.go` | 多處 | **僅限測試檔案** |

**生產環境中 `AnnotateEvent()` 的呼叫者**：**零**

**`NarrativeEvent` 的建構位置**（這些位置會產生事件，但永遠不會呼叫 `AnnotateEvent()`）：

| 檔案 | 說明 |
|------|------|
| `internal/narrative/ingestor.go` | `MacroIngestor.Ingest()` 從巨集資料變更中建構 `[]NarrativeEvent` |
| `internal/monitoring/service/narrative.go:68-69` | `NarrativeService.DetectEvents()` 呼叫 `NarrativeEngine.DetectEvents()` |
| `internal/monitoring/service/macro.go` | 巨集事件檢測與發佈 |
| `internal/eventbus/eventbus.go` | `EventNarrative` 事件型別 — 透過事件匯流排發佈 |

**`NarrativeEvent` 的處理位置**：

| 檔案 | 說明 |
|------|------|
| `internal/monitoring/api/events/sse_handler.go:75` | `BufferNarrativeEvent()` — SSE 事件緩衝區 |
| `internal/monitoring/api/narrative/handlers.go` | 敘述 API 端點 |
| `internal/narrative/report_generator.go` | 每日摘要報告產生 |

**觸發該呼叫者的 API 端點**：**無** — 沒有任何 API 端點呼叫 `AnnotateEvent()`

**觸發該呼叫者的排程任務**：**無** — 巨集攝入（`macro_ingest`）會產生 `NarrativeEvent`，但從不呼叫 `AnnotateEvent()`

**SSE 串流**：敘述事件會經過 SSE 串流發佈（`eventbus.EventNarrative`），但事件物件**永遠不會經過 `AnnotateEvent()` 處理**，因此 `Explanation` 和 `SentimentExplanation` 欄位永遠為空。

**觸發機率**：**無** — `AnnotateEvent()` 無任何生產環境呼叫者。掛載變數 `RegimeExplainer` 和 `SentimentExplainer` 必須先被賦值，且必須在事件建構之後新增對 `AnnotateEvent()` 的呼叫。

---

### 掛載 4：`risk.PerformanceForensics`

**呼叫者函數/方法**：

| 檔案 | 行號 | 上下文 |
|------|------|--------|
| `internal/risk/forensics_hook.go` | 21 | `AnnotateSnapshot()` — 定義 |
| `internal/risk/forensics_hook_test.go` | 多處 | **僅限測試檔案** |

**生產環境中 `AnnotateSnapshot()` 的呼叫者**：**零**

**`RiskSnapshot` 的建構位置**（這些位置會計算快照，但永遠不會呼叫 `AnnotateSnapshot()`）：

| 檔案 | 行號 | 觸發情境 |
|------|------|----------|
| `internal/orchestrator/system.go` | 517 | 每次每日模擬後（`RunDailySimulation`），若回報 ≥ 30 筆 |
| `internal/monitoring/service/report.go` | 299 | 回測報告載入時 |
| `internal/monitoring/api/risk/handlers.go` | 94 | `GET /api/v1/risk/snapshot` 或類似端點 |
| `internal/monitoring/api/live/handlers.go` | 473 | 即時交易儀表板端點 |
| `internal/risktest/scenarios.go` | 54, 317 | 風險測試情境 |

**觸發該呼叫者的 API 端點**：**無** — 沒有任何 API 端點呼叫 `AnnotateSnapshot()`

**觸發該呼叫者的排程任務**：**無** — `auto_daily_simulation` 會計算 `RiskSnapshot`，但從不呼叫 `AnnotateSnapshot()`

**觸發機率**：**無** — `AnnotateSnapshot()` 無任何生產環境呼叫者。函數本身的文件註解（`forensics_hook.go:19`）明確指出：「**此函數不會自動接入現有 VaR pipeline——當需要基於 LLM 的 forensics 時，呼叫者必須顯式呼叫它。**」

---

## 第三節：死碼分析

### 總結

**全部 4 個掛載目前都是死碼。** Phase 2 建立了掛載變數、helper 函數、config 旗標和測試，但 `cmd/atlas/main.go` 從未將這些 function pointers 賦值給任何 LLM client 或 router。若無這些賦值，變數將永遠保持 nil，條件檢查也永遠無法抵達 LLM 程式碼路徑。

### 掛載接線狀態

| 掛載變數 | Config 旗標 | Config 旗標存在於 config.go？ | main.go 讀取旗標？ | main.go 賦值掛載？ | 目前可觸發？ |
|-----------|------------|------------------------------|-------------------|-------------------|-------------|
| `narrative.RationaleTranslator` | `LLM_RATIONALE_TRANSLATION_ENABLED` | ✅ 第 63 行 | ❌ | ❌ | ❌ |
| `orchestrator.ScenarioExplainer` | `LLM_PRISM_SCENARIO_ENABLED` | ✅ 第 64 行 | ❌ | ❌ | ❌ |
| `narrative.RegimeExplainer` | `LLM_NARRATIVE_EXPLAIN_ENABLED` | ✅ 第 65 行 | ❌ | ❌ | ❌ |
| `narrative.SentimentExplainer` | `LLM_NARRATIVE_EXPLAIN_ENABLED` | ✅ 第 65 行 | ❌ | ❌ | ❌ |
| `risk.PerformanceForensics` | `LLM_RISK_FORENSICS_ENABLED` | ✅ 第 66 行 | ❌ | ❌ | ❌ |

### 各掛載的具體死碼狀態

1. **`narrative.RationaleTranslator`**：掛載變數已定義且有文件說明。`TranslateReason()` 在生產環境中被呼叫（透過管線 API 端點）。Passthrough seam 會檢查 `RationaleTranslator != nil`。但變數從未被賦值，因此永遠不會呼叫 LLM。

2. **`orchestrator.ScenarioExplainer`**：掛載變數已定義且有文件說明。`PRISMTrainingExecutor.Run()` 在生產環境中被呼叫（透過 swarm 排程任務）。執行後 hook 會檢查 `ScenarioExplainer != nil`。但變數從未被賦值。

3. **`narrative.RegimeExplainer` + `narrative.SentimentExplainer`**：掛載變數已定義且有文件說明。但 `AnnotateEvent()` 在生產環境中永遠不會被呼叫。即便變數被賦值，若無額外程式碼在事件建構後呼叫 `AnnotateEvent()`，hook 仍為死碼。

4. **`risk.PerformanceForensics`**：掛載變數已定義且有文件說明。`AnnotateSnapshot()` 在生產環境中永遠不會被呼叫。文件註解明確指出必須顯式呼叫。

---

## 第四節：儀表板 / API 整合

### `cmd/atlas/main.go` 是否註冊了任何會觸發 `NarrativeEvent` 處理的路由？

否。敘述事件透過事件匯流排 (`dashEventBus.Publish(...)`) 發佈，並由 SSE handler 透過 `BufferNarrativeEvent()` 吞噬。沒有任何路由在發佈後處理 `NarrativeEvent`（即呼叫 `AnnotateEvent()`）。

### SSE 事件串流是否包含 `NarrativeEvent`？

是——但僅作為原始事件物件。SSE handler（`sse_handler.go:75`）將 `eventbus.BusEvent` 封裝序列化為 JSON，並在 `event: narrative` 下作為 SSE 發送。對 `NarrativeEvent` 底層型別（內含 `Explanation` 和 `SentimentExplanation` 欄位）的**反序列化**發生在 DashEventBus 訂閱者中（`main.go:415-418` → `BufferNarrativeEvent()`）。但 `AnnotateEvent()` 永遠不會被呼叫，因此這些欄位永遠為空。

### 儀表板的 `SetStrategiesAnnotator` 是否與 `narrative.RationaleTranslator` 重疊？

**否，它們是獨立系統**：

- `dashboard.SetStrategiesAnnotator(kimi)`（main.go:1635）將 `llm_annotator.KimiClient` 接入 `<StrategiesHandlers>`，用於 `POST /api/strategies/{id}/annotate` 端點。此端點使用 Kimi 的原始 API（非 Router）為單一策略失效提供 on-demand attribution。

- `narrative.RationaleTranslator` 是一個通用翻譯 hook，用於管線 API 回應中顯示的投資理由文本。它與策略 attribution 無關。

- 主要註解（main.go:1642-1643）明確說明：「**Does NOT replace dashboard.SetStrategiesAnnotator(kimi) above — that still receives the raw *KimiClient required by dashboard_api.go:880 type assertion.**」

### 是否已有 `/api/v1/regime-explanation` 或類似端點？

**否**。搜尋結果中沒有此類端點。最接近的端點為：
- `GET /api/dashboard/recommendation-pipeline` — 渲染帶有靜態翻譯理由文本的管線項目
- `POST /api/strategies/{id}/annotate` — 策略失效的 on-demand LLM attribution
- `GET /api/v1/narrative/*` — 敘述 API 端點（存在，但使用 `NarrativeService`，非 `AnnotateEvent`）

---

## 第五節：建議

### 哪些掛載將立即觸發（僅需設定環境變數，無需程式碼變更）？

**無**。即便設定了 `LLM_*_ENABLED` 環境變數，若 main.go 未加入讀取旗標並賦值掛載的程式碼，便不會有任何效果。

### 哪些掛載需要新增呼叫者（Phase 4 工作）？

| 掛載 | 需要 main.go 賦值？ | 需要新增生產環境呼叫者？ | 預估工作量 |
|------|-------------------|---------------------|-----------|
| `RationaleTranslator` | ✅ | ❌（`TranslateReason()` 已有呼叫者） | 小 — 賦值 + 讀取旗標 |
| `ScenarioExplainer` | ✅ | ❌（`PRISMTrainingExecutor.Run()` 已有呼叫者） | 小 — 賦值 + 讀取旗標 |
| `RegimeExplainer` | ✅ | ✅（需在事件建構後新增 `AnnotateEvent()` 呼叫） | 中 — 賦值 + 新增呼叫點 |
| `SentimentExplainer` | ✅ | ✅（同上） | 中 — 賦值 + 新增呼叫點 |
| `PerformanceForensics` | ✅ | ✅（需在 `RiskSnapshot` 計算後新增 `AnnotateSnapshot()` 呼叫） | 中 — 賦值 + 新增呼叫點 |

### 哪些掛載應被移除（死碼）？

若無計畫為**全部 4 個掛載**實作接線，則應移除**全部 4 個**。它們會增加維護負擔（變數、測試、config 旗標），卻無法提供任何功能。但若接線已在路線圖中，則維持現狀——Phase 2 架構是健全的，僅需完成接線。

### 現有的 Phase 1 `/api/strategies/{id}/annotate` 端點是否應擴展以使用新的 Router？

**否**。Phase 1 annotate 端點使用 `llm_annotator.KimiClient`（透過 `SetStrategiesAnnotator`），此 client 基於原始 API key 身份驗證，獨立於 Router。Router 使用 capability-based provider chain（DeepSeek → MiniMax → OpenCode-Go），並有 DataClass gating。這兩條路徑用於不同目的：

- **Strategies Annotator**：on-demand、user-triggered、per-strategy failure attribution（PHASE 1）
- **Router**：自動化、pipeline-embedded、capability-based routing（PHASE 2+）

若未來需求需要策略 annotation 使用多 provider fallback，可將 `llm_annotator.KimiClient` 包裝為 Router adapter（類似現有的 `llm/adapters/annotator_adapter.go`），但這不在 PHASE 3 範圍內。

### 接線前綴建議

若決定接線，以下為 `cmd/atlas/main.go` 中所需的 pseudocode 模式：

```go
// Phase 3: LLM feature hook wiring (opt-in via env vars)
if cfg.LLMRationaleTranslationEnabled && llmRouter != nil {
    narrative.RationaleTranslator = func(ctx context.Context, englishText, dataClass string) (string, error) {
        resp, err := llmRouter.Call(ctx, llm.Request{
            Capability: llm.CapabilityRationaleGeneration,
            Input:      englishText,
            DataClass:  llm.DataClassPublic,
        })
        if err != nil {
            return "", err
        }
        return resp.Output, nil
    }
}

if cfg.LLMPrismScenarioEnabled && llmRouter != nil {
    orchestrator.ScenarioExplainer = func(ctx context.Context, result interface{}) (string, error) {
        resp, err := llmRouter.Call(ctx, llm.Request{
            Capability: llm.CapabilityScenarioSimulation,
            Input:      result,
            DataClass:  llm.DataClassPublic,
        })
        if err != nil {
            return "", err
        }
        return resp.Output, nil
    }
}
```

**需求確認**：`llmRouter` 必須已在 hook 賦值之前建立並完成初始化。目前 `llmRouter` 是在 `main.go:1645-1653` 建立，位置在 `SetStrategiesAnnotator` 之後。接線程式碼應放置在 router 建立之後。
